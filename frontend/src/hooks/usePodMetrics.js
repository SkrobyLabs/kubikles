import { useState, useEffect, useCallback, useRef } from 'react';
import { GetPodMetrics } from '../../wailsjs/go/main/App';
import { useK8s } from '../context/K8sContext';

const POLL_INTERVAL = 30000; // 30 seconds

export const usePodMetrics = (isVisible) => {
    const [metrics, setMetrics] = useState({});
    const [available, setAvailable] = useState(true);
    const [loading, setLoading] = useState(false);
    const { currentContext } = useK8s();
    const intervalRef = useRef(null);

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
                const cpuCommittedPercent = m.nodeCpuCapacity > 0
                    ? Math.round((m.cpuCommitted / m.nodeCpuCapacity) * 100)
                    : 0;
                const memCommittedPercent = m.nodeMemCapacity > 0
                    ? Math.round((m.memCommitted / m.nodeMemCapacity) * 100)
                    : 0;

                metricsMap[key] = {
                    cpuPercent,
                    memPercent,
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
        intervalRef.current = setInterval(fetchMetrics, POLL_INTERVAL);

        return () => {
            if (intervalRef.current) {
                clearInterval(intervalRef.current);
                intervalRef.current = null;
            }
        };
    }, [fetchMetrics, isVisible]);

    // Reset on context change
    useEffect(() => {
        setMetrics({});
        setAvailable(true);
    }, [currentContext]);

    return { metrics, available, loading };
};
