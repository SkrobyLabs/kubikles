import { isBulkMethod } from './bulkMethods';
import type { BulkCaps, BulkInfo, BulkStats } from './types';

/** Thrown when the bulk route cannot serve a request; callers fall back to the binding. */
export class BulkUnavailableError extends Error {
    /** HTTP status when the failure was a response, so callers can tell 410 from a dead route. */
    status?: number;
    constructor(message: string, status?: number) {
        super(message);
        this.name = 'BulkUnavailableError';
        this.status = status;
    }
}

/** The method ran and reported an error; this is the real result, never retried. */
export class BulkApplicationError extends Error {
    constructor(message: string) {
        super(message);
        this.name = 'BulkApplicationError';
    }
}

const MAX_ARGS_JSON = 60 * 1024; // Go rejects > 64KB with 414
const CALL_TIMEOUT_MS = 5 * 60 * 1000;
const DEGRADE_AFTER = 3;
// Mirrors CONFIG_STORAGE_KEY in ConfigContext; the setting must be honoured
// before React effects have pushed it to the backend.
const SETTINGS_KEY = 'kubikles_settings';

function emptyCaps(): BulkCaps {
    return {
        enabled: false,
        fetch: false,
        streaming: false,
        buffered: false,
        workerBlob: false,
        workerFetch: false,
        workerScriptUrl: false,
        degraded: false,
        notes: [],
        probedAt: 0,
        userAgent: typeof navigator !== 'undefined' ? navigator.userAgent : '',
        benchMs: { bulkJson: null, bulkNdjson: null, ipc: null },
    };
}

let infoLoader: (() => Promise<BulkInfo>) | null = null;
let info: BulkInfo | null = null;
let capsPromise: Promise<BulkCaps> | null = null;
let caps: BulkCaps = emptyCaps();
let stats: BulkStats = { calls: 0, fallbacks: 0, failures: 0, consecutiveFailures: 0, lastError: '' };
const listeners = new Set<() => void>();
let version = 0;

function notify(): void {
    version++;
    for (const l of [...listeners]) l();
}

/** The adapter registers how to reach GetBulkInfo (avoids an import cycle). */
export function setBulkInfoLoader(fn: () => Promise<BulkInfo>): void {
    infoLoader = fn;
}

export function subscribeBulk(cb: () => void): () => void {
    listeners.add(cb);
    return () => listeners.delete(cb);
}

export function getBulkVersion(): number {
    return version;
}

export function readStoredBulkPreference(): 'auto' | 'off' {
    try {
        const raw = localStorage.getItem(SETTINGS_KEY);
        if (!raw) return 'auto';
        const v = JSON.parse(raw)?.performance?.bulkTransport;
        return v === 'off' ? 'off' : 'auto';
    } catch {
        return 'auto';
    }
}

async function loadInfo(): Promise<BulkInfo | null> {
    if (info) return info;
    if (!infoLoader) return null;
    try {
        const i = await infoLoader();
        if (i && typeof i === 'object') info = i;
    } catch {
        info = null;
    }
    return info;
}

/**
 * Capabilities as currently known. Memoized; before the probe has run,
 * server mode (a normal browser, where fetch is certain) is optimistic and
 * desktop is not.
 */
export function getBulkCaps(): Promise<BulkCaps> {
    if (!capsPromise) {
        capsPromise = (async () => {
            const i = await loadInfo();
            const c = emptyCaps();
            if (readStoredBulkPreference() === 'off' || !i || !i.enabled || !i.token) {
                c.notes.push(i && !i.enabled ? 'disabled by backend' : 'bulk info unavailable');
                caps = c;
                return caps;
            }
            c.enabled = true;
            c.server = i.server;
            c.fetch = i.server;
            c.streaming = i.server;
            caps = c;
            notify();
            return caps;
        })();
    }
    return capsPromise;
}

/** Synchronous snapshot for UI. */
export function peekBulkCaps(): BulkCaps {
    return caps;
}

