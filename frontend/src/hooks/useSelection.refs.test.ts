import { beforeEach, describe, expect, it, vi } from 'vitest';

const runner = vi.hoisted(() => ({ slots: [] as any[], index: 0 }));
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
                    slot.value = typeof next === 'function' ? next(slot.value) : next;
                } };
                runner.slots[index] = slot;
            }
            return [runner.slots[index].value, runner.slots[index].set];
        },
    };
});

import { useSelection } from './useSelection';

const hook = () => { runner.index = 0; return useSelection(); };
const ref = (uid: string) => ({ metadata: { uid, name: uid, namespace: 'ns' } });

beforeEach(() => { runner.slots = []; runner.index = 0; });

describe('useSelection ref-based selection (windowed lists)', () => {
    it('keeps the identity of toggled rows for bulk actions', () => {
        let sel = hook();
        sel.toggleRef(ref('a'), 3);
        sel = hook();
        sel.toggleRef(ref('b'), 5);
        sel = hook();
        expect(sel.selectedCount).toBe(2);
        expect(sel.getRefItems().map(r => r.metadata.uid)).toEqual(['a', 'b']);
        expect(sel.getLastIndex()).toBe(5);
        sel.toggleRef(ref('a'), 3);
        sel = hook();
        expect(sel.getRefItems().map(r => r.metadata.uid)).toEqual(['b']);
    });

    it('setRefs replaces the selection (select all) and addRefs extends it (range)', () => {
        let sel = hook();
        sel.setRefs([ref('a'), ref('b'), ref('c')]);
        sel = hook();
        expect(sel.selectedCount).toBe(3);
        sel.addRefs([ref('d')], 9);
        sel = hook();
        expect(sel.selectedCount).toBe(4);
        expect(sel.getLastIndex()).toBe(9);
        expect(sel.getRefItems()).toHaveLength(4);
    });

    it('prunes the uids the backend reports gone, together with their refs', () => {
        let sel = hook();
        sel.setRefs([ref('a'), ref('b')]);
        sel = hook();
        sel.pruneByUids(['a', 'zzz']);
        sel = hook();
        expect([...sel.selectedIds]).toEqual(['b']);
        expect(sel.getRefItems().map(r => r.metadata.uid)).toEqual(['b']);
        const before = sel.selectedIds;
        sel.pruneByUids(['nope']);
        sel = hook();
        expect(sel.selectedIds).toBe(before);
    });

    it('deselectAll clears the refs too', () => {
        let sel = hook();
        sel.setRefs([ref('a')]);
        sel = hook();
        sel.deselectAll();
        sel = hook();
        expect(sel.getRefItems()).toEqual([]);
        expect(sel.getLastIndex()).toBeNull();
    });

    it('leaves the item-array API untouched for legacy lists', () => {
        let sel = hook();
        const items = [{ metadata: { uid: 'x' } }, { metadata: { uid: 'y' } }];
        sel.toggleItem('x', 0, items, false);
        sel = hook();
        expect(sel.getSelectedItems(items)).toEqual([items[0]]);
        expect(sel.getSelectionState(items)).toBe('some');
    });
});
