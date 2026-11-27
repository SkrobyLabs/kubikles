import { beforeEach, describe, expect, it, vi } from 'vitest';

const runner = vi.hoisted(() => ({ slots: [] as any[], index: 0, effects: [] as Array<() => void>, dirty: false }));
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

import { useLogSearch } from './useLogSearch';
import * as logUtils from './logUtils';

const entry = (content: string) => ({ timestamp: '', content, source: 'stream' });

function mount(initial: any[], firstId = 500) {
    const state = { logs: initial, count: initial.length, firstId };
    const getSafeConfig = (_p: string, fallback: any) => fallback;
    let result: any;
    const render = () => {
        for (let i = 0; i < 10; i++) {
            runner.index = 0; runner.effects = [];
            result = useLogSearch({ logs: state.logs, logCount: state.count, firstId: state.firstId, getConfig: () => undefined, getSafeConfig });
            runner.dirty = false;
            runner.effects.forEach(effect => effect());
            if (!runner.dirty) break;
        }
        return result;
    };
    render();
    const rows = () => Array.from({ length: result.displayCount }, (_, i) => result.getRow(i));
    return {
        state, render, get: () => result, rows,
        search(term: string) { result.setSearchInput(term); render(); result.handleSearchKeyDown({ key: 'Enter' }); return render(); },
        append(items: any[]) { state.logs.push(...items); state.count = state.logs.length; return render(); },
    };
}

beforeEach(() => {
    runner.slots.forEach(slot => slot?.cleanup?.());
    runner.slots = []; runner.index = 0; runner.effects = []; runner.dirty = false;
    (globalThis as any).window = { addEventListener: vi.fn(), removeEventListener: vi.fn() };
});

describe('useLogSearch with an in-place growing buffer', () => {
    it('serves raw entries and stable ids without deriving per-entry copies', () => {
        const logs = [entry('a'), entry('b'), entry('c')];
        const view = mount(logs);
        const rows = view.rows();
        expect(rows.map((r: any) => r.originalIndex)).toEqual([500, 501, 502]);
        // The entry object is the original, so entry-keyed caches and React.memo keep working.
        expect(rows[1].entry).toBe(logs[1]);
        view.append([entry('d')]);
        expect(view.get().displayCount).toBe(4);
        expect(view.get().getRow(3).originalIndex).toBe(503);
        expect(view.get().getRow(4)).toBeNull();
    });

    it('scans only appended lines when the term is unchanged', () => {
        const logs = Array.from({ length: 200 }, (_, i) => entry(i % 50 === 0 ? `\x1b[31mhit\x1b[0m ${i}` : `plain ${i}`));
        const view = mount(logs);
        view.search('hit');
        expect(view.get().matchCount).toBe(4);
        const strip = vi.spyOn(logUtils, 'stripAnsiCodes');
        view.append([entry('another hit'), entry('nothing')]);
        expect(view.get().matchCount).toBe(5);
        expect(view.get().getRow(200)).toMatchObject({ originalIndex: 700, isMatch: true });
        expect(view.get().getRow(201)).toMatchObject({ isMatch: false });
        // Only the two new lines were examined (neither contains an escape, so no stripping at all).
        expect(strip).not.toHaveBeenCalled();
        strip.mockRestore();
    });

    it('rescans fully when the head is trimmed (new array and first id)', () => {
        const view = mount([entry('hit 0'), entry('x'), entry('hit 2')]);
        view.search('hit');
        expect([...view.get().matchIndices]).toEqual([0, 2]);
        view.state.logs = [entry('x'), entry('hit 2'), entry('y')];
        view.state.count = 3;
        view.state.firstId = 501;
        const result = view.render();
        expect([...result.matchIndices]).toEqual([1]);
        expect(result.getRow(1)).toMatchObject({ originalIndex: 502, isMatch: true });
    });

    it('rebuilds filter windows incrementally, letting new context reach back across old lines', () => {
        const view = mount(Array.from({ length: 10 }, (_, i) => entry(i === 2 ? 'needle' : `f${i}`)));
        view.get().setFilterOnly(true);
        view.render();
        view.search('needle');
        const before = view.get().displayCount;
        view.append([entry('f10'), entry('needle again')]);
        const ids = view.rows().map((r: any) => (r.isSkipIndicator ? `skip:${r.skippedCount}` : r.originalIndex));
        // First window: 1 before + 5 after the first match, then a skip, then the new match with 1 line before.
        expect(ids.slice(0, before)).toEqual([501, 502, 503, 504, 505, 506, 507]);
        expect(ids.slice(before)).toEqual(['skip:2', 510, 511]);
    });

    it('does not reuse matches when the regex options change', () => {
        const view = mount([entry('Hit'), entry('hit'), entry('h.t')]);
        view.search('h.t');
        expect(view.get().matchCount).toBe(1);
        view.get().setIsRegex(true);
        // Same term, now a regex: "h.t" matches all three, so cached literal matches must not be reused.
        expect(view.render().matchCount).toBe(3);
        view.get().setIsRegex(false);
        expect(view.render().matchCount).toBe(1);
    });
});
