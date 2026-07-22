import { describe, expect, it, vi } from 'vitest';
import { startCompletionPolling } from './useCompletionPolling';

describe('startCompletionPolling', () => {
    it('does not start another poll while a slow request is active', async () => {
        vi.useFakeTimers();
        let finish!: () => void;
        const poll = vi.fn(
            () =>
                new Promise<void>((resolve) => {
                    finish = resolve;
                })
        );
        const cancel = startCompletionPolling(poll, 10_000, () => 0.5);

        await vi.advanceTimersByTimeAsync(25_000);
        expect(poll).toHaveBeenCalledTimes(1);
        finish();
        await Promise.resolve();
        await vi.advanceTimersByTimeAsync(9_999);
        expect(poll).toHaveBeenCalledTimes(1);
        await vi.advanceTimersByTimeAsync(1);
        expect(poll).toHaveBeenCalledTimes(2);
        cancel();
        vi.useRealTimers();
    });

    it('marks an active completion stale and schedules nothing after cancellation', async () => {
        vi.useFakeTimers();
        let finish!: () => void;
        let isCurrent!: () => boolean;
        const poll = vi.fn((current: () => boolean) => {
            isCurrent = current;
            return new Promise<void>((resolve) => {
                finish = resolve;
            });
        });
        const cancel = startCompletionPolling(poll, 10_000, () => 0.5);
        await vi.advanceTimersByTimeAsync(10_000);
        cancel();
        expect(isCurrent()).toBe(false);
        finish();
        await vi.advanceTimersByTimeAsync(30_000);
        expect(poll).toHaveBeenCalledTimes(1);
        vi.useRealTimers();
    });

    it('contains poll failures and continues on the next interval', async () => {
        vi.useFakeTimers();
        const errorSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
        const poll = vi.fn().mockRejectedValueOnce(new Error('not found')).mockResolvedValue(undefined);
        const cancel = startCompletionPolling(poll, 10_000, () => 0.5);
        await vi.advanceTimersByTimeAsync(20_000);
        expect(poll).toHaveBeenCalledTimes(2);
        expect(errorSpy).toHaveBeenCalledWith('Background poll failed', expect.any(Error));
        cancel();
        errorSpy.mockRestore();
        vi.useRealTimers();
    });
});
