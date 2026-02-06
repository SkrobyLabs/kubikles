import { useState, useEffect, useRef, useCallback } from 'react';
import { useUI } from '../context';
import { useConfig } from '../context';
import PerformancePanel from '../components/shared/PerformancePanel';
import { GetPerformanceMetrics } from 'wailsjs/go/main/App';
import { main } from 'wailsjs/go/models';

export const usePerformancePanel = (): {
    openPerformancePanel: () => void;
    isPolling: boolean;
    error: Error | null;
} => {
    const [backendMetrics, setBackendMetrics] = useState<main.PerformanceMetrics | null>(null);
    const [metricsHistory, setMetricsHistory] = useState<main.PerformanceMetrics[]>([]);
    const [isPolling, setIsPolling] = useState<boolean>(false);
    const [error, setError] = useState<Error | null>(null);
    const pollIntervalRef = useRef<ReturnType<typeof setInterval> | null>(null);
    const { openTab, bottomTabs, setBottomTabs } = useUI();
    const { getConfig } = useConfig();
    const pollInterval = getConfig('performance.pollIntervalMs') ?? 1500;

    // Fetch metrics from backend
    const fetchMetrics = useCallback(async (): Promise<void> => {
        try {
            const metrics = await GetPerformanceMetrics();
            setBackendMetrics(metrics);
            setError(null);

            // Keep history for charts (last 60 data points = ~90 seconds at 1.5s interval)
            setMetricsHistory(prev => {
                const newHistory = [...prev, metrics];
                if (newHistory.length > 60) {
                    return newHistory.slice(-60);
                }
                return newHistory;
            });
        } catch (err: any) {
            console.error('Failed to fetch performance metrics:', err);
            setError(err as Error);
        }
    }, []);

    // Start polling
    const startPolling = useCallback((): void => {
        if (pollIntervalRef.current) return;
        setIsPolling(true);
        fetchMetrics(); // Immediate first fetch
        pollIntervalRef.current = setInterval(fetchMetrics, pollInterval);
    }, [fetchMetrics, pollInterval]);

    // Stop polling
    const stopPolling = useCallback((): void => {
        if (pollIntervalRef.current) {
            clearInterval(pollIntervalRef.current);
            pollIntervalRef.current = null;
        }
        setIsPolling(false);
    }, []);

    // Toggle polling
    const togglePolling = useCallback((): void => {
        if (isPolling) {
            stopPolling();
        } else {
            startPolling();
        }
    }, [isPolling, startPolling, stopPolling]);

    // Update tab content when metrics change
    useEffect((): void => {
        setBottomTabs(prev => prev.map((tab: any) => {
            if (tab.id === 'performance-panel') {
                return {
                    ...tab,
                    content: (
                        <PerformancePanel
                            backendMetrics={backendMetrics}
                            metricsHistory={metricsHistory}
                            isPolling={isPolling}
                            onTogglePolling={togglePolling}
                            bottomTabs={prev}
                        />
                    )
                };
            }
            return tab;
        }));
    }, [backendMetrics, metricsHistory, isPolling, setBottomTabs, togglePolling]);

    // Open performance panel
    const openPerformancePanel = useCallback((): void => {
        const tabId = 'performance-panel';
        const existingTab = bottomTabs.find((t: any) => t.id === tabId);

        if (!existingTab) {
            startPolling();
            openTab({
                id: tabId,
                title: 'Performance',
                context: null, // Context-independent
                content: (
                    <PerformancePanel
                        backendMetrics={backendMetrics}
                        metricsHistory={metricsHistory}
                        isPolling={isPolling}
                        onTogglePolling={togglePolling}
                        bottomTabs={bottomTabs}
                    />
                )
            });
        } else {
            // If it exists, just set it as active and ensure polling is on
            if (!isPolling) {
                startPolling();
            }
            openTab(existingTab);
        }
    }, [bottomTabs, openTab, backendMetrics, metricsHistory, isPolling, startPolling, togglePolling]);

    // Stop polling when tab is closed
    useEffect(() => {
        const hasTab = bottomTabs.some((t: any) => t.id === 'performance-panel');
        if (!hasTab && isPolling) {
            stopPolling();
        }
    }, [bottomTabs, isPolling, stopPolling]);

    // Cleanup on unmount
    useEffect(() => {
        return () => stopPolling();
    }, [stopPolling]);

    return {
        openPerformancePanel,
        isPolling,
        error
    };
};
