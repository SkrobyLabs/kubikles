import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ViewGoneError } from './protocol';
import { FakeBackend, FakeFeed, baseSpec, flush, notice } from './testSupport';
import { RANGE_SHRINK_DELAY_MS, createProxyHandle } from './viewClientProxy';
import type { ViewHandle } from './viewClient';
import { ViewHost, type FailureInfo } from './viewHost';
import { backendFetch, backendServices, makeFakeWorker, type FakeWorker } from './workerTestSupport';
import type { WorkerMode } from './workerProtocol';

const last = <T>(a: T[]): T | undefined => a[a.length - 1];

let backend: FakeBackend;
let feed: FakeFeed;
let worker: FakeWorker;
let host: ViewHost;
let proxy: ViewHandle;
let fetchLog: string[];
let failures: FailureInfo[];
let fetchImpl: typeof fetch;
let streaming = false;

function setup(mode: WorkerMode = 'W', fetchOverride?: typeof fetch): void {
    fetchLog = [];
    failures = [];
    fetchImpl = fetchOverride ?? backendFetch(backend, fetchLog);
    worker = makeFakeWorker({ fetchImpl });
    host = new ViewHost({
        mode,
        createWorker: () => worker,
        services: backendServices(backend),
        notices: feed,
        initInfo: () => ({ origin: 'http://app.test', base: '/_kubikles/v1', token: 'tok', streaming }),
        onFailure: (_h, info) => { failures.push(info); },
    });
    host.start();
    proxy = createProxyHandle(host, {
        requestFrame: cb => setTimeout(cb, 16),
        cancelFrame: h => clearTimeout(h as ReturnType<typeof setTimeout>),
    });
}

async function frames(n = 1): Promise<void> {
    for (let i = 0; i < n; i++) {
        await flush(6);
        await vi.advanceTimersByTimeAsync(20);
        await flush(6);
    }
}

beforeEach(() => {
    vi.useFakeTimers();
    backend = new FakeBackend();
    feed = new FakeFeed();
    streaming = false;
});

afterEach(() => {
    proxy?.close();
    host?.dispose();
    vi.useRealTimers();
});

