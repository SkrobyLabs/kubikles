import { describe, expect, it } from 'vitest';
import { createPollingBudget } from './pollingBudget';

describe('pollingBudget', () => {
    it('caps concurrent operations and releases queued work', async () => {
        const run = createPollingBudget(2);
        let active = 0;
        let peak = 0;
        const releases: Array<(() => void) | undefined> = [];
        const operations = Array.from({ length: 5 }, (_, index) => run(async () => {
            active += 1;
            peak = Math.max(peak, active);
            await new Promise<void>(resolve => { releases[index] = resolve; });
            active -= 1;
            return index;
        }));
        await Promise.resolve();
        await Promise.resolve();
        expect(active).toBe(2);
        for (let index = 0; index < 5; index += 1) {
            releases[index]?.();
            await Promise.resolve();
            await Promise.resolve();
        }
        await expect(Promise.all(operations)).resolves.toEqual([0, 1, 2, 3, 4]);
        expect(peak).toBe(2);
    });
});
