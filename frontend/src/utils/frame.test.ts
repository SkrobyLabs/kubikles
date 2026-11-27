import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { FRAME_FALLBACK_MS, requestFrameOrTimeout } from './frame';

describe('requestFrameOrTimeout', () => {
    let frames: Array<() => void>;
    beforeEach(() => {
        vi.useFakeTimers();
        frames = [];
        vi.stubGlobal('requestAnimationFrame', (cb: () => void) => { frames.push(cb); return frames.length; });
        vi.stubGlobal('cancelAnimationFrame', vi.fn());
    });
    afterEach(() => { vi.useRealTimers(); vi.unstubAllGlobals(); });

    it('runs on the frame when frames fire, and only once', () => {
        const cb = vi.fn();
        requestFrameOrTimeout(cb);
        frames[0]();
        vi.advanceTimersByTime(FRAME_FALLBACK_MS * 2);
        expect(cb).toHaveBeenCalledTimes(1);
    });

    it('runs from the timer when no frame ever fires', () => {
        const cb = vi.fn();
        requestFrameOrTimeout(cb);
        vi.advanceTimersByTime(FRAME_FALLBACK_MS - 1);
        expect(cb).not.toHaveBeenCalled();
        vi.advanceTimersByTime(1);
        expect(cb).toHaveBeenCalledTimes(1);
        frames[0]();
        expect(cb).toHaveBeenCalledTimes(1);
    });

    it('does not run after cancel', () => {
        const cb = vi.fn();
        requestFrameOrTimeout(cb).cancel();
        frames[0]();
        vi.advanceTimersByTime(FRAME_FALLBACK_MS * 2);
        expect(cb).not.toHaveBeenCalled();
    });
});
