import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { ArrowUturnLeftIcon, ChartBarIcon, ExclamationTriangleIcon } from '@heroicons/react/24/outline';
import { CancelMetricsRequest, DetectPrometheus, GetMetricsEventMarkers, GetNodeMemoryDiagnosticsHistory, GetNodeMemoryDiagnosticsHistoryRange } from 'wailsjs/go/main/App';
import type { EventMarker } from './metrics/MetricsChart';
import NodeMemoryDiagnostics from './NodeMemoryDiagnosticView';
import { integerTimeRange } from './nodeMemoryDiagnostics';

const DURATIONS = [
    { value: '1h', label: '1h' },
    { value: '6h', label: '6h' },
    { value: '24h', label: '24h' },
    { value: '7d', label: '7d' },
    { value: '30d', label: '30d' },
    { value: 'all', label: 'All' },
];

export default function NodeMemoryDiagnosticsTab({ nodeName, isStale }: { nodeName: string; isStale: boolean }) {
    const [prometheusInfo, setPrometheusInfo] = useState<any>(null);
    const [detecting, setDetecting] = useState(true);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState<string | null>(null);
    const [diagnosticsData, setDiagnosticsData] = useState<any>(null);
    const [duration, setDuration] = useState('1h');
    const [eventMarkers, setEventMarkers] = useState<EventMarker[]>([]);
    const [zoomRange, setZoomRange] = useState<{ startMs: number; endMs: number } | null>(null);
    const requestIdRef = useRef(0);
    const markerRequestIdRef = useRef(0);

    useEffect(() => {
        DetectPrometheus()
            .then(setPrometheusInfo)
            .catch(() => setPrometheusInfo({ available: false }))
            .finally(() => setDetecting(false));
    }, []);

    useEffect(() => {
        const currentMarkerRequestId = ++markerRequestIdRef.current;
        setEventMarkers([]);

        if (!nodeName || isStale) return;

        GetMetricsEventMarkers('', nodeName, 'node', duration)
            .then((markers: EventMarker[]) => {
                if (currentMarkerRequestId === markerRequestIdRef.current) {
                    setEventMarkers(markers || []);
                }
            })
            .catch(() => {
                if (currentMarkerRequestId === markerRequestIdRef.current) {
                    setEventMarkers([]);
                }
            });

        return () => {
            markerRequestIdRef.current++;
        };
    }, [nodeName, duration, isStale]);

    useEffect(() => {
        if (!prometheusInfo?.available || isStale) return;

        const currentRequestId = ++requestIdRef.current;
        const requestId = `node-memory-diagnostics-${nodeName}`;
        setDiagnosticsData(null);
        setLoading(true);
        setError(null);

        const fetchDiagnostics = async () => {
            try {
                const data = zoomRange
                    ? await GetNodeMemoryDiagnosticsHistoryRange(requestId, prometheusInfo.namespace, prometheusInfo.service, prometheusInfo.port, nodeName, zoomRange.startMs, zoomRange.endMs)
                    : await GetNodeMemoryDiagnosticsHistory(requestId, prometheusInfo.namespace, prometheusInfo.service, prometheusInfo.port, nodeName, duration);
                if (currentRequestId === requestIdRef.current) {
                    setDiagnosticsData(data);
                    setLoading(false);
                }
            } catch (err: any) {
                if (currentRequestId === requestIdRef.current) {
                    setError(err.toString());
                    setLoading(false);
                }
            }
        };
        fetchDiagnostics();

        return () => {
            requestIdRef.current++;
            CancelMetricsRequest(requestId);
        };
    }, [prometheusInfo, nodeName, duration, isStale, zoomRange]);

    const handleZoomSelect = useCallback((startMs: number, endMs: number) => setZoomRange(integerTimeRange(startMs, endMs)), []);
    const handleDurationChange = useCallback((nextDuration: string) => {
        setDuration(nextDuration);
        setZoomRange(null);
    }, []);
    const filteredMarkers = useMemo(() => zoomRange ? eventMarkers.filter(marker => marker.timestamp >= zoomRange.startMs && marker.timestamp <= zoomRange.endMs) : eventMarkers, [eventMarkers, zoomRange]);

    if (detecting) return <div className="flex h-full items-center justify-center text-gray-500"><div className="flex items-center gap-2"><div className="h-5 w-5 animate-spin rounded-full border-b-2 border-primary" />Detecting Prometheus...</div></div>;
    if (!prometheusInfo?.available) return <div className="flex h-full flex-col items-center justify-center p-8 text-gray-500"><ChartBarIcon className="mb-4 h-16 w-16 text-gray-600" /><h3 className="mb-2 text-lg font-medium text-gray-400">Prometheus Not Detected</h3><p className="max-w-md text-center text-sm">Memory diagnostics require Prometheus with node-exporter in your cluster.</p></div>;
    if (isStale) return <div className="flex h-full items-center justify-center p-8 text-gray-500"><ExclamationTriangleIcon className="mr-2 h-5 w-5 text-yellow-500" /><span>Diagnostics unavailable for resources from a different context</span></div>;

    return <div className="flex h-full flex-col overflow-hidden">
        <div className="flex shrink-0 items-center gap-4 border-b border-border px-4 py-3">
            <div className="flex items-center gap-2"><div className="flex items-center gap-1 rounded-md bg-surface-light p-0.5">{DURATIONS.map(option => <button key={option.value} onClick={() => handleDurationChange(option.value)} className={`rounded px-3 py-1 text-xs font-medium transition-colors ${duration === option.value && !zoomRange ? 'bg-primary text-white' : 'text-gray-400 hover:text-white'}`}>{option.label}</button>)}</div>{zoomRange && <button onClick={() => setZoomRange(null)} className="flex items-center gap-1 px-2 py-1 text-xs text-blue-400 transition-colors hover:text-blue-300"><ArrowUturnLeftIcon className="h-3 w-3" />Reset zoom</button>}{loading && <div className="h-4 w-4 animate-spin rounded-full border-2 border-primary border-t-transparent" />}</div>
            <div className="ml-auto text-xs text-gray-500">Prometheus: {prometheusInfo.namespace}/{prometheusInfo.service}</div>
        </div>
        <div className="flex-1 overflow-auto p-4">
            {loading && !diagnosticsData && <div className="flex h-full items-center justify-center text-gray-500"><div className="mr-2 h-5 w-5 animate-spin rounded-full border-b-2 border-primary" />Loading diagnostics...</div>}
            {error && <div className="mb-4 flex items-center gap-2 rounded border border-red-500/30 bg-red-500/10 p-4 text-red-400"><ExclamationTriangleIcon className="h-5 w-5" /><span className="text-sm">{error}</span></div>}
            {diagnosticsData && <NodeMemoryDiagnostics memory={diagnosticsData.memory} contributors={diagnosticsData.memoryContributors} rangeStartMs={Number(diagnosticsData.rangeStartMs)} rangeEndMs={Number(diagnosticsData.rangeEndMs)} stepMs={Number(diagnosticsData.stepMs)} markers={filteredMarkers} onZoomSelect={handleZoomSelect} />}
        </div>
    </div>;
}
