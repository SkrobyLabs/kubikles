import { describe, expect, it, vi } from 'vitest';
import {
    applyColumnFilters,
    compareNames,
    compileColumnFilters,
    computeColumnUniqueValues,
    detectColumnFilterTypes,
    getColumnValue,
    getNumericValue,
    selectColumnHasFilterValues,
    sortItems,
} from './resourceListPipeline';

// The comparator ResourceList used before the sort was hoisted; sortItems must order identically.
function referenceSort(items: any[], columns: any[], sortConfig: { key: string | null; direction: string }) {
    if (!sortConfig.key) return items;
    return [...items].sort((a, b) => {
        const column = columns.find((col: any) => col.key === sortConfig.key);
        if (!column) return 0;
        const getValue = column.getValue || ((i: any) => i[column.key]);
        const aValue = getValue(a);
        const bValue = getValue(b);
        if (aValue < bValue) return sortConfig.direction === 'asc' ? -1 : 1;
        if (aValue > bValue) return sortConfig.direction === 'asc' ? 1 : -1;
        if (sortConfig.key !== 'age') {
            const aDate = a.metadata?.creationTimestamp;
            const bDate = b.metadata?.creationTimestamp;
            if (aDate && bDate && aDate !== bDate) return aDate > bDate ? -1 : 1;
        }
        if (sortConfig.key !== 'name') {
            const aName = a.metadata?.name || '';
            const bName = b.metadata?.name || '';
            return aName.localeCompare(bName);
        }
        return 0;
    });
}

const columns: any[] = [
    { key: 'name', getValue: (i: any) => i.metadata?.name },
    { key: 'age', getValue: (i: any) => i.metadata?.creationTimestamp },
    { key: 'tier', getValue: (i: any) => i.tier },
    { key: 'plain' },
];

// Deterministic pseudo-random data with many ties, mixed case, accents and missing fields.
function makeItems(n: number) {
    let seed = 12345;
    const rnd = () => { seed = (seed * 1103515245 + 12345) & 0x7fffffff; return seed / 0x7fffffff; };
    const names = ['alpha', 'Alpha', 'beta', 'Beta', 'gamma', 'delta', 'a-1', 'a-10', 'a-2', 'zeta', 'Zeta', 'eta', 'ecole', 'école', ''];
    return Array.from({ length: n }, (_, i) => ({
        metadata: {
            name: names[Math.floor(rnd() * names.length)] + (rnd() < 0.5 ? '' : String(Math.floor(rnd() * 5))),
            uid: `u${i}`,
            creationTimestamp: rnd() < 0.1 ? undefined : `2024-01-0${1 + Math.floor(rnd() * 3)}T00:00:00Z`,
        },
        tier: rnd() < 0.1 ? undefined : Math.floor(rnd() * 4),
        plain: rnd() < 0.5 ? 'x' : 'y',
    }));
}

describe('sortItems', () => {
    const items = makeItems(400);
    for (const key of ['name', 'age', 'tier', 'plain']) {
        for (const direction of ['asc', 'desc']) {
            it(`orders exactly like the previous inline comparator (${key} ${direction})`, () => {
                const expected = referenceSort(items, columns, { key, direction });
                expect(sortItems(items, columns, { key, direction })).toEqual(expected);
                // identity, not just equality
                const actual = sortItems(items, columns, { key, direction });
                expected.forEach((row, i) => expect(actual[i]).toBe(row));
            });
        }
    }

    it('calls getValue once per item instead of once per comparison', () => {
        const getValue = vi.fn((i: any) => i.tier);
        sortItems(items, [{ key: 'tier', getValue }], { key: 'tier', direction: 'asc' });
        expect(getValue).toHaveBeenCalledTimes(items.length);
    });

    it('does not mutate its input and returns it untouched without a usable sort key', () => {
        const copy = [...items];
        sortItems(items, columns, { key: 'name', direction: 'asc' });
        expect(items).toEqual(copy);
        expect(sortItems(items, columns, { key: null, direction: 'asc' })).toBe(items);
        expect(sortItems(items, columns, { key: 'missing', direction: 'asc' })).toBe(items);
    });

    it('is stable for full ties', () => {
        const same = Array.from({ length: 50 }, (_, i) => ({ metadata: { name: 'same', uid: `u${i}`, creationTimestamp: 'T' }, tier: 1 }));
        expect(sortItems(same, columns, { key: 'tier', direction: 'asc' })).toEqual(same);
    });

    it('compareNames matches String.localeCompare for the default locale', () => {
        const samples = ['a', 'A', 'b', 'B', 'a-1', 'a-10', 'a-2', 'é', 'e', 'z', 'Z', '', ' ', '10', '9', 'ä', 'ss', 'ß'];
        for (const a of samples) for (const b of samples) {
            expect(Math.sign(compareNames(a, b))).toBe(Math.sign(a.localeCompare(b)));
        }
    });
});

