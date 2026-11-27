import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

// List-then-watch ordering, paged lists assembled from list-page events alone, and
// the publish throttle. React is replaced by a tiny synchronous runner, as in
// useResource.pipeline.test.ts, so hook state can be driven deterministically.
const runner = vi.hoisted(() => ({ slots: [] as any[], index: 0, effects: [] as Array<() => void>, dirty: false }));
const mocks = vi.hoisted(() => ({
    context: {} as any,
    cancel: vi.fn().mockResolvedValue(undefined),
    outcome: vi.fn(),
    subscribe: vi.fn(), subscribeFrom: vi.fn(), crdSubscribe: vi.fn(), crdSubscribeFrom: vi.fn(), unsubscribe: vi.fn(),
    watcher: vi.fn(),
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
    UnsubscribeWatcher: mocks.unsubscribe,
}));
vi.mock('wailsjs/runtime/runtime', () => ({ EventsOn: (name: string, fn: any) => {
    if (!mocks.events.has(name)) mocks.events.set(name, new Set());
    mocks.events.get(name)!.add(fn);
    return () => mocks.events.get(name)!.delete(fn);
} }));

import { createNamespacedResourceHook, PAGE_COMPLETION_TIMEOUT_MS, PAGE_PUBLISH_INTERVAL_MS } from './useResource';
import { useResourceWatcher, useCRDWatcher } from './useResourceWatcher';

let typeCounter = 0;
const freshType = () => `listwatch-${++typeCounter}`;
const item = (namespace: string | undefined, uid: string, extra: any = {}) => ({ metadata: { uid, namespace, name: uid }, ...extra });
const uids = (list: any[]) => list.map(entry => entry.metadata.uid);
const deferred = <T>() => {
    let resolve!: (value: T) => void; let reject!: (error: Error) => void;
    const promise = new Promise<T>((res, rej) => { resolve = res; reject = rej; });
    return { promise, resolve, reject };
};
const page = (event: any) => mocks.events.get('list-page')?.forEach(listener => listener(event));
const resourceEvent = (event: any) => mocks.events.get('resource-event')?.forEach(listener => listener(event));

function setup(hook: any, selection: string[] = ['*']) {
    let result: any;
    const render = () => {
        runner.index = 0; runner.effects = []; runner.dirty = false;
        result = hook('a', selection, true);
        runner.effects.forEach(effect => effect());
        return result;
    };
    const flush = async () => {
        for (let i = 0; i < 60; i++) { await Promise.resolve(); if (runner.dirty) render(); }
        return result;
    };
    render();
    return { render, flush, last: () => result };
}
/** Latest options passed to a recorded fake watcher. */
const watcherOptions = () => mocks.watcher.mock.lastCall![5];

beforeEach(() => {
    runner.slots.forEach(slot => slot.cleanup?.());
    runner.slots = []; runner.index = 0; runner.effects = []; runner.dirty = false;
    vi.clearAllMocks(); mocks.events.clear();
    mocks.outcome.mockResolvedValue({ found: false });
    mocks.subscribe.mockResolvedValue('watch-plain');
    mocks.subscribeFrom.mockResolvedValue('watch-anchored');
    mocks.crdSubscribe.mockResolvedValue('crd-plain');
    mocks.crdSubscribeFrom.mockResolvedValue('crd-anchored');
    mocks.unsubscribe.mockResolvedValue(undefined);
    mocks.context = { currentContext: 'a', namespaces: Array.from({ length: 100 }, (_, i) => `ns-${i}`), lastRefresh: 1,
        reconcileToken: 0, connectionMode: 'streaming', registerPolling: vi.fn(() => vi.fn()), checkConnectionError: vi.fn() };
});

