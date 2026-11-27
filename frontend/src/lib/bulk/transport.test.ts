import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
    applyProbedCaps,
    BulkUnavailableError,
    bulkFetchJson,
    bulkFetchNdjson,
    getBulkCaps,
    getBulkStats,
    LineSplitter,
    peekBulkCaps,
    refreshBulkInfo,
    resetBulkCaps,
    setBulkInfoLoader,
    setWorkerCallFetcher,
    bulkFetchBytes,
    tryBulkCall,
} from './transport';
import type { BulkCaps, BulkInfo } from './types';

const info = (over: Partial<BulkInfo> = {}): BulkInfo => ({
    enabled: true, base: '/_kubikles/v1', token: 'tok', streaming: true, server: true, version: 1, ...over,
});

const caps = (over: Partial<BulkCaps> = {}): BulkCaps => ({
    enabled: true, fetch: true, streaming: true, buffered: false, workerBlob: false, workerFetch: false,
    workerScriptUrl: false, degraded: false, notes: [], probedAt: 1, userAgent: 'test',
    benchMs: { bulkJson: null, bulkNdjson: null, ipc: null }, ...over,
});

const enc = new TextEncoder();

function streamOf(chunks: Uint8Array[]): ReadableStream<Uint8Array> {
    let i = 0;
    return new ReadableStream({
        pull(controller) {
            if (i < chunks.length) controller.enqueue(chunks[i++]);
            else controller.close();
        },
    });
}

function ndjsonResponse(chunks: Uint8Array[], status = 200): Response {
    return new Response(streamOf(chunks), { status, headers: { 'Content-Type': 'application/x-ndjson' } });
}

function jsonResponse(body: unknown, status = 200): Response {
    return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
}

let fetchMock: ReturnType<typeof vi.fn>;
let storage: Record<string, string>;

beforeEach(() => {
    fetchMock = vi.fn();
    (globalThis as any).fetch = fetchMock;
    storage = {};
    (globalThis as any).localStorage = {
        getItem: (k: string) => (k in storage ? storage[k] : null),
        setItem: (k: string, v: string) => { storage[k] = v; },
        removeItem: (k: string) => { delete storage[k]; },
    };
    resetBulkCaps();
    setBulkInfoLoader(async () => info());
});

afterEach(() => {
    delete (globalThis as any).fetch;
    delete (globalThis as any).localStorage;
});

describe('LineSplitter', () => {
    it('splits lines across chunk boundaries', () => {
        const s = new LineSplitter();
        expect(s.push(enc.encode('{"a":1}\n{"b'))).toEqual(['{"a":1}']);
        expect(s.push(enc.encode('":2}\n'))).toEqual(['{"b":2}']);
        expect(s.end()).toEqual([]);
    });

    it('keeps multi-byte UTF-8 sequences split across chunks intact', () => {
        const bytes = enc.encode('{"s":"héllo € 😀"}\n');
        const s = new LineSplitter();
        const out: string[] = [];
        for (let i = 0; i < bytes.length; i++) out.push(...s.push(bytes.subarray(i, i + 1)));
        expect(out).toEqual(['{"s":"héllo € 😀"}']);
    });

    it('returns an unterminated final line on end and ignores blank lines and CR', () => {
        const s = new LineSplitter();
        expect(s.push(enc.encode('a\r\n\nb'))).toEqual(['a']);
        expect(s.end()).toEqual(['b']);
    });
});

