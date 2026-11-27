import { beforeEach, describe, expect, it, vi } from 'vitest';

// A minimal hook runner: state slots, effects with cleanups and deps.
const runner = vi.hoisted(() => ({ slots: [] as any[], index: 0, effects: [] as Array<() => void>, rerender: () => {} }));
vi.mock('react', () => ({
    useCallback: (fn: any) => fn,
    useState: (initial: any) => {
        const i = runner.index++;
        if (!runner.slots[i]) runner.slots[i] = { value: initial };
        const slot = runner.slots[i];
        return [slot.value, (next: any) => { slot.value = typeof next === 'function' ? next(slot.value) : next; runner.rerender(); }];
    },
    useEffect: (effect: () => any, deps: any[]) => {
        const i = runner.index++;
        const old = runner.slots[i];
        if (!old || deps.some((d, k) => !Object.is(d, old.deps[k]))) {
            const slot = { deps, cleanup: undefined as any };
            runner.slots[i] = slot;
            runner.effects.push(() => { old?.cleanup?.(); slot.cleanup = effect(); });
        }
    },
}));
vi.mock('../context', () => ({ useNotification: () => ({ addNotification: vi.fn() }) }));

import { fetchAllViewObjects, useAllViewObjects } from './useViewObject';

const flush = async () => { for (let i = 0; i < 8; i++) await Promise.resolve(); };
const node = (name: string) => ({ metadata: { name, uid: `u-${name}` } });

function fakeView(objects: any[]) {
    return {
        selectAllRefs: vi.fn(async () => objects.map(o => ({ uid: o.metadata.uid, ns: '', name: o.metadata.name }))),
        getObjects: vi.fn(async (refs: any[]) => refs.map(r => objects.find(o => o.metadata.uid === r.uid))),
        setRange: vi.fn(),
    };
}

function mount(view: any, snapshot: any, enabled: boolean) {
    const args = { view, snapshot, enabled };
    let result: any;
    const render = () => {
        runner.index = 0; runner.effects = [];
        result = useAllViewObjects(args.view, args.snapshot, args.enabled);
        runner.effects.forEach(e => e());
        return result;
    };
    runner.rerender = () => { render(); };
    render();
    return { get: () => result, update: (patch: Partial<typeof args>) => { Object.assign(args, patch); render(); } };
}

beforeEach(() => { runner.slots = []; runner.index = 0; runner.effects = []; });

describe('fetchAllViewObjects', () => {
    it('returns every object of the view ordered by name, like a list response', async () => {
        const view = fakeView([node('b'), node('a'), node('c')]);
        expect((await fetchAllViewObjects(view as any)).map(o => o.metadata.name)).toEqual(['a', 'b', 'c']);
    });
    it('does not ask for objects when the view is empty', async () => {
        const view = fakeView([]);
        expect(await fetchAllViewObjects(view as any)).toEqual([]);
        expect(view.getObjects).not.toHaveBeenCalled();
    });
});

describe('useAllViewObjects', () => {
    const snap = (over: any = {}) => ({ status: 'open', filtered: 2, state: { version: 1 }, ...over });

    it('opens a window over the whole scope and loads the objects once the view is open', async () => {
        const view = fakeView([node('b'), node('a')]);
        const m = mount(view, snap(), true);
        expect(view.setRange).toHaveBeenCalledWith(0, 2);
        expect(m.get().loading).toBe(true);
        await flush();
        expect(m.get().loading).toBe(false);
        expect(m.get().objects.map((o: any) => o.metadata.name)).toEqual(['a', 'b']);
    });

    it('re-reads the objects when the view reports a new version and widens the window with the scope', async () => {
        const view = fakeView([node('a')]);
        const m = mount(view, snap({ filtered: 1 }), true);
        await flush();
        expect(view.selectAllRefs).toHaveBeenCalledTimes(1);
        view.getObjects.mockImplementation(async () => [node('a'), node('z')]);
        view.selectAllRefs.mockImplementation(async () => [{ uid: 'u-a', ns: '', name: 'a' }, { uid: 'u-z', ns: '', name: 'z' }]);
        m.update({ snapshot: snap({ filtered: 2, state: { version: 2 } }) });
        await flush();
        expect(view.setRange).toHaveBeenLastCalledWith(0, 2);
        expect(view.selectAllRefs).toHaveBeenCalledTimes(2);
        expect(m.get().objects.map((o: any) => o.metadata.name)).toEqual(['a', 'z']);
    });

    it('does nothing while disabled, without a view or before the view is open', async () => {
        const view = fakeView([node('a')]);
        mount(view, snap(), false);
        mount(null, snap(), true);
        mount(view, snap({ status: 'opening' }), true);
        await flush();
        expect(view.setRange).not.toHaveBeenCalled();
        expect(view.selectAllRefs).not.toHaveBeenCalled();
    });

    it('ignores a result that arrives after the view changed', async () => {
        let release: (v: any[]) => void = () => {};
        const view = fakeView([node('old')]);
        view.selectAllRefs.mockImplementationOnce(() => new Promise(resolve => { release = resolve; }));
        const m = mount(view, snap({ filtered: 1 }), true);
        m.update({ snapshot: snap({ filtered: 1, state: { version: 2 } }) });
        view.getObjects.mockImplementation(async () => [node('new')]);
        view.selectAllRefs.mockImplementation(async () => [{ uid: 'u-new', ns: '', name: 'new' }]);
        m.update({ snapshot: snap({ filtered: 1, state: { version: 3 } }) });
        release([{ uid: 'u-old', ns: '', name: 'old' }]);
        await flush();
        expect(m.get().objects.map((o: any) => o.metadata.name)).toEqual(['new']);
    });
});
