import { useEffect, useRef } from 'react';
import { useK8s } from '../context/K8sContext';
import { runWithPollingBudget } from '../utils/pollingBudget';
export { startCompletionPolling, nextPollingDelay } from '../utils/completionPolling';

/** Polls only after the previous request settles and ignores stale completions. */
export function useCompletionPolling(
    enabled: boolean,
    poll: (isCurrent: () => boolean) => Promise<void>,
    dependencies: React.DependencyList,
    refreshOnDemand: boolean = false,
): void {
    const { registerPolling, connectionMode, lastRefresh } = useK8s();
    const previousRefresh = useRef(lastRefresh);
    useEffect(() => {
        let cancelled = false;
        let running = false;
        const run = async () => {
            if (running || cancelled) return;
            running = true;
            try {
                await runWithPollingBudget(() => cancelled ? Promise.resolve() : poll(() => !cancelled));
            } catch (error) {
                if (!cancelled) console.error('Background poll failed', error);
            } finally {
                running = false;
            }
        };
        const manualRefresh = previousRefresh.current !== lastRefresh;
        previousRefresh.current = lastRefresh;
        if (!enabled) return;
        if (manualRefresh && refreshOnDemand) void run();
        const unregister = connectionMode === 'polling' ? registerPolling(run) : undefined;
        return () => { cancelled = true; unregister?.(); };
    }, [enabled, registerPolling, connectionMode, lastRefresh, refreshOnDemand, ...dependencies]);
}
