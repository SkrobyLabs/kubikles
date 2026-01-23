import React, { useState, useEffect, useMemo, useRef, useCallback } from 'react';
import { ChartBarIcon, ExclamationTriangleIcon, ChevronDownIcon } from '@heroicons/react/24/outline';
import { DetectPrometheus, GetPodMetricsHistory } from '../../../wailsjs/go/main/App';
import { formatBytes } from '../../utils/formatting';

// Parse Kubernetes CPU quantity (e.g., "100m", "0.5", "1") to millicores
const parseCPU = (value) => {
    if (!value) return null;
    const str = String(value);
    if (str.endsWith('m')) {
        return parseFloat(str.slice(0, -1));
    }
    // Cores to millicores
    return parseFloat(str) * 1000;
};

// Parse Kubernetes memory quantity (e.g., "128Mi", "1Gi", "1000000") to bytes
const parseMemory = (value) => {
    if (!value) return null;
    const str = String(value);
    const units = {
        'Ki': 1024,
        'Mi': 1024 * 1024,
        'Gi': 1024 * 1024 * 1024,
        'Ti': 1024 * 1024 * 1024 * 1024,
        'K': 1000,
        'M': 1000 * 1000,
        'G': 1000 * 1000 * 1000,
        'T': 1000 * 1000 * 1000 * 1000,
    };
    for (const [suffix, multiplier] of Object.entries(units)) {
        if (str.endsWith(suffix)) {
            return parseFloat(str.slice(0, -suffix.length)) * multiplier;
        }
    }
    return parseFloat(str);
};

// Format time for display (timestamp is already in milliseconds from backend)
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

