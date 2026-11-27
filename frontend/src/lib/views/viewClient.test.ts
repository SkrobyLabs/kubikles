import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createViewClient, type ViewHandle } from './viewClient';
import { ViewGoneError } from './protocol';
import { FakeBackend, FakeFeed, baseSpec, flush, notice } from './testSupport';

const last = <T>(a: T[]): T => a[a.length - 1];

let backend: FakeBackend;
let feed: FakeFeed;
let client: ViewHandle;

function make(over: Partial<Parameters<typeof createViewClient>[0]> = {}): ViewHandle {
    client = createViewClient({ transport: backend, notices: feed, ...over });
    return client;
}

beforeEach(() => {
    vi.useFakeTimers();
    backend = new FakeBackend();
    feed = new FakeFeed();
});

afterEach(() => {
    client?.close();
    vi.useRealTimers();
});

describe('open', () => {
    it('opens with the spec, adopts the columns and fetches the first window', async () => {
        backend.add(5);
        make();
        await client.open(baseSpec());
        await flush();
        expect(backend.openCalls).toHaveLength(1);
        const snap = client.getSnapshot();
        expect(snap.status).toBe('open');
        expect(snap.columns).toEqual(['name', 'age']);
        expect(snap.filtered).toBe(5);
        expect(backend.windowCalls[0]).toMatchObject({ offset: 0, have: 0 });
        expect(client.rowAt(0)?.metadata.name).toBe('r0');
        expect(client.rowAt(0)?.cells).toEqual({ name: 'r0', age: 'age-r0' });
        expect(client.rowAt(9)).toBeUndefined();
    });

    it('reports a failed open and retries from the retained spec, then gives up as permanent', async () => {
        backend.failOpen = 99;
        make({ recoveryBaseMs: 10, maxRecoveries: 3 });
        await client.open(baseSpec());
        expect(client.getSnapshot().status).toBe('failed');
        expect(client.getSnapshot().permanent).toBe(false);
        await vi.advanceTimersByTimeAsync(1000);
        const snap = client.getSnapshot();
        expect(snap.permanent).toBe(true);
        expect(snap.error).toBe('open failed');
        expect(backend.openCalls.length).toBe(4);
    });

    it('reopen() starts over after a permanent failure', async () => {
        backend.failOpen = 99;
        make({ recoveryBaseMs: 10, maxRecoveries: 1 });
        await client.open(baseSpec());
        await vi.advanceTimersByTimeAsync(500);
        expect(client.getSnapshot().permanent).toBe(true);
        backend.failOpen = 0;
        backend.add(2);
        client.reopen();
        await flush();
        expect(client.getSnapshot().status).toBe('open');
        expect(client.getSnapshot().permanent).toBe(false);
    });
});

describe('windows and the have version', () => {
    it('asks with have equal to the last applied version and keeps row identity', async () => {
        backend.add(5);
        make();
        await client.open(baseSpec());
        await flush();
        const first = client.rowAt(1)!;
        const version = client.getSnapshot().state!.version;
        backend.touch('r2');
        feed.emit(notice({ version: backend.version, total: 5, filtered: 5, windowChanged: true }));
        await flush();
        expect(last(backend.windowCalls)!.have).toBe(version);
        // r1 was not resent: same object. r2 changed: new object.
        expect(client.rowAt(1)).toBe(first);
        expect(client.rowAt(2)!.__seq).toBe(backend.version);
    });

    it('changes the epoch only when a window is applied', async () => {
        backend.add(3);
        make();
        await client.open(baseSpec());
        await flush();
        const before = client.getSnapshot().epoch;
        feed.emit(notice({ version: 1, total: 3, filtered: 3 }));
        await flush();
        expect(client.getSnapshot().epoch).toBe(before);
    });

    it('asks again in full when the server omits a row the client no longer holds', async () => {
        backend.add(1000);
        make();
        await client.open(baseSpec());
        await flush();
        client.setRange(0, 20);
        await vi.advanceTimersByTimeAsync(60);
        client.setRange(800, 820);
        await vi.advanceTimersByTimeAsync(60);
        await flush();
        expect(client.rowAt(3)).toBeUndefined();
        // A server that wrongly believes the client still holds r3.
        const original = backend.window.bind(backend);
        let omitted = false;
        backend.window = async (id, offset, limit, have) => {
            const w = await original(id, offset, limit, have);
            if (!omitted && have !== 0) {
                omitted = true;
                delete w.rows.r3;
            }
            return w;
        };
        backend.windowCalls.length = 0;
        client.setRange(0, 20);
        await vi.advanceTimersByTimeAsync(60);
        await flush();
        expect(omitted).toBe(true);
        expect(backend.windowCalls.map(c => c.have === 0)).toEqual([false, true]);
        expect(client.rowAt(3)?.metadata.name).toBe('r3');
    });
});

