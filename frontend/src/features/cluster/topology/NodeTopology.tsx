import React, { useMemo, useState, useCallback, useRef, useEffect } from 'react';
import {
    ReactFlow,
    ReactFlowProvider,
    Controls,
    Background,
    MiniMap,
    useOnViewportChange,
    applyNodeChanges,
} from '@xyflow/react';
import type { NodeChange } from '@xyflow/react';
import '@xyflow/react/dist/style.css';
import { usePods } from '~/hooks/resources';
import { usePodMetrics } from '~/hooks/usePodMetrics';
import { useK8s } from '~/context';
import { getPodStatus } from '~/utils/k8s-helpers';
import TopologyNodeComponent from './TopologyNodeComponent';
import {
    groupPodsByNode,
    getZoomLevel,
    computeGridPositions,
    computeResourceMaxes,
    computePodGridHeight,
    getPodSquareColor,
    getPodCommitted,
    getPodResourceRequests,
    getNamespaceColor,
    formatCpuMillis,
    formatMemBytes,
    ZoomLevel,
    ColorMode,
    EvictionCategory,
    PodMetricsMap,
} from './topologyUtils';
import PodDetails from '~/components/shared/PodDetails';
import { useUI } from '~/context';
import { useNotification } from '~/context/NotificationContext';
import { GetPodEvictionInfo, EvictPod } from 'wailsjs/go/main/App';
import { CubeIcon, SwatchIcon, SignalIcon } from '@heroicons/react/24/outline';

interface NodeTopologyProps {
    isVisible: boolean;
    nodes: any[];
    nodesLoading: boolean;
    metrics: Record<string, any>;
    metricsAvailable: boolean | null;
    nodeActions: any;
}

const nodeTypes = { topologyNode: TopologyNodeComponent };

// ---------------------------------------------------------------------------
// Status legend items
// ---------------------------------------------------------------------------
const STATUS_LEGEND = [
    ['Running', '#22c55e'],
    ['Pending', '#f59e0b'],
    ['Failed', '#ef4444'],
    ['CrashLoop', '#f97316'],
    ['Terminating', '#6b7280'],
] as const;

const RESOURCE_LEGEND = [
    ['Top', '#ef4444'],
    ['High', '#f59e0b'],
    ['Moderate', '#22c55e'],
    ['Low', '#3b82f6'],
    ['No requests', '#6b7280'],
] as const;

/** Walk up from target to find the closest element with data-pod-uid */
function findPodUid(target: EventTarget | null): string | null {
    let el = target as HTMLElement | null;
    while (el) {
        if (el.dataset?.podUid) return el.dataset.podUid;
        if (el.dataset?.nodeName !== undefined) return null; // hit node root, stop
        el = el.parentElement;
    }
    return null;
}

/** Check if click target is an interactive element inside a node (namespace toggle, expand button) */
function isNodeInteractive(target: EventTarget | null): boolean {
    let el = target as HTMLElement | null;
    while (el) {
        if (el.dataset?.nsToggle !== undefined || el.dataset?.nodeAction !== undefined) return true;
        if (el.dataset?.nodeName !== undefined) return false;
        el = el.parentElement;
    }
    return false;
}

