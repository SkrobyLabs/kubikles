import { useMemo, useCallback } from 'react';
import { useNodeMetrics } from './useNodeMetrics';
import { usePodMetrics } from './usePodMetrics';

/**
 * Hook providing namespace-level metrics aggregated from pod metrics.
 * Metrics are calculated as percentages of cluster total capacity.
 *
 * @param {boolean} isVisible - Whether the component is visible
 * @param {boolean} isReady - Whether to start fetching (e.g., after namespace list loads)
 */
export const useNamespaceMetrics = (isVisible, isReady = true) => {
    // Disable auto-polling - metrics loaded on demand
    const {
        metrics: nodeMetricsMap,
        available: nodeAvailable,
        loading: nodeLoading,
        refresh: refreshNodes
    } = useNodeMetrics(isVisible, isReady, false);

    const {
        metrics: podMetricsMap,
        available: podAvailable,
        loading: podLoading,
        refresh: refreshPods
    } = usePodMetrics(isVisible, isReady, false);

    // Calculate cluster totals from node metrics
    const clusterTotals = useMemo(() => {
        const nodes = Object.values(nodeMetricsMap);
        if (nodes.length === 0) {
            return { cpuCapacity: 0, memCapacity: 0 };
        }
        return nodes.reduce((acc, node) => ({
            cpuCapacity: acc.cpuCapacity + (node.cpuCapacity || 0),
            memCapacity: acc.memCapacity + (node.memCapacity || 0),
        }), { cpuCapacity: 0, memCapacity: 0 });
    }, [nodeMetricsMap]);

    // Aggregate pod metrics by namespace
    const namespaceMetrics = useMemo(() => {
        const byNamespace = {};

        // Iterate through pod metrics and aggregate by namespace
        for (const [key, pod] of Object.entries(podMetricsMap)) {
            const namespace = key.split('/')[0];

            if (!byNamespace[namespace]) {
                byNamespace[namespace] = {
                    cpuUsage: 0,
                    memUsage: 0,
                    cpuCommitted: 0,
                    memCommitted: 0,
                    podCount: 0
                };
            }

            byNamespace[namespace].cpuUsage += pod.cpuUsage || 0;
            byNamespace[namespace].memUsage += pod.memoryUsage || 0;
            byNamespace[namespace].cpuCommitted += pod.cpuCommitted || 0;
            byNamespace[namespace].memCommitted += pod.memCommitted || 0;
            byNamespace[namespace].podCount += 1;
        }

        // Calculate percentages against cluster capacity
        const result = {};
        for (const [namespace, metrics] of Object.entries(byNamespace)) {
            result[namespace] = {
                ...metrics,
                cpuUsagePercent: clusterTotals.cpuCapacity > 0
                    ? Math.round((metrics.cpuUsage / clusterTotals.cpuCapacity) * 100)
                    : 0,
                memUsagePercent: clusterTotals.memCapacity > 0
                    ? Math.round((metrics.memUsage / clusterTotals.memCapacity) * 100)
                    : 0,
                cpuCommittedPercent: clusterTotals.cpuCapacity > 0
                    ? Math.round((metrics.cpuCommitted / clusterTotals.cpuCapacity) * 100)
                    : 0,
                memCommittedPercent: clusterTotals.memCapacity > 0
                    ? Math.round((metrics.memCommitted / clusterTotals.memCapacity) * 100)
                    : 0,
            };
        }

        return result;
    }, [podMetricsMap, clusterTotals]);

    // Combined refresh
    const refresh = useCallback(() => {
        refreshNodes();
        refreshPods();
    }, [refreshNodes, refreshPods]);

    // Available when both sources are available
    const available = nodeAvailable === null || podAvailable === null
        ? null
        : nodeAvailable && podAvailable;
    const loading = nodeLoading || podLoading;

    return {
        metrics: namespaceMetrics,
        available,
        loading,
        refresh
    };
};