// Interactive line chart component with proper axes
// Memoized to prevent re-renders when parent updates with same props
const MetricsChart = React.memo(({ data, color, label, formatValue, duration, request, limit }) => {
    const containerRef = useRef(null);
    const [hoveredIndex, setHoveredIndex] = useState(null);
    const [mousePos, setMousePos] = useState({ x: 0, y: 0 });
    const [showRequest, setShowRequest] = useState(true);
    const [showLimit, setShowLimit] = useState(true);

    // Chart dimensions (constants) - wider aspect ratio for better horizontal usage
    const width = 500;
    const height = 200;
    const paddingLeft = 60;
    const paddingRight = 20;
    const paddingTop = 20;
    const paddingBottom = 30;
    const chartWidth = width - paddingLeft - paddingRight;
    const chartHeight = height - paddingTop - paddingBottom;

    // Memoize all expensive chart calculations
    const chartData = useMemo(() => {
        if (!data || data.length === 0) return null;

        const values = data.map(d => d.value);
        const timestamps = data.map(d => d.timestamp);
        let max = Math.max(...values) || 1;
        let min = Math.min(...values);

        // Extend max to include limit/request if they're higher and visible
        if (showLimit && limit && limit > max) max = limit * 1.05;
        if (showRequest && request && request > max) max = request * 1.05;

        const range = max - min || 1;
        const yMin = Math.max(0, min - range * 0.05);
        const yMax = max + range * 0.05;
        const yRange = yMax - yMin || 1;

        // Generate points for the line
        const points = data.map((d, i) => {
            const x = paddingLeft + (i / (data.length - 1)) * chartWidth;
            const y = paddingTop + chartHeight - ((d.value - yMin) / yRange) * chartHeight;
            return { x, y, value: d.value, timestamp: d.timestamp };
        });

        const linePath = points.map((p, i) => `${i === 0 ? 'M' : 'L'} ${p.x} ${p.y}`).join(' ');
        const areaPath = `${linePath} L ${points[points.length - 1].x} ${paddingTop + chartHeight} L ${paddingLeft} ${paddingTop + chartHeight} Z`;

        // Y axis ticks (5 ticks)
        const yTicks = Array.from({ length: 5 }, (_, i) => {
            const value = yMin + (yRange * i) / 4;
            const y = paddingTop + chartHeight - (i / 4) * chartHeight;
            return { value, y };
        });

        // X axis ticks (show first, middle, and last times)
        const xTickIndices = [0, Math.floor(data.length / 2), data.length - 1];
        const xTicks = xTickIndices.map(i => ({
            timestamp: timestamps[i],
            x: paddingLeft + (i / (data.length - 1)) * chartWidth
        }));

        // Y positions for request/limit lines
        const getYPos = (value) => {
            if (value == null || value < yMin || value > yMax) return null;
            return paddingTop + chartHeight - ((value - yMin) / yRange) * chartHeight;
        };

        return {
            points,
            linePath,
            areaPath,
            yTicks,
            xTicks,
            currentValue: values[values.length - 1],
            requestY: getYPos(request),
            limitY: getYPos(limit),
        };
    }, [data, showRequest, showLimit, request, limit, chartWidth, chartHeight]);

    if (!chartData) {
        return (
            <div className="h-56 flex items-center justify-center text-gray-500 text-sm bg-background rounded border border-border">
                No data available
            </div>
        );
    }

    const { points, linePath, areaPath, yTicks, xTicks, currentValue, requestY, limitY } = chartData;

    // Handle mouse move on the chart
    const handleMouseMove = useCallback((e) => {
        if (!containerRef.current) return;
        const rect = containerRef.current.getBoundingClientRect();
        // Account for CSS zoom applied to document body
        const zoom = parseFloat(document.body.style.zoom) || 1;
        const mouseX = e.clientX / zoom;
        const mouseY = e.clientY / zoom;

        // Calculate actual SVG render size (preserveAspectRatio="xMinYMin meet" maintains aspect ratio)
        const viewBoxAspect = width / height;
        const containerAspect = rect.width / rect.height;
        let svgRenderWidth, svgRenderHeight;
        if (containerAspect > viewBoxAspect) {
            // Container is wider - SVG is height-constrained
            svgRenderHeight = rect.height;
            svgRenderWidth = rect.height * viewBoxAspect;
        } else {
            // Container is taller - SVG is width-constrained
            svgRenderWidth = rect.width;
            svgRenderHeight = rect.width / viewBoxAspect;
        }

        const svgX = ((mouseX - rect.left) / svgRenderWidth) * width;
        const svgY = ((mouseY - rect.top) / svgRenderHeight) * height;

        if (svgX >= paddingLeft && svgX <= width - paddingRight &&
            svgY >= paddingTop && svgY <= height - paddingBottom) {
            const chartX = svgX - paddingLeft;
            const index = Math.round((chartX / chartWidth) * (data.length - 1));
            const clampedIndex = Math.max(0, Math.min(data.length - 1, index));
            setHoveredIndex(clampedIndex);
            setMousePos({ x: mouseX - rect.left, y: mouseY - rect.top });
        } else {
            setHoveredIndex(null);
        }
    }, [data.length, chartWidth]);

    const handleMouseLeave = useCallback(() => {
        setHoveredIndex(null);
    }, []);

    const hoveredPoint = hoveredIndex !== null ? points[hoveredIndex] : null;

    return (
        <div className="relative">
            <div className="flex items-center justify-between mb-2">
                <div className="flex items-center gap-3">
                    <span className="text-sm font-medium text-gray-300">{label}</span>
                    {/* Legend - clickable to toggle */}
                    <div className="flex items-center gap-2 text-xs">
                        {request != null && (
                            <button
                                onClick={() => setShowRequest(!showRequest)}
                                className={`flex items-center gap-1 px-1.5 py-0.5 rounded transition-colors hover:bg-white/10 ${!showRequest ? 'opacity-40' : ''}`}
                                title={showRequest ? 'Hide request line' : 'Show request line'}
                            >
                                <span className={`w-3 h-0.5 ${showRequest ? 'bg-orange-500' : 'bg-gray-500'}`}></span>
                                <span className={showRequest ? 'text-orange-400' : 'text-gray-500'}>Request</span>
                            </button>
                        )}
                        {limit != null && (
                            <button
                                onClick={() => setShowLimit(!showLimit)}
                                className={`flex items-center gap-1 px-1.5 py-0.5 rounded transition-colors hover:bg-white/10 ${!showLimit ? 'opacity-40' : ''}`}
                                title={showLimit ? 'Hide limit line' : 'Show limit line'}
                            >
                                <span className={`w-3 h-0.5 ${showLimit ? 'bg-red-500' : 'bg-gray-500'}`}></span>
                                <span className={showLimit ? 'text-red-400' : 'text-gray-500'}>Limit</span>
                            </button>
                        )}
                    </div>
                </div>
                <span className="text-sm text-gray-400">
                    Current: <span className={`font-medium ${color.replace('stroke-', 'text-')}`}>
                        {formatValue(currentValue)}
                    </span>
                </span>
            </div>
            <div
                ref={containerRef}
                className="h-56 bg-background rounded border border-border relative"
                onMouseMove={handleMouseMove}
                onMouseLeave={handleMouseLeave}
            >
                <svg viewBox={`0 0 ${width} ${height}`} preserveAspectRatio="xMinYMin meet" className="w-full h-full">
                    {/* Y axis grid lines */}
                    {yTicks.map((tick, i) => (
                        <g key={i}>
                            <line
                                x1={paddingLeft}
                                y1={tick.y}
                                x2={width - paddingRight}
                                y2={tick.y}
                                className="stroke-gray-700"
                                strokeWidth="0.5"
                                strokeDasharray={i === 0 ? "0" : "2,2"}
                            />
                            <text
                                x={paddingLeft - 8}
                                y={tick.y + 3}
                                textAnchor="end"
                                className="fill-gray-500"
                                fontSize="9"
                            >
                                {formatValue(tick.value)}
                            </text>
                        </g>
                    ))}

                    {/* X axis labels */}
                    {xTicks.map((tick, i) => (
                        <text
                            key={i}
                            x={tick.x}
                            y={height - 8}
                            textAnchor="middle"
                            className="fill-gray-500"
                            fontSize="9"
                        >
                            {formatTime(tick.timestamp, duration)}
                        </text>
                    ))}

                    {/* Y axis line */}
                    <line
                        x1={paddingLeft}
                        y1={paddingTop}
                        x2={paddingLeft}
                        y2={paddingTop + chartHeight}
                        className="stroke-gray-600"
                        strokeWidth="1"
                    />

                    {/* X axis line */}
                    <line
                        x1={paddingLeft}
                        y1={paddingTop + chartHeight}
                        x2={width - paddingRight}
                        y2={paddingTop + chartHeight}
                        className="stroke-gray-600"
                        strokeWidth="1"
                    />

                    {/* Request line (orange) */}
                    {showRequest && requestY != null && (
                        <line
                            x1={paddingLeft}
                            y1={requestY}
                            x2={width - paddingRight}
                            y2={requestY}
                            className="stroke-orange-500"
                            strokeWidth="1.5"
                            strokeDasharray="6,3"
                        />
                    )}

                    {/* Limit line (red) */}
                    {showLimit && limitY != null && (
                        <line
                            x1={paddingLeft}
                            y1={limitY}
                            x2={width - paddingRight}
                            y2={limitY}
                            className="stroke-red-500"
                            strokeWidth="1.5"
                            strokeDasharray="6,3"
                        />
                    )}

                    {/* Area fill */}
                    <path
                        d={areaPath}
                        className={`${color.replace('stroke-', 'fill-')} opacity-20`}
                    />

                    {/* Line */}
                    <path
                        d={linePath}
                        fill="none"
                        className={color}
                        strokeWidth="2"
                        strokeLinecap="round"
                        strokeLinejoin="round"
                    />

                    {/* Hover indicator */}
                    {hoveredPoint && (
                        <>
                            <line
                                x1={hoveredPoint.x}
                                y1={paddingTop}
                                x2={hoveredPoint.x}
                                y2={paddingTop + chartHeight}
                                className="stroke-gray-400"
                                strokeWidth="1"
                                strokeDasharray="4,2"
                            />
                            <circle
                                cx={hoveredPoint.x}
                                cy={hoveredPoint.y}
                                r="4"
                                className={`${color.replace('stroke-', 'fill-')}`}
                                stroke="white"
                                strokeWidth="2"
                            />
                        </>
                    )}
                </svg>

                {/* Tooltip */}
                {hoveredPoint && (
                    <div
                        className="absolute z-10 pointer-events-none bg-surface border border-border rounded-lg shadow-lg px-3 py-2"
                        style={{
                            left: Math.min(mousePos.x + 10, containerRef.current?.offsetWidth - 150 || 0),
                            top: mousePos.y - 60
                        }}
                    >
                        <div className="text-xs text-gray-400 mb-1">
                            {new Date(hoveredPoint.timestamp).toLocaleString()}
                        </div>
                        <div className={`text-sm font-medium ${color.replace('stroke-', 'text-')}`}>
                            {formatValue(hoveredPoint.value)}
                        </div>
                        {showRequest && request != null && (
                            <div className="text-xs text-orange-400">
                                Request: {formatValue(request)}
                            </div>
                        )}
                        {showLimit && limit != null && (
                            <div className="text-xs text-red-400">
                                Limit: {formatValue(limit)}
                            </div>
                        )}
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
    const [viewMode, setViewMode] = useState('bandwidth'); // 'bandwidth' or 'packets'

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

    // Select data based on view mode
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

    // Y axis ticks
    const yTicks = Array.from({ length: 5 }, (_, i) => {
        const value = (yRange * i) / 4;
        const y = paddingTop + chartHeight - (i / 4) * chartHeight;
        return { value, y };
    });

    // X axis ticks
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
            <div ref={containerRef} className="h-56 bg-background rounded border border-border relative"
                onMouseMove={handleMouseMove} onMouseLeave={() => setHoveredIndex(null)}>
                <svg viewBox={`0 0 ${width} ${height}`} preserveAspectRatio="xMinYMin meet" className="w-full h-full">
                    {/* Y axis grid lines */}
                    {yTicks.map((tick, i) => (
                        <g key={i}>
                            <line x1={paddingLeft} y1={tick.y} x2={width - paddingRight} y2={tick.y}
                                className="stroke-gray-700" strokeWidth="0.5" strokeDasharray={i === 0 ? "0" : "2,2"} />
                            <text x={paddingLeft - 8} y={tick.y + 3} textAnchor="end" className="fill-gray-500" fontSize="9">
                                {formatValue(tick.value)}
                            </text>
                        </g>
                    ))}

                    {/* X axis labels */}
                    {xTicks.map((tick, i) => (
                        <text key={i} x={tick.x} y={height - 8} textAnchor="middle" className="fill-gray-500" fontSize="9">
                            {tick.timestamp ? new Date(tick.timestamp).toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' }) : ''}
                        </text>
                    ))}

                    {/* Axes */}
                    <line x1={paddingLeft} y1={paddingTop} x2={paddingLeft} y2={paddingTop + chartHeight} className="stroke-gray-600" strokeWidth="1" />
                    <line x1={paddingLeft} y1={paddingTop + chartHeight} x2={width - paddingRight} y2={paddingTop + chartHeight} className="stroke-gray-600" strokeWidth="1" />

                    {/* RX line (cyan) */}
                    {rxPoints.length > 0 && (
                        <path d={createPath(rxPoints)} fill="none" className="stroke-cyan-500" strokeWidth="2" strokeLinecap="round" />
                    )}

                    {/* TX line (yellow) */}
                    {txPoints.length > 0 && (
                        <path d={createPath(txPoints)} fill="none" className="stroke-yellow-500" strokeWidth="2" strokeLinecap="round" />
                    )}

                    {hoveredRxPoint && (
                        <circle cx={hoveredRxPoint.x} cy={hoveredRxPoint.y} r="4"
                            className="fill-cyan-500" stroke="white" strokeWidth="2" />
                    )}
                    {hoveredTxPoint && (
                        <circle cx={hoveredTxPoint.x} cy={hoveredTxPoint.y} r="4"
                            className="fill-yellow-500" stroke="white" strokeWidth="2" />
                    )}
                </svg>
                {(hoveredRxPoint || hoveredTxPoint) && (
                    <div className="absolute z-10 pointer-events-none bg-surface border border-border rounded-lg shadow-lg px-3 py-2"
                        style={{ left: Math.min(mousePos.x + 10, containerRef.current?.offsetWidth - 160 || 0), top: mousePos.y - 70 }}>
                        <div className="text-xs text-gray-400 mb-1">
                            {hoveredRxPoint?.timestamp ? new Date(hoveredRxPoint.timestamp).toLocaleString() : ''}
                        </div>
                        {hoveredRxPoint && <div className="text-cyan-400">RX: {formatValue(hoveredRxPoint.value)}</div>}
                        {hoveredTxPoint && <div className="text-yellow-400">TX: {formatValue(hoveredTxPoint.value)}</div>}
                        {!isBandwidthView && (hoveredRxDropped || hoveredTxDropped) && (
                            <div className="text-red-400 text-[10px] mt-0.5">
                                Dropped: ↓{formatPacketRate(hoveredRxDropped?.value || 0)} ↑{formatPacketRate(hoveredTxDropped?.value || 0)}
                            </div>
                        )}
                    </div>
                )}
            </div>
        </div>
    );
});

// Duration options
const DURATIONS = [
    { value: '1h', label: '1h' },
    { value: '6h', label: '6h' },
    { value: '24h', label: '24h' },
    { value: '7d', label: '7d' },
    { value: '30d', label: '30d' },
    { value: 'all', label: 'All' },
];

export default function PodMetricsTab({ pod, isStale }) {
    const [prometheusInfo, setPrometheusInfo] = useState(null);
    const [detecting, setDetecting] = useState(true);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState(null);
    const [metricsData, setMetricsData] = useState(null);
    const [duration, setDuration] = useState('1h');
    const [selectedContainer, setSelectedContainer] = useState('all');
    const [dropdownOpen, setDropdownOpen] = useState(false);
    const requestIdRef = useRef(0); // Track current request to cancel stale ones

    const namespace = pod.metadata?.namespace;
    const podName = pod.metadata?.name;

    // Get container names and resources
    const containers = useMemo(() => {
        const initContainers = (pod.spec?.initContainers || []).map(c => ({
            name: c.name,
            isInit: true,
            resources: c.resources || {}
        }));
        const regularContainers = (pod.spec?.containers || []).map(c => ({
            name: c.name,
            isInit: false,
            resources: c.resources || {}
        }));
        return [...initContainers, ...regularContainers];
    }, [pod]);

    // Get resources for a specific container
    const getContainerResources = useCallback((containerName) => {
        const container = containers.find(c => c.name === containerName);
        if (!container) return { cpuRequest: null, cpuLimit: null, memRequest: null, memLimit: null };

        return {
            cpuRequest: parseCPU(container.resources?.requests?.cpu),
            cpuLimit: parseCPU(container.resources?.limits?.cpu),
            memRequest: parseMemory(container.resources?.requests?.memory),
            memLimit: parseMemory(container.resources?.limits?.memory),
        };
    }, [containers]);

    // Detect Prometheus on mount
    useEffect(() => {
        const detect = async () => {
            try {
                const info = await DetectPrometheus();
                setPrometheusInfo(info);
            } catch (err) {
                console.error('Failed to detect Prometheus:', err);
                setPrometheusInfo({ available: false });
            } finally {
                setDetecting(false);
            }
        };
        detect();
    }, []);

    // Fetch metrics when prometheus is available and params change
    useEffect(() => {
        if (!prometheusInfo?.available || isStale) return;

        // Increment request ID to invalidate any in-flight requests
        const currentRequestId = ++requestIdRef.current;
        // Use stable ID for backend cancellation (without counter)
        const requestIdString = `pod-metrics-${namespace}-${podName}`;

        const fetchMetrics = async () => {
            setLoading(true);
            setError(null);
            try {
                const data = await GetPodMetricsHistory(
                    requestIdString,
                    prometheusInfo.namespace,
                    prometheusInfo.service,
                    prometheusInfo.port,
                    namespace,
                    podName,
                    selectedContainer,
                    duration
                );
                // Only update state if this request is still current
                if (currentRequestId === requestIdRef.current) {
                    setMetricsData(data);
                    setLoading(false);
                }
            } catch (err) {
                // Only update state if this request is still current
                if (currentRequestId === requestIdRef.current) {
                    console.error('Failed to fetch metrics:', err);
                    setError(err.toString());
                    setLoading(false);
                }
            }
        };

        fetchMetrics();
    }, [prometheusInfo, namespace, podName, selectedContainer, duration, isStale]);

    // Format CPU (millicores)
    const formatCPU = (value) => {
        if (value < 1) return `${(value * 1000).toFixed(0)}µ`;
        if (value < 1000) return `${value.toFixed(1)}m`;
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
                    Historical metrics require a Prometheus installation in your cluster.
                    Install kube-prometheus-stack or configure a custom endpoint in the Metrics settings.
                </p>
            </div>
        );
    }

    if (isStale) {
        return (
            <div className="flex items-center justify-center h-full text-gray-500 p-8">
                <div className="flex items-center gap-2">
                    <ExclamationTriangleIcon className="h-5 w-5 text-yellow-500" />
                    <span>Metrics unavailable for pods from a different context</span>
                </div>
            </div>
        );
    }

    const selectedContainerInfo = containers.find(c => c.name === selectedContainer);

    return (
        <div className="h-full flex flex-col overflow-hidden">
            {/* Controls */}
            <div className="flex items-center gap-4 px-4 py-3 border-b border-border shrink-0">
                {/* Container selector */}
                <div className="flex items-center gap-2">
                    <label className="text-xs font-medium text-gray-500 uppercase tracking-wider">
                        Container
                    </label>
                    <div className="relative">
                        <button
                            onClick={() => setDropdownOpen(!dropdownOpen)}
                            className="flex items-center gap-2 px-3 py-1.5 bg-surface border border-border rounded-lg text-sm hover:bg-surface-light transition-colors min-w-[180px]"
                        >
                            <span className="flex-1 text-left text-gray-200">
                                {selectedContainer === 'all' ? 'All Containers' : selectedContainer}
                                {selectedContainerInfo?.isInit && <span className="ml-2 text-xs text-yellow-400">(init)</span>}
                            </span>
                            <ChevronDownIcon className={`w-4 h-4 text-gray-400 transition-transform ${dropdownOpen ? 'rotate-180' : ''}`} />
                        </button>
                        {dropdownOpen && (
                            <div className="absolute top-full left-0 mt-1 w-full bg-surface border border-border rounded-lg shadow-lg z-10 py-1 max-h-60 overflow-auto">
                                <button
                                    onClick={() => {
                                        setSelectedContainer('all');
                                        setDropdownOpen(false);
                                    }}
                                    className={`w-full px-3 py-2 text-sm text-left hover:bg-white/5 ${
                                        selectedContainer === 'all' ? 'bg-primary/10 text-primary' : 'text-gray-300'
                                    }`}
                                >
                                    All Containers
                                </button>
                                {containers.map((c) => (
                                    <button
                                        key={c.name}
                                        onClick={() => {
                                            setSelectedContainer(c.name);
                                            setDropdownOpen(false);
                                        }}
                                        className={`w-full px-3 py-2 text-sm text-left hover:bg-white/5 ${
                                            c.name === selectedContainer ? 'bg-primary/10 text-primary' : 'text-gray-300'
                                        }`}
                                    >
                                        {c.name}
                                        {c.isInit && <span className="ml-2 text-xs text-yellow-400">(init)</span>}
                                    </button>
                                ))}
                            </div>
                        )}
                    </div>
                </div>

                {/* Duration selector */}
                <div className="flex items-center gap-2">
                    <div className="flex items-center gap-1 bg-surface-light rounded-md p-0.5">
                        {DURATIONS.map(d => (
                            <button
                                key={d.value}
                                onClick={() => setDuration(d.value)}
                                className={`px-3 py-1 text-xs font-medium rounded transition-colors ${
                                    duration === d.value
                                        ? 'bg-primary text-white'
                                        : 'text-gray-400 hover:text-white'
                                }`}
                            >
                                {d.label}
                            </button>
                        ))}
                    </div>
                    {loading && (
                        <div className="animate-spin rounded-full h-4 w-4 border-2 border-primary border-t-transparent" />
                    )}
                </div>

                {/* Prometheus info */}
                <div className="ml-auto text-xs text-gray-500">
                    Prometheus: {prometheusInfo.namespace}/{prometheusInfo.service}
                </div>
            </div>

            {/* Content */}
            <div className="flex-1 overflow-auto p-4">
                {loading && !metricsData && (
                    <div className="flex items-center justify-center h-full text-gray-500">
                        <div className="flex items-center gap-2">
                            <div className="animate-spin rounded-full h-5 w-5 border-b-2 border-primary"></div>
                            Loading metrics...
                        </div>
                    </div>
                )}

                {error && (
                    <div className="flex items-center gap-2 p-4 bg-red-500/10 border border-red-500/30 rounded text-red-400">
                        <ExclamationTriangleIcon className="h-5 w-5" />
                        <span className="text-sm">{error}</span>
                    </div>
                )}

                {metricsData && (
                    <div className="space-y-6">
                        {metricsData.containers && metricsData.containers.length > 0 ? (
                            <>
                                {metricsData.containers.map(container => {
                                    const resources = getContainerResources(container.container);
                                    return (
                                        <div key={container.container} className="space-y-4">
                                            {selectedContainer === 'all' && (
                                                <h3 className="text-sm font-medium text-gray-300 border-b border-border pb-2">
                                                    {container.container}
                                                </h3>
                                            )}
                                            <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
                                                <MetricsChart
                                                    data={container.cpu}
                                                    color="stroke-blue-500"
                                                    label="CPU Usage"
                                                    formatValue={formatCPU}
                                                    duration={duration}
                                                    request={resources.cpuRequest}
                                                    limit={resources.cpuLimit}
                                                />
                                                <MetricsChart
                                                    data={container.memory}
                                                    color="stroke-purple-500"
                                                    label="Memory Usage"
                                                    formatValue={formatBytes}
                                                    duration={duration}
                                                    request={resources.memRequest}
                                                    limit={resources.memLimit}
                                                />
                                            </div>
                                        </div>
                                    );
                                })}
                                {/* Network metrics (pod-level) */}
                                {metricsData.network && (
                                    <div className="pt-2 border-t border-border">
                                        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
                                            <NetworkChart
                                                data={metricsData.network}
                                                duration={duration}
                                            />
                                        </div>
                                    </div>
                                )}
                            </>
                        ) : (
                            <div className="flex items-center justify-center h-48 text-gray-500">
                                No metrics data available for this time range
                            </div>
                        )}
                    </div>
                )}
            </div>
        </div>
    );
}