describe('range and eviction', () => {
    it('debounces range changes and pads the request by one viewport', async () => {
        backend.add(500);
        make();
        await client.open(baseSpec());
        await flush();
        backend.windowCalls.length = 0;
        client.setRange(200, 220);
        client.setRange(210, 230);
        client.setRange(300, 320);
        expect(backend.windowCalls).toHaveLength(0);
        await vi.advanceTimersByTimeAsync(49);
        expect(backend.windowCalls).toHaveLength(0);
        await vi.advanceTimersByTimeAsync(5);
        await flush();
        expect(backend.windowCalls).toHaveLength(1);
        // viewport 20 rows: pad 20 on each side.
        expect(backend.windowCalls[0]).toMatchObject({ offset: 280, limit: 60 });
    });

    it('does not fetch while the viewport stays inside the covered window', async () => {
        backend.add(500);
        make();
        await client.open(baseSpec());
        await flush();
        client.setRange(0, 20);
        await vi.advanceTimersByTimeAsync(100);
        backend.windowCalls.length = 0;
        client.setRange(2, 22);
        await vi.advanceTimersByTimeAsync(100);
        expect(backend.windowCalls).toHaveLength(0);
    });

    it('evicts rows far from the viewport but keeps selected ones', async () => {
        backend.add(1000);
        make();
        await client.open(baseSpec());
        await flush();
        client.setRange(0, 20);
        await vi.advanceTimersByTimeAsync(60);
        await flush();
        expect(client.rowAt(0)).toBeDefined();
        client.setSelection(['r0']);
        client.setRange(800, 820);
        await vi.advanceTimersByTimeAsync(60);
        await flush();
        expect(client.rowAt(0)).toBeUndefined();
        expect(client.rowAt(810)?.metadata.name).toBe('r810');
        // Held rows stay bounded: only the viewport and its neighbours.
        expect(client.loadedRows().length).toBeLessThan(100);
    });

    it('keeps the whole covered window when the viewport grows after the first render', async () => {
        backend.add(300);
        make();
        await client.open(baseSpec());
        await flush();
        client.setRange(0, 3);
        await vi.advanceTimersByTimeAsync(60);
        await flush();
        backend.windowCalls.length = 0;
        client.setRange(0, 28);
        await vi.advanceTimersByTimeAsync(60);
        await flush();
        expect(client.rowAt(20)?.metadata.name).toBe('r20');
        expect(backend.windowCalls).toHaveLength(0);
    });

    it('shows nothing for indices whose window is still in flight', async () => {
        backend.add(300);
        make();
        await client.open(baseSpec());
        await flush();
        backend.hold();
        client.setRange(250, 270);
        await vi.advanceTimersByTimeAsync(60);
        expect(client.rowAt(260)).toBeUndefined();
        backend.releaseHold();
        await flush();
        expect(client.rowAt(260)).toBeDefined();
    });
});

