import { useMemo } from 'react';
import { useNodeMetrics } from './useNodeMetrics';

/**
 * Hook combining node and pod metrics with cluster-wide aggregations.
 * Used by MetricsOverview for dashboard data.
 *
 * Provides:
 * - Cluster totals: CPU/Memory usage, reserved, committed, capacity
 * - Node metrics: Per-node breakdown
 * - Pod count
 * - Manual refresh (no auto-polling)
 */
export const useClusterMetrics = (isVisible: boolean) => {
    // Disable auto-polling - user will manually refresh
    // Only use node metrics - they include pod count per node, which is all we need for Overview
    const { metrics: nodeMetricsMap, available: nodeAvailable, loading: nodeLoading, source: metricsSource, refresh: refreshNodes } = useNodeMetrics(isVisible, true, false);

    // Refresh is just node metrics refresh
    const refresh = refreshNodes;

    // Convert node metrics map to array
    const nodeMetrics = useMemo(() => {
        return Object.entries(nodeMetricsMap)
            .map(([name, metrics]) => ({ name, ...metrics }));
    }, [nodeMetricsMap]);

    // Cluster totals aggregated from node metrics
    const clusterTotals = useMemo(() => {
        const nodes = Object.values(nodeMetricsMap);

        if (nodes.length === 0) {
            return {
                nodeCount: 0,
                podCount: 0,
                podCapacity: 0,
                podPercent: 0,
                // CPU
                cpuUsage: 0,
                cpuReserved: 0,
                cpuCommitted: 0,
                cpuCapacity: 0,
                cpuUsagePercent: 0,
                cpuReservedPercent: 0,
                cpuCommittedPercent: 0,
                // Memory
                memUsage: 0,
                memReserved: 0,
                memCommitted: 0,
                memCapacity: 0,
                memUsagePercent: 0,
                memReservedPercent: 0,
                memCommittedPercent: 0
            };
        }

        const totals = nodes.reduce((acc: any, node: any) => ({
            cpuUsage: acc.cpuUsage + (node.cpuUsage || 0),
            cpuReserved: acc.cpuReserved + (node.cpuRequested || 0),  // Field is named cpuRequested in useNodeMetrics
            cpuCommitted: acc.cpuCommitted + (node.cpuCommitted || 0),
            cpuCapacity: acc.cpuCapacity + (node.cpuCapacity || 0),
            memUsage: acc.memUsage + (node.memoryUsage || 0),
            memReserved: acc.memReserved + (node.memRequested || 0),  // Field is named memRequested in useNodeMetrics
            memCommitted: acc.memCommitted + (node.memCommitted || 0),
            memCapacity: acc.memCapacity + (node.memCapacity || 0),
            podCount: acc.podCount + (node.podCount || 0),
            podCapacity: acc.podCapacity + (node.podCapacity || 0),
        }), {
            cpuUsage: 0, cpuReserved: 0, cpuCommitted: 0, cpuCapacity: 0,
            memUsage: 0, memReserved: 0, memCommitted: 0, memCapacity: 0,
            podCount: 0, podCapacity: 0
        });

        return {
            nodeCount: nodes.length,
            podCount: totals.podCount,
            podCapacity: totals.podCapacity,
            podPercent: totals.podCapacity > 0
                ? Math.round((totals.podCount / totals.podCapacity) * 100)
                : 0,
            // CPU
            cpuUsage: totals.cpuUsage,
            cpuReserved: totals.cpuReserved,
            cpuCommitted: totals.cpuCommitted,
            cpuCapacity: totals.cpuCapacity,
            cpuUsagePercent: totals.cpuCapacity > 0
                ? Math.round((totals.cpuUsage / totals.cpuCapacity) * 100)
                : 0,
            cpuReservedPercent: totals.cpuCapacity > 0
                ? Math.round((totals.cpuReserved / totals.cpuCapacity) * 100)
                : 0,
            cpuCommittedPercent: totals.cpuCapacity > 0
                ? Math.round((totals.cpuCommitted / totals.cpuCapacity) * 100)
                : 0,
            // Memory
            memUsage: totals.memUsage,
            memReserved: totals.memReserved,
            memCommitted: totals.memCommitted,
            memCapacity: totals.memCapacity,
            memUsagePercent: totals.memCapacity > 0
                ? Math.round((totals.memUsage / totals.memCapacity) * 100)
                : 0,
            memReservedPercent: totals.memCapacity > 0
                ? Math.round((totals.memReserved / totals.memCapacity) * 100)
                : 0,
            memCommittedPercent: totals.memCapacity > 0
                ? Math.round((totals.memCommitted / totals.memCapacity) * 100)
                : 0
        };
    }, [nodeMetricsMap]);

    // Available when node metrics are available (either from K8s or Prometheus)
    const available = nodeAvailable;
    const loading = nodeLoading;

    return {
        nodeMetrics,
        clusterTotals,
        available,
        loading,
        source: metricsSource, // 'k8s' or 'prometheus'
        refresh
    };
};
