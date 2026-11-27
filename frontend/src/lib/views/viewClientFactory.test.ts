import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { BulkCaps } from '../bulk/types';
import { FakeBackend, FakeFeed, baseSpec, flush, notice } from './testSupport';
import { createViewClient } from './viewClient';
import { ViewFactory, chooseLevel, type ViewLevel, type ViewWorkerSetting } from './viewClientFactory';
import { backendFetch, backendServices, makeFakeWorker, type FakeWorker } from './workerTestSupport';

function caps(over: Partial<BulkCaps> = {}): BulkCaps {
    return {
        enabled: true, fetch: true, streaming: false, buffered: true, workerBlob: true, workerFetch: true,
        workerScriptUrl: true, degraded: false, notes: [], probedAt: 1, userAgent: 't',
        benchMs: { bulkJson: null, bulkNdjson: null, ipc: null }, ...over,
    };
}

describe('chooseLevel (probe state x viewWorker setting)', () => {
    const cases: Array<[string, Partial<BulkCaps>, ViewWorkerSetting, boolean, ViewLevel]> = [
        ['worker fetch works', {}, 'auto', true, 'W'],
        ['blob worker only', { workerFetch: false }, 'auto', true, 'M'],
        ['no blob worker', { workerBlob: false, workerFetch: false }, 'auto', true, 'I'],
        ['worker fetch reported without blob', { workerBlob: false }, 'auto', true, 'I'],
        ['fetch to the bulk route fails', { fetch: false, workerBlob: false, workerFetch: false }, 'auto', true, 'I'],
        ['bulk disabled by backend', { enabled: false }, 'auto', true, 'I'],
        ['bulk degraded after failures', { degraded: true }, 'auto', true, 'I'],
        ['setting off beats a perfect probe', {}, 'off', true, 'I'],
        ['no Worker in this runtime', {}, 'auto', false, 'I'],
        ['unprobed desktop caps', { fetch: false, workerBlob: false, workerFetch: false, probedAt: 0 }, 'auto', true, 'I'],
    ];
    it.each(cases)('%s', (_name, over, setting, supported, expected) => {
        expect(chooseLevel(caps(over), setting, supported)).toBe(expected);
    });
});

const last = <T>(a: T[]): T | undefined => a[a.length - 1];

let backend: FakeBackend;
let feed: FakeFeed;
let workers: FakeWorker[];
let currentCaps: BulkCaps;
let setting: ViewWorkerSetting;
let supported: boolean;
let factory: ViewFactory;
let hangOnCreate: boolean;
let fetchLog: string[];

function makeFactory(): ViewFactory {
    return new ViewFactory({
        getCaps: async () => currentCaps,
        peekCaps: () => currentCaps,
        getSetting: () => setting,
        workerSupported: () => supported,
        createWorker: () => {
            const w = makeFakeWorker({ fetchImpl: backendFetch(backend, fetchLog) });
            if (hangOnCreate) w.hang();
            workers.push(w);
            return w;
        },
        services: () => backendServices(backend),
        notices: () => feed,
        initInfo: () => ({ origin: 'http://app.test', base: '/_kubikles/v1', token: 'tok', streaming: false }),
        makeInline: () => createViewClient({ transport: backend, notices: feed }),
        proxy: { requestFrame: cb => setTimeout(cb, 16), cancelFrame: h => clearTimeout(h as ReturnType<typeof setTimeout>) },
    });
}

async function settle(ms = 100): Promise<void> {
    await flush(8);
    await vi.advanceTimersByTimeAsync(ms);
    await flush(8);
}

beforeEach(() => {
    vi.useFakeTimers();
    backend = new FakeBackend();
    feed = new FakeFeed();
    workers = [];
    fetchLog = [];
    currentCaps = caps();
    setting = 'auto';
    supported = true;
    hangOnCreate = false;
    factory = makeFactory();
});

afterEach(() => {
    factory.dispose();
    vi.useRealTimers();
});

