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

const entry = (content: string, timestamp = '') => ({ timestamp, content, source: 'initial' });
const RED = '\x1b[31m';
const RESET = '\x1b[0m';

function mount(logs: any[], config: Record<string, any> = {}) {
    let current = logs;
    // Defaults mirror LogViewer: enter-to-search, one line of context before and five after.
    const getSafeConfig = (path: string, fallback: any) => (path in config ? config[path] : fallback);
    let result: any;
    const render = () => {
        for (let i = 0; i < 10; i++) {
            runner.index = 0; runner.effects = [];
            result = useLogSearch({ logs: current, getConfig: () => undefined, getSafeConfig });
            runner.dirty = false;
            runner.effects.forEach(effect => effect());
            if (!runner.dirty) break;
        }
        return result;
    };
    render();
    return {
        get: () => result,
        setLogs: (next: any[]) => { current = next; return render(); },
        search: (term: string) => {
            result.setSearchInput(term);
            render();
            result.handleSearchKeyDown({ key: 'Enter' });
            return render();
        },
        render,
    };
}

beforeEach(() => {
    runner.slots.forEach(slot => slot?.cleanup?.());
    runner.slots = []; runner.index = 0; runner.effects = []; runner.dirty = false;
    (globalThis as any).window = { addEventListener: vi.fn(), removeEventListener: vi.fn() };
});

const sample = [
    entry('server started', '2024-05-01T10:00:01.000000000Z'),
    entry('GET /health 200'),
    entry(`${RED}ERROR${RESET} db connection lost`),
    entry('retrying'),
    entry('retrying again'),
    entry('connected'),
    entry('ERROR: disk full'),
    entry('shutting down'),
];

describe('useLogSearch without a search term', () => {
    it('returns every entry tagged with its original index and no match info', () => {
        const view = mount(sample);
        const { displayLogs, matchCount, matchIndices, searchRegex } = view.get();
        expect(displayLogs).toHaveLength(sample.length);
        expect(displayLogs.map((e: any) => e.originalIndex)).toEqual([0, 1, 2, 3, 4, 5, 6, 7]);
        expect(displayLogs[0]).toEqual({ ...sample[0], originalIndex: 0 });
        expect(displayLogs.some((e: any) => 'isMatch' in e)).toBe(false);
        expect(matchCount).toBe(0);
        expect(matchIndices.size).toBe(0);
        expect(searchRegex).toBeNull();
    });

    it('returns an empty result for no logs', () => {
        const view = mount([]);
        expect(view.get().displayLogs).toEqual([]);
        expect(view.get().matchCount).toBe(0);
    });

    it('does not search until Enter when search-on-enter is active', () => {
        const view = mount(sample);
        view.get().setSearchInput('error');
        view.render();
        expect(view.get().searchTerm).toBe('');
        expect(view.get().matchCount).toBe(0);
        view.get().handleSearchKeyDown({ key: 'a' });
        expect(view.render().searchTerm).toBe('');
        view.get().handleSearchKeyDown({ key: 'Enter' });
        expect(view.render().searchTerm).toBe('error');
    });
});

describe('useLogSearch match highlighting mode', () => {
    it('marks matching entries (case-insensitive) and keeps all lines', () => {
        const result = mount(sample).search('error');
        expect(result.displayLogs).toHaveLength(sample.length);
        expect(result.displayLogs.filter((e: any) => e.isMatch).map((e: any) => e.originalIndex)).toEqual([2, 6]);
        expect(result.matchCount).toBe(2);
        expect([...result.matchIndices]).toEqual([2, 6]);
        expect(result.displayLogs[0].isMatch).toBe(false);
    });

    it('strips ANSI escapes before matching, so a pattern can span a colour change', () => {
        const result = mount(sample).search('ERROR db');
        // "ERROR" + RESET + " db" only matches once escape codes are removed.
        expect([...result.matchIndices]).toEqual([2]);
    });

    it('does not match against ANSI parameter text', () => {
        const result = mount([entry(`${RED}hello${RESET}`)]).search('31m');
        expect(result.matchCount).toBe(0);
    });

    it('does not match against the timestamp', () => {
        const result = mount(sample).search('2024-05-01');
        expect(result.matchCount).toBe(0);
    });

    it('escapes regex metacharacters in literal mode', () => {
        const result = mount([entry('a.b'), entry('axb'), entry('GET /health (200)')]).search('a.b');
        expect([...result.matchIndices]).toEqual([0]);
        expect([...mount([entry('GET /health (200)')]).search('(200)').matchIndices]).toEqual([0]);
    });

    it('treats the term as a regex when enabled and reports invalid patterns', () => {
        const view = mount(sample);
        view.get().setIsRegex(true);
        view.render();
        view.get().setSearchInput('^(retry|conn)');
        view.render();
        view.get().handleSearchKeyDown({ key: 'Enter' });
        const result = view.render();
        expect([...result.matchIndices]).toEqual([3, 4, 5]);

        view.get().setSearchInput('([');
        view.render();
        view.get().handleSearchKeyDown({ key: 'Enter' });
        const broken = view.render();
        expect(broken.searchRegex).toBeNull();
        expect(broken.regexError).not.toBe('');
        // An invalid regex falls back to showing all entries unmarked.
        expect(broken.displayLogs).toHaveLength(sample.length);
        expect(broken.matchCount).toBe(0);
    });

    it('resets global-regex state between entries so consecutive matching lines all match', () => {
        const result = mount([entry('error 1'), entry('error 2'), entry('error 3')]).search('error');
        expect(result.matchCount).toBe(3);
    });

    it('recomputes when the log list grows', () => {
        const view = mount(sample);
        view.search('error');
        const grown = view.setLogs([...sample, entry('error again')]);
        expect(grown.matchCount).toBe(3);
        expect(grown.displayLogs.at(-1)).toMatchObject({ originalIndex: 8, isMatch: true });
    });
});