describe('notices', () => {
    it('refetches when the window changed and only merges counts otherwise', async () => {
        backend.add(5);
        make();
        await client.open(baseSpec());
        await flush();
        backend.windowCalls.length = 0;

        feed.emit(notice({ version: backend.version + 3, total: 5, filtered: 5, phase: 'refreshing', loaded: 5, expected: 9 }));
        await flush();
        expect(backend.windowCalls).toHaveLength(0);
        expect(client.getSnapshot().state!.phase).toBe('refreshing');
        expect(client.getSnapshot().state!.expected).toBe(9);

        backend.touch('r1');
        feed.emit(notice({ version: backend.version, total: 5, filtered: 5, windowChanged: true }));
        await flush();
        expect(backend.windowCalls).toHaveLength(1);
    });

    it('ignores notices for other views and notices older than the applied window', async () => {
        backend.add(3);
        make();
        await client.open(baseSpec());
        await flush();
        const filtered = client.getSnapshot().filtered;
        backend.windowCalls.length = 0;
        feed.emit(notice({ v: 'other', version: 99, total: 9, filtered: 9, windowChanged: true }));
        feed.emit(notice({ version: 0, total: 9, filtered: 9, windowChanged: true }));
        await flush();
        expect(backend.windowCalls).toHaveLength(0);
        expect(client.getSnapshot().filtered).toBe(filtered);
    });

    it('refetches a window that was short when the list grows', async () => {
        backend.add(2);
        make();
        await client.open(baseSpec());
        await flush();
        backend.windowCalls.length = 0;
        backend.add(10);
        feed.emit(notice({ version: backend.version, total: 12, filtered: 12 }));
        await flush();
        expect(backend.windowCalls).toHaveLength(1);
        expect(client.rowAt(11)).toBeDefined();
    });

    it('a context close ends the client; an expiry re-opens it', async () => {
        backend.add(2);
        make();
        await client.open(baseSpec());
        await flush();
        feed.emit(notice({ closed: 'expired' }));
        await flush();
        expect(backend.openCalls).toHaveLength(2);
        feed.emit(notice({ closed: 'replaced' }));
        await flush();
        expect(backend.openCalls).toHaveLength(2);
        feed.emit(notice({ closed: 'context' }));
        expect(client.getSnapshot().status).toBe('closed');
        expect(client.getSnapshot().closedReason).toBe('context');
        expect(feed.subscribers).toBe(0);
    });

    it('forwards pod transitions to listeners, also from a notice older than the applied version', async () => {
        backend.add(2);
        make();
        await client.open(baseSpec({ notifyOn: ['pods'] }));
        await flush();
        const seen: any[][] = [];
        const off = client.onTransitions(e => seen.push(e));
        const events = [{ kind: 'restart' as const, uid: 'u', namespace: 'd', name: 'p', total: 2, containers: [{ name: 'c', count: 2 }] }];
        feed.emit(notice({ version: 0, transitions: events }));
        expect(seen).toEqual([events]);
        feed.emit(notice({ version: backend.version }));
        expect(seen).toHaveLength(1);
        off();
        feed.emit(notice({ version: backend.version, transitions: events }));
        expect(seen).toHaveLength(1);
        expect(client.spec()?.notifyOn).toEqual(['pods']);
    });

    it('forwards selectionStale to listeners', async () => {
        backend.add(2);
        make();
        await client.open(baseSpec());
        const seen: string[][] = [];
        client.onSelectionStale(u => seen.push(u));
        feed.emit(notice({ version: backend.version, selectionStale: ['r1'] }));
        expect(seen).toEqual([['r1']]);
    });
});

