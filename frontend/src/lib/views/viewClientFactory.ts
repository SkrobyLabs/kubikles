import { bulkFetchBytes, getBulkCaps, peekBulkCaps, peekBulkInfo, setWorkerCallFetcher } from '../bulk/transport';
import type { BulkCaps } from '../bulk/types';
import type { PodTransition, ViewPatch, ViewSpec } from './protocol';
import type { RowItem } from './rowItem';
import { createDefaultTransport, getNoticeFeed, type NoticeFeed } from './transport';
import { createViewClient, type ItemRef, type ViewHandle, type ViewSnapshot } from './viewClient';
import { createProxyHandle, mergePatch, type ProxyDeps } from './viewClientProxy';
import { ViewHost, type FailureInfo, type HostInitInfo, type HostServices, type WorkerLike } from './viewHost';
import type { FetchedCall, WorkerMode, WorkerStats } from './workerProtocol';

// Chooses where a view's engine runs and keeps it running there.
//   W  a worker fetches, parses and applies deltas itself
//   M  a worker applies, the main thread fetches and transfers the bytes
//   I  the engine runs inline on the main thread (still fetch based, bindings as fallback)
// A failing host steps every view down one level (a worker that never worked goes straight to I)
// and re-opens each view from its retained spec, range and selection.

export type ViewLevel = 'W' | 'M' | 'I';
export type ViewWorkerSetting = 'auto' | 'off';

const ORDER: Record<ViewLevel, number> = { W: 2, M: 1, I: 0 };
const SETTINGS_KEY = 'kubikles_settings';
const PROBE_WAIT_MS = 3000;
const WATCHDOG_LOG_LIMIT = 20;

export function readViewWorkerSetting(): ViewWorkerSetting {
    try {
        const raw = localStorage.getItem(SETTINGS_KEY);
        if (!raw) return 'auto';
        return JSON.parse(raw)?.performance?.viewWorker === 'off' ? 'off' : 'auto';
    } catch {
        return 'auto';
    }
}

/** Pure gating: probe result x setting x worker support. */
export function chooseLevel(caps: BulkCaps, setting: ViewWorkerSetting, workerSupported: boolean): ViewLevel {
    if (setting === 'off' || !workerSupported) return 'I';
    if (!caps.enabled || !caps.fetch || caps.degraded) return 'I';
    if (caps.workerBlob && caps.workerFetch) return 'W';
    if (caps.workerBlob) return 'M';
    return 'I';
}

function lower(level: ViewLevel, ceiling: ViewLevel): ViewLevel {
    return ORDER[level] <= ORDER[ceiling] ? level : ceiling;
}

function nextBelow(mode: WorkerMode): ViewLevel {
    return mode === 'W' ? 'M' : 'I';
}

export interface WatchdogEvent {
    at: number;
    mode: WorkerMode;
    reason: string;
    to: ViewLevel;
}

export interface FactoryStatus {
    setting: ViewWorkerSetting;
    /** Highest level still allowed this session (lowered by the watchdog). */
    ceiling: ViewLevel;
    /** Level chosen for the most recent view. */
    level: ViewLevel | null;
    views: number;
    viewsOpen: number;
    noticesPerSec: number;
    worker: WorkerStats | null;
    watchdog: WatchdogEvent[];
}

export interface FactoryDeps {
    getCaps: () => Promise<BulkCaps>;
    /** Caps as currently known, without waiting (defaults to the transport's snapshot). */
    peekCaps?: () => BulkCaps;
    getSetting: () => ViewWorkerSetting;
    workerSupported: () => boolean;
    createWorker: () => Promise<WorkerLike> | WorkerLike;
    services: () => HostServices;
    notices: () => NoticeFeed;
    initInfo: () => HostInitInfo | null;
    makeInline: () => ViewHandle;
    now?: () => number;
    ackTimeoutMs?: number;
    heartbeatMs?: number;
    missedHeartbeats?: number;
    proxy?: ProxyDeps;
}

