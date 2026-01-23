import { useState, useEffect, useCallback, useRef } from 'react';
import { GetNodeMetrics } from '../../wailsjs/go/main/App';
import { useK8s } from '../context/K8sContext';
import { useConfig } from '../context/ConfigContext';

export const useNodeMetrics = (isVisible) => {
    const [metrics, setMetrics] = useState({});
    const [available, setAvailable] = useState(true);
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

                metricsMap[m.name] = {
                    cpuPercent,
                    memPercent,
                    cpuReservedPercent,
                    memReservedPercent,
                    cpuCommittedPercent,
                    memCommittedPercent,
                    cpuUsage: m.cpuUsage,
                    memoryUsage: m.memoryUsage,
                    cpuCapacity: m.cpuCapacity,
                    memCapacity: m.memCapacity,
                    cpuRequested: m.cpuRequested,
                    memRequested: m.memRequested,
                    cpuCommitted: m.cpuCommitted,
                    memCommitted: m.memCommitted
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

    // Initial fetch and polling
    useEffect(() => {
        if (!isVisible) {
            // Clear interval when not visible
            if (intervalRef.current) {
                clearInterval(intervalRef.current);
                intervalRef.current = null;
            }
            return;
        }

        // Fetch immediately
        fetchMetrics();

        // Set up polling
        intervalRef.current = setInterval(fetchMetrics, pollInterval);

        return () => {
            if (intervalRef.current) {
                clearInterval(intervalRef.current);
                intervalRef.current = null;
            }
        };
    }, [fetchMetrics, isVisible, pollInterval]);

    // Reset on context change
    useEffect(() => {
        setMetrics({});
        setAvailable(true);
    }, [currentContext]);

    return { metrics, available, loading };
};