export function peekBulkInfo(): BulkInfo | null {
    return info;
}

export function getBulkStats(): BulkStats {
    return stats;
}

/** Probe results replace the current capability set. */
export function applyProbedCaps(next: BulkCaps): void {
    caps = next;
    capsPromise = Promise.resolve(next);
    stats = { ...stats, consecutiveFailures: 0 };
    notify();
}

/**
 * Re-read the backend descriptor after the bulkTransport setting was pushed.
 * Returns true when the enabled state flipped (capabilities were dropped and a
 * new probe is needed); otherwise probe results are kept.
 */
export async function refreshBulkInfo(): Promise<boolean> {
    const hadCaps = capsPromise !== null;
    const wasEnabled = caps.enabled;
    info = null;
    const next = await loadInfo();
    if (!hadCaps) return false;
    const nowEnabled = !!next?.enabled && !!next.token && readStoredBulkPreference() !== 'off';
    if (wasEnabled === nowEnabled) return false;
    resetBulkCaps();
    return true;
}

/** Forget everything (after a setting change or for a re-probe). */
export function resetBulkCaps(): void {
    info = null;
    capsPromise = null;
    caps = emptyCaps();
    stats = { calls: 0, fallbacks: 0, failures: 0, consecutiveFailures: 0, lastError: '' };
    notify();
}

function headers(i: BulkInfo): Record<string, string> {
    return { 'X-Kubikles': '1', 'X-Kubikles-Token': i.token };
}

export function bulkUrl(i: BulkInfo, path: string): string {
    return `${i.base}/${path.replace(/^\//, '')}`;
}

async function bulkFetch(path: string, signal?: AbortSignal): Promise<Response> {
    const i = await loadInfo();
    if (!i || !i.enabled || !i.token) throw new BulkUnavailableError('bulk transport not enabled');
    if (typeof fetch !== 'function') throw new BulkUnavailableError('fetch unavailable');
    let resp: Response;
    try {
        resp = await fetch(bulkUrl(i, path), { method: 'GET', headers: headers(i), cache: 'no-store', signal });
    } catch (e) {
        if ((e as Error)?.name === 'AbortError') throw e;
        throw new BulkUnavailableError(`fetch failed: ${(e as Error)?.message ?? e}`);
    }
    return resp;
}

async function readErrorBody(resp: Response): Promise<string> {
    try {
        const j = JSON.parse(await resp.text());
        if (j && typeof j.error === 'string') return j.error;
    } catch {
        // not JSON
    }
    return `HTTP ${resp.status}`;
}

/** GET a JSON document. Non-2xx responses surface as BulkUnavailableError. */
export async function bulkFetchJson<T = unknown>(path: string, signal?: AbortSignal): Promise<T> {
    const resp = await bulkFetch(path, signal);
    if (!resp.ok) throw new BulkUnavailableError(await readErrorBody(resp), resp.status);
    try {
        return (await resp.json()) as T;
    } catch (e) {
        throw new BulkUnavailableError(`bad JSON: ${(e as Error)?.message ?? e}`);
    }
}

/** GET a body as raw bytes (for the view worker's level M, where the main thread fetches). */
export async function bulkFetchBytes(path: string): Promise<{ status: number; contentType: string; bytes: ArrayBuffer }> {
    const resp = await bulkFetch(path);
    let bytes: ArrayBuffer;
    try {
        bytes = await resp.arrayBuffer();
    } catch (e) {
        throw new BulkUnavailableError(`body read failed: ${(e as Error)?.message ?? e}`);
    }
    return { status: resp.status, contentType: resp.headers.get('Content-Type') || '', bytes };
}

/**
 * Splits a byte stream into lines. Holds partial lines and partial UTF-8
 * sequences across chunk boundaries (TextDecoder in stream mode).
 */
export class LineSplitter {
    private decoder = new TextDecoder('utf-8');
    private rest = '';

    push(chunk: Uint8Array): string[] {
        this.rest += this.decoder.decode(chunk, { stream: true });
        return this.drain();
    }