describe('recovery', () => {
    it('re-opens from the retained spec on 410 and requests the range again', async () => {
        backend.add(5);
        make();
        await client.open(baseSpec({ search: 'x' }));
        await flush();
        client.update({ sort: { key: 'age', dir: 'desc' } });
        await flush();
        backend.exists = false; // server restarted: the view is gone
        backend.windowCalls.length = 0;
        feed.emit(notice({ version: backend.version + 1, total: 5, filtered: 5, windowChanged: true }));
        await flush();
        expect(backend.openCalls).toHaveLength(2);
        expect(backend.openCalls[1]).toMatchObject({ search: 'x', sort: { key: 'age', dir: 'desc' } });
        expect(last(backend.windowCalls)).toMatchObject({ have: 0 });
        expect(client.getSnapshot().status).toBe('open');
        expect(client.rowAt(0)).toBeDefined();
    });

    it('refetches the window when the event channel reconnects', async () => {
        backend.add(3);
        make();
        await client.open(baseSpec());
        await flush();
        backend.windowCalls.length = 0;
        backend.add(1);
        feed.reconnect();
        await flush();
        expect(backend.windowCalls).toHaveLength(1);
        expect(backend.windowCalls[0].have).toBe(0);
        expect(client.rowAt(3)).toBeDefined();
    });

    it('re-opens after repeated window failures', async () => {
        backend.add(3);
        make();
        await client.open(baseSpec());
        await flush();
        backend.windowError = new Error('network down');
        for (let i = 0; i < 3; i++) {
            client.refresh();
            await flush();
        }
        backend.windowError = null;
        await flush();
        expect(backend.openCalls.length).toBeGreaterThanOrEqual(2);
    });

    it('the 30 second safety check re-reads the window', async () => {
        backend.add(3);
        make();
        await client.open(baseSpec());
        await flush();
        backend.windowCalls.length = 0;
        backend.add(1);
        await vi.advanceTimersByTimeAsync(30_000);
        await flush();
        expect(backend.windowCalls.length).toBeGreaterThanOrEqual(1);
        expect(client.rowAt(3)).toBeDefined();
    });

    it('pings the server while idle so the lease stays warm', async () => {
        backend.add(1);
        make();
        await client.open(baseSpec());
        await flush();
        backend.activeCalls.length = 0;
        await vi.advanceTimersByTimeAsync(20_000);
        expect(backend.activeCalls.length).toBeGreaterThanOrEqual(1);
    });
});

describe('updates', () => {
    it('sends only changed keys, retains them in the spec, and re-reads in full', async () => {
        backend.add(5);
        make();
        await client.open(baseSpec());
        await flush();
        client.update({ search: 'a', sort: { key: 'name', dir: 'asc' } });
        await flush();
        expect(backend.updateCalls).toHaveLength(1);
        expect(backend.updateCalls[0].patch).toEqual({ search: 'a' });
        expect(client.spec()!.search).toBe('a');
        expect(last(backend.windowCalls)!.have).toBe(0);
        client.update({ search: 'a' });
        await flush();
        expect(backend.updateCalls).toHaveLength(1);
    });

    it('discards a window that was in flight when a patch was sent', async () => {
        backend.add(5);
        make();
        await client.open(baseSpec());
        await flush();
        backend.hold();
        client.refresh();
        await flush();
        const before = client.getSnapshot().epoch;
        client.update({ search: 'zzz' });
        backend.items = [{ uid: 'only', ns: 'ns', name: 'only', seq: ++backend.version }];
        backend.releaseHold();
        await flush();
        // The stale response (5 old rows) must not have been applied over the patched view.
        expect(client.getSnapshot().filtered).toBe(1);
        expect(client.rowAt(0)?.metadata.name).toBe('only');
        expect(client.getSnapshot().epoch).toBeGreaterThan(before);
    });

    it('a column change drops the cached cells and decodes with the new order', async () => {
        backend.add(3);
        make();
        await client.open(baseSpec());
        await flush();
        client.update({ columns: ['age', 'name'] });
        await flush();
        expect(client.rowAt(0)?.cells).toEqual({ age: 'age-r0', name: 'r0' });
    });

    it('applies a patch made while the view was still opening', async () => {
        backend.add(3);
        make();
        const opening = client.open(baseSpec());
        client.update({ search: 'late' });
        await opening;
        await flush();
        expect(backend.updateCalls.map(c => c.patch.search)).toContain('late');
    });

    it('a patch for a vanished view re-opens with the patch included', async () => {
        backend.add(3);
        make();
        await client.open(baseSpec());
        await flush();
        backend.exists = false;
        client.update({ search: 'q' });
        await flush();
        expect(last(backend.openCalls)!.search).toBe('q');
    });
});