// ---------------------------------------------------------------------------
// Inner component — must be inside ReactFlowProvider for useOnViewportChange
// ---------------------------------------------------------------------------
function TopologyCanvas({
    rfNodes,
    podsLoading,
    onNodeClick,
    onPaneClick,
}: {
    rfNodes: any[];
    podsLoading: boolean;
    onNodeClick: (event: React.MouseEvent, rfNode: any) => void;
    onPaneClick: () => void;
}) {
    const [zoomLevel, setZoomLevel] = useState<ZoomLevel>('medium');
    const zoomRef = useRef<ZoomLevel>('medium');

    // Track user-dragged positions so they survive data/zoom updates
    const draggedPositions = useRef<Map<string, { x: number; y: number }>>(new Map());
    // Pause upstream syncs while any node is being dragged
    const isDragging = useRef(false);

    // State-based nodes for real-time drag preview
    const [displayNodes, setDisplayNodes] = useState<any[]>([]);

    // Sync incoming rfNodes + zoomLevel into displayNodes, preserving dragged positions.
    // Skipped while dragging — deferred sync happens on drag end.
    const pendingSync = useRef(false);
    useEffect(() => {
        if (isDragging.current) {
            pendingSync.current = true;
            return;
        }
        setDisplayNodes(
            rfNodes.map((n: any) => {
                const podGridH = computePodGridHeight(n.data?.pods?.length || 0, zoomLevel);
                const dragged = draggedPositions.current.get(n.id);
                return {
                    ...n,
                    position: dragged || n.position,
                    data: { ...n.data, zoomLevel },
                    style: { ...n.style, height: 120 + podGridH },
                };
            }),
        );
    }, [rfNodes, zoomLevel]);

    useOnViewportChange({
        onEnd: useCallback((viewport: any) => {
            const level = getZoomLevel(viewport.zoom);
            if (level !== zoomRef.current) {
                zoomRef.current = level;
                setZoomLevel(level);
            }
        }, []),
    });

    // Refs for deferred sync after drag ends
    const rfNodesRef = useRef(rfNodes);
    rfNodesRef.current = rfNodes;
    const zoomLevelRef = useRef(zoomLevel);
    zoomLevelRef.current = zoomLevel;

    // Handle all node changes (position during drag, selection, etc.)
    const handleNodesChange = useCallback((changes: NodeChange[]) => {
        setDisplayNodes((prev) => applyNodeChanges(changes, prev));

        for (const change of changes) {
            if (change.type === 'position') {
                if (change.dragging) {
                    isDragging.current = true;
                } else {
                    // Drag ended — persist position and flush any deferred sync
                    isDragging.current = false;
                    if (change.position) {
                        draggedPositions.current.set(change.id, { x: change.position.x, y: change.position.y });
                    }
                    if (pendingSync.current) {
                        pendingSync.current = false;
                        const nodes = rfNodesRef.current;
                        const zl = zoomLevelRef.current;
                        setDisplayNodes(
                            nodes.map((n: any) => {
                                const podGridH = computePodGridHeight(n.data?.pods?.length || 0, zl);
                                const dragged = draggedPositions.current.get(n.id);
                                return {
                                    ...n,
                                    position: dragged || n.position,
                                    data: { ...n.data, zoomLevel: zl },
                                    style: { ...n.style, height: 120 + podGridH },
                                };
                            }),
                        );
                    }
                }
            }
        }
    }, []);

    return (
        <ReactFlow
            nodes={displayNodes}
            edges={[]}
            nodeTypes={nodeTypes}
            onNodesChange={handleNodesChange}
            fitView
            fitViewOptions={{ padding: 0.3 }}
            minZoom={0.1}
            maxZoom={2}
            panOnScroll
            nodesConnectable={false}
            nodesDraggable
            elementsSelectable={false}
            onNodeClick={onNodeClick}
            onPaneClick={onPaneClick}
            proOptions={{ hideAttribution: true }}
        >
            <Controls className="!bg-surface-light !border-border" />
            <Background color="#3d3d3d" gap={16} />
            <MiniMap
                nodeColor={(n) => {
                    const data = n.data as any;
                    if (!data?.isReady) return '#ef4444';
                    if (data?.isUnschedulable) return '#f59e0b';
                    return '#334155';
                }}
                maskColor="rgba(0,0,0,0.7)"
                className="!bg-gray-900 !border-border"
            />
        </ReactFlow>
    );
}

