import { BulkBenchIPC, ReportBulkProbe } from 'wailsjs/go/main/App';
import probeWorkerSource from './probeWorker.js?raw';
import {
    applyProbedCaps,
    bulkFetchJson,
    bulkFetchNdjson,
    getBulkCaps,
    peekBulkInfo,
    readStoredBulkPreference,
} from './transport';
import type { BulkBenchMs, BulkCaps } from './types';

const BENCH_OBJECTS = 2000;
const STREAM_CHUNKS = 6;
const STREAM_DELAY_MS = 100;
const WORKER_TIMEOUT_MS = 2000;
const SUMMARY_KEY = 'kubikles_bulk_probe';
const APP_VERSION_TAG = 'v1';

// ---------------------------------------------------------------------------
// Decision table: raw outcomes -> caps. Pure so every combination is testable.
// ---------------------------------------------------------------------------

export interface ProbeOutcomes {
    enabled: boolean;
    server?: boolean;
    ping: { ok: boolean; error?: string };
    /** Arrival times (ms) of each streamed line; empty when the stream failed. */
    streamArrivals: number[];
    streamDelayMs: number;
    workerBlob: { ok: boolean; error?: string };
    workerFetch: { ok: boolean; error?: string };
    workerScriptUrl: { ok: boolean; error?: string };
    benchMs: BulkBenchMs;
    longTaskMs?: BulkBenchMs;
    bytesMs?: Record<string, number>;
    userAgent: string;
    probedAt: number;
}

/** Streaming iff the first line arrived well before the last. */
export function isStreaming(arrivals: number[], delayMs: number): boolean {
    if (arrivals.length < 2) return false;
    const spread = arrivals[arrivals.length - 1] - arrivals[0];
    const expected = (arrivals.length - 1) * delayMs;
    return spread >= expected * 0.5;
}

export function decideCaps(o: ProbeOutcomes): BulkCaps {
    const notes: string[] = [];
    const caps: BulkCaps = {
        enabled: o.enabled,
        fetch: false,
        streaming: false,
        buffered: false,
        workerBlob: false,
        workerFetch: false,
        workerScriptUrl: false,
        degraded: false,
        notes,
        probedAt: o.probedAt,
        userAgent: o.userAgent,
        benchMs: o.benchMs,
        server: o.server,
        longTaskMs: o.longTaskMs,
        bytesMs: o.bytesMs,
    };
    if (!o.enabled) {
        notes.push('bulk transport disabled by backend or setting');
        return caps;
    }
    if (!o.ping.ok) {
        notes.push(`fetch to bulk route failed: ${o.ping.error ?? 'unknown'}; bulk stays off, bindings are used`);
        return caps;
    }
    caps.fetch = true;
    if (o.streamArrivals.length === 0) {
        notes.push('stream probe failed; treating bodies as buffered');
        caps.buffered = true;
    } else if (isStreaming(o.streamArrivals, o.streamDelayMs)) {
        caps.streaming = true;
    } else {
        caps.buffered = true;
        notes.push('bodies are delivered whole (no incremental streaming)');
    }
    caps.workerBlob = o.workerBlob.ok;
    if (!o.workerBlob.ok) notes.push(`blob worker: ${o.workerBlob.error ?? 'failed'}`);
    caps.workerFetch = o.workerBlob.ok && o.workerFetch.ok;
    if (o.workerBlob.ok && !o.workerFetch.ok) notes.push(`worker fetch: ${o.workerFetch.error ?? 'failed'}`);
    caps.workerScriptUrl = o.workerScriptUrl.ok;
    if (!o.workerScriptUrl.ok) notes.push(`script-url worker: ${o.workerScriptUrl.error ?? 'failed'}`);
    return caps;
}

/**
 * Host level a later package can use, given the caps:
 * W worker fetches itself, M main thread fetches and transfers buffers to a
 * worker, I engine inline on the main thread, L legacy bindings only.
 */
