import React, { useState, useEffect, useMemo, useRef, useCallback } from 'react';
import { formatChartTime as formatTime } from '~/utils/formatting';

// Shared single-series usage chart with optional request/limit reference lines,
// event markers, hover tooltip, and drag-to-zoom. Used by the pod, namespace, and
// controller metrics tabs. Node metrics use a distinct multi-series chart.

export interface EventMarker {
    timestamp: number;
    reason: string;
    severity: string; // "error" | "warning" | "info"
    message: string;
    kind: string;
}

export const MARKER_COLORS: Record<string, { line: string; fill: string; text: string }> = {
    error: { line: 'stroke-red-500', fill: 'fill-red-500', text: 'text-red-400' },
    warning: { line: 'stroke-amber-500', fill: 'fill-amber-500', text: 'text-amber-400' },
    info: { line: 'stroke-gray-500', fill: 'fill-gray-500', text: 'text-gray-400' },
};

export const MetricsChart = React.memo(({ data, color, label, formatValue, duration, request, limit, markers, onZoomSelect, defaultLinesVisible = true }: { data: any; color: string; label: string; formatValue: (value: number) => string; duration: string; request?: any; limit?: any; markers?: EventMarker[]; onZoomSelect?: (startMs: number, endMs: number) => void; defaultLinesVisible?: boolean }) => {
    const containerRef = useRef<HTMLDivElement>(null);
    const [hoveredIndex, setHoveredIndex] = useState<number | null>(null);
    const [mousePos, setMousePos] = useState({ x: 0, y: 0 });
    const [showRequest, setShowRequest] = useState(defaultLinesVisible);
    const [showLimit, setShowLimit] = useState(defaultLinesVisible);
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