// ---------------------------------------------------------------------------
// Main component
// ---------------------------------------------------------------------------
export default function NodeTopology({
    isVisible,
    nodes,
    nodesLoading,
    metrics,
    metricsAvailable,
    nodeActions,
}: NodeTopologyProps) {
    const { currentContext } = useK8s();
    const { openTab, openModal, closeModal } = useUI();
    const { addNotification } = useNotification();

    const [colorMode, setColorMode] = useState<ColorMode>('status');
    const [dimmedNamespaces, setDimmedNamespaces] = useState<Set<string>>(new Set());

    const handleNamespaceToggle = useCallback((ns: string) => {
        setDimmedNamespaces((prev) => {
            const next = new Set(prev);
            if (next.has(ns)) next.delete(ns);
            else next.add(ns);
            return next;
        });
    }, []);

    // Fetch all pods (all namespaces) only when topology is visible
    const { pods: allPods, loading: podsLoading } = usePods(currentContext, [''], isVisible) as any;

    // Fetch pod metrics for committed resource values (polls every 30s)
    const hasPods = !!(allPods && allPods.length > 0);
    const { metrics: podMetricsRaw, available: podMetricsAvailable } = usePodMetrics(isVisible, hasPods);
    const podMetrics: PodMetricsMap | undefined = podMetricsAvailable ? podMetricsRaw : undefined;

    // Group pods by node (Succeeded pods already excluded)
    const podsByNode = useMemo(() => groupPodsByNode(allPods || []), [allPods]);

    // Max committed resource values per node for relative coloring
    const resourceMaxesByNode = useMemo(() => {
        const map = new Map<string, { maxCpu: number; maxMem: number }>();
        for (const [nodeName, pods] of podsByNode) {
            map.set(nodeName, computeResourceMaxes(pods, podMetrics));
        }
        return map;
    }, [podsByNode, podMetrics]);

    // Pod lookup by uid — for resolving clicks from data-pod-uid
    const podByUid = useMemo(() => {
        const map = new Map<string, any>();
        for (const pod of allPods || []) {
            const uid = pod.metadata?.uid;
            if (uid) map.set(uid, pod);
        }
        return map;
    }, [allPods]);

    // -----------------------------------------------------------------------
    // Imperative tooltip — zero re-renders
    // -----------------------------------------------------------------------
    const tooltipRef = useRef<HTMLDivElement>(null);
    const ttNameRef = useRef<HTMLDivElement>(null);
    const ttNsDotRef = useRef<HTMLSpanElement>(null);
    const ttNsRef = useRef<HTMLSpanElement>(null);
    const ttStatusRef = useRef<HTMLDivElement>(null);
    const ttResourcesRef = useRef<HTMLDivElement>(null);

    const showPodTooltip = useCallback((e: React.MouseEvent, pod: any) => {
        const tip = tooltipRef.current;
        if (!tip) return;
        const rect = (e.currentTarget as HTMLElement).getBoundingClientRect();
        tip.style.display = 'block';
        tip.style.left = `${rect.left + rect.width / 2}px`;
        tip.style.top = `${rect.top}px`;

        const name = pod.metadata?.name || '';
        const ns = pod.metadata?.namespace || '';
        const status = getPodStatus(pod);

        if (ttNameRef.current) ttNameRef.current.textContent = name;
        if (ttNsRef.current) ttNsRef.current.textContent = ns;
        if (ttNsDotRef.current) ttNsDotRef.current.style.backgroundColor = getNamespaceColor(ns);
        if (ttStatusRef.current) {
            ttStatusRef.current.textContent = status;
            ttStatusRef.current.style.color = getPodSquareColor(pod, 'status');
        }
        if (ttResourcesRef.current) {
            const { cpuMillis, memBytes } = getPodCommitted(pod, podMetricsRef.current);
            if (cpuMillis || memBytes) {
                ttResourcesRef.current.textContent = `${formatCpuMillis(cpuMillis)} CPU · ${formatMemBytes(memBytes)} Mem`;
                ttResourcesRef.current.style.display = '';
            } else {
                ttResourcesRef.current.style.display = 'none';
            }
        }
    }, []);

    const hidePodTooltip = useCallback(() => {
        const tip = tooltipRef.current;
        if (tip) tip.style.display = 'none';
    }, []);

    // -----------------------------------------------------------------------
    // Imperative pod action popover — zero re-renders
    // -----------------------------------------------------------------------
    const popoverRef = useRef<HTMLDivElement>(null);
    const popoverNameRef = useRef<HTMLDivElement>(null);
    const popoverPodRef = useRef<any>(null);

    const hidePopover = useCallback(() => {
        const el = popoverRef.current;
        if (el) el.style.display = 'none';
        popoverPodRef.current = null;
    }, []);

    // Close popover when clicking anywhere outside (direct DOM, no deps)
    React.useEffect(() => {
        const handler = (e: MouseEvent) => {
            const el = popoverRef.current;
            if (el && el.style.display !== 'none' && !el.contains(e.target as Node)) {
                el.style.display = 'none';
                popoverPodRef.current = null;
            }
        };
        document.addEventListener('mousedown', handler);
        return () => document.removeEventListener('mousedown', handler);
    }, []);

    const openPodDetails = useCallback((pod: any) => {
        const name = pod.metadata?.name;
        const tabId = `details-pod-${pod.metadata?.uid}`;
        openTab({
            id: tabId,
            title: name,
            icon: CubeIcon,
            content: <PodDetails pod={pod} tabContext={currentContext} />,
            resourceMeta: { kind: 'Pod', name, namespace: pod.metadata?.namespace },
        });
    }, [currentContext, openTab]);

    const showEvictionDialog = useCallback(async (pod: any) => {
        const ns = pod.metadata?.namespace || '';
        const name = pod.metadata?.name || '';
        let info: { category: EvictionCategory; ownerKind: string; ownerName: string };
        try {
            info = await GetPodEvictionInfo(ns, name);
        } catch (err: any) {
            addNotification({ type: 'error', message: `Failed to get eviction info: ${err?.message || err}` });
            return;
        }

        const doEvict = async () => {
            closeModal();
            try {
                await EvictPod(ns, name);
                addNotification({ type: 'success', message: `Pod "${name}" evicted` });
            } catch (err: any) {
                addNotification({ type: 'error', message: `Eviction failed: ${err?.message || err}` });
            }
        };

        const ownerLabel = info.ownerKind ? `${info.ownerKind}/${info.ownerName}` : '';

        if (info.category === 'daemon') {
            openModal({
                title: `Cannot Evict "${name}"`,
                content: (
                    <div className="text-sm text-gray-300 space-y-2">
                        <p>Managed by <span className="font-medium text-white">{ownerLabel}</span>. Evicting will just respawn it on this node.</p>
                    </div>
                ),
            });
        } else if (info.category === 'killable') {
            const desc = info.ownerKind === 'Job' ? 'a Job pod' : 'a standalone pod';
            openModal({
                title: `Kill Pod "${name}"?`,
                content: (
                    <div className="text-sm text-gray-300 space-y-2">
                        <p>This is {desc}. It will <span className="font-medium text-red-400">NOT</span> be rescheduled.</p>
                    </div>
                ),
                confirmText: 'Kill',
                confirmStyle: 'danger',
                onConfirm: doEvict,
            });
        } else {
            openModal({
                title: `Evict Pod "${name}"?`,
                content: (
                    <div className="text-sm text-gray-300 space-y-2">
                        <p>Managed by <span className="font-medium text-white">{ownerLabel}</span>. A new pod will be scheduled on another node.</p>
                    </div>
                ),
                confirmText: 'Evict',
                onConfirm: doEvict,
            });
        }
    }, [addNotification, openModal, closeModal]);

    // -----------------------------------------------------------------------
    // Stable callbacks
    // -----------------------------------------------------------------------
    const showNodeDetailsRef = useRef(nodeActions.handleShowDetails);
    showNodeDetailsRef.current = nodeActions.handleShowDetails;

    const openPodDetailsRef = useRef(openPodDetails);
    openPodDetailsRef.current = openPodDetails;
    const showEvictionDialogRef = useRef(showEvictionDialog);
    showEvictionDialogRef.current = showEvictionDialog;
    const podByUidRef = useRef(podByUid);
    podByUidRef.current = podByUid;
    const podMetricsRef = useRef(podMetrics);
    podMetricsRef.current = podMetrics;

    const handlePopoverDetails = useCallback(() => {
        const pod = popoverPodRef.current;
        hidePopover();
        if (pod) openPodDetailsRef.current(pod);
    }, [hidePopover]);

    const handlePopoverEvict = useCallback(() => {
        const pod = popoverPodRef.current;
        hidePopover();
        if (pod) showEvictionDialogRef.current(pod);
    }, [hidePopover]);

    // React Flow onNodeClick — routes to pod popover or node details
    const handleRFNodeClick = useCallback((event: React.MouseEvent, _rfNode: any) => {
        // Interactive elements (namespace toggles, expand buttons) handle their own clicks
        if (isNodeInteractive(event.target)) return;

        hidePodTooltip();
        hidePopover();

        // Check if a pod square was clicked
        const podUid = findPodUid(event.target);
        if (podUid) {
            const pod = podByUidRef.current.get(podUid);
            if (pod) {
                const el = popoverRef.current;
                if (!el) return;
                // Position popover near the clicked element, with boundary detection
                const target = event.target as HTMLElement;
                const rect = target.getBoundingClientRect();
                el.style.display = 'block';
                // Measure popover dimensions for boundary check
                const popW = el.offsetWidth;
                const popH = el.offsetHeight;
                const vw = window.innerWidth;
                const vh = window.innerHeight;
                // Horizontal: center on target, clamp to viewport
                let left = rect.left + rect.width / 2;
                if (left - popW / 2 < 8) left = popW / 2 + 8;
                else if (left + popW / 2 > vw - 8) left = vw - popW / 2 - 8;
                // Vertical: prefer below, flip above if not enough space
                let top: number;
                let flipAbove = false;
                if (rect.bottom + 4 + popH > vh - 8) {
                    top = rect.top - 4;
                    flipAbove = true;
                } else {
                    top = rect.bottom + 4;
                }
                el.style.left = `${left}px`;
                el.style.top = `${top}px`;
                el.style.transform = flipAbove ? 'translate(-50%, -100%)' : 'translateX(-50%)';
                popoverPodRef.current = pod;
                if (popoverNameRef.current) popoverNameRef.current.textContent = pod.metadata?.name || '';
                return;
            }
        }

        // Otherwise, find the node object from data-node-name
        let el = event.target as HTMLElement | null;
        while (el) {
            if (el.dataset?.nodeName) {
                const nodeName = el.dataset.nodeName;
                const node = nodes.find((n: any) => n.metadata?.name === nodeName);
                if (node) showNodeDetailsRef.current(node);
                return;
            }
            el = el.parentElement;
        }
    }, [nodes, hidePodTooltip, hidePopover]);

    // -----------------------------------------------------------------------
    // Build React Flow nodes
    // -----------------------------------------------------------------------
    const rfNodes = useMemo(() => {
        if (!nodes || nodes.length === 0) return [];
        const positions = computeGridPositions(nodes.length);
        return nodes.map((node: any, i: number) => {
            const name = node.metadata?.name || '';
            const conditions = node.status?.conditions || [];
            const readyCondition = conditions.find((c: any) => c.type === 'Ready');
            const isReady = readyCondition?.status === 'True';
            const isUnschedulable = node.spec?.unschedulable === true;
            const taintCount = (node.spec?.taints || []).length;
            const nodePods = podsByNode.get(name) || [];
            const nodeMetrics = metrics[name] || null;

            const podGridH = computePodGridHeight(nodePods.length, 'medium');
            const h = 120 + podGridH;

            return {
                id: `node-${name}`,
                type: 'topologyNode' as const,
                position: positions[i],
                data: {
                    nodeName: name,
                    node,
                    pods: nodePods,
                    metrics: nodeMetrics,
                    metricsAvailable,
                    zoomLevel: 'medium' as ZoomLevel,
                    colorMode,
                    resourceMaxes: resourceMaxesByNode.get(name) || { maxCpu: 0, maxMem: 0 },
                    podMetrics,
                    isReady,
                    isUnschedulable,
                    taintCount,
                    dimmedNamespaces,
                    onPodHover: showPodTooltip,
                    onPodHoverEnd: hidePodTooltip,
                    onNamespaceToggle: handleNamespaceToggle,
                },
                style: { width: 280, height: h },
            };
        });
    }, [nodes, podsByNode, metrics, metricsAvailable, colorMode, resourceMaxesByNode, podMetrics, dimmedNamespaces, showPodTooltip, hidePodTooltip, handleNamespaceToggle]);

    // -----------------------------------------------------------------------
    // Render
    // -----------------------------------------------------------------------
    if (nodesLoading && nodes.length === 0) {
        return (
            <div className="flex items-center justify-center h-full bg-background">
                <div className="text-gray-400">Loading nodes...</div>
            </div>
        );
    }

    if (nodes.length === 0) {
        return (
            <div className="flex items-center justify-center h-full bg-background">
                <div className="text-gray-400">No nodes found</div>
            </div>
        );
    }

    const legendItems = colorMode === 'status' ? STATUS_LEGEND : RESOURCE_LEGEND;

    return (
        <div className="h-full w-full bg-background relative">
            <ReactFlowProvider>
                <TopologyCanvas rfNodes={rfNodes} podsLoading={podsLoading} onNodeClick={handleRFNodeClick} onPaneClick={hidePopover} />
            </ReactFlowProvider>

            {/* Color mode toggle */}
            <div className="absolute top-4 left-4 bg-surface-light border border-border rounded-lg p-0.5 flex gap-0.5 z-10">
                <button
                    onClick={() => setColorMode('status')}
                    className={`flex items-center gap-1 px-2 py-1 rounded text-[11px] transition-colors ${
                        colorMode === 'status' ? 'bg-surface text-white' : 'text-gray-500 hover:text-gray-300'
                    }`}
                    title="Color by pod status"
                >
                    <SignalIcon className="w-3.5 h-3.5" />
                    Status
                </button>
                <button
                    onClick={() => setColorMode('resource')}
                    className={`flex items-center gap-1 px-2 py-1 rounded text-[11px] transition-colors ${
                        colorMode === 'resource' ? 'bg-surface text-white' : 'text-gray-500 hover:text-gray-300'
                    }`}
                    title="Color by resource profile (CPU vs memory requests)"
                >
                    <SwatchIcon className="w-3.5 h-3.5" />
                    Resources
                </button>
            </div>

            {/* Legend */}
            <div className="absolute bottom-4 left-4 bg-surface-light border border-border rounded-lg p-3 text-xs z-10">
                <div className="text-gray-400 font-medium mb-2">
                    {colorMode === 'status' ? 'Pod Status' : 'Resource Profile'}
                </div>
                <div className="grid grid-cols-2 gap-x-4 gap-y-1">
                    {legendItems.map(([label, color]) => (
                        <div key={label} className="flex items-center gap-2">
                            <div className="w-3 h-3 rounded-sm" style={{ backgroundColor: color }} />
                            <span className="text-gray-300">{label}</span>
                        </div>
                    ))}
                </div>
            </div>

            {/* Pod loading indicator */}
            {podsLoading && !allPods?.length && (
                <div className="absolute top-4 right-4 bg-surface-light border border-border rounded-lg px-3 py-1.5 text-xs text-gray-400 z-10">
                    Loading pods...
                </div>
            )}

            {/* Imperative tooltip — positioned fixed, outside React Flow */}
            <div
                ref={tooltipRef}
                className="fixed z-[100] pointer-events-none"
                style={{ display: 'none', transform: 'translate(-50%, -100%) translateY(-6px)' }}
            >
                <div
                    className="rounded shadow-lg whitespace-nowrap"
                    style={{ fontSize: 10, padding: '6px 8px', background: '#111827', border: '1px solid #374151' }}
                >
                    <div ref={ttNameRef} className="font-medium" style={{ color: 'white' }} />
                    <div className="flex items-center gap-1" style={{ color: '#9ca3af' }}>
                        <span ref={ttNsDotRef} className="inline-block rounded-full" style={{ width: 6, height: 6 }} />
                        <span ref={ttNsRef} />
                    </div>
                    <div ref={ttStatusRef} />
                    <div ref={ttResourcesRef} style={{ color: '#9ca3af' }} />
                </div>
            </div>

            {/* Pod action popover — positioned fixed, below clicked pod */}
            <div
                ref={popoverRef}
                className="fixed z-[110]"
                style={{ display: 'none', transform: 'translateX(-50%)' }}
            >
                <div
                    className="rounded-lg shadow-xl overflow-hidden"
                    style={{ background: '#111827', border: '1px solid #374151', minWidth: 120 }}
                >
                    <div ref={popoverNameRef} className="px-3 py-1.5 text-[11px] font-medium text-white truncate max-w-[200px] border-b border-gray-700/50" />
                    <button
                        className="w-full px-3 py-1.5 text-left text-xs text-gray-200 hover:bg-gray-700/60 transition-colors"
                        onClick={handlePopoverDetails}
                    >
                        View Details
                    </button>
                    <button
                        className="w-full px-3 py-1.5 text-left text-xs text-orange-400 hover:bg-gray-700/60 transition-colors"
                        onClick={handlePopoverEvict}
                    >
                        Evict Pod...
                    </button>
                </div>
            </div>
        </div>
    );
}
