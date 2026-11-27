import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

// useResourceView is driven by a tiny synchronous React runner (as in useMetrics.hiddenGate.test.ts).
// The view client runs for real against a scripted backend, so context switches, visibility and
// polling are tested end to end up to the transport.
const runner = vi.hoisted(() => ({ slots: [] as any[], index: 0, effects: [] as Array<() => void>, dirty: false }));
const mocks = vi.hoisted(() => ({
    k8s: {} as any,
    ui: { pendingSearch: null as string | null },
    degraded: vi.fn(),
    makeHandle: null as null | (() => any),
}));

vi.mock('react', () => {
    const memo = (factory: () => any, deps: any[]) => {
        const index = runner.index++;
        const old = runner.slots[index];
        if (!old || deps.some((dep, i) => !Object.is(dep, old.deps[i]))) runner.slots[index] = { value: factory(), deps };
        return runner.slots[index].value;
    };
    return {
        useMemo: memo,
        useCallback: (fn: any, deps: any[]) => memo(() => fn, deps),
        useRef: (initial: any) => memo(() => ({ current: initial }), []),
        useState: (initial: any) => {
            const index = runner.index++;
            if (!(index in runner.slots)) {
                const slot = { value: typeof initial === 'function' ? initial() : initial, set: (next: any) => {
                    const value = typeof next === 'function' ? next(slot.value) : next;
                    if (!Object.is(value, slot.value)) { slot.value = value; runner.dirty = true; }
                } };
                runner.slots[index] = slot;
            }
            return [runner.slots[index].value, runner.slots[index].set];
        },
        useEffect: (effect: () => any, deps?: any[]) => {
            const index = runner.index++;
            const old = runner.slots[index];
            if (!old || !deps || deps.some((dep, i) => !Object.is(dep, old.deps[i]))) {
                const slot = { deps, cleanup: undefined as any };
                runner.slots[index] = slot;
                runner.effects.push(() => { old?.cleanup?.(); slot.cleanup = effect(); });
            }
        },
        useSyncExternalStore: (subscribe: any, getSnapshot: any) => {
            const index = runner.index++;
            const old = runner.slots[index];
            if (!old || old.subscribe !== subscribe) {
                const slot = { subscribe, cleanup: undefined as any };
                runner.slots[index] = slot;
                runner.effects.push(() => { old?.cleanup?.(); slot.cleanup = subscribe(() => { runner.dirty = true; }); });
            }
            return getSnapshot();
        },
    };
});
vi.mock('../context', () => ({ useK8s: () => mocks.k8s, useUI: () => mocks.ui }));
// Hooks without an injected transport take their handle from the factory, which is the path that parks views.
vi.mock('../lib/views/viewClientFactory', () => ({ createViewHandle: () => mocks.makeHandle!() }));
vi.mock('../lib/wailsjs-adapter/runtime/runtime', () => ({ getClientId: () => 'client-1' }));
vi.mock('../lib/views/availability', () => ({ markKindDegraded: (...a: any[]) => mocks.degraded(...a) }));

type Handler = () => void;
function installDom(visible: boolean) {
    const handlers: Record<string, Set<Handler>> = {};
    const add = (t: string, h: Handler) => { (handlers[t] ||= new Set()).add(h); };
    const remove = (t: string, h: Handler) => { handlers[t]?.delete(h); };
    const doc: any = { visibilityState: visible ? 'visible' : 'hidden', hidden: !visible, addEventListener: add, removeEventListener: remove };
    (globalThis as any).document = doc;
    (globalThis as any).window = { addEventListener: add, removeEventListener: remove };
    return {
        set(v: boolean) {
            doc.visibilityState = v ? 'visible' : 'hidden';
            doc.hidden = !v;
            handlers['visibilitychange']?.forEach(h => h());
        },
    };
}

import { useResourceView, type UseResourceViewOptions } from './useResourceView';
import { FakeBackend, FakeFeed, flush, notice } from '../lib/views/testSupport';
import { createViewClient } from '../lib/views/viewClient';
import { clearParkedViews } from '../lib/views/viewParking';

const lastOf = <T>(a: T[]): T => a[a.length - 1];

let backend: FakeBackend;
let feed: FakeFeed;
let current: UseResourceViewOptions;
let last: ReturnType<typeof useResourceView>;
const last_ = () => last;

function render(): ReturnType<typeof useResourceView> {
    for (let i = 0; i < 25; i++) {
        runner.index = 0; runner.effects = [];
        last = useResourceView('configmaps', current);
        runner.dirty = false;
        runner.effects.forEach(effect => effect());
        if (!runner.dirty) break;
    }
    return last;
}

async function settle(): Promise<ReturnType<typeof useResourceView>> {
    await flush();
    return render();
}

function unmount(): void {
    runner.slots.forEach(slot => slot?.cleanup?.());
    runner.slots = [];
}

let dom: ReturnType<typeof installDom>;

