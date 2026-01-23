import React, { useState, useEffect, useMemo, useRef, useCallback } from 'react';
import { ChartBarIcon, ExclamationTriangleIcon } from '@heroicons/react/24/outline';
import { DetectPrometheus, GetNamespaceMetricsHistory } from '../../../wailsjs/go/main/App';
import { formatBytes } from '../../utils/formatting';

// Format time for display
const formatTime = (timestamp, duration) => {
    const date = new Date(timestamp);
    if (duration === '30d' || duration === 'all') {
        return date.toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
    }
    if (duration === '7d' || duration === '24h') {
        return date.toLocaleDateString(undefined, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });
    }
    return date.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' });
};

// Interactive line chart component
const MetricsChart = React.memo(({ data, color, label, formatValue, duration }) => {
    const containerRef = useRef(null);
    const [hoveredIndex, setHoveredIndex] = useState(null);
    const [mousePos, setMousePos] = useState({ x: 0, y: 0 });

    const width = 500;
    const height = 200;
    const paddingLeft = 60;
    const paddingRight = 20;
    const paddingTop = 20;
    const paddingBottom = 30;
    const chartWidth = width - paddingLeft - paddingRight;
    const chartHeight = height - paddingTop - paddingBottom;

    const chartData = useMemo(() => {
        if (!data || data.length === 0) return null;

        const values = data.map(d => d.value);
        const timestamps = data.map(d => d.timestamp);
        const max = Math.max(...values) || 1;
        const min = Math.min(...values);
        const range = max - min || 1;
        const yMin = Math.max(0, min - range * 0.05);
        const yMax = max + range * 0.05;
        const yRange = yMax - yMin || 1;

        const points = data.map((d, i) => {
            const x = paddingLeft + (i / (data.length - 1)) * chartWidth;
            const y = paddingTop + chartHeight - ((d.value - yMin) / yRange) * chartHeight;
            return { x, y, value: d.value, timestamp: d.timestamp };
        });

        const linePath = points.map((p, i) => `${i === 0 ? 'M' : 'L'} ${p.x} ${p.y}`).join(' ');
        const areaPath = `${linePath} L ${points[points.length - 1].x} ${paddingTop + chartHeight} L ${paddingLeft} ${paddingTop + chartHeight} Z`;

        const yTicks = Array.from({ length: 5 }, (_, i) => {
            const value = yMin + (yRange * i) / 4;
            const y = paddingTop + chartHeight - (i / 4) * chartHeight;
            return { value, y };
        });

        const xTickIndices = [0, Math.floor(data.length / 2), data.length - 1];
        const xTicks = xTickIndices.map(i => ({
            timestamp: timestamps[i],
            x: paddingLeft + (i / (data.length - 1)) * chartWidth
        }));

        return { points, linePath, areaPath, yTicks, xTicks, currentValue: values[values.length - 1] };
    }, [data, chartWidth, chartHeight]);

    if (!chartData) {
        return (
            <div className="h-56 flex items-center justify-center text-gray-500 text-sm bg-background rounded border border-border">
                No data available
            </div>
        );
    }

    const { points, linePath, areaPath, yTicks, xTicks, currentValue } = chartData;

    const handleMouseMove = useCallback((e) => {
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
            const index = Math.round((chartX / chartWidth) * (data.length - 1));
            setHoveredIndex(Math.max(0, Math.min(data.length - 1, index)));
            setMousePos({ x: mouseX - rect.left, y: mouseY - rect.top });
        } else {
            setHoveredIndex(null);
        }
    }, [data.length, chartWidth]);

    const hoveredPoint = hoveredIndex !== null ? points[hoveredIndex] : null;

    return (
        <div className="relative">
            <div className="flex items-center justify-between mb-2">
                <span className="text-sm font-medium text-gray-300">{label}</span>
                <span className="text-sm text-gray-400">
                    Current: <span className={`font-medium ${color.replace('stroke-', 'text-')}`}>
                        {formatValue(currentValue)}
                    </span>
                </span>
            </div>
            <div ref={containerRef} className="h-56 bg-background rounded border border-border relative"
                onMouseMove={handleMouseMove} onMouseLeave={() => setHoveredIndex(null)}>
                <svg viewBox={`0 0 ${width} ${height}`} preserveAspectRatio="xMinYMin meet" className="w-full h-full">
                    {yTicks.map((tick, i) => (
                        <g key={i}>
                            <line x1={paddingLeft} y1={tick.y} x2={width - paddingRight} y2={tick.y}
                                className="stroke-gray-700" strokeWidth="0.5" strokeDasharray={i === 0 ? "0" : "2,2"} />
                            <text x={paddingLeft - 8} y={tick.y + 3} textAnchor="end" className="fill-gray-500" fontSize="9">
                                {formatValue(tick.value)}
                            </text>
                        </g>
                    ))}
                    {xTicks.map((tick, i) => (
                        <text key={i} x={tick.x} y={height - 8} textAnchor="middle" className="fill-gray-500" fontSize="9">
                            {formatTime(tick.timestamp, duration)}
                        </text>
                    ))}
                    <line x1={paddingLeft} y1={paddingTop} x2={paddingLeft} y2={paddingTop + chartHeight} className="stroke-gray-600" strokeWidth="1" />
                    <line x1={paddingLeft} y1={paddingTop + chartHeight} x2={width - paddingRight} y2={paddingTop + chartHeight} className="stroke-gray-600" strokeWidth="1" />
                    <path d={areaPath} className={`${color.replace('stroke-', 'fill-')} opacity-20`} />
                    <path d={linePath} fill="none" className={color} strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
                    {hoveredPoint && (
                        <>
                            <line x1={hoveredPoint.x} y1={paddingTop} x2={hoveredPoint.x} y2={paddingTop + chartHeight}
                                className="stroke-gray-400" strokeWidth="1" strokeDasharray="4,2" />
                            <circle cx={hoveredPoint.x} cy={hoveredPoint.y} r="4"
                                className={`${color.replace('stroke-', 'fill-')}`} stroke="white" strokeWidth="2" />
                        </>
                    )}
                </svg>
                {hoveredPoint && (
                    <div className="absolute z-10 pointer-events-none bg-surface border border-border rounded-lg shadow-lg px-3 py-2"
                        style={{ left: Math.min(mousePos.x + 10, containerRef.current?.offsetWidth - 150 || 0), top: mousePos.y - 60 }}>
                        <div className="text-xs text-gray-400 mb-1">{new Date(hoveredPoint.timestamp).toLocaleString()}</div>
                        <div className={`text-sm font-medium ${color.replace('stroke-', 'text-')}`}>{formatValue(hoveredPoint.value)}</div>
                    </div>
                )}
            </div>
        </div>
    );
});