    /** Flush at end of input; returns a final unterminated line, if any. */
    end(): string[] {
        this.rest += this.decoder.decode();
        const out = this.drain();
        if (this.rest.length > 0) {
            out.push(this.rest);
            this.rest = '';
        }
        return out;
    }

    private drain(): string[] {
        const out: string[] = [];
        let idx = this.rest.indexOf('\n');
        while (idx >= 0) {
            let line = this.rest.slice(0, idx);
            if (line.endsWith('\r')) line = line.slice(0, -1);
            if (line.length > 0) out.push(line);
            this.rest = this.rest.slice(idx + 1);
            idx = this.rest.indexOf('\n');
        }
        return out;
    }
}

async function* ndjsonLines(resp: Response): AsyncGenerator<string> {
    const body = resp.body;
    if (!body || typeof body.getReader !== 'function') {
        // No streaming body: parse the whole text.
        const splitter = new LineSplitter();
        const text = await resp.text();
        for (const l of splitter.push(new TextEncoder().encode(text))) yield l;
        for (const l of splitter.end()) yield l;
        return;
    }
    const reader = body.getReader();
    const splitter = new LineSplitter();
    try {
        for (;;) {
            const { done, value } = await reader.read();
            if (done) break;
            if (value) for (const l of splitter.push(value)) yield l;
        }
        for (const l of splitter.end()) yield l;
    } finally {
        try {
            reader.releaseLock();
        } catch {
            // already released
        }
    }
}

/** GET an NDJSON stream as an async iterator of parsed lines. */
export async function* bulkFetchNdjson<T = unknown>(path: string, signal?: AbortSignal): AsyncGenerator<T> {
    const resp = await bulkFetch(path, signal);
    if (!resp.ok) throw new BulkUnavailableError(await readErrorBody(resp), resp.status);
    try {
        for await (const line of ndjsonLines(resp)) {
            let v: unknown;
            try {
                v = JSON.parse(line);
            } catch (e) {
                throw new BulkUnavailableError(`bad NDJSON line: ${(e as Error)?.message ?? e}`);
            }
            yield v as T;
        }
    } catch (e) {
        if (e instanceof BulkUnavailableError || (e as Error)?.name === 'AbortError') throw e;
        throw new BulkUnavailableError(`stream failed: ${(e as Error)?.message ?? e}`);
    }
}

export type BulkCallOutcome = { used: true; value: unknown } | { used: false };

/** Methods whose results can be large: JSON parsing moves to the view worker when it is available. */
const WORKER_CALL_RE = /(Yaml|YAML|History|HistoryRange|LogsBefore|LogsAfter)$/;

export interface CallFetch {
    status: number;
    ok: boolean;
    items?: unknown[];
    doc?: { data?: unknown; error?: string } | null;
}

type WorkerCallFetcher = (path: string) => Promise<CallFetch | null>;
let workerCallFetcher: WorkerCallFetcher | null = null;

/**
 * The view host registers how to run a call fetch inside the worker (level W only). The fetcher
 * answers null when no worker is usable, and callers then fetch on the main thread.
 */
export function setWorkerCallFetcher(fn: WorkerCallFetcher | null): void {
    workerCallFetcher = fn;
}

function recordFailure(msg: string): void {
    stats.fallbacks++;
    stats.failures++;
    stats.consecutiveFailures++;
    stats.lastError = msg;
    if (stats.consecutiveFailures >= DEGRADE_AFTER && !caps.degraded) {
        caps = { ...caps, degraded: true, notes: [...caps.notes, `degraded after ${DEGRADE_AFTER} consecutive failures: ${msg}`] };
    }
    notify();
}

function recordSuccess(): void {
    stats.calls++;
    if (stats.consecutiveFailures !== 0) stats.consecutiveFailures = 0;
}

/** Lets other bulk routes (views) feed the same failure counter that degrades the transport. */
export function noteBulkResult(ok: boolean, message = ''): void {
    if (ok) recordSuccess();
    else recordFailure(message);
}

