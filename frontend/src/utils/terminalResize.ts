/**
 * Resize helpers for the xterm panel. Dragging a panel edge produces dozens of
 * size changes per second; each must not turn into an IPC round trip.
 */

/** Wraps a sender so identical consecutive (cols, rows) are not sent twice. */
export function createDedupedSizeSender(send: (cols: number, rows: number) => void) {
    let lastCols = -1;
    let lastRows = -1;
    return {
        send(cols: number, rows: number): boolean {
            if (cols <= 0 || rows <= 0) return false;
            if (cols === lastCols && rows === lastRows) return false;
            lastCols = cols;
            lastRows = rows;
            send(cols, rows);
            return true;
        },
        /** Forget the last size, e.g. when a new backend session starts. */
        reset() {
            lastCols = -1;
            lastRows = -1;
        },
    };
}

export interface FitScheduler {
    request(): void;
    cancel(): void;
}

/**
 * Leading + trailing fit. The first request in a quiet period fits at once so
 * the terminal looks right immediately; further requests inside the window
 * collapse into a single trailing fit when the burst settles.
 * `canFit` is checked at run time so hidden (zero-size) terminals are skipped;
 * a skipped fit is not retried, the caller requests one when the terminal is
 * shown again.
 */
export function createFitScheduler(
    fit: () => void,
    canFit: () => boolean,
    delayMs = 100,
    now: () => number = () => Date.now(),
): FitScheduler {
    let timer: ReturnType<typeof setTimeout> | undefined;
    let lastRun = -Infinity;

    const run = () => {
        lastRun = now();
        if (canFit()) fit();
    };

    return {
        request() {
            if (timer === undefined && now() - lastRun >= delayMs) {
                run();
            }
            if (timer !== undefined) clearTimeout(timer);
            timer = setTimeout(() => {
                timer = undefined;
                run();
            }, delayMs);
        },
        cancel() {
            if (timer !== undefined) clearTimeout(timer);
            timer = undefined;
        },
    };
}