describe('selection and bulk helpers', () => {
    it('debounces selection updates and reports stale uids from the ack', async () => {
        backend.add(3);
        make({ selectionDebounceMs: 100 });
        await client.open(baseSpec());
        await flush();
        const stale: string[][] = [];
        client.onSelectionStale(u => stale.push(u));
        backend.selectionStale = ['r9'];
        client.setSelection(['r0']);
        client.setSelection(['r0', 'r1', 'r9']);
        await vi.advanceTimersByTimeAsync(99);
        expect(backend.selectionCalls).toHaveLength(0);
        await vi.advanceTimersByTimeAsync(5);
        await flush();
        expect(backend.selectionCalls).toEqual([['r0', 'r1', 'r9']]);
        expect(stale).toEqual([['r9']]);
    });

    it('refs() and selectAllRefs() use the refs stream', async () => {
        backend.add(10);
        make();
        await client.open(baseSpec());
        await flush();
        const range = await client.refs(2, 4);
        expect(range.map(r => r.uid)).toEqual(['r2', 'r3', 'r4']);
        expect(last(backend.refsCalls)).toMatchObject({ offset: 2, limit: 3 });
        const all = await client.selectAllRefs();
        expect(all).toHaveLength(10);
        expect(last(backend.refsCalls)).toEqual({ offset: 0, limit: 0 });
    });

    it('selectAllRefs recovers once from a vanished view', async () => {
        backend.add(4);
        make();
        await client.open(baseSpec());
        await flush();
        const original = backend.refs.bind(backend);
        let calls = 0;
        backend.refs = async (id, o, l) => {
            if (calls++ === 0) throw new ViewGoneError();
            return original(id, o, l);
        };
        const all = await client.selectAllRefs();
        expect(all).toHaveLength(4);
        expect(backend.openCalls).toHaveLength(2);
    });

    it('facet() and getObject() go through the transport; objects are addressed by type', async () => {
        backend.add(2);
        make();
        await client.open(baseSpec({ type: 'events' }));
        await flush();
        expect((await client.facet('type')).values).toEqual(['a']);
        const spy = vi.spyOn(backend, 'getObject');
        const obj = await client.getObject(client.rowAt(0)!);
        expect(obj.spec).toEqual({ full: true });
        expect(spy).toHaveBeenCalledWith({ type: 'events', namespace: 'ns', name: 'r0', uid: 'r0' });
        const many = await client.getObjects([{ uid: 'a', ns: 'n', name: 'a' }, { metadata: { uid: 'b', name: 'b', namespace: 'n' } }]);
        expect(many).toHaveLength(2);
    });
});

describe('lifecycle', () => {
    it('close() closes the server view, releases feeds and stops fetching', async () => {
        backend.add(3);
        make();
        await client.open(baseSpec());
        await flush();
        expect(feed.subscribers).toBeGreaterThan(0);
        backend.windowCalls.length = 0;
        client.close();
        expect(backend.closeCalls).toEqual(['v1']);
        expect(feed.subscribers).toBe(0);
        feed.emit(notice({ version: 99, windowChanged: true }));
        await vi.advanceTimersByTimeAsync(60_000);
        expect(backend.windowCalls).toHaveLength(0);
        expect(client.getSnapshot().status).toBe('closed');
    });

    it('closing while the open is in flight still closes the server view', async () => {
        make();
        const opening = client.open(baseSpec());
        client.close();
        await opening;
        await flush();
        expect(backend.closeCalls).toContain('v1');
    });

    it('setActive(false) tells the server and setActive(true) refetches', async () => {
        backend.add(3);
        make();
        await client.open(baseSpec());
        await flush();
        backend.windowCalls.length = 0;
        client.setActive(false);
        expect(last(backend.activeCalls)).toBe(false);
        client.setActive(true);
        await flush();
        expect(last(backend.activeCalls)).toBe(true);
        expect(backend.windowCalls).toHaveLength(1);
    });

    it('subscribers see a new snapshot object for every change', async () => {
        backend.add(3);
        make();
        const seen: unknown[] = [];
        client.subscribe(() => seen.push(client.getSnapshot()));
        await client.open(baseSpec());
        await flush();
        expect(seen.length).toBeGreaterThan(1);
        expect(new Set(seen).size).toBe(seen.length);
    });
});

describe('hidden pages', () => {
    it('keeps the lease warm instead of fetching when isHidden says so', async () => {
        backend.add(3);
        let hidden = true;
        make({ isHidden: () => hidden, safetyCheckMs: 1000 });
        await client.open(baseSpec());
        await flush();
        const windows = backend.windowCalls.length;
        const actives = backend.activeCalls.length;
        await vi.advanceTimersByTimeAsync(1100);
        expect(backend.windowCalls.length).toBe(windows);
        expect(backend.activeCalls.length).toBeGreaterThan(actives);
        hidden = false;
        await vi.advanceTimersByTimeAsync(1100);
        expect(backend.windowCalls.length).toBeGreaterThan(windows);
    });
});

