export type ConnectionMode = 'streaming' | 'polling' | 'manual';
export const POLLING_INTERVALS = [5, 10, 30, 60] as const;
export const WATCH_FAILURE_WINDOW_MS = 60_000;
export const WATCH_FAILURE_THRESHOLD = 5;

export const restoreConnectionMode = (context: string, storage: Pick<Storage, 'getItem'> = localStorage): ConnectionMode => {
    const mode = context && storage.getItem(`kubikles_connection_mode_${context}`);
    return mode === 'polling' || mode === 'manual' ? mode : 'streaming';
};
export const restorePollingInterval = (context: string, storage: Pick<Storage, 'getItem'> = localStorage): number => {
    const seconds = Number(context && storage.getItem(`kubikles_polling_interval_${context}`));
    return POLLING_INTERVALS.some(value => value === seconds) ? seconds * 1000 : 10_000;
};
export const isStreamTransportError = (error: unknown): boolean => /watch|stream|upgrade|method not allowed|status code 405|unexpected eof|context deadline exceeded|awaiting headers|connection reset|http2.*closed|proxy.*closed/i.test(String(error));
export const isImmediateWatchClosure = (event: { premature?: boolean; receivedAny?: boolean; openDurationMillis?: number }): boolean => event.premature === true && event.receivedAny !== true && (event.openDurationMillis ?? 0) < 5_000;

/** Failures are isolated per watch and expire, even across quiet sessions. */
export class WatchFailureWindow {
    private failures = new Map<string, number[]>();
    private connectedAt = new Map<string, number>();

    status(key: string, status: string, now: number): void {
        if (status === 'connected') this.connectedAt.set(key, now);
        else {
            this.clearHealthyHistory(key, now);
            this.connectedAt.delete(key);
            if (status === 'stopped') this.failures.delete(key);
        }
    }
    private clearHealthyHistory(key: string, now: number): void {
        const opened = this.connectedAt.get(key);
        if (opened !== undefined && now - opened >= WATCH_FAILURE_WINDOW_MS) this.failures.delete(key);
    }
    fail(key: string, now: number): boolean {
        this.clearHealthyHistory(key, now);
        this.connectedAt.delete(key);
        const recent = (this.failures.get(key) ?? []).filter(time => now - time < WATCH_FAILURE_WINDOW_MS);
        recent.push(now);
        this.failures.set(key, recent);
        return recent.length >= WATCH_FAILURE_THRESHOLD;
    }
}