describe('list then watch', () => {
    it('defers the backend subscription until the list completes, then anchors it to that list', async () => {
        const gate = deferred<any[]>();
        const list = vi.fn((_id: string, _namespace: string) => gate.promise);
        const view = setup(createNamespacedResourceHook(freshType(), list, 'pods', mocks.watcher));
        expect(watcherOptions().subscribe).toBe(false);
        // Listeners stay enabled meanwhile so events from a shared watcher reach the journal.
        expect(mocks.watcher.mock.lastCall![3]).toBe(true);
        gate.resolve([item('ns-0', 'a')]);
        await view.flush();
        expect(watcherOptions().subscribe).toBe(true);

        mocks.outcome.mockResolvedValueOnce({ found: true, context: 'a', resourceVersion: '4711', streamed: false, loaded: 1 });
        expect(await watcherOptions().resourceVersionFor('')).toBe('4711');
        expect(mocks.outcome).toHaveBeenCalledWith(list.mock.calls[0][0]);
    });

    it('hands out each list anchor once so a later resubscription never reuses a stale version', async () => {
        const list = vi.fn(async () => [item('ns-0', 'a')]);
        const view = setup(createNamespacedResourceHook(freshType(), list, 'pods', mocks.watcher));
        await view.flush();
        mocks.outcome.mockResolvedValue({ found: true, context: 'a', resourceVersion: '9', streamed: false, loaded: 1 });
        expect(await watcherOptions().resourceVersionFor('')).toBe('9');
        expect(await watcherOptions().resourceVersionFor('')).toBeUndefined();
    });

    it('falls back to an unanchored subscription when the outcome is unknown, from another context, or fails', async () => {
        const list = vi.fn(async () => [item('ns-0', 'a')]);
        const view = setup(createNamespacedResourceHook(freshType(), list, 'pods', mocks.watcher));
        await view.flush();
        const resolve = () => watcherOptions().resourceVersionFor('');
        mocks.outcome.mockResolvedValueOnce({ found: false });
        expect(await resolve()).toBeUndefined();

        mocks.cancel.mockClear();
        mocks.context.reconcileToken++;
        view.render(); await view.flush();
        mocks.outcome.mockResolvedValueOnce({ found: true, context: 'other', resourceVersion: '5', streamed: false, loaded: 1 });
        expect(await resolve()).toBeUndefined();

        mocks.context.reconcileToken++;
        view.render(); await view.flush();
        mocks.outcome.mockRejectedValueOnce(new Error('bridge down'));
        expect(await resolve()).toBeUndefined();

        mocks.context.reconcileToken++;
        view.render(); await view.flush();
        mocks.outcome.mockResolvedValueOnce({ found: true, context: 'a', resourceVersion: '', streamed: false, loaded: 1 });
        expect(await resolve()).toBeUndefined();
    });

    it('does not subscribe when the list fails, and subscribes after a successful retry', async () => {
        const list = vi.fn(async (): Promise<any[]> => { throw new Error('boom'); });
        const view = setup(createNamespacedResourceHook(freshType(), list, 'pods', mocks.watcher));
        await view.flush();
        expect(watcherOptions().subscribe).toBe(false);
        list.mockResolvedValue([item('ns-0', 'a')]);
        view.last().refetch();
        // refetch bumps state inside the hook; render picks it up.
        await view.flush();
        view.render(); await view.flush();
        expect(watcherOptions().subscribe).toBe(true);
    });

    it('anchors every namespace of a multi-namespace scope to its own list', async () => {
        const list = vi.fn(async (_id: string, namespace: string) => [item(namespace, `u-${namespace}`)]);
        const view = setup(createNamespacedResourceHook(freshType(), list, 'pods', mocks.watcher), ['ns-1', 'ns-2']);
        await view.flush();
        const ids = Object.fromEntries(list.mock.calls.map(call => [call[1], call[0]]));
        mocks.outcome.mockImplementation(async (id: string) => ({
            found: true, context: 'a', resourceVersion: id === ids['ns-1'] ? '100' : '200', streamed: false, loaded: 1 }));
        expect(await watcherOptions().resourceVersionFor('ns-1')).toBe('100');
        expect(await watcherOptions().resourceVersionFor('ns-2')).toBe('200');
    });

    it('keeps merging events that arrive while the list is in flight (journal) with listeners already enabled', async () => {
        const gate = deferred<any[]>();
        const view = setup(createNamespacedResourceHook(freshType(), () => gate.promise, 'pods', mocks.watcher));
        const handler = mocks.watcher.mock.lastCall![2];
        handler({ type: 'ADDED', context: 'a', resource: item('ns-0', 'late') });
        gate.resolve([item('ns-0', 'listed')]);
        expect(uids((await view.flush()).pods)).toEqual(['listed', 'late']);
    });

    it('shows a cached snapshot immediately but still waits for a fresh list before subscribing', async () => {
        const type = freshType();
        const hook = createNamespacedResourceHook(type, vi.fn(async () => [item('ns-0', 'a')]), 'pods', mocks.watcher);
        const first = setup(hook);
        await first.flush();
        runner.slots.forEach(slot => slot.cleanup?.());
        runner.slots = [];

        const gate = deferred<any[]>();
        const secondList = vi.fn(() => gate.promise);
        const second = setup(createNamespacedResourceHook(type, secondList, 'pods', mocks.watcher));
        await second.flush();
        expect(uids(second.render().pods)).toEqual(['a']);
        expect(watcherOptions().subscribe).toBe(false);
        gate.resolve([item('ns-0', 'a'), item('ns-0', 'b')]);
        await second.flush();
        expect(watcherOptions().subscribe).toBe(true);
        expect(uids(second.render().pods)).toEqual(['a', 'b']);
    });
});