function idleSnapshot(status: ViewSnapshot['status'], epoch: number, columns: readonly string[] = []): ViewSnapshot {
    return {
        status, error: null, permanent: false, state: null, columns, filterable: {}, schemaVersion: '', schema: null,
        epoch, closedReason: null, total: 0, filtered: 0,
    };
}

export class ViewFactory {
    private readonly deps: FactoryDeps;
    private ceiling: ViewLevel = 'W';
    private lastLevel: ViewLevel | null = null;
    private readonly hosts = new Map<WorkerMode, ViewHost>();
    private readonly hostStarts = new Map<WorkerMode, Promise<ViewHost | null>>();
    private readonly handles = new Set<SwitchableHandle>();
    private readonly listeners = new Set<() => void>();
    private readonly watchdog: WatchdogEvent[] = [];
    private version = 0;
    private noticeCount = 0;
    private offNotices: (() => void) | undefined;
    private rate = { at: 0, count: 0, perSec: 0 };

    constructor(deps: FactoryDeps) {
        this.deps = deps;
    }

    now(): number {
        return (this.deps.now ?? Date.now)();
    }

    createHandle(): ViewHandle {
        const h = new SwitchableHandle(this);
        this.handles.add(h);
        this.attachCounter();
        this.changed();
        return h;
    }

    /** @internal */
    unregister(h: SwitchableHandle): void {
        this.handles.delete(h);
        if (this.handles.size === 0) {
            this.offNotices?.();
            this.offNotices = undefined;
        }
        this.changed();
    }

    private attachCounter(): void {
        if (this.offNotices) return;
        this.offNotices = this.deps.notices().subscribe(list => { this.noticeCount += list.length; });
    }

    // ---- level decisions ---------------------------------------------------------------------

    /** The level a view opened now should use. */
    async resolveLevel(): Promise<ViewLevel> {
        const caps = await this.deps.getCaps();
        const level = lower(chooseLevel(caps, this.deps.getSetting(), this.deps.workerSupported()), this.ceiling);
        this.lastLevel = level;
        this.changed();
        return level;
    }

    /** @internal */
    async ensureHost(mode: WorkerMode): Promise<ViewHost | null> {
        const existing = this.hosts.get(mode);
        if (existing?.isAlive) return existing;
        const pending = this.hostStarts.get(mode);
        if (pending) return pending;
        const start = (async (): Promise<ViewHost | null> => {
            let host: ViewHost | null = null;
            try {
                const worker = await this.deps.createWorker();
                host = new ViewHost({
                    mode,
                    createWorker: () => worker,
                    services: this.deps.services(),
                    notices: this.deps.notices(),
                    initInfo: this.deps.initInfo,
                    onFailure: (h, info) => this.onHostFailure(h, info),
                    now: this.deps.now,
                    ackTimeoutMs: this.deps.ackTimeoutMs,
                    heartbeatMs: this.deps.heartbeatMs,
                    missedHeartbeats: this.deps.missedHeartbeats,
                });
                this.hosts.set(mode, host);
                host.start();
                return host;
            } catch (e) {
                if (host) this.hosts.delete(mode);
                host?.dispose();
                this.recordFailure(mode, `worker start failed: ${(e as Error)?.message ?? e}`, true);
                return null;
            } finally {
                this.hostStarts.delete(mode);
            }
        })();
        this.hostStarts.set(mode, start);
        return start;
    }

    private onHostFailure(host: ViewHost, info: FailureInfo): void {
        if (this.hosts.get(host.mode) === host) this.hosts.delete(host.mode);
        this.recordFailure(host.mode, info.reason, info.fatal);
        for (const h of [...this.handles]) h.hostLost(host);
    }

    private recordFailure(mode: WorkerMode, reason: string, fatal: boolean): void {
        const to: ViewLevel = fatal ? 'I' : nextBelow(mode);
        this.ceiling = lower(this.ceiling, to);
        this.watchdog.push({ at: this.now(), mode, reason, to });
        if (this.watchdog.length > WATCHDOG_LOG_LIMIT) this.watchdog.shift();
        this.changed();
    }

