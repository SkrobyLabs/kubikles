import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { createDedupedSizeSender, createFitScheduler } from './terminalResize';

describe('createDedupedSizeSender', () => {
    it('sends only when the size changes and ignores non-positive sizes', () => {
        const send = vi.fn();
        const s = createDedupedSizeSender(send);
        expect(s.send(80, 24)).toBe(true);
        expect(s.send(80, 24)).toBe(false);
        expect(s.send(100, 24)).toBe(true);
        expect(s.send(0, 24)).toBe(false);
        expect(send).toHaveBeenCalledTimes(2);
        s.reset();
        expect(s.send(100, 24)).toBe(true);
        expect(send).toHaveBeenCalledTimes(3);
    });
});

describe('createFitScheduler', () => {
    beforeEach(() => { vi.useFakeTimers(); });
    afterEach(() => { vi.useRealTimers(); });

    it('fits immediately then once more after a burst settles', () => {
        const fit = vi.fn();
        const s = createFitScheduler(fit, () => true, 100, () => Date.now());
        for (let i = 0; i < 20; i++) {
            s.request();
            vi.advanceTimersByTime(10);
        }
        expect(fit).toHaveBeenCalledTimes(1);
        vi.advanceTimersByTime(100);
        expect(fit).toHaveBeenCalledTimes(2);
        vi.advanceTimersByTime(1000);
        expect(fit).toHaveBeenCalledTimes(2);
    });

    it('skips fits while the terminal cannot be fitted', () => {
        const fit = vi.fn();
        let ok = false;
        const s = createFitScheduler(fit, () => ok, 100, () => Date.now());
        s.request();
        vi.advanceTimersByTime(200);
        expect(fit).not.toHaveBeenCalled();
        ok = true;
        s.request();
        expect(fit).toHaveBeenCalledTimes(1);
    });

    it('cancel drops the trailing fit', () => {
        const fit = vi.fn();
        const s = createFitScheduler(fit, () => true, 100, () => Date.now());
        s.request();
        s.cancel();
        vi.advanceTimersByTime(500);
        expect(fit).toHaveBeenCalledTimes(1);
    });
});
