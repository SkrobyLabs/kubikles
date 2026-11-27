import { LineSplitter } from '../bulk/transport';
import {
    ViewGoneError, ViewStaleError, isViewGone, isViewStale, isViewWindow,
    type Facet, type ObjectRef, type RowRef, type SelectionAck, type ViewNotice, type ViewOpenResult,
    type ViewState, type ViewWindow,
} from './protocol';
import type { RowItem } from './rowItem';
import type { NoticeFeed, ViewTransport } from './transport';
import { createViewClient, type ItemRef, type ViewClientDeps, type ViewHandle, type ViewSnapshot } from './viewClient';
import {
    PACKED_REFS_THRESHOLD, packRefs, toWireError,
    type FetchedCall, type FromWorker, type RefsResult, type RpcOp, type ToWorker, type WireError,
    type WorkerInit, type WorkerStats,
} from './workerProtocol';

// The pure core of the view worker. It runs the same view client as the main thread, but its
// transport fetches and parses in this thread and its notices arrive as messages. Control calls
// (open, update, facet, ...) go over bindings that only exist on the main thread, so they are
// asked of the host through rpc messages. Nothing here touches `self`, so it runs in vitest.

type Post = (msg: FromWorker, transfer?: Transferable[]) => void;

export interface WorkerCoreDeps {
    fetchImpl?: typeof fetch;
    now?: () => number;
    createClient?: (deps: ViewClientDeps) => ViewHandle;
    /** Row padding around the requested range. */
    rowPad?: number;
    setInterval?: typeof setInterval;
    clearInterval?: typeof clearInterval;
}

const MIN_ROW_PAD = 10;
const FETCH_FAILURES_BEFORE_REPORT = 3;

/** Feeds notices and reconnects that the host forwards from the main thread's event channel. */
export class MessageFeed implements NoticeFeed {
    private noticeCbs = new Set<(n: ViewNotice[]) => void>();
    private reconnectCbs = new Set<() => void>();

    subscribe(cb: (n: ViewNotice[]) => void): () => void {
        this.noticeCbs.add(cb);
        return () => { this.noticeCbs.delete(cb); };
    }

    onReconnect(cb: () => void): () => void {
        this.reconnectCbs.add(cb);
        return () => { this.reconnectCbs.delete(cb); };
    }

    pushNotices(list: ViewNotice[]): void {
        for (const cb of [...this.noticeCbs]) cb(list);
    }

    pushReconnect(): void {
        for (const cb of [...this.reconnectCbs]) cb();
    }
}

interface Pending {
    resolve: (v: any) => void;
    reject: (e: unknown) => void;
}

function fromWireError(e: WireError | undefined): Error {
    if (e?.gone) return new ViewGoneError(e.message);
    const err = new Error(e?.message ?? 'rpc failed') as Error & { status?: number };
    if (e?.status !== undefined) err.status = e.status;
    return err;
}

export class WorkerCore {
    private readonly post: Post;
    private readonly deps: WorkerCoreDeps;
    private readonly feed = new MessageFeed();
    private readonly views = new Map<string, ViewEntry>();
    private readonly rpcs = new Map<number, Pending>();
    private rpcSeq = 0;
    private init: WorkerInit | null = null;
    private transport: ViewTransport | null = null;
    private heartbeat: ReturnType<typeof setInterval> | undefined;
    private decodeMs = 0;
    private fetches = 0;
    private fetchFailures = 0;
    private consecutiveFailures = 0;
    private reportedFailing = false;
    private disposed = false;

    constructor(post: Post, deps: WorkerCoreDeps = {}) {
        this.post = post;
        this.deps = deps;
    }

    private now(): number {
        return (this.deps.now ?? (() => (typeof performance !== 'undefined' ? performance.now() : Date.now())))();
    }

    // ---- inbound -----------------------------------------------------------------------------

    handle(msg: ToWorker): void {
        if (this.disposed) return;
        try {
            this.dispatch(msg);
        } catch (e) {
            const id = (msg as { id?: string }).id;
            this.post({ t: 'view.error', id: id ?? '', error: (e as Error)?.message ?? String(e) });
        }
    }

