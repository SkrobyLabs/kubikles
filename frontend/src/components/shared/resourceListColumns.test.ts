import { describe, it, expect } from 'vitest';
import { calculateTextWidth, calculateColumnWidths, MIN_COLUMN_WIDTHS } from './resourceListColumns';

const CHAR = 7.5, PAD = 24;

describe('calculateTextWidth', () => {
    it('returns 0 for empty/nullish text', () => {
        expect(calculateTextWidth('')).toBe(0);
        expect(calculateTextWidth(null)).toBe(0);
        expect(calculateTextWidth(undefined)).toBe(0);
    });
    it('scales with string length plus cell padding', () => {
        expect(calculateTextWidth('abcd')).toBe(Math.ceil(4 * CHAR) + PAD);
    });
});

describe('calculateColumnWidths', () => {
    const rows = (names: string[]) => names.map(n => ({ metadata: { name: n } }));

    it('returns empty object for no data', () => {
        expect(calculateColumnWidths([{ key: 'name' }], [], {})).toEqual({});
    });

    it('skips selection, column-selector, and fixed-width columns', () => {
        const cols = [
            { key: '_selection', isSelectionColumn: true },
            { key: '_cols', isColumnSelector: true },
            { key: 'cpu' },
            { key: 'memory' },
            { key: 'pods' },
        ];
        expect(calculateColumnWidths(cols as any, rows(['a', 'b']), {})).toEqual({});
    });

    it('does not recompute columns that already have a saved width', () => {
        const result = calculateColumnWidths([{ key: 'name' }] as any, rows(['abc']), { name: 999 });
        expect(result.name).toBeUndefined();
    });

    it('never returns less than the per-column minimum width', () => {
        const result = calculateColumnWidths([{ key: 'name' }] as any, rows(['x']), {});
        expect(result.name).toBeGreaterThanOrEqual(MIN_COLUMN_WIDTHS.name);
    });

    it('caps very wide content (350 default, 500 for message)', () => {
        const long = 'x'.repeat(300);
        const general = calculateColumnWidths([{ key: 'reason', getValue: (i: any) => i.v }] as any,
            [{ v: long }], {});
        expect(general.reason).toBeLessThanOrEqual(350);
        const message = calculateColumnWidths([{ key: 'message', getValue: (i: any) => i.v }] as any,
            [{ v: long }], {});
        expect(message.message).toBeLessThanOrEqual(500);
        expect(message.message).toBeGreaterThan(350);
    });
});
