import {
    ViewGoneError, isViewGone, isViewStale,
    type DynamicRowSchema, type Facet, type ObjectRef, type PodTransition, type RowRef, type ViewNotice, type ViewOpenResult, type ViewPatch,
    type ViewSpec, type ViewState, type ViewWindow,
} from './protocol';
import { makeRowItem, type RowItem } from './rowItem';
import type { NoticeFeed, ViewTransport } from './transport';

// The view client is a pure module: everything it touches (transport, notices, clock) is injected,
// so it runs unchanged on the main thread or inside a worker.

export const RANGE_DEBOUNCE_MS = 50;
export const SELECTION_DEBOUNCE_MS = 150;
export const SAFETY_CHECK_MS = 30_000;
export const PING_MS = 20_000;
/** A window or refs request that has not answered by then is aborted and counted as a failure. */
export const REQUEST_TIMEOUT_MS = 20_000;
export const MAX_RECOVERIES = 4;
const RECOVERY_BASE_MS = 400;
const RECOVERY_MAX_MS = 4_000;
const MAX_WINDOW_LIMIT = 1000;
const MIN_PAD = 10;

export type ViewStatus = 'idle' | 'opening' | 'open' | 'failed' | 'closed';

/** Immutable description of the client; a new object is published whenever anything changes. */
export interface ViewSnapshot {
    status: ViewStatus;
    /** Set when status is failed or the last operation failed. */
    error: string | null;
    /** True when recovery gave up: the caller should fall back to the legacy path. */
    permanent: boolean;
    state: ViewState | null;
    columns: readonly string[];
    filterable: Readonly<Record<string, boolean>>;
    schemaVersion: string;
    /** Row schema delivered with the open result of a dynamic type (custom resources); null for generated ones. */
    schema: DynamicRowSchema | null;
    /** Increases whenever rows or their order changed. */
    epoch: number;
    closedReason: string | null;
    total: number;
    filtered: number;
}

export interface ItemRef {
    uid?: string;
    ns?: string;
    name?: string;
    metadata?: { uid?: string; name?: string; namespace?: string };
}

export interface ViewHandle {
    readonly viewId: string;
    open(spec: ViewSpec): Promise<void>;
    update(patch: ViewPatch): void;
    close(): void;
    getSnapshot(): ViewSnapshot;
    subscribe(cb: () => void): () => void;
    setRange(start: number, end: number): void;
    rowAt(index: number): RowItem | undefined;
    /** Rows currently held, in index order (for width calculation). */
    loadedRows(): RowItem[];
    refs(from: number, to: number): Promise<RowRef[]>;
    selectAllRefs(): Promise<RowRef[]>;
    facet(column: string): Promise<Facet>;
    setSelection(uids: Iterable<string>): void;
    onSelectionStale(cb: (uids: string[]) => void): () => void;
    /** Pod events of a view opened with notifyOn "pods", delivered per notice (never skipped by version checks). */
    onTransitions(cb: (events: PodTransition[]) => void): () => void;
    getObject(item: ItemRef): Promise<Record<string, any>>;
    getObjects(items: ItemRef[]): Promise<Array<Record<string, any>>>;
    /** Re-fetch the current window now (explicit refresh, reconcile). */
    refresh(): void;
    /** Re-open from the retained spec (retry after an error). */
    reopen(): void;
    setActive(active: boolean): void;
    /** The retained spec, including every patch applied so far. */
    spec(): ViewSpec | null;
}

export interface ViewClientDeps {
    transport: ViewTransport;
    notices: NoticeFeed;
    now?: () => number;
    rangeDebounceMs?: number;
    selectionDebounceMs?: number;
    safetyCheckMs?: number;
    pingMs?: number;
    requestTimeoutMs?: number;
    maxRecoveries?: number;
    recoveryBaseMs?: number;
    /** True when the page is hidden; the safety fetch then only keeps the lease warm. */
    isHidden?: () => boolean;
}

interface Range {
    start: number;
    end: number;
}

