import { useState, useEffect, useCallback, useRef } from 'react';
import { GetPodMetrics, GetPodMetricsFromPrometheus, DetectPrometheus } from 'wailsjs/go/main/App';
import { useK8s } from '../context';
import { useConfig } from '../context';
import { k8s } from 'wailsjs/go/models';
import { startVisibleInterval } from '../utils/windowVisibility';
import { transformPodMetrics, type PodMetricsMap } from '../utils/metricsTransform';

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
    // Background polls must not toggle loading (it re-renders every consumer); only the first load does.
    const hasLoadedRef = useRef(false);
    const prometheusInfoRef = useRef<k8s.PrometheusInfo | null>(null); // Cache prometheus info for fallback
    const pollInterval = getConfig('kubernetes.metricsPollIntervalMs') ?? 30000;

    // Ref to current metrics for structural comparison — skip setState when values unchanged
    const metricsRef = useRef<PodMetricsMap>(metrics);
    const updateMetrics = useCallback((next: PodMetricsMap) => {
        const prev = metricsRef.current;
        const prevKeys = Object.keys(prev);
        const nextKeys = Object.keys(next);
        if (prevKeys.length === nextKeys.length && nextKeys.every((k) => {
            const a = prev[k], b = next[k];
            return a && b &&
                a.cpuPercent === b.cpuPercent && a.memPercent === b.memPercent &&
                a.cpuUsage === b.cpuUsage && a.memoryUsage === b.memoryUsage &&
                a.cpuRequested === b.cpuRequested && a.memRequested === b.memRequested &&
                a.cpuCommitted === b.cpuCommitted && a.memCommitted === b.memCommitted;
        })) return;
        metricsRef.current = next;
        setMetrics(next);
    }, []);

    // Transform metrics result to map format
    const transformMetrics = useCallback((result: k8s.PodMetricsResult): PodMetricsMap => transformPodMetrics(result), []);

    // Get preferred source from config
    const preferredSource = (getConfig('metrics.preferredSource') ?? 'auto') as 'auto' | 'k8s' | 'prometheus';

    // Helper to detect and cache prometheus info
    const ensurePrometheusInfo = useCallback(async (): Promise<k8s.PrometheusInfo | null> => {
        if (!prometheusInfoRef.current) {
            try {
                const promInfo = await DetectPrometheus();
                if (promInfo?.available) {
                    prometheusInfoRef.current = promInfo;
                }
            } catch (e: any) {
            }
        }
        return prometheusInfoRef.current;
    }, []);

    // Helper to fetch from K8s Metrics API
    const fetchFromK8s = useCallback(async (): Promise<k8s.PodMetricsResult> => {
        const result = await GetPodMetrics();
        return result;
    }, []);

    // Helper to fetch from Prometheus
    const fetchFromPrometheus = useCallback(async (): Promise<k8s.PodMetricsResult> => {
        const promInfo = await ensurePrometheusInfo();
        if (!promInfo) {
            return { available: false, metrics: [], error: "No Prometheus available" } as any;
        }
        const { namespace, service, port } = promInfo;
        try {
            const promResult = await GetPodMetricsFromPrometheus(namespace, service, port);
            return promResult;
        } catch (e: any) {
            return { available: false, metrics: [], error: String(e) } as any;
        }
    }, [ensurePrometheusInfo]);

    const fetchMetrics = useCallback(async (): Promise<void> => {
        if (!currentContext || !isVisible) return;

        const showLoading = !hasLoadedRef.current;
        if (showLoading) setLoading(true);
        try {
            // Handle based on preferred source
            if (preferredSource === 'k8s') {
                // K8s only - no fallback
                const result = await fetchFromK8s();
                if (result.available) {
                    setAvailable(true);
                    setSource('k8s');
                    updateMetrics(transformMetrics(result));
                } else {
                    setAvailable(false);
                    setSource(null);
                    updateMetrics({});
                }
                return;
            }

            if (preferredSource === 'prometheus') {
                // Prometheus only - no fallback
                const promResult = await fetchFromPrometheus();
                if (promResult.available) {
                    setAvailable(true);
                    setSource('prometheus');
                    updateMetrics(transformMetrics(promResult));
                } else {
                    setAvailable(false);
                    setSource(null);
                    updateMetrics({});
                }
                return;
            }

            // Auto mode: try K8s first, then Prometheus fallback
            const result = await fetchFromK8s();
            if (result.available) {
                setAvailable(true);
                setSource('k8s');
                updateMetrics(transformMetrics(result));
                return;
            }

            // K8s metrics not available, try Prometheus fallback
            const promResult = await fetchFromPrometheus();
            if (promResult.available) {
                setAvailable(true);
                setSource('prometheus');
                updateMetrics(transformMetrics(promResult));
                return;
            }

            // Neither source available
            setAvailable(false);
            setSource(null);
            updateMetrics({});
        } catch (err: any) {
            console.error("Failed to fetch pod metrics", err);
            setAvailable(false);
            setSource(null);
        } finally {
            hasLoadedRef.current = true;
            if (showLoading) setLoading(false);
        }
    }, [currentContext, isVisible, transformMetrics, preferredSource, fetchFromK8s, fetchFromPrometheus]);

    // Initial fetch and polling - wait for isReady (e.g., pods loaded) before fetching
    useEffect(() => {
        if (!isVisible || !isReady) return;

        // Fetch once ready, but never while the window is hidden: the fetch
        // runs on becoming visible instead. Polling (autoPoll) skips hidden
        // ticks and catches up once on show.
        const stop = startVisibleInterval(fetchMetrics, autoPoll ? pollInterval : null);

        return stop;
    }, [fetchMetrics, isVisible, isReady, autoPoll, pollInterval]);

    // Reset on context change
    useEffect(() => {
        hasLoadedRef.current = false;
        updateMetrics({});
        setAvailable(null); // Reset to unknown until next fetch
        setSource(null);
        prometheusInfoRef.current = null; // Clear cached prometheus info
    }, [currentContext]);

    return { metrics, available, loading, source, refresh: fetchMetrics };
};
