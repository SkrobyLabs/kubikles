import { useEffect } from 'react';

export const nextPollingDelay = (
    baseMs: number = 10_000,
    random: number = Math.random()
): number => Math.round(baseMs * (0.9 + random * 0.2));

export function startCompletionPolling(
    poll: (isCurrent: () => boolean) => Promise<void>,
    intervalMs: number = 10_000,
    random: () => number = Math.random
): () => void {
    let cancelled = false;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const run = async () => {
        try {
            await poll(() => !cancelled);
        } catch (error) {
            if (!cancelled) console.error('Background poll failed', error);
        } finally {
            if (!cancelled) timer = setTimeout(run, nextPollingDelay(intervalMs, random()));
        }
    };
    timer = setTimeout(run, nextPollingDelay(intervalMs, random()));
    return () => {
        cancelled = true;
        if (timer !== undefined) clearTimeout(timer);
    };
}

/** Polls only after the previous request settles and ignores stale completions. */
export function useCompletionPolling(
    enabled: boolean,
    poll: (isCurrent: () => boolean) => Promise<void>,
    dependencies: React.DependencyList,
    intervalMs: number = 10_000
): void {
    useEffect(() => {
        if (!enabled) return;
        return startCompletionPolling(poll, intervalMs);
    }, [enabled, intervalMs, ...dependencies]);
}
