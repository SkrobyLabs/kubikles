import { beforeEach, describe, expect, it, vi } from 'vitest';

// A dependency-aware hook runner: execute the production hook and its real effects,
// retaining state/refs between renders and running cleanup when dependencies change.
const runner = vi.hoisted(() => ({ slots: [] as any[], index: 0, effects: [] as Array<() => void>, dirty: false }));
const mocks = vi.hoisted(() => ({
    context: {} as any,
    cancel: vi.fn().mockResolvedValue(undefined), watcher: vi.fn(), crdWatcher: vi.fn(),
    subscribe: vi.fn(async (_type, namespace) => `watch-${namespace}`), unsubscribe: vi.fn().mockResolvedValue(undefined), customList: vi.fn(),
    events: new Map<string, Set<(event: any) => void>>(),
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
        useEffect: (effect: () => any, deps: any[]) => {
            const index = runner.index++;
            const old = runner.slots[index];
            if (!old || deps.some((dep, i) => !Object.is(dep, old.deps[i]))) {
                const slot = { deps, cleanup: undefined as any };
                runner.slots[index] = slot;
                runner.effects.push(() => { old?.cleanup?.(); slot.cleanup = effect(); });
            }
        },
    };
});
vi.mock('../context', () => ({ useK8s: () => mocks.context }));
vi.mock('./useResourceWatcher', () => ({ useResourceWatcher: mocks.watcher, useCRDWatcher: mocks.crdWatcher }));
vi.mock('wailsjs/go/main/App', () => ({ CancelListRequest: mocks.cancel, ListCustomResources: mocks.customList, SubscribeResourceWatcher: mocks.subscribe, UnsubscribeWatcher: mocks.unsubscribe }));
vi.mock('wailsjs/runtime/runtime', () => ({ EventsOn: (name: string, fn: any) => {
    if (!mocks.events.has(name)) mocks.events.set(name, new Set());
    mocks.events.get(name)!.add(fn);
    return () => mocks.events.get(name)!.delete(fn);
} }));
vi.mock('../utils/Logger', () => ({ default: { debug: vi.fn() } }));
import { createNamespacedResourceHook } from './useResource';
import { useCustomResources } from './useCustomResources';

const ns = (count: number) => Array.from({ length: count }, (_, i) => `ns-${i}`);
const item = (namespace: string, uid = namespace) => ({ metadata: { uid, namespace } });
function setup(list: any, selection: string[] = ['*'], options: { useHook?: any; watcher?: any } = {}) {
    const hook = options.useHook || createNamespacedResourceHook('pods', list, 'pods', options.watcher);
    let selected = selection;
    let context = 'a';
    let visible = true;
    let result: any;
    const render = () => {
        runner.index = 0; runner.effects = []; runner.dirty = false;
        result = hook(context, selected, visible);
        runner.effects.forEach(effect => effect());
        return result;
    };
    const flush = async () => {
        for (let i = 0; i < 40; i++) { await Promise.resolve(); if (runner.dirty) render(); }
        return result;
    };
    render();
    return { flush, render, select: (value: string[]) => { selected = value; return render(); },
        context: (value: string) => { context = value; mocks.context.currentContext = value; return render(); },
        hide: () => { visible = false; return render(); },
        event: (event: any) => mocks.watcher.mock.lastCall![2](event),
    };
}
beforeEach(() => {
    runner.slots.forEach(slot => slot.cleanup?.());
    runner.slots = []; runner.index = 0; runner.effects = []; runner.dirty = false;
    vi.clearAllMocks(); mocks.events.clear();
    mocks.context = { currentContext: 'a', namespaces: ns(100), lastRefresh: 1, reconcileToken: 0, connectionMode: 'streaming',
        registerPolling: vi.fn(() => vi.fn()), checkConnectionError: vi.fn() };
});