    private dispatch(msg: ToWorker): void {
        switch (msg.t) {
            case 'init': return this.onInit(msg);
            case 'view.open': return void this.onOpen(msg.rid, msg.id, msg.spec);
            case 'view.update': return this.view(msg.id)?.handle.update(msg.patch);
            case 'view.close': return this.onClose(msg.id);
            case 'view.setRange': return this.onRange(msg.id, msg.start, msg.end);
            case 'view.setSelection': return this.view(msg.id)?.handle.setSelection(msg.uids);
            case 'view.setActive': return this.onActive(msg.id, msg.active);
            case 'view.refresh': return this.view(msg.id)?.handle.refresh();
            case 'view.reopen': return this.view(msg.id)?.handle.reopen();
            case 'req': return void this.onRequest(msg.rid, msg.id, msg.op, msg.args);
            case 'notice': return this.feed.pushNotices(msg.notices);
            case 'reconnect': return this.feed.pushReconnect();
            case 'fetch.call': return void this.onFetchCall(msg.rid, msg.path);
            case 'rpc.result': return this.onRpcResult(msg.rid, msg.ok, msg.value, msg.error);
            case 'dispose': return this.dispose();
        }
    }

    private onInit(init: WorkerInit): void {
        this.init = init;
        this.transport = this.makeTransport(init);
        if (!this.heartbeat) {
            const si = this.deps.setInterval ?? setInterval;
            this.heartbeat = si(() => this.beat(), init.heartbeatMs);
        }
        this.post({ t: 'ack' });
        this.beat();
    }

    private beat(): void {
        this.post({ t: 'heartbeat', stats: this.stats() });
    }

    stats(): WorkerStats {
        let rows = 0;
        for (const v of this.views.values()) rows += v.sent.size;
        return {
            views: this.views.size, rowsResident: rows, decodeMs: Math.round(this.decodeMs * 10) / 10,
            fetches: this.fetches, fetchFailures: this.fetchFailures,
        };
    }

    dispose(): void {
        if (this.disposed) return;
        this.disposed = true;
        const ci = this.deps.clearInterval ?? clearInterval;
        if (this.heartbeat) ci(this.heartbeat);
        this.heartbeat = undefined;
        for (const v of [...this.views.values()]) v.off();
        for (const v of [...this.views.values()]) v.handle.close();
        this.views.clear();
        for (const p of this.rpcs.values()) p.reject(new Error('worker disposed'));
        this.rpcs.clear();
    }

    // ---- rpc to the host ---------------------------------------------------------------------

    private rpc<T>(op: RpcOp, args: unknown[]): Promise<T> {
        const rid = ++this.rpcSeq;
        return new Promise<T>((resolve, reject) => {
            this.rpcs.set(rid, { resolve, reject });
            this.post({ t: 'rpc', rid, op, args });
        });
    }

    private onRpcResult(rid: number, ok: boolean, value: unknown, error?: WireError): void {
        const p = this.rpcs.get(rid);
        if (!p) return;
        this.rpcs.delete(rid);
        if (ok) p.resolve(value);
        else p.reject(fromWireError(error));
    }

    // ---- fetching ----------------------------------------------------------------------------

    private parseJson<T>(text: string): T {
        const t0 = this.now();
        try {
            return JSON.parse(text) as T;
        } finally {
            this.decodeMs += this.now() - t0;
        }
    }

    private noteFetch(ok: boolean, error = ''): void {
        this.fetches++;
        if (ok) {
            this.consecutiveFailures = 0;
            return;
        }
        this.fetchFailures++;
        this.consecutiveFailures++;
        if (this.consecutiveFailures >= FETCH_FAILURES_BEFORE_REPORT && !this.reportedFailing && this.init?.mode === 'W') {
            this.reportedFailing = true;
            this.post({ t: 'fetch.failing', error });
        }
    }

    /** One GET. Level W fetches here; level M asks the host for the bytes. */
    private async get(path: string, signal?: AbortSignal): Promise<{ status: number; contentType: string; body: Response | null; bytes: ArrayBuffer | null }> {
        const init = this.init!;
        if (init.mode === 'M') {
            const r = await this.rpc<{ status: number; contentType: string; bytes: ArrayBuffer }>('bytes', [path]);
            return { status: r.status, contentType: r.contentType, body: null, bytes: r.bytes };
        }
        const f = this.deps.fetchImpl ?? fetch;
        const resp = await f(`${init.origin}${init.base}/${path.replace(/^\//, '')}`, {
            method: 'GET',
            headers: { 'X-Kubikles': '1', 'X-Kubikles-Token': init.token },
            cache: 'no-store',
            signal,
        });
        return { status: resp.status, contentType: resp.headers.get('Content-Type') || '', body: resp, bytes: null };
    }

