import React, { useState, useEffect, useMemo, useRef, useCallback } from 'react';
import { ChartBarIcon, ExclamationTriangleIcon, ArrowUturnLeftIcon } from '@heroicons/react/24/outline';
import { DetectPrometheus, GetControllerMetricsHistory, GetControllerMetricsHistoryRange, GetMetricsEventMarkers } from 'wailsjs/go/main/App';
import { formatBytes } from '~/utils/formatting';

interface EventMarker {
    timestamp: number;
    reason: string;
    severity: string;
    message: string;
    kind: string;
}

const MARKER_COLORS: Record<string, { line: string; fill: string; text: string }> = {
    error: { line: 'stroke-red-500', fill: 'fill-red-500', text: 'text-red-400' },
    warning: { line: 'stroke-amber-500', fill: 'fill-amber-500', text: 'text-amber-400' },
    info: { line: 'stroke-gray-500', fill: 'fill-gray-500', text: 'text-gray-400' },
};

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

interface MetricsChartProps {
    data: MetricPoint[];
    color: string;
    label: string;
    formatValue: (value: number) => string;
    duration: string;
    request?: MetricPoint[];
    limit?: MetricPoint[];
    markers?: EventMarker[];
    onZoomSelect?: (startMs: number, endMs: number) => void;
}

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

// Format time for display
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

