import React, { useState, useEffect, useMemo, useRef, useCallback } from 'react';
import { ChartBarIcon, ExclamationTriangleIcon, ArrowUturnLeftIcon } from '@heroicons/react/24/outline';
import { DetectPrometheus, GetControllerMetricsHistory, GetControllerMetricsHistoryRange, GetMetricsEventMarkers } from 'wailsjs/go/main/App';
import { formatBytes, formatChartTime as formatTime } from '~/utils/formatting';
import { MetricsChart, MARKER_COLORS, type EventMarker } from './metrics/MetricsChart';

interface MetricPoint {
    value: number;
    timestamp: string;
}

interface ChartPoint {
    x: number;
    y: number;
    value: number;
    timestamp: string;
}

// Resolve a metric time-series to its latest scalar value for the chart's
// request/limit reference lines (mirrors the previous in-chart derivation).
const lastMetricValue = (series?: MetricPoint[]): number | null =>
    series && series.length > 0 ? series[series.length - 1]?.value ?? null : null;

interface CountChartProps {
    data: MetricPoint[];
    color: string;
    label: string;
    duration: string;
}

interface NetworkData {
    receiveBytes?: MetricPoint[];
    transmitBytes?: MetricPoint[];
    receivePackets?: MetricPoint[];
    transmitPackets?: MetricPoint[];
    receiveDropped?: MetricPoint[];
    transmitDropped?: MetricPoint[];
}

interface NetworkChartProps {
    data: NetworkData;
    duration: string;
}

interface PrometheusInfo {
    available: boolean;
    namespace?: string;
    service?: string;
    port?: number;
}

interface ControllerMetricsTabProps {
    namespace: string;
    name: string;
    controllerType: string;
    isStale: boolean;
}

// Simple count chart for pods/restarts
// Memoized to prevent re-renders when parent updates with same props
const CountChart = React.memo(({ data, color, label, duration }: CountChartProps) => {
    const containerRef = useRef<HTMLDivElement>(null);
    const [hoveredIndex, setHoveredIndex] = useState<number | null>(null);
    const [mousePos, setMousePos] = useState({ x: 0, y: 0 });
    const [containerWidth, setContainerWidth] = useState(400);

    // Track container size for responsive width
    useEffect(() => {
        if (!containerRef.current) return;
        const observer = new ResizeObserver(entries => {
            const entry = entries[0];
            if (entry) {
                setContainerWidth(entry.contentRect.width);
            }
        });
        observer.observe(containerRef.current);
        return () => observer.disconnect();
    }, []);

    if (!data || data.length === 0) {
        return (
            <div className="h-32 flex items-center justify-center text-gray-500 text-sm bg-background rounded border border-border">
                No data available
            </div>
        );
    }

    const values = data.map((d: MetricPoint) => d.value);
    const timestamps = data.map((d: MetricPoint) => d.timestamp);
    const max = Math.max(...values, 1);
    const min = Math.min(...values, 0);
    const range = max - min || 1;
    const yMin = Math.max(0, min - range * 0.1);
    const yMax = max + range * 0.1;
    const yRange = yMax - yMin || 1;

    const width = Math.max(300, containerWidth);
    const height = 100;
    const paddingLeft = 40;
    const paddingRight = 20;
    const paddingTop = 15;
    const paddingBottom = 25;
    const chartWidth = width - paddingLeft - paddingRight;
    const chartHeight = height - paddingTop - paddingBottom;

    const points: ChartPoint[] = data.map((d: MetricPoint, i: number) => {
        const x = paddingLeft + (i / (data.length - 1)) * chartWidth;
        const y = paddingTop + chartHeight - ((d.value - yMin) / yRange) * chartHeight;
        return { x, y, value: d.value, timestamp: d.timestamp };
    });

    const linePath = points.map((p: ChartPoint, i: number) => `${i === 0 ? 'M' : 'L'} ${p.x} ${p.y}`).join(' ');

    const handleMouseMove = useCallback((e: any) => {
        if (!containerRef.current) return;
        const rect = containerRef.current.getBoundingClientRect();
        // Account for CSS zoom applied to document body
        const zoom = parseFloat(document.body.style.zoom) || 1;
        const mouseX = e.clientX / zoom;
        const mouseY = e.clientY / zoom;

        // Calculate actual SVG render size (preserveAspectRatio="xMinYMin meet" maintains aspect ratio)
        const viewBoxAspect = width / height;
        const containerAspect = rect.width / rect.height;
        let svgRenderWidth;
        if (containerAspect > viewBoxAspect) {
            svgRenderWidth = rect.height * viewBoxAspect;
        } else {
            svgRenderWidth = rect.width;
        }

        const svgX = ((mouseX - rect.left) / svgRenderWidth) * width;
        if (svgX >= paddingLeft && svgX <= width - paddingRight) {
            const chartX = svgX - paddingLeft;
            const index = Math.round((chartX / chartWidth) * (data.length - 1));
            setHoveredIndex(Math.max(0, Math.min(data.length - 1, index)));
            setMousePos({ x: mouseX - rect.left, y: mouseY - rect.top });
        } else {
            setHoveredIndex(null);
        }
    }, [data.length, chartWidth]);

    const currentValue = values[values.length - 1];
    const hoveredPoint = hoveredIndex !== null ? points[hoveredIndex] : null;

    return (
        <div className="relative">
            <div className="flex items-center justify-between mb-1">
                <span className="text-xs font-medium text-gray-400">{label}</span>
                <span className={`text-sm font-medium ${color.replace('stroke-', 'text-')}`}>{Math.round(currentValue)}</span>
            </div>
            <div ref={containerRef} className="h-24 bg-background rounded border border-border relative"
                onMouseMove={handleMouseMove} onMouseLeave={() => setHoveredIndex(null)}>
                <svg viewBox={`0 0 ${width} ${height}`} preserveAspectRatio="xMinYMin meet" className="w-full h-full">
                    <line x1={paddingLeft} y1={paddingTop + chartHeight} x2={width - paddingRight} y2={paddingTop + chartHeight}
                        className="stroke-gray-700" strokeWidth="0.5" />
                    <path d={linePath} fill="none" className={color} strokeWidth="2" strokeLinecap="round" />
                    {hoveredPoint && (
                        <circle cx={hoveredPoint.x} cy={hoveredPoint.y} r="3"
                            className={`${color.replace('stroke-', 'fill-')}`} stroke="white" strokeWidth="1.5" />
                    )}
                </svg>
                {hoveredPoint && (
                    <div className="absolute z-10 pointer-events-none bg-surface border border-border rounded px-2 py-1 text-xs"
                        style={{ left: Math.min(mousePos.x + 10, (containerRef.current?.offsetWidth ?? 0) - 80 || 0), top: mousePos.y - 30 }}>
                        <span className={color.replace('stroke-', 'text-')}>{Math.round(hoveredPoint.value)}</span>
                    </div>
                )}
            </div>
        </div>
    );
});