describe('level selection at open', () => {
    it('runs the engine in a worker at level W', async () => {
        backend.add(10);
        const h = factory.createHandle();
        h.setRange(0, 10);
        await h.open(baseSpec());
        await settle();
        expect(factory.status().level).toBe('W');
        expect(workers).toHaveLength(1);
        expect(h.getSnapshot().status).toBe('open');
        expect(h.rowAt(0)?.metadata.name).toBe('r0');
        expect(fetchLog.some(p => p.includes('/window'))).toBe(true);
    });

    it('uses a worker with main-thread fetches at level M', async () => {
        currentCaps = caps({ workerFetch: false });
        backend.add(10);
        const h = factory.createHandle();
        h.setRange(0, 10);
        await h.open(baseSpec());
        await settle();
        expect(factory.status().level).toBe('M');
        expect(workers[0].sent.some(m => m.t === 'rpc' && m.op === 'bytes')).toBe(true);
        expect(h.rowAt(1)?.metadata.name).toBe('r1');
    });

    it('never creates a worker with viewWorker off', async () => {
        setting = 'off';
        backend.add(5);
        const h = factory.createHandle();
        h.setRange(0, 10);
        await h.open(baseSpec());
        await settle();
        expect(workers).toHaveLength(0);
        expect(factory.status().level).toBe('I');
        expect(h.rowAt(0)?.metadata.name).toBe('r0');
    });

    it('runs inline when the probe failed (IPC twin through the binding transport)', async () => {
        currentCaps = caps({ fetch: false, workerBlob: false, workerFetch: false });
        backend.add(5);
        const h = factory.createHandle();
        h.setRange(0, 10);
        await h.open(baseSpec());
        await settle();
        expect(workers).toHaveLength(0);
        expect(h.rowAt(2)?.metadata.name).toBe('r2');
        expect(backend.windowCalls.length).toBeGreaterThan(0);
    });

    it('runs inline without Worker support', async () => {
        supported = false;
        backend.add(3);
        const h = factory.createHandle();
        await h.open(baseSpec());
        await settle();
        expect(workers).toHaveLength(0);
        expect(h.getSnapshot().status).toBe('open');
    });

    it('shares one worker between views', async () => {
        backend.add(3);
        const a = factory.createHandle();
        const b = factory.createHandle();
        await Promise.all([a.open(baseSpec({ viewId: 'a' })), b.open(baseSpec({ viewId: 'b' }))]);
        await settle();
        expect(workers).toHaveLength(1);
        expect(factory.status().viewsOpen).toBe(2);
    });

    it('forwards notices to every worker view', async () => {
        backend.add(5);
        const h = factory.createHandle();
        await h.open(baseSpec());
        await settle();
        feed.emit(notice({ version: 50, total: 5, filtered: 5, phase: 'refreshing', loaded: 1, expected: 5 }));
        await settle();
        expect(h.getSnapshot().state?.phase).toBe('refreshing');
        expect(factory.status().noticesPerSec).toBeGreaterThanOrEqual(0);
    });
});

describe('unbound use', () => {
    it('works when methods are detached from the handle (useSyncExternalStore style)', async () => {
        backend.add(3);
        const h = factory.createHandle();
        const { subscribe, getSnapshot, rowAt, setRange } = h;
        const cb = vi.fn();
        const off = subscribe(cb);
        setRange(0, 10);
        await h.open(baseSpec());
        await settle();
        expect(getSnapshot().status).toBe('open');
        expect(rowAt(0)?.metadata.name).toBe('r0');
        expect(cb).toHaveBeenCalled();
        off();
    });
});