    build(level: ViewLevel, host: ViewHost | null): ViewHandle {
        if (level === 'I' || !host) return this.deps.makeInline();
        return createProxyHandle(host, this.deps.proxy);
    }

    // ---- bulk calls in the worker ------------------------------------------------------------

    /** Runs a call fetch inside the worker when the session level is W; null otherwise. */
    async fetchCallInWorker(path: string): Promise<FetchedCall | null> {
        const caps = (this.deps.peekCaps ?? peekBulkCaps)();
        const level = lower(chooseLevel(caps, this.deps.getSetting(), this.deps.workerSupported()), this.ceiling);
        if (level !== 'W') return null;
        const host = await this.ensureHost('W');
        if (!host?.isAlive) return null;
        return host.fetchCall(path);
    }

    // ---- status ------------------------------------------------------------------------------

    changed(): void {
        this.version++;
        for (const l of [...this.listeners]) l();
    }

    subscribe(cb: () => void): () => void {
        this.listeners.add(cb);
        return () => { this.listeners.delete(cb); };
    }

    getVersion(): number {
        return this.version;
    }

    status(): FactoryStatus {
        const t = this.now();
        if (this.rate.at === 0) this.rate = { at: t, count: this.noticeCount, perSec: 0 };
        else if (t - this.rate.at >= 500) {
            this.rate = { at: t, count: this.noticeCount, perSec: ((this.noticeCount - this.rate.count) * 1000) / (t - this.rate.at) };
        }
        let worker: WorkerStats | null = null;
        for (const h of this.hosts.values()) worker = h.status().worker ?? worker;
        let open = 0;
        for (const h of this.handles) if (h.getSnapshot().status === 'open') open++;
        return {
            setting: this.deps.getSetting(),
            ceiling: this.ceiling,
            level: this.lastLevel,
            views: this.handles.size,
            viewsOpen: open,
            noticesPerSec: Math.round(this.rate.perSec * 10) / 10,
            worker,
            watchdog: [...this.watchdog],
        };
    }

    /** Forget degradation (tests, or a manual re-probe). */
    resetCeiling(): void {
        this.ceiling = 'W';
        this.changed();
    }

    dispose(): void {
        for (const h of [...this.handles]) h.close();
        for (const host of [...this.hosts.values()]) host.dispose();
        this.hosts.clear();
        this.offNotices?.();
        this.offNotices = undefined;
    }
}

/**
 * A ViewHandle whose engine can move: it retains what it takes to rebuild the view (spec with all
 * patches, range, selection, activity) and replays it on a new inner handle when the host fails.
 */
class SwitchableHandle implements ViewHandle {
    private readonly factory: ViewFactory;
    private inner: ViewHandle | null = null;
    private innerHost: ViewHost | null = null;
    private innerOffs: Array<() => void> = [];
    private spec_: ViewSpec | null = null;
    private range: { start: number; end: number } | null = null;
    private selected = new Set<string>();
    private active = true;
    private disposed = false;
    private gen = 0;
    private epochBase = 0;
    private exposed: ViewSnapshot = idleSnapshot('idle', 0);
    private exposedFrom: ViewSnapshot | null = null;
    private exposedBase = -1;
    private readonly listeners = new Set<() => void>();
    private readonly staleListeners = new Set<(uids: string[]) => void>();
    private readonly transitionListeners = new Set<(events: PodTransition[]) => void>();

    constructor(factory: ViewFactory) {
        this.factory = factory;
        // Callers pass methods around unbound (useSyncExternalStore gets view.subscribe).
        const self = this as unknown as Record<string, unknown>;
        for (const name of ['update', 'close', 'getSnapshot', 'subscribe', 'setRange', 'rowAt', 'loadedRows', 'refs',
            'selectAllRefs', 'facet', 'setSelection', 'onSelectionStale', 'onTransitions', 'getObject', 'getObjects', 'refresh',
            'reopen', 'setActive', 'spec', 'open']) {
            self[name] = (self[name] as (...a: unknown[]) => unknown).bind(this);
        }
    }