// Network I/O chart with bandwidth and packets view toggle
const NetworkChart = React.memo(({ data, duration }: NetworkChartProps) => {
    const containerRef = useRef<HTMLDivElement>(null);
    const [hoveredIndex, setHoveredIndex] = useState<number | null>(null);
    const [mousePos, setMousePos] = useState({ x: 0, y: 0 });
    const [viewMode, setViewMode] = useState('bandwidth'); // 'bandwidth' or 'packets'
    const [containerWidth, setContainerWidth] = useState(400);

    // Track container size for responsive width
    useEffect(() => {
        if (!containerRef.current) return;
        const observer = new ResizeObserver(entries => {
            const entry = entries[0];
            if (entry) {
                setContainerWidth(entry.contentRect.width);
            }
        });
        observer.observe(containerRef.current);
        return () => observer.disconnect();
    }, []);

    const hasRxBytes = (data?.receiveBytes?.length ?? 0) > 0;
    const hasTxBytes = (data?.transmitBytes?.length ?? 0) > 0;
    const hasRxPackets = (data?.receivePackets?.length ?? 0) > 0;
    const hasTxPackets = (data?.transmitPackets?.length ?? 0) > 0;
    const hasRxDropped = (data?.receiveDropped?.length ?? 0) > 0;
    const hasTxDropped = (data?.transmitDropped?.length ?? 0) > 0;

    const hasBandwidth = hasRxBytes || hasTxBytes;
    const hasPackets = hasRxPackets || hasTxPackets;

    if (!hasBandwidth && !hasPackets) {
        return (
            <div className="relative">
                <div className="text-sm font-medium text-gray-300 mb-2">Network I/O</div>
                <div className="h-24 flex items-center justify-center text-gray-500 text-sm bg-background rounded border border-border">
                    No data available
                </div>
            </div>
        );
    }

    // Select data based on view mode
    const isBandwidthView = viewMode === 'bandwidth';
    const rx = isBandwidthView ? (data.receiveBytes || []) : (data.receivePackets || []);
    const tx = isBandwidthView ? (data.transmitBytes || []) : (data.transmitPackets || []);
    const rxDropped = data.receiveDropped || [];
    const txDropped = data.transmitDropped || [];
    const hasRx = isBandwidthView ? hasRxBytes : hasRxPackets;
    const hasTx = isBandwidthView ? hasTxBytes : hasTxPackets;

    const allValues = [...rx.map((d: MetricPoint) => d.value), ...tx.map((d: MetricPoint) => d.value)];
    const max = Math.max(...allValues, 1);
    const yMax = max * 1.1;
    const yRange = yMax || 1;

    const width = Math.max(300, containerWidth);
    const height = 100;
    const paddingLeft = 55;
    const paddingRight = 20;
    const paddingTop = 15;
    const paddingBottom = 25;
    const chartWidth = width - paddingLeft - paddingRight;
    const chartHeight = height - paddingTop - paddingBottom;

    const baseData = hasRx ? rx : tx;
    const generatePoints = (dataPoints: MetricPoint[]): ChartPoint[] => {
        return dataPoints.map((d: MetricPoint, i: number) => {
            const x = paddingLeft + (i / (dataPoints.length - 1)) * chartWidth;
            const y = paddingTop + chartHeight - (d.value / yRange) * chartHeight;
            return { x, y, value: d.value, timestamp: d.timestamp };
        });
    };

    const rxPoints = hasRx ? generatePoints(rx) : [];
    const txPoints = hasTx ? generatePoints(tx) : [];

    const createPath = (points: ChartPoint[]) => points.map((p: ChartPoint, i: number) => `${i === 0 ? 'M' : 'L'} ${p.x} ${p.y}`).join(' ');

    const handleMouseMove = useCallback((e: any) => {
        if (!containerRef.current) return;
        const rect = containerRef.current.getBoundingClientRect();
        const zoom = parseFloat(document.body.style.zoom) || 1;
        const mouseX = e.clientX / zoom;
        const mouseY = e.clientY / zoom;

        const viewBoxAspect = width / height;
        const containerAspect = rect.width / rect.height;
        let svgRenderWidth;
        if (containerAspect > viewBoxAspect) {
            svgRenderWidth = rect.height * viewBoxAspect;
        } else {
            svgRenderWidth = rect.width;
        }

        const svgX = ((mouseX - rect.left) / svgRenderWidth) * width;
        if (svgX >= paddingLeft && svgX <= width - paddingRight) {
            const chartX = svgX - paddingLeft;
            const index = Math.round((chartX / chartWidth) * (baseData.length - 1));
            setHoveredIndex(Math.max(0, Math.min(baseData.length - 1, index)));
            setMousePos({ x: mouseX - rect.left, y: mouseY - rect.top });
        } else {
            setHoveredIndex(null);
        }
    }, [baseData.length, chartWidth]);

    const formatRate = (value: number) => {
        if (value == null || isNaN(value)) return '-';
        return `${formatBytes(value)}/s`;
    };

    const formatPacketRate = (value: number) => {
        if (value == null || isNaN(value)) return '-';
        if (value >= 1000000) return `${(value / 1000000).toFixed(1)}M/s`;
        if (value >= 1000) return `${(value / 1000).toFixed(1)}K/s`;
        return `${Math.round(value)}/s`;
    };

    const currentRx = rx[rx.length - 1]?.value || 0;
    const currentTx = tx[tx.length - 1]?.value || 0;
    const currentRxDropped = rxDropped[rxDropped.length - 1]?.value || 0;
    const currentTxDropped = txDropped[txDropped.length - 1]?.value || 0;
    const hoveredRxPoint = hoveredIndex !== null && rxPoints[hoveredIndex];
    const hoveredTxPoint = hoveredIndex !== null && txPoints[hoveredIndex];
    const hoveredRxDropped = hoveredIndex !== null && rxDropped[hoveredIndex];
    const hoveredTxDropped = hoveredIndex !== null && txDropped[hoveredIndex];

    const formatValue = isBandwidthView ? formatRate : formatPacketRate;

    return (
        <div className="relative">
            <div className="flex items-center justify-between mb-1">
                <div className="flex items-center gap-3">
                    <span className="text-xs font-medium text-gray-400">Network</span>
                    {/* View toggle */}
                    <div className="flex items-center gap-0.5 bg-gray-800 rounded p-0.5">
                        <button
                            onClick={() => setViewMode('bandwidth')}
                            className={`px-1.5 py-0.5 text-[10px] rounded transition-colors ${
                                viewMode === 'bandwidth'
                                    ? 'bg-gray-600 text-white'
                                    : 'text-gray-400 hover:text-gray-300'
                            }`}
                        >
                            Bytes
                        </button>
                        <button
                            onClick={() => setViewMode('packets')}
                            className={`px-1.5 py-0.5 text-[10px] rounded transition-colors ${
                                viewMode === 'packets'
                                    ? 'bg-gray-600 text-white'
                                    : 'text-gray-400 hover:text-gray-300'
                            }`}
                        >
                            Packets
                        </button>
                    </div>
                    <div className="flex items-center gap-2 text-xs">
                        {hasRx && (
                            <span className="flex items-center gap-1 text-cyan-400">
                                <span className="w-3 h-0.5 bg-cyan-500"></span>
                                RX
                            </span>
                        )}
                        {hasTx && (
                            <span className="flex items-center gap-1 text-yellow-400">
                                <span className="w-3 h-0.5 bg-yellow-500"></span>
                                TX
                            </span>
                        )}
                    </div>
                </div>
                <div className="text-xs flex items-center gap-2">
                    {hasRx && <span className="text-cyan-400">↓{formatValue(currentRx)}</span>}
                    {hasTx && <span className="text-yellow-400">↑{formatValue(currentTx)}</span>}
                    {!isBandwidthView && (currentRxDropped > 0 || currentTxDropped > 0) && (
                        <span className="text-red-400" title="Dropped packets">
                            ⚠ {formatPacketRate(currentRxDropped + currentTxDropped)} dropped
                        </span>
                    )}
                </div>
            </div>
            <div ref={containerRef} className="h-24 bg-background rounded border border-border relative"
                onMouseMove={handleMouseMove} onMouseLeave={() => setHoveredIndex(null)}>
                <svg viewBox={`0 0 ${width} ${height}`} preserveAspectRatio="xMinYMin meet" className="w-full h-full">
                    <line x1={paddingLeft} y1={paddingTop + chartHeight} x2={width - paddingRight} y2={paddingTop + chartHeight}
                        className="stroke-gray-700" strokeWidth="0.5" />

                    {/* RX line (cyan) */}
                    {rxPoints.length > 0 && (
                        <path d={createPath(rxPoints)} fill="none" className="stroke-cyan-500" strokeWidth="2" strokeLinecap="round" />
                    )}

                    {/* TX line (yellow) */}
                    {txPoints.length > 0 && (
                        <path d={createPath(txPoints)} fill="none" className="stroke-yellow-500" strokeWidth="2" strokeLinecap="round" />
                    )}

                    {hoveredRxPoint && (
                        <circle cx={hoveredRxPoint.x} cy={hoveredRxPoint.y} r="3"
                            className="fill-cyan-500" stroke="white" strokeWidth="1.5" />
                    )}
                    {hoveredTxPoint && (
                        <circle cx={hoveredTxPoint.x} cy={hoveredTxPoint.y} r="3"
                            className="fill-yellow-500" stroke="white" strokeWidth="1.5" />
                    )}
                </svg>
                {(hoveredRxPoint || hoveredTxPoint) && (
                    <div className="absolute z-10 pointer-events-none bg-surface border border-border rounded px-2 py-1 text-xs"
                        style={{ left: Math.min(mousePos.x + 10, (containerRef.current?.offsetWidth ?? 0) - 140 || 0), top: mousePos.y - 50 }}>
                        {hoveredRxPoint && <div className="text-cyan-400">RX: {formatValue(hoveredRxPoint.value)}</div>}
                        {hoveredTxPoint && <div className="text-yellow-400">TX: {formatValue(hoveredTxPoint.value)}</div>}
                        {!isBandwidthView && (hoveredRxDropped || hoveredTxDropped) && (
                            <div className="text-red-400 text-[10px] mt-0.5">
                                Dropped: ↓{formatPacketRate((hoveredRxDropped as any)?.value || 0)} ↑{formatPacketRate((hoveredTxDropped as any)?.value || 0)}
                            </div>
                        )}
                    </div>
                )}
            </div>
        </div>
    );
});