describe('watchdog degradation', () => {
    async function openWithState() {
        backend.add(60);
        const h = factory.createHandle();
        h.setRange(10, 25);
        await h.open(baseSpec());
        await settle();
        h.update({ search: 'abc', sort: { key: 'age', dir: 'desc' } });
        h.setSelection(['r12', 'r13']);
        await vi.advanceTimersByTimeAsync(400);
        await settle();
        return h;
    }

    it('a worker that never acks sends every view straight to inline, keeping spec, range and selection', async () => {
        backend.add(60);
        const h = factory.createHandle();
        h.setRange(10, 25);
        h.setSelection(['r12']);
        hangOnCreate = true;
        const opening = h.open(baseSpec({ search: 'seed' }));
        await flush(8);
        h.update({ search: 'abc' });
        await vi.advanceTimersByTimeAsync(2500);
        await opening.catch(() => {});
        await settle(500);
        const st = factory.status();
        expect(st.ceiling).toBe('I');
        expect(st.watchdog).toHaveLength(1);
        expect(st.watchdog[0]).toMatchObject({ mode: 'W', to: 'I' });
        expect(st.watchdog[0].reason).toMatch(/no init ack/);
        expect(h.getSnapshot().status).toBe('open');
        expect(h.spec()).toMatchObject({ search: 'abc' });
        expect(last(backend.openCalls)).toMatchObject({ search: 'abc' });
        expect(h.rowAt(12)?.metadata.name).toBe('r12');
        expect(last(backend.selectionCalls)).toEqual(['r12']);
        expect(workers[0].terminated).toBe(true);
    });

    it('missed heartbeats step W to M and re-open with the retained state', async () => {
        const h = await openWithState();
        const epochBefore = h.getSnapshot().epoch;
        const opens = backend.openCalls.length;
        workers[0].hang();
        await vi.advanceTimersByTimeAsync(9000);
        await settle(500);

        const st = factory.status();
        expect(st.watchdog).toHaveLength(1);
        expect(st.watchdog[0]).toMatchObject({ mode: 'W', to: 'M' });
        expect(st.watchdog[0].reason).toMatch(/missed heartbeats/);
        expect(workers).toHaveLength(2);
        expect(workers[0].terminated).toBe(true);
        expect(backend.openCalls.length).toBe(opens + 1);
        expect(last(backend.openCalls)).toMatchObject({ search: 'abc', sort: { key: 'age', dir: 'desc' } });
        expect(h.getSnapshot().status).toBe('open');
        expect(h.rowAt(12)?.metadata.name).toBe('r12');
        expect(h.getSnapshot().epoch).toBeGreaterThan(epochBefore);
        // selection survived and was re-sent to the new engine
        expect(last(backend.selectionCalls)).toEqual(['r12', 'r13']);
        // the range survived: the new worker mirrors rows around 10..25
        expect(workers[1].received.some(m => m.t === 'view.setRange' && m.start === 10 && m.end === 25)).toBe(true);
    });

    it('M failing steps down to inline; a later view starts inline', async () => {
        const h = await openWithState();
        workers[0].hang();
        await vi.advanceTimersByTimeAsync(9000);
        await settle(500);
        workers[1].hang();
        await vi.advanceTimersByTimeAsync(9000);
        await settle(500);
        const st = factory.status();
        expect(st.watchdog.map(w => w.to)).toEqual(['M', 'I']);
        expect(st.ceiling).toBe('I');
        expect(h.getSnapshot().status).toBe('open');
        expect(h.rowAt(12)?.metadata.name).toBe('r12');

        const later = factory.createHandle();
        await later.open(baseSpec({ viewId: 'later' }));
        await settle();
        expect(workers).toHaveLength(2);
        expect(factory.status().level).toBe('I');
    });

    it('a worker error event degrades one level', async () => {
        const h = await openWithState();
        workers[0].crash('script error');
        await settle(500);
        expect(factory.status().watchdog[0]).toMatchObject({ mode: 'W', to: 'M' });
        expect(factory.status().watchdog[0].reason).toMatch(/script error/);
        expect(h.getSnapshot().status).toBe('open');
    });

    it('repeated worker fetch failures step W down to M', async () => {
        currentCaps = caps();
        const failing = vi.fn(async () => { throw new TypeError('nope'); }) as unknown as typeof fetch;
        factory = new ViewFactory({
            getCaps: async () => currentCaps,
            getSetting: () => setting,
            workerSupported: () => true,
            createWorker: () => {
                const w = makeFakeWorker({ fetchImpl: workers.length === 0 ? failing : backendFetch(backend, fetchLog) });
                workers.push(w);
                return w;
            },
            services: () => backendServices(backend),
            notices: () => feed,
            initInfo: () => ({ origin: 'http://app.test', base: '/_kubikles/v1', token: 'tok', streaming: false }),
            makeInline: () => createViewClient({ transport: backend, notices: feed }),
            proxy: { requestFrame: cb => setTimeout(cb, 16), cancelFrame: h => clearTimeout(h as ReturnType<typeof setTimeout>) },
        });
        backend.add(5);
        const h = factory.createHandle();
        h.setRange(0, 10);
        await h.open(baseSpec());
        await settle();
        for (let i = 0; i < 4; i++) {
            h.refresh();
            await settle();
        }
        await settle(500);
        expect(factory.status().watchdog[0]).toMatchObject({ mode: 'W', to: 'M' });
        expect(workers).toHaveLength(2);
        expect(h.rowAt(1)?.metadata.name).toBe('r1');
    });

    it('a view closed during degradation stays closed', async () => {
        const h = await openWithState();
        workers[0].hang();
        h.close();
        await vi.advanceTimersByTimeAsync(9000);
        await settle(500);
        expect(h.getSnapshot().status).toBe('closed');
        expect(factory.status().views).toBe(0);
    });
});

describe('bulk calls in the worker', () => {
    it('returns null when the session is not at level W', async () => {
        setting = 'off';
        expect(await factory.fetchCallInWorker('call/GetXYaml?args=%5B%5D')).toBeNull();
        expect(workers).toHaveLength(0);
    });

    it('returns null at level M and I', async () => {
        currentCaps = caps({ workerFetch: false });
        expect(await factory.fetchCallInWorker('call/GetXYaml?args=%5B%5D')).toBeNull();
        currentCaps = caps({ workerBlob: false, workerFetch: false });
        expect(await factory.fetchCallInWorker('call/GetXYaml?args=%5B%5D')).toBeNull();
    });

    it('parses the body in the worker at level W, creating the worker on demand', async () => {
        const p = factory.fetchCallInWorker('call/GetXYaml?args=%5B%5D');
        await settle();
        const r = await p;
        expect(r).toMatchObject({ ok: true, doc: { data: 'called GetXYaml' } });
        expect(workers).toHaveLength(1);
    });

    it('answers null once the ceiling dropped below W', async () => {
        backend.add(2);
        const h = factory.createHandle();
        await h.open(baseSpec());
        await settle();
        workers[0].crash('dead');
        await settle(500);
        expect(await factory.fetchCallInWorker('call/GetXYaml?args=%5B%5D')).toBeNull();
    });
});