// Interactive line chart component with toggleable request/limit lines
// Memoized to prevent re-renders when parent updates with same props
const MetricsChart = React.memo(({ data, color, label, formatValue, duration, request, limit, markers, onZoomSelect }: MetricsChartProps) => {
    const containerRef = useRef<HTMLDivElement>(null);
    const [hoveredIndex, setHoveredIndex] = useState<number | null>(null);
    const [mousePos, setMousePos] = useState({ x: 0, y: 0 });
    const [showRequest, setShowRequest] = useState(false);
    const [showLimit, setShowLimit] = useState(false);
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

    // Get request/limit values (use last data point if available)
    const requestValue = (request?.length ?? 0) > 0 ? request![request!.length - 1]?.value : null;
    const limitValue = (limit?.length ?? 0) > 0 ? limit![limit!.length - 1]?.value : null;

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

        const values = data.map((d: MetricPoint) => d.value);
        const timestamps = data.map((d: MetricPoint) => d.timestamp);
        let max = Math.max(...values) || 1;
        let min = Math.min(...values);

        // Extend max to include limit/request if they're higher and visible
        if (showLimit && limitValue && limitValue > max) max = limitValue * 1.05;
        if (showRequest && requestValue && requestValue > max) max = requestValue * 1.05;

        const range = max - min || 1;
        const yMin = Math.max(0, min - range * 0.05);
        const yMax = max + range * 0.05;
        const yRange = yMax - yMin || 1;

        // Generate points for the line
        const points: ChartPoint[] = data.map((d: MetricPoint, i: number) => {
            const x = paddingLeft + (i / (data.length - 1)) * chartWidth;
            const y = paddingTop + chartHeight - ((d.value - yMin) / yRange) * chartHeight;
            return { x, y, value: d.value, timestamp: d.timestamp };
        });

        const linePath = points.map((p: ChartPoint, i: number) => `${i === 0 ? 'M' : 'L'} ${p.x} ${p.y}`).join(' ');
        const areaPath = `${linePath} L ${points[points.length - 1].x} ${paddingTop + chartHeight} L ${paddingLeft} ${paddingTop + chartHeight} Z`;

        // Y axis ticks
        const yTicks = Array.from({ length: 5 }, (_, i) => {
            const value = yMin + (yRange * i) / 4;
            const y = paddingTop + chartHeight - (i / 4) * chartHeight;
            return { value, y };
        });

        // X axis ticks
        const xTickIndices = [0, Math.floor(data.length / 2), data.length - 1];
        const xTicks = xTickIndices.map((i: any) => ({
            timestamp: timestamps[i],
            x: paddingLeft + (i / (data.length - 1)) * chartWidth
        }));

        // Y positions for request/limit lines
        const getYPos = (value: number | null) => {
            if (value == null || value < yMin || value > yMax) return null;
            return paddingTop + chartHeight - ((value - yMin) / yRange) * chartHeight;
        };

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
            requestY: getYPos(requestValue),
            limitY: getYPos(limitValue),
            markerPositions,
        };
    }, [data, showRequest, showLimit, requestValue, limitValue, chartWidth, chartHeight, markers]);

    if (!chartData) {
        return (
            <div className="h-56 flex items-center justify-center text-gray-500 text-sm bg-background rounded border border-border">
                No data available
            </div>
        );
    }

    const { points, linePath, areaPath, yTicks, xTicks, currentValue, requestY, limitY, markerPositions } = chartData;

    const hoveredMarker = hoveredIndex !== null && showMarkers
        ? markerPositions.find((m: any) => Math.abs(m.dataIndex - hoveredIndex) <= 1)
        : null;

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
                        {requestValue != null && (
                            <button
                                onClick={() => setShowRequest(!showRequest)}
                                className={`flex items-center gap-1 px-1.5 py-0.5 rounded transition-colors hover:bg-white/10 ${!showRequest ? 'opacity-40' : ''}`}
                                title={showRequest ? 'Hide request line' : 'Show request line'}
                            >
                                <span className={`w-3 h-0.5 ${showRequest ? 'bg-orange-500' : 'bg-gray-500'}`}></span>
                                <span className={showRequest ? 'text-orange-400' : 'text-gray-500'}>Request</span>
                            </button>
                        )}
                        {limitValue != null && (
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

                    {/* Axes */}
                    <line x1={paddingLeft} y1={paddingTop} x2={paddingLeft} y2={paddingTop + chartHeight} className="stroke-gray-600" strokeWidth="1" />
                    <line x1={paddingLeft} y1={paddingTop + chartHeight} x2={width - paddingRight} y2={paddingTop + chartHeight} className="stroke-gray-600" strokeWidth="1" />

                    {/* Request line (orange) */}
                    {showRequest && requestY != null && (
                        <line x1={paddingLeft} y1={requestY} x2={width - paddingRight} y2={requestY}
                            className="stroke-orange-500" strokeWidth="1.5" strokeDasharray="6,3" />
                    )}

                    {/* Limit line (red) */}
                    {showLimit && limitY != null && (
                        <line x1={paddingLeft} y1={limitY} x2={width - paddingRight} y2={limitY}
                            className="stroke-red-500" strokeWidth="1.5" strokeDasharray="6,3" />
                    )}

                    {/* Area fill */}
                    <path d={areaPath} className={`${color.replace('stroke-', 'fill-')} opacity-20`} />

                    {/* Line */}
                    <path d={linePath} fill="none" className={color} strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />

                    {/* Event markers */}
                    {showMarkers && markerPositions.map((m: any, i: number) => {
                        const mc = MARKER_COLORS[m.severity] || MARKER_COLORS.info;
                        return (
                            <g key={i}>
                                <line x1={m.x} y1={paddingTop} x2={m.x} y2={paddingTop + chartHeight}
                                    className={mc.line} strokeWidth="1" strokeDasharray="3,3" opacity="0.7" />
                                <polygon
                                    points={`${m.x},${paddingTop - 1} ${m.x + 4},${paddingTop + 4} ${m.x},${paddingTop + 9} ${m.x - 4},${paddingTop + 4}`}
                                    className={mc.fill} opacity="0.9" />
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
                            <line x1={hoveredPoint.x} y1={paddingTop} x2={hoveredPoint.x} y2={paddingTop + chartHeight}
                                className="stroke-gray-400" strokeWidth="1" strokeDasharray="4,2" />
                            <circle cx={hoveredPoint.x} cy={hoveredPoint.y} r="4"
                                className={`${color.replace('stroke-', 'fill-')}`} stroke="white" strokeWidth="2" />
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
                        {showRequest && requestValue != null && (
                            <div className="text-xs text-orange-400">Request: {formatValue(requestValue)}</div>
                        )}
                        {showLimit && limitValue != null && (
                            <div className="text-xs text-red-400">Limit: {formatValue(limitValue)}</div>
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
                                request={metricsData.cpu?.request}
                                limit={metricsData.cpu?.limit}
                                markers={filteredMarkers}
                                onZoomSelect={handleZoomSelect}
                            />
                            <MetricsChart
                                data={metricsData.memory?.usage}
                                color="stroke-purple-500"
                                label="Memory Usage (Aggregated)"
                                formatValue={formatBytes}
                                duration={effectiveDuration}
                                request={metricsData.memory?.request}
                                limit={metricsData.memory?.limit}
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
