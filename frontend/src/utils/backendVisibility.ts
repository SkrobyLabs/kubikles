import { getWindowVisibleSnapshot, subscribeWindowVisibility } from './windowVisibility';

/**
 * Forwards window visibility to the Go backend (App.SetWindowVisible) so its
 * coalescers drop to a slow cadence while nobody can see the UI.
 *
 * Desktop only. In headless server mode the backend has no per-client notion:
 * one hidden browser tab would slow updates for every other connected viewer,
 * so nothing is reported there.
 *
 * Hiding is debounced so a quick alt-tab or tab flick does not flap the
 * backend. Showing is sent immediately: the user must see fresh data at once.
 */

export const HIDE_DEBOUNCE_MS = 750;

export interface BackendVisibilityDeps {
    send: (visible: boolean) => unknown;
    getSnapshot: () => boolean;
    subscribe: (cb: (visible: boolean) => void) => () => void;
    debounceMs?: number;
    setTimer?: (fn: () => void, ms: number) => unknown;
    clearTimer?: (h: unknown) => void;
}

export function startBackendVisibilitySync(deps: BackendVisibilityDeps): () => void {
    const setT = deps.setTimer ?? ((fn, ms) => setTimeout(fn, ms));
    const clearT = deps.clearTimer ?? ((h) => clearTimeout(h as ReturnType<typeof setTimeout>));
    const delay = deps.debounceMs ?? HIDE_DEBOUNCE_MS;

    let reported = true; // backend starts in the visible state
    let pending: unknown = null;

    const report = (visible: boolean) => {
        if (visible === reported) return;
        reported = visible;
        try {
            const r = deps.send(visible);
            if (r && typeof (r as Promise<unknown>).catch === 'function') {
                (r as Promise<unknown>).catch(() => {});
            }
        } catch {
            // backend not ready; the next change retries
        }
    };

    const onChange = (visible: boolean) => {
        if (pending !== null) {
            clearT(pending);
            pending = null;
        }
        if (visible) {
            report(true);
            return;
        }
        pending = setT(() => {
            pending = null;
            report(false);
        }, delay);
    };

    const unsubscribe = deps.subscribe(onChange);
    if (!deps.getSnapshot()) onChange(false);

    return () => {
        unsubscribe();
        if (pending !== null) clearT(pending);
        pending = null;
    };
}

/** True when running inside the Wails desktop shell. */
export function isDesktopRuntime(): boolean {
    return typeof window !== 'undefined' && typeof (window as any).go !== 'undefined';
}

export function startDefaultBackendVisibilitySync(): () => void {
    if (!isDesktopRuntime()) return () => {};
    return startBackendVisibilitySync({
        send: (v) => import('~/lib/wailsjs-adapter/go/main/App').then((m: any) => m.SetWindowVisible(v)),
        getSnapshot: getWindowVisibleSnapshot,
        subscribe: subscribeWindowVisibility as (cb: (visible: boolean) => void) => () => void,
    });
}
