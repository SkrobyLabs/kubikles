import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { isListCovered, isListShown, watchListReady } from './listReady';

describe('isListCovered', () => {
    it('is not covered once the current list is ready', () => {
        expect(isListCovered('live', 'live', 100)).toBe(false);
    });

    it('covers the pane from the moment the list identity changes until it is ready', () => {
        expect(isListCovered('filtered', 'live', 100)).toBe(true);
        expect(isListCovered('live', 'filtered', 100)).toBe(true);
    });

    it('never covers a list with no rows, which would never become visible', () => {
        expect(isListCovered('filtered', 'live', 0)).toBe(false);
    });
});

describe('isListShown', () => {
    const container = (list: any) => ({ querySelector: () => list }) as any;
    it('is false while the item list is hidden or empty, true once it shows rows', () => {
        expect(isListShown(null)).toBe(false);
        expect(isListShown(container(null))).toBe(false);
        expect(isListShown(container({ style: { visibility: 'hidden' }, childElementCount: 5 }))).toBe(false);
        expect(isListShown(container({ style: { visibility: 'visible' }, childElementCount: 0 }))).toBe(false);
        expect(isListShown(container({ style: { visibility: 'visible' }, childElementCount: 5 }))).toBe(true);
        expect(isListShown(container({ style: { visibility: '' }, childElementCount: 5 }))).toBe(true);
    });
});

describe('watchListReady', () => {
    beforeEach(() => { vi.useFakeTimers(); });
    afterEach(() => { vi.useRealTimers(); });

    it('keeps polling while the list is hidden and reports ready the moment it shows', () => {
        let shown = false;
        const ready = vi.fn();
        watchListReady(() => shown, ready);
        vi.advanceTimersByTime(300);
        expect(ready).not.toHaveBeenCalled();
        shown = true;
        vi.advanceTimersByTime(60);
        expect(ready).toHaveBeenCalledTimes(1);
        vi.advanceTimersByTime(1000);
        expect(ready).toHaveBeenCalledTimes(1);
    });

    it('gives up after the cap so a cover cannot stay up for good', () => {
        const ready = vi.fn();
        watchListReady(() => false, ready, 1000, () => Date.now());
        vi.advanceTimersByTime(900);
        expect(ready).not.toHaveBeenCalled();
        vi.advanceTimersByTime(300);
        expect(ready).toHaveBeenCalledTimes(1);
    });

    it('stops when cancelled', () => {
        const ready = vi.fn();
        const cancel = watchListReady(() => false, ready, 1000);
        cancel();
        vi.advanceTimersByTime(5000);
        expect(ready).not.toHaveBeenCalled();
    });
});
