import { ViewGoneError, isViewGone, type ViewNotice } from './protocol';
import type { NoticeFeed, ViewTransport } from './transport';
import {
    ACK_TIMEOUT_MS, HEARTBEAT_MS, MISSED_HEARTBEATS, toWireError,
    type FetchedCall, type FromWorker, type ToWorker, type WireError, type WorkerMode, type WorkerStats,
} from './workerProtocol';

// The main-thread end of the view worker: owns the Worker, routes messages to the proxies,
// answers the worker's rpc calls (bindings and level-M fetches only exist here), forwards
// notices and watches the worker's health.

export interface WorkerLike {
    postMessage(msg: unknown, transfer?: Transferable[]): void;
    terminate(): void;
    onmessage: ((ev: { data: any }) => void) | null;
    onerror: ((ev: any) => void) | null;
    onmessageerror?: ((ev: any) => void) | null;
}

/** What the main thread can do on the worker's behalf. */
export interface HostServices {
    transport: ViewTransport;
    fetchBytes(path: string): Promise<{ status: number; contentType: string; bytes: ArrayBuffer }>;
}

export interface HostInitInfo {
    origin: string;
    base: string;
    token: string;
    streaming: boolean;
}

export interface FailureInfo {
    reason: string;
    /** The worker never worked (no ack, constructor threw): stepping to M would fail the same way. */
    fatal: boolean;
}

export interface ViewHostDeps {
    mode: WorkerMode;
    createWorker: () => WorkerLike;
    services: HostServices;
    notices: NoticeFeed;
    initInfo: () => HostInitInfo | null;
    onFailure: (host: ViewHost, info: FailureInfo) => void;
    now?: () => number;
    ackTimeoutMs?: number;
    heartbeatMs?: number;
    missedHeartbeats?: number;
}

/** What a proxy needs from its host. */
export interface ProxyHost {
    send(msg: ToWorker, transfer?: Transferable[]): void;
    register(id: string, sink: (m: FromWorker) => void): () => void;
    request<T>(msg: ToWorker & { rid: number }): Promise<T>;
    nextRid(): number;
}

export interface HostStatus {
    mode: WorkerMode;
    alive: boolean;
    acked: boolean;
    worker: WorkerStats | null;
    noticesForwarded: number;
}

interface Waiter {
    resolve: (v: any) => void;
    reject: (e: unknown) => void;
}

export class ViewHost implements ProxyHost {
    readonly mode: WorkerMode;
    private readonly deps: ViewHostDeps;
    private worker: WorkerLike | null = null;
    private sinks = new Map<string, (m: FromWorker) => void>();
    private waiters = new Map<number, Waiter>();
    private rid = 0;
    private acked = false;
    private alive = false;
    private lastTick = 0;
    private lastBeat = 0;
    private ackTimer: ReturnType<typeof setTimeout> | undefined;
    private watchTimer: ReturnType<typeof setInterval> | undefined;
    private offNotices: (() => void) | undefined;
    private offReconnect: (() => void) | undefined;
    private stats: WorkerStats | null = null;
    private noticeCount = 0;
    private failed = false;

    constructor(deps: ViewHostDeps) {
        this.deps = deps;
        this.mode = deps.mode;
    }

    private now(): number {
        return (this.deps.now ?? Date.now)();
    }

    /** Creates the worker and sends init. Throws when construction fails (a fatal failure). */
    start(): void {
        const info = this.deps.initInfo();
        if (!info) throw new Error('bulk transport info unavailable');
        const w = this.deps.createWorker();
        this.worker = w;
        this.alive = true;
        this.lastBeat = this.now();
        w.onmessage = (ev) => this.onMessage(ev.data as FromWorker);
        w.onerror = (ev) => this.fail(`worker error: ${ev?.message ?? 'unknown'}`, !this.acked);
        w.onmessageerror = () => this.fail('worker messageerror', false);
        const ack = this.deps.ackTimeoutMs ?? ACK_TIMEOUT_MS;
        this.ackTimer = setTimeout(() => {
            if (!this.acked) this.fail(`no init ack in ${ack}ms`, true);
        }, ack);
        const beat = this.deps.heartbeatMs ?? HEARTBEAT_MS;
        this.lastTick = this.now();
        this.watchTimer = setInterval(() => {
            const t = this.now();
            const late = t - this.lastTick > beat * 2;
            this.lastTick = t;
            if (!this.acked || !this.alive) return;
            if (late) {
                // This timer itself was late: the page was suspended (sleep, a frozen or throttled
                // tab) and the worker was paused with it, so its heartbeat is not missing, only
                // delayed. Grant a fresh window; a hung worker is still caught on later ticks.
                this.lastBeat = t;
                return;
            }
            const missed = this.deps.missedHeartbeats ?? MISSED_HEARTBEATS;
            if (t - this.lastBeat > beat * missed) this.fail(`${missed} missed heartbeats`, false);
        }, beat);
        this.offNotices = this.deps.notices.subscribe((list: ViewNotice[]) => {
            this.noticeCount += list.length;
            this.send({ t: 'notice', notices: list });
        });
        this.offReconnect = this.deps.notices.onReconnect(() => this.send({ t: 'reconnect' }));
        w.postMessage({
            t: 'init', origin: info.origin, base: info.base, token: info.token, mode: this.mode,
            streaming: info.streaming, heartbeatMs: beat,
        } satisfies ToWorker);
    }