describe('bulkFetchNdjson / bulkFetchJson', () => {
    beforeEach(async () => { await getBulkCaps(); });

    it('sends the guard headers and no request body', async () => {
        fetchMock.mockResolvedValue(jsonResponse({ ok: true }));
        await bulkFetchJson('probe/ping');
        const [url, init] = fetchMock.mock.calls[0];
        expect(url).toBe('/_kubikles/v1/probe/ping');
        expect(init.method).toBe('GET');
        expect(init.headers).toEqual({ 'X-Kubikles': '1', 'X-Kubikles-Token': 'tok' });
        expect(init.body).toBeUndefined();
        expect(init.cache).toBe('no-store');
    });

    it('iterates NDJSON delivered in awkward chunks', async () => {
        const all = enc.encode('{"n":1}\n{"s":"é€"}\n{"n":3}\n');
        const chunks = [all.subarray(0, 5), all.subarray(5, 14), all.subarray(14, 15), all.subarray(15)];
        fetchMock.mockResolvedValue(ndjsonResponse(chunks));
        const got: unknown[] = [];
        for await (const v of bulkFetchNdjson('x')) got.push(v);
        expect(got).toEqual([{ n: 1 }, { s: 'é€' }, { n: 3 }]);
    });

    it('falls back to response.text() when the body stream is absent', async () => {
        const resp = { ok: true, status: 200, body: null, headers: new Headers(), text: async () => '{"a":1}\n{"a":2}\n' } as unknown as Response;
        fetchMock.mockResolvedValue(resp);
        const got: unknown[] = [];
        for await (const v of bulkFetchNdjson('x')) got.push(v);
        expect(got).toEqual([{ a: 1 }, { a: 2 }]);
    });

    it('maps transport failures and HTTP errors to BulkUnavailableError', async () => {
        fetchMock.mockRejectedValueOnce(new TypeError('Failed to fetch'));
        await expect(bulkFetchJson('x')).rejects.toBeInstanceOf(BulkUnavailableError);
        fetchMock.mockResolvedValueOnce(jsonResponse({ error: 'invalid token' }, 403));
        await expect(bulkFetchJson('x')).rejects.toThrow('invalid token');
        fetchMock.mockResolvedValueOnce(ndjsonResponse([enc.encode('not json\n')]));
        const it = bulkFetchNdjson('x');
        await expect(it.next()).rejects.toBeInstanceOf(BulkUnavailableError);
    });

    it('passes an AbortSignal to fetch and surfaces abort as AbortError', async () => {
        fetchMock.mockImplementation((_url: string, init: any) => new Promise((_res, rej) => {
            const fail = () => rej(Object.assign(new Error('aborted'), { name: 'AbortError' }));
            if (init.signal.aborted) fail();
            else init.signal.addEventListener('abort', fail);
        }));
        const ac = new AbortController();
        const p = bulkFetchJson('x', ac.signal);
        ac.abort();
        await expect(p).rejects.toMatchObject({ name: 'AbortError' });
    });
});