beforeEach(() => {
    vi.useFakeTimers();
    dom = installDom(true);
    runner.slots = []; runner.index = 0; runner.effects = [];
    backend = new FakeBackend();
    feed = new FakeFeed();
    mocks.degraded.mockReset();
    mocks.ui = { pendingSearch: null };
    mocks.makeHandle = () => createViewClient({ transport: backend, notices: feed });
    mocks.k8s = {
        lastRefresh: 1, reconcileToken: 0, connectionMode: 'streaming', registerPolling: vi.fn(() => vi.fn()),
        checkConnectionError: vi.fn(),
    };
    current = { enabled: true, context: 'ctx-a', namespaces: ['b', 'a'], transport: backend, notices: feed };
});

afterEach(() => {
    unmount();
    clearParkedViews();
    vi.useRealTimers();
    delete (globalThis as any).document;
    delete (globalThis as any).window;
});

describe('opening and closing', () => {
    it('opens a view with the seeded query and the namespace selection', async () => {
        backend.add(3);
        current = { ...current, sort: { key: 'age', direction: 'desc' }, hide: ['system'], columns: ['name', 'age'] };
        render();
        const res = await settle();
        expect(backend.openCalls).toHaveLength(1);
        expect(backend.openCalls[0]).toMatchObject({
            type: 'configmaps', namespaces: ['a', 'b'], clientId: 'client-1', hide: ['system'],
            sort: { key: 'age', dir: 'desc' }, columns: ['name', 'age'],
        });
        expect(backend.openCalls[0].viewId).toMatch(/^v-/);
        expect(backend.openCalls[0].clientToken).toBeTruthy();
        expect(res.view).not.toBeNull();
        expect(res.filtered).toBe(3);
        expect(res.loadState.phase).toBe('complete');
        expect(res.loading).toBe(false);
    });

    it('treats an empty selection as no namespaces and any wildcard as all', async () => {
        current = { ...current, namespaces: [] };
        render();
        await settle();
        expect(backend.openCalls[0].namespaces).toEqual([]);
        unmount();
        backend.openCalls.length = 0;
        current = { ...current, namespaces: ['a', '*'] };
        render();
        await settle();
        expect(backend.openCalls[0].namespaces).toEqual(['*']);
    });

    it('does not open while disabled and closes the view when it becomes disabled', async () => {
        current = { ...current, enabled: false };
        let res = render();
        await settle();
        expect(backend.openCalls).toHaveLength(0);
        expect(res.view).toBeNull();

        current = { ...current, enabled: true };
        render();
        res = await settle();
        expect(res.view).not.toBeNull();

        const id = backend.openCalls[0].viewId;
        current = { ...current, enabled: false };
        res = render();
        await flush();
        expect(backend.closeCalls).toEqual([id]);
        expect(res.view).toBeNull();
    });

    it('does not open without a context', async () => {
        current = { ...current, context: null };
        render();
        await settle();
        expect(backend.openCalls).toHaveLength(0);
    });

    it('a context switch closes the old view and opens a new one', async () => {
        backend.add(2);
        render();
        await settle();
        const first = backend.openCalls[0].viewId;
        current = { ...current, context: 'ctx-b' };
        const res = render();
        await settle();
        expect(backend.closeCalls).toEqual([first]);
        expect(backend.openCalls).toHaveLength(2);
        expect(backend.openCalls[1].viewId).not.toBe(first);
        expect(res.view?.viewId).toBe(backend.openCalls[1].viewId);
    });

    it('unmounting closes the view', async () => {
        render();
        await settle();
        unmount();
        await flush();
        expect(backend.closeCalls).toHaveLength(1);
        expect(feed.subscribers).toBe(0);
    });
});

describe('scope changes', () => {
    it('namespace and hide changes patch the open view instead of re-opening it', async () => {
        backend.add(2);
        render();
        await settle();
        current = { ...current, namespaces: ['a'], hide: ['helm'] };
        render();
        await settle();
        expect(backend.openCalls).toHaveLength(1);
        expect(backend.updateCalls.map(c => c.patch)).toEqual([{ namespaces: ['a'], hide: ['helm'] }]);
    });

    it('equal selections in a new array change nothing', async () => {
        render();
        await settle();
        current = { ...current, namespaces: ['a', 'b'] };
        render();
        await settle();
        expect(backend.updateCalls).toHaveLength(0);
    });
});

describe('refresh and polling', () => {
    it('an explicit refresh or reconcile re-reads the window without re-opening', async () => {
        backend.add(2);
        render();
        await settle();
        backend.windowCalls.length = 0;
        mocks.k8s = { ...mocks.k8s, lastRefresh: 2 };
        render();
        await settle();
        expect(backend.windowCalls).toHaveLength(1);
        mocks.k8s = { ...mocks.k8s, reconcileToken: 1 };
        render();
        await settle();
        expect(backend.windowCalls).toHaveLength(2);
        expect(backend.openCalls).toHaveLength(1);
    });

    it('registers a poll callback in polling mode and removes it with the view', async () => {
        const unregister = vi.fn();
        let poll: (() => Promise<void>) | undefined;
        mocks.k8s = { ...mocks.k8s, connectionMode: 'polling', registerPolling: vi.fn((fn: any) => { poll = fn; return unregister; }) };
        backend.add(2);
        render();
        await settle();
        expect(mocks.k8s.registerPolling).toHaveBeenCalledTimes(1);
        backend.windowCalls.length = 0;
        await poll!();
        await flush();
        expect(backend.windowCalls).toHaveLength(1);
        unmount();
        expect(unregister).toHaveBeenCalled();
    });

    it('does not register polling while streaming', async () => {
        render();
        await settle();
        expect(mocks.k8s.registerPolling).not.toHaveBeenCalled();
    });

    it('retry re-opens the view from the retained spec', async () => {
        const res = render();
        await settle();
        res.refetch();
        await flush();
        expect(backend.openCalls).toHaveLength(2);
        expect(backend.openCalls[1].viewId).toBe(backend.openCalls[0].viewId);
    });
});

