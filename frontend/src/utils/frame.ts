// A frame request that cannot stall. WKWebView (and browsers generally) stop or
// heavily throttle requestAnimationFrame while the window is not the active
// one, is occluded, or the display sleeps. Work that must eventually happen
// (publishing streamed data) races the frame against a timer so it runs at
// most `maxDelayMs` late instead of never.

export const FRAME_FALLBACK_MS = 100;

export type FrameHandle = { cancel: () => void };

export function requestFrameOrTimeout(cb: () => void, maxDelayMs: number = FRAME_FALLBACK_MS): FrameHandle {
    let done = false;
    let rafId: number | null = null;
    const run = () => {
        if (done) return;
        done = true;
        if (rafId !== null && typeof cancelAnimationFrame === 'function') cancelAnimationFrame(rafId);
        clearTimeout(timer);
        cb();
    };
    const timer = setTimeout(run, maxDelayMs);
    if (typeof requestAnimationFrame === 'function') rafId = requestAnimationFrame(run);
    return {
        cancel: () => {
            if (done) return;
            done = true;
            if (rafId !== null && typeof cancelAnimationFrame === 'function') cancelAnimationFrame(rafId);
            clearTimeout(timer);
        },
    };
}