/** True when the fetch transport can be tried right now for a route that is not a call/ method. */
export async function bulkFetchUsable(): Promise<boolean> {
    const c = await getBulkCaps();
    return c.enabled && c.fetch && !c.degraded;
}

/** True when the bulk path should be tried for this call right now. */
export function bulkRoutable(method: string, c: BulkCaps = caps): boolean {
    return isBulkMethod(method) && c.enabled && c.fetch && !c.degraded;
}

async function fetchCallOnMain(path: string, signal: AbortSignal): Promise<CallFetch> {
    const resp = await bulkFetch(path, signal);
    const ctype = resp.headers.get('Content-Type') || '';
    if (resp.ok && ctype.startsWith('application/x-ndjson')) {
        const items: unknown[] = [];
        for await (const line of ndjsonLines(resp)) items.push(JSON.parse(line));
        return { status: resp.status, ok: true, items };
    }
    let doc: CallFetch['doc'] = null;
    try {
        doc = await resp.json();
    } catch {
        doc = null;
    }
    return { status: resp.status, ok: resp.ok, doc };
}

async function fetchCall(method: string, path: string, signal: AbortSignal, timeoutMs: number): Promise<CallFetch> {
    if (workerCallFetcher && WORKER_CALL_RE.test(method)) {
        try {
            let timer: ReturnType<typeof setTimeout> | undefined;
            const viaWorker = await Promise.race([
                workerCallFetcher(path),
                new Promise<null>((resolve) => {
                    timer = setTimeout(() => resolve(null), timeoutMs);
                }),
            ]).finally(() => clearTimeout(timer));
            if (viaWorker) return viaWorker;
        } catch {
            // the worker could not serve it: use the main thread
        }
    }
    return fetchCallOnMain(path, signal);
}

/**
 * Try a read method over the bulk route. Returns {used:false} whenever the
 * caller should use the binding instead (not routable, transport failure).
 * A method-level error is thrown as BulkApplicationError: the method ran, so
 * it is not retried.
 */
export async function tryBulkCall(method: string, args: unknown[]): Promise<BulkCallOutcome> {
    if (!isBulkMethod(method)) return { used: false };
    await getBulkCaps();
    if (!bulkRoutable(method)) return { used: false };

    let argsJson: string;
    try {
        argsJson = JSON.stringify(args);
    } catch {
        return { used: false };
    }
    if (argsJson.length > MAX_ARGS_JSON) return { used: false };

    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), CALL_TIMEOUT_MS);
    try {
        const got = await fetchCall(method, `call/${method}?args=${encodeURIComponent(argsJson)}`, controller.signal, CALL_TIMEOUT_MS);
        if (got.ok && got.items) {
            const items: unknown[] = [];
            for (const v of got.items) {
                if (v && typeof v === 'object' && '$error' in (v as Record<string, unknown>)) {
                    throw new BulkUnavailableError(String((v as Record<string, unknown>).$error));
                }
                items.push(v);
            }
            recordSuccess();
            return { used: true, value: items };
        }
        const doc = got.doc ?? null;
        if (!doc) throw new BulkUnavailableError(`HTTP ${got.status} with non-JSON body`);
        if (got.ok && 'data' in doc) {
            recordSuccess();
            return { used: true, value: doc.data ?? null };
        }
        if (got.status === 500 && typeof doc.error === 'string') {
            recordSuccess();
            throw new BulkApplicationError(doc.error);
        }
        throw new BulkUnavailableError(`HTTP ${got.status}${doc?.error ? `: ${doc.error}` : ''}`);
    } catch (e) {
        if (e instanceof BulkApplicationError) {
            // Mirror each path's own error shape: Wails bindings reject with
            // the bare message string, /api/call throws an Error.
            if (info && !info.server) throw e.message;
            throw new Error(e.message);
        }
        const msg = (e as Error)?.message ?? String(e);
        recordFailure(msg);
        return { used: false };
    } finally {
        clearTimeout(timer);
    }
}
