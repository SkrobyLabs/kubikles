import React, { useState, useEffect, useMemo, useRef, useCallback } from 'react';
import { ChartBarIcon, ExclamationTriangleIcon, ChevronDownIcon } from '@heroicons/react/24/outline';
import { ArrowUturnLeftIcon } from '@heroicons/react/24/outline';
import { DetectPrometheus, GetPodMetricsHistory, GetPodMetricsHistoryRange, GetMetricsEventMarkers } from 'wailsjs/go/main/App';
import { formatBytes } from '~/utils/formatting';

interface EventMarker {
    timestamp: number;
    reason: string;
    severity: string; // "error" | "warning" | "info"
    message: string;
    kind: string;
}

const MARKER_COLORS: Record<string, { line: string; fill: string; text: string }> = {
    error: { line: 'stroke-red-500', fill: 'fill-red-500', text: 'text-red-400' },
    warning: { line: 'stroke-amber-500', fill: 'fill-amber-500', text: 'text-amber-400' },
    info: { line: 'stroke-gray-500', fill: 'fill-gray-500', text: 'text-gray-400' },
};

// Parse Kubernetes CPU quantity (e.g., "100m", "0.5", "1") to millicores
const parseCPU = (value: any) => {
    if (!value) return null;
    const str = String(value);
    if (str.endsWith('m')) {
        return parseFloat(str.slice(0, -1));
    }
    // Cores to millicores
    return parseFloat(str) * 1000;
};

