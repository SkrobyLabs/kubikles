import React, { useState, useEffect, useMemo, useRef, useCallback } from 'react';
import { ChartBarIcon, ExclamationTriangleIcon } from '@heroicons/react/24/outline';
import { DetectPrometheus, GetControllerMetricsHistory } from '../../../wailsjs/go/main/App';
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

// Interactive line chart component with toggleable request/limit lines
const MetricsChart = ({ data, color, label, formatValue, duration, request, limit }) => {
    const containerRef = useRef(null);
    const [hoveredIndex, setHoveredIndex] = useState(null);
    const [mousePos, setMousePos] = useState({ x: 0, y: 0 });
    const [showRequest, setShowRequest] = useState(false);
    const [showLimit, setShowLimit] = useState(false);

    // Get request/limit values (use last data point if available)
    const requestValue = request?.length > 0 ? request[request.length - 1]?.value : null;
    const limitValue = limit?.length > 0 ? limit[limit.length - 1]?.value : null;

    // Chart dimensions (constants)
    const width = 400;
    const height = 160;
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
        if (showLimit && limitValue && limitValue > max) max = limitValue * 1.05;
        if (showRequest && requestValue && requestValue > max) max = requestValue * 1.05;

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

        // Y axis ticks
        const yTicks = Array.from({ length: 5 }, (_, i) => {
            const value = yMin + (yRange * i) / 4;
            const y = paddingTop + chartHeight - (i / 4) * chartHeight;
            return { value, y };
        });

        // X axis ticks
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
            requestY: getYPos(requestValue),
            limitY: getYPos(limitValue),
        };
    }, [data, showRequest, showLimit, requestValue, limitValue, chartWidth, chartHeight]);

    if (!chartData) {
        return (
            <div className="h-48 flex items-center justify-center text-gray-500 text-sm bg-background rounded border border-border">
                No data available
            </div>
        );
    }

    const { points, linePath, areaPath, yTicks, xTicks, currentValue, requestY, limitY } = chartData;

    const handleMouseMove = useCallback((e) => {
        if (!containerRef.current) return;
        const rect = containerRef.current.getBoundingClientRect();
        const svgX = ((e.clientX - rect.left) / rect.width) * width;
        const svgY = ((e.clientY - rect.top) / rect.height) * height;

        if (svgX >= paddingLeft && svgX <= width - paddingRight &&
            svgY >= paddingTop && svgY <= height - paddingBottom) {
            const chartX = svgX - paddingLeft;
            const index = Math.round((chartX / chartWidth) * (data.length - 1));
            const clampedIndex = Math.max(0, Math.min(data.length - 1, index));
            setHoveredIndex(clampedIndex);
            setMousePos({ x: e.clientX - rect.left, y: e.clientY - rect.top });
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
                className="h-48 bg-background rounded border border-border relative"
                onMouseMove={handleMouseMove}
                onMouseLeave={handleMouseLeave}
            >
                <svg viewBox={`0 0 ${width} ${height}`} className="w-full h-full" preserveAspectRatio="none">
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

                    {/* Hover indicator */}
                    {hoveredPoint && (
                        <>
                            <line x1={hoveredPoint.x} y1={paddingTop} x2={hoveredPoint.x} y2={paddingTop + chartHeight}
                                className="stroke-gray-400" strokeWidth="1" strokeDasharray="4,2" />
                            <circle cx={hoveredPoint.x} cy={hoveredPoint.y} r="4"
                                className={`${color.replace('stroke-', 'fill-')}`} stroke="white" strokeWidth="2" />
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
                        {showRequest && requestValue != null && (
                            <div className="text-xs text-orange-400">Request: {formatValue(requestValue)}</div>
                        )}
                        {showLimit && limitValue != null && (
                            <div className="text-xs text-red-400">Limit: {formatValue(limitValue)}</div>
                        )}
                    </div>
                )}
            </div>
        </div>
    );
};

// Simple count chart for pods/restarts
const CountChart = ({ data, color, label, duration }) => {
    const containerRef = useRef(null);
    const [hoveredIndex, setHoveredIndex] = useState(null);
    const [mousePos, setMousePos] = useState({ x: 0, y: 0 });

    if (!data || data.length === 0) {
        return (
            <div className="h-32 flex items-center justify-center text-gray-500 text-sm bg-background rounded border border-border">
                No data available
            </div>
        );
    }

    const values = data.map(d => d.value);
    const timestamps = data.map(d => d.timestamp);
    const max = Math.max(...values, 1);
    const min = Math.min(...values, 0);
    const range = max - min || 1;
    const yMin = Math.max(0, min - range * 0.1);
    const yMax = max + range * 0.1;
    const yRange = yMax - yMin || 1;

    const width = 400;
    const height = 100;
    const paddingLeft = 40;
    const paddingRight = 20;
    const paddingTop = 15;
    const paddingBottom = 25;
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
        const svgX = ((e.clientX - rect.left) / rect.width) * width;
        if (svgX >= paddingLeft && svgX <= width - paddingRight) {
            const chartX = svgX - paddingLeft;
            const index = Math.round((chartX / chartWidth) * (data.length - 1));
            setHoveredIndex(Math.max(0, Math.min(data.length - 1, index)));
            setMousePos({ x: e.clientX - rect.left, y: e.clientY - rect.top });
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
                <svg viewBox={`0 0 ${width} ${height}`} className="w-full h-full" preserveAspectRatio="none">
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
                        style={{ left: Math.min(mousePos.x + 10, containerRef.current?.offsetWidth - 80 || 0), top: mousePos.y - 30 }}>
                        <span className={color.replace('stroke-', 'text-')}>{Math.round(hoveredPoint.value)}</span>
                    </div>
                )}
            </div>
        </div>
    );
};

const DURATIONS = [
    { value: '1h', label: '1h' },
    { value: '6h', label: '6h' },
    { value: '24h', label: '24h' },
    { value: '7d', label: '7d' },
    { value: '30d', label: '30d' },
    { value: 'all', label: 'All' },
];

export default function ControllerMetricsTab({ namespace, name, controllerType, isStale }) {
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
                const data = await GetControllerMetricsHistory(
                    prometheusInfo.namespace,
                    prometheusInfo.service,
                    prometheusInfo.port,
                    namespace,
                    name,
                    controllerType,
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
    }, [prometheusInfo, namespace, name, controllerType, duration, isStale]);

    const formatCPU = (value) => {
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
                <div className="flex items-center gap-1 bg-[#2d2d2d] rounded-md p-0.5">
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

                {metricsData && (
                    <div className="space-y-6">
                        {/* CPU and Memory charts */}
                        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
                            <MetricsChart
                                data={metricsData.cpu?.usage}
                                color="stroke-blue-500"
                                label="CPU Usage (Aggregated)"
                                formatValue={formatCPU}
                                duration={duration}
                                request={metricsData.cpu?.request}
                                limit={metricsData.cpu?.limit}
                            />
                            <MetricsChart
                                data={metricsData.memory?.usage}
                                color="stroke-purple-500"
                                label="Memory Usage (Aggregated)"
                                formatValue={formatBytes}
                                duration={duration}
                                request={metricsData.memory?.request}
                                limit={metricsData.memory?.limit}
                            />
                        </div>

                        {/* Pod count and Restarts */}
                        <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
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
                        </div>
                    </div>
                )}
            </div>
        </div>
    );
}