    private async readText(r: { body: Response | null; bytes: ArrayBuffer | null }): Promise<string> {
        if (r.bytes) return new TextDecoder().decode(r.bytes);
        return r.body!.text();
    }

    private async *readLines(r: { body: Response | null; bytes: ArrayBuffer | null }): AsyncGenerator<string> {
        const streaming = !!this.init?.streaming && !!r.body?.body && typeof r.body.body.getReader === 'function';
        if (!streaming) {
            const text = await this.readText(r);
            for (const line of text.split('\n')) {
                const l = line.endsWith('\r') ? line.slice(0, -1) : line;
                if (l.length > 0) yield l;
            }
            return;
        }
        const reader = r.body!.body!.getReader();
        const splitter = new LineSplitter();
        try {
            for (;;) {
                const { done, value } = await reader.read();
                if (done) break;
                if (value) for (const l of splitter.push(value)) yield l;
            }
            for (const l of splitter.end()) yield l;
        } finally {
            try { reader.releaseLock(); } catch { /* already released */ }
        }
    }

    private async errorOf(r: { status: number; body: Response | null; bytes: ArrayBuffer | null }): Promise<string> {
        try {
            const j = JSON.parse(await this.readText(r));
            if (j && typeof j.error === 'string') return j.error;
        } catch { /* not JSON */ }
        return `HTTP ${r.status}`;
    }

    private async fetchJson<T>(path: string, signal?: AbortSignal): Promise<T> {
        let r;
        try {
            r = await this.get(path, signal);
        } catch (e) {
            this.noteFetch(false, (e as Error)?.message ?? String(e));
            throw e;
        }
        if (r.status === 410) {
            this.noteFetch(true);
            throw new ViewGoneError();
        }
        if (r.status < 200 || r.status > 299) {
            // The route answered: an application error, not a transport failure.
            const msg = await this.errorOf(r);
            this.noteFetch(true);
            throw new Error(msg);
        }
        let text: string;
        try {
            text = await this.readText(r);
        } catch (e) {
            this.noteFetch(false, (e as Error)?.message ?? String(e));
            throw e;
        }
        const v = this.parseJson<T>(text);
        this.noteFetch(true);
        return v;
    }

    private makeTransport(init: WorkerInit): ViewTransport {
        void init;
        const enc = encodeURIComponent;
        const gone = async <T>(run: () => Promise<T>): Promise<T> => {
            try { return await run(); } catch (e) { if (isViewGone(e)) throw new ViewGoneError(); throw e; }
        };
        return {
            open: spec => this.rpc<ViewOpenResult>('open', [spec]),
            update: (id, patch) => gone(() => this.rpc<ViewState>('update', [id, patch])),
            close: id => this.rpc<void>('close', [id]),
            setActive: (id, active) => this.rpc<void>('setActive', [id, active]),

            window: async (id, offset, limit, have, signal) => {
                try {
                    const w = await this.fetchJson<ViewWindow>(`views/${enc(id)}/window?offset=${offset}&limit=${limit}&have=${have}`, signal);
                    if (!isViewWindow(w)) throw new Error('malformed window');
                    return w;
                } catch (e) {
                    if (isViewGone(e)) throw new ViewGoneError();
                    if (signal?.aborted) throw e;
                    // The IPC twin: small payloads, and the main thread's own bulk fallback chain.
                    return gone(() => this.rpc<ViewWindow>('window', [id, offset, limit, have]));
                }
            },

            refs: async (id, offset, limit, signal, version) => {
                try {
                    const v = version ? `&version=${version}` : '';
                    return await this.fetchRefs(`views/${enc(id)}/refs?offset=${offset}&limit=${limit}${v}`, signal);
                } catch (e) {
                    if (isViewGone(e)) throw new ViewGoneError();
                    if (isViewStale(e)) throw new ViewStaleError();
                    if (signal?.aborted) throw e;
                    return gone(() => this.rpc<RowRef[]>('refs', [id, offset, limit, version ?? 0]));
                }
            },

            facet: (id, column) => gone(() => this.rpc<Facet>('facet', [id, column])),
            setSelection: (id, uids) => gone(() => this.rpc<SelectionAck>('setSelection', [id, uids])),

            getObject: ref => this.fetchObject(ref),
            getObjects: async refs => {
                const out: Array<Record<string, any>> = new Array(refs.length);
                let next = 0;
                const run = async () => {
                    for (;;) {
                        const i = next++;
                        if (i >= refs.length) return;
                        out[i] = await this.fetchObject(refs[i]);
                    }
                };
                await Promise.all(Array.from({ length: Math.min(6, refs.length) }, run));
                return out;
            },
        };
    }

