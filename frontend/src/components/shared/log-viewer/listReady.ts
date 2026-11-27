import { requestFrameOrTimeout } from '../../../utils/frame';

/**
 * The log list is a virtualised scroller that stays hidden until it has measured its first rows. A new
 * list identity (the plain live list becoming the filtered search list and back) remounts it, which
 * left the pane empty until that measuring finished. The viewer covers the pane with a loading state
 * for exactly that window; this decides when the cover is up and when it can come down.
 */
export function isListCovered(listKey: string, readyKey: string, rowCount: number): boolean {
    // Nothing to measure: an empty list never becomes visible, so it must not stay covered.
    if (rowCount === 0) return false;
    return listKey !== readyKey;
}

/** True once the scroller inside `container` shows its rows (Virtuoso toggles inline visibility on the item list). */
export function isListShown(container: Element | null): boolean {
    const list = container?.querySelector('[data-testid="virtuoso-item-list"]') as HTMLElement | null;
    return !!list && list.style.visibility !== 'hidden' && list.childElementCount > 0;
}

export const LIST_READY_POLL_MS = 50;
export const LIST_READY_MAX_MS = 5000;

/**
 * Polls `probe` until it is true (or `maxMs` passed, so a cover can never stay up for good) and then
 * calls `onReady` once. Uses a frame raced against a timer so a window that is not being painted
 * still finishes. Returns a cancel function.
 */
export function watchListReady(
    probe: () => boolean,
    onReady: () => void,
    maxMs: number = LIST_READY_MAX_MS,
    now: () => number = Date.now,
): () => void {
    const startedAt = now();
    let cancelled = false;
    let handle: { cancel: () => void } | null = null;
    const step = () => {
        if (cancelled) return;
        if (probe() || now() - startedAt >= maxMs) {
            onReady();
            return;
        }
        handle = requestFrameOrTimeout(step, LIST_READY_POLL_MS);
    };
    step();
    return () => {
        cancelled = true;
        handle?.cancel();
    };
}