describe('column value helpers', () => {
    it('getColumnValue prefers getFilterValue, then name/namespace, then string getValue, then the field', () => {
        const i = { metadata: { name: 'n', namespace: 'ns' }, phase: 'Up' };
        expect(getColumnValue({ key: 'x', getFilterValue: () => 7 }, i)).toBe('7');
        expect(getColumnValue({ key: 'x', getFilterValue: () => undefined }, i)).toBe('');
        expect(getColumnValue({ key: 'name', getValue: () => 'ignored' }, i)).toBe('n');
        expect(getColumnValue({ key: 'namespace' }, i)).toBe('ns');
        expect(getColumnValue({ key: 'x', getValue: () => 'v' }, i)).toBe('v');
        expect(getColumnValue({ key: 'x', getValue: () => 5 }, i)).toBe('');
        expect(getColumnValue({ key: 'phase' }, i)).toBe('Up');
    });

    it('getNumericValue handles explicit getters, age, numeric getValue and the NaN fallback', () => {
        vi.useFakeTimers({ toFake: ['Date'] });
        vi.setSystemTime(new Date('2024-01-01T10:00:00Z'));
        try {
            expect(getNumericValue({ key: 'x', getNumericValue: () => 3 }, {})).toBe(3);
            expect(getNumericValue({ key: 'age' }, { metadata: { creationTimestamp: '2024-01-01T08:00:00Z' } })).toBe(2);
            expect(getNumericValue({ key: 'age' }, { metadata: {} })).toBeNaN();
            expect(getNumericValue({ key: 'x', getValue: () => 4 }, {})).toBe(4);
            expect(getNumericValue({ key: 'x', getValue: () => '4' }, {})).toBeNaN();
            expect(getNumericValue({ key: 'x' }, {})).toBeNaN();
        } finally {
            vi.useRealTimers();
        }
    });

    it('detectColumnFilterTypes follows the documented precedence', () => {
        expect(detectColumnFilterTypes([
            { key: 'name' }, { key: 'namespace' }, { key: 'age' }, { key: 'status' },
            { key: 'cpu', filterType: 'numeric' }, { key: 'c', filterable: false }, { key: 'sel', isColumnSelector: true },
            { key: 'chk', isSelectionColumn: true }, { key: 'x', filterable: false, filterType: 'regex' },
        ])).toEqual({
            name: 'regex', namespace: 'regex', age: 'numeric', status: 'select', cpu: 'numeric',
            c: false, sel: false, chk: false, x: false,
        });
    });
});

