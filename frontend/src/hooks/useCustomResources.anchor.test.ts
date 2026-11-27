import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

// Custom resource lists anchor their CRD watch to the list's resourceVersion, like
// built-in lists do. React is replaced by a tiny synchronous runner, as in
// useResource.pipeline.test.ts, so hook state can be driven deterministically.
const runner = vi.hoisted(() => ({ slots: [] as any[], index: 0, effects: [] as Array<() => void>, dirty: false }));
const mocks = vi.hoisted(() => ({
    context: {} as any,
    cancel: vi.fn().mockResolvedValue(undefined),
    outcome: vi.fn(),
    subscribe: vi.fn(), subscribeFrom: vi.fn(), crdSubscribe: vi.fn(), crdSubscribeFrom: vi.fn(), unsubscribe: vi.fn(),
    watcher: vi.fn(), list: vi.fn(),
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
            if (!old || !deps || deps.some((dep, i) => !Object.is(dep, old.deps[i]))) {
                const slot = { deps, cleanup: undefined as any };
                runner.slots[index] = slot;
                runner.effects.push(() => { old?.cleanup?.(); slot.cleanup = effect(); });
            }
        },
    };
});
vi.mock('../context', () => ({ useK8s: () => mocks.context }));
vi.mock('wailsjs/go/main/App', () => ({
    CancelListRequest: mocks.cancel, GetListOutcome: mocks.outcome,
    SubscribeResourceWatcher: mocks.subscribe, SubscribeResourceWatcherFrom: mocks.subscribeFrom,
    SubscribeCRDWatcher: mocks.crdSubscribe, SubscribeCRDWatcherFrom: mocks.crdSubscribeFrom,
    UnsubscribeWatcher: mocks.unsubscribe, ListCustomResources: mocks.list,
}));
vi.mock('wailsjs/runtime/runtime', () => ({ EventsOn: (name: string, fn: any) => {
    if (!mocks.events.has(name)) mocks.events.set(name, new Set());
    mocks.events.get(name)!.add(fn);
    return () => mocks.events.get(name)!.delete(fn);
} }));


import { useCustomResources } from './useCustomResources';

const item = (uid: string) => ({ metadata: { uid, namespace: 'ns-1', name: uid } });

function setup() {
    let result: any;
    const render = () => {
        runner.index = 0; runner.effects = []; runner.dirty = false;
        result = useCustomResources('a', 'g.io', 'v1', 'things', ['ns-1'], true, true);
        runner.effects.forEach(effect => effect());
        return result;
    };
    const flush = async () => {
        for (let i = 0; i < 60; i++) { await Promise.resolve(); if (runner.dirty) render(); }
        return result;
    };
    render();
    return { flush, last: () => result };
}

beforeEach(() => {
    runner.slots.forEach(slot => slot.cleanup?.());
    runner.slots = []; runner.index = 0; runner.effects = []; runner.dirty = false;
    vi.clearAllMocks(); mocks.events.clear();
    mocks.list.mockResolvedValue([item('a')]);
    mocks.crdSubscribe.mockResolvedValue('crd-plain');
    mocks.crdSubscribeFrom.mockResolvedValue('crd-anchored');
    mocks.unsubscribe.mockResolvedValue(undefined);
    mocks.context = { currentContext: 'a', namespaces: ['ns-1', 'ns-2'], lastRefresh: 1,
        reconcileToken: 0, connectionMode: 'streaming', registerPolling: vi.fn(() => vi.fn()), checkConnectionError: vi.fn() };
});

afterEach(() => {
    runner.slots.forEach(slot => slot.cleanup?.());
});

describe('custom resource list then watch', () => {
    it('subscribes the CRD watcher from the version of the list it just completed', async () => {
        mocks.outcome.mockResolvedValue({ found: true, context: 'a', resourceVersion: '31', streamed: false, loaded: 1 });
        const view = setup();
        await view.flush();
        expect(mocks.list).toHaveBeenCalled();
        expect(mocks.crdSubscribeFrom).toHaveBeenCalledWith('g.io', 'v1', 'things', 'ns-1', '31');
        expect(mocks.crdSubscribe).not.toHaveBeenCalled();
    });

    it('falls back to a plain CRD subscription when the list outcome is unknown', async () => {
        mocks.outcome.mockResolvedValue({ found: false });
        const view = setup();
        await view.flush();
        expect(mocks.crdSubscribe).toHaveBeenCalledWith('g.io', 'v1', 'things', 'ns-1');
        expect(mocks.crdSubscribeFrom).not.toHaveBeenCalled();
    });

    it('does not subscribe before the list has completed', async () => {
        let release!: (v: any[]) => void;
        mocks.list.mockReturnValue(new Promise<any[]>((resolve) => { release = resolve; }));
        mocks.outcome.mockResolvedValue({ found: true, context: 'a', resourceVersion: '5', streamed: false, loaded: 1 });
        const view = setup();
        await view.flush();
        expect(mocks.crdSubscribeFrom).not.toHaveBeenCalled();
        expect(mocks.crdSubscribe).not.toHaveBeenCalled();
        release([item('a')]);
        await view.flush();
        expect(mocks.crdSubscribeFrom).toHaveBeenCalledWith('g.io', 'v1', 'things', 'ns-1', '5');
    });
});