function itemIdentity(item: ItemRef): { uid: string; ns: string; name: string } {
    return {
        uid: item.metadata?.uid ?? item.uid ?? '',
        ns: item.metadata?.namespace ?? item.ns ?? '',
        name: item.metadata?.name ?? item.name ?? '',
    };
}

function sameOverlays(a: ViewState['overlays'], b: ViewState['overlays']): boolean {
    if (a === b) return true;
    if (!a || !b) return false;
    const keys = Object.keys(b);
    if (keys.length !== Object.keys(a).length) return false;
    return keys.every(k => a[k]?.available === b[k].available);
}

function errorMessage(e: unknown): string {
    if (typeof e === 'string') return e;
    return (e as Error | null)?.message ?? String(e);
}

export function createViewClient(deps: ViewClientDeps): ViewHandle {
    const { transport, notices } = deps;
    const now = deps.now ?? Date.now;
    const rangeDebounceMs = deps.rangeDebounceMs ?? RANGE_DEBOUNCE_MS;
    const selectionDebounceMs = deps.selectionDebounceMs ?? SELECTION_DEBOUNCE_MS;
    const safetyCheckMs = deps.safetyCheckMs ?? SAFETY_CHECK_MS;
    const pingMs = deps.pingMs ?? PING_MS;
    const requestTimeoutMs = deps.requestTimeoutMs ?? REQUEST_TIMEOUT_MS;
    const maxRecoveries = deps.maxRecoveries ?? MAX_RECOVERIES;
    const recoveryBaseMs = deps.recoveryBaseMs ?? RECOVERY_BASE_MS;
    const isHidden = deps.isHidden ?? (() => typeof document !== 'undefined' && document.hidden);

    // Retained spec: everything needed to re-open the view after a failure.
    let spec: ViewSpec | null = null;
    let viewId = '';
    let columns: string[] = [];
    let columnsEpoch = 0;
    let status: ViewStatus = 'idle';
    let error: string | null = null;
    let permanent = false;
    let state: ViewState | null = null;
    let filterable: Record<string, boolean> = {};
    let schemaVersion = '';
    let schema: DynamicRowSchema | null = null;
    let closedReason: string | null = null;
    let epoch = 0;
    let disposed = false;
    let active = true;

    // Row cache and window index. The index maps list positions to uids and is only valid for the
    // version it was built at; rows shift whenever the version moves.
    const cache = new Map<string, RowItem>();
    const index = new Map<number, string>();
    let indexVersion = -1;
    let appliedVersion = 0;
    let selected = new Set<string>();
    let coverage: Range | null = null;
    let coverageLimit = 0;
    let range: Range | null = null;

    // Request bookkeeping.
    let ticket = 0;
    let inFlight = false;
    let fetchAgain = false;
    let forceFull = true;
    let patchInFlight = false;
    let pendingPatch: ViewPatch | null = null;
    let recoveries = 0;
    let recovering: Promise<void> | null = null;
    let windowFailures = 0;
    let specRev = 0;
    // In-flight read requests, so a superseded or closed view does not keep consuming connections.
    const windowAborts = new Set<AbortController>();
    const refsAborts = new Set<AbortController>();

    const listeners = new Set<() => void>();
    const staleListeners = new Set<(uids: string[]) => void>();
    const transitionListeners = new Set<(events: PodTransition[]) => void>();
    let rangeTimer: ReturnType<typeof setTimeout> | undefined;
    let selectionTimer: ReturnType<typeof setTimeout> | undefined;
    let recoverTimer: ReturnType<typeof setTimeout> | undefined;
    let safetyTimer: ReturnType<typeof setInterval> | undefined;
    let pingTimer: ReturnType<typeof setInterval> | undefined;
    let offNotices: (() => void) | undefined;
    let offReconnect: (() => void) | undefined;
    let lastLoaded: { epoch: number; rows: RowItem[] } = { epoch: -1, rows: [] };

    let snapshot: ViewSnapshot = buildSnapshot();

    function buildSnapshot(): ViewSnapshot {
        return {
            status, error, permanent, state, columns, filterable, schemaVersion, schema, epoch, closedReason,
            total: state?.total ?? 0, filtered: state?.filtered ?? 0,
        };
    }

    function publish(): void {
        snapshot = buildSnapshot();
        for (const l of [...listeners]) l();
    }

    function bumpRows(): void {
        epoch++;
    }

    // ---- lifecycle ---------------------------------------------------------------------------

    function attachFeeds(): void {
        if (!offNotices) offNotices = notices.subscribe(onNotices);
        if (!offReconnect) {
            offReconnect = notices.onReconnect(() => {
                if (disposed || status === 'closed') return;
                // Notices may have been lost: re-check the window, which also detects a
                // view that vanished with a server restart (410).
                if (!permanent) recoveries = 0;
                forceFull = true;
                void fetchWindow();
            });
        }
        if (!safetyTimer) safetyTimer = setInterval(() => {
            if (status !== 'open') return;
            if (isHidden()) {
                void ping();
                return;
            }
            void fetchWindow();
        }, safetyCheckMs);
        if (!pingTimer) pingTimer = setInterval(() => { if (status === 'open') void ping(); }, pingMs);
    }

    function abortAll(set: Set<AbortController>): void {
        for (const ac of [...set]) ac.abort();
        set.clear();
    }

    /** Runs a read with a deadline and an abort handle; a request that never answers cannot wedge the view. */
    async function withDeadline<T>(set: Set<AbortController>, run: (signal: AbortSignal) => Promise<T>): Promise<T> {
        const ac = new AbortController();
        set.add(ac);
        let timer: ReturnType<typeof setTimeout> | undefined;
        const timeout = new Promise<never>((_, reject) => {
            timer = setTimeout(() => {
                ac.abort();
                reject(new Error(`request timed out after ${requestTimeoutMs}ms`));
            }, requestTimeoutMs);
        });
        const work = run(ac.signal);
        work.catch(() => {}); // the loser of the race must not surface as unhandled
        try {
            return await Promise.race([work, timeout]);
        } finally {
            clearTimeout(timer);
            set.delete(ac);
        }
    }

    function detachFeeds(): void {
        abortAll(windowAborts);
        abortAll(refsAborts);
        offNotices?.(); offNotices = undefined;
        offReconnect?.(); offReconnect = undefined;
        if (safetyTimer) clearInterval(safetyTimer);
        if (pingTimer) clearInterval(pingTimer);
        safetyTimer = pingTimer = undefined;
        if (rangeTimer) clearTimeout(rangeTimer);
        if (selectionTimer) clearTimeout(selectionTimer);
        if (recoverTimer) clearTimeout(recoverTimer);
        rangeTimer = selectionTimer = recoverTimer = undefined;
    }

    // Keeps the server-side lease warm when no window call is due (hidden tab, idle list).
    async function ping(): Promise<void> {
        try {
            await transport.setActive(viewId, active);
        } catch (e) {
            // A hidden tab only pings, so a view the server dropped (a lease that expired after a
            // reconnect, a restart) is discovered here instead of by a window fetch.
            if (isViewGone(e) && !disposed && status === 'open') void recover('view expired');
        }
    }

    async function doOpen(): Promise<void> {
        if (!spec) return;
        status = 'opening';
        error = null;
        pendingPatch = null;
        const openedRev = specRev;
        publish();
        let res: ViewOpenResult;
        try {
            res = await transport.open({ ...spec, viewId });
        } catch (e) {
            if (disposed) return;
            await failOpen(e);
            return;
        }
        if (disposed) {
            void transport.close(res.viewId).catch(() => {});
            return;
        }
        viewId = res.viewId || viewId;
        columns = [...res.columns];
        columnsEpoch++;
        schemaVersion = res.schemaVersion;
        schema = res.schema ?? null;
        filterable = { ...(res.filterable ?? {}) };
        state = res.state;
        cache.clear();
        index.clear();
        indexVersion = -1;
        appliedVersion = 0;
        coverage = null;
        forceFull = true;
        status = 'open';
        error = null;
        closedReason = null;
        bumpRows();
        publish();
        attachFeeds();
        if (selected.size > 0) void sendSelection();
        if (specRev !== openedRev && spec) {
            // The query changed while the open was in flight: bring the server view up to date.
            pendingPatch = {
                namespaces: spec.namespaces, search: spec.search, columnFilters: spec.columnFilters,
                sort: spec.sort, columns: spec.columns, locale: spec.locale, hide: spec.hide,
                ...(spec.notifyOn ? { notifyOn: spec.notifyOn } : {}),
            };
            void flushPatch();
            return;
        }
        void fetchWindow();
    }

    async function failOpen(e: unknown): Promise<void> {
        error = errorMessage(e);
        if (recoveries >= maxRecoveries) {
            status = 'failed';
            permanent = true;
            publish();
            return;
        }
        status = 'failed';
        publish();
        scheduleRecovery();
    }

    function scheduleRecovery(): void {
        if (disposed || recoverTimer) return;
        const delay = Math.min(RECOVERY_MAX_MS, recoveryBaseMs * 2 ** recoveries);
        recoveries++;
        recoverTimer = setTimeout(() => {
            recoverTimer = undefined;
            if (disposed) return;
            void doOpen();
        }, delay);
    }

    /** The view is gone or unusable: re-open from the retained spec. */
    function recover(reason: string): Promise<void> {
        if (disposed || status === 'closed') return Promise.resolve();
        if (recovering) return recovering;
        if (recoveries >= maxRecoveries) {
            error = reason;
            status = 'failed';
            permanent = true;
            publish();
            return Promise.resolve();
        }
        recoveries++;
        recovering = (async () => {
            try {
                await doOpen();
            } finally {
                recovering = null;
            }
        })();
        return recovering;
    }

    async function withRecovery<T>(run: () => Promise<T>): Promise<T> {
        try {
            return await run();
        } catch (e) {
            if (!isViewGone(e) || disposed) throw e;
            await recover('view expired');
            if (status !== 'open') throw new ViewGoneError();
            return run();
        }
    }

    // ---- windows -----------------------------------------------------------------------------

    function desiredWindow(): { offset: number; limit: number } {
        const r = range ?? { start: 0, end: 50 };
        const size = Math.max(1, r.end - r.start);
        const pad = Math.max(MIN_PAD, size);
        const offset = Math.max(0, r.start - pad);
        const limit = Math.min(MAX_WINDOW_LIMIT, r.end - offset + pad);
        return { offset, limit };
    }

    function needsFetch(): boolean {
        if (!coverage || !range) return true;
        const total = state?.filtered ?? 0;
        const size = Math.max(1, range.end - range.start);
        const half = Math.max(1, Math.ceil(Math.max(MIN_PAD, size) / 2));
        const startOk = coverage.start === 0 || range.start >= coverage.start + half;
        const endOk = coverage.end >= total || range.end <= coverage.end - half;
        return !(startOk && endOk);
    }

    async function fetchWindow(): Promise<void> {
        if (disposed || status !== 'open') return;
        if (inFlight || patchInFlight) {
            fetchAgain = true;
            return;
        }
        inFlight = true;
        try {
            do {
                fetchAgain = false;
                await fetchOnce();
            } while (fetchAgain && !disposed && status === 'open' && !patchInFlight);
        } finally {
            inFlight = false;
        }
    }

    async function fetchOnce(): Promise<void> {
        const { offset, limit } = desiredWindow();
        const myTicket = ++ticket;
        const myColumnsEpoch = columnsEpoch;
        const have = forceFull ? 0 : appliedVersion;
        const rangeAtRequest = range;
        let win: ViewWindow;
        try {
            win = await withDeadline(windowAborts, signal => transport.window(viewId, offset, limit, have, signal));
        } catch (e) {
            if (disposed || myTicket !== ticket) return;
            if (isViewGone(e)) {
                windowFailures = 0;
                void recover('view expired');
                return;
            }
            windowFailures++;
            error = errorMessage(e);
            if (windowFailures >= 3) {
                windowFailures = 0;
                void recover(errorMessage(e));
            } else {
                publish();
            }
            return;
        }
        if (disposed || myTicket !== ticket || myColumnsEpoch !== columnsEpoch) return;
        // A response older than what is already applied must not roll the list back.
        if (win.version < appliedVersion && have !== 0) return;
        windowFailures = 0;
        applyWindow(win, offset, limit, have, rangeAtRequest);
    }

    function applyWindow(win: ViewWindow, requestedOffset: number, requestedLimit: number, have: number, rangeAtRequest: Range | null): void {
        const next = new Map<string, RowItem>();
        let missing = 0;
        for (const uid of win.order) {
            const wire = win.rows[uid];
            if (wire) {
                next.set(uid, makeRowItem(uid, wire, columns, cache.get(uid)));
            } else {
                const known = cache.get(uid);
                if (known) next.set(uid, known);
                else missing++;
            }
        }
        if (missing > 0 && have !== 0) {
            // The server believes we hold rows we evicted: ask for everything again.
            forceFull = true;
            fetchAgain = true;
            return;
        }
        forceFull = false;
        recoveries = 0;
        permanent = false;
        error = null;

        if (win.version !== indexVersion) index.clear();
        indexVersion = win.version;
        appliedVersion = win.version;
        for (let i = 0; i < win.order.length; i++) {
            const uid = win.order[i];
            index.set(win.offset + i, uid);
            const item = next.get(uid);
            if (item) cache.set(uid, item);
        }
        coverage = { start: win.offset, end: win.offset + win.order.length };
        // A full response that came back short means the list ends here.
        if (win.order.length < requestedLimit && win.offset === requestedOffset) {
            coverage = { start: win.offset, end: Math.max(coverage.end, win.filtered) };
        }
        coverageLimit = requestedLimit;
        state = win.state;
        trim();
        bumpRows();
        publish();
        // The viewport may have moved while this window was in flight.
        if (range !== rangeAtRequest && needsFetch() && range) fetchAgain = true;
    }

    // Bounds memory: only rows within one window of the viewport (and selected rows) stay.
    function trim(): void {
        const r = range;
        if (r) {
            // Whatever the last window covered stays while the viewport is inside it: the
            // viewport can grow (rows measured after the first render) without a refetch.
            const covered = coverage ? coverage.end - coverage.start : 0;
            const size = Math.max(MIN_PAD, r.end - r.start, covered);
            const lo = r.start - size;
            const hi = r.end + size;
            for (const i of [...index.keys()]) {
                if (i < lo || i > hi) index.delete(i);
            }
        }
        const keep = new Set<string>(index.values());
        for (const uid of selected) keep.add(uid);
        for (const uid of [...cache.keys()]) {
            if (!keep.has(uid)) cache.delete(uid);
        }
    }

    // ---- notices -----------------------------------------------------------------------------

    function onNotices(list: ViewNotice[]): void {
        if (disposed) return;
        let changed = false;
        let needFetch = false;
        for (const n of list) {
            if (n.v !== viewId) continue;
            if (n.closed) {
                if (n.closed === 'context') {
                    status = 'closed';
                    closedReason = 'context';
                    detachFeeds();
                    publish();
                    return;
                }
                if (n.closed === 'expired') {
                    recoveries = 0;
                    void recover('view expired');
                }
                // 'replaced' and 'client' are our own doing.
                continue;
            }
            if (n.selectionStale && n.selectionStale.length > 0) {
                for (const l of [...staleListeners]) l(n.selectionStale);
            }
            if (n.transitions && n.transitions.length > 0) {
                for (const l of [...transitionListeners]) l(n.transitions);
            }
            if (n.version < appliedVersion) continue;
            const prev = state;
            if (prev) {
                if (prev.total !== n.total || prev.filtered !== n.filtered || prev.phase !== n.phase ||
                    prev.loaded !== n.loaded || prev.expected !== n.expected) {
                    state = { ...prev, total: n.total, filtered: n.filtered, phase: n.phase, loaded: n.loaded, expected: n.expected };
                    changed = true;
                }
            }
            if (prev && n.overlays && !sameOverlays(state?.overlays, n.overlays)) {
                state = { ...(state ?? prev), overlays: n.overlays };
                changed = true;
            }
            const grew = !!prev && prev.filtered !== n.filtered && underfilled(n.filtered);
            if (n.version > appliedVersion && (n.windowChanged || grew)) needFetch = true;
        }
        if (changed) publish();
        if (needFetch) void fetchWindow();
    }

    // True when the covered range could hold more rows than it does.
    function underfilled(filtered: number): boolean {
        if (!coverage) return true;
        const end = coverage.start + coverageLimit;
        return coverage.end < Math.min(end, filtered);
    }

    // ---- updates -----------------------------------------------------------------------------

    function mergeSpec(patch: ViewPatch): void {
        if (!spec) return;
        specRev++;
        spec = {
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

    function samePatchValue(patch: ViewPatch): ViewPatch | null {
        if (!spec) return null;
        const out: ViewPatch = {};
        let any = false;
        for (const key of Object.keys(patch) as Array<keyof ViewPatch>) {
            const value = patch[key];
            if (value === undefined) continue;
            if (JSON.stringify(value) === JSON.stringify((spec as any)[key])) continue;
            (out as any)[key] = value;
            any = true;
        }
        return any ? out : null;
    }

    function update(patch: ViewPatch): void {
        if (disposed || !spec) return;
        const effective = samePatchValue(patch);
        if (!effective) return;
        mergeSpec(effective);
        pendingPatch = { ...(pendingPatch ?? {}), ...effective };
        if (effective.columns) {
            columnsEpoch++;
            cache.clear();
            index.clear();
            coverage = null;
        }
        // Anything in flight predates this patch.
        ticket++;
        abortAll(windowAborts);
        if (status === 'open') void flushPatch();
    }

    async function flushPatch(): Promise<void> {
        if (patchInFlight || !pendingPatch || status !== 'open') return;
        patchInFlight = true;
        try {
            while (pendingPatch && status === 'open' && !disposed) {
                const patch = pendingPatch;
                pendingPatch = null;
                try {
                    const st = await transport.update(viewId, patch);
                    if (disposed) return;
                    state = st;
                    if (patch.columns) columns = patch.columns.filter((c, i, a) => a.indexOf(c) === i);
                    forceFull = true;
                    ticket++;
                    publish();
                } catch (e) {
                    if (disposed) return;
                    if (isViewGone(e)) {
                        // The retained spec already includes this patch.
                        patchInFlight = false;
                        pendingPatch = null;
                        await recover('view expired');
                        return;
                    }
                    error = errorMessage(e);
                    publish();
                }
            }
        } finally {
            patchInFlight = false;
        }
        if (!disposed && status === 'open') void fetchWindow();
    }

    // ---- public ------------------------------------------------------------------------------

    function setRange(start: number, end: number): void {
        if (disposed) return;
        const changed = !range || range.start !== start || range.end !== end;
        if (!changed) return;
        range = { start: Math.max(0, start), end: Math.max(start, end) };
        if (status !== 'open') return;
        if (!needsFetch()) {
            // Still drop what scrolled far away.
            trim();
            return;
        }
        if (!coverage) {
            void fetchWindow();
            return;
        }
        if (rangeTimer) clearTimeout(rangeTimer);
        rangeTimer = setTimeout(() => {
            rangeTimer = undefined;
            if (needsFetch()) void fetchWindow();
        }, rangeDebounceMs);
    }

    async function sendSelection(): Promise<void> {
        if (status !== 'open') return;
        try {
            const ack = await transport.setSelection(viewId, [...selected]);
            if (ack.stale.length > 0) for (const l of [...staleListeners]) l(ack.stale);
        } catch (e) {
            if (isViewGone(e)) void recover('view expired');
        }
    }

    function toObjectRef(item: ItemRef): ObjectRef {
        const id = itemIdentity(item);
        return { type: spec?.type ?? '', namespace: id.ns, name: id.name, ...(id.uid ? { uid: id.uid } : {}) };
    }

    const handle: ViewHandle = {
        get viewId() { return viewId; },

        async open(next: ViewSpec): Promise<void> {
            if (disposed) return;
            spec = { ...next };
            viewId = next.viewId;
            await doOpen();
        },

        update,

        close(): void {
            if (disposed) return;
            disposed = true;
            const id = viewId;
            const wasOpen = status === 'open' || status === 'opening';
            status = 'closed';
            detachFeeds();
            cache.clear();
            index.clear();
            if (id && wasOpen) void Promise.resolve(transport.close(id)).catch(() => {});
            publish();
            listeners.clear();
            staleListeners.clear();
            transitionListeners.clear();
        },

        getSnapshot: () => snapshot,

        subscribe(cb) {
            listeners.add(cb);
            return () => { listeners.delete(cb); };
        },

        setRange,

        rowAt(i: number): RowItem | undefined {
            const uid = index.get(i);
            return uid === undefined ? undefined : cache.get(uid);
        },

        loadedRows(): RowItem[] {
            if (lastLoaded.epoch === epoch) return lastLoaded.rows;
            const keys = [...index.keys()].sort((a, b) => a - b);
            const rows: RowItem[] = [];
            for (const k of keys) {
                const item = cache.get(index.get(k)!);
                if (item) rows.push(item);
            }
            lastLoaded = { epoch, rows };
            return rows;
        },

        refs(from, to) {
            const limit = Math.max(0, to - from + 1);
            // The indexes come from the rows on screen: the server refuses them when the list
            // moved on since, and the window is refreshed so the user can repeat the gesture.
            const at = appliedVersion;
            return withRecovery(() => withDeadline(refsAborts, signal => transport.refs(viewId, Math.max(0, from), limit, signal, at)))
                .catch((e) => {
                    if (isViewStale(e) && !disposed) {
                        forceFull = true;
                        void fetchWindow();
                    }
                    throw e;
                });
        },

        selectAllRefs() {
            return withRecovery(() => withDeadline(refsAborts, signal => transport.refs(viewId, 0, 0, signal)));
        },

        facet(column) {
            return withRecovery(() => transport.facet(viewId, column));
        },

        setSelection(uids) {
            selected = new Set(uids);
            if (selectionTimer) clearTimeout(selectionTimer);
            selectionTimer = setTimeout(() => { selectionTimer = undefined; void sendSelection(); }, selectionDebounceMs);
        },

        onTransitions(cb) {
            transitionListeners.add(cb);
            return () => { transitionListeners.delete(cb); };
        },
        onSelectionStale(cb) {
            staleListeners.add(cb);
            return () => { staleListeners.delete(cb); };
        },

        getObject(item) {
            return transport.getObject(toObjectRef(item));
        },

        getObjects(items) {
            return transport.getObjects(items.map(toObjectRef));
        },

        refresh(): void {
            if (disposed || status !== 'open') return;
            void fetchWindow();
        },

        reopen(): void {
            if (disposed || !spec) return;
            if (recoverTimer) { clearTimeout(recoverTimer); recoverTimer = undefined; }
            recoveries = 0;
            permanent = false;
            void doOpen();
        },

        setActive(next: boolean): void {
            if (disposed) return;
            active = next;
            if (status !== 'open') return;
            void transport.setActive(viewId, next).catch(() => {});
            if (next) {
                forceFull = false;
                void fetchWindow();
            }
        },

        spec: () => (spec ? { ...spec } : null),
    };
    // `now` is part of the injected surface so a worker host can supply its own clock.
    void now;
    return handle;
}
