import { beforeEach, describe, expect, it, vi } from 'vitest';

// Complements useResource.namespaces.test.ts: pins the event journal, list-page and
// scope-filter behaviour of the resource pipeline that is about to be optimised.
const runner = vi.hoisted(() => ({ slots: [] as any[], index: 0, effects: [] as Array<() => void>, dirty: false }));
const mocks = vi.hoisted(() => ({
    context: {} as any,
    cancel: vi.fn().mockResolvedValue(undefined), watcher: vi.fn(),
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
vi.mock('./useResourceWatcher', () => ({ useResourceWatcher: mocks.watcher }));
vi.mock('wailsjs/go/main/App', () => ({ CancelListRequest: mocks.cancel }));
vi.mock('wailsjs/runtime/runtime', () => ({ EventsOn: (name: string, fn: any) => {
    if (!mocks.events.has(name)) mocks.events.set(name, new Set());
    mocks.events.get(name)!.add(fn);
    return () => mocks.events.get(name)!.delete(fn);
} }));

import { createClusterScopedResourceHook, createNamespacedResourceHook } from './useResource';
import { applyResourceEvents } from './useResourceEventHandler';

let typeCounter = 0;
// A unique resource type per test keeps the shared navigation-snapshot cache from leaking between tests.
const freshType = () => `pipeline-${++typeCounter}`;
const ns = (count: number) => Array.from({ length: count }, (_, i) => `ns-${i}`);
const item = (namespace: string | undefined, uid: string, extra: any = {}) => ({ metadata: { uid, namespace, name: uid }, ...extra });

function setup(hook: any, selection: string[] = ['*']) {
    let result: any;
    let selected = selection;
    const render = () => {
        runner.index = 0; runner.effects = []; runner.dirty = false;
        result = hook('a', selected, true);
        runner.effects.forEach(effect => effect());
        return result;
    };
    const flush = async () => {
        for (let i = 0; i < 40; i++) { await Promise.resolve(); if (runner.dirty) render(); }
        return result;
    };
    render();
    return {
        flush, render,
        select: (value: string[]) => { selected = value; return render(); },
        event: (event: any) => mocks.watcher.mock.lastCall![2]({ context: 'a', ...event }),
        batch: (events: any[]) => mocks.watcher.mock.lastCall![2].batch(events.map(event => ({ context: 'a', ...event }))),
        page: (event: any) => mocks.events.get('list-page')?.forEach(listener => listener(event)),
    };
}
const deferred = <T>() => {
    let resolve!: (value: T) => void; let reject!: (error: Error) => void;
    const promise = new Promise<T>((res, rej) => { resolve = res; reject = rej; });
    return { promise, resolve, reject };
};
const uids = (pods: any[]) => pods.map(pod => pod.metadata.uid);

beforeEach(() => {
    runner.slots.forEach(slot => slot.cleanup?.());
    runner.slots = []; runner.index = 0; runner.effects = []; runner.dirty = false;
    vi.clearAllMocks(); mocks.events.clear();
    mocks.context = { currentContext: 'a', namespaces: ns(100), lastRefresh: 1, reconcileToken: 0, connectionMode: 'streaming',
        registerPolling: vi.fn(() => vi.fn()), checkConnectionError: vi.fn() };
});

describe('events received while a list is in flight (journal)', () => {
    const inFlight = async (initial: any[] = [item('ns-0', 'existing', { v: 'listed' })]) => {
        const type = freshType();
        const first = deferred<any[]>();
        const list = vi.fn(() => first.promise);
        const view = setup(createNamespacedResourceHook(type, list, 'pods'));
        return { view, finish: (items: any[] = initial) => first.resolve(items), list };
    };

    it('merges an ADDED event that the list response does not contain', async () => {
        const { view, finish } = await inFlight();
        view.event({ type: 'ADDED', resource: item('ns-0', 'late-added') });
        finish();
        expect(uids((await view.flush()).pods)).toEqual(['existing', 'late-added']);
    });

    it('lets a MODIFIED event win over the older copy in the list response', async () => {
        const { view, finish } = await inFlight();
        view.event({ type: 'MODIFIED', resource: item('ns-0', 'existing', { v: 'newer' }) });
        finish();
        expect((await view.flush()).pods[0].v).toBe('newer');
    });

    it('drops a resource that was added and then deleted during the list', async () => {
        const { view, finish } = await inFlight();
        view.event({ type: 'ADDED', resource: item('ns-0', 'blip') });
        view.event({ type: 'DELETED', resource: item('ns-0', 'blip') });
        finish([item('ns-0', 'existing'), item('ns-0', 'blip')]);
        expect(uids((await view.flush()).pods)).toEqual(['existing']);
    });

    it('keeps the listed copy when a duplicate ADDED arrives for a listed uid', async () => {
        const { view, finish } = await inFlight();
        view.event({ type: 'ADDED', resource: item('ns-0', 'existing', { v: 'duplicate' }) });
        finish();
        expect((await view.flush()).pods[0].v).toBe('listed');
    });

    it('ignores a late DELETED for a resource that was never listed', async () => {
        const { view, finish } = await inFlight();
        view.event({ type: 'DELETED', resource: item('ns-0', 'ghost') });
        finish();
        expect(uids((await view.flush()).pods)).toEqual(['existing']);
    });

    it('ignores a MODIFIED-with-deletionTimestamp for a resource already gone', async () => {
        const { view, finish } = await inFlight();
        view.event({ type: 'DELETED', resource: item('ns-0', 'existing') });
        view.event({ type: 'MODIFIED', resource: { metadata: { uid: 'existing', namespace: 'ns-0', deletionTimestamp: 'now' } } });
        finish();
        expect(uids((await view.flush()).pods)).toEqual([]);
    });

    it('resurrects a deleted uid only through a fresh ADDED', async () => {
        const { view, finish } = await inFlight();
        view.event({ type: 'DELETED', resource: item('ns-0', 'existing') });
        view.event({ type: 'ADDED', resource: item('ns-0', 'existing', { v: 'recreated' }) });
        finish();
        const pods = (await view.flush()).pods;
        expect(pods).toHaveLength(1);
        expect(pods[0].v).toBe('recreated');
    });

    it('does not journal or apply events outside the fetched scope, from other contexts or without a uid', async () => {
        const type = freshType();
        const first = deferred<any[]>();
        const view = setup(createNamespacedResourceHook(type, () => first.promise, 'pods'), ['ns-0']);
        view.event({ type: 'ADDED', resource: item('ns-9', 'elsewhere') });
        view.event({ type: 'ADDED', context: 'other', resource: item('ns-0', 'wrong-ctx') });
        view.event({ type: 'ADDED', resource: { metadata: { namespace: 'ns-0' } } });
        view.event({ type: 'ADDED', resource: item('ns-0', 'wanted') });
        first.resolve([]);
        expect(uids((await view.flush()).pods)).toEqual(['wanted']);
    });

    it('takes the namespace from the event envelope when the resource carries none', async () => {
        const type = freshType();
        const first = deferred<any[]>();
        const view = setup(createNamespacedResourceHook(type, () => first.promise, 'pods'), ['ns-0']);
        view.event({ type: 'ADDED', namespace: 'ns-0', resource: { metadata: { uid: 'enveloped' } } });
        view.event({ type: 'ADDED', namespace: 'ns-5', resource: { metadata: { uid: 'enveloped-elsewhere' } } });
        first.resolve([]);
        // Scope accepts the event, but the display filter requires metadata.namespace to be selected.
        expect(uids((await view.flush()).pods)).toEqual([]);
    });

    it('applies a batch of events with the same semantics as individual events', async () => {
        const { view, finish } = await inFlight();
        view.batch([
            { type: 'ADDED', resource: item('ns-0', 'b1') },
            { type: 'ADDED', resource: item('ns-0', 'b1', { v: 'dup' }) },
            { type: 'MODIFIED', resource: item('ns-0', 'existing', { v: 'batched' }) },
            { type: 'DELETED', resource: item('ns-0', 'never-listed') },
        ]);
        finish();
        const pods = (await view.flush()).pods;
        expect(uids(pods)).toEqual(['existing', 'b1']);
        expect(pods[0].v).toBe('batched');
        expect(pods[1].v).toBeUndefined();
    });
});

describe('events after the list has completed', () => {
    it('updates in place, ignores duplicate ADDED, removes on DELETED and tolerates a repeated DELETED', async () => {
        const list = vi.fn(async () => [item('ns-0', 'a', { v: 1 }), item('ns-0', 'b', { v: 1 })]);
        const view = setup(createNamespacedResourceHook(freshType(), list, 'pods'));
        await view.flush();
        view.event({ type: 'ADDED', resource: item('ns-0', 'a', { v: 'dup' }) });
        view.event({ type: 'MODIFIED', resource: item('ns-0', 'b', { v: 2 }) });
        view.event({ type: 'ADDED', resource: item('ns-0', 'c', { v: 1 }) });
        const result = await view.flush();
        expect(result.pods.map((p: any) => [p.metadata.uid, p.v])).toEqual([['a', 1], ['b', 2], ['c', 1]]);
        view.event({ type: 'DELETED', resource: item('ns-0', 'a') });
        view.event({ type: 'DELETED', resource: item('ns-0', 'a') });
        expect(uids((await view.flush()).pods)).toEqual(['b', 'c']);
    });

    it('does not re-render for events that change nothing', async () => {
        const list = vi.fn(async () => [item('ns-0', 'a')]);
        const view = setup(createNamespacedResourceHook(freshType(), list, 'pods'));
        const before = (await view.flush()).pods;
        view.event({ type: 'DELETED', resource: item('ns-0', 'zzz') });
        view.event({ type: 'ADDED', resource: item('ns-0', 'a') });
        expect(runner.dirty).toBe(false);
        expect(view.render().pods).toBe(before);
    });

    it('treats a MODIFIED for an unknown uid as an add (event raced the list)', async () => {
        const list = vi.fn(async () => [item('ns-0', 'a')]);
        const view = setup(createNamespacedResourceHook(freshType(), list, 'pods'));
        await view.flush();
        view.event({ type: 'MODIFIED', resource: item('ns-0', 'b') });
        expect(uids((await view.flush()).pods)).toEqual(['a', 'b']);
    });
});

describe('list-page handling', () => {
    it('uses a page- request id on first load and a plain id on refresh', async () => {
        const list = vi.fn(async (_id: string, _ns: string) => [item('ns-0', 'a')]);
        const view = setup(createNamespacedResourceHook(freshType(), list, 'pods'));
        await view.flush();
        expect(list.mock.calls[0][0]).toMatch(/^page-list-/);
        mocks.context.reconcileToken++;
        view.render();
        await view.flush();
        expect(list.mock.calls[1][0]).toMatch(/^list-/);
        expect(list.mock.calls[1][0]).not.toMatch(/^page-/);
    });

    it('skips page items that have no uid and ignores malformed page payloads', async () => {
        const gate = deferred<any[]>();
        const list = vi.fn((_id: string) => gate.promise);
        const view = setup(createNamespacedResourceHook(freshType(), list, 'pods'));
        const requestId = list.mock.calls[0][0];
        view.page({ requestId, items: 'not-an-array', loaded: 1, total: 1 });
        view.page({ requestId: undefined, items: [item('ns-0', 'x')], loaded: 1, total: 1 });
        view.page(null);
        expect(view.render().pods).toEqual([]);
        view.page({ requestId, items: [{ metadata: { namespace: 'ns-0' } }, item('ns-0', 'ok')], loaded: 2, total: 2 });
        expect(uids(view.render().pods)).toEqual(['ok']);
    });

    it('replaces partial page rows with the authoritative list when it completes', async () => {
        const gate = deferred<any[]>();
        const list = vi.fn((_id: string) => gate.promise);
        const view = setup(createNamespacedResourceHook(freshType(), list, 'pods'));
        view.page({ requestId: list.mock.calls[0][0], items: [item('ns-0', 'ghost'), item('ns-0', 'real')], loaded: 2, total: 3 });
        expect(uids(view.render().pods)).toEqual(['ghost', 'real']);
        gate.resolve([item('ns-0', 'real'), item('ns-0', 'final')]);
        expect(uids((await view.flush()).pods)).toEqual(['real', 'final']);
        expect(view.render().loadingProgress).toEqual({ loaded: 2, total: 3 });
    });

    it('does not publish throttled partial rows after the list failed', async () => {
        vi.useFakeTimers();
        try {
            const gate = deferred<any[]>();
            const list = vi.fn((_id: string) => gate.promise);
            const view = setup(createNamespacedResourceHook(freshType(), list, 'pods'));
            const requestId = list.mock.calls[0][0];
            view.page({ requestId, items: [item('ns-0', 'first')], loaded: 1, total: 4 });
            // Inside the throttle window: this page arms the trailing publish timer.
            view.page({ requestId, items: [item('ns-0', 'second')], loaded: 2, total: 4 });
            gate.reject(new Error('boom'));
            await view.flush();
            const before = view.render();
            expect(before.error).toBeTruthy();
            const shown = uids(before.pods);
            await vi.advanceTimersByTimeAsync(1000);
            expect(uids(view.render().pods)).toEqual(shown);
            expect(vi.getTimerCount()).toBe(0);
        } finally {
            vi.useRealTimers();
        }
    });

    it('filters page rows through the namespace selection while loading', async () => {
        const gate = deferred<any[]>();
        const list = vi.fn((_id: string) => gate.promise);
        mocks.context.namespaces = ns(1000);
        const view = setup(createNamespacedResourceHook(freshType(), list, 'pods'), ['ns-1']);
        view.page({ requestId: list.mock.calls[0][0], items: [item('ns-0', 'a'), item('ns-1', 'b')], loaded: 2, total: 2 });
        expect(uids(view.render().pods)).toEqual(['b']);
    });

    it('does not show page rows during a refresh that already has data', async () => {
        const first = vi.fn(async () => [item('ns-0', 'old')]);
        const view = setup(createNamespacedResourceHook(freshType(), first, 'pods'));
        await view.flush();
        const gate = deferred<any[]>();
        first.mockImplementation(() => gate.promise as any);
        mocks.context.reconcileToken++;
        view.render();
        const refreshId = (first.mock.calls[first.mock.calls.length - 1] as any[])[0];
        view.page({ requestId: refreshId, items: [item('ns-0', 'partial')], loaded: 1, total: 5 });
        expect(uids(view.render().pods)).toEqual(['old']);
        gate.resolve([item('ns-0', 'old'), item('ns-0', 'new')]);
        expect(uids((await view.flush()).pods)).toEqual(['old', 'new']);
    });
});

describe('namespace filtering of displayed data', () => {
    it('hides items without a namespace from a namespaced selection but shows them for "*"', async () => {
        const list = vi.fn(async () => [item('ns-0', 'a'), item(undefined, 'cluster-wide')]);
        mocks.context.namespaces = ns(1000);
        const view = setup(createNamespacedResourceHook(freshType(), list, 'pods'));
        expect(uids((await view.flush()).pods)).toEqual(['a', 'cluster-wide']);
        expect(uids(view.select(['ns-0']).pods)).toEqual(['a']);
        expect(uids(view.select(['*']).pods)).toEqual(['a', 'cluster-wide']);
    });

    it('re-filters local data when the selection changes between small scopes without re-listing broadly', async () => {
        mocks.context.namespaces = ns(1000);
        const list = vi.fn(async () => ns(5).map((n, i) => item(n, `u${i}`)));
        const view = setup(createNamespacedResourceHook(freshType(), list, 'pods'));
        await view.flush();
        expect(uids(view.select(['ns-1', 'ns-3']).pods)).toEqual(['u1', 'u3']);
        expect(uids(view.select(['ns-4']).pods)).toEqual(['u4']);
        expect(list).toHaveBeenCalledTimes(1);
    });

    it('cluster-scoped hook lists once without a namespace and shows every item', async () => {
        const list = vi.fn(async (_id: string) => [item(undefined, 'n1'), item(undefined, 'n2')]);
        const cluster = createClusterScopedResourceHook(freshType(), list, 'nodes');
        const view = setup((ctx: string, _sel: string[], visible: boolean) => cluster(ctx, visible));
        const result = await view.flush();
        expect(uids(result.nodes)).toEqual(['n1', 'n2']);
        expect(list).toHaveBeenCalledTimes(1);
        expect(list.mock.calls[0]).toHaveLength(1);
        expect(mocks.watcher.mock.lastCall![1]).toEqual(['']);
        view.event({ type: 'ADDED', resource: item(undefined, 'n3') });
        expect(uids((await view.flush()).nodes)).toEqual(['n1', 'n2', 'n3']);
    });

    it('reports an empty initial state without a flash of data for a different context', async () => {
        const list = vi.fn(async () => [item('ns-0', 'a')]);
        const view = setup(createNamespacedResourceHook(freshType(), list, 'pods'));
        expect(view.render().pods).toEqual([]);
        expect(view.render().loadState.phase).toBe('loading');
        await view.flush();
        expect(view.render().loadState.phase).toBe('complete');
    });
});

describe('applyResourceEvents', () => {
    const r = (uid: string, extra: any = {}) => ({ metadata: { uid }, ...extra });

    it('returns the identical map for no-op sequences and never mutates the input', () => {
        const previous = new Map([['a', r('a')]]);
        const same = applyResourceEvents(previous, [
            { type: 'ADDED', resource: r('a', { v: 2 }) },
            { type: 'DELETED', resource: r('zzz') },
            { type: 'MODIFIED', resource: { metadata: { uid: 'gone', deletionTimestamp: 't' } } },
            { type: 'ADDED', resource: { metadata: {} } },
            { type: 'BOOKMARK' as any, resource: r('a') },
        ]);
        expect(same).toBe(previous);
        const changed = applyResourceEvents(previous, [{ type: 'MODIFIED', resource: r('a', { v: 3 }) }]);
        expect(changed).not.toBe(previous);
        expect((previous.get('a') as any).v).toBeUndefined();
        expect((changed.get('a') as any).v).toBe(3);
    });

    it('preserves insertion order: modified keeps position, delete+add moves to the end', () => {
        const previous = new Map([['a', r('a')], ['b', r('b')], ['c', r('c')]]);
        const modified = applyResourceEvents(previous, [{ type: 'MODIFIED', resource: r('a', { v: 1 }) }]);
        expect([...modified.keys()]).toEqual(['a', 'b', 'c']);
        const moved = applyResourceEvents(previous, [
            { type: 'DELETED', resource: r('a') }, { type: 'ADDED', resource: r('a', { v: 2 }) },
        ]);
        expect([...moved.keys()]).toEqual(['b', 'c', 'a']);
    });

    it('allows MODIFIED with a deletionTimestamp for an existing uid (terminating resource stays visible)', () => {
        const previous = new Map([['a', r('a')]]);
        const result = applyResourceEvents(previous, [{ type: 'MODIFIED', resource: { metadata: { uid: 'a', deletionTimestamp: 't' } } }]);
        expect((result.get('a') as any).metadata.deletionTimestamp).toBe('t');
    });

    it('processes 10000 mixed events against a 10000 entry map with the expected end state', () => {
        const previous = new Map(Array.from({ length: 10000 }, (_, i) => [`u${i}`, r(`u${i}`, { v: 0 })] as [string, any]));
        const events: any[] = [];
        for (let i = 0; i < 10000; i++) {
            if (i % 4 === 0) events.push({ type: 'DELETED', resource: r(`u${i}`) });
            else if (i % 4 === 1) events.push({ type: 'MODIFIED', resource: r(`u${i}`, { v: 1 }) });
            else if (i % 4 === 2) events.push({ type: 'ADDED', resource: r(`u${i}`, { v: 9 }) }); // duplicate, ignored
            else events.push({ type: 'ADDED', resource: r(`new${i}`, { v: 2 }) });
        }
        const result = applyResourceEvents(previous, events);
        expect(result.size).toBe(10000 - 2500 + 2500);
        expect(result.has('u0')).toBe(false);
        expect(result.get('u1').v).toBe(1);
        expect(result.get('u2').v).toBe(0);
        expect(result.get('new3').v).toBe(2);
        expect(previous.size).toBe(10000);
    });
});
