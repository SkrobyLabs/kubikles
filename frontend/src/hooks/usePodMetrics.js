import { useState, useEffect, useCallback, useRef } from 'react';
import { GetPodMetrics } from '../../wailsjs/go/main/App';
import { useK8s } from '../context/K8sContext';
import { useConfig } from '../context/ConfigContext';

export const usePodMetrics = (isVisible, isReady = true, autoPoll = true) => {
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
            const result = await GetPodMetrics();

            if (!result.available) {
                setAvailable(false);
                setMetrics({});
                return;
            }

            setAvailable(true);

            // Transform array to map keyed by namespace/name with percentages relative to node
            const metricsMap = {};
            for (const m of (result.metrics || [])) {
                const key = `${m.namespace}/${m.name}`;

                const cpuPercent = m.nodeCpuCapacity > 0
                    ? Math.round((m.cpuUsage / m.nodeCpuCapacity) * 100)
                    : 0;
                const memPercent = m.nodeMemCapacity > 0
                    ? Math.round((m.memoryUsage / m.nodeMemCapacity) * 100)
                    : 0;
                const cpuReservedPercent = m.nodeCpuCapacity > 0
                    ? Math.round((m.cpuRequested / m.nodeCpuCapacity) * 100)
                    : 0;
                const memReservedPercent = m.nodeMemCapacity > 0
                    ? Math.round((m.memRequested / m.nodeMemCapacity) * 100)
                    : 0;
                const cpuCommittedPercent = m.nodeCpuCapacity > 0
                    ? Math.round((m.cpuCommitted / m.nodeCpuCapacity) * 100)
                    : 0;
                const memCommittedPercent = m.nodeMemCapacity > 0
                    ? Math.round((m.memCommitted / m.nodeMemCapacity) * 100)
                    : 0;

                metricsMap[key] = {
                    cpuPercent,
                    memPercent,
                    cpuReservedPercent,
                    memReservedPercent,
                    cpuCommittedPercent,
                    memCommittedPercent,
                    cpuUsage: m.cpuUsage,
                    memoryUsage: m.memoryUsage,
                    cpuRequested: m.cpuRequested,
                    memRequested: m.memRequested,
                    cpuCommitted: m.cpuCommitted,
                    memCommitted: m.memCommitted,
                    nodeCpuCapacity: m.nodeCpuCapacity,
                    nodeMemCapacity: m.nodeMemCapacity,
                    nodeName: m.nodeName
                };
            }
            setMetrics(metricsMap);
        } catch (err) {
            console.error("Failed to fetch pod metrics", err);
            setAvailable(false);
        } finally {
            setLoading(false);
        }
    }, [currentContext, isVisible]);

    // Initial fetch and polling - wait for isReady (e.g., pods loaded) before fetching
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
