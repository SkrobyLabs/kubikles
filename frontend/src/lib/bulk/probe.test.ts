import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const app = vi.hoisted(() => ({ bench: vi.fn(), report: vi.fn() }));
vi.mock('wailsjs/go/main/App', () => ({ BulkBenchIPC: app.bench, ReportBulkProbe: app.report }));

import { decideCaps, hostLevel, isStreaming, runBulkProbe, type ProbeOutcomes } from './probe';
import { peekBulkCaps, resetBulkCaps, setBulkInfoLoader } from './transport';

const ok = { ok: true };
const fail = (error: string) => ({ ok: false, error });
const base = (over: Partial<ProbeOutcomes> = {}): ProbeOutcomes => ({
    enabled: true,
    ping: ok,
    streamArrivals: [0, 100, 200, 300, 400, 500],
    streamDelayMs: 100,
    workerBlob: ok,
    workerFetch: ok,
    workerScriptUrl: ok,
    benchMs: { bulkJson: 10, bulkNdjson: 12, ipc: 30 },
    userAgent: 'ua',
    probedAt: 5,
    ...over,
});

describe('isStreaming', () => {
    it('is true when lines arrive spread out, false when they arrive together', () => {
        expect(isStreaming([0, 100, 200, 300, 400, 500], 100)).toBe(true);
        expect(isStreaming([498, 499, 500, 500, 501, 501], 100)).toBe(false);
        expect(isStreaming([5], 100)).toBe(false);
        expect(isStreaming([], 100)).toBe(false);
    });
});

describe('decideCaps decision table', () => {
    it('everything works: streaming, worker fetch, level W', () => {
        const c = decideCaps(base());
        expect(c).toMatchObject({ enabled: true, fetch: true, streaming: true, buffered: false, workerBlob: true, workerFetch: true, workerScriptUrl: true });
        expect(hostLevel(c)).toBe('W');
        expect(c.notes).toEqual([]);
    });

    it('buffered bodies (Windows-like): fetch works, no streaming', () => {
        const c = decideCaps(base({ streamArrivals: [500, 500, 501, 501, 501, 502] }));
        expect(c).toMatchObject({ fetch: true, streaming: false, buffered: true });
        expect(c.notes.join(' ')).toMatch(/whole/);
    });

    it('stream probe failure counts as buffered', () => {
        const c = decideCaps(base({ streamArrivals: [] }));
        expect(c).toMatchObject({ fetch: true, streaming: false, buffered: true });
    });

    it('blob worker works but cannot fetch: level M', () => {
        const c = decideCaps(base({ workerFetch: fail('blocked') }));
        expect(c).toMatchObject({ workerBlob: true, workerFetch: false });
        expect(hostLevel(c)).toBe('M');
        expect(c.notes.join(' ')).toMatch(/worker fetch: blocked/);
    });

    it('no worker at all: level I', () => {
        const c = decideCaps(base({ workerBlob: fail('Worker unavailable'), workerFetch: fail('x'), workerScriptUrl: fail('y') }));
        expect(c).toMatchObject({ workerBlob: false, workerFetch: false, workerScriptUrl: false });
        expect(hostLevel(c)).toBe('I');
    });

    it('worker fetch result is ignored when the blob worker itself failed', () => {
        const c = decideCaps(base({ workerBlob: fail('no'), workerFetch: ok }));
        expect(c.workerFetch).toBe(false);
    });

    it('failed ping: bulk stays off, level L, nothing else is trusted', () => {
        const c = decideCaps(base({ ping: fail('Failed to fetch') }));
        expect(c).toMatchObject({ enabled: true, fetch: false, streaming: false, buffered: false, workerBlob: false });
        expect(hostLevel(c)).toBe('L');
        expect(c.notes[0]).toMatch(/Failed to fetch/);
    });

    it('disabled by backend: level L', () => {
        const c = decideCaps(base({ enabled: false }));
        expect(c).toMatchObject({ enabled: false, fetch: false });
        expect(hostLevel(c)).toBe('L');
    });

    it('script-url worker is informational only', () => {
        const c = decideCaps(base({ workerScriptUrl: fail('nope') }));
        expect(c.workerScriptUrl).toBe(false);
        expect(hostLevel(c)).toBe('W');
    });
});