    private async fetchObject(ref: ObjectRef): Promise<Record<string, any>> {
        const q = new URLSearchParams({ ns: ref.namespace ?? '', name: ref.name });
        if (ref.uid) q.set('uid', ref.uid);
        try {
            return await this.fetchJson<Record<string, any>>(`objects/${encodeURIComponent(ref.type)}?${q.toString()}`);
        } catch {
            return this.rpc<Record<string, any>>('getObject', [ref]);
        }
    }

    private async fetchRefs(path: string, signal?: AbortSignal): Promise<RowRef[]> {
        let r;
        try {
            r = await this.get(path, signal);
        } catch (e) {
            this.noteFetch(false, (e as Error)?.message ?? String(e));
            throw e;
        }
        if (r.status === 410) {
            this.noteFetch(true);
            throw new ViewGoneError();
        }
        if (r.status === 409) {
            this.noteFetch(true);
            throw new ViewStaleError();
        }
        if (r.status < 200 || r.status > 299) {
            // The route answered: an application error, not a transport failure.
            const msg = await this.errorOf(r);
            this.noteFetch(true);
            throw new Error(msg);
        }
        const out: RowRef[] = [];
        let first = true;
        try {
            for await (const line of this.readLines(r)) {
                const v = this.parseJson<unknown>(line);
                if (first) { first = false; continue; }
                const [uid, ns, name] = v as [string, string, string];
                out.push({ uid, ns, name });
            }
        } catch (e) {
            this.noteFetch(false, (e as Error)?.message ?? String(e));
            throw e;
        }
        this.noteFetch(true);
        return out;
    }

    private async onFetchCall(rid: number, path: string): Promise<void> {
        try {
            const r = await this.get(path);
            const result: FetchedCall = { status: r.status, ok: r.status >= 200 && r.status <= 299 };
            if (result.ok && r.contentType.startsWith('application/x-ndjson')) {
                const items: unknown[] = [];
                for await (const line of this.readLines(r)) items.push(this.parseJson<unknown>(line));
                result.items = items;
            } else {
                try {
                    result.doc = this.parseJson<FetchedCall['doc']>(await this.readText(r));
                } catch {
                    result.doc = null;
                }
            }
            this.noteFetch(true);
            this.post({ t: 'res', rid, ok: true, value: result });
        } catch (e) {
            this.noteFetch(false, (e as Error)?.message ?? String(e));
            this.post({ t: 'res', rid, ok: false, error: toWireError(e, false) });
        }
    }

    // ---- views -------------------------------------------------------------------------------

    private view(id: string): ViewEntry | undefined {
        return this.views.get(id);
    }

    private async onOpen(rid: number, id: string, spec: Parameters<ViewHandle['open']>[0]): Promise<void> {
        let entry = this.views.get(id);
        if (!entry) {
            entry = this.createView(id);
            this.views.set(id, entry);
        }
        try {
            await entry.handle.open(spec);
            this.post({ t: 'res', rid, ok: true, value: null });
        } catch (e) {
            this.post({ t: 'res', rid, ok: false, error: toWireError(e, false) });
        }
    }

    private createView(id: string): ViewEntry {
        const make = this.deps.createClient ?? createViewClient;
        const entry: ViewEntry = {
            id,
            handle: null as unknown as ViewHandle,
            range: null,
            sent: new Map(),
            lastSnapshot: null,
            lastEpoch: -1,
            lastWindow: '',
            active: true,
            off: () => {},
        };
        entry.handle = make({
            transport: this.transport!,
            notices: this.feed,
            isHidden: () => !entry.active,
        });
        const offSnap = entry.handle.subscribe(() => this.publish(entry));
        const offStale = entry.handle.onSelectionStale(uids => this.post({ t: 'view.stale', id, uids }));
        const offTransitions = entry.handle.onTransitions(events => this.post({ t: 'view.transitions', id, events }));
        entry.off = () => { offSnap(); offStale(); offTransitions(); };
        return entry;
    }