export function hostLevel(c: BulkCaps): 'W' | 'M' | 'I' | 'L' {
    if (!c.enabled || !c.fetch) return 'L';
    if (c.workerBlob && c.workerFetch) return 'W';
    if (c.workerBlob) return 'M';
    return 'I';
}

// ---------------------------------------------------------------------------
// Measurements
// ---------------------------------------------------------------------------

interface LongTaskMeter {
    stop(): number;
}

function startLongTaskMeter(): LongTaskMeter {
    let total = 0;
    let obs: PerformanceObserver | null = null;
    try {
        if (typeof PerformanceObserver !== 'undefined') {
            obs = new PerformanceObserver((list) => {
                for (const e of list.getEntries()) total += e.duration;
            });
            obs.observe({ entryTypes: ['longtask'] });
        }
    } catch {
        obs = null;
    }
    return {
        stop() {
            try {
                obs?.takeRecords().forEach((e) => {
                    total += e.duration;
                });
                obs?.disconnect();
            } catch {
                // ignore
            }
            return Math.round(total);
        },
    };
}

async function timed<T>(fn: () => Promise<T>): Promise<{ ms: number | null; longTask: number; error?: string }> {
    const meter = startLongTaskMeter();
    const t0 = performance.now();
    try {
        await fn();
        return { ms: Math.round(performance.now() - t0), longTask: meter.stop() };
    } catch (e) {
        return { ms: null, longTask: meter.stop(), error: (e as Error)?.message ?? String(e) };
    }
}

async function probeStream(): Promise<number[]> {
    const arrivals: number[] = [];
    const t0 = performance.now();
    try {
        for await (const _ of bulkFetchNdjson(`probe/stream?chunks=${STREAM_CHUNKS}&delayMs=${STREAM_DELAY_MS}&size=64`)) {
            arrivals.push(performance.now() - t0);
        }
    } catch {
        return [];
    }
    return arrivals;
}

interface WorkerResult {
    ok: boolean;
    error?: string;
}

function runWorker(
    make: () => Worker,
    messages: Array<{ send: unknown; expect: string }>,
    cleanup?: () => void,
): Promise<WorkerResult[]> {
    return new Promise((resolve) => {
        const results: WorkerResult[] = [];
        let worker: Worker | null = null;
        let idx = 0;
        let timer: ReturnType<typeof setTimeout> | null = null;
        const finish = (error?: string) => {
            if (timer) clearTimeout(timer);
            try {
                worker?.terminate();
            } catch {
                // ignore
            }
            cleanup?.();
            while (results.length < messages.length) results.push({ ok: false, error: error ?? 'not reached' });
            resolve(results);
        };
        const arm = () => {
            if (timer) clearTimeout(timer);
            timer = setTimeout(() => finish(`timed out after ${WORKER_TIMEOUT_MS}ms`), WORKER_TIMEOUT_MS);
        };
        try {
            worker = make();
        } catch (e) {
            finish((e as Error)?.message ?? 'constructor threw');
            return;
        }
        worker.onerror = (ev: ErrorEvent) => finish(ev?.message || 'worker error');
        worker.onmessage = (ev: MessageEvent) => {
            const m = ev.data || {};
            if (m.type !== messages[idx]?.expect) return;
            results.push({ ok: !!m.ok, error: m.ok ? undefined : m.error || (m.status ? `HTTP ${m.status}` : 'failed') });
            idx++;
            if (idx >= messages.length) {
                finish();
                return;
            }
            arm();
            worker?.postMessage(messages[idx].send);
        };
        arm();
        worker.postMessage(messages[0].send);
    });
}

