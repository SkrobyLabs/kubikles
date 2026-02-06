import { useState, useEffect, useCallback, useRef } from 'react';
import { GetNodeMetrics, GetNodeMetricsFromPrometheus, DetectPrometheus } from 'wailsjs/go/main/App';
import { useK8s } from '../context';
import { useConfig } from '../context';
import { k8s } from 'wailsjs/go/models';

interface NodeMetricData {
    cpuPercent: number;
    memPercent: number;
    cpuReservedPercent: number;
    memReservedPercent: number;
    cpuCommittedPercent: number;
    memCommittedPercent: number;
    podPercent: number;
    cpuUsage: number;
    memoryUsage: number;
    cpuCapacity: number;
    memCapacity: number;
    cpuRequested: number;
    memRequested: number;
    cpuCommitted: number;
    memCommitted: number;
    podCount: number;
    podCapacity: number;
}

interface NodeMetricsMap {
    [nodeName: string]: NodeMetricData;
}

type MetricsSource = 'k8s' | 'prometheus' | null;

export const useNodeMetrics = (isVisible: boolean, isReady: boolean = true, autoPoll: boolean = true): {
    metrics: NodeMetricsMap;
    available: boolean | null;
    loading: boolean;
    source: MetricsSource;
    refresh: () => Promise<void>;
} => {
    const [metrics, setMetrics] = useState<NodeMetricsMap>({});
    const [available, setAvailable] = useState<boolean | null>(null); // null = not yet checked
    const [loading, setLoading] = useState<boolean>(false);
    const [source, setSource] = useState<MetricsSource>(null); // 'k8s' or 'prometheus'
    const { currentContext } = useK8s();
    const { getConfig } = useConfig();
    const intervalRef = useRef<NodeJS.Timeout | null>(null);
    const prometheusInfoRef = useRef<k8s.PrometheusInfo | null>(null); // Cache prometheus info for fallback
    const pollInterval = getConfig('kubernetes.metricsPollIntervalMs') ?? 30000;

    // Transform metrics result to map format
    const transformMetrics = useCallback((result: k8s.NodeMetricsResult): NodeMetricsMap => {
        const metricsMap: NodeMetricsMap = {};
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
        return metricsMap;
    }, []);

    // Get preferred source from config
    const preferredSource = (getConfig('metrics.preferredSource') ?? 'auto') as 'auto' | 'k8s' | 'prometheus';

    // Helper to detect and cache prometheus info
    const ensurePrometheusInfo = useCallback(async (): Promise<k8s.PrometheusInfo | null> => {
        if (!prometheusInfoRef.current) {
            try {
                console.log("Detecting Prometheus...");
                const promInfo = await DetectPrometheus();
                console.log("Prometheus detection result:", promInfo);
                if (promInfo?.available) {
                    prometheusInfoRef.current = promInfo;
                }
            } catch (e) {
                console.log("Prometheus detection failed:", e);
            }
        }
        return prometheusInfoRef.current;
    }, []);

    // Helper to fetch from K8s Metrics API
    const fetchFromK8s = useCallback(async (): Promise<k8s.NodeMetricsResult> => {
        console.log("Fetching K8s metrics...");
        const result = await GetNodeMetrics();
        console.log("K8s metrics result:", result);
        return result;
    }, []);

    // Helper to fetch from Prometheus
    const fetchFromPrometheus = useCallback(async (): Promise<k8s.NodeMetricsResult> => {
        const promInfo = await ensurePrometheusInfo();
        if (!promInfo) {
            console.log("No Prometheus available");
            return { available: false, metrics: [], error: "No Prometheus available" };
        }
        const { namespace, service, port } = promInfo;
        console.log(`Querying Prometheus at ${namespace}/${service}:${port}...`);
        try {
            const promResult = await GetNodeMetricsFromPrometheus(namespace, service, port);
            console.log("Prometheus metrics result:", promResult);
            return promResult;
        } catch (e) {
            console.log("Prometheus query failed:", e);
            return { available: false, metrics: [], error: String(e) };
        }
    }, [ensurePrometheusInfo]);

    const fetchMetrics = useCallback(async (): Promise<void> => {
        if (!currentContext || !isVisible) return;

        setLoading(true);
        try {
            // Handle based on preferred source
            if (preferredSource === 'k8s') {
                // K8s only - no fallback
                const result = await fetchFromK8s();
                if (result.available) {
                    setAvailable(true);
                    setSource('k8s');
                    setMetrics(transformMetrics(result));
                } else {
                    setAvailable(false);
                    setSource(null);
                    setMetrics({});
                }
                return;
            }

            if (preferredSource === 'prometheus') {
                // Prometheus only - no fallback
                const promResult = await fetchFromPrometheus();
                if (promResult.available) {
                    setAvailable(true);
                    setSource('prometheus');
                    setMetrics(transformMetrics(promResult));
                } else {
                    setAvailable(false);
                    setSource(null);
                    setMetrics({});
                }
                return;
            }

            // Auto mode: try K8s first, then Prometheus fallback
            const result = await fetchFromK8s();
            if (result.available) {
                console.log("K8s metrics available, using them");
                setAvailable(true);
                setSource('k8s');
                setMetrics(transformMetrics(result));
                return;
            }
            console.log("K8s metrics not available (available=" + result.available + ", error=" + result.error + ")");

            // K8s metrics not available, try Prometheus fallback
            console.log("K8s metrics unavailable, trying Prometheus fallback...");
            const promResult = await fetchFromPrometheus();
            if (promResult.available) {
                setAvailable(true);
                setSource('prometheus');
                setMetrics(transformMetrics(promResult));
                return;
            }

            // Neither source available
            setAvailable(false);
            setSource(null);
            setMetrics({});
        } catch (err) {
            console.error("Failed to fetch node metrics", err);
            setAvailable(false);
            setSource(null);
        } finally {
            setLoading(false);
        }
    }, [currentContext, isVisible, transformMetrics, preferredSource, fetchFromK8s, fetchFromPrometheus]);

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
        setSource(null);
        prometheusInfoRef.current = null; // Clear cached prometheus info
    }, [currentContext]);

    return { metrics, available, loading, source, refresh: fetchMetrics };
};
