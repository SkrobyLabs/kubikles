import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { sleep, executeBulkOperations } from './bulkExecutor';

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

describe('executeBulkOperations', () => {
    beforeEach(() => {
        vi.useFakeTimers();
    });

    afterEach(() => {
        vi.useRealTimers();
    });

    it('executes all items with delayMs=0 (no setTimeout)', async () => {
        const items = ['a', 'b', 'c'];
        const calls: string[] = [];
        const operation = vi.fn(async (item: string) => { calls.push(item); });

        const promise = executeBulkOperations(items, operation, { delayMs: 0 });
        const results = await promise;

        expect(calls).toEqual(['a', 'b', 'c']);
        expect(results).toHaveLength(3);
        expect(results.every(r => r.success)).toBe(true);
    });

    it('applies delay between items', async () => {
        const items = ['a', 'b', 'c'];
        const calls: string[] = [];
        const operation = vi.fn(async (item: string) => { calls.push(item); });

        const promise = executeBulkOperations(items, operation, { delayMs: 500 });

        // First item executes immediately
        await vi.advanceTimersByTimeAsync(0);
        expect(calls).toEqual(['a']);

        // After 500ms delay, second item executes
        await vi.advanceTimersByTimeAsync(500);
        expect(calls).toEqual(['a', 'b']);

        // After another 500ms delay, third item executes (no delay after last)
        await vi.advanceTimersByTimeAsync(500);
        expect(calls).toEqual(['a', 'b', 'c']);

        const results = await promise;
        expect(results).toHaveLength(3);
    });

    it('does not delay for single-item operations', async () => {
        const operation = vi.fn(async () => {});
        const setTimeoutSpy = vi.spyOn(globalThis, 'setTimeout');
        const callsBefore = setTimeoutSpy.mock.calls.length;

        const promise = executeBulkOperations(['only'], operation, { delayMs: 2000 });
        const results = await promise;

        // setTimeout should not have been called for delay (sleep returns immediately for last item)
        const callsAfter = setTimeoutSpy.mock.calls.length;
        expect(callsAfter - callsBefore).toBe(0);
        expect(results).toHaveLength(1);

        setTimeoutSpy.mockRestore();
    });

    it('reports progress correctly', async () => {
        const items = ['a', 'b', 'c'];
        const operation = vi.fn(async () => {});
        const progressCalls: [number, number, string, boolean][] = [];
        const onProgress = vi.fn((current, total, item, success) => {
            progressCalls.push([current, total, item, success]);
        });

        const promise = executeBulkOperations(items, operation, { delayMs: 0, onProgress });
        await promise;

        expect(progressCalls).toEqual([
            [1, 3, 'a', true],
            [2, 3, 'b', true],
            [3, 3, 'c', true],
        ]);
    });

    it('continues on operation failure', async () => {
        const items = ['ok1', 'fail', 'ok2'];
        const operation = vi.fn(async (item: string) => {
            if (item === 'fail') throw new Error('boom');
        });

        const promise = executeBulkOperations(items, operation, { delayMs: 0 });
        const results = await promise;

        expect(results).toHaveLength(3);
        expect(results[0]).toEqual({ item: 'ok1', success: true });
        expect(results[1]).toEqual({ item: 'fail', success: false, error: new Error('boom') });
        expect(results[2]).toEqual({ item: 'ok2', success: true });
    });

    it('returns empty array for empty items', async () => {
        const operation = vi.fn(async () => {});
        const results = await executeBulkOperations([], operation, { delayMs: 100 });
        expect(results).toEqual([]);
        expect(operation).not.toHaveBeenCalled();
    });
});