describe('level W: the worker fetches and applies', () => {
    it('opens, mirrors the requested range and fetches windows inside the worker', async () => {
        backend.add(40);
        setup('W');
        proxy.setRange(0, 10);
        await proxy.open(baseSpec());
        await frames();
        expect(proxy.getSnapshot().status).toBe('open');
        expect(proxy.getSnapshot().columns).toEqual(['name', 'age']);
        expect(proxy.getSnapshot().filtered).toBe(40);
        expect(proxy.rowAt(0)?.metadata.name).toBe('r0');
        expect(proxy.rowAt(0)?.cells.name).toBe('r0');
        expect(proxy.rowAt(9)?.metadata.name).toBe('r9');
        expect(fetchLog.some(p => p.includes('/views/v1/window'))).toBe(true);
        expect(worker.sent.some(m => m.t === 'rpc' && m.op === 'window')).toBe(false);
        expect(worker.sent.some(m => m.t === 'rpc' && m.op === 'open')).toBe(true);
    });

    it('mirrors only rows near the range, not the whole window the client holds', async () => {
        backend.add(500);
        setup('W');
        proxy.setRange(0, 10);
        await proxy.open(baseSpec());
        await frames();
        expect(proxy.rowAt(5)).toBeDefined();
        expect(proxy.rowAt(300)).toBeUndefined();
        expect(proxy.loadedRows().length).toBeLessThanOrEqual(30);
    });

    it('moves the mirror when the range moves', async () => {
        backend.add(300);
        setup('W');
        proxy.setRange(0, 10);
        await proxy.open(baseSpec());
        await frames();
        proxy.setRange(200, 210);
        await frames(6);
        expect(proxy.rowAt(205)?.metadata.name).toBe('r205');
        expect(proxy.rowAt(0)).toBeUndefined();
    });

    it('sends only changed rows after a notice and keeps identity of the others', async () => {
        backend.add(20);
        setup('W');
        proxy.setRange(0, 10);
        await proxy.open(baseSpec());
        await frames();
        const before = proxy.rowAt(3)!;
        const other = proxy.rowAt(4)!;
        const rowsMsgs = () => worker.sent.filter(m => m.t === 'view.rows');
        const n = rowsMsgs().length;

        backend.touch('r3');
        feed.emit(notice({ version: backend.version, total: 20, filtered: 20, windowChanged: true, loaded: 20, expected: 20 }));
        await frames(2);

        const after = proxy.rowAt(3)!;
        expect(after).not.toBe(before);
        expect(after.__seq).toBeGreaterThan(before.__seq);
        expect(proxy.rowAt(4)).toBe(other);
        const fresh = rowsMsgs().slice(n);
        const sentRows = fresh.flatMap(m => (m.t === 'view.rows' ? m.rows : []));
        expect(sentRows.map(r => r.metadata.uid)).toEqual(['r3']);
    });

    it('forwards only through the host: notices reach the worker as messages', async () => {
        backend.add(5);
        setup('W');
        await proxy.open(baseSpec());
        await frames();
        feed.emit(notice({ version: 99, total: 5, filtered: 5, phase: 'loading', loaded: 2, expected: 9 }));
        await frames();
        expect(worker.received.some(m => m.t === 'notice')).toBe(true);
        expect(proxy.getSnapshot().state?.phase).toBe('loading');
        expect(proxy.getSnapshot().state?.loaded).toBe(2);
    });

    it('forwards reconnects', async () => {
        backend.add(3);
        setup('W');
        await proxy.open(baseSpec());
        await frames();
        const calls = backend.windowCalls.length;
        feed.reconnect();
        await frames(2);
        expect(worker.received.some(m => m.t === 'reconnect')).toBe(true);
        expect(backend.windowCalls.length).toBeGreaterThan(calls);
    });
});

describe('level M: the main thread fetches and transfers bytes', () => {
    it('serves windows from transferred buffers and never fetches in the worker', async () => {
        backend.add(15);
        const guard = vi.fn(async () => { throw new Error('worker must not fetch'); }) as unknown as typeof fetch;
        setup('M', guard);
        proxy.setRange(0, 10);
        await proxy.open(baseSpec());
        await frames();
        expect(guard).not.toHaveBeenCalled();
        expect(proxy.rowAt(2)?.metadata.name).toBe('r2');
        expect(worker.sent.some(m => m.t === 'rpc' && m.op === 'bytes')).toBe(true);
    });

    it('reads refs from transferred buffers', async () => {
        backend.add(8);
        setup('M', vi.fn() as unknown as typeof fetch);
        await proxy.open(baseSpec());
        await frames();
        const refs = await proxy.selectAllRefs();
        expect(refs.map(r => r.uid)).toEqual(['r0', 'r1', 'r2', 'r3', 'r4', 'r5', 'r6', 'r7']);
    });
});

