import React, { useState, useEffect, useRef, useCallback, useMemo } from 'react';
import { ChartBarIcon, ExclamationTriangleIcon } from '@heroicons/react/24/outline';
import { DetectPrometheus, GetNodeMetricsHistory } from '../../../wailsjs/go/main/App';
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

// Node resource chart with toggleable lines:
// - Usage (blue), Allocatable (gray dashed), Uncommitted (green) visible by default
// - Committed (orange) toggleable, off by default
// Memoized to prevent re-renders when parent updates with same props
const NodeResourceChart = React.memo(({ data, color, label, formatValue, duration }) => {
    const containerRef = useRef(null);
    const [hoveredIndex, setHoveredIndex] = useState(null);
    const [mousePos, setMousePos] = useState({ x: 0, y: 0 });
    const [showCommitted, setShowCommitted] = useState(false);

    const hasUsage = data?.usage?.length > 0;
    const hasAllocatable = data?.allocatable?.length > 0;
    const hasUncommitted = data?.uncommitted?.length > 0;
    const hasCommitted = data?.committed?.length > 0;

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
    const uncommitted = data.uncommitted || [];
    const committed = data.committed || [];

    // Calculate Y-axis bounds
    const allValues = [
        ...usage.map(d => d.value),
        ...allocatable.map(d => d.value),
        ...uncommitted.map(d => d.value),
        ...(showCommitted ? committed.map(d => d.value) : [])
    ];
    const max = Math.max(...allValues) || 1;
    const min = 0;
    const range = max - min || 1;
    const yMin = 0;
    const yMax = max * 1.05;
    const yRange = yMax - yMin || 1;

    // Chart dimensions - wider aspect ratio for better horizontal usage
    const width = 500;
    const height = 200;
    const paddingLeft = 70;
    const paddingRight = 20;
    const paddingTop = 20;
    const paddingBottom = 30;
    const chartWidth = width - paddingLeft - paddingRight;
    const chartHeight = height - paddingTop - paddingBottom;

    // Generate points for lines
    const generatePoints = (dataPoints) => {
        return dataPoints.map((d, i) => {
            const x = paddingLeft + (i / (dataPoints.length - 1)) * chartWidth;
            const y = paddingTop + chartHeight - ((d.value - yMin) / yRange) * chartHeight;
            return { x, y, value: d.value, timestamp: d.timestamp };
        });
    };

    const usagePoints = generatePoints(usage);
    const allocatablePoints = hasAllocatable ? generatePoints(allocatable) : [];
    const uncommittedPoints = hasUncommitted ? generatePoints(uncommitted) : [];
    const committedPoints = hasCommitted ? generatePoints(committed) : [];

    const createPath = (points) => points.map((p, i) => `${i === 0 ? 'M' : 'L'} ${p.x} ${p.y}`).join(' ');

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
    const xTicks = xTickIndices.map(i => ({
        timestamp: usage[i]?.timestamp,
        x: paddingLeft + (i / (usage.length - 1)) * chartWidth
    }));

    const currentUsage = usage[usage.length - 1]?.value || 0;
    const currentAllocatable = allocatable[allocatable.length - 1]?.value || 0;
    const currentUncommitted = uncommitted[uncommitted.length - 1]?.value || 0;

    const handleMouseMove = useCallback((e) => {
        if (!containerRef.current) return;
        const rect = containerRef.current.getBoundingClientRect();
        const svgX = ((e.clientX - rect.left) / rect.width) * width;
        const svgY = ((e.clientY - rect.top) / rect.height) * height;

        if (svgX >= paddingLeft && svgX <= width - paddingRight &&
            svgY >= paddingTop && svgY <= height - paddingBottom) {
            const chartX = svgX - paddingLeft;
            const index = Math.round((chartX / chartWidth) * (usage.length - 1));
            const clampedIndex = Math.max(0, Math.min(usage.length - 1, index));
            setHoveredIndex(clampedIndex);
            setMousePos({ x: e.clientX - rect.left, y: e.clientY - rect.top });
        } else {
            setHoveredIndex(null);
        }
    }, [usage.length, chartWidth]);

    const handleMouseLeave = useCallback(() => {
        setHoveredIndex(null);
    }, []);

    const hoveredPoint = hoveredIndex !== null ? usagePoints[hoveredIndex] : null;
    const hoveredAllocatable = hoveredIndex !== null && allocatablePoints[hoveredIndex];
    const hoveredUncommitted = hoveredIndex !== null && uncommittedPoints[hoveredIndex];
    const hoveredCommitted = hoveredIndex !== null && committedPoints[hoveredIndex];

    return (
        <div className="relative">
            <div className="flex items-center justify-between mb-2">
                <div className="flex items-center gap-3">
                    <span className="text-sm font-medium text-gray-300">{label}</span>
                    {/* Legend - clickable to toggle */}
                    <div className="flex items-center gap-2 text-xs">
                        <span className="flex items-center gap-1 text-blue-400">
                            <span className="w-3 h-0.5 bg-blue-500"></span>
                            Usage
                        </span>
                        <span className="flex items-center gap-1 text-gray-400">
                            <span className="w-3 h-0.5 bg-gray-500 border-dashed"></span>
                            Allocatable
                        </span>
                        <span className="flex items-center gap-1 text-green-400">
                            <span className="w-3 h-0.5 bg-green-500"></span>
                            Uncommitted
                        </span>
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
                    </div>
                </div>
                <span className="text-sm text-gray-400">
                    Current: <span className={`font-medium ${color.replace('stroke-', 'text-')}`}>
                        {formatValue(currentUsage)}
                    </span>
                </span>
            </div>
            <div
                ref={containerRef}
                className="h-56 bg-background rounded border border-border relative"
                onMouseMove={handleMouseMove}
                onMouseLeave={handleMouseLeave}
            >
                <svg viewBox={`0 0 ${width} ${height}`} className="w-full h-full">
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
                    {xTicks.map((tick, i) => (
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

                    {/* Uncommitted line (green) */}
                    {uncommittedPoints.length > 0 && (
                        <path d={createPath(uncommittedPoints)} fill="none" className="stroke-green-500" strokeWidth="1.5" />
                    )}

                    {/* Committed line (orange, toggleable) */}
                    {showCommitted && committedPoints.length > 0 && (
                        <path d={createPath(committedPoints)} fill="none" className="stroke-orange-500" strokeWidth="1.5" strokeDasharray="4,2" />
                    )}

                    {/* Usage area fill */}
                    <path d={areaPath} className="fill-blue-500 opacity-20" />

                    {/* Usage line (blue) */}
                    <path d={usagePath} fill="none" className={color} strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />

                    {/* Hover indicator */}
                    {hoveredPoint && (
                        <>
                            <line x1={hoveredPoint.x} y1={paddingTop} x2={hoveredPoint.x} y2={paddingTop + chartHeight}
                                className="stroke-gray-400" strokeWidth="1" strokeDasharray="4,2" />
                            <circle cx={hoveredPoint.x} cy={hoveredPoint.y} r="4"
                                className="fill-blue-500" stroke="white" strokeWidth="2" />
                            {hoveredAllocatable && (
                                <circle cx={hoveredAllocatable.x} cy={hoveredAllocatable.y} r="3"
                                    className="fill-gray-500" stroke="white" strokeWidth="1.5" />
                            )}
                            {hoveredUncommitted && (
                                <circle cx={hoveredUncommitted.x} cy={hoveredUncommitted.y} r="3"
                                    className="fill-green-500" stroke="white" strokeWidth="1.5" />
                            )}
                            {showCommitted && hoveredCommitted && (
                                <circle cx={hoveredCommitted.x} cy={hoveredCommitted.y} r="3"
                                    className="fill-orange-500" stroke="white" strokeWidth="1.5" />
                            )}
                        </>
                    )}
                </svg>

                {/* Tooltip */}
                {hoveredPoint && (
                    <div
                        className="absolute z-10 pointer-events-none bg-surface border border-border rounded-lg shadow-lg px-3 py-2"
                        style={{
                            left: Math.min(mousePos.x + 10, containerRef.current?.offsetWidth - 170 || 0),
                            top: mousePos.y - 80
                        }}
                    >
                        <div className="text-xs text-gray-400 mb-1">
                            {new Date(hoveredPoint.timestamp).toLocaleString()}
                        </div>
                        <div className="text-sm font-medium text-blue-400">
                            Usage: {formatValue(hoveredPoint.value)}
                        </div>
                        {hoveredAllocatable && (
                            <div className="text-xs text-gray-400">Allocatable: {formatValue(hoveredAllocatable.value)}</div>
                        )}
                        {hoveredUncommitted && (
                            <div className="text-xs text-green-400">Uncommitted: {formatValue(hoveredUncommitted.value)}</div>
                        )}
                        {showCommitted && hoveredCommitted && (
                            <div className="text-xs text-orange-400">Committed: {formatValue(hoveredCommitted.value)}</div>
                        )}
                    </div>
                )}
            </div>
        </div>
    );
});

// Pod count chart showing current vs capacity
// Memoized to prevent re-renders when parent updates with same props
const PodCountChart = React.memo(({ data, duration }) => {
    const containerRef = useRef(null);
    const [hoveredIndex, setHoveredIndex] = useState(null);
    const [mousePos, setMousePos] = useState({ x: 0, y: 0 });

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

    const allValues = [...running.map(d => d.value), ...capacity.map(d => d.value)];
    const max = Math.max(...allValues, 1);
    const yMax = max * 1.1;
    const yRange = yMax || 1;

    const width = 400;
    const height = 120;
    const paddingLeft = 45;
    const paddingRight = 20;
    const paddingTop = 15;
    const paddingBottom = 25;
    const chartWidth = width - paddingLeft - paddingRight;
    const chartHeight = height - paddingTop - paddingBottom;

    const generatePoints = (dataPoints) => {
        return dataPoints.map((d, i) => {
            const x = paddingLeft + (i / (dataPoints.length - 1)) * chartWidth;
            const y = paddingTop + chartHeight - (d.value / yRange) * chartHeight;
            return { x, y, value: d.value, timestamp: d.timestamp };
        });
    };

    const runningPoints = generatePoints(running);
    const capacityPoints = hasCapacity ? generatePoints(capacity) : [];

    const createPath = (points) => points.map((p, i) => `${i === 0 ? 'M' : 'L'} ${p.x} ${p.y}`).join(' ');

    const handleMouseMove = useCallback((e) => {
        if (!containerRef.current) return;
        const rect = containerRef.current.getBoundingClientRect();
        const svgX = ((e.clientX - rect.left) / rect.width) * width;
        if (svgX >= paddingLeft && svgX <= width - paddingRight) {
            const chartX = svgX - paddingLeft;
            const index = Math.round((chartX / chartWidth) * (running.length - 1));
            setHoveredIndex(Math.max(0, Math.min(running.length - 1, index)));
            setMousePos({ x: e.clientX - rect.left, y: e.clientY - rect.top });
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
                <svg viewBox={`0 0 ${width} ${height}`} className="w-full h-full">
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
                        style={{ left: Math.min(mousePos.x + 10, containerRef.current?.offsetWidth - 100 || 0), top: mousePos.y - 40 }}>
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

// Network I/O chart
// Memoized to prevent re-renders when parent updates with same props
const NetworkChart = React.memo(({ data, duration }) => {
    const containerRef = useRef(null);
    const [hoveredIndex, setHoveredIndex] = useState(null);
    const [mousePos, setMousePos] = useState({ x: 0, y: 0 });

    const hasRx = data?.receiveBytes?.length > 0;
    const hasTx = data?.transmitBytes?.length > 0;

    if (!hasRx && !hasTx) {
        return (
            <div className="relative">
                <div className="text-sm font-medium text-gray-300 mb-2">Network I/O</div>
                <div className="h-36 flex items-center justify-center text-gray-500 text-sm bg-background rounded border border-border">
                    No data available
                </div>
            </div>
        );
    }

    const rx = data.receiveBytes || [];
    const tx = data.transmitBytes || [];

    const allValues = [...rx.map(d => d.value), ...tx.map(d => d.value)];
    const max = Math.max(...allValues, 1);
    const yMax = max * 1.1;
    const yRange = yMax || 1;

    const width = 400;
    const height = 120;
    const paddingLeft = 55;
    const paddingRight = 20;
    const paddingTop = 15;
    const paddingBottom = 25;
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
        const svgX = ((e.clientX - rect.left) / rect.width) * width;
        if (svgX >= paddingLeft && svgX <= width - paddingRight) {
            const chartX = svgX - paddingLeft;
            const index = Math.round((chartX / chartWidth) * (baseData.length - 1));
            setHoveredIndex(Math.max(0, Math.min(baseData.length - 1, index)));
            setMousePos({ x: e.clientX - rect.left, y: e.clientY - rect.top });
        } else {
            setHoveredIndex(null);
        }
    }, [baseData.length, chartWidth]);

    const formatRate = (value) => {
        if (value == null || isNaN(value)) return '-';
        return `${formatBytes(value)}/s`;
    };

    const currentRx = rx[rx.length - 1]?.value || 0;
    const currentTx = tx[tx.length - 1]?.value || 0;
    const hoveredRxPoint = hoveredIndex !== null && rxPoints[hoveredIndex];
    const hoveredTxPoint = hoveredIndex !== null && txPoints[hoveredIndex];

    return (
        <div className="relative">
            <div className="flex items-center justify-between mb-1">
                <div className="flex items-center gap-3">
                    <span className="text-xs font-medium text-gray-400">Network I/O</span>
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
                <div className="text-xs">
                    {hasRx && <span className="text-cyan-400 mr-2">↓{formatRate(currentRx)}</span>}
                    {hasTx && <span className="text-yellow-400">↑{formatRate(currentTx)}</span>}
                </div>
            </div>
            <div ref={containerRef} className="h-36 bg-background rounded border border-border relative"
                onMouseMove={handleMouseMove} onMouseLeave={() => setHoveredIndex(null)}>
                <svg viewBox={`0 0 ${width} ${height}`} className="w-full h-full">
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
                        style={{ left: Math.min(mousePos.x + 10, containerRef.current?.offsetWidth - 120 || 0), top: mousePos.y - 40 }}>
                        {hoveredRxPoint && <div className="text-cyan-400">RX: {formatRate(hoveredRxPoint.value)}</div>}
                        {hoveredTxPoint && <div className="text-yellow-400">TX: {formatRate(hoveredTxPoint.value)}</div>}
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

export default function NodeMetricsTab({ nodeName, isStale }) {
    const [prometheusInfo, setPrometheusInfo] = useState(null);
    const [detecting, setDetecting] = useState(true);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState(null);
    const [metricsData, setMetricsData] = useState(null);
    const [duration, setDuration] = useState('1h');

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
        if (!prometheusInfo?.available || isStale) return;

        const fetchMetrics = async () => {
            setLoading(true);
            setError(null);
            try {
                const data = await GetNodeMetricsHistory(
                    prometheusInfo.namespace,
                    prometheusInfo.service,
                    prometheusInfo.port,
                    nodeName,
                    duration
                );
                setMetricsData(data);
            } catch (err) {
                setError(err.toString());
            } finally {
                setLoading(false);
            }
        };

        fetchMetrics();
    }, [prometheusInfo, nodeName, duration, isStale]);

    // Calculate uncommitted = allocatable - committed for each data point
    const enrichedMetricsData = useMemo(() => {
        if (!metricsData) return null;

        const calculateUncommitted = (allocatable, committed) => {
            if (!allocatable?.length) return [];
            if (!committed?.length) {
                // If no committed data, uncommitted = allocatable
                return allocatable.map(p => ({ timestamp: p.timestamp, value: p.value }));
            }

            // Find closest committed value for each allocatable timestamp
            return allocatable.map(p => {
                // Find the committed point with the closest timestamp
                let closestCommitted = committed[0];
                let minDiff = Math.abs(p.timestamp - committed[0].timestamp);
                for (const c of committed) {
                    const diff = Math.abs(p.timestamp - c.timestamp);
                    if (diff < minDiff) {
                        minDiff = diff;
                        closestCommitted = c;
                    }
                }
                return {
                    timestamp: p.timestamp,
                    value: Math.max(0, p.value - closestCommitted.value)
                };
            });
        };

        return {
            ...metricsData,
            cpu: metricsData.cpu ? {
                ...metricsData.cpu,
                uncommitted: calculateUncommitted(metricsData.cpu.allocatable, metricsData.cpu.committed)
            } : null,
            memory: metricsData.memory ? {
                ...metricsData.memory,
                uncommitted: calculateUncommitted(metricsData.memory.allocatable, metricsData.memory.committed)
            } : null
        };
    }, [metricsData]);

    const formatCPU = (value) => {
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
                <div className="flex items-center gap-1 bg-surface-light rounded-md p-0.5">
                    {DURATIONS.map(d => (
                        <button
                            key={d.value}
                            onClick={() => setDuration(d.value)}
                            className={`px-3 py-1 text-xs font-medium rounded transition-colors ${
                                duration === d.value ? 'bg-primary text-white' : 'text-gray-400 hover:text-white'
                            }`}
                        >
                            {d.label}
                        </button>
                    ))}
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
                                duration={duration}
                            />
                            <NodeResourceChart
                                data={enrichedMetricsData.memory}
                                color="stroke-blue-500"
                                label="Memory"
                                formatValue={formatBytes}
                                duration={duration}
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
