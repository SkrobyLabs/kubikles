import React, { useMemo, useState } from 'react';
import { ZoomLevel, ColorMode, ResourceMaxes, PodMetricsMap, getPodSquareColor, getNamespaceColor, sortPods, computePodGridHeight, getPodSquareSize, getMaxVisiblePods } from './topologyUtils';

/** Show top N namespaces in the close-zoom breakdown */
const MAX_NAMESPACE_LABELS = 5;

interface TopologyNodeData {
    nodeName: string;
    node: any;
    pods: any[];
    metrics: any | null;
    metricsAvailable: boolean | null;
    zoomLevel: ZoomLevel;
    colorMode: ColorMode;
    resourceMaxes: ResourceMaxes;
    podMetrics?: PodMetricsMap;
    isReady: boolean;
    isUnschedulable: boolean;
    taintCount: number;
    dimmedNamespaces: Set<string>;
    onPodHover: (e: React.MouseEvent, pod: any) => void;
    onPodHoverEnd: () => void;
    onNamespaceToggle: (ns: string) => void;
}

/** Micro-bar for CPU/Memory percentage */
function MetricBar({ label, percent, color }: { label: string; percent: number; color: string }) {
    return (
        <div className="flex items-center gap-1.5 text-[10px]">
            <span className="text-gray-400 w-8 shrink-0">{label}</span>
            <div className="flex-1 h-1.5 bg-gray-700 rounded-full overflow-hidden min-w-[40px]">
                <div
                    className="h-full rounded-full transition-all"
                    style={{ width: `${Math.min(100, percent)}%`, backgroundColor: color }}
                />
            </div>
            <span className="text-gray-400 w-8 text-right">{percent}%</span>
        </div>
    );
}

/** Pod square — click handled by React Flow onNodeClick, tooltip via onPodHover */
const PodSquare = React.memo(function PodSquare({
    pod,
    zoomLevel,
    colorMode,
    resourceMaxes,
    podMetrics,
    isDimmed,
    onHover,
    onHoverEnd,
}: {
    pod: any;
    zoomLevel: ZoomLevel;
    colorMode: ColorMode;
    resourceMaxes: ResourceMaxes;
    podMetrics?: PodMetricsMap;
    isDimmed: boolean;
    onHover: (e: React.MouseEvent, pod: any) => void;
    onHoverEnd: () => void;
}) {
    const bgColor = getPodSquareColor(pod, colorMode, resourceMaxes, podMetrics);
    const size = getPodSquareSize(zoomLevel);

    return (
        <div
            className="cursor-pointer rounded-sm"
            data-pod-uid={pod.metadata?.uid}
            style={{
                backgroundColor: bgColor,
                width: size,
                height: size,
                minWidth: size,
                minHeight: size,
                opacity: isDimmed ? 0.15 : 1,
            }}
            onMouseEnter={(e) => onHover(e, pod)}
            onMouseLeave={onHoverEnd}
        />
    );
});

