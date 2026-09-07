import { afterEach, describe, expect, it, vi } from 'vitest';
import { readResourceSnapshot, writeResourceSnapshot } from './resourceSnapshots';
afterEach(() => vi.useRealTimers());
describe('navigation snapshot bounds', () => {
    it('expires snapshots after thirty seconds', () => {
        vi.useFakeTimers();
        writeResourceSnapshot('expiry', new Map([['a', {}]]));
        expect(readResourceSnapshot('expiry')?.size).toBe(1);
        vi.advanceTimersByTime(30000);
        expect(readResourceSnapshot('expiry')).toBeUndefined();
    });
    it('evicts older snapshots and refuses excessively large lists', () => {
        for (let i = 0; i < 9; i++) writeResourceSnapshot(`lru-${i}`, new Map());
        expect(readResourceSnapshot('lru-0')).toBeUndefined();
        expect(readResourceSnapshot('lru-8')).toBeDefined();
        writeResourceSnapshot('huge', new Map(Array.from({ length: 10001 }, (_, i) => [String(i), {}])));
        expect(readResourceSnapshot('huge')).toBeUndefined();
    });
});