describe('namespaced resource fetching and filtering', () => {
    it('uses unique IDs for ten namespace requests and returns all results', async () => {
        const list = vi.fn(async (_id, namespace) => [item(namespace)]);
        const view = setup(list, ns(10));
        const result = await view.flush();
        expect(list).toHaveBeenCalledTimes(10);
        expect(new Set(list.mock.calls.map(call => call[0])).size).toBe(10);
        expect(result.pods).toHaveLength(10);
        expect(result.error).toBeNull();
    });
    it('reuses the broad list and watcher when narrowing, and keeps future namespaces out of explicit selections', async () => {
        mocks.context.namespaces = ns(1000);
        const list = vi.fn(async () => [item('ns-0'), item('ns-1'), item('ns-2')]);
        const view = setup(list);
        await view.flush();
        expect(view.select(['ns-0']).pods).toEqual([item('ns-0')]);
        await view.flush();
        expect(list).toHaveBeenCalledTimes(1);
        expect(mocks.watcher.mock.lastCall![1]).toEqual(['']);
        view.event({ type: 'ADDED', context: 'a', resource: item('future') });
        expect((await view.flush()).pods).toEqual([item('ns-0')]);
        view.select(['*']);
        expect((await view.flush()).pods).toHaveLength(4);
        expect(list).toHaveBeenCalledTimes(1);
    });
    it('falls back to individual namespaces after cluster-wide list permission failure', async () => {
        mocks.context.namespaces = ns(8);
        const list = vi.fn(async (_id, namespace) => {
            if (!namespace) throw new Error('Forbidden');
            return [item(namespace)];
        });
        const view = setup(list, ns(4));
        const result = await view.flush();
        expect(list.mock.calls.map(call => call[1])).toEqual(['', ...ns(4)]);
        expect(result.pods).toHaveLength(4);
        expect(result.error).toBeNull();
        expect(mocks.watcher.mock.lastCall![1]).toEqual(ns(4));
        expect(mocks.context.checkConnectionError).not.toHaveBeenCalled();
    });
    it('falls back when listing is allowed but watching all namespaces is forbidden', async () => {
        mocks.context.namespaces = ns(8);
        const list = vi.fn(async (_id, namespace) => namespace ? [item(namespace)] : ns(8).map(n => item(n)));
        const view = setup(list, ns(4));
        await view.flush();
        mocks.events.get('watcher-error')!.forEach(fn => fn({ resourceType: 'pods', namespace: '', context: 'a', error: 'Forbidden' }));
        await view.flush();
        expect(list).toHaveBeenCalledTimes(5);
        expect(mocks.watcher.mock.lastCall![1]).toEqual(ns(4));
    });
    it('keeps prior data and exposes failures rather than applying a partial refresh', async () => {
        const list = vi.fn(async (_id, namespace) => [item(namespace)]);
        const view = setup(list, ns(2));
        await view.flush();
        list.mockImplementation(async (_id, namespace) => {
            if (namespace === 'ns-0') throw new Error('unavailable');
            return [];
        });
        mocks.context.reconcileToken++;
        view.render();
        const result = await view.flush();
        expect(result.pods).toHaveLength(2);
        expect(result.error?.message).toBe('unavailable');
    });
    it('cancels every active request and does not launch queued requests after hiding', async () => {
        const resolvers: Array<(value: any[]) => void> = [];
        const list = vi.fn(() => new Promise<any[]>(resolve => resolvers.push(resolve)));
        const view = setup(list, ns(10));
        expect(list).toHaveBeenCalledTimes(4);
        view.hide();
        expect(new Set(mocks.cancel.mock.calls.map(call => call[0]))).toEqual(new Set(list.mock.calls.map((call: any[]) => call[0])));
        resolvers.forEach(resolve => resolve([]));
        await view.flush();
        expect(list).toHaveBeenCalledTimes(4);
    });
    it('ignores old-context list responses and watcher events', async () => {
        let finish!: (value: any[]) => void;
        const list = vi.fn().mockImplementationOnce(() => new Promise(resolve => { finish = resolve; }))
            .mockResolvedValue([item('new')]);
        const view = setup(list);
        view.context('b');
        await view.flush();
        finish([item('old')]);
        view.event({ type: 'ADDED', context: 'a', resource: item('old-event') });
        expect((await view.flush()).pods).toEqual([item('new')]);
    });
    it('preserves watcher deletions received during a refresh', async () => {
        let finish!: (value: any[]) => void;
        const list = vi.fn().mockResolvedValueOnce([item('ns-0')])
            .mockImplementationOnce(() => new Promise(resolve => { finish = resolve; }));
        const view = setup(list);
        await view.flush();
        mocks.context.reconcileToken++;
        view.render();
        view.event({ type: 'DELETED', context: 'a', resource: item('ns-0') });
        finish([item('ns-0')]);
        expect((await view.flush()).pods).toEqual([]);
    });
    it('polls the broad scope while the display is filtered', async () => {
        mocks.context.connectionMode = 'polling';
        const list = vi.fn(async (_id: string, _namespace: string) => [item('ns-0'), item('ns-1')]);
        const view = setup(list);
        await view.flush();
        view.select(['ns-0']);
        await view.flush();
        const poll = mocks.context.registerPolling.mock.lastCall[0];
        await poll();
        expect((await view.flush()).pods).toEqual([item('ns-0')]);
        expect(list.mock.calls.map(call => call[1])).toEqual(['', '']);
    });
    it('makes no requests and displays nothing for an empty selection', async () => {
        const list = vi.fn(async () => [item('ns-0')]);
        const view = setup(list, []);
        expect((await view.flush()).pods).toEqual([]);
        expect(list).not.toHaveBeenCalled();
        expect(mocks.watcher.mock.lastCall![3]).toBe(false);
    });
});