describe('backend context change', () => {
    it('re-opens a view the backend closed for a context change while the UI context is unchanged', async () => {
        backend.add(2);
        render();
        await settle();
        const id = backend.openCalls[0].viewId;
        feed.emit(notice({ v: id, closed: 'context' }));
        render();
        expect(last_().snapshot?.status).toBe('closed');
        await vi.advanceTimersByTimeAsync(400);
        await flush();
        render();
        expect(backend.openCalls).toHaveLength(2);
        expect(last_().snapshot?.status).toBe('open');
    });
});

describe('visibility', () => {
    it('tells the backend when the window is hidden and shown', async () => {
        render();
        await settle();
        backend.activeCalls.length = 0;
        dom.set(false);
        render();
        await flush();
        expect(lastOf(backend.activeCalls)).toBe(false);
        dom.set(true);
        render();
        await flush();
        expect(lastOf(backend.activeCalls)).toBe(true);
    });
});

describe('load state', () => {
    it('is loading before the first answer and carries progress while the list fills', async () => {
        backend.phase = 'loading';
        backend.add(4);
        const original = backend.state.bind(backend);
        backend.state = () => ({ ...original(), phase: 'loading', loaded: 4, expected: 10 });
        render();
        const res = await settle();
        expect(res.loadState.phase).toBe('loading');
        expect(res.loadingProgress).toEqual({ loaded: 4, total: 10 });
        expect(res.loading).toBe(true);
    });

    it('moves to complete through a notice', async () => {
        backend.phase = 'loading';
        backend.add(2);
        render();
        await settle();
        feed.emit(notice({ v: backend.openCalls[0].viewId, version: backend.version + 1, total: 2, filtered: 2, phase: 'complete', loaded: 2, expected: 2 }));
        const res = render();
        expect(res.loadState.phase).toBe('complete');
        expect(res.loadingProgress).toBeNull();
    });

    it('reports a failed open as incomplete with an error and marks the kind legacy once it gives up', async () => {
        backend.failOpen = 99;
        render();
        let res = await settle();
        expect(res.loadState.phase).toBe('incomplete');
        expect(res.error?.message).toBe('open failed');
        expect(mocks.k8s.checkConnectionError).toHaveBeenCalled();
        await vi.advanceTimersByTimeAsync(60_000);
        res = render();
        expect(res.snapshot?.permanent).toBe(true);
        expect(mocks.degraded).toHaveBeenCalledWith('configmaps');
    });

    it('surfaces a backend error from the view state', async () => {
        const original = backend.state.bind(backend);
        backend.state = () => ({ ...original(), phase: 'incomplete', error: 'forbidden: nope' });
        render();
        const res = await settle();
        expect(res.loadState.phase).toBe('incomplete');
        expect(res.error?.message).toBe('forbidden: nope');
    });
});

describe('reusing the view of a list that was just left', () => {
    // No injected transport: the handle comes from the factory and is parked on unmount.
    const viaFactory = () => { current = { enabled: true, context: 'ctx-a', namespaces: ['a'] }; };

    it('reuses the parked view when the list comes back with the same query', async () => {
        viaFactory();
        backend.add(3);
        render();
        await settle();
        expect(backend.openCalls).toHaveLength(1);
        unmount();
        expect(backend.closeCalls).toHaveLength(0);

        // The first render already has the view and its rows, with no second open.
        const first = render();
        expect(first.view).not.toBeNull();
        expect(first.filtered).toBe(3);
        await settle();
        expect(backend.openCalls).toHaveLength(1);
    });

    it('opens a fresh view when the parked one holds another search', async () => {
        viaFactory();
        backend.add(3);
        render();
        const opened = await settle();
        opened.view!.update({ search: 'web-' });
        await settle();
        unmount();

        // Rows of the old search must not be shown as the new list's rows.
        const first = render();
        expect(first.view === null || first.filtered === 0).toBe(true);
        await settle();
        expect(backend.openCalls).toHaveLength(2);
        expect(backend.closeCalls).toHaveLength(1);
    });

    it('opens a fresh view when the list is opened by a link that carries a search', async () => {
        viaFactory();
        backend.add(3);
        render();
        await settle();
        unmount();

        mocks.ui = { pendingSearch: 'name:"r1"' };
        const first = render();
        expect(first.filtered).toBe(0);
        await settle();
        expect(backend.openCalls).toHaveLength(2);
        expect(backend.closeCalls).toHaveLength(1);
    });
});