/** Custom React Flow node for a K8s node */
const TopologyNodeComponent = React.memo(function TopologyNodeComponent({ data }: { data: TopologyNodeData }) {
    const {
        nodeName,
        node,
        pods,
        metrics,
        metricsAvailable,
        zoomLevel,
        colorMode,
        resourceMaxes,
        podMetrics,
        isReady,
        isUnschedulable,
        taintCount,
        dimmedNamespaces,
        onPodHover,
        onPodHoverEnd,
        onNamespaceToggle,
    } = data;

    const [expanded, setExpanded] = useState(false);

    const sortedPods = useMemo(() => sortPods(pods, colorMode, resourceMaxes, podMetrics), [pods, colorMode, resourceMaxes, podMetrics]);
    const podCount = sortedPods.length;
    const maxVisible = getMaxVisiblePods(zoomLevel);
    const visiblePods = expanded ? sortedPods : sortedPods.slice(0, maxVisible);
    const hiddenCount = expanded ? 0 : podCount - visiblePods.length;

    // Dynamic height based on pod count and square size
    const baseHeight = 120;
    const podGridHeight = computePodGridHeight(visiblePods.length, zoomLevel);
    const nodeHeight = baseHeight + podGridHeight;

    // Namespace labels for close zoom
    const namespaceStats = useMemo(() => {
        if (zoomLevel !== 'close') return [];
        const nsMap = new Map<string, number>();
        for (const pod of pods) {
            const ns = pod.metadata?.namespace || 'default';
            nsMap.set(ns, (nsMap.get(ns) || 0) + 1);
        }
        return Array.from(nsMap.entries())
            .sort((a, b) => b[1] - a[1])
            .slice(0, MAX_NAMESPACE_LABELS);
    }, [zoomLevel, pods]);

    return (
        <div
            className="rounded-lg border-2 cursor-pointer transition-shadow hover:shadow-lg hover:shadow-black/30"
            data-node-name={node.metadata?.name}
            style={{
                width: 280,
                minHeight: nodeHeight,
                backgroundColor: '#1e293b',
                borderColor: isReady ? (isUnschedulable ? '#f59e0b' : '#334155') : '#ef4444',
            }}
        >
            {/* Header */}
            <div className="px-3 py-2 border-b border-gray-700/50">
                <div className="flex items-center justify-between">
                    <div className="flex items-center gap-2 min-w-0">
                        <span className="text-sm font-medium text-white truncate">{nodeName}</span>
                    </div>
                    <div className="flex items-center gap-1.5 shrink-0">
                        <span className="px-1.5 py-0.5 text-[10px] rounded bg-blue-500/20 text-blue-400">
                            {podCount} pods
                        </span>
                        {!isReady && (
                            <span className="px-1.5 py-0.5 text-[10px] rounded bg-red-500/20 text-red-400">
                                NotReady
                            </span>
                        )}
                        {isUnschedulable && (
                            <span className="px-1.5 py-0.5 text-[10px] rounded bg-yellow-500/20 text-yellow-400">
                                Cordoned
                            </span>
                        )}
                    </div>
                </div>

                {metricsAvailable !== false && metrics && (
                    <div className="mt-1.5 space-y-0.5">
                        <MetricBar label="CPU" percent={metrics.cpuPercent} color="#3b82f6" />
                        <MetricBar label="Mem" percent={metrics.memPercent} color="#8b5cf6" />
                    </div>
                )}

                {zoomLevel !== 'far' && taintCount > 0 && (
                    <div className="mt-1 text-[10px] text-gray-500">
                        {taintCount} taint{taintCount > 1 ? 's' : ''}
                    </div>
                )}
            </div>

            {/* Pod grid (medium + close zoom) */}
            {zoomLevel !== 'far' && podCount > 0 && (
                <div className="px-3 py-2">
                    <div className="flex flex-wrap gap-1">
                        {visiblePods.map((pod: any) => (
                            <PodSquare
                                key={pod.metadata?.uid}
                                pod={pod}
                                zoomLevel={zoomLevel}
                                colorMode={colorMode}
                                resourceMaxes={resourceMaxes}
                                podMetrics={podMetrics}
                                isDimmed={dimmedNamespaces.has(pod.metadata?.namespace || 'default')}
                                onHover={onPodHover}
                                onHoverEnd={onPodHoverEnd}
                            />
                        ))}
                        {hiddenCount > 0 && (
                            <button
                                className="flex items-center justify-center text-[10px] text-blue-400 hover:text-blue-300 px-1 cursor-pointer"
                                data-node-action
                                onClick={(e) => { e.stopPropagation(); setExpanded(true); }}
                            >
                                +{hiddenCount}
                            </button>
                        )}
                        {expanded && podCount > maxVisible && (
                            <button
                                className="flex items-center justify-center text-[10px] text-gray-500 hover:text-gray-400 px-1 cursor-pointer"
                                data-node-action
                                onClick={(e) => { e.stopPropagation(); setExpanded(false); }}
                            >
                                show less
                            </button>
                        )}
                    </div>

                    {zoomLevel === 'close' && namespaceStats.length > 0 && (
                        <div className="flex flex-wrap gap-1 mt-2 pt-1.5 border-t border-gray-700/50">
                            {namespaceStats.map(([ns, count]) => {
                                const isDimmed = dimmedNamespaces.has(ns);
                                return (
                                    <button
                                        key={ns}
                                        className="flex items-center gap-1 text-[9px] rounded px-0.5 transition-opacity hover:opacity-80"
                                        style={{
                                            color: isDimmed ? '#4b5563' : '#9ca3af',
                                            textDecoration: isDimmed ? 'line-through' : 'none',
                                        }}
                                        data-ns-toggle={ns}
                                        onClick={(e) => { e.stopPropagation(); onNamespaceToggle(ns); }}
                                    >
                                        <span
                                            className="inline-block w-1.5 h-1.5 rounded-full"
                                            style={{
                                                backgroundColor: getNamespaceColor(ns),
                                                opacity: isDimmed ? 0.3 : 1,
                                            }}
                                        />
                                        {ns} ({count})
                                    </button>
                                );
                            })}
                        </div>
                    )}
                </div>
            )}
        </div>
    );
});

export default TopologyNodeComponent;