describe('real watcher hook with anchors', () => {
    const run = (hook: any) => {
        let selected = ['*'];
        const render = () => { runner.index = 0; runner.effects = []; runner.dirty = false; hook(selected); runner.effects.forEach(effect => effect()); };
        render();
        return { render, flush: async () => { for (let i = 0; i < 40; i++) await Promise.resolve(); } };
    };

    it('uses the From binding with the list version and the plain binding without one', async () => {
        const options = { subscribe: true, resourceVersionFor: vi.fn(async (ns: string) => ns === 'a' ? '77' : undefined) };
        const view = run((_: string[]) => useResourceWatcher('pods', ['a', 'b'], vi.fn() as any, true, undefined, options));
        await view.flush();
        expect(mocks.subscribeFrom).toHaveBeenCalledWith('pods', 'a', '77');
        expect(mocks.subscribe).toHaveBeenCalledWith('pods', 'b');
    });

    it('subscribes the CRD binding with an anchor too', async () => {
        const options = { subscribe: true, resourceVersionFor: async () => '12' };
        const view = run(() => useCRDWatcher('g.io', 'v1', 'things', [''], vi.fn() as any, true, undefined, options));
        await view.flush();
        expect(mocks.crdSubscribeFrom).toHaveBeenCalledWith('g.io', 'v1', 'things', '', '12');
        expect(mocks.crdSubscribe).not.toHaveBeenCalled();
    });

    it('requests no watcher while subscribe is false but still delivers events, then subscribes once enabled', async () => {
        let subscribe = false;
        const received: any[] = [];
        const onEvent: any = (event: any) => received.push(event);
        const view = run(() => useResourceWatcher('pods', [''], onEvent, true, undefined, { subscribe }));
        await view.flush();
        expect(mocks.subscribe).not.toHaveBeenCalled();
        resourceEvent({ resourceType: 'pods', type: 'ADDED', context: 'a', resource: item('ns-0', 'x') });
        expect(received).toHaveLength(1);
        subscribe = true;
        view.render(); await view.flush();
        expect(mocks.subscribe).toHaveBeenCalledTimes(1);
        expect(mocks.events.get('resource-event')!.size).toBe(1);
    });

    it('treats a throwing anchor lookup as no anchor', async () => {
        const options = { subscribe: true, resourceVersionFor: async () => { throw new Error('no'); } };
        const view = run(() => useResourceWatcher('pods', [''], vi.fn() as any, true, undefined, options));
        await view.flush();
        expect(mocks.subscribe).toHaveBeenCalledWith('pods', '');
        expect(mocks.subscribeFrom).not.toHaveBeenCalled();
    });
});