    get viewId(): string {
        return this.inner?.viewId || this.spec_?.viewId || '';
    }

    async open(spec: ViewSpec): Promise<void> {
        if (this.disposed) return;
        this.spec_ = { ...spec };
        this.exposed = idleSnapshot('opening', this.epochBase);
        this.exposedFrom = null;
        this.notify();
        await this.start();
    }

    private async start(): Promise<void> {
        const gen = ++this.gen;
        const level = await this.factory.resolveLevel();
        if (this.disposed || gen !== this.gen) return;
        const mode: WorkerMode | null = level === 'I' ? null : level;
        let host: ViewHost | null = null;
        if (mode) {
            host = await this.factory.ensureHost(mode);
            if (this.disposed || gen !== this.gen) return;
            if (!host || !host.isAlive) {
                // The host failed to start or died meanwhile; the factory has lowered its ceiling.
                return this.start();
            }
        }
        const inner = this.factory.build(level, host);
        this.attach(inner, host);
        // Replay everything but the spec, then open from the retained spec.
        if (this.range) inner.setRange(this.range.start, this.range.end);
        if (this.selected.size > 0) inner.setSelection([...this.selected]);
        if (!this.active) inner.setActive(false);
        await inner.open({ ...this.spec_! });
    }

    private attach(inner: ViewHandle, host: ViewHost | null): void {
        this.detach(false);
        this.inner = inner;
        this.innerHost = host;
        this.innerOffs = [
            inner.subscribe(() => this.notify()),
            inner.onSelectionStale(uids => { for (const l of [...this.staleListeners]) l(uids); }),
            inner.onTransitions(events => { for (const l of [...this.transitionListeners]) l(events); }),
        ];
        this.notify();
    }

    private detach(closeInner: boolean): void {
        for (const off of this.innerOffs) off();
        this.innerOffs = [];
        if (this.inner) {
            // Keep the epoch monotone across engines so memoised row tables refresh.
            this.epochBase = this.exposedEpoch() + 1;
            if (closeInner) this.inner.close();
        }
        this.inner = null;
        this.innerHost = null;
    }

    private exposedEpoch(): number {
        return this.getSnapshot().epoch;
    }

    /** @internal Called by the factory when a host this handle may be using went away. */
    hostLost(host: ViewHost): void {
        if (this.disposed || this.innerHost !== host) return;
        const had = this.inner;
        this.detach(false);
        try { had?.close(); } catch { /* the host is gone */ }
        this.exposed = idleSnapshot('opening', this.epochBase);
        this.exposedFrom = null;
        this.notify();
        void this.start();
    }

    private notify(): void {
        for (const l of [...this.listeners]) l();
    }

    // ---- ViewHandle --------------------------------------------------------------------------

    update(patch: ViewPatch): void {
        if (this.disposed || !this.spec_) return;
        this.spec_ = mergePatch(this.spec_, patch);
        this.inner?.update(patch);
    }

    close(): void {
        if (this.disposed) return;
        this.disposed = true;
        this.gen++;
        const inner = this.inner;
        this.detach(false);
        inner?.close();
        this.exposed = idleSnapshot('closed', this.epochBase + 1);
        this.exposedFrom = null;
        this.factory.unregister(this);
        for (const l of [...this.listeners]) l();
        this.listeners.clear();
        this.staleListeners.clear();
        this.transitionListeners.clear();
    }

    getSnapshot(): ViewSnapshot {
        const inner = this.inner;
        if (!inner) return this.exposed;
        const snap = inner.getSnapshot();
        if (snap !== this.exposedFrom || this.exposedBase !== this.epochBase) {
            this.exposed = { ...snap, epoch: snap.epoch + this.epochBase };
            this.exposedFrom = snap;
            this.exposedBase = this.epochBase;
        }
        return this.exposed;
    }

    subscribe(cb: () => void): () => void {
        this.listeners.add(cb);
        return () => { this.listeners.delete(cb); };
    }

    setRange(start: number, end: number): void {
        if (this.disposed) return;
        this.range = { start, end };
        this.inner?.setRange(start, end);
    }