async function probeWorkers(info: { base: string; token: string }): Promise<{
    blob: WorkerResult;
    fetch: WorkerResult;
    script: WorkerResult;
}> {
    const none = { ok: false, error: 'Worker unavailable' };
    if (typeof Worker === 'undefined') return { blob: none, fetch: none, script: none };

    // location.origin is "null" for some custom schemes; build it by hand.
    const origin = `${location.protocol}//${location.host}`;
    const url = `${origin}${info.base}/probe/ping`;
    const hdrs = { 'X-Kubikles': '1', 'X-Kubikles-Token': info.token };

    let blobUrl = '';
    const blobRes = await runWorker(
        () => {
            blobUrl = URL.createObjectURL(new Blob([probeWorkerSource], { type: 'text/javascript' }));
            return new Worker(blobUrl);
        },
        [
            { send: { type: 'hello' }, expect: 'hello' },
            { send: { type: 'fetch', url, headers: hdrs }, expect: 'fetch' },
        ],
        () => {
            if (blobUrl) URL.revokeObjectURL(blobUrl);
        },
    );

    let scriptRes: WorkerResult[];
    try {
        scriptRes = await runWorker(
            () => new Worker(new URL('./probeWorker.js', import.meta.url)),
            [{ send: { type: 'hello' }, expect: 'hello' }],
        );
    } catch (e) {
        scriptRes = [{ ok: false, error: (e as Error)?.message ?? 'failed' }];
    }
    return { blob: blobRes[0], fetch: blobRes[1], script: scriptRes[0] };
}

// ---------------------------------------------------------------------------
// Orchestration
// ---------------------------------------------------------------------------

function summaryKey(): string {
    return `${APP_VERSION_TAG}|${typeof navigator !== 'undefined' ? navigator.userAgent : ''}`;
}

function loadSummary(): BulkCaps | null {
    try {
        const raw = localStorage.getItem(SUMMARY_KEY);
        if (!raw) return null;
        const parsed = JSON.parse(raw);
        if (parsed?.key !== summaryKey() || !parsed.caps) return null;
        return parsed.caps as BulkCaps;
    } catch {
        return null;
    }
}

function saveSummary(c: BulkCaps): void {
    try {
        localStorage.setItem(SUMMARY_KEY, JSON.stringify({ key: summaryKey(), caps: c }));
    } catch {
        // storage unavailable
    }
}

let running: Promise<BulkCaps> | null = null;

export interface RunProbeOptions {
    /** Ignore the cached summary and measure again. */
    force?: boolean;
}

async function pingOk(): Promise<boolean> {
    try {
        const r = await bulkFetchJson<{ ok?: boolean }>('probe/ping');
        return r?.ok === true;
    } catch {
        return false;
    }
}