    get isAlive(): boolean {
        return this.alive && !this.failed;
    }

    status(): HostStatus {
        return {
            mode: this.mode, alive: this.isAlive, acked: this.acked, worker: this.stats,
            noticesForwarded: this.noticeCount,
        };
    }

    // ---- ProxyHost ---------------------------------------------------------------------------

    nextRid(): number {
        return ++this.rid;
    }

    send(msg: ToWorker, transfer?: Transferable[]): void {
        if (!this.worker || !this.alive) return;
        try {
            this.worker.postMessage(msg, transfer ?? []);
        } catch (e) {
            this.fail(`postMessage failed: ${(e as Error)?.message ?? e}`, false);
        }
    }

    register(id: string, sink: (m: FromWorker) => void): () => void {
        this.sinks.set(id, sink);
        return () => {
            if (this.sinks.get(id) === sink) this.sinks.delete(id);
        };
    }

    request<T>(msg: ToWorker & { rid: number }): Promise<T> {
        return new Promise<T>((resolve, reject) => {
            if (!this.worker || !this.alive) {
                reject(new Error('view worker is not running'));
                return;
            }
            this.waiters.set(msg.rid, { resolve, reject });
            this.send(msg);
        });
    }

    /** Runs one call inside the worker (fetchBulkInWorker). */
    fetchCall(path: string): Promise<FetchedCall> {
        return this.request<FetchedCall>({ t: 'fetch.call', rid: this.nextRid(), path });
    }

    // ---- inbound -----------------------------------------------------------------------------

    private onMessage(m: FromWorker): void {
        if (this.failed) return;
        switch (m.t) {
            case 'ack':
                this.acked = true;
                this.lastBeat = this.now();
                if (this.ackTimer) clearTimeout(this.ackTimer);
                this.ackTimer = undefined;
                return;
            case 'heartbeat':
                this.lastBeat = this.now();
                this.stats = m.stats;
                return;
            case 'res': {
                const w = this.waiters.get(m.rid);
                if (!w) return;
                this.waiters.delete(m.rid);
                if (m.ok) w.resolve(m.value);
                else w.reject(toError(m.error));
                return;
            }
            case 'rpc':
                void this.serveRpc(m.rid, m.op, m.args);
                return;
            case 'fetch.failing':
                this.fail(`worker fetch failing: ${m.error}`, false);
                return;
            case 'view.state':
            case 'view.rows':
            case 'view.stale':
            case 'view.transitions':
            case 'view.error':
                this.sinks.get(m.id)?.(m);
                return;
        }
    }

    private async serveRpc(rid: number, op: string, args: unknown[]): Promise<void> {
        const t = this.deps.services.transport;
        try {
            let value: unknown;
            let transfer: Transferable[] | undefined;
            switch (op) {
                case 'open': value = await t.open(args[0] as never); break;
                case 'update': value = await t.update(args[0] as string, args[1] as never); break;
                case 'close': value = await t.close(args[0] as string); break;
                case 'setActive': value = await t.setActive(args[0] as string, args[1] as boolean); break;
                case 'window': value = await t.window(args[0] as string, args[1] as number, args[2] as number, args[3] as number); break;
                case 'refs': value = await t.refs(args[0] as string, args[1] as number, args[2] as number, undefined, args[3] as number | undefined); break;
                case 'facet': value = await t.facet(args[0] as string, args[1] as string); break;
                case 'setSelection': value = await t.setSelection(args[0] as string, args[1] as string[]); break;
                case 'getObject': value = await t.getObject(args[0] as never); break;
                case 'getObjects': value = await t.getObjects(args[0] as never); break;
                case 'bytes': {
                    const r = await this.deps.services.fetchBytes(args[0] as string);
                    value = r;
                    transfer = [r.bytes];
                    break;
                }
                default: throw new Error(`unknown rpc ${op}`);
            }
            this.send({ t: 'rpc.result', rid, ok: true, value: value ?? null }, transfer);
        } catch (e) {
            this.send({ t: 'rpc.result', rid, ok: false, error: toWireError(e, isViewGone(e)) });
        }
    }

    // ---- failure and teardown ----------------------------------------------------------------

    private fail(reason: string, fatal: boolean): void {
        if (this.failed) return;
        this.failed = true;
        this.dispose();
        this.deps.onFailure(this, { reason, fatal });
    }

    /** Stops the worker and rejects everything in flight. Safe to call twice. */
    dispose(): void {
        const w = this.worker;
        this.alive = false;
        this.worker = null;
        if (this.ackTimer) clearTimeout(this.ackTimer);
        if (this.watchTimer) clearInterval(this.watchTimer);
        this.ackTimer = this.watchTimer = undefined;
        this.offNotices?.();
        this.offReconnect?.();
        this.offNotices = this.offReconnect = undefined;
        if (w) {
            try { w.postMessage({ t: 'dispose' } satisfies ToWorker); } catch { /* already gone */ }
            try { w.terminate(); } catch { /* already gone */ }
            w.onmessage = w.onerror = null;
        }
        for (const waiter of this.waiters.values()) waiter.reject(new Error('view worker stopped'));
        this.waiters.clear();
        this.sinks.clear();
    }
}

function toError(e: WireError | undefined): Error {
    if (e?.gone) return new ViewGoneError(e.message);
    const err = new Error(e?.message ?? 'worker request failed') as Error & { status?: number };
    if (e?.status !== undefined) err.status = e.status;
    return err;
}
