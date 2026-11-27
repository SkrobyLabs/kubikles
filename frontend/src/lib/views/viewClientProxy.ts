import type { Facet, PodTransition, RowRef, ViewPatch, ViewSpec } from './protocol';
import type { RowItem } from './rowItem';
import type { ProxyHost } from './viewHost';
import type { ItemRef, ViewHandle, ViewSnapshot } from './viewClient';
import { isPackedRefs, unpackRefs, type FromWorker, type RefsResult } from './workerProtocol';

// ViewHandle over the view worker. The worker owns fetching, parsing, the row cache and delta
// application; this side keeps a mirror of only the rows near the requested range so rowAt stays
// synchronous, and applies worker messages once per animation frame.

/** Upper bound for a flush when no animation frame arrives (hidden window). */
const FLUSH_FALLBACK_MS = 100;

export interface ProxyDeps {
    requestFrame?: (cb: () => void) => unknown;
    cancelFrame?: (handle: unknown) => void;
}

let proxySeq = 0;

function emptySnapshot(): ViewSnapshot {
    return {
        status: 'idle', error: null, permanent: false, state: null, columns: [], filterable: {},
        schemaVersion: '', schema: null, epoch: 0, closedReason: null, total: 0, filtered: 0,
    };
}

/** How long a range that only shrinks waits before it is sent to the worker. */
export const RANGE_SHRINK_DELAY_MS = 250;

export function mergePatch(spec: ViewSpec, patch: ViewPatch): ViewSpec {
    return {
        ...spec,
        ...(patch.namespaces !== undefined ? { namespaces: patch.namespaces } : {}),
        ...(patch.search !== undefined ? { search: patch.search } : {}),
        ...(patch.columnFilters !== undefined ? { columnFilters: patch.columnFilters } : {}),
        ...(patch.sort !== undefined ? { sort: patch.sort } : {}),
        ...(patch.columns !== undefined ? { columns: patch.columns } : {}),
        ...(patch.locale !== undefined ? { locale: patch.locale } : {}),
        ...(patch.hide !== undefined ? { hide: patch.hide } : {}),
        ...(patch.notifyOn !== undefined ? { notifyOn: patch.notifyOn } : {}),
    };
}

