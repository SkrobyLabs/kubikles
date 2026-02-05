import { useState, useEffect, useCallback, useRef } from 'react';
import { GetPodMetrics, GetPodMetricsFromPrometheus, DetectPrometheus } from '../../wailsjs/go/main/App';
import { useK8s } from '../context';
import { useConfig } from '../context';
import { k8s } from '../../wailsjs/go/models';

interface PodMetricData {
    cpuPercent: number;
    memPercent: number;
    cpuReservedPercent: number;
    memReservedPercent: number;
    cpuCommittedPercent: number;
    memCommittedPercent: number;
    cpuUsage: number;
    memoryUsage: number;
    cpuRequested: number;
    memRequested: number;
    cpuCommitted: number;
    memCommitted: number;
    nodeCpuCapacity: number;
    nodeMemCapacity: number;
    nodeName: string;
}

interface PodMetricsMap {
    [key: string]: PodMetricData;
}

type MetricsSource = 'k8s' | 'prometheus' | null;

export const usePodMetrics = (isVisible: boolean, isReady: boolean = true, autoPoll: boolean = true): {
    metrics: PodMetricsMap;
    available: boolean | null;
    loading: boolean;
    source: MetricsSource;
    refresh: () => Promise<void>;
} => {
    const [metrics, setMetrics] = useState<PodMetricsMap>({});
    const [available, setAvailable] = useState<boolean | null>(null); // null = not yet checked
    const [loading, setLoading] = useState<boolean>(false);
    const [source, setSource] = useState<MetricsSource>(null); // 'k8s' or 'prometheus'
    const { currentContext } = useK8s();
    const { getConfig } = useConfig();
    const intervalRef = useRef<NodeJS.Timeout | null>(null);
    const prometheusInfoRef = useRef<k8s.PrometheusInfo | null>(null); // Cache prometheus info for fallback
    const pollInterval = getConfig('kubernetes.metricsPollIntervalMs') ?? 30000;

    // Transform metrics result to map format
    const transformMetrics = useCallback((result: k8s.PodMetricsResult): PodMetricsMap => {
        const metricsMap: PodMetricsMap = {};
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
        return metricsMap;
    }, []);

    // Get preferred source from config
    const preferredSource = (getConfig('metrics.preferredSource') ?? 'auto') as 'auto' | 'k8s' | 'prometheus';

    // Helper to detect and cache prometheus info
    const ensurePrometheusInfo = useCallback(async (): Promise<k8s.PrometheusInfo | null> => {
        if (!prometheusInfoRef.current) {
            try {
                console.log("Pod metrics: Detecting Prometheus...");
                const promInfo = await DetectPrometheus();
                console.log("Pod metrics: Prometheus detection result:", promInfo);
                if (promInfo?.available) {
                    prometheusInfoRef.current = promInfo;
                }
            } catch (e) {
                console.log("Pod metrics: Prometheus detection failed:", e);
            }
        }
        return prometheusInfoRef.current;
    }, []);

    // Helper to fetch from K8s Metrics API
    const fetchFromK8s = useCallback(async (): Promise<k8s.PodMetricsResult> => {
        console.log("Pod metrics: Fetching K8s metrics...");
        const result = await GetPodMetrics();
        console.log("Pod metrics: K8s result:", result);
        return result;
    }, []);

    // Helper to fetch from Prometheus
    const fetchFromPrometheus = useCallback(async (): Promise<k8s.PodMetricsResult> => {
        const promInfo = await ensurePrometheusInfo();
        if (!promInfo) {
            console.log("Pod metrics: No Prometheus available");
            return { available: false, metrics: [], error: "No Prometheus available" };
        }
        const { namespace, service, port } = promInfo;
        console.log(`Pod metrics: Querying Prometheus at ${namespace}/${service}:${port}...`);
        try {
            const promResult = await GetPodMetricsFromPrometheus(namespace, service, port);
            console.log("Pod metrics: Prometheus result:", promResult);
            return promResult;
        } catch (e) {
            console.log("Pod metrics: Prometheus query failed:", e);
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
                console.log("Pod metrics: K8s metrics available, using them");
                setAvailable(true);
                setSource('k8s');
                setMetrics(transformMetrics(result));
                return;
            }
            console.log("Pod metrics: K8s metrics not available (available=" + result.available + ", error=" + result.error + ")");

            // K8s metrics not available, try Prometheus fallback
            console.log("Pod metrics: K8s metrics unavailable, trying Prometheus fallback...");
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
            console.error("Failed to fetch pod metrics", err);
            setAvailable(false);
            setSource(null);
        } finally {
            setLoading(false);
        }
    }, [currentContext, isVisible, transformMetrics, preferredSource, fetchFromK8s, fetchFromPrometheus]);

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
        setSource(null);
        prometheusInfoRef.current = null; // Clear cached prometheus info
    }, [currentContext]);

    return { metrics, available, loading, source, refresh: fetchMetrics };
};
