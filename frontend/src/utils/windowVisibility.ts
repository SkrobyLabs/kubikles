import { useSyncExternalStore } from 'react';

/**
 * Shared window-visibility source. "Visible" means the document is not hidden
 * (minimised, other desktop, background tab). Window blur alone is deliberately
 * NOT treated as hidden: a user looking at the app while typing in another
 * window still expects live data. Blur/focus are only used as extra wake-up
 * hints because WKWebView does not always fire visibilitychange on minimise.
 */

type Listener = (visible: boolean) => void;

const listeners = new Set<Listener>();
let attached = false;
let lastVisible = true;

export function getWindowVisibleSnapshot(): boolean {
    if (typeof document === 'undefined') return true;
    return document.visibilityState !== 'hidden';
}

function emitIfChanged(): void {
    const now = getWindowVisibleSnapshot();
    if (now === lastVisible) return;
    lastVisible = now;
    for (const l of [...listeners]) l(now);
}

function attach(): void {
    if (attached || typeof document === 'undefined') return;
    attached = true;
    lastVisible = getWindowVisibleSnapshot();
    document.addEventListener('visibilitychange', emitIfChanged);
    if (typeof window !== 'undefined') {
        window.addEventListener('focus', emitIfChanged);
        window.addEventListener('blur', emitIfChanged);
        window.addEventListener('pageshow', emitIfChanged);
        window.addEventListener('pagehide', emitIfChanged);
    }
}

function detach(): void {
    if (!attached || typeof document === 'undefined') return;
    attached = false;
    document.removeEventListener('visibilitychange', emitIfChanged);
    if (typeof window !== 'undefined') {
        window.removeEventListener('focus', emitIfChanged);
        window.removeEventListener('blur', emitIfChanged);
        window.removeEventListener('pageshow', emitIfChanged);
        window.removeEventListener('pagehide', emitIfChanged);
    }
}

/**
 * subscribe/getSnapshot pair (useSyncExternalStore compatible). The callback
 * form takes no args so it can be passed straight to useSyncExternalStore; a
 * later integration step can also pass a callback and read the snapshot, e.g.
 * to forward the state to the Go backend.
 */
export function subscribeWindowVisibility(onChange: () => void): () => void;
export function subscribeWindowVisibility(onChange: (visible: boolean) => void): () => void;
export function subscribeWindowVisibility(onChange: Listener): () => void {
    attach();
    listeners.add(onChange);
    return () => {
        listeners.delete(onChange);
        if (listeners.size === 0) detach();
    };
}

/** React hook: true while the window is visible. */
export function useWindowVisible(): boolean {
    return useSyncExternalStore(subscribeWindowVisibility as (cb: () => void) => () => void, getWindowVisibleSnapshot, () => true);
}

/**
 * Wrap a periodic job so ticks are skipped while hidden and one catch-up run
 * happens when the window becomes visible again (only if a tick was missed).
 * Returns the gated tick function and a dispose function. The caller owns the
 * timer; this only gates and catches up.
 */
export function createVisibilityGate(run: () => void | Promise<void>): { tick: () => void; dispose: () => void } {
    let missed = false;
    const unsubscribe = subscribeWindowVisibility((visible: boolean) => {
        if (visible && missed) {
            missed = false;
            void run();
        }
    });
    return {
        tick: () => {
            if (!getWindowVisibleSnapshot()) {
                missed = true;
                return;
            }
            void run();
        },
        dispose: unsubscribe,
    };
}

/**
 * Interval that only runs while the window is visible: fires once immediately
 * (when visible; when hidden, once on becoming visible), then every
 * `intervalMs`, skipping hidden ticks and catching up once on show. A null
 * interval means a single gated run with no timer. Returns a stop function.
 */
export function startVisibleInterval(run: () => void | Promise<void>, intervalMs: number | null): () => void {
    const gate = createVisibilityGate(run);
    gate.tick();
    const timer = intervalMs === null ? null : setInterval(gate.tick, intervalMs);
    return () => {
        if (timer !== null) clearInterval(timer);
        gate.dispose();
    };
}