    rowAt(index: number): RowItem | undefined {
        return this.inner?.rowAt(index);
    }

    loadedRows(): RowItem[] {
        return this.inner?.loadedRows() ?? [];
    }

    private need(): ViewHandle {
        if (!this.inner) throw new Error('view is not open');
        return this.inner;
    }

    refs(from: number, to: number) {
        return Promise.resolve().then(() => this.need().refs(from, to));
    }

    selectAllRefs() {
        return Promise.resolve().then(() => this.need().selectAllRefs());
    }

    facet(column: string) {
        return Promise.resolve().then(() => this.need().facet(column));
    }

    setSelection(uids: Iterable<string>): void {
        this.selected = new Set(uids);
        this.inner?.setSelection(this.selected);
    }

    onSelectionStale(cb: (uids: string[]) => void): () => void {
        this.staleListeners.add(cb);
        return () => { this.staleListeners.delete(cb); };
    }

    onTransitions(cb: (events: PodTransition[]) => void): () => void {
        this.transitionListeners.add(cb);
        return () => { this.transitionListeners.delete(cb); };
    }

    getObject(item: ItemRef) {
        return Promise.resolve().then(() => this.need().getObject(item));
    }

    getObjects(items: ItemRef[]) {
        return Promise.resolve().then(() => this.need().getObjects(items));
    }

    refresh(): void {
        this.inner?.refresh();
    }

    reopen(): void {
        this.inner?.reopen();
    }

    setActive(active: boolean): void {
        this.active = active;
        this.inner?.setActive(active);
    }

    spec(): ViewSpec | null {
        return this.spec_ ? { ...this.spec_ } : null;
    }
}

// ---- the process-wide factory ----------------------------------------------------------------

async function defaultGetCaps(): Promise<BulkCaps> {
    await getBulkCaps();
    const c = peekBulkCaps();
    if (c.enabled && c.probedAt === 0) {
        // The probe decides what this WebView can do; wait for it briefly rather than guess.
        try {
            const { runBulkProbe } = await import('../bulk/probe');
            await Promise.race([runBulkProbe(), new Promise(r => setTimeout(r, PROBE_WAIT_MS))]);
        } catch {
            // keep whatever is known
        }
    }
    return peekBulkCaps();
}

function defaultInitInfo(): HostInitInfo | null {
    const info = peekBulkInfo();
    if (!info || !info.enabled || !info.token || typeof location === 'undefined') return null;
    return {
        origin: `${location.protocol}//${location.host}`,
        base: info.base,
        token: info.token,
        streaming: peekBulkCaps().streaming,
    };
}

let shared: ViewFactory | null = null;

export function getViewFactory(): ViewFactory {
    if (shared) return shared;
    shared = new ViewFactory({
        getCaps: defaultGetCaps,
        getSetting: readViewWorkerSetting,
        workerSupported: () => typeof Worker !== 'undefined' && typeof Blob !== 'undefined' && typeof URL !== 'undefined',
        createWorker: async () => (await import('./viewWorkerLoader')).createViewWorker(),
        services: () => ({ transport: createDefaultTransport(), fetchBytes: bulkFetchBytes }),
        notices: getNoticeFeed,
        initInfo: defaultInitInfo,
        makeInline: () => createViewClient({ transport: createDefaultTransport(), notices: getNoticeFeed() }),
    });
    return shared;
}

/** Test hook: drop the process-wide factory. */
export function resetViewFactoryForTests(): void {
    shared?.dispose();
    shared = null;
}

/** A view handle that runs at the best level this WebView supports. */
export function createViewHandle(): ViewHandle {
    return getViewFactory().createHandle();
}

/**
 * Runs a bulk GET inside the view worker, so JSON parsing of a large body stays off the main
 * thread. Resolves null when the session is not at level W (callers then fetch themselves).
 */
export function fetchBulkInWorker(path: string): Promise<FetchedCall | null> {
    return getViewFactory().fetchCallInWorker(path);
}

setWorkerCallFetcher(path => fetchBulkInWorker(path));