const DURATIONS = [
    { value: '1h', label: '1h' },
    { value: '6h', label: '6h' },
    { value: '24h', label: '24h' },
    { value: '7d', label: '7d' },
    { value: '30d', label: '30d' },
    { value: 'all', label: 'All' },
];

export default function ControllerMetricsTab({ namespace, name, controllerType, isStale }: ControllerMetricsTabProps) {
    const [prometheusInfo, setPrometheusInfo] = useState<PrometheusInfo | null>(null);
    const [detecting, setDetecting] = useState(true);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState<string | null>(null);
    const [metricsData, setMetricsData] = useState<any>(null);
    const [duration, setDuration] = useState('1h');
    const [eventMarkers, setEventMarkers] = useState<EventMarker[]>([]);
    const [zoomRange, setZoomRange] = useState<{ startMs: number; endMs: number } | null>(null);
    const requestIdRef = useRef(0);

    useEffect(() => {
        const detect = async () => {
            try {
                const info = await DetectPrometheus();
                setPrometheusInfo(info);
            } catch (err: any) {
                setPrometheusInfo({ available: false });
            } finally {
                setDetecting(false);
            }
        };
        detect();
    }, []);

    useEffect(() => {
        if (!prometheusInfo?.available || isStale) return;

        const currentRequestId = ++requestIdRef.current;
        const requestIdString = `controller-metrics-${namespace}-${name}`;

        const fetchMetrics = async () => {
            setLoading(true);
            setError(null);
            try {
                const data = zoomRange
                    ? await GetControllerMetricsHistoryRange(
                        requestIdString,
                        prometheusInfo!.namespace!,
                        prometheusInfo!.service!,
                        prometheusInfo!.port!,
                        namespace,
                        name,
                        controllerType,
                        zoomRange.startMs,
                        zoomRange.endMs
                    )
                    : await GetControllerMetricsHistory(
                        requestIdString,
                        prometheusInfo!.namespace!,
                        prometheusInfo!.service!,
                        prometheusInfo!.port!,
                        namespace,
                        name,
                        controllerType,
                        duration
                    );
                if (currentRequestId === requestIdRef.current) {
                    setMetricsData(data);
                    setLoading(false);
                }
            } catch (err: any) {
                if (currentRequestId === requestIdRef.current) {
                    setError(err.toString());
                    setLoading(false);
                }
            }
        };

        fetchMetrics();
    }, [prometheusInfo, namespace, name, controllerType, duration, zoomRange, isStale]);

    // Fetch event markers
    useEffect(() => {
        if (!namespace || !name || isStale) return;
        GetMetricsEventMarkers(namespace, name, controllerType, duration)
            .then((m: EventMarker[]) => setEventMarkers(m || []))
            .catch(() => setEventMarkers([]));
    }, [namespace, name, controllerType, duration, isStale]);

    const handleZoomSelect = useCallback((startMs: number, endMs: number) => {
        setZoomRange({ startMs, endMs });
    }, []);

    const handleDurationChange = useCallback((d: string) => {
        setDuration(d);
        setZoomRange(null);
    }, []);

    const effectiveDuration = useMemo(() => {
        if (!zoomRange) return duration;
        const rangeHours = (zoomRange.endMs - zoomRange.startMs) / 3_600_000;
        if (rangeHours <= 1) return '1h';
        if (rangeHours <= 6) return '6h';
        if (rangeHours <= 24) return '24h';
        if (rangeHours <= 168) return '7d';
        return '30d';
    }, [zoomRange, duration]);

    const filteredMarkers = useMemo(() => {
        if (!zoomRange) return eventMarkers;
        return eventMarkers.filter(m => m.timestamp >= zoomRange.startMs && m.timestamp <= zoomRange.endMs);
    }, [eventMarkers, zoomRange]);

    const formatCPU = (value: number) => {
        if (value == null || isNaN(value)) return '-';
        if (value < 1) return `${(value * 1000).toFixed(0)}µ`;
        if (value < 1000) return `${value.toFixed(0)}m`;
        return `${(value / 1000).toFixed(2)}`;
    };

    if (detecting) {
        return (
            <div className="flex items-center justify-center h-full text-gray-500">
                <div className="flex items-center gap-2">
                    <div className="animate-spin rounded-full h-5 w-5 border-b-2 border-primary"></div>
                    Detecting Prometheus...
                </div>
            </div>
        );
    }

    if (!prometheusInfo?.available) {
        return (
            <div className="flex flex-col items-center justify-center h-full text-gray-500 p-8">
                <ChartBarIcon className="h-16 w-16 mb-4 text-gray-600" />
                <h3 className="text-lg font-medium text-gray-400 mb-2">Prometheus Not Detected</h3>
                <p className="text-sm text-center max-w-md">
                    Historical metrics require Prometheus with kube-state-metrics in your cluster.
                </p>
            </div>
        );
    }

    if (isStale) {
        return (
            <div className="flex items-center justify-center h-full text-gray-500 p-8">
                <ExclamationTriangleIcon className="h-5 w-5 text-yellow-500 mr-2" />
                <span>Metrics unavailable for resources from a different context</span>
            </div>
        );
    }

    return (
        <div className="h-full flex flex-col overflow-hidden">
            {/* Controls */}
            <div className="flex items-center gap-4 px-4 py-3 border-b border-border shrink-0">
                <div className="flex items-center gap-2">
                    <div className="flex items-center gap-1 bg-surface-light rounded-md p-0.5">
                        {DURATIONS.map((d: any) => (
                            <button
                                key={d.value}
                                onClick={() => handleDurationChange(d.value)}
                                className={`px-3 py-1 text-xs font-medium rounded transition-colors ${
                                    duration === d.value && !zoomRange ? 'bg-primary text-white' : 'text-gray-400 hover:text-white'
                                }`}
                            >
                                {d.label}
                            </button>
                        ))}
                    </div>
                    {zoomRange && (
                        <button onClick={() => setZoomRange(null)}
                            className="flex items-center gap-1 px-2 py-1 text-xs text-blue-400 hover:text-blue-300 transition-colors">
                            <ArrowUturnLeftIcon className="w-3 h-3" /> Reset zoom
                        </button>
                    )}
                    {loading && (
                        <div className="animate-spin rounded-full h-4 w-4 border-2 border-primary border-t-transparent" />
                    )}
                </div>
                <div className="ml-auto text-xs text-gray-500">
                    Prometheus: {prometheusInfo.namespace}/{prometheusInfo.service}
                </div>
            </div>

            {/* Content */}
            <div className="flex-1 overflow-auto p-4">
                {loading && !metricsData && (
                    <div className="flex items-center justify-center h-full text-gray-500">
                        <div className="animate-spin rounded-full h-5 w-5 border-b-2 border-primary mr-2"></div>
                        Loading metrics...
                    </div>
                )}

                {error && (
                    <div className="flex items-center gap-2 p-4 bg-red-500/10 border border-red-500/30 rounded text-red-400 mb-4">
                        <ExclamationTriangleIcon className="h-5 w-5" />
                        <span className="text-sm">{error}</span>
                    </div>
                )}

                {metricsData && (
                    <div className="space-y-6">
                        {/* CPU and Memory charts */}
                        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
                            <MetricsChart
                                data={metricsData.cpu?.usage}
                                color="stroke-blue-500"
                                label="CPU Usage (Aggregated)"
                                formatValue={formatCPU}
                                duration={effectiveDuration}
                                request={lastMetricValue(metricsData.cpu?.request)}
                                limit={lastMetricValue(metricsData.cpu?.limit)}
                                defaultLinesVisible={false}
                                markers={filteredMarkers}
                                onZoomSelect={handleZoomSelect}
                            />
                            <MetricsChart
                                data={metricsData.memory?.usage}
                                color="stroke-purple-500"
                                label="Memory Usage (Aggregated)"
                                formatValue={formatBytes}
                                duration={effectiveDuration}
                                request={lastMetricValue(metricsData.memory?.request)}
                                limit={lastMetricValue(metricsData.memory?.limit)}
                                defaultLinesVisible={false}
                                markers={filteredMarkers}
                                onZoomSelect={handleZoomSelect}
                            />
                        </div>

                        {/* Pod count, Restarts, and Network */}
                        <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
                            <CountChart
                                data={metricsData.pods?.running}
                                color="stroke-green-500"
                                label="Running Pods"
                                duration={duration}
                            />
                            <CountChart
                                data={metricsData.restarts}
                                color="stroke-red-500"
                                label="Total Restarts"
                                duration={duration}
                            />
                            <NetworkChart
                                data={metricsData.network}
                                duration={duration}
                            />
                        </div>
                    </div>
                )}
            </div>
        </div>
    );
}