// Network I/O chart with bandwidth and packets view toggle
const NetworkChart = React.memo(({ data, duration }) => {
    const containerRef = useRef(null);
    const [hoveredIndex, setHoveredIndex] = useState(null);
    const [mousePos, setMousePos] = useState({ x: 0, y: 0 });
    const [viewMode, setViewMode] = useState('bandwidth');

    const hasRxBytes = data?.receiveBytes?.length > 0;
    const hasTxBytes = data?.transmitBytes?.length > 0;
    const hasRxPackets = data?.receivePackets?.length > 0;
    const hasTxPackets = data?.transmitPackets?.length > 0;

    const hasBandwidth = hasRxBytes || hasTxBytes;
    const hasPackets = hasRxPackets || hasTxPackets;

    if (!hasBandwidth && !hasPackets) {
        return (
            <div className="relative">
                <div className="text-sm font-medium text-gray-300 mb-2">Network I/O</div>
                <div className="h-56 flex items-center justify-center text-gray-500 text-sm bg-background rounded border border-border">
                    No data available
                </div>
            </div>
        );
    }

    const isBandwidthView = viewMode === 'bandwidth';
    const rx = isBandwidthView ? (data.receiveBytes || []) : (data.receivePackets || []);
    const tx = isBandwidthView ? (data.transmitBytes || []) : (data.transmitPackets || []);
    const rxDropped = data.receiveDropped || [];
    const txDropped = data.transmitDropped || [];
    const hasRx = isBandwidthView ? hasRxBytes : hasRxPackets;
    const hasTx = isBandwidthView ? hasTxBytes : hasTxPackets;

    const allValues = [...rx.map(d => d.value), ...tx.map(d => d.value)];
    const max = Math.max(...allValues, 1);
    const yMax = max * 1.1;
    const yRange = yMax || 1;

    const width = 500;
    const height = 200;
    const paddingLeft = 60;
    const paddingRight = 20;
    const paddingTop = 20;
    const paddingBottom = 30;
    const chartWidth = width - paddingLeft - paddingRight;
    const chartHeight = height - paddingTop - paddingBottom;

    const baseData = hasRx ? rx : tx;
    const generatePoints = (dataPoints) => {
        return dataPoints.map((d, i) => {
            const x = paddingLeft + (i / (dataPoints.length - 1)) * chartWidth;
            const y = paddingTop + chartHeight - (d.value / yRange) * chartHeight;
            return { x, y, value: d.value, timestamp: d.timestamp };
        });
    };

    const rxPoints = hasRx ? generatePoints(rx) : [];
    const txPoints = hasTx ? generatePoints(tx) : [];
    const createPath = (points) => points.map((p, i) => `${i === 0 ? 'M' : 'L'} ${p.x} ${p.y}`).join(' ');

    const handleMouseMove = useCallback((e) => {
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

    const formatRate = (value) => {
        if (value == null || isNaN(value)) return '-';
        return `${formatBytes(value)}/s`;
    };

    const formatPacketRate = (value) => {
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

    const yTicks = Array.from({ length: 5 }, (_, i) => {
        const value = (yRange * i) / 4;
        const y = paddingTop + chartHeight - (i / 4) * chartHeight;
        return { value, y };
    });

    const xTickIndices = baseData.length > 0 ? [0, Math.floor(baseData.length / 2), baseData.length - 1] : [];
    const xTicks = xTickIndices.map(i => ({
        timestamp: baseData[i]?.timestamp,
        x: paddingLeft + (i / (baseData.length - 1)) * chartWidth
    }));

    return (
        <div className="relative">
            <div className="flex items-center justify-between mb-2">
                <div className="flex items-center gap-3">
                    <span className="text-sm font-medium text-gray-300">Network I/O</span>
                    <div className="flex items-center gap-0.5 bg-gray-800 rounded p-0.5">
                        <button onClick={() => setViewMode('bandwidth')}
                            className={`px-1.5 py-0.5 text-[10px] rounded transition-colors ${viewMode === 'bandwidth' ? 'bg-gray-600 text-white' : 'text-gray-400 hover:text-gray-300'}`}>
                            Bytes
                        </button>
                        <button onClick={() => setViewMode('packets')}
                            className={`px-1.5 py-0.5 text-[10px] rounded transition-colors ${viewMode === 'packets' ? 'bg-gray-600 text-white' : 'text-gray-400 hover:text-gray-300'}`}>
                            Packets
                        </button>
                    </div>
                    <div className="flex items-center gap-2 text-xs">
                        {hasRx && <span className="flex items-center gap-1 text-cyan-400"><span className="w-3 h-0.5 bg-cyan-500"></span>RX</span>}
                        {hasTx && <span className="flex items-center gap-1 text-yellow-400"><span className="w-3 h-0.5 bg-yellow-500"></span>TX</span>}
                    </div>
                </div>
                <div className="text-xs flex items-center gap-2">
                    {hasRx && <span className="text-cyan-400">↓{formatValue(currentRx)}</span>}
                    {hasTx && <span className="text-yellow-400">↑{formatValue(currentTx)}</span>}
                    {!isBandwidthView && (currentRxDropped > 0 || currentTxDropped > 0) && (
                        <span className="text-red-400" title="Dropped packets">⚠ {formatPacketRate(currentRxDropped + currentTxDropped)} dropped</span>
                    )}
                </div>
            </div>
            <div ref={containerRef} className="h-56 bg-background rounded border border-border relative"
                onMouseMove={handleMouseMove} onMouseLeave={() => setHoveredIndex(null)}>
                <svg viewBox={`0 0 ${width} ${height}`} preserveAspectRatio="xMinYMin meet" className="w-full h-full">
                    {yTicks.map((tick, i) => (
                        <g key={i}>
                            <line x1={paddingLeft} y1={tick.y} x2={width - paddingRight} y2={tick.y}
                                className="stroke-gray-700" strokeWidth="0.5" strokeDasharray={i === 0 ? "0" : "2,2"} />
                            <text x={paddingLeft - 8} y={tick.y + 3} textAnchor="end" className="fill-gray-500" fontSize="9">
                                {formatValue(tick.value)}
                            </text>
                        </g>
                    ))}
                    {xTicks.map((tick, i) => (
                        <text key={i} x={tick.x} y={height - 8} textAnchor="middle" className="fill-gray-500" fontSize="9">
                            {tick.timestamp ? formatTime(tick.timestamp, duration) : ''}
                        </text>
                    ))}
                    <line x1={paddingLeft} y1={paddingTop} x2={paddingLeft} y2={paddingTop + chartHeight} className="stroke-gray-600" strokeWidth="1" />
                    <line x1={paddingLeft} y1={paddingTop + chartHeight} x2={width - paddingRight} y2={paddingTop + chartHeight} className="stroke-gray-600" strokeWidth="1" />
                    {rxPoints.length > 0 && <path d={createPath(rxPoints)} fill="none" className="stroke-cyan-500" strokeWidth="2" strokeLinecap="round" />}
                    {txPoints.length > 0 && <path d={createPath(txPoints)} fill="none" className="stroke-yellow-500" strokeWidth="2" strokeLinecap="round" />}
                    {hoveredRxPoint && <circle cx={hoveredRxPoint.x} cy={hoveredRxPoint.y} r="4" className="fill-cyan-500" stroke="white" strokeWidth="2" />}
                    {hoveredTxPoint && <circle cx={hoveredTxPoint.x} cy={hoveredTxPoint.y} r="4" className="fill-yellow-500" stroke="white" strokeWidth="2" />}
                </svg>
                {(hoveredRxPoint || hoveredTxPoint) && (
                    <div className="absolute z-10 pointer-events-none bg-surface border border-border rounded-lg shadow-lg px-3 py-2"
                        style={{ left: Math.min(mousePos.x + 10, containerRef.current?.offsetWidth - 160 || 0), top: mousePos.y - 70 }}>
                        <div className="text-xs text-gray-400 mb-1">{hoveredRxPoint?.timestamp ? new Date(hoveredRxPoint.timestamp).toLocaleString() : ''}</div>
                        {hoveredRxPoint && <div className="text-cyan-400">RX: {formatValue(hoveredRxPoint.value)}</div>}
                        {hoveredTxPoint && <div className="text-yellow-400">TX: {formatValue(hoveredTxPoint.value)}</div>}
                        {!isBandwidthView && (hoveredRxDropped || hoveredTxDropped) && (
                            <div className="text-red-400 text-[10px] mt-0.5">Dropped: ↓{formatPacketRate(hoveredRxDropped?.value || 0)} ↑{formatPacketRate(hoveredTxDropped?.value || 0)}</div>
                        )}
                    </div>
                )}
            </div>
        </div>
    );
});

// Simple count chart for pod count
const CountChart = React.memo(({ data, color, label, duration }) => {
    const containerRef = useRef(null);
    const [hoveredIndex, setHoveredIndex] = useState(null);
    const [mousePos, setMousePos] = useState({ x: 0, y: 0 });

    if (!data || data.length === 0) {
        return (
            <div className="h-56 flex items-center justify-center text-gray-500 text-sm bg-background rounded border border-border">
                No data available
            </div>
        );
    }

    const values = data.map(d => d.value);
    const max = Math.max(...values, 1);
    const min = Math.min(...values, 0);
    const range = max - min || 1;
    const yMin = Math.max(0, min - range * 0.1);
    const yMax = max + range * 0.1;
    const yRange = yMax - yMin || 1;

    const width = 500;
    const height = 200;
    const paddingLeft = 60;
    const paddingRight = 20;
    const paddingTop = 20;
    const paddingBottom = 30;
    const chartWidth = width - paddingLeft - paddingRight;
    const chartHeight = height - paddingTop - paddingBottom;

    const points = data.map((d, i) => {
        const x = paddingLeft + (i / (data.length - 1)) * chartWidth;
        const y = paddingTop + chartHeight - ((d.value - yMin) / yRange) * chartHeight;
        return { x, y, value: d.value, timestamp: d.timestamp };
    });

    const linePath = points.map((p, i) => `${i === 0 ? 'M' : 'L'} ${p.x} ${p.y}`).join(' ');

    const handleMouseMove = useCallback((e) => {
        if (!containerRef.current) return;
        const rect = containerRef.current.getBoundingClientRect();
        const zoom = parseFloat(document.body.style.zoom) || 1;
        const mouseX = e.clientX / zoom;

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
            setMousePos({ x: mouseX - rect.left, y: e.clientY / (parseFloat(document.body.style.zoom) || 1) - rect.top });
        } else {
            setHoveredIndex(null);
        }
    }, [data.length, chartWidth]);

    const currentValue = values[values.length - 1];
    const hoveredPoint = hoveredIndex !== null ? points[hoveredIndex] : null;

    const yTicks = Array.from({ length: 5 }, (_, i) => {
        const value = yMin + (yRange * i) / 4;
        const y = paddingTop + chartHeight - (i / 4) * chartHeight;
        return { value, y };
    });

    const xTickIndices = [0, Math.floor(data.length / 2), data.length - 1];
    const xTicks = xTickIndices.map(i => ({
        timestamp: data[i]?.timestamp,
        x: paddingLeft + (i / (data.length - 1)) * chartWidth
    }));

    return (
        <div className="relative">
            <div className="flex items-center justify-between mb-2">
                <span className="text-sm font-medium text-gray-300">{label}</span>
                <span className={`text-sm font-medium ${color.replace('stroke-', 'text-')}`}>{Math.round(currentValue)}</span>
            </div>
            <div ref={containerRef} className="h-56 bg-background rounded border border-border relative"
                onMouseMove={handleMouseMove} onMouseLeave={() => setHoveredIndex(null)}>
                <svg viewBox={`0 0 ${width} ${height}`} preserveAspectRatio="xMinYMin meet" className="w-full h-full">
                    {yTicks.map((tick, i) => (
                        <g key={i}>
                            <line x1={paddingLeft} y1={tick.y} x2={width - paddingRight} y2={tick.y}
                                className="stroke-gray-700" strokeWidth="0.5" strokeDasharray={i === 0 ? "0" : "2,2"} />
                            <text x={paddingLeft - 8} y={tick.y + 3} textAnchor="end" className="fill-gray-500" fontSize="9">
                                {Math.round(tick.value)}
                            </text>
                        </g>
                    ))}
                    {xTicks.map((tick, i) => (
                        <text key={i} x={tick.x} y={height - 8} textAnchor="middle" className="fill-gray-500" fontSize="9">
                            {formatTime(tick.timestamp, duration)}
                        </text>
                    ))}
                    <line x1={paddingLeft} y1={paddingTop} x2={paddingLeft} y2={paddingTop + chartHeight} className="stroke-gray-600" strokeWidth="1" />
                    <line x1={paddingLeft} y1={paddingTop + chartHeight} x2={width - paddingRight} y2={paddingTop + chartHeight} className="stroke-gray-600" strokeWidth="1" />
                    <path d={linePath} fill="none" className={color} strokeWidth="2" strokeLinecap="round" />
                    {hoveredPoint && (
                        <circle cx={hoveredPoint.x} cy={hoveredPoint.y} r="4"
                            className={`${color.replace('stroke-', 'fill-')}`} stroke="white" strokeWidth="2" />
                    )}
                </svg>
                {hoveredPoint && (
                    <div className="absolute z-10 pointer-events-none bg-surface border border-border rounded-lg shadow-lg px-3 py-2"
                        style={{ left: Math.min(mousePos.x + 10, containerRef.current?.offsetWidth - 100 || 0), top: mousePos.y - 50 }}>
                        <div className="text-xs text-gray-400 mb-1">{new Date(hoveredPoint.timestamp).toLocaleString()}</div>
                        <span className={color.replace('stroke-', 'text-')}>{Math.round(hoveredPoint.value)}</span>
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
];

export default function NamespaceMetricsTab({ namespace, isStale }) {
    const [prometheusInfo, setPrometheusInfo] = useState(null);
    const [detecting, setDetecting] = useState(true);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState(null);
    const [metricsData, setMetricsData] = useState(null);
    const [duration, setDuration] = useState('1h');
    const requestIdRef = useRef(0);

    const namespaceName = namespace.metadata?.name;

    useEffect(() => {
        const detect = async () => {
            try {
                const info = await DetectPrometheus();
                setPrometheusInfo(info);
            } catch (err) {
                setPrometheusInfo({ available: false });
            } finally {
                setDetecting(false);
            }
        };
        detect();
    }, []);

    useEffect(() => {
        if (!prometheusInfo?.available || isStale || !namespaceName) return;

        const currentRequestId = ++requestIdRef.current;
        const requestIdString = `namespace-metrics-${namespaceName}`;

        const fetchMetrics = async () => {
            setLoading(true);
            setError(null);
            try {
                const data = await GetNamespaceMetricsHistory(
                    requestIdString,
                    prometheusInfo.namespace,
                    prometheusInfo.service,
                    prometheusInfo.port,
                    namespaceName,
                    duration
                );
                if (currentRequestId === requestIdRef.current) {
                    setMetricsData(data);
                    setLoading(false);
                }
            } catch (err) {
                if (currentRequestId === requestIdRef.current) {
                    setError(err.toString());
                    setLoading(false);
                }
            }
        };

        fetchMetrics();
    }, [prometheusInfo, namespaceName, duration, isStale]);

    const formatCPU = (value) => {
        if (value == null || isNaN(value)) return '-';
        if (value < 1) return `${(value * 1000).toFixed(0)}µ`;
        if (value < 1000) return `${value.toFixed(0)}m`;
        return `${(value / 1000).toFixed(2)} cores`;
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
            <div className="flex items-center gap-4 px-4 py-3 border-b border-border shrink-0">
                <div className="flex items-center gap-2">
                    <div className="flex items-center gap-1 bg-surface-light rounded-md p-0.5">
                        {DURATIONS.map(d => (
                            <button key={d.value} onClick={() => setDuration(d.value)}
                                className={`px-3 py-1 text-xs font-medium rounded transition-colors ${duration === d.value ? 'bg-primary text-white' : 'text-gray-400 hover:text-white'}`}>
                                {d.label}
                            </button>
                        ))}
                    </div>
                    {loading && <div className="animate-spin rounded-full h-4 w-4 border-2 border-primary border-t-transparent" />}
                </div>
                <div className="ml-auto text-xs text-gray-500">
                    Prometheus: {prometheusInfo.namespace}/{prometheusInfo.service}
                </div>
            </div>

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
                        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
                            <MetricsChart data={metricsData.cpu} color="stroke-blue-500" label="CPU Usage (Total)" formatValue={formatCPU} duration={duration} />
                            <MetricsChart data={metricsData.memory} color="stroke-purple-500" label="Memory Usage (Total)" formatValue={formatBytes} duration={duration} />
                        </div>
                        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
                            <NetworkChart data={metricsData.network} duration={duration} />
                            <CountChart data={metricsData.podCount} color="stroke-green-500" label="Pod Count" duration={duration} />
                        </div>
                    </div>
                )}
            </div>
        </div>
    );
}
