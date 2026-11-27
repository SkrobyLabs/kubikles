import { beforeEach, describe, expect, it, vi } from 'vitest';

const m = vi.hoisted(() => ({
    usable: true,
    fetchJson: vi.fn(),
    fetchNdjson: vi.fn(),
    noted: [] as Array<[boolean, string]>,
    clientId: '',
    handlers: new Map<string, Set<(d: any) => void>>(),
    api: {
        OpenView: vi.fn(), UpdateView: vi.fn(), CloseView: vi.fn(), SetViewActive: vi.fn(), GetViewWindow: vi.fn(),
        GetViewRefs: vi.fn(), GetViewFacet: vi.fn(), SetViewSelection: vi.fn(), GetResourceObject: vi.fn(),
        GetResourceObjects: vi.fn(), GetViewsInfo: vi.fn(),
    },
}));

vi.mock('wailsjs/go/main/App', () => m.api);
vi.mock('wailsjs/runtime/runtime', () => ({
    getClientId: () => m.clientId,
    EventsOn: (name: string, cb: (d: any) => void) => {
        if (!m.handlers.has(name)) m.handlers.set(name, new Set());
        m.handlers.get(name)!.add(cb);
        return () => m.handlers.get(name)!.delete(cb);
    },
}));
vi.mock('../bulk/transport', () => {
    class FakeUnavailable extends Error {
        status?: number;
        constructor(message: string, status?: number) { super(message); this.status = status; }
    }
    return {
        BulkUnavailableError: FakeUnavailable,
        bulkFetchUsable: async () => m.usable,
        bulkFetchJson: (...a: any[]) => m.fetchJson(...a),
        bulkFetchNdjson: (...a: any[]) => m.fetchNdjson(...a),
        noteBulkResult: (ok: boolean, msg = '') => { m.noted.push([ok, msg]); },
    };
});

import { BulkUnavailableError } from '../bulk/transport';
import { createDefaultTransport, getNoticeFeed } from './transport';
import { ViewGoneError, ViewStaleError } from './protocol';
import fixtures from './__fixtures__/protocol.json';

const win = fixtures.window as any;

beforeEach(() => {
    m.usable = true;
    m.noted.length = 0;
    m.clientId = '';
    m.handlers.clear();
    for (const fn of [m.fetchJson, m.fetchNdjson, ...Object.values(m.api)]) fn.mockReset();
});

describe('window', () => {
    it('is fetched over the bulk route when it is usable', async () => {
        m.fetchJson.mockResolvedValue(win);
        const w = await createDefaultTransport().window('v1', 5, 100, 7);
        expect(w).toBe(win);
        expect(m.fetchJson).toHaveBeenCalledWith('views/v1/window?offset=5&limit=100&have=7', undefined);
        expect(m.api.GetViewWindow).not.toHaveBeenCalled();
        expect(m.noted).toEqual([[true, '']]);
    });

    it('falls back to the binding when the bulk route fails, and says so', async () => {
        m.fetchJson.mockRejectedValue(new BulkUnavailableError('fetch failed: boom', undefined));
        m.api.GetViewWindow.mockResolvedValue(win);
        const w = await createDefaultTransport().window('v1', 0, 50, 0);
        expect(w).toBe(win);
        expect(m.api.GetViewWindow).toHaveBeenCalledWith('v1', 0, 50, 0);
        expect(m.noted[0][0]).toBe(false);
    });

    it('falls back when the bulk body is not a window', async () => {
        m.fetchJson.mockResolvedValue({ nope: true });
        m.api.GetViewWindow.mockResolvedValue(win);
        expect(await createDefaultTransport().window('v1', 0, 1, 0)).toBe(win);
    });

    it('uses the binding directly when the bulk transport is not usable', async () => {
        m.usable = false;
        m.api.GetViewWindow.mockResolvedValue(win);
        await createDefaultTransport().window('v1', 0, 1, 0);
        expect(m.fetchJson).not.toHaveBeenCalled();
    });

    it('turns a 410 into a view-gone error without trying the binding', async () => {
        m.fetchJson.mockRejectedValue(new BulkUnavailableError('view not found', 410));
        await expect(createDefaultTransport().window('v1', 0, 1, 0)).rejects.toBeInstanceOf(ViewGoneError);
        expect(m.api.GetViewWindow).not.toHaveBeenCalled();
    });

    it('turns both binding error shapes for a missing view into view-gone', async () => {
        m.usable = false;
        m.api.GetViewWindow.mockRejectedValueOnce('view not found');
        await expect(createDefaultTransport().window('v1', 0, 1, 0)).rejects.toBeInstanceOf(ViewGoneError);
        m.api.GetViewWindow.mockRejectedValueOnce(new Error('view not found'));
        await expect(createDefaultTransport().window('v1', 0, 1, 0)).rejects.toBeInstanceOf(ViewGoneError);
        m.api.GetViewWindow.mockRejectedValueOnce(new Error('disk on fire'));
        await expect(createDefaultTransport().window('v1', 0, 1, 0)).rejects.toThrow('disk on fire');
    });
});