export function createProxyHandle(host: ProxyHost, deps: ProxyDeps = {}): ViewHandle {
    const id = `p${++proxySeq}`;
    const requestFrame = deps.requestFrame ?? ((cb: () => void): unknown => {
        if (typeof requestAnimationFrame === 'function') return requestAnimationFrame(cb);
        return setTimeout(cb, 16);
    });
    const cancelFrame = deps.cancelFrame ?? ((h: unknown): void => {
        if (typeof cancelAnimationFrame === 'function' && typeof requestAnimationFrame === 'function') cancelAnimationFrame(h as number);
        else clearTimeout(h as ReturnType<typeof setTimeout>);
    });

    let spec: ViewSpec | null = null;
    let viewId = '';
    let selected = new Set<string>();
    let range: { start: number; end: number } | null = null;
    let shrinkTimer: ReturnType<typeof setTimeout> | undefined;
    let active = true;
    let disposed = false;

    let workerSnapshot: ViewSnapshot = emptySnapshot();
    let snapshot: ViewSnapshot = workerSnapshot;
    let epoch = 0;

    // Mirror of the rows near the range.
    let cache = new Map<string, RowItem>();
    let index = new Map<number, string>();
    let lastLoaded: { epoch: number; rows: RowItem[] } = { epoch: -1, rows: [] };

    const listeners = new Set<() => void>();
    const staleListeners = new Set<(uids: string[]) => void>();
    const transitionListeners = new Set<(events: PodTransition[]) => void>();
    const queue: FromWorker[] = [];
    let frame: unknown;
    let fallback: ReturnType<typeof setTimeout> | undefined;

    const off = host.register(id, onMessage);

    function onMessage(m: FromWorker): void {
        if (disposed) return;
        if (m.t === 'view.stale') {
            for (const l of [...staleListeners]) l(m.uids);
            return;
        }
        if (m.t === 'view.transitions') {
            for (const l of [...transitionListeners]) l(m.events);
            return;
        }
        queue.push(m);
        schedule();
    }

    function schedule(): void {
        if (frame !== undefined || disposed) return;
        frame = requestFrame(flush);
        fallback = setTimeout(flush, FLUSH_FALLBACK_MS);
    }

    function clearSchedule(): void {
        if (frame !== undefined) cancelFrame(frame);
        if (fallback) clearTimeout(fallback);
        frame = undefined;
        fallback = undefined;
    }

    /** Applies everything queued so far as one React update. */
    function flush(): void {
        clearSchedule();
        if (disposed || queue.length === 0) return;
        let changed = false;
        for (const m of queue.splice(0)) {
            if (m.t === 'view.state') {
                workerSnapshot = m.snapshot;
                changed = true;
            } else if (m.t === 'view.rows') {
                applyRows(m.start, m.order, m.rows);
                changed = true;
            } else if (m.t === 'view.error') {
                workerSnapshot = { ...workerSnapshot, error: m.error };
                changed = true;
            }
        }
        if (changed) publishSnapshot();
    }

    function applyRows(start: number, order: string[], rows: RowItem[]): void {
        const fresh = new Map<string, RowItem>();
        for (const r of rows) fresh.set(r.metadata.uid, r);
        const nextCache = new Map<string, RowItem>();
        const nextIndex = new Map<number, string>();
        for (let i = 0; i < order.length; i++) {
            const uid = order[i];
            if (!uid) continue;
            const item = fresh.get(uid) ?? cache.get(uid);
            if (!item) continue;
            nextCache.set(uid, item);
            nextIndex.set(start + i, uid);
        }
        cache = nextCache;
        index = nextIndex;
        epoch++;
    }

    function publishSnapshot(): void {
        snapshot = { ...workerSnapshot, epoch: epoch + workerSnapshot.epoch };
        for (const l of [...listeners]) l();
    }

    function publishLocal(): void {
        epoch++;
        snapshot = { ...snapshot, epoch: epoch + workerSnapshot.epoch };
        for (const l of [...listeners]) l();
    }

    const handle: ViewHandle = {
        get viewId() { return viewId; },

        async open(next: ViewSpec): Promise<void> {
            if (disposed) return;
            spec = { ...next };
            viewId = next.viewId;
            await host.request<null>({ t: 'view.open', rid: host.nextRid(), id, spec: { ...next } }).catch((e) => {
                if (disposed) return;
                workerSnapshot = { ...workerSnapshot, status: 'failed', error: (e as Error)?.message ?? String(e) };
                publishSnapshot();
            });
            if (disposed) return;
            if (range) host.send({ t: 'view.setRange', id, start: range.start, end: range.end });
            if (selected.size > 0) host.send({ t: 'view.setSelection', id, uids: [...selected] });
            if (!active) host.send({ t: 'view.setActive', id, active: false });
        },

        update(patch: ViewPatch): void {
            if (disposed || !spec) return;
            const prevColumns = spec.columns ?? [];
            spec = mergePatch(spec, patch);
            const nextColumns = patch.columns;
            const columnsChanged = !!nextColumns
                && (nextColumns.length !== prevColumns.length || nextColumns.some((c, i) => c !== prevColumns[i]));
            if (columnsChanged) {
                // The worker drops its rows for the old column set; match that now so cells keyed
                // by old columns never render against the new layout. An unchanged column set is a
                // no-op in the worker, which then sends no rows again, so the mirror must be kept.
                cache = new Map();
                index = new Map();
                publishLocal();
            }
            host.send({ t: 'view.update', id, patch });
        },

        close(): void {
            if (disposed) return;
            host.send({ t: 'view.close', id });
            disposed = true;
            if (shrinkTimer) { clearTimeout(shrinkTimer); shrinkTimer = undefined; }
            clearSchedule();
            queue.length = 0;
            off();
            cache = new Map();
            index = new Map();
            workerSnapshot = { ...workerSnapshot, status: 'closed' };
            epoch++;
            snapshot = { ...workerSnapshot, epoch: epoch + workerSnapshot.epoch };
            for (const l of [...listeners]) l();
            listeners.clear();
            staleListeners.clear();
            transitionListeners.clear();
        },

        getSnapshot: () => snapshot,

        subscribe(cb) {
            listeners.add(cb);
            return () => { listeners.delete(cb); };
        },

        setRange(start: number, end: number): void {
            if (disposed) return;
            if (shrinkTimer) { clearTimeout(shrinkTimer); shrinkTimer = undefined; }
            if (range && range.start === start && range.end === end) return;
            const next = { start: Math.max(0, start), end: Math.max(start, end) };
            const send = () => {
                shrinkTimer = undefined;
                if (disposed) return;
                range = next;
                host.send({ t: 'view.setRange', id, start: next.start, end: next.end });
            };
            // A range inside the current one only lets the mirror drop rows. A list mounting over a
            // view that is already open reports a few rows first and its full range a moment later;
            // sending the small one would drop rows that are then asked for again and show as
            // placeholders in between. So a pure shrink waits, and is dropped if the range grows back.
            if (range && next.start >= range.start && next.end <= range.end) {
                shrinkTimer = setTimeout(send, RANGE_SHRINK_DELAY_MS);
                return;
            }
            send();
        },

        rowAt(i: number): RowItem | undefined {
            const uid = index.get(i);
            return uid === undefined ? undefined : cache.get(uid);
        },

        loadedRows(): RowItem[] {
            if (lastLoaded.epoch === snapshot.epoch) return lastLoaded.rows;
            const keys = [...index.keys()].sort((a, b) => a - b);
            const rows: RowItem[] = [];
            for (const k of keys) {
                const item = cache.get(index.get(k)!);
                if (item) rows.push(item);
            }
            lastLoaded = { epoch: snapshot.epoch, rows };
            return rows;
        },

        refs(from, to) {
            return request<RefsResult>('refs', [from, to]).then(unpack);
        },

        selectAllRefs() {
            return request<RefsResult>('selectAll', []).then(unpack);
        },

        facet(column) {
            return request<Facet>('facet', [column]);
        },

        setSelection(uids) {
            selected = new Set(uids);
            if (disposed) return;
            host.send({ t: 'view.setSelection', id, uids: [...selected] });
        },

        onSelectionStale(cb) {
            staleListeners.add(cb);
            return () => { staleListeners.delete(cb); };
        },

        onTransitions(cb) {
            transitionListeners.add(cb);
            return () => { transitionListeners.delete(cb); };
        },

        getObject(item: ItemRef) {
            return request<Record<string, any>>('getObject', [item]);
        },

        getObjects(items: ItemRef[]) {
            return request<Array<Record<string, any>>>('getObjects', [items]);
        },

        refresh(): void {
            if (!disposed) host.send({ t: 'view.refresh', id });
        },

        reopen(): void {
            if (!disposed) host.send({ t: 'view.reopen', id });
        },

        setActive(next: boolean): void {
            active = next;
            if (!disposed) host.send({ t: 'view.setActive', id, active: next });
        },

        spec: () => (spec ? { ...spec } : null),
    };

    function request<T>(op: 'refs' | 'selectAll' | 'facet' | 'getObject' | 'getObjects', args: unknown[]): Promise<T> {
        if (disposed) return Promise.reject(new Error('view closed'));
        return host.request<T>({ t: 'req', rid: host.nextRid(), id, op, args });
    }

    function unpack(r: RefsResult): RowRef[] {
        return isPackedRefs(r) ? unpackRefs(r) : r;
    }

    return handle;
}