describe('requests', () => {
    it('returns small refs as arrays and large ones as a packed transfer', async () => {
        backend.add(12_000);
        setup('W');
        await proxy.open(baseSpec());
        await frames();
        const refs = await proxy.selectAllRefs();
        expect(refs).toHaveLength(12_000);
        expect(refs[11_999]).toEqual({ uid: 'r11999', ns: 'ns', name: 'r11999' });
        const res = worker.sent.filter(m => m.t === 'res').map(m => (m.t === 'res' ? m.value : null));
        expect(res.some(v => (v as { packed?: boolean } | null)?.packed === true)).toBe(true);

        const small = await proxy.refs(0, 4);
        expect(small).toHaveLength(5);
        expect(Array.isArray(small)).toBe(true);
    });

    it('fetches objects in the worker (level W) and facets over rpc', async () => {
        backend.add(2);
        setup('W');
        await proxy.open(baseSpec());
        await frames();
        const obj = await proxy.getObject({ metadata: { uid: 'r1', name: 'r1', namespace: 'ns' } });
        expect(obj).toMatchObject({ via: 'fetch', metadata: { name: 'r1' } });
        const facet = await proxy.facet('name');
        expect(facet.values).toEqual(['a']);
    });

    it('relays selection and selection-stale notices', async () => {
        backend.add(4);
        backend.selectionStale = ['r9'];
        setup('W');
        await proxy.open(baseSpec());
        await frames();
        const stale: string[][] = [];
        proxy.onSelectionStale(u => stale.push(u));
        proxy.setSelection(['r1', 'r9']);
        await vi.advanceTimersByTimeAsync(300);
        await frames();
        expect(last(backend.selectionCalls)).toEqual(['r1', 'r9']);
        expect(stale).toEqual([['r9']]);
    });

    it('relays pod transitions from the worker to the page', async () => {
        backend.add(2);
        setup('W');
        await proxy.open(baseSpec({ notifyOn: ['pods'] }));
        await frames();
        const seen: any[][] = [];
        proxy.onTransitions(e => seen.push(e));
        const events = [{ kind: 'deleted' as const, uid: 'u', namespace: 'd', name: 'p', owner: 'ReplicaSet/w' }];
        feed.emit(notice({ version: 0, transitions: events }));
        await frames();
        expect(seen).toEqual([events]);
        expect(worker.received.find(m => m.t === 'view.open')).toBeDefined();
    });

    it('rejects requests after close', async () => {
        backend.add(1);
        setup('W');
        await proxy.open(baseSpec());
        proxy.close();
        await expect(proxy.facet('name')).rejects.toThrow();
        expect(worker.received.some(m => m.t === 'view.close')).toBe(true);
    });
});

describe('updates', () => {
    it('forwards patches, retains the merged spec and drops mirrored rows on a column change', async () => {
        backend.add(6);
        setup('W');
        proxy.setRange(0, 10);
        await proxy.open(baseSpec());
        await frames();
        expect(proxy.rowAt(0)).toBeDefined();
        proxy.update({ search: 'x', columns: ['name'] });
        expect(proxy.rowAt(0)).toBeUndefined();
        expect(proxy.spec()).toMatchObject({ search: 'x', columns: ['name'] });
        await frames(3);
        expect(last(backend.updateCalls)?.patch).toMatchObject({ search: 'x' });
        expect(proxy.getSnapshot().columns).toEqual(['name']);
        expect(proxy.rowAt(0)?.cells).toEqual({ name: 'r0' });
    });

    it('keeps mirrored rows when a column patch repeats the current columns', async () => {
        // A list that mounts over an open view pushes its columns again. The worker treats that
        // as a no-op and sends no rows, so dropping the mirror would leave placeholder rows.
        backend.add(6);
        setup('W');
        proxy.setRange(0, 10);
        await proxy.open({ ...baseSpec(), columns: ['name'] });
        await frames();
        const row = proxy.rowAt(0);
        expect(row).toBeDefined();
        proxy.update({ columns: ['name'] });
        expect(proxy.rowAt(0)).toBe(row);
        await frames(3);
        expect(proxy.rowAt(0)).toBeDefined();
    });

    it('keeps mirrored rows when the range shrinks and grows back at once', async () => {
        // A list mounting over an open view reports a few rows first and its full range right
        // after. The rows beyond the small range must stay available throughout.
        backend.add(60);
        setup('W');
        proxy.setRange(0, 40);
        await proxy.open(baseSpec());
        await frames();
        expect(proxy.rowAt(39)).toBeDefined();
        proxy.setRange(0, 1);
        await frames(3);
        expect(proxy.rowAt(39)).toBeDefined();
        proxy.setRange(0, 40);
        await vi.advanceTimersByTimeAsync(RANGE_SHRINK_DELAY_MS * 2);
        await frames(3);
        expect(proxy.rowAt(39)).toBeDefined();
    });

    it('still applies a range that stays smaller', async () => {
        backend.add(200);
        setup('W');
        proxy.setRange(0, 100);
        await proxy.open(baseSpec());
        await frames();
        expect(proxy.rowAt(150)).toBeDefined();
        proxy.setRange(0, 5);
        await vi.advanceTimersByTimeAsync(RANGE_SHRINK_DELAY_MS + 50);
        await frames(3);
        expect(proxy.rowAt(0)).toBeDefined();
        expect(proxy.rowAt(150)).toBeUndefined();
    });
});