describe('refs', () => {
    async function* stream(lines: unknown[]) { for (const l of lines) yield l; }

    it('reads the NDJSON stream and skips its header line', async () => {
        m.fetchNdjson.mockReturnValue(stream([{ version: 1, total: 2, filtered: 2 }, ['u1', 'n', 'a'], ['u2', 'n', 'b']]));
        const refs = await createDefaultTransport().refs('v1', 0, 0);
        expect(refs).toEqual([{ uid: 'u1', ns: 'n', name: 'a' }, { uid: 'u2', ns: 'n', name: 'b' }]);
        expect(m.fetchNdjson).toHaveBeenCalledWith('views/v1/refs?offset=0&limit=0', undefined);
    });

    it('falls back to the binding when streaming fails', async () => {
        m.fetchNdjson.mockImplementation(async function* () { throw new BulkUnavailableError('stream failed'); });
        m.api.GetViewRefs.mockResolvedValue([{ uid: 'u', ns: 'n', name: 'a' }]);
        expect(await createDefaultTransport().refs('v1', 3, 4)).toEqual([{ uid: 'u', ns: 'n', name: 'a' }]);
        expect(m.api.GetViewRefs).toHaveBeenCalledWith('v1', 3, 4, 0);
    });
});

describe('objects', () => {
    const ref = { type: 'events', namespace: 'ns', name: 'e 1', uid: 'u1' };

    it('fetches one object over the bulk route with an encoded path', async () => {
        m.fetchJson.mockResolvedValue({ kind: 'Event' });
        await createDefaultTransport().getObject(ref);
        expect(m.fetchJson).toHaveBeenCalledWith('objects/events?ns=ns&name=e+1&uid=u1');
    });

    it('falls back to the binding for an object', async () => {
        m.usable = false;
        m.api.GetResourceObject.mockResolvedValue({ kind: 'Event' });
        expect(await createDefaultTransport().getObject(ref)).toEqual({ kind: 'Event' });
    });

    it('fetches many objects in order, in one binding call when bulk is unusable', async () => {
        m.usable = false;
        m.api.GetResourceObjects.mockResolvedValue([{ a: 1 }, { a: 2 }]);
        const out = await createDefaultTransport().getObjects([ref, { ...ref, name: 'b' }]);
        expect(out).toEqual([{ a: 1 }, { a: 2 }]);
        expect(m.api.GetResourceObjects).toHaveBeenCalledTimes(1);
    });

    it('fetches many objects concurrently and keeps their order over bulk', async () => {
        m.fetchJson.mockImplementation(async (path: string) => ({ path }));
        const refs = Array.from({ length: 20 }, (_, i) => ({ ...ref, name: `n${i}` }));
        const out = await createDefaultTransport().getObjects(refs);
        expect(out.map((o: any) => o.path.match(/name=(n\d+)/)![1])).toEqual(refs.map(r => r.name));
    });
});

describe('control calls', () => {
    it('go over the bindings and map a vanished view to view-gone', async () => {
        m.api.UpdateView.mockRejectedValue('view not found');
        await expect(createDefaultTransport().update('v1', { search: 'x' })).rejects.toBeInstanceOf(ViewGoneError);
        m.api.SetViewSelection.mockResolvedValue({ selected: 1, stale: [] });
        expect(await createDefaultTransport().setSelection('v1', ['a'])).toEqual({ selected: 1, stale: [] });
    });
});