describe('tryBulkCall', () => {
    it('does not use bulk on desktop until the probe passed', async () => {
        setBulkInfoLoader(async () => info({ server: false }));
        expect(await tryBulkCall('ListPods', ['r', ''])).toEqual({ used: false });
        expect(fetchMock).not.toHaveBeenCalled();
        applyProbedCaps(caps());
        fetchMock.mockResolvedValue(ndjsonResponse([enc.encode('{"a":1}\n')]));
        expect(await tryBulkCall('ListPods', ['r', ''])).toEqual({ used: true, value: [{ a: 1 }] });
    });

    it('is optimistic in server mode and builds the GET call URL', async () => {
        fetchMock.mockResolvedValue(ndjsonResponse([enc.encode('{"a":1}\n{"a":2}\n')]));
        const r = await tryBulkCall('ListPods', ['req 1', 'ns/x']);
        expect(r).toEqual({ used: true, value: [{ a: 1 }, { a: 2 }] });
        const url: string = fetchMock.mock.calls[0][0];
        expect(url.startsWith('/_kubikles/v1/call/ListPods?args=')).toBe(true);
        expect(JSON.parse(decodeURIComponent(url.split('args=')[1]))).toEqual(['req 1', 'ns/x']);
    });

    it('returns the data of non-slice results, including null', async () => {
        fetchMock.mockResolvedValueOnce(jsonResponse({ data: 'yaml: text' }));
        expect(await tryBulkCall('GetPodYaml', ['ns', 'p'])).toEqual({ used: true, value: 'yaml: text' });
        fetchMock.mockResolvedValueOnce(jsonResponse({ data: null }));
        expect(await tryBulkCall('ListPods', ['r', ''])).toEqual({ used: true, value: null });
    });

    it('leaves non-bulk methods alone', async () => {
        expect(await tryBulkCall('DeletePod', ['ns', 'p'])).toEqual({ used: false });
        expect(await tryBulkCall('SetForceHTTP1', [true])).toEqual({ used: false });
        expect(fetchMock).not.toHaveBeenCalled();
    });

    it('rethrows method errors without falling back', async () => {
        fetchMock.mockResolvedValue(jsonResponse({ error: 'forbidden: pods is forbidden' }, 500));
        await expect(tryBulkCall('ListPods', ['r', ''])).rejects.toThrow('forbidden: pods is forbidden');
        expect(getBulkStats().fallbacks).toBe(0);
        expect(getBulkStats().consecutiveFailures).toBe(0);
    });

    it('rejects with the bare string on desktop, like a Wails binding', async () => {
        setBulkInfoLoader(async () => info({ server: false }));
        resetBulkCaps();
        await getBulkCaps();
        applyProbedCaps(caps());
        fetchMock.mockResolvedValue(jsonResponse({ error: 'boom' }, 500));
        await expect(tryBulkCall('ListPods', ['r', ''])).rejects.toBe('boom');
    });

    it('falls back on transport failure and degrades after 3 consecutive failures', async () => {
        fetchMock.mockRejectedValue(new TypeError('Failed to fetch'));
        for (let i = 0; i < 3; i++) expect(await tryBulkCall('ListPods', ['r', ''])).toEqual({ used: false });
        expect(getBulkStats().fallbacks).toBe(3);
        expect(peekBulkCaps().degraded).toBe(true);
        fetchMock.mockClear();
        expect(await tryBulkCall('ListPods', ['r', ''])).toEqual({ used: false });
        expect(fetchMock).not.toHaveBeenCalled();
    });

    it('a success resets the consecutive failure count', async () => {
        fetchMock.mockRejectedValueOnce(new TypeError('x'));
        fetchMock.mockRejectedValueOnce(new TypeError('x'));
        fetchMock.mockResolvedValueOnce(jsonResponse({ data: 1 }));
        fetchMock.mockRejectedValueOnce(new TypeError('x'));
        await tryBulkCall('GetPodYaml', ['a', 'b']);
        await tryBulkCall('GetPodYaml', ['a', 'b']);
        await tryBulkCall('GetPodYaml', ['a', 'b']);
        await tryBulkCall('GetPodYaml', ['a', 'b']);
        expect(peekBulkCaps().degraded).toBe(false);
        expect(getBulkStats().consecutiveFailures).toBe(1);
    });

    it('treats 403/414 and non-JSON bodies as unavailable', async () => {
        fetchMock.mockResolvedValueOnce(jsonResponse({ error: 'invalid token' }, 403));
        expect(await tryBulkCall('ListPods', ['r', ''])).toEqual({ used: false });
        fetchMock.mockResolvedValueOnce(new Response('<html>', { status: 200, headers: { 'Content-Type': 'text/html' } }));
        expect(await tryBulkCall('ListPods', ['r', ''])).toEqual({ used: false });
    });

    it('does not send oversized args', async () => {
        const r = await tryBulkCall('ListPods', ['x'.repeat(70_000)]);
        expect(r).toEqual({ used: false });
        expect(fetchMock).not.toHaveBeenCalled();
    });

    it('is off when the setting is off, even if the backend allows it', async () => {
        storage.kubikles_settings = JSON.stringify({ performance: { bulkTransport: 'off' } });
        expect(await tryBulkCall('ListPods', ['r', ''])).toEqual({ used: false });
        expect(fetchMock).not.toHaveBeenCalled();
    });

    it('is off when the backend withholds the token', async () => {
        setBulkInfoLoader(async () => info({ enabled: false, token: '' }));
        expect(await tryBulkCall('ListPods', ['r', ''])).toEqual({ used: false });
        expect(fetchMock).not.toHaveBeenCalled();
    });

    it('an NDJSON error line falls back instead of returning a truncated list', async () => {
        fetchMock.mockResolvedValue(ndjsonResponse([enc.encode('{"a":1}\n{"$error":"encode failed"}\n')]));
        expect(await tryBulkCall('ListPods', ['r', ''])).toEqual({ used: false });
    });
});

