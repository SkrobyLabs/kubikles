import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { sleep } from './bulkExecutor';

describe('sleep', () => {
    beforeEach(() => {
        vi.useFakeTimers();
    });

    afterEach(() => {
        vi.useRealTimers();
    });

    it('resolves immediately for 0ms', async () => {
        const p = sleep(0);
        await p;
    });

    it('resolves immediately for negative values', async () => {
        const p = sleep(-100);
        await p;
    });

    it('resolves after the specified duration', async () => {
        let resolved = false;
        sleep(1000).then(() => { resolved = true; });

        await vi.advanceTimersByTimeAsync(999);
        expect(resolved).toBe(false);

        await vi.advanceTimersByTimeAsync(1);
        expect(resolved).toBe(true);
    });
});