describe('notice feed', () => {
    it('keeps one subscription per event for all listeners and detaches when none remain', () => {
        const feed = getNoticeFeed();
        const a = vi.fn();
        const b = vi.fn();
        const offA = feed.subscribe(a);
        const offB = feed.subscribe(b);
        expect(m.handlers.get('view-updates')!.size).toBe(1);
        const notice = { v: 'v1', version: 1 };
        for (const cb of m.handlers.get('view-updates')!) cb([notice]);
        expect(a).toHaveBeenCalledWith([notice]);
        expect(b).toHaveBeenCalledWith([notice]);
        offA();
        expect(m.handlers.get('view-updates')!.size).toBe(1);
        offB();
        expect(m.handlers.get('view-updates')!.size).toBe(0);
        expect(m.handlers.get('ws:reconnected')!.size).toBe(0);
    });

    it('accepts a single notice object and ignores junk payloads', () => {
        const feed = getNoticeFeed();
        const cb = vi.fn();
        const off = feed.subscribe(cb);
        const handler = [...m.handlers.get('view-updates')!][0];
        handler({ v: 'x', version: 1 });
        handler(null);
        handler('nope');
        handler([]);
        expect(cb).toHaveBeenCalledTimes(1);
        off();
    });

    it('forwards reconnects', () => {
        const feed = getNoticeFeed();
        const cb = vi.fn();
        const off = feed.onReconnect(cb);
        for (const h of m.handlers.get('ws:reconnected')!) h(undefined);
        expect(cb).toHaveBeenCalledTimes(1);
        off();
    });
});

describe('range selection staleness', () => {
    async function* stream(lines: unknown[]) { for (const l of lines) yield l; }

    it('sends the expected version and maps a 409 to a stale error without using the binding', async () => {
        m.fetchNdjson.mockReturnValue(stream([{ version: 7, total: 1, filtered: 1 }, ['u1', 'n', 'a']]));
        await createDefaultTransport().refs('v1', 0, 2, undefined, 7);
        expect(m.fetchNdjson).toHaveBeenCalledWith('views/v1/refs?offset=0&limit=2&version=7', undefined);

        m.fetchNdjson.mockImplementation(async function* () { throw new BulkUnavailableError('view changed', 409); });
        await expect(createDefaultTransport().refs('v1', 0, 2, undefined, 7)).rejects.toBeInstanceOf(ViewStaleError);
        expect(m.api.GetViewRefs).not.toHaveBeenCalled();
    });

    it('maps a stale binding error too', async () => {
        m.usable = false;
        m.api.GetViewRefs.mockRejectedValue('view changed: refresh and retry');
        await expect(createDefaultTransport().refs('v1', 0, 2, undefined, 7)).rejects.toBeInstanceOf(ViewStaleError);
        expect(m.api.GetViewRefs).toHaveBeenCalledWith('v1', 0, 2, 7);
    });
});

describe('application errors are not transport failures', () => {
    const ref = { type: 'pods', namespace: 'ns', name: 'p' };

    it('a 404 or 500 from the object route falls back without counting against the transport', async () => {
        m.api.GetResourceObject.mockResolvedValue({ kind: 'Pod' });
        for (const status of [404, 500, 404]) {
            m.fetchJson.mockRejectedValueOnce(new BulkUnavailableError('nope', status));
            await createDefaultTransport().getObject(ref);
        }
        expect(m.noted.every(([ok]) => ok)).toBe(true);
    });

    it('gateway failures and network errors do count', async () => {
        m.api.GetResourceObject.mockResolvedValue({ kind: 'Pod' });
        m.fetchJson.mockRejectedValueOnce(new BulkUnavailableError('bad gateway', 502));
        m.fetchJson.mockRejectedValueOnce(new BulkUnavailableError('fetch failed'));
        await createDefaultTransport().getObject(ref);
        await createDefaultTransport().getObject(ref);
        expect(m.noted.map(([ok]) => ok)).toEqual([false, false]);
    });
});

describe('view ownership', () => {
    it('stamps the current client id on every open', async () => {
        m.api.OpenView.mockResolvedValue({ viewId: 'v1' });
        m.clientId = 'ws-9';
        await createDefaultTransport().open({ viewId: 'v1', clientId: 'ws-1' } as any);
        expect(m.api.OpenView.mock.calls[0][0].clientId).toBe('ws-9');
        m.clientId = '';
        await createDefaultTransport().open({ viewId: 'v1', clientId: 'ws-1' } as any);
        expect(m.api.OpenView.mock.calls[1][0].clientId).toBe('ws-1');
    });
});

describe('loadViewsInfo', () => {
    it('keeps the dynamic flag that turns windowed custom resource lists on', async () => {
        const { loadViewsInfo } = await import('./transport');
        m.api.GetViewsInfo.mockResolvedValueOnce({ enabled: true, types: ['pods'], dynamic: true });
        expect(await loadViewsInfo()).toEqual({ enabled: true, types: ['pods'], dynamic: true });
        m.api.GetViewsInfo.mockResolvedValueOnce({ enabled: true, types: ['pods'] });
        expect(await loadViewsInfo()).toEqual({ enabled: true, types: ['pods'], dynamic: false });
    });
});