    private onClose(id: string): void {
        const entry = this.views.get(id);
        if (!entry) return;
        entry.off();
        this.views.delete(id);
        entry.handle.close();
    }

    private onActive(id: string, active: boolean): void {
        const entry = this.views.get(id);
        if (!entry) return;
        entry.active = active;
        entry.handle.setActive(active);
    }

    private onRange(id: string, start: number, end: number): void {
        const entry = this.views.get(id);
        if (!entry) return;
        entry.range = { start, end };
        entry.handle.setRange(start, end);
        this.emitRows(entry, false);
    }

    /** Rows around the requested range are mirrored; the pad covers Virtuoso overscan. */
    private paddedRange(entry: ViewEntry): { start: number; end: number } | null {
        if (!entry.range) return null;
        const size = Math.max(1, entry.range.end - entry.range.start);
        const pad = this.deps.rowPad ?? Math.max(MIN_ROW_PAD, size);
        return { start: Math.max(0, entry.range.start - pad), end: entry.range.end + pad };
    }

    private publish(entry: ViewEntry): void {
        const snap = entry.handle.getSnapshot();
        if (snap !== entry.lastSnapshot) {
            entry.lastSnapshot = snap;
            this.post({ t: 'view.state', id: entry.id, snapshot: snap });
        }
        this.emitRows(entry, false);
    }

    private emitRows(entry: ViewEntry, force: boolean): void {
        const r = this.paddedRange(entry);
        const snap = entry.handle.getSnapshot();
        if (!r) return;
        const key = `${r.start}:${r.end}`;
        if (!force && snap.epoch === entry.lastEpoch && key === entry.lastWindow) return;
        entry.lastEpoch = snap.epoch;
        entry.lastWindow = key;

        const order: string[] = [];
        const rows: RowItem[] = [];
        const nextSent = new Map<string, RowItem>();
        let lastLoaded = -1;
        for (let i = r.start; i < r.end; i++) {
            const item = entry.handle.rowAt(i);
            if (!item) {
                order.push('');
                continue;
            }
            lastLoaded = i - r.start;
            const uid = item.metadata.uid;
            order.push(uid);
            nextSent.set(uid, item);
            if (entry.sent.get(uid) !== item) rows.push(item);
        }
        order.length = lastLoaded + 1;
        entry.sent = nextSent;
        this.post({ t: 'view.rows', id: entry.id, start: r.start, order, rows });
    }

    // ---- requests ----------------------------------------------------------------------------

    private async onRequest(rid: number, id: string, op: string, args: unknown[]): Promise<void> {
        const entry = this.views.get(id);
        try {
            if (!entry) throw new ViewGoneError();
            const h = entry.handle;
            let value: unknown;
            switch (op) {
                case 'refs': {
                    const [from, to] = args as [number, number];
                    value = this.maybePack(await h.refs(from, to));
                    break;
                }
                case 'selectAll':
                    value = this.maybePack(await h.selectAllRefs());
                    break;
                case 'facet':
                    value = await h.facet(args[0] as string);
                    break;
                case 'getObject':
                    value = await h.getObject(args[0] as ItemRef);
                    break;
                case 'getObjects':
                    value = await h.getObjects(args[0] as ItemRef[]);
                    break;
                default:
                    throw new Error(`unknown request ${op}`);
            }
            if (value && typeof value === 'object' && (value as { packed?: boolean }).packed) {
                const p = value as { index: ArrayBuffer };
                this.post({ t: 'res', rid, ok: true, value }, [p.index]);
            } else {
                this.post({ t: 'res', rid, ok: true, value });
            }
        } catch (e) {
            this.post({ t: 'res', rid, ok: false, error: toWireError(e, isViewGone(e)) });
        }
    }

    private maybePack(refs: RowRef[]): RefsResult {
        return refs.length > PACKED_REFS_THRESHOLD ? packRefs(refs) : refs;
    }
}

interface ViewEntry {
    id: string;
    handle: ViewHandle;
    range: { start: number; end: number } | null;
    /** Rows the main thread already holds, by uid. Compared by identity with the client's cache. */
    sent: Map<string, RowItem>;
    lastSnapshot: ViewSnapshot | null;
    lastEpoch: number;
    lastWindow: string;
    active: boolean;
    off: () => void;
}