describe('runBulkProbe', () => {
    let storage: Record<string, string>;
    let routes: Record<string, () => Response>;

    const enc = new TextEncoder();
    function delayedStream(n: number, delay: number): Response {
        let i = 0;
        const body = new ReadableStream<Uint8Array>({
            async pull(controller) {
                if (i >= n) return controller.close();
                if (i > 0) await new Promise((r) => setTimeout(r, delay));
                controller.enqueue(enc.encode(`{"i":${i++}}\n`));
            },
        });
        return new Response(body, { headers: { 'Content-Type': 'application/x-ndjson' } });
    }

    beforeEach(() => {
        storage = {};
        (globalThis as any).localStorage = {
            getItem: (k: string) => (k in storage ? storage[k] : null),
            setItem: (k: string, v: string) => { storage[k] = v; },
            removeItem: (k: string) => { delete storage[k]; },
        };
        resetBulkCaps();
        app.bench.mockReset();
        app.report.mockReset().mockResolvedValue(undefined);
        app.bench.mockImplementation(async (n: number) => Array.from({ length: n }, (_, i) => ({ i })));
        setBulkInfoLoader(async () => ({ enabled: true, base: '/_kubikles/v1', token: 't', streaming: true, server: false, version: 1 }));
        routes = {
            'probe/ping': () => new Response(JSON.stringify({ ok: true }), { headers: { 'Content-Type': 'application/json' } }),
            'probe/stream': () => delayedStream(6, 70),
            'probe/bench?objects=2000&mode=json': () => new Response(JSON.stringify({ data: Array.from({ length: 2000 }, (_, i) => ({ i })) })),
            'probe/bench?objects=2000&mode=ndjson': () => new Response(Array.from({ length: 2000 }, (_, i) => `{"i":${i}}`).join('\n') + '\n'),
            'probe/bytes?mb=1': () => new Response(new Uint8Array(1 << 20)),
            'probe/bytes?mb=16': () => new Response(new Uint8Array(16 << 20)),
        };
        (globalThis as any).fetch = vi.fn(async (url: string) => {
            const path = url.replace('/_kubikles/v1/', '');
            const key = Object.keys(routes).find((k) => path === k || (k === 'probe/stream' && path.startsWith('probe/stream')));
            if (!key) return new Response('{"error":"nope"}', { status: 404 });
            return routes[key]();
        });
    });

    afterEach(() => {
        delete (globalThis as any).fetch;
        delete (globalThis as any).localStorage;
    });

    it('measures, applies, reports and caches a summary', async () => {
        const caps = await runBulkProbe({ force: true });
        expect(caps).toMatchObject({ enabled: true, fetch: true, streaming: true, buffered: false });
        // No Worker in the node test environment.
        expect(caps.workerBlob).toBe(false);
        expect(hostLevel(caps)).toBe('I');
        expect(caps.benchMs.bulkJson).not.toBeNull();
        expect(caps.benchMs.bulkNdjson).not.toBeNull();
        expect(caps.benchMs.ipc).not.toBeNull();
        expect(Object.keys(caps.bytesMs ?? {})).toEqual(['1', '16']);
        expect(peekBulkCaps()).toBe(caps);
        expect(app.report).toHaveBeenCalledTimes(1);
        expect(app.report.mock.calls[0][0]).toMatchObject({ fetch: true, hostLevel: 'I' });
        expect(JSON.parse(storage.kubikles_bulk_probe).caps.fetch).toBe(true);
    });

    it('reuses the cached summary after a successful ping and does not re-measure', async () => {
        await runBulkProbe({ force: true });
        resetBulkCaps();
        app.bench.mockClear();
        const caps = await runBulkProbe();
        expect(caps.cached).toBe(true);
        expect(caps.fetch).toBe(true);
        expect(app.bench).not.toHaveBeenCalled();
    });

    it('a cached summary is ignored when the ping fails, and bulk stays off', async () => {
        await runBulkProbe({ force: true });
        resetBulkCaps();
        routes['probe/ping'] = () => new Response('{"error":"x"}', { status: 403 });
        const caps = await runBulkProbe();
        expect(caps.cached).toBeUndefined();
        expect(caps.fetch).toBe(false);
        expect(hostLevel(caps)).toBe('L');
    });

    it('records failed fetch as a note and leaves bulk off', async () => {
        (globalThis as any).fetch = vi.fn(async () => { throw new TypeError('Failed to fetch'); });
        const caps = await runBulkProbe({ force: true });
        expect(caps.fetch).toBe(false);
        expect(caps.notes.join(' ')).toMatch(/Failed to fetch/);
    });

    it('does nothing when the backend disabled the transport', async () => {
        setBulkInfoLoader(async () => ({ enabled: false, base: '/_kubikles/v1', token: '', streaming: false, server: false, version: 1 }));
        const caps = await runBulkProbe({ force: true });
        expect(caps.enabled).toBe(false);
        expect((globalThis as any).fetch).not.toHaveBeenCalled();
    });

    it('shares one run between concurrent callers', async () => {
        const [a, b] = await Promise.all([runBulkProbe({ force: true }), runBulkProbe({ force: true })]);
        expect(a).toBe(b);
        expect(app.report).toHaveBeenCalledTimes(1);
    });
});
