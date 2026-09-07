import { describe, expect, it, vi } from 'vitest';
import { chooseNamespaceScope, listNamespaceScope, namespaceSelection, observeNamespaceList } from './namespaceQuery';

const namespaces = (count: number) => Array.from({ length: count }, (_, i) => `ns-${i}`);
const observed = (overrides = {}) => ({ count: 10, bytes: 1000, durationMs: 100, observedAt: Date.now(), ...overrides });

describe('namespace query planning', () => {
    it('preserves empty, explicit and future-inclusive selections', () => {
        expect(namespaceSelection([])).toEqual([]);
        expect(namespaceSelection(['b', 'a', 'a'])).toEqual(['a', 'b']);
        expect(namespaceSelection('')).toEqual(['*']);
        expect(chooseNamespaceScope(['*'], [], undefined, { broadDenied: true })).toEqual(['']);
        expect(chooseNamespaceScope([], [], observed())).toEqual([]);
    });
    it('starts broad at four namespaces and 50% coverage', () => {
        expect(chooseNamespaceScope(namespaces(4), namespaces(8))).toEqual(['']);
        expect(chooseNamespaceScope(namespaces(3), namespaces(4))).toEqual(namespaces(3));
        expect(chooseNamespaceScope(namespaces(4), namespaces(9))).toEqual(namespaces(4));
    });
    it('uses a measured sparse resource list even with 1000 namespaces', () => {
        expect(chooseNamespaceScope(namespaces(2), namespaces(1000), observed())).toEqual(['']);
        expect(chooseNamespaceScope(namespaces(2), namespaces(1000))).toEqual(namespaces(2));
    });
    it('reuses a small broad scope for a single namespace', () => {
        expect(chooseNamespaceScope(namespaces(1), namespaces(1000), observed(), { reuseBroad: true })).toEqual(['']);
        expect(chooseNamespaceScope(namespaces(1), namespaces(1000), observed())).toEqual(namespaces(1));
    });
    it.each([{ count: 6000 }, { bytes: 21 * 1024 * 1024 }, { durationMs: 3000 }])('avoids expensive broad lists: %j', overrides => {
        expect(chooseNamespaceScope(namespaces(8), namespaces(10), observed(overrides))).toEqual(namespaces(8));
    });
    it('requires 80% coverage and favorable latency for larger measured lists', () => {
        const observation = observed({ count: 1000, durationMs: 800 });
        expect(chooseNamespaceScope(namespaces(8), namespaces(10), observation, { scopedDurationMs: 500 })).toEqual(['']);
        expect(chooseNamespaceScope(namespaces(8), namespaces(10), observation, { scopedDurationMs: 100 })).toEqual(namespaces(8));
        expect(chooseNamespaceScope(namespaces(7), namespaces(10), observation)).toEqual(namespaces(7));
    });
    it('does not reuse stale measurements or retry denied broad access', () => {
        expect(chooseNamespaceScope(namespaces(2), namespaces(1000), observed({ observedAt: 0 }))).toEqual(namespaces(2));
        expect(chooseNamespaceScope(namespaces(8), namespaces(10), observed(), { broadDenied: true })).toEqual(namespaces(8));
    });
    it('measures serialized payload size as well as count', () => {
        const data = [{ value: 'é' }];
        expect(observeNamespaceList(data, 50)).toMatchObject({ count: 1, bytes: new TextEncoder().encode(JSON.stringify(data)).length, durationMs: 50 });
    });
});

describe('namespace list batching', () => {
    it('bounds concurrent requests to four and collects all namespaces', async () => {
        let active = 0;
        let peak = 0;
        const results = await listNamespaceScope(namespaces(10), async ns => {
            peak = Math.max(peak, ++active);
            await Promise.resolve();
            active--;
            return [ns];
        });
        expect(peak).toBe(4);
        expect(results).toEqual(namespaces(10));
    });
    it('fails atomically and stops queued work after a failure', async () => {
        const list = vi.fn(async () => { throw new Error('unavailable'); });
        await expect(listNamespaceScope(namespaces(10), list)).rejects.toThrow('unavailable');
        expect(list).toHaveBeenCalledTimes(4);
    });
    it('never starts queued work after cancellation', async () => {
        let current = true;
        const list = vi.fn(async () => { current = false; return []; });
        await listNamespaceScope(namespaces(10), list, () => current);
        expect(list).toHaveBeenCalledTimes(1);
    });
});

it('bounds payload measurement work on large lists', () => {
    const serialize = vi.fn(() => ({ value: 'large' }));
    const items = Array.from({ length: 10000 }, () => ({ toJSON: serialize }));
    const result = observeNamespaceList(items, 10);
    expect(serialize).toHaveBeenCalledTimes(32);
    expect(result.count).toBe(10000);
    expect(result.bytes).toBeGreaterThan(10000);
});