describe('request deadlines', () => {
    it('aborts a window that never answers, counts it and carries on with the next fetch', async () => {
        backend.add(5);
        backend.hangWindows = 1;
        make({ requestTimeoutMs: 1000 });
        await client.open(baseSpec());
        await flush();
        expect(backend.windowCalls).toHaveLength(1);
        expect(client.rowAt(0)).toBeUndefined();
        await vi.advanceTimersByTimeAsync(1000);
        await flush();
        expect(backend.windowSignals[0]?.aborted).toBe(true);
        expect(client.getSnapshot().error).toMatch(/timed out/);
        // The view is not wedged: a notice fetches again.
        feed.emit(notice({ version: backend.version, total: 5, filtered: 5, windowChanged: true }));
        await flush();
        expect(backend.windowCalls).toHaveLength(2);
        expect(client.rowAt(0)?.metadata.name).toBe('r0');
        expect(client.getSnapshot().error).toBeNull();
    });

    it('reopens the view after three window timeouts in a row', async () => {
        backend.add(3);
        backend.hangWindows = 3;
        make({ requestTimeoutMs: 500, recoveryBaseMs: 10 });
        await client.open(baseSpec());
        await flush();
        for (let i = 0; i < 3; i++) {
            await vi.advanceTimersByTimeAsync(500);
            await flush();
            if (i < 2) {
                feed.emit(notice({ version: backend.version, total: 3, filtered: 3, windowChanged: true }));
                await flush();
            }
        }
        await vi.advanceTimersByTimeAsync(200);
        await flush();
        expect(backend.openCalls.length).toBeGreaterThanOrEqual(2);
    });

    it('aborts the in-flight window when a patch supersedes it', async () => {
        backend.add(5);
        backend.hangWindows = 1;
        make({ requestTimeoutMs: 60_000 });
        await client.open(baseSpec());
        await flush();
        client.update({ search: 'x' });
        await flush();
        expect(backend.windowSignals[0]?.aborted).toBe(true);
        expect(backend.windowCalls.length).toBeGreaterThanOrEqual(2);
    });

    it('aborts in-flight requests on close', async () => {
        backend.add(5);
        backend.hangWindows = 1;
        make({ requestTimeoutMs: 60_000 });
        await client.open(baseSpec());
        await flush();
        client.close();
        await flush();
        expect(backend.windowSignals[0]?.aborted).toBe(true);
    });
});

describe('ping', () => {
    it('reopens a view the server dropped even when the tab is hidden', async () => {
        backend.add(3);
        make({ pingMs: 1000, safetyCheckMs: 60_000, isHidden: () => true });
        await client.open(baseSpec());
        await flush();
        expect(backend.openCalls).toHaveLength(1);
        backend.activeError = new ViewGoneError();
        await vi.advanceTimersByTimeAsync(1000);
        backend.activeError = null;
        await vi.advanceTimersByTimeAsync(1000);
        await flush();
        expect(backend.openCalls.length).toBeGreaterThanOrEqual(2);
    });
});

describe('range selection', () => {
    it('sends the version the indexes were read at and refreshes the window when the list moved on', async () => {
        backend.add(10);
        make();
        await client.open(baseSpec());
        await flush();
        const version = client.getSnapshot().state!.version;
        await client.refs(2, 4);
        expect(last(backend.refsCalls)).toMatchObject({ offset: 2, limit: 3, version });

        const windows = backend.windowCalls.length;
        backend.staleRefs = true;
        await expect(client.refs(2, 4)).rejects.toThrow(/view changed/);
        await flush();
        expect(backend.windowCalls.length).toBeGreaterThan(windows);
    });

    it('select-all does not pin a version', async () => {
        backend.add(4);
        make();
        await client.open(baseSpec());
        await flush();
        await client.selectAllRefs();
        expect(last(backend.refsCalls).version).toBeUndefined();
    });
});