describe('frame coalescing and ordering', () => {
    it('publishes one update per frame for a burst of worker messages', async () => {
        backend.add(30);
        setup('W');
        proxy.setRange(0, 10);
        await proxy.open(baseSpec());
        await frames();
        const cb = vi.fn();
        proxy.subscribe(cb);
        const epoch = proxy.getSnapshot().epoch;
        for (let i = 0; i < 5; i++) {
            backend.touch(`r${i}`);
            feed.emit(notice({ version: backend.version, total: 30, filtered: 30, windowChanged: true }));
        }
        await flush(12);
        await vi.advanceTimersByTimeAsync(20);
        await flush(12);
        expect(cb.mock.calls.length).toBeLessThanOrEqual(2);
        expect(proxy.getSnapshot().epoch).toBeGreaterThan(epoch);
        expect(proxy.rowAt(4)!.__seq).toBe(backend.items[4].seq);
    });

    it('applies state before the rows that followed it', async () => {
        backend.add(3);
        setup('W');
        proxy.setRange(0, 10);
        await proxy.open(baseSpec());
        await frames();
        expect(proxy.getSnapshot().filtered).toBe(3);
        expect(proxy.loadedRows().map(r => r.metadata.uid)).toEqual(['r0', 'r1', 'r2']);
    });
});

describe('errors', () => {
    it('falls back to the IPC twin when the worker fetch fails and keeps working', async () => {
        backend.add(5);
        const failing = vi.fn(async () => { throw new TypeError('network down'); }) as unknown as typeof fetch;
        setup('W', failing);
        proxy.setRange(0, 10);
        await proxy.open(baseSpec());
        await frames();
        expect(proxy.rowAt(1)?.metadata.name).toBe('r1');
        expect(worker.sent.some(m => m.t === 'rpc' && m.op === 'window')).toBe(true);
    });

    it('asks the host to step down after repeated worker fetch failures', async () => {
        backend.add(5);
        const failing = vi.fn(async () => { throw new TypeError('network down'); }) as unknown as typeof fetch;
        setup('W', failing);
        await proxy.open(baseSpec());
        await frames();
        for (let i = 0; i < 4; i++) {
            proxy.refresh();
            await frames();
        }
        expect(failures.map(f => f.fatal)).toEqual([false]);
        expect(failures[0].reason).toMatch(/fetch failing/);
    });

    it('does not count application errors as transport failures', async () => {
        backend.add(5);
        setup('W', (async () => new Response(JSON.stringify({ error: 'boom' }), { status: 500 })) as unknown as typeof fetch);
        await proxy.open(baseSpec());
        await frames();
        for (let i = 0; i < 5; i++) {
            proxy.refresh();
            await frames();
        }
        expect(failures).toEqual([]);
    });

    it('re-opens from the retained spec when the view is gone (410)', async () => {
        backend.add(5);
        setup('W');
        proxy.setRange(0, 10);
        await proxy.open(baseSpec());
        await frames();
        expect(backend.openCalls).toHaveLength(1);
        backend.exists = false;
        backend.windowError = null;
        proxy.refresh();
        await frames(2);
        backend.exists = true;
        await vi.advanceTimersByTimeAsync(1000);
        await frames(3);
        expect(backend.openCalls.length).toBeGreaterThanOrEqual(2);
        expect(last(backend.openCalls)?.viewId).toBe('v1');
        expect(ViewGoneError).toBeDefined();
    });

    it('reports an open that cannot reach the worker as a failed snapshot', async () => {
        backend.add(1);
        setup('W');
        host.dispose();
        await proxy.open(baseSpec());
        expect(proxy.getSnapshot().status).toBe('failed');
    });
});