describe('streamed pages', () => {
    it('assembles a paged list from list-page events when the call returns no items', async () => {
        const calls: { id?: string } = {};
        const gate = deferred<any[]>();
        const list = vi.fn((id: string) => { calls.id = id; return gate.promise; });
        const view = setup(createNamespacedResourceHook(freshType(), list, 'pods', mocks.watcher));
        page({ requestId: calls.id, items: [item('ns-0', 'a'), item('ns-0', 'b')], loaded: 2, total: 3 });
        page({ requestId: calls.id, items: [item('ns-0', 'c')], loaded: 3, total: 3 });
        mocks.outcome.mockResolvedValue({ found: true, context: 'a', resourceVersion: '55', streamed: true, loaded: 3 });
        gate.resolve([]);
        const result = await view.flush();
        expect(uids(result.pods)).toEqual(['a', 'b', 'c']);
        expect(result.loadState.phase).toBe('complete');
        expect(list).toHaveBeenCalledTimes(1);
    });

    it('waits for pages that are still in flight when the call result arrives first', async () => {
        const calls: { id?: string } = {};
        const list = vi.fn(async (id: string) => { calls.id = id; return [] as any[]; });
        mocks.outcome.mockResolvedValue({ found: true, context: 'a', resourceVersion: '1', streamed: true, loaded: 2 });
        const view = setup(createNamespacedResourceHook(freshType(), list, 'pods', mocks.watcher));
        await view.flush();
        expect(view.render().loadState.phase).toBe('loading');
        page({ requestId: calls.id, items: [item('ns-0', 'a')], loaded: 1, total: 2 });
        await view.flush();
        expect(view.render().loadState.phase).toBe('loading');
        page({ requestId: calls.id, items: [item('ns-0', 'b')], loaded: 2, total: 2 });
        const result = await view.flush();
        expect(uids(result.pods)).toEqual(['a', 'b']);
        expect(result.loadState.phase).toBe('complete');
    });

    it('re-lists without streaming when pages never arrive', async () => {
        vi.useFakeTimers();
        try {
            const list = vi.fn(async (id: string) => id.startsWith('page-') ? [] : [item('ns-0', 'full')]);
            mocks.outcome.mockResolvedValue({ found: true, context: 'a', resourceVersion: '1', streamed: true, loaded: 1 });
            const view = setup(createNamespacedResourceHook(freshType(), list, 'pods', mocks.watcher));
            await vi.advanceTimersByTimeAsync(PAGE_COMPLETION_TIMEOUT_MS + 1);
            const result = await view.flush();
            expect(list).toHaveBeenCalledTimes(2);
            expect(list.mock.calls[1][0]).toMatch(/^list-/);
            expect(uids(result.pods)).toEqual(['full']);
        } finally { vi.useRealTimers(); }
    });

    it('does not wait or re-list for a genuinely empty list', async () => {
        const list = vi.fn(async () => [] as any[]);
        mocks.outcome.mockResolvedValue({ found: true, context: 'a', resourceVersion: '1', streamed: false, loaded: 0 });
        const view = setup(createNamespacedResourceHook(freshType(), list, 'pods', mocks.watcher));
        const result = await view.flush();
        expect(list).toHaveBeenCalledTimes(1);
        expect(result.pods).toEqual([]);
        expect(result.loadState.phase).toBe('complete');
    });

    it('does not query the outcome for non-empty results or refreshes', async () => {
        const list = vi.fn(async () => [item('ns-0', 'a')]);
        const view = setup(createNamespacedResourceHook(freshType(), list, 'pods', mocks.watcher));
        await view.flush();
        expect(mocks.outcome).not.toHaveBeenCalled();
    });

    it('anchors a watcher to the request that assembled the pages', async () => {
        const calls: { id?: string } = {};
        const list = vi.fn(async (id: string) => { calls.id = id; return [] as any[]; });
        mocks.outcome.mockResolvedValue({ found: true, context: 'a', resourceVersion: '321', streamed: true, loaded: 1 });
        const view = setup(createNamespacedResourceHook(freshType(), list, 'pods', mocks.watcher));
        await view.flush();
        page({ requestId: calls.id, items: [item('ns-0', 'a')], loaded: 1, total: 1 });
        await view.flush();
        expect(await watcherOptions().resourceVersionFor('')).toBe('321');
        expect(mocks.outcome).toHaveBeenLastCalledWith(calls.id);
    });
});

