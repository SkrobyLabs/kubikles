import React, { useState, useEffect, useRef, useCallback, useMemo } from 'react';
import { ChartBarIcon, ExclamationTriangleIcon, ArrowUturnLeftIcon } from '@heroicons/react/24/outline';
import { DetectPrometheus, GetNodeMetricsHistory, GetNodeMetricsHistoryRange, GetMetricsEventMarkers } from 'wailsjs/go/main/App';
import { formatBytes, formatChartTime as formatTime } from '~/utils/formatting';
import { MARKER_COLORS, type EventMarker } from './metrics/MetricsChart';

// Format time for display
// Node resource chart with toggleable scheduling lines. The memory variant also
// exposes OS and kubelet pressure signals so low requests can be compared with
// the memory the kernel and root cgroup actually account for.
// Memoized to prevent re-renders when parent updates with same props
const NodeResourceChart = React.memo(({ data, color, label, formatValue, duration, memoryDetails = false, markers, onZoomSelect }: { data: any; color: string; label: string; formatValue: (value: number) => string; duration: string; memoryDetails?: boolean; markers?: EventMarker[]; onZoomSelect?: (startMs: number, endMs: number) => void }) => {
    const containerRef = useRef<HTMLDivElement>(null);
    const [hoveredIndex, setHoveredIndex] = useState<number | null>(null);
    const [mousePos, setMousePos] = useState({ x: 0, y: 0 });
    const [showCommitted, setShowCommitted] = useState(!memoryDetails);
    const [showReserved, setShowReserved] = useState(false);
    const [showAvailable, setShowAvailable] = useState(true);
    const [showKubeletAvailable, setShowKubeletAvailable] = useState(true);
    const [showWorkingSet, setShowWorkingSet] = useState(true);
    const [showPageCache, setShowPageCache] = useState(false);
    const [showSlab, setShowSlab] = useState(false);
    const [showSharedMemory, setShowSharedMemory] = useState(false);
    const [showMarkers, setShowMarkers] = useState(true);
    const [containerWidth, setContainerWidth] = useState(500);
    const [isDragging, setIsDragging] = useState(false);
    const [dragStartIndex, setDragStartIndex] = useState<number | null>(null);
    const [dragEndIndex, setDragEndIndex] = useState<number | null>(null);

    // Track container size for responsive chart
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

    const hasUsage = data?.usage?.length > 0;
    const hasAllocatable = data?.allocatable?.length > 0;
    const hasReserved = data?.reserved?.length > 0;
    const hasCommitted = data?.committed?.length > 0;
    const hasAvailable = memoryDetails && data?.available?.length > 0;
    const hasKubeletAvailable = memoryDetails && data?.kubeletAvailable?.length > 0;
    const hasWorkingSet = memoryDetails && data?.workingSet?.length > 0;
    const hasPageCache = memoryDetails && data?.pageCache?.length > 0;
    const hasSlab = memoryDetails && data?.slab?.length > 0;
    const hasSharedMemory = memoryDetails && data?.sharedMemory?.length > 0;

    if (!hasUsage) {
        return (
            <div className="relative">
                <div className="text-sm font-medium text-gray-300 mb-2">{label}</div>
                <div className="h-56 flex items-center justify-center text-gray-500 text-sm bg-background rounded border border-border">
                    No data available
                </div>
            </div>
        );
    }

    const usage = data.usage;
    const allocatable = data.allocatable || [];
    const reserved = data.reserved || [];
    const committed = data.committed || [];
    const available = data.available || [];
    const kubeletAvailable = data.kubeletAvailable || [];
    const workingSet = data.workingSet || [];
    const pageCache = data.pageCache || [];
    const slab = data.slab || [];
    const sharedMemory = data.sharedMemory || [];

    // Calculate Y-axis bounds
    const allValues = [
        ...usage.map((d: any) => d.value),
        ...allocatable.map((d: any) => d.value),
        ...(showReserved ? reserved.map((d: any) => d.value) : []),
        ...(showCommitted ? committed.map((d: any) => d.value) : []),
        ...(showAvailable ? available.map((d: any) => d.value) : []),
        ...(showKubeletAvailable ? kubeletAvailable.map((d: any) => d.value) : []),
        ...(showWorkingSet ? workingSet.map((d: any) => d.value) : []),
        ...(showPageCache ? pageCache.map((d: any) => d.value) : []),
        ...(showSlab ? slab.map((d: any) => d.value) : []),
        ...(showSharedMemory ? sharedMemory.map((d: any) => d.value) : [])
    ];
    const max = Math.max(...allValues) || 1;
    const min = 0;
    const range = max - min || 1;
    const yMin = 0;
    const yMax = max * 1.05;
    const yRange = yMax - yMin || 1;

    // Chart dimensions - width from container, fixed height
    const width = Math.max(300, containerWidth);
    const height = 200;
    const paddingLeft = 70;
    const paddingRight = 20;
    const paddingTop = 20;
    const paddingBottom = 30;
    const chartWidth = width - paddingLeft - paddingRight;
    const chartHeight = height - paddingTop - paddingBottom;

    // Generate points for lines
    const generatePoints = (dataPoints: any[]) => {
        return dataPoints.map((d: any, i: number) => {
            const x = paddingLeft + (i / (dataPoints.length - 1)) * chartWidth;
            const y = paddingTop + chartHeight - ((d.value - yMin) / yRange) * chartHeight;
            return { x, y, value: d.value, timestamp: d.timestamp };
        });
    };

    const usagePoints = generatePoints(usage);
    const allocatablePoints = hasAllocatable ? generatePoints(allocatable) : [];
    const reservedPoints = hasReserved ? generatePoints(reserved) : [];
    const committedPoints = hasCommitted ? generatePoints(committed) : [];
    const availablePoints = hasAvailable ? generatePoints(available) : [];
    const kubeletAvailablePoints = hasKubeletAvailable ? generatePoints(kubeletAvailable) : [];
    const workingSetPoints = hasWorkingSet ? generatePoints(workingSet) : [];
    const pageCachePoints = hasPageCache ? generatePoints(pageCache) : [];
    const slabPoints = hasSlab ? generatePoints(slab) : [];
    const sharedMemoryPoints = hasSharedMemory ? generatePoints(sharedMemory) : [];

    const createPath = (points: any[]) => points.map((p: any, i: number) => `${i === 0 ? 'M' : 'L'} ${p.x} ${p.y}`).join(' ');

    const usagePath = createPath(usagePoints);
    const areaPath = `${usagePath} L ${usagePoints[usagePoints.length - 1].x} ${paddingTop + chartHeight} L ${paddingLeft} ${paddingTop + chartHeight} Z`;

    // Y axis ticks
    const yTicks = Array.from({ length: 5 }, (_, i) => {
        const value = yMin + (yRange * i) / 4;
        const y = paddingTop + chartHeight - (i / 4) * chartHeight;
        return { value, y };
    });

    // X axis ticks
    const xTickIndices = [0, Math.floor(usage.length / 2), usage.length - 1];
    const xTicks = xTickIndices.map((i: any) => ({
        timestamp: usage[i]?.timestamp,
        x: paddingLeft + (i / (usage.length - 1)) * chartWidth
    }));

    // Compute marker x-positions
    const markerPositions = useMemo(() => {
        if (!markers || markers.length === 0 || usage.length === 0) return [];
        return markers.map(m => {
            let closestIdx = 0;
            let closestDist = Math.abs(Number(usage[0]?.timestamp) - m.timestamp);
            for (let i = 1; i < usage.length; i++) {
                const dist = Math.abs(Number(usage[i]?.timestamp) - m.timestamp);
                if (dist < closestDist) {
                    closestDist = dist;
                    closestIdx = i;
                }
            }
            return { ...m, x: usagePoints[closestIdx].x, dataIndex: closestIdx };
        });
    }, [markers, usage, usagePoints]);

    const currentUsage = usage[usage.length - 1]?.value || 0;

    const getDataIndex = useCallback((e: any): number | null => {
        if (!containerRef.current || !usage || usage.length === 0) return null;
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
            const index = Math.round((chartX / chartWidth) * (usage.length - 1));
            return Math.max(0, Math.min(usage.length - 1, index));
        }
        return null;
    }, [usage?.length, chartWidth, width, height]);

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
        if (!isDragging || dragStartIndex == null || dragEndIndex == null || !usage || !onZoomSelect) {
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
            const startMs = Number(usage[minIdx].timestamp);
            const endMs = Number(usage[maxIdx].timestamp);
            onZoomSelect(startMs, endMs);
        }
    }, [isDragging, dragStartIndex, dragEndIndex, usage, onZoomSelect]);

    const handleMouseLeave = useCallback(() => {
        setHoveredIndex(null);
        if (isDragging) {
            setIsDragging(false);
            setDragStartIndex(null);
            setDragEndIndex(null);
        }
    }, [isDragging]);

    const hoveredPoint = hoveredIndex !== null ? usagePoints[hoveredIndex] : null;
    const hoveredAllocatable = hoveredIndex !== null && allocatablePoints[hoveredIndex];
    const hoveredReserved = hoveredIndex !== null && reservedPoints[hoveredIndex];
    const hoveredCommitted = hoveredIndex !== null && committedPoints[hoveredIndex];
    const hoveredAvailable = hoveredIndex !== null && availablePoints[hoveredIndex];
    const hoveredKubeletAvailable = hoveredIndex !== null && kubeletAvailablePoints[hoveredIndex];
    const hoveredWorkingSet = hoveredIndex !== null && workingSetPoints[hoveredIndex];
    const hoveredPageCache = hoveredIndex !== null && pageCachePoints[hoveredIndex];
    const hoveredSlab = hoveredIndex !== null && slabPoints[hoveredIndex];
    const hoveredSharedMemory = hoveredIndex !== null && sharedMemoryPoints[hoveredIndex];
    const hoveredMarker = hoveredIndex !== null && showMarkers
        ? markerPositions.find((m: any) => Math.abs(m.dataIndex - hoveredIndex) <= 1)
        : null;

    return (
        <div className="relative">
            <div className="flex items-start justify-between gap-3 mb-2">
                <div className="flex items-start gap-3 min-w-0">
                    <span className="text-sm font-medium text-gray-300 shrink-0 pt-0.5">{label}</span>
                    {/* Legend - clickable to toggle */}
                    <div className="flex flex-wrap items-center gap-1 text-xs">
                        <span
                            className="flex items-center gap-1 text-blue-400"
                            title={memoryDetails ? 'MemTotal minus MemAvailable; falls back to summed pod working sets when node-exporter is unavailable' : undefined}
                        >
                            <span className="w-3 h-0.5 bg-blue-500"></span>
                            {memoryDetails ? 'Used' : 'Usage'}
                        </span>
                        <span className="flex items-center gap-1 text-gray-400">
                            <span className="w-3 h-0.5 bg-gray-500 border-dashed"></span>
                            Allocatable
                        </span>
                        {hasAvailable && (
                            <button
                                onClick={() => setShowAvailable(!showAvailable)}
                                className={`flex items-center gap-1 px-1.5 py-0.5 rounded transition-colors hover:bg-white/10 ${!showAvailable ? 'opacity-40' : ''}`}
                                title="Linux MemAvailable: the kernel estimate of memory available without swapping"
                            >
                                <span className={`w-3 h-0.5 ${showAvailable ? 'bg-emerald-500' : 'bg-gray-500'}`}></span>
                                <span className={showAvailable ? 'text-emerald-400' : 'text-gray-500'}>OS available</span>
                            </button>
                        )}
                        {hasKubeletAvailable && (
                            <button
                                onClick={() => setShowKubeletAvailable(!showKubeletAvailable)}
                                className={`flex items-center gap-1 px-1.5 py-0.5 rounded transition-colors hover:bg-white/10 ${!showKubeletAvailable ? 'opacity-40' : ''}`}
                                title="Kubelet eviction estimate: node capacity minus the root-cgroup working set"
                            >
                                <span className={`w-3 h-0.5 border-t border-dashed ${showKubeletAvailable ? 'border-lime-400' : 'border-gray-500'}`}></span>
                                <span className={showKubeletAvailable ? 'text-lime-300' : 'text-gray-500'}>Eviction available</span>
                            </button>
                        )}
                        {hasWorkingSet && (
                            <button
                                onClick={() => setShowWorkingSet(!showWorkingSet)}
                                className={`flex items-center gap-1 px-1.5 py-0.5 rounded transition-colors hover:bg-white/10 ${!showWorkingSet ? 'opacity-40' : ''}`}
                                title="Root-cgroup working set used by kubelet to estimate memory.available"
                            >
                                <span className={`w-3 h-0.5 ${showWorkingSet ? 'bg-purple-500' : 'bg-gray-500'}`}></span>
                                <span className={showWorkingSet ? 'text-purple-400' : 'text-gray-500'}>Root working set</span>
                            </button>
                        )}
                        {hasPageCache && (
                            <button
                                onClick={() => setShowPageCache(!showPageCache)}
                                className={`flex items-center gap-1 px-1.5 py-0.5 rounded transition-colors hover:bg-white/10 ${!showPageCache ? 'opacity-40' : ''}`}
                                title="Filesystem page cache and block-device buffers; generally reclaimable"
                            >
                                <span className={`w-3 h-0.5 ${showPageCache ? 'bg-cyan-500' : 'bg-gray-500'}`}></span>
                                <span className={showPageCache ? 'text-cyan-400' : 'text-gray-500'}>Page cache</span>
                            </button>
                        )}
                        {hasSlab && (
                            <button
                                onClick={() => setShowSlab(!showSlab)}
                                className={`flex items-center gap-1 px-1.5 py-0.5 rounded transition-colors hover:bg-white/10 ${!showSlab ? 'opacity-40' : ''}`}
                                title="Kernel slab allocations"
                            >
                                <span className={`w-3 h-0.5 ${showSlab ? 'bg-rose-500' : 'bg-gray-500'}`}></span>
                                <span className={showSlab ? 'text-rose-400' : 'text-gray-500'}>Slab</span>
                            </button>
                        )}
                        {hasSharedMemory && (
                            <button
                                onClick={() => setShowSharedMemory(!showSharedMemory)}
                                className={`flex items-center gap-1 px-1.5 py-0.5 rounded transition-colors hover:bg-white/10 ${!showSharedMemory ? 'opacity-40' : ''}`}
                                title="Shared memory, including memory-backed tmpfs filesystems"
                            >
                                <span className={`w-3 h-0.5 ${showSharedMemory ? 'bg-pink-500' : 'bg-gray-500'}`}></span>
                                <span className={showSharedMemory ? 'text-pink-400' : 'text-gray-500'}>Shmem/tmpfs</span>
                            </button>
                        )}
                        {hasReserved && (
                            <button
                                onClick={() => setShowReserved(!showReserved)}
                                className={`flex items-center gap-1 px-1.5 py-0.5 rounded transition-colors hover:bg-white/10 ${!showReserved ? 'opacity-40' : ''}`}
                                title={showReserved ? 'Hide reserved line' : 'Show reserved line'}
                            >
                                <span className={`w-3 h-0.5 ${showReserved ? 'bg-yellow-500' : 'bg-gray-500'}`}></span>
                                <span className={showReserved ? 'text-yellow-400' : 'text-gray-500'}>Reserved</span>
                            </button>
                        )}
                        {hasCommitted && (
                            <button
                                onClick={() => setShowCommitted(!showCommitted)}
                                className={`flex items-center gap-1 px-1.5 py-0.5 rounded transition-colors hover:bg-white/10 ${!showCommitted ? 'opacity-40' : ''}`}
                                title={showCommitted ? 'Hide committed line' : 'Show committed line'}
                            >
                                <span className={`w-3 h-0.5 ${showCommitted ? 'bg-orange-500' : 'bg-gray-500'}`}></span>
                                <span className={showCommitted ? 'text-orange-400' : 'text-gray-500'}>Committed</span>
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
                <span className="text-sm text-gray-400 shrink-0">
                    Current{memoryDetails ? ' used' : ''}: <span className={`font-medium ${color.replace('stroke-', 'text-')}`}>
                        {formatValue(currentUsage)}
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
                                y={tick.y + 4}
                                textAnchor="end"
                                className="fill-gray-500"
                                fontSize="11"
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
                            y={height - 6}
                            textAnchor="middle"
                            className="fill-gray-500"
                            fontSize="11"
                        >
                            {tick.timestamp && formatTime(tick.timestamp, duration)}
                        </text>
                    ))}

                    {/* Axes */}
                    <line x1={paddingLeft} y1={paddingTop} x2={paddingLeft} y2={paddingTop + chartHeight} className="stroke-gray-600" strokeWidth="1" />
                    <line x1={paddingLeft} y1={paddingTop + chartHeight} x2={width - paddingRight} y2={paddingTop + chartHeight} className="stroke-gray-600" strokeWidth="1" />

                    {/* Allocatable line (gray dashed) */}
                    {allocatablePoints.length > 0 && (
                        <path d={createPath(allocatablePoints)} fill="none" className="stroke-gray-500" strokeWidth="1.5" strokeDasharray="6,3" />
                    )}

                    {/* Reserved line (yellow, toggleable) */}
                    {showReserved && reservedPoints.length > 0 && (
                        <path d={createPath(reservedPoints)} fill="none" className="stroke-yellow-500" strokeWidth="1.5" strokeDasharray="4,2" />
                    )}

                    {/* Committed line (orange, toggleable) */}
                    {showCommitted && committedPoints.length > 0 && (
                        <path d={createPath(committedPoints)} fill="none" className="stroke-orange-500" strokeWidth="1.5" />
                    )}

                    {/* Memory pressure signals (memory chart only) */}
                    {showAvailable && availablePoints.length > 0 && (
                        <path d={createPath(availablePoints)} fill="none" className="stroke-emerald-500" strokeWidth="1.5" />
                    )}
                    {showKubeletAvailable && kubeletAvailablePoints.length > 0 && (
                        <path d={createPath(kubeletAvailablePoints)} fill="none" className="stroke-lime-400" strokeWidth="1.5" strokeDasharray="6,3" />
                    )}
                    {showWorkingSet && workingSetPoints.length > 0 && (
                        <path d={createPath(workingSetPoints)} fill="none" className="stroke-purple-500" strokeWidth="1.5" />
                    )}
                    {showPageCache && pageCachePoints.length > 0 && (
                        <path d={createPath(pageCachePoints)} fill="none" className="stroke-cyan-500" strokeWidth="1.5" />
                    )}
                    {showSlab && slabPoints.length > 0 && (
                        <path d={createPath(slabPoints)} fill="none" className="stroke-rose-500" strokeWidth="1.5" />
                    )}
                    {showSharedMemory && sharedMemoryPoints.length > 0 && (
                        <path d={createPath(sharedMemoryPoints)} fill="none" className="stroke-pink-500" strokeWidth="1.5" />
                    )}

                    {/* Usage area fill */}
                    <path d={areaPath} className="fill-blue-500 opacity-20" />

                    {/* Usage line (blue) */}
                    <path d={usagePath} fill="none" className={color} strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />

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
                    {isDragging && dragStartIndex != null && dragEndIndex != null && usagePoints.length > 0 && (() => {
                        const x1 = usagePoints[Math.min(dragStartIndex, dragEndIndex)]?.x ?? 0;
                        const x2 = usagePoints[Math.max(dragStartIndex, dragEndIndex)]?.x ?? 0;
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
                                className="fill-blue-500" stroke="white" strokeWidth="2" />
                            {hoveredAllocatable && (
                                <circle cx={hoveredAllocatable.x} cy={hoveredAllocatable.y} r="3"
                                    className="fill-gray-500" stroke="white" strokeWidth="1.5" />
                            )}
                            {showReserved && hoveredReserved && (
                                <circle cx={hoveredReserved.x} cy={hoveredReserved.y} r="3"
                                    className="fill-yellow-500" stroke="white" strokeWidth="1.5" />
                            )}
                            {showCommitted && hoveredCommitted && (
                                <circle cx={hoveredCommitted.x} cy={hoveredCommitted.y} r="3"
                                    className="fill-orange-500" stroke="white" strokeWidth="1.5" />
                            )}
                            {showAvailable && hoveredAvailable && (
                                <circle cx={hoveredAvailable.x} cy={hoveredAvailable.y} r="3"
                                    className="fill-emerald-500" stroke="white" strokeWidth="1.5" />
                            )}
                            {showKubeletAvailable && hoveredKubeletAvailable && (
                                <circle cx={hoveredKubeletAvailable.x} cy={hoveredKubeletAvailable.y} r="3"
                                    className="fill-lime-400" stroke="white" strokeWidth="1.5" />
                            )}
                            {showWorkingSet && hoveredWorkingSet && (
                                <circle cx={hoveredWorkingSet.x} cy={hoveredWorkingSet.y} r="3"
                                    className="fill-purple-500" stroke="white" strokeWidth="1.5" />
                            )}
                            {showPageCache && hoveredPageCache && (
                                <circle cx={hoveredPageCache.x} cy={hoveredPageCache.y} r="3"
                                    className="fill-cyan-500" stroke="white" strokeWidth="1.5" />
                            )}
                            {showSlab && hoveredSlab && (
                                <circle cx={hoveredSlab.x} cy={hoveredSlab.y} r="3"
                                    className="fill-rose-500" stroke="white" strokeWidth="1.5" />
                            )}
                            {showSharedMemory && hoveredSharedMemory && (
                                <circle cx={hoveredSharedMemory.x} cy={hoveredSharedMemory.y} r="3"
                                    className="fill-pink-500" stroke="white" strokeWidth="1.5" />
                            )}
                        </>
                    )}
                </svg>

                {/* Tooltip */}
                {!isDragging && hoveredPoint && (
                    <div
                        className="absolute z-10 pointer-events-none bg-surface border border-border rounded-lg shadow-lg px-3 py-2"
                        style={{
                            left: Math.min(mousePos.x + 10, (containerRef.current?.offsetWidth ?? 0) - 210 || 0),
                            top: Math.max(4, mousePos.y - 100)
                        }}
                    >
                        <div className="text-xs text-gray-400 mb-1">
                            {new Date(hoveredPoint.timestamp).toLocaleString()}
                        </div>
                        <div className="text-sm font-medium text-blue-400">
                            {memoryDetails ? 'Used' : 'Usage'}: {formatValue(hoveredPoint.value)}
                        </div>
                        {hoveredAllocatable && (
                            <div className="text-xs text-gray-400">Allocatable: {formatValue(hoveredAllocatable.value)}</div>
                        )}
                        {showReserved && hoveredReserved && (
                            <div className="text-xs text-yellow-400">Reserved: {formatValue(hoveredReserved.value)}</div>
                        )}
                        {showCommitted && hoveredCommitted && (
                            <div className="text-xs text-orange-400">Committed: {formatValue(hoveredCommitted.value)}</div>
                        )}
                        {showAvailable && hoveredAvailable && (
                            <div className="text-xs text-emerald-400">OS available: {formatValue(hoveredAvailable.value)}</div>
                        )}
                        {showKubeletAvailable && hoveredKubeletAvailable && (
                            <div className="text-xs text-lime-300">Eviction available: {formatValue(hoveredKubeletAvailable.value)}</div>
                        )}
                        {showWorkingSet && hoveredWorkingSet && (
                            <div className="text-xs text-purple-400">Root working set: {formatValue(hoveredWorkingSet.value)}</div>
                        )}
                        {showPageCache && hoveredPageCache && (
                            <div className="text-xs text-cyan-400">Page cache: {formatValue(hoveredPageCache.value)}</div>
                        )}
                        {showSlab && hoveredSlab && (
                            <div className="text-xs text-rose-400">Slab: {formatValue(hoveredSlab.value)}</div>
                        )}
                        {showSharedMemory && hoveredSharedMemory && (
                            <div className="text-xs text-pink-400">Shmem/tmpfs: {formatValue(hoveredSharedMemory.value)}</div>
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

// Pod count chart showing current vs capacity
// Memoized to prevent re-renders when parent updates with same props
const PodCountChart = React.memo(({ data, duration }: { data: any; duration: string }) => {
    const containerRef = useRef<HTMLDivElement>(null);
    const [hoveredIndex, setHoveredIndex] = useState<number | null>(null);
    const [mousePos, setMousePos] = useState({ x: 0, y: 0 });
    const [containerWidth, setContainerWidth] = useState(400);

    // Track container size for responsive chart
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

    const hasRunning = data?.running?.length > 0;
    const hasCapacity = data?.capacity?.length > 0;

    if (!hasRunning) {
        return (
            <div className="relative">
                <div className="text-sm font-medium text-gray-300 mb-2">Pods</div>
                <div className="h-36 flex items-center justify-center text-gray-500 text-sm bg-background rounded border border-border">
                    No data available
                </div>
            </div>
        );
    }

    const running = data.running;
    const capacity = data.capacity || [];

    const allValues = [...running.map((d: any) => d.value), ...capacity.map((d: any) => d.value)];
    const max = Math.max(...allValues, 1);
    const yMax = max * 1.1;
    const yRange = yMax || 1;

    const width = Math.max(300, containerWidth);
    const height = 120;
    const paddingLeft = 45;
    const paddingRight = 20;
    const paddingTop = 15;
    const paddingBottom = 25;
    const chartWidth = width - paddingLeft - paddingRight;
    const chartHeight = height - paddingTop - paddingBottom;

    const generatePoints = (dataPoints: any[]) => {
        return dataPoints.map((d: any, i: number) => {
            const x = paddingLeft + (i / (dataPoints.length - 1)) * chartWidth;
            const y = paddingTop + chartHeight - (d.value / yRange) * chartHeight;
            return { x, y, value: d.value, timestamp: d.timestamp };
        });
    };

    const runningPoints = generatePoints(running);
    const capacityPoints = hasCapacity ? generatePoints(capacity) : [];

    const createPath = (points: any[]) => points.map((p: any, i: number) => `${i === 0 ? 'M' : 'L'} ${p.x} ${p.y}`).join(' ');

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
            const index = Math.round((chartX / chartWidth) * (running.length - 1));
            setHoveredIndex(Math.max(0, Math.min(running.length - 1, index)));
            setMousePos({ x: mouseX - rect.left, y: mouseY - rect.top });
        } else {
            setHoveredIndex(null);
        }
    }, [running.length, chartWidth]);

    const currentRunning = running[running.length - 1]?.value || 0;
    const currentCapacity = capacity[capacity.length - 1]?.value || 0;
    const hoveredPoint = hoveredIndex !== null ? runningPoints[hoveredIndex] : null;
    const hoveredCapPoint = hoveredIndex !== null && capacityPoints[hoveredIndex];

    return (
        <div className="relative">
            <div className="flex items-center justify-between mb-1">
                <div className="flex items-center gap-3">
                    <span className="text-xs font-medium text-gray-400">Pods</span>
                    <div className="flex items-center gap-2 text-xs">
                        <span className="flex items-center gap-1 text-green-400">
                            <span className="w-3 h-0.5 bg-green-500"></span>
                            Running
                        </span>
                        {hasCapacity && (
                            <span className="flex items-center gap-1 text-gray-400">
                                <span className="w-3 h-0.5 bg-gray-500 border-dashed"></span>
                                Capacity
                            </span>
                        )}
                    </div>
                </div>
                <span className="text-sm font-medium text-green-400">
                    {Math.round(currentRunning)}{hasCapacity && ` / ${Math.round(currentCapacity)}`}
                </span>
            </div>
            <div ref={containerRef} className="h-36 bg-background rounded border border-border relative"
                onMouseMove={handleMouseMove} onMouseLeave={() => setHoveredIndex(null)}>
                <svg viewBox={`0 0 ${width} ${height}`} preserveAspectRatio="xMinYMin meet" className="w-full h-full">
                    <line x1={paddingLeft} y1={paddingTop + chartHeight} x2={width - paddingRight} y2={paddingTop + chartHeight}
                        className="stroke-gray-700" strokeWidth="0.5" />

                    {/* Capacity line (gray dashed) */}
                    {capacityPoints.length > 0 && (
                        <path d={createPath(capacityPoints)} fill="none" className="stroke-gray-500" strokeWidth="1.5" strokeDasharray="6,3" />
                    )}

                    {/* Running line (green) */}
                    <path d={createPath(runningPoints)} fill="none" className="stroke-green-500" strokeWidth="2" strokeLinecap="round" />

                    {hoveredPoint && (
                        <>
                            <circle cx={hoveredPoint.x} cy={hoveredPoint.y} r="3"
                                className="fill-green-500" stroke="white" strokeWidth="1.5" />
                            {hoveredCapPoint && (
                                <circle cx={hoveredCapPoint.x} cy={hoveredCapPoint.y} r="2"
                                    className="fill-gray-500" stroke="white" strokeWidth="1" />
                            )}
                        </>
                    )}
                </svg>
                {hoveredPoint && (
                    <div className="absolute z-10 pointer-events-none bg-surface border border-border rounded px-2 py-1 text-xs"
                        style={{ left: Math.min(mousePos.x + 10, (containerRef.current?.offsetWidth ?? 0) - 100 || 0), top: mousePos.y - 40 }}>
                        <span className="text-green-400">Running: {Math.round(hoveredPoint.value)}</span>
                        {hoveredCapPoint && (
                            <span className="text-gray-400 ml-2">Capacity: {Math.round(hoveredCapPoint.value)}</span>
                        )}
                    </div>
                )}
            </div>
        </div>
    );
});

// Network I/O chart with bandwidth and packets view toggle
// Memoized to prevent re-renders when parent updates with same props
const NetworkChart = React.memo(({ data, duration }: { data: any; duration: string }) => {
    const containerRef = useRef<HTMLDivElement>(null);
    const [hoveredIndex, setHoveredIndex] = useState<number | null>(null);
    const [mousePos, setMousePos] = useState({ x: 0, y: 0 });
    const [viewMode, setViewMode] = useState('bandwidth'); // 'bandwidth' or 'packets'
    const [containerWidth, setContainerWidth] = useState(400);

    // Track container size for responsive chart
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
    const hasRxDropped = data?.receiveDropped?.length > 0;
    const hasTxDropped = data?.transmitDropped?.length > 0;

    const hasBandwidth = hasRxBytes || hasTxBytes;
    const hasPackets = hasRxPackets || hasTxPackets;

    if (!hasBandwidth && !hasPackets) {
        return (
            <div className="relative">
                <div className="text-sm font-medium text-gray-300 mb-2">Network I/O</div>
                <div className="h-36 flex items-center justify-center text-gray-500 text-sm bg-background rounded border border-border">
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
    const height = 120;
    const paddingLeft = 55;
    const paddingRight = 20;
    const paddingTop = 15;
    const paddingBottom = 25;
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
    const unit = isBandwidthView ? '' : ' pkt';

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
            <div ref={containerRef} className="h-36 bg-background rounded border border-border relative"
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
                                Dropped: ↓{formatPacketRate(hoveredRxDropped?.value || 0)} ↑{formatPacketRate(hoveredTxDropped?.value || 0)}
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

export default function NodeMetricsTab({ nodeName, isStale }: { nodeName: string; isStale: boolean }) {
    const [prometheusInfo, setPrometheusInfo] = useState<any>(null);
    const [detecting, setDetecting] = useState(true);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState<string | null>(null);
    const [metricsData, setMetricsData] = useState<any>(null);
    const [duration, setDuration] = useState('1h');
    const [eventMarkers, setEventMarkers] = useState<EventMarker[]>([]);
    const [zoomRange, setZoomRange] = useState<{ startMs: number; endMs: number } | null>(null);
    const requestIdRef = useRef(0); // Track current request to cancel stale ones

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

    // Fetch event markers
    useEffect(() => {
        if (!nodeName || isStale) return;
        GetMetricsEventMarkers('', nodeName, 'node', duration)
            .then((m: EventMarker[]) => setEventMarkers(m || []))
            .catch(() => setEventMarkers([]));
    }, [nodeName, duration, isStale]);

    useEffect(() => {
        if (!prometheusInfo?.available || isStale) return;

        // Increment request ID to invalidate any in-flight requests
        const currentRequestId = ++requestIdRef.current;
        // Use stable ID for backend cancellation (without counter)
        const requestIdString = `node-metrics-${nodeName}`;

        const fetchMetrics = async () => {
            setLoading(true);
            setError(null);
            try {
                const data = zoomRange
                    ? await GetNodeMetricsHistoryRange(
                        requestIdString,
                        prometheusInfo.namespace,
                        prometheusInfo.service,
                        prometheusInfo.port,
                        nodeName,
                        zoomRange.startMs,
                        zoomRange.endMs
                    )
                    : await GetNodeMetricsHistory(
                        requestIdString,
                        prometheusInfo.namespace,
                        prometheusInfo.service,
                        prometheusInfo.port,
                        nodeName,
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
                    setError(err.toString());
                    setLoading(false);
                }
            }
        };

        fetchMetrics();
    }, [prometheusInfo, nodeName, duration, isStale, zoomRange]);

    // Pass through metrics data (reserved data comes from backend if available)
    const enrichedMetricsData = useMemo(() => {
        if (!metricsData) return null;
        return metricsData;
    }, [metricsData]);

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
        if (value < 0.01) return `${(value * 1000).toFixed(0)}m`;
        return `${value.toFixed(2)} cores`;
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
                    Historical metrics require Prometheus with node-exporter in your cluster.
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

                {enrichedMetricsData && (
                    <div className="space-y-6">
                        {/* CPU and Memory charts */}
                        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
                            <NodeResourceChart
                                data={enrichedMetricsData.cpu}
                                color="stroke-blue-500"
                                label="CPU"
                                formatValue={formatCPU}
                                duration={effectiveDuration}
                                markers={filteredMarkers}
                                onZoomSelect={handleZoomSelect}
                            />
                            <NodeResourceChart
                                data={enrichedMetricsData.memory}
                                color="stroke-blue-500"
                                label="Memory"
                                formatValue={formatBytes}
                                duration={effectiveDuration}
                                memoryDetails
                                markers={filteredMarkers}
                                onZoomSelect={handleZoomSelect}
                            />
                        </div>

                        {/* Pods and Network */}
                        <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
                            <PodCountChart
                                data={enrichedMetricsData.pods}
                                duration={duration}
                            />
                            <NetworkChart
                                data={enrichedMetricsData.network}
                                duration={duration}
                            />
                        </div>
                    </div>
                )}
            </div>
        </div>
    );
}