describe('custom resources and watcher integration', () => {
    it('shares filtering for custom resources and isolates changing resource types', async () => {
        let resource = 'widgets';
        mocks.customList.mockResolvedValue([item('ns-0'), item('ns-1')]);
        const view = setup(null, ['*'], { useHook: (context: string, selected: string[], visible: boolean) =>
            useCustomResources(context, 'example.io', 'v1', resource, selected, visible, true) });
        await view.flush();
        expect(mocks.customList.mock.lastCall!.slice(1)).toEqual(['example.io', 'v1', 'widgets', '']);
        view.select(['ns-0']);
        expect((await view.flush()).resources).toEqual([item('ns-0')]);
        expect(mocks.customList).toHaveBeenCalledTimes(1);
        expect(mocks.crdWatcher.mock.lastCall![3]).toEqual(['']);
        resource = 'gadgets';
        expect(view.render().resources).toEqual([]);
        await view.flush();
        expect(mocks.customList.mock.lastCall!.slice(1)).toEqual(['example.io', 'v1', 'gadgets', 'ns-0']);
    });
    it('uses all scope for cluster-scoped custom resources even with no namespaces selected', async () => {
        mocks.customList.mockResolvedValue([{ metadata: { uid: 'global' } }]);
        const view = setup(null, [], { useHook: (context: string, selected: string[], visible: boolean) =>
            useCustomResources(context, 'example.io', 'v1', 'globals', selected, visible, false) });
        expect((await view.flush()).resources).toHaveLength(1);
        expect(mocks.crdWatcher.mock.lastCall![3]).toEqual(['']);
    });
    it('retains one real watcher across local filtering and unsubscribes on hide', async () => {
        const { useResourceWatcher } = await vi.importActual<typeof import('./useResourceWatcher')>('./useResourceWatcher');
        const view = setup(vi.fn(async () => [item('ns-0'), item('ns-1')]), ['*'], { watcher: useResourceWatcher });
        await view.flush();
        view.select(['ns-0']);
        await view.flush();
        expect(mocks.subscribe).toHaveBeenCalledTimes(1);
        mocks.events.get('resource-event')!.forEach(fn => fn({ resourceType: 'pods', type: 'ADDED', context: 'a', resource: item('ns-0', 'second') }));
        expect((await view.flush()).pods).toHaveLength(2);
        view.hide();
        expect(mocks.unsubscribe).toHaveBeenCalledWith('watch-');
    });
    it('handles a rejected watcher subscription with scoped fallback', async () => {
        const { useResourceWatcher } = await vi.importActual<typeof import('./useResourceWatcher')>('./useResourceWatcher');
        mocks.context.namespaces = ns(8);
        mocks.subscribe.mockRejectedValueOnce(new Error('Forbidden'));
        const spy = vi.spyOn(console, 'error').mockImplementation(() => {});
        try {
            const view = setup(vi.fn(async (_id, namespace) => namespace ? [item(namespace)] : ns(8).map(n => item(n))), ns(4), { watcher: useResourceWatcher });
            await view.flush();
            expect(mocks.subscribe.mock.calls.map(call => call[1])).toEqual(['', ...ns(4)]);
        } finally { spy.mockRestore(); }
    });
});