describe('refreshBulkInfo', () => {
    it('keeps probe results when the enabled state did not change and drops them when it did', async () => {
        setBulkInfoLoader(async () => info({ server: false }));
        await getBulkCaps();
        applyProbedCaps(caps({ workerBlob: true }));
        expect(await refreshBulkInfo()).toBe(false);
        expect(peekBulkCaps().workerBlob).toBe(true);
        setBulkInfoLoader(async () => info({ enabled: false, token: '', server: false }));
        expect(await refreshBulkInfo()).toBe(true);
        expect(peekBulkCaps().enabled).toBe(false);
    });
});

describe('large bodies in the view worker', () => {
    afterEach(() => setWorkerCallFetcher(null));

    beforeEach(async () => {
        await getBulkCaps();
    });

    it('uses the worker fetcher for yaml and history methods and skips the main-thread fetch', async () => {
        const fetcher = vi.fn(async () => ({ status: 200, ok: true, doc: { data: 'yaml-from-worker' } }));
        setWorkerCallFetcher(fetcher);
        expect(await tryBulkCall('GetPodYaml', ['c', 'ns', 'p'])).toEqual({ used: true, value: 'yaml-from-worker' });
        expect(fetcher).toHaveBeenCalledTimes(1);
        expect(fetchMock).not.toHaveBeenCalled();
    });

    it('turns worker NDJSON items into the same list result', async () => {
        setWorkerCallFetcher(async () => ({ status: 200, ok: true, items: [{ l: 1 }, { l: 2 }] }));
        expect(await tryBulkCall('GetPodLogsBefore', ['x'])).toEqual({ used: true, value: [{ l: 1 }, { l: 2 }] });
    });

    it('keeps application errors and $error lines semantics for worker results', async () => {
        setWorkerCallFetcher(async () => ({ status: 500, ok: false, doc: { error: 'nope' } }));
        await expect(tryBulkCall('GetPodYaml', [])).rejects.toThrow('nope');
        setWorkerCallFetcher(async () => ({ status: 200, ok: true, items: [{ a: 1 }, { $error: 'bad' }] }));
        expect(await tryBulkCall('GetPodLogsBefore', [])).toEqual({ used: false });
    });

    it('does not use the worker for ordinary list methods', async () => {
        const fetcher = vi.fn(async () => null);
        setWorkerCallFetcher(fetcher);
        fetchMock.mockResolvedValue(jsonResponse({ data: [1] }));
        expect(await tryBulkCall('ListPods', [])).toEqual({ used: true, value: [1] });
        expect(fetcher).not.toHaveBeenCalled();
    });

    it('falls back to the main thread when the worker cannot serve the call', async () => {
        setWorkerCallFetcher(async () => null);
        fetchMock.mockResolvedValue(jsonResponse({ data: 'main' }));
        expect(await tryBulkCall('GetPodYaml', [])).toEqual({ used: true, value: 'main' });
        setWorkerCallFetcher(async () => { throw new Error('worker gone'); });
        fetchMock.mockResolvedValue(jsonResponse({ data: 'main2' }));
        expect(await tryBulkCall('GetPodYaml', [])).toEqual({ used: true, value: 'main2' });
    });
});

describe('bulkFetchBytes', () => {
    it('returns the status, content type and raw bytes', async () => {
        await getBulkCaps();
        fetchMock.mockResolvedValue(new Response(enc.encode('abc'), { status: 200, headers: { 'Content-Type': 'application/json' } }));
        const r = await bulkFetchBytes('views/v/window?offset=0');
        expect(r.status).toBe(200);
        expect(r.contentType).toBe('application/json');
        expect(new TextDecoder().decode(r.bytes)).toBe('abc');
    });
});
