import { useState, useEffect, useCallback, useRef } from 'react';
import { GetNodeMetrics } from '../../wailsjs/go/main/App';
import { useK8s } from '../context/K8sContext';
import { useConfig } from '../context/ConfigContext';

export const useNodeMetrics = (isVisible, isReady = true, autoPoll = true) => {
    const [metrics, setMetrics] = useState({});
    const [available, setAvailable] = useState(null); // null = not yet checked
    const [loading, setLoading] = useState(false);
    const { currentContext } = useK8s();
    const { getConfig } = useConfig();
    const intervalRef = useRef(null);
    const pollInterval = getConfig('kubernetes.metricsPollIntervalMs') ?? 30000;

    const fetchMetrics = useCallback(async () => {
        if (!currentContext || !isVisible) return;

        setLoading(true);
        try {
            const result = await GetNodeMetrics();

            if (!result.available) {
                setAvailable(false);
                setMetrics({});
                return;
            }

            setAvailable(true);

            // Transform array to map keyed by node name with percentages
            const metricsMap = {};
            for (const m of (result.metrics || [])) {
                const cpuPercent = m.cpuCapacity > 0
                    ? Math.round((m.cpuUsage / m.cpuCapacity) * 100)
                    : 0;
                const memPercent = m.memCapacity > 0
                    ? Math.round((m.memoryUsage / m.memCapacity) * 100)
                    : 0;
                const cpuReservedPercent = m.cpuCapacity > 0
                    ? Math.round((m.cpuRequested / m.cpuCapacity) * 100)
                    : 0;
                const memReservedPercent = m.memCapacity > 0
                    ? Math.round((m.memRequested / m.memCapacity) * 100)
                    : 0;
                const cpuCommittedPercent = m.cpuCapacity > 0
                    ? Math.round((m.cpuCommitted / m.cpuCapacity) * 100)
                    : 0;
                const memCommittedPercent = m.memCapacity > 0
                    ? Math.round((m.memCommitted / m.memCapacity) * 100)
                    : 0;

                const podPercent = m.podCapacity > 0
                    ? Math.round((m.podCount / m.podCapacity) * 100)
                    : 0;

                metricsMap[m.name] = {
                    cpuPercent,
                    memPercent,
                    cpuReservedPercent,
                    memReservedPercent,
                    cpuCommittedPercent,
                    memCommittedPercent,
                    podPercent,
                    cpuUsage: m.cpuUsage,
                    memoryUsage: m.memoryUsage,
                    cpuCapacity: m.cpuCapacity,
                    memCapacity: m.memCapacity,
                    cpuRequested: m.cpuRequested,
                    memRequested: m.memRequested,
                    cpuCommitted: m.cpuCommitted,
                    memCommitted: m.memCommitted,
                    podCount: m.podCount,
                    podCapacity: m.podCapacity
                };
            }
            setMetrics(metricsMap);
        } catch (err) {
            console.error("Failed to fetch node metrics", err);
            setAvailable(false);
        } finally {
            setLoading(false);
        }
    }, [currentContext, isVisible]);

    // Initial fetch and polling - wait for isReady (e.g., nodes loaded) before fetching
    useEffect(() => {
        if (!isVisible || !isReady) {
            // Clear interval when not visible or not ready
            if (intervalRef.current) {
                clearInterval(intervalRef.current);
                intervalRef.current = null;
            }
            return;
        }

        // Fetch immediately once ready
        fetchMetrics();

        // Set up polling only if autoPoll is enabled
        if (autoPoll) {
            intervalRef.current = setInterval(fetchMetrics, pollInterval);
        }

        return () => {
            if (intervalRef.current) {
                clearInterval(intervalRef.current);
                intervalRef.current = null;
            }
        };
    }, [fetchMetrics, isVisible, isReady, autoPoll, pollInterval]);

    // Reset on context change
    useEffect(() => {
        setMetrics({});
        setAvailable(null); // Reset to unknown until next fetch
    }, [currentContext]);

    return { metrics, available, loading, refresh: fetchMetrics };
};