describe('column filters', () => {
    const rows = ['a', 'b', 'ab', 'c'].map((name, i) => ({ metadata: { name, uid: `u${i}` }, n: i }));
    const cols: any[] = [
        { key: 'name' },
        { key: 'n', filterType: 'numeric', getNumericValue: (r: any) => (r.n === 3 ? NaN : r.n) },
    ];

    it('compiles to no predicate when nothing applies and ignores filters on unknown columns', () => {
        expect(compileColumnFilters(cols, {})).toBeNull();
        expect(compileColumnFilters(cols, { ghost: { type: 'select', values: new Set(['x']) } })).toBeNull();
        expect(applyColumnFilters(rows, cols, {})).toBe(rows);
    });

    it('builds each regex once per pass, not once per row', () => {
        const RealRegExp = RegExp;
        let constructed = 0;
        vi.stubGlobal('RegExp', class extends RealRegExp {
            constructor(...args: ConstructorParameters<typeof RegExp>) { super(...args); constructed++; }
        });
        try {
            const out = applyColumnFilters(rows, cols, { name: { type: 'regex', conditions: ['a', 'b'], logic: 'and' } });
            expect(out.map(r => r.metadata.name)).toEqual(['ab']);
            expect(constructed).toBe(2);
        } finally {
            vi.unstubAllGlobals();
        }
    });

    it('treats an invalid regex as matching everything, in AND and OR mode', () => {
        expect(applyColumnFilters(rows, cols, { name: { type: 'regex', conditions: ['('], logic: 'and' } })).toHaveLength(4);
        expect(applyColumnFilters(rows, cols, { name: { type: 'regex', conditions: ['(', 'zzz'], logic: 'or' } })).toHaveLength(4);
        expect(applyColumnFilters(rows, cols, { name: { type: 'regex', conditions: ['(', 'zzz'], logic: 'and' } })).toHaveLength(0);
    });

    it('applies numeric conditions with AND/OR and drops NaN rows', () => {
        const names = (f: any) => applyColumnFilters(rows, cols, { n: f }).map(r => r.metadata.name);
        expect(names({ type: 'numeric', conditions: [{ operator: '>=', value: 1 }, { operator: '<', value: 3 }], logic: 'and' })).toEqual(['b', 'ab']);
        expect(names({ type: 'numeric', conditions: [{ operator: '=', value: 0 }, { operator: '=', value: 2 }], logic: 'or' })).toEqual(['a', 'ab']);
        expect(names({ type: 'numeric', conditions: [{ operator: '??', value: 0 }], logic: 'and' })).toEqual(['a', 'b', 'ab']);
        expect(names({ type: 'numeric', operator: '!=', value: 1 })).toEqual(['a', 'ab']);
    });

    it('select filters match by exact value and an empty selection hides everything', () => {
        expect(applyColumnFilters(rows, cols, { name: { type: 'select', values: new Set(['a', 'c']) } }).map(r => r.metadata.name)).toEqual(['a', 'c']);
        expect(applyColumnFilters(rows, cols, { name: { type: 'select', values: new Set() } })).toEqual([]);
    });

    it('all active filters must pass', () => {
        const out = applyColumnFilters(rows, cols, {
            name: { type: 'regex', conditions: ['a'], logic: 'and' },
            n: { type: 'numeric', conditions: [{ operator: '>', value: 0 }], logic: 'and' },
        });
        expect(out.map(r => r.metadata.name)).toEqual(['ab']);
    });
});

describe('unique values', () => {
    const col = { key: 'status', getFilterValue: (i: any) => i.s };
    const data = (...values: Array<string | undefined>) => values.map((s, i) => ({ metadata: { uid: String(i) }, s }));

    it('lists predefined values first with zero counts, then dynamic values alphabetically', () => {
        const result = computeColumnUniqueValues(col, data('Zed', 'Running', 'Alpha', 'Running'), 'jobs')!;
        expect(result.values).toEqual(['Complete', 'Failed', 'Running', 'Suspended', 'Alpha', 'Zed']);
        expect(result.counts).toMatchObject({ Complete: 0, Running: 2, Alpha: 1, Zed: 1 });
    });

    it('skips empty values and returns undefined without any value', () => {
        expect(computeColumnUniqueValues(col, data(undefined, ''), 'unknown')).toBeUndefined();
    });

    it('returns undefined above 200 distinct values and keeps exactly 200', () => {
        const many = (n: number) => data(...Array.from({ length: n }, (_, i) => `v${i}`));
        expect(computeColumnUniqueValues(col, many(200), null)?.values).toHaveLength(200);
        expect(computeColumnUniqueValues(col, many(201), null)).toBeUndefined();
    });

    it('selectColumnHasFilterValues agrees with computeColumnUniqueValues and stops scanning past the cap', () => {
        for (const rt of [null, 'jobs']) {
            for (const d of [data(), data('a'), data(...Array.from({ length: 200 }, (_, i) => `v${i}`)), data(...Array.from({ length: 201 }, (_, i) => `v${i}`))]) {
                expect(selectColumnHasFilterValues(col, d, rt)).toBe(computeColumnUniqueValues(col, d, rt) !== undefined);
            }
        }
        const getFilterValue = vi.fn((i: any) => i.s);
        selectColumnHasFilterValues({ key: 'status', getFilterValue }, data(...Array.from({ length: 5000 }, (_, i) => `v${i}`)), null);
        expect(getFilterValue.mock.calls.length).toBe(201);
    });
});