describe('useLogSearch filter mode', () => {
    const filterConfig = { 'logs.search.filterOnly': true };

    it('includes context lines (1 before, 5 after by default) and inserts skip indicators', () => {
        const result = mount(sample, filterConfig).search('ERROR: disk');
        expect(result.matchCount).toBe(1);
        // Match at 6: one line before and the one remaining line after. No indicator precedes the first window.
        expect(result.displayLogs.map((e: any) => (e.isSkipIndicator ? `skip:${e.skippedCount}` : e.originalIndex)))
            .toEqual([5, 6, 7]);
    });

    it('reports skipped line counts between disjoint windows', () => {
        const logs = Array.from({ length: 30 }, (_, i) => entry(i === 3 || i === 20 ? 'needle' : `filler ${i}`));
        const view = mount(logs, { ...filterConfig, 'logs.search.contextLinesBefore': 1, 'logs.search.contextLinesAfter': 2 });
        const result = view.search('needle');
        const shape = result.displayLogs.map((e: any) => (e.isSkipIndicator ? `skip:${e.skippedCount}` : e.originalIndex));
        expect(shape).toEqual([2, 3, 4, 5, 'skip:13', 19, 20, 21, 22]);
        const skip = result.displayLogs.find((e: any) => e.isSkipIndicator);
        expect(skip.key).toBe('skip-5-19');
        expect(result.displayLogs.filter((e: any) => !e.isSkipIndicator && e.isMatch).map((e: any) => e.originalIndex)).toEqual([3, 20]);
    });

    it('merges overlapping context windows without indicators', () => {
        const logs = Array.from({ length: 12 }, (_, i) => entry(i === 2 || i === 4 ? 'needle' : `filler ${i}`));
        const view = mount(logs, { ...filterConfig, 'logs.search.contextLinesBefore': 1, 'logs.search.contextLinesAfter': 1 });
        const result = view.search('needle');
        expect(result.displayLogs.map((e: any) => e.originalIndex)).toEqual([1, 2, 3, 4, 5]);
        expect(result.displayLogs.some((e: any) => e.isSkipIndicator)).toBe(false);
    });

    it('clamps context at both ends of the log and keeps original indexes for display', () => {
        const logs = [entry('needle first'), entry('x'), entry('y'), entry('needle last')];
        const view = mount(logs, { ...filterConfig, 'logs.search.contextLinesBefore': 10, 'logs.search.contextLinesAfter': 10 });
        const result = view.search('needle');
        expect(result.displayLogs.map((e: any) => e.originalIndex)).toEqual([0, 1, 2, 3]);
    });

    it('yields an empty display when nothing matches', () => {
        const result = mount(sample, filterConfig).search('zzz-nothing');
        expect(result.displayLogs).toEqual([]);
        expect(result.matchCount).toBe(0);
    });
});

describe('useLogSearch scale', () => {
    it('keeps indexes and match positions correct over 20000 lines', () => {
        const logs = Array.from({ length: 20000 }, (_, i) => entry(i % 1000 === 0 ? `\x1b[33mWARN\x1b[0m item ${i}` : `info ${i}`));
        const result = mount(logs).search('warn item');
        expect(result.matchCount).toBe(20);
        expect([...result.matchIndices].slice(0, 3)).toEqual([0, 1000, 2000]);
        expect(result.displayLogs).toHaveLength(20000);
        expect(result.displayLogs[19999].originalIndex).toBe(19999);
        expect(result.displayLogs[1000]).toMatchObject({ isMatch: true, originalIndex: 1000 });
    });
});
