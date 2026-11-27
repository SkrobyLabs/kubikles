import { useMemo, useCallback } from 'react';
import { useNodeMetrics } from './useNodeMetrics';
import { usePodMetrics } from './usePodMetrics';
import { aggregateNamespaceMetrics } from '../utils/metricsTransform';

/**
 * Hook providing namespace-level metrics aggregated from pod metrics.
 * Metrics are calculated as percentages of cluster total capacity.
 *
 * @param {boolean} isVisible - Whether the component is visible
 * @param {boolean} isReady - Whether to start fetching (e.g., after namespace list loads)
 */
export const useNamespaceMetrics = (isVisible: boolean, isReady = true) => {
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

    // Aggregate pod metrics by namespace, as percentages of the cluster capacity
    const namespaceMetrics = useMemo(() => aggregateNamespaceMetrics(podMetricsMap, nodeMetricsMap), [podMetricsMap, nodeMetricsMap]);

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