async function measure(): Promise<BulkCaps> {
    await getBulkCaps();
    const info = peekBulkInfo();
    const base: Pick<ProbeOutcomes, 'userAgent' | 'probedAt' | 'streamDelayMs'> = {
        userAgent: typeof navigator !== 'undefined' ? navigator.userAgent : '',
        probedAt: Date.now(),
        streamDelayMs: STREAM_DELAY_MS,
    };
    const emptyBench: BulkBenchMs = { bulkJson: null, bulkNdjson: null, ipc: null };
    if (!info || !info.enabled || !info.token || readStoredBulkPreference() === 'off') {
        return decideCaps({
            ...base,
            enabled: false,
            ping: { ok: false },
            streamArrivals: [],
            workerBlob: { ok: false },
            workerFetch: { ok: false },
            workerScriptUrl: { ok: false },
            benchMs: emptyBench,
        });
    }

    let pingError = '';
    let ping = false;
    try {
        const r = await bulkFetchJson<{ ok?: boolean }>('probe/ping');
        ping = r?.ok === true;
        if (!ping) pingError = 'unexpected ping body';
    } catch (e) {
        pingError = (e as Error)?.message ?? String(e);
    }
    if (!ping) {
        return decideCaps({
            ...base,
            enabled: true,
            server: info.server,
            ping: { ok: false, error: pingError },
            streamArrivals: [],
            workerBlob: { ok: false },
            workerFetch: { ok: false },
            workerScriptUrl: { ok: false },
            benchMs: emptyBench,
        });
    }

    const streamArrivals = await probeStream();

    const benchMs: BulkBenchMs = { bulkJson: null, bulkNdjson: null, ipc: null };
    const longTaskMs: BulkBenchMs = { bulkJson: null, bulkNdjson: null, ipc: null };
    const n = BENCH_OBJECTS;
    const j = await timed(async () => {
        const doc = await bulkFetchJson<{ data: unknown[] }>(`probe/bench?objects=${n}&mode=json`);
        if (doc.data.length !== n) throw new Error('short bench body');
    });
    benchMs.bulkJson = j.ms;
    longTaskMs.bulkJson = j.longTask;
    const nd = await timed(async () => {
        let count = 0;
        for await (const _ of bulkFetchNdjson(`probe/bench?objects=${n}&mode=ndjson`)) count++;
        if (count !== n) throw new Error('short bench stream');
    });
    benchMs.bulkNdjson = nd.ms;
    longTaskMs.bulkNdjson = nd.longTask;
    const ipc = await timed(async () => {
        const r = await BulkBenchIPC(n);
        if (!Array.isArray(r) || r.length !== n) throw new Error('short IPC bench');
    });
    benchMs.ipc = ipc.ms;
    longTaskMs.ipc = ipc.longTask;

    const bytesMs: Record<string, number> = {};
    for (const mb of [1, 16]) {
        const t = await timed(async () => {
            const i = peekBulkInfo();
            if (!i) throw new Error('no info');
            const resp = await fetch(`${i.base}/probe/bytes?mb=${mb}`, {
                headers: { 'X-Kubikles': '1', 'X-Kubikles-Token': i.token },
                cache: 'no-store',
            });
            const buf = await resp.arrayBuffer();
            if (buf.byteLength !== mb << 20) throw new Error(`short body ${buf.byteLength}`);
        });
        if (t.ms !== null) bytesMs[String(mb)] = t.ms;
    }

    const w = await probeWorkers(info);
    return decideCaps({
        ...base,
        enabled: true,
        server: info.server,
        ping: { ok: true },
        streamArrivals,
        workerBlob: w.blob,
        workerFetch: w.fetch,
        workerScriptUrl: w.script,
        benchMs,
        longTaskMs,
        bytesMs,
    });
}

/**
 * Measure what this WebView can do and apply it. Concurrent calls share one
 * run. A cached summary (same app tag and user agent) is reused after a ping
 * confirms the route still works; force re-measures.
 */
export function runBulkProbe(opts: RunProbeOptions = {}): Promise<BulkCaps> {
    if (running) return running;
    running = (async () => {
        try {
            await getBulkCaps();
            if (!opts.force) {
                const cached = loadSummary();
                if (cached && cached.enabled && cached.fetch && peekBulkInfo()?.enabled && (await pingOk())) {
                    const c = { ...cached, cached: true, degraded: false };
                    applyProbedCaps(c);
                    void report(c);
                    return c;
                }
            }
            const caps = await measure();
            applyProbedCaps(caps);
            saveSummary(caps);
            void report(caps);
            return caps;
        } finally {
            running = null;
        }
    })();
    return running;
}

async function report(c: BulkCaps): Promise<void> {
    try {
        const payload: Record<string, unknown> = { ...c, hostLevel: hostLevel(c) };
        await ReportBulkProbe(payload);
    } catch {
        // diagnostics only
    }
}

/** Run the probe once after first paint, and expose window.__kubiklesProbe. */
export function scheduleBulkProbe(): void {
    if (typeof window === 'undefined') return;
    (window as unknown as { __kubiklesProbe?: () => Promise<BulkCaps> }).__kubiklesProbe = () => runBulkProbe({ force: true });
    const run = () => {
        void runBulkProbe().catch(() => {});
    };
    const ric = (window as unknown as { requestIdleCallback?: (cb: () => void, o?: { timeout: number }) => void }).requestIdleCallback;
    if (typeof ric === 'function') ric(run, { timeout: 5000 });
    else setTimeout(run, 1500);
}