// Parse Kubernetes memory quantity (e.g., "128Mi", "1Gi", "1000000") to bytes
const parseMemory = (value: any) => {
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
const formatTime = (timestamp: string, duration: string) => {
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
const MetricsChart = React.memo(({ data, color, label, formatValue, duration, request, limit, markers, onZoomSelect }: { data: any; color: string; label: string; formatValue: (value: number) => string; duration: string; request?: any; limit?: any; markers?: EventMarker[]; onZoomSelect?: (startMs: number, endMs: number) => void }) => {
    const containerRef = useRef<HTMLDivElement>(null);
    const [hoveredIndex, setHoveredIndex] = useState<number | null>(null);
    const [mousePos, setMousePos] = useState({ x: 0, y: 0 });
    const [showRequest, setShowRequest] = useState(true);
    const [showLimit, setShowLimit] = useState(true);
    const [showMarkers, setShowMarkers] = useState(true);
    const [containerWidth, setContainerWidth] = useState(500);
    const [isDragging, setIsDragging] = useState(false);
    const [dragStartIndex, setDragStartIndex] = useState<number | null>(null);
    const [dragEndIndex, setDragEndIndex] = useState<number | null>(null);

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

    // Chart dimensions - width from container, fixed height
    const width = Math.max(300, containerWidth);
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

        const values = data.map((d: any) => d.value);
        const timestamps = data.map((d: any) => d.timestamp);
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
        const points = data.map((d: any, i: number) => {
            const x = paddingLeft + (i / (data.length - 1)) * chartWidth;
            const y = paddingTop + chartHeight - ((d.value - yMin) / yRange) * chartHeight;
            return { x, y, value: d.value, timestamp: d.timestamp };
        });

        const linePath = points.map((p: any, i: number) => `${i === 0 ? 'M' : 'L'} ${p.x} ${p.y}`).join(' ');
        const areaPath = `${linePath} L ${points[points.length - 1].x} ${paddingTop + chartHeight} L ${paddingLeft} ${paddingTop + chartHeight} Z`;

        // Y axis ticks (5 ticks)
        const yTicks = Array.from({ length: 5 }, (_, i) => {
            const value = yMin + (yRange * i) / 4;
            const y = paddingTop + chartHeight - (i / 4) * chartHeight;
            return { value, y };
        });

        // X axis ticks (show first, middle, and last times)
        const xTickIndices = [0, Math.floor(data.length / 2), data.length - 1];
        const xTicks = xTickIndices.map((i: any) => ({
            timestamp: timestamps[i],
            x: paddingLeft + (i / (data.length - 1)) * chartWidth
        }));

        // Y positions for request/limit lines
        const getYPos = (value: any) => {
            if (value == null || value < yMin || value > yMax) return null;
            return paddingTop + chartHeight - ((value - yMin) / yRange) * chartHeight;
        };

        // Compute marker x-positions by mapping timestamps to nearest data point
        const markerPositions = (markers || []).map(m => {
            let closestIdx = 0;
            let closestDist = Math.abs(Number(timestamps[0]) - m.timestamp);
            for (let i = 1; i < timestamps.length; i++) {
                const dist = Math.abs(Number(timestamps[i]) - m.timestamp);
                if (dist < closestDist) {
                    closestDist = dist;
                    closestIdx = i;
                }
            }
            return { ...m, x: points[closestIdx].x, dataIndex: closestIdx };
        });

        return {
            points,
            linePath,
            areaPath,
            yTicks,
            xTicks,
            currentValue: values[values.length - 1],
            requestY: getYPos(request),
            limitY: getYPos(limit),
            markerPositions,
        };
    }, [data, showRequest, showLimit, request, limit, chartWidth, chartHeight, markers]);

    if (!chartData) {
        return (
            <div className="h-56 flex items-center justify-center text-gray-500 text-sm bg-background rounded border border-border">
                No data available
            </div>
        );
    }

    const { points, linePath, areaPath, yTicks, xTicks, currentValue, requestY, limitY, markerPositions } = chartData;

    // Find marker near hovered index (within ±1 data index)
    const hoveredMarker = hoveredIndex !== null && showMarkers
        ? markerPositions.find((m: any) => Math.abs(m.dataIndex - hoveredIndex) <= 1)
        : null;

    // Compute data index from mouse event
    const getDataIndex = useCallback((e: any): number | null => {
        if (!containerRef.current || !data || data.length === 0) return null;
        const rect = containerRef.current.getBoundingClientRect();
        const zoom = parseFloat(document.body.style.zoom) || 1;
        const mouseX = e.clientX / zoom;
        const mouseY = e.clientY / zoom;

        const viewBoxAspect = width / height;
        const containerAspect = rect.width / rect.height;
        let svgRenderWidth, svgRenderHeight;
        if (containerAspect > viewBoxAspect) {
            svgRenderHeight = rect.height;
            svgRenderWidth = rect.height * viewBoxAspect;
        } else {
            svgRenderWidth = rect.width;
            svgRenderHeight = rect.width / viewBoxAspect;
        }

        const svgX = ((mouseX - rect.left) / svgRenderWidth) * width;
        const svgY = ((mouseY - rect.top) / svgRenderHeight) * height;

        if (svgX >= paddingLeft && svgX <= width - paddingRight &&
            svgY >= paddingTop && svgY <= height - paddingBottom) {
            const chartX = svgX - paddingLeft;
            const index = Math.round((chartX / chartWidth) * (data.length - 1));
            return Math.max(0, Math.min(data.length - 1, index));
        }
        return null;
    }, [data?.length, chartWidth, width, height]);

    const handleMouseMove = useCallback((e: any) => {
        const index = getDataIndex(e);
        if (isDragging) {
            if (index !== null) setDragEndIndex(index);
            return;
        }
        if (index !== null) {
            const rect = containerRef.current!.getBoundingClientRect();
            const zoom = parseFloat(document.body.style.zoom) || 1;
            setHoveredIndex(index);
            setMousePos({ x: e.clientX / zoom - rect.left, y: e.clientY / zoom - rect.top });
        } else {
            setHoveredIndex(null);
        }
    }, [getDataIndex, isDragging]);

    const handleMouseDown = useCallback((e: any) => {
        if (!onZoomSelect) return;
        const index = getDataIndex(e);
        if (index !== null) {
            setIsDragging(true);
            setDragStartIndex(index);
            setDragEndIndex(index);
            setHoveredIndex(null);
        }
    }, [getDataIndex, onZoomSelect]);

    const handleMouseUp = useCallback(() => {
        if (!isDragging || dragStartIndex == null || dragEndIndex == null || !data || !onZoomSelect) {
            setIsDragging(false);
            setDragStartIndex(null);
            setDragEndIndex(null);
            return;
        }
        const minIdx = Math.min(dragStartIndex, dragEndIndex);
        const maxIdx = Math.max(dragStartIndex, dragEndIndex);
        setIsDragging(false);
        setDragStartIndex(null);
        setDragEndIndex(null);
        if (maxIdx - minIdx >= 2) {
            const startMs = Number(data[minIdx].timestamp);
            const endMs = Number(data[maxIdx].timestamp);
            onZoomSelect(startMs, endMs);
        }
    }, [isDragging, dragStartIndex, dragEndIndex, data, onZoomSelect]);

    const handleMouseLeave = useCallback(() => {
        setHoveredIndex(null);
        if (isDragging) {
            setIsDragging(false);
            setDragStartIndex(null);
            setDragEndIndex(null);
        }
    }, [isDragging]);

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
                        {markerPositions.length > 0 && (
                            <button
                                onClick={() => setShowMarkers(!showMarkers)}
                                className={`flex items-center gap-1 px-1.5 py-0.5 rounded transition-colors hover:bg-white/10 ${!showMarkers ? 'opacity-40' : ''}`}
                                title={showMarkers ? 'Hide event markers' : 'Show event markers'}
                            >
                                <span className={`w-1.5 h-1.5 rotate-45 ${showMarkers ? 'bg-amber-500' : 'bg-gray-500'}`}></span>
                                <span className={showMarkers ? 'text-amber-400' : 'text-gray-500'}>Events</span>
                                <span className={`text-[10px] px-1 rounded-full ${showMarkers ? 'bg-amber-500/20 text-amber-400' : 'bg-gray-700 text-gray-500'}`}>{markerPositions.length}</span>
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
                className={`h-56 bg-background rounded border border-border relative ${onZoomSelect ? 'cursor-crosshair' : ''}`}
                onMouseMove={handleMouseMove}
                onMouseDown={handleMouseDown}
                onMouseUp={handleMouseUp}
                onMouseLeave={handleMouseLeave}
            >
                <svg viewBox={`0 0 ${width} ${height}`} preserveAspectRatio="xMinYMin meet" className="w-full h-full select-none">
                    {/* Y axis grid lines */}
                    {yTicks.map((tick: any, i: number) => (
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
                    {xTicks.map((tick: any, i: number) => (
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

                    {/* Event markers */}
                    {showMarkers && markerPositions.map((m: any, i: number) => {
                        const mc = MARKER_COLORS[m.severity] || MARKER_COLORS.info;
                        return (
                            <g key={i}>
                                <line
                                    x1={m.x} y1={paddingTop} x2={m.x} y2={paddingTop + chartHeight}
                                    className={mc.line} strokeWidth="1" strokeDasharray="3,3" opacity="0.7"
                                />
                                <polygon
                                    points={`${m.x},${paddingTop - 1} ${m.x + 4},${paddingTop + 4} ${m.x},${paddingTop + 9} ${m.x - 4},${paddingTop + 4}`}
                                    className={mc.fill} opacity="0.9"
                                />
                            </g>
                        );
                    })}

                    {/* Drag selection overlay */}
                    {isDragging && dragStartIndex != null && dragEndIndex != null && points.length > 0 && (() => {
                        const x1 = points[Math.min(dragStartIndex, dragEndIndex)]?.x ?? 0;
                        const x2 = points[Math.max(dragStartIndex, dragEndIndex)]?.x ?? 0;
                        return (
                            <rect x={Math.min(x1, x2)} y={paddingTop} width={Math.abs(x2 - x1)} height={chartHeight}
                                className="fill-blue-500" opacity={0.15} />
                        );
                    })()}

                    {/* Hover indicator */}
                    {!isDragging && hoveredPoint && (
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
                {!isDragging && hoveredPoint && (
                    <div
                        className="absolute z-10 pointer-events-none bg-surface border border-border rounded-lg shadow-lg px-3 py-2"
                        style={{
                            left: Math.min(mousePos.x + 10, (containerRef.current?.offsetWidth ?? 0) - 150 || 0),
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
                        {hoveredMarker && (
                            <div className="border-t border-border mt-1 pt-1">
                                <div className={`text-xs font-medium ${MARKER_COLORS[hoveredMarker.severity]?.text || 'text-gray-400'}`}>
                                    {hoveredMarker.reason}
                                </div>
                                <div className="text-[10px] text-gray-500 max-w-[200px] truncate">{hoveredMarker.message}</div>
                            </div>
                        )}
                    </div>
                )}
            </div>
        </div>
    );
});

// Network I/O chart with bandwidth and packets view toggle
const NetworkChart = React.memo(({ data, duration }: { data: any; duration: string }) => {
    const containerRef = useRef<HTMLDivElement>(null);
    const [hoveredIndex, setHoveredIndex] = useState<number | null>(null);
    const [mousePos, setMousePos] = useState({ x: 0, y: 0 });
    const [viewMode, setViewMode] = useState('bandwidth'); // 'bandwidth' or 'packets'
    const [containerWidth, setContainerWidth] = useState(500);

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

    const allValues = [...rx.map((d: any) => d.value), ...tx.map((d: any) => d.value)];
    const max = Math.max(...allValues, 1);
    const yMax = max * 1.1;
    const yRange = yMax || 1;

    const width = Math.max(300, containerWidth);
    const height = 200;
    const paddingLeft = 60;
    const paddingRight = 20;
    const paddingTop = 20;
    const paddingBottom = 30;
    const chartWidth = width - paddingLeft - paddingRight;
    const chartHeight = height - paddingTop - paddingBottom;

    const baseData = hasRx ? rx : tx;
    const generatePoints = (dataPoints: any[]) => {
        return dataPoints.map((d: any, i: number) => {
            const x = paddingLeft + (i / (dataPoints.length - 1)) * chartWidth;
            const y = paddingTop + chartHeight - (d.value / yRange) * chartHeight;
            return { x, y, value: d.value, timestamp: d.timestamp };
        });
    };

    const rxPoints = hasRx ? generatePoints(rx) : [];
    const txPoints = hasTx ? generatePoints(tx) : [];

    const createPath = (points: any[]) => points.map((p: any, i: number) => `${i === 0 ? 'M' : 'L'} ${p.x} ${p.y}`).join(' ');

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

    // Y axis ticks
    const yTicks = Array.from({ length: 5 }, (_, i) => {
        const value = (yRange * i) / 4;
        const y = paddingTop + chartHeight - (i / 4) * chartHeight;
        return { value, y };
    });

    // X axis ticks
    const xTickIndices = baseData.length > 0 ? [0, Math.floor(baseData.length / 2), baseData.length - 1] : [];
    const xTicks = xTickIndices.map((i: any) => ({
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
                    {yTicks.map((tick: any, i: number) => (
                        <g key={i}>
                            <line x1={paddingLeft} y1={tick.y} x2={width - paddingRight} y2={tick.y}
                                className="stroke-gray-700" strokeWidth="0.5" strokeDasharray={i === 0 ? "0" : "2,2"} />
                            <text x={paddingLeft - 8} y={tick.y + 3} textAnchor="end" className="fill-gray-500" fontSize="9">
                                {formatValue(tick.value)}
                            </text>
                        </g>
                    ))}

                    {/* X axis labels */}
                    {xTicks.map((tick: any, i: number) => (
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
                        style={{ left: Math.min(mousePos.x + 10, (containerRef.current?.offsetWidth ?? 0) - 160 || 0), top: mousePos.y - 70 }}>
                        <div className="text-xs text-gray-400 mb-1">
                            {(hoveredRxPoint as any)?.timestamp ? new Date((hoveredRxPoint as any).timestamp).toLocaleString() : ''}
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

export default function PodMetricsTab({ pod, isStale }: { pod: any; isStale: boolean }) {
    const [prometheusInfo, setPrometheusInfo] = useState<any>(null);
    const [detecting, setDetecting] = useState(true);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState<string | null>(null);
    const [metricsData, setMetricsData] = useState<any>(null);
    const [duration, setDuration] = useState('1h');
    const [selectedContainer, setSelectedContainer] = useState('all');
    const [dropdownOpen, setDropdownOpen] = useState(false);
    const [eventMarkers, setEventMarkers] = useState<EventMarker[]>([]);
    const [zoomRange, setZoomRange] = useState<{ startMs: number; endMs: number } | null>(null);
    const requestIdRef = useRef(0); // Track current request to cancel stale ones

    const namespace = pod.metadata?.namespace;
    const podName = pod.metadata?.name;

    // Get container names and resources
    const containers = useMemo(() => {
        const initContainers = (pod.spec?.initContainers || []).map((c: any) => ({
            name: c.name,
            isInit: true,
            resources: c.resources || {}
        }));
        const regularContainers = (pod.spec?.containers || []).map((c: any) => ({
            name: c.name,
            isInit: false,
            resources: c.resources || {}
        }));
        return [...initContainers, ...regularContainers];
    }, [pod]);

    // Get resources for a specific container
    const getContainerResources = useCallback((containerName: string) => {
        const container = containers.find((c: any) => c.name === containerName);
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
            } catch (err: any) {
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
                const data = zoomRange
                    ? await GetPodMetricsHistoryRange(
                        requestIdString,
                        prometheusInfo.namespace,
                        prometheusInfo.service,
                        prometheusInfo.port,
                        namespace,
                        podName,
                        selectedContainer,
                        zoomRange.startMs,
                        zoomRange.endMs
                    )
                    : await GetPodMetricsHistory(
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
            } catch (err: any) {
                // Only update state if this request is still current
                if (currentRequestId === requestIdRef.current) {
                    console.error('Failed to fetch metrics:', err);
                    setError(err.toString());
                    setLoading(false);
                }
            }
        };

        fetchMetrics();
    }, [prometheusInfo, namespace, podName, selectedContainer, duration, zoomRange, isStale]);

    // Fetch event markers (pod-level, shared across CPU/Memory charts)
    useEffect(() => {
        if (!namespace || !podName || isStale) return;
        GetMetricsEventMarkers(namespace, podName, 'pod', duration)
            .then((m: EventMarker[]) => setEventMarkers(m || []))
            .catch(() => setEventMarkers([]));
    }, [namespace, podName, duration, isStale]);

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

    // Format CPU (millicores)
    const formatCPU = (value: number) => {
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

    const selectedContainerInfo = containers.find((c: any) => c.name === selectedContainer);

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
                        {DURATIONS.map((d: any) => (
                            <button
                                key={d.value}
                                onClick={() => handleDurationChange(d.value)}
                                className={`px-3 py-1 text-xs font-medium rounded transition-colors ${
                                    duration === d.value && !zoomRange
                                        ? 'bg-primary text-white'
                                        : 'text-gray-400 hover:text-white'
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
                                {metricsData.containers.map((container: any) => {
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
                                                    duration={effectiveDuration}
                                                    request={resources.cpuRequest}
                                                    limit={resources.cpuLimit}
                                                    markers={filteredMarkers}
                                                    onZoomSelect={handleZoomSelect}
                                                />
                                                <MetricsChart
                                                    data={container.memory}
                                                    color="stroke-purple-500"
                                                    label="Memory Usage"
                                                    formatValue={formatBytes}
                                                    duration={effectiveDuration}
                                                    markers={filteredMarkers}
                                                    request={resources.memRequest}
                                                    limit={resources.memLimit}
                                                    onZoomSelect={handleZoomSelect}
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