describe('page publishing', () => {
    beforeEach(() => { vi.useFakeTimers(); });
    afterEach(() => { vi.useRealTimers(); });

    it('renders the first page at once and coalesces the rest into one update per interval', () => {
        const calls: { id?: string } = {};
        const gate = deferred<any[]>();
        const list = vi.fn((id: string) => { calls.id = id; return gate.promise; });
        const view = setup(createNamespacedResourceHook(freshType(), list, 'pods', mocks.watcher));
        page({ requestId: calls.id, items: [item('ns-0', 'p0')], loaded: 1, total: 50 });
        expect(uids(view.render().pods)).toEqual(['p0']);
        const snapshotsBefore = view.render().pods;
        for (let i = 1; i < 50; i++) page({ requestId: calls.id, items: [item('ns-0', `p${i}`)], loaded: i + 1, total: 50 });
        // Nothing more is published inside the interval ...
        expect(view.render().pods).toBe(snapshotsBefore);
        vi.advanceTimersByTime(PAGE_PUBLISH_INTERVAL_MS);
        // ... and then everything arrives in one update.
        expect(view.render().pods).toHaveLength(50);
        expect(view.render().loadingProgress).toEqual({ loaded: 50, total: 50 });
    });

    it('does not mutate a published snapshot when later pages arrive', () => {
        const calls: { id?: string } = {};
        const list = vi.fn((id: string) => { calls.id = id; return new Promise<any[]>(() => {}); });
        const view = setup(createNamespacedResourceHook(freshType(), list, 'pods', mocks.watcher));
        page({ requestId: calls.id, items: [item('ns-0', 'a')], loaded: 1, total: 2 });
        const shown = view.render().pods;
        page({ requestId: calls.id, items: [item('ns-0', 'b')], loaded: 2, total: 2 });
        vi.advanceTimersByTime(PAGE_PUBLISH_INTERVAL_MS);
        expect(uids(shown)).toEqual(['a']);
        expect(uids(view.render().pods)).toEqual(['a', 'b']);
    });

    it('publishes the final progress when the list completes inside the interval', async () => {
        const calls: { id?: string } = {};
        const gate = deferred<any[]>();
        const list = vi.fn((id: string) => { calls.id = id; return gate.promise; });
        const view = setup(createNamespacedResourceHook(freshType(), list, 'pods', mocks.watcher));
        page({ requestId: calls.id, items: [item('ns-0', 'a')], loaded: 1, total: 2 });
        page({ requestId: calls.id, items: [item('ns-0', 'b')], loaded: 2, total: 2 });
        gate.resolve([item('ns-0', 'a'), item('ns-0', 'b')]);
        const result = await view.flush();
        expect(result.loadingProgress).toEqual({ loaded: 2, total: 2 });
        expect(uids(result.pods)).toEqual(['a', 'b']);
    });

    it('stops publishing after the effect is cancelled', () => {
        const calls: { id?: string } = {};
        const list = vi.fn((id: string) => { calls.id = id; return new Promise<any[]>(() => {}); });
        const view = setup(createNamespacedResourceHook(freshType(), list, 'pods', mocks.watcher));
        page({ requestId: calls.id, items: [item('ns-0', 'a')], loaded: 1, total: 3 });
        page({ requestId: calls.id, items: [item('ns-0', 'b')], loaded: 2, total: 3 });
        runner.slots.forEach(slot => slot.cleanup?.());
        vi.advanceTimersByTime(PAGE_PUBLISH_INTERVAL_MS * 2);
        expect(uids(view.render().pods)).toEqual(['a']);
    });
});

describe('render stability', () => {
    it('returns the same result object and loadState across renders with unchanged inputs', async () => {
        const list = vi.fn(async () => [item('ns-0', 'a')]);
        const view = setup(createNamespacedResourceHook(freshType(), list, 'pods', mocks.watcher));
        await view.flush();
        const first = view.render();
        const second = view.render();
        expect(second).toBe(first);
        expect(second.loadState).toBe(first.loadState);
    });
});