describe('fetch.call for large bodies', () => {
    it('parses a call response inside the worker', async () => {
        setup('W');
        await frames();
        const r = await host.fetchCall('call/GetThingYaml?args=%5B%5D');
        expect(r).toMatchObject({ ok: true, status: 200, doc: { data: 'called GetThingYaml' } });
        expect(fetchLog.some(p => p.includes('/call/GetThingYaml'))).toBe(true);
    });
});

describe('worker stats', () => {
    it('reports resident rows and decode time through heartbeats', async () => {
        backend.add(50);
        setup('W');
        proxy.setRange(0, 10);
        await proxy.open(baseSpec());
        await frames();
        await vi.advanceTimersByTimeAsync(2100);
        const st = host.status();
        expect(st.acked).toBe(true);
        expect(st.worker?.views).toBe(1);
        expect(st.worker?.rowsResident).toBeGreaterThan(0);
        expect(st.worker?.fetches).toBeGreaterThan(0);
    });
});

describe('streamed NDJSON', () => {
    it('reads refs incrementally when the body streams, including a line split across chunks', async () => {
        backend.add(3);
        streaming = true;
        const enc = new TextEncoder();
        const base = backendFetch(backend, []);
        const streamed = (async (input: RequestInfo | URL) => {
            if (!String(input).includes('/refs')) return base(input);
            const body = new ReadableStream<Uint8Array>({
                start(c) {
                    c.enqueue(enc.encode('{"version":1,"total":3,"filtered":3}\n["r0","ns","r0"]\n["r1","n'));
                    c.enqueue(enc.encode('s","r1"]\n["r2","ns","r2"]\n'));
                    c.close();
                },
            });
            return new Response(body, { status: 200, headers: { 'Content-Type': 'application/x-ndjson' } });
        }) as typeof fetch;
        setup('W', streamed);
        await proxy.open(baseSpec());
        await frames();
        const refs = await proxy.selectAllRefs();
        expect(refs).toEqual([
            { uid: 'r0', ns: 'ns', name: 'r0' }, { uid: 'r1', ns: 'ns', name: 'r1' }, { uid: 'r2', ns: 'ns', name: 'r2' },
        ]);
    });
});

describe('worker watchdog', () => {
    it('fails a worker that stops answering heartbeats', async () => {
        backend.add(5);
        setup('W');
        await proxy.open(baseSpec());
        await frames();
        await vi.advanceTimersByTimeAsync(2100);
        expect(host.status().acked).toBe(true);
        worker.hang();
        await vi.advanceTimersByTimeAsync(10_000);
        expect(failures).toHaveLength(1);
        expect(failures[0].reason).toMatch(/missed heartbeats/);
    });

    it('does not blame the worker when the whole page was suspended', async () => {
        backend.add(5);
        setup('W');
        await proxy.open(baseSpec());
        await frames();
        await vi.advanceTimersByTimeAsync(2100);
        worker.hang();
        // Sleep or a frozen tab: the clock jumps, the first timer tick after resume is late.
        vi.setSystemTime(Date.now() + 60_000);
        await vi.advanceTimersByTimeAsync(2000);
        expect(failures).toHaveLength(0);
        // A worker that is really hung is still caught afterwards.
        await vi.advanceTimersByTimeAsync(10_000);
        expect(failures).toHaveLength(1);
    });
});
