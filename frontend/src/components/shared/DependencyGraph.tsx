import React, { useState, useEffect, useCallback, useMemo, useRef } from 'react';
import ReactDOM from 'react-dom';
import {
    ReactFlow,
    Controls,
    Background,
    useNodesState,
    useEdgesState,
    MarkerType,
    Handle,
    Position,
} from '@xyflow/react';
import dagre from 'dagre';
import '@xyflow/react/dist/style.css';
import { GetResourceDependencies, ExpandDependencyNode } from 'wailsjs/go/main/App';
import { useUI } from '~/context';
import { LazyYamlEditor as YamlEditor } from '../lazy';
import Logger from '~/utils/Logger';
import {
    CubeIcon,
    RocketLaunchIcon,
    CircleStackIcon,
    CpuChipIcon,
    Square2StackIcon,
    CommandLineIcon,
    ClockIcon,
    DocumentTextIcon,
    LockClosedIcon,
    GlobeAltIcon,
    ServerStackIcon,
    ServerIcon,
    QueueListIcon,
    BoltIcon,
    ShieldCheckIcon,
    ArrowsRightLeftIcon,
    TagIcon,
    UserCircleIcon,
    ArrowsPointingOutIcon,
    ShieldExclamationIcon,
    EllipsisHorizontalIcon,
} from '@heroicons/react/24/outline';

// Resource kinds that support dependency graph queries
const DEPENDENCY_SUPPORTED_KINDS = new Set([
    'Pod', 'Deployment', 'StatefulSet', 'DaemonSet', 'ReplicaSet',
    'Job', 'CronJob', 'ConfigMap', 'Secret', 'Service',
    'PersistentVolumeClaim', 'PersistentVolume', 'StorageClass',
    'Ingress', 'IngressClass', 'Endpoints', 'NetworkPolicy', 'PriorityClass',
    'ServiceAccount', 'HorizontalPodAutoscaler', 'PodDisruptionBudget'
]);

// Map resource kinds to icons and colors
const resourceStyles = {
    Pod: { icon: CubeIcon, color: '#3b82f6', bgColor: '#3b82f620' },
    Deployment: { icon: RocketLaunchIcon, color: '#22c55e', bgColor: '#22c55e20' },
    StatefulSet: { icon: CircleStackIcon, color: '#8b5cf6', bgColor: '#8b5cf620' },
    DaemonSet: { icon: CpuChipIcon, color: '#f59e0b', bgColor: '#f59e0b20' },
    ReplicaSet: { icon: Square2StackIcon, color: '#06b6d4', bgColor: '#06b6d420' },
    Job: { icon: CommandLineIcon, color: '#ec4899', bgColor: '#ec489920' },
    CronJob: { icon: ClockIcon, color: '#f97316', bgColor: '#f9731620' },
    ConfigMap: { icon: DocumentTextIcon, color: '#84cc16', bgColor: '#84cc1620' },
    Secret: { icon: LockClosedIcon, color: '#ef4444', bgColor: '#ef444420' },
    Service: { icon: GlobeAltIcon, color: '#14b8a6', bgColor: '#14b8a620' },
    PersistentVolumeClaim: { icon: CircleStackIcon, color: '#a855f7', bgColor: '#a855f720' },
    PersistentVolume: { icon: ServerStackIcon, color: '#6366f1', bgColor: '#6366f120' },
    StorageClass: { icon: ServerIcon, color: '#78716c', bgColor: '#78716c20' },
    Ingress: { icon: ArrowsRightLeftIcon, color: '#f472b6', bgColor: '#f472b620' },
    IngressClass: { icon: TagIcon, color: '#94a3b8', bgColor: '#94a3b820' },
    Endpoints: { icon: QueueListIcon, color: '#22d3ee', bgColor: '#22d3ee20' },
    NetworkPolicy: { icon: ShieldCheckIcon, color: '#fb923c', bgColor: '#fb923c20' },
    PriorityClass: { icon: BoltIcon, color: '#facc15', bgColor: '#facc1520' },
    ServiceAccount: { icon: UserCircleIcon, color: '#c084fc', bgColor: '#c084fc20' },
    HorizontalPodAutoscaler: { icon: ArrowsPointingOutIcon, color: '#2dd4bf', bgColor: '#2dd4bf20' },
    PodDisruptionBudget: { icon: ShieldExclamationIcon, color: '#fbbf24', bgColor: '#fbbf2420' },
};

// Pure function - moved outside component to avoid recreation
const getStatusColor = (status: string | undefined): string => {
    if (!status) return 'text-gray-400';
    const s = status.toLowerCase();
    if (s === 'running' || s === 'bound' || s === 'available' || s === 'active') return 'text-green-400';
    if (s === 'pending' || s === 'released') return 'text-yellow-400';
    if (s === 'failed' || s === 'lost' || s === 'terminated') return 'text-red-400';
    return 'text-gray-400';
};

// Parse metadata values once - returns { ready, total } or null
const parseRatio = (value: string | undefined): { ready: number; total: number } | null => {
    if (!value) return null;
    const [ready, total] = value.split('/').map(Number);
    return { ready, total };
};

// Custom node component - memoized to prevent unnecessary re-renders
const ResourceNode = React.memo(function ResourceNode({ data }: any) {
    const [showTooltip, setShowTooltip] = useState(false);
    const [tooltipPos, setTooltipPos] = useState({ x: 0, y: 0 });
    const nodeRef = useRef<HTMLDivElement>(null);

    // Memoize style lookup - only recalculates when kind changes
    const style = useMemo(
        () => (resourceStyles as Record<string, any>)[data.kind] || { icon: CubeIcon, color: '#6b7280', bgColor: '#6b728020' },
        [data.kind]
    );
    const Icon = data.isSummary ? EllipsisHorizontalIcon : style.icon;

    // Memoize charm calculation - only recalculates when metadata changes
    const charm = useMemo(() => {
        if (!data.metadata) return null;
        // Replicas charm for workloads (Deployment, StatefulSet, DaemonSet, ReplicaSet)
        const replicas = parseRatio(data.metadata.replicas);
        if (replicas) {
            const isHealthy = replicas.ready === replicas.total && replicas.total > 0;
            return {
                text: data.metadata.replicas,
                label: 'Replicas',
                ready: replicas.ready,
                total: replicas.total,
                color: isHealthy ? 'text-green-400' : 'text-yellow-400',
                bg: isHealthy ? 'bg-green-500/30' : 'bg-yellow-500/30',
                border: isHealthy ? 'border-green-500/50' : 'border-yellow-500/50',
            };
        }
        // Completions charm for Jobs
        const completions = parseRatio(data.metadata.completions);
        if (completions) {
            const isComplete = completions.ready === completions.total;
            return {
                text: data.metadata.completions,
                label: 'Completions',
                ready: completions.ready,
                total: completions.total,
                color: isComplete ? 'text-green-400' : 'text-blue-400',
                bg: isComplete ? 'bg-green-500/30' : 'bg-blue-500/30',
                border: isComplete ? 'border-green-500/50' : 'border-blue-500/50',
            };
        }
        // Restarts charm for Pods
        if (data.metadata.restarts) {
            const restarts = parseInt(data.metadata.restarts, 10);
            const isHigh = restarts > 5;
            return {
                text: restarts.toString(),
                label: 'Restarts',
                ready: restarts,
                total: 0,
                color: isHigh ? 'text-red-400' : 'text-yellow-400',
                bg: isHigh ? 'bg-red-500/30' : 'bg-yellow-500/30',
                border: isHigh ? 'border-red-500/50' : 'border-yellow-500/50',
                icon: 'restart',
            };
        }
        // Capacity charm for PVC/PV
        if (data.metadata.capacity) {
            return {
                text: data.metadata.capacity,
                label: 'Capacity',
                ready: 0,
                total: 0,
                color: 'text-purple-400',
                bg: 'bg-purple-500/30',
                border: 'border-purple-500/50',
            };
        }
        // Key count charm for Secret
        if (data.metadata.keys && data.kind === 'Secret') {
            const keys = parseInt(data.metadata.keys, 10);
            return {
                text: `${keys}`,
                label: 'Keys',
                ready: keys,
                total: 0,
                color: 'text-red-400',
                bg: 'bg-red-500/30',
                border: 'border-red-500/50',
                icon: 'lock',
            };
        }
        // Key count charm for ConfigMap
        if (data.metadata.keys && data.kind === 'ConfigMap') {
            const keys = parseInt(data.metadata.keys, 10);
            return {
                text: `${keys}`,
                label: 'Keys',
                ready: keys,
                total: 0,
                color: 'text-lime-400',
                bg: 'bg-lime-500/30',
                border: 'border-lime-500/50',
                icon: 'values',
            };
        }
        return null;
    }, [data.metadata]);

    // Memoize event handlers
    const handleMouseEnter = useCallback(() => {
        if (nodeRef.current) {
            const rect = nodeRef.current.getBoundingClientRect();
            setTooltipPos({ x: rect.right + 8, y: rect.top });
            setShowTooltip(true);
        }
    }, []);

    const handleMouseLeave = useCallback(() => {
        setShowTooltip(false);
    }, []);

    // Memoize tooltip content - reuses parsed charm data
    const tooltipContent = useMemo(() => {
        const lines: { label: string; value: string; color?: string }[] = [];
        lines.push({ label: 'Name', value: data.label });
        if (data.namespace) {
            lines.push({ label: 'Namespace', value: data.namespace });
        }
        if (data.status) {
            lines.push({ label: 'Status', value: data.status, color: getStatusColor(data.status) });
        }
        // Reuse charm's parsed values instead of re-parsing
        if (charm?.label === 'Replicas') {
            lines.push({
                label: 'Replicas',
                value: `${charm.ready} ready / ${charm.total} desired`,
                color: charm.color
            });
        } else if (charm?.label === 'Completions') {
            lines.push({
                label: 'Completions',
                value: `${charm.ready} / ${charm.total}`,
                color: charm.color
            });
        } else if (charm?.label === 'Restarts') {
            lines.push({
                label: 'Restarts',
                value: charm.text,
                color: charm.color
            });
        } else if (charm?.label === 'Capacity') {
            lines.push({
                label: 'Capacity',
                value: charm.text,
                color: charm.color
            });
        } else if (charm?.label === 'Keys') {
            lines.push({
                label: 'Keys',
                value: `${charm.ready} key${charm.ready !== 1 ? 's' : ''}`,
                color: charm.color
            });
        }
        return lines;
    }, [data.label, data.namespace, data.status, charm]);

    // Summary nodes have a dashed border and different styling
    if (data.isSummary) {
        return (
            <div
                className="px-3 py-2 rounded-lg border-2 border-dashed min-w-[140px] cursor-pointer transition-all hover:scale-105"
                style={{
                    backgroundColor: 'rgb(var(--gray-700))',
                    borderColor: style.color,
                }}
                onContextMenu={data.onContextMenu}
            >
                <Handle type="target" position={Position.Top} className="!bg-gray-500" />
                <div className="flex items-center gap-2">
                    <Icon className="h-5 w-5" style={{ color: style.color }} />
                    <div className="flex flex-col">
                        <span className="text-xs text-gray-400">{data.kind}</span>
                        <span className="text-sm font-medium text-gray-300">
                            {data.label}
                        </span>
                        <span className="text-xs text-gray-500">Right-click to expand</span>
                    </div>
                </div>
                <Handle type="source" position={Position.Bottom} className="!bg-gray-500" />
            </div>
        );
    }

    return (
        <>
            <div
                ref={nodeRef}
                className="px-3 py-2 rounded-lg border-2 min-w-[140px] cursor-pointer transition-all hover:scale-105"
                style={{
                    backgroundColor: style.bgColor,
                    borderColor: style.color,
                }}
                onContextMenu={data.onContextMenu}
                onMouseEnter={handleMouseEnter}
                onMouseLeave={handleMouseLeave}
            >
                <Handle type="target" position={Position.Top} className="!bg-gray-500" />
                <div className="flex items-center gap-2">
                    <Icon className="h-5 w-5" style={{ color: style.color }} />
                    <div className="flex flex-col flex-1 min-w-0">
                        <div className="flex items-center justify-between gap-2">
                            <span className="text-xs text-gray-400 shrink-0">{data.kind}</span>
                            {charm && (
                                <span
                                    className={`text-[11px] font-semibold px-1 py-0.5 rounded ${charm.color} ${charm.bg} border ${charm.border} flex items-center gap-0.5 whitespace-nowrap shrink-0`}
                                >
                                    {charm.icon === 'restart' && <span>&#x21bb;</span>}
                                    {charm.icon === 'lock' && <LockClosedIcon className="h-3 w-3" />}
                                    {charm.icon === 'values' && <span>#</span>}
                                    {charm.text}
                                </span>
                            )}
                        </div>
                        <span className="text-sm font-medium text-text truncate max-w-[120px]">
                            {data.label}
                        </span>
                        {data.status && (
                            <span className={`text-xs ${getStatusColor(data.status)}`}>
                                {data.status}
                            </span>
                        )}
                    </div>
                </div>
                <Handle type="source" position={Position.Bottom} className="!bg-gray-500" />
            </div>
            {/* Tooltip rendered via portal to escape React Flow's clipping */}
            {showTooltip && ReactDOM.createPortal(
                <div
                    className="fixed z-[9999] bg-gray-900 border border-gray-700 rounded-lg shadow-xl p-3 text-sm pointer-events-none"
                    style={{ left: tooltipPos.x, top: tooltipPos.y }}
                >
                    <div className="font-medium text-text mb-2 flex items-center gap-2">
                        <Icon className="h-4 w-4" style={{ color: style.color }} />
                        {data.kind}
                    </div>
                    <div className="space-y-1">
                        {tooltipContent.map((item, i) => (
                            <div key={i} className="flex justify-between gap-4">
                                <span className="text-gray-400">{item.label}:</span>
                                <span className={item.color || 'text-text'}>{item.value}</span>
                            </div>
                        ))}
                    </div>
                </div>,
                document.body
            )}
        </>
    );
});

const nodeTypes = {
    resource: ResourceNode,
};

// Layout the graph using dagre
function getLayoutedElements(nodes: any, edges: any) {
    const dagreGraph = new dagre.graphlib.Graph();
    dagreGraph.setDefaultEdgeLabel(() => ({}));
    dagreGraph.setGraph({ rankdir: 'TB', nodesep: 60, ranksep: 80 });

    nodes.forEach((node: any) => {
        dagreGraph.setNode(node.id, { width: 160, height: 70 });
    });

    edges.forEach((edge: any) => {
        dagreGraph.setEdge(edge.source, edge.target);
    });

    dagre.layout(dagreGraph);

    const layoutedNodes = nodes.map((node: any) => {
        const nodeWithPosition = dagreGraph.node(node.id);
        return {
            ...node,
            position: {
                x: nodeWithPosition.x - 80,
                y: nodeWithPosition.y - 35,
            },
        };
    });

    return { nodes: layoutedNodes, edges };
}

// Edge styles based on relation type
const getEdgeStyle = (relation: any) => {
    switch (relation) {
        case 'owns':
            return { stroke: '#22c55e', strokeWidth: 2 };
        case 'uses':
            return { stroke: '#3b82f6', strokeWidth: 2, strokeDasharray: '5,5' };
        case 'binds':
            return { stroke: '#a855f7', strokeWidth: 2 };
        case 'selects':
            return { stroke: '#14b8a6', strokeWidth: 2, strokeDasharray: '3,3' };
        case 'routes-to':
            return { stroke: '#f472b6', strokeWidth: 2 };
        case 'references':
            return { stroke: '#22d3ee', strokeWidth: 2, strokeDasharray: '5,5' };
        case 'applies-to':
            return { stroke: '#fb923c', strokeWidth: 2, strokeDasharray: '3,3' };
        case 'scales':
            return { stroke: '#2dd4bf', strokeWidth: 2 };
        case 'protects':
            return { stroke: '#fbbf24', strokeWidth: 2, strokeDasharray: '3,3' };
        default:
            return { stroke: '#6b7280', strokeWidth: 1 };
    }
};

export default function DependencyGraph({ resourceType, namespace, resourceName, onClose }: any) {
    const { openTab, closeTab, openDiagnostic, navigateWithSearch } = useUI();
    const [nodes, setNodes, onNodesChange] = useNodesState([]);
    const [edges, setEdges, onEdgesChange] = useEdgesState([]);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<any>(null);
    const [contextMenu, setContextMenu] = useState<any>(null);
    const [expansionOffsets, setExpansionOffsets] = useState<Record<string, any>>({}); // Track offset per summary node

    // Refs to track current state for reading without triggering setState callbacks
    const nodesRef = useRef(nodes);
    const edgesRef = useRef(edges);
    useEffect(() => { nodesRef.current = nodes; }, [nodes]);
    useEffect(() => { edgesRef.current = edges; }, [edges]);

    // Map resource type to kind for context menu actions
    const getResourceTypeFromKind = (kind: any) => {
        const mapping = {
            Pod: 'pod',
            Deployment: 'deployment',
            StatefulSet: 'statefulset',
            DaemonSet: 'daemonset',
            ReplicaSet: 'replicaset',
            Job: 'job',
            CronJob: 'cronjob',
            ConfigMap: 'configmap',
            Secret: 'secret',
            Service: 'service',
            PersistentVolumeClaim: 'pvc',
            PersistentVolume: 'pv',
            StorageClass: 'storageclass',
            Ingress: 'ingress',
            IngressClass: 'ingressclass',
            Endpoints: 'endpoints',
            NetworkPolicy: 'networkpolicy',
            PriorityClass: 'priorityclass',
            ServiceAccount: 'serviceaccount',
            HorizontalPodAutoscaler: 'hpa',
            PodDisruptionBudget: 'pdb',
        };
        return (mapping as Record<string, string>)[kind] || kind.toLowerCase();
    };

    // Map resource kind to view name for navigation
    const kindToView: Record<string, string> = {
        Pod: 'pods',
        Deployment: 'deployments',
        StatefulSet: 'statefulsets',
        DaemonSet: 'daemonsets',
        ReplicaSet: 'replicasets',
        Job: 'jobs',
        CronJob: 'cronjobs',
        ConfigMap: 'configmaps',
        Secret: 'secrets',
        Service: 'services',
        PersistentVolumeClaim: 'pvcs',
        PersistentVolume: 'pvs',
        StorageClass: 'storageclasses',
        Ingress: 'ingresses',
        IngressClass: 'ingressclasses',
        Endpoints: 'endpoints',
        NetworkPolicy: 'networkpolicies',
        PriorityClass: 'priorityclasses',
        ServiceAccount: 'serviceaccounts',
        HorizontalPodAutoscaler: 'hpas',
        PodDisruptionBudget: 'pdbs',
    };

    const handleNodeContextMenu = useCallback((event: any, nodeData: any) => {
        event.preventDefault();
        setContextMenu({
            x: event.clientX,
            y: event.clientY,
            node: nodeData,
        });
    }, []);

    const handleEditYaml = useCallback((node: any) => {
        const resType = getResourceTypeFromKind(node.kind);
        const tabId = `yaml-${resType}-${node.namespace || ''}-${node.label}`;

        openTab({
            id: tabId,
            title: `${node.label}`,
            content: (
                <YamlEditor
                    resourceType={resType}
                    namespace={node.namespace}
                    resourceName={node.label}
                    onClose={() => closeTab(tabId)}
                />
            ),
        });
        setContextMenu(null);
    }, [openTab, closeTab]);

    const handleShowDependencies = useCallback((node: any) => {
        const resType = getResourceTypeFromKind(node.kind);
        const tabId = `deps-${resType}-${node.namespace || ''}-${node.label}`;

        openTab({
            id: tabId,
            title: `${node.label}`,
            content: (
                <DependencyGraph
                    resourceType={resType}
                    namespace={node.namespace}
                    resourceName={node.label}
                    onClose={() => closeTab(tabId)}
                />
            ),
        });
        setContextMenu(null);
    }, [openTab, closeTab]);

    const handleCompareResource = useCallback((node: any) => {
        const resType = getResourceTypeFromKind(node.kind);
        // Pre-fill both source and target with the same resource
        // User typically just needs to change the target context
        openDiagnostic('resource-diff', {
            initialSource: {
                kind: resType,
                namespace: node.namespace || 'default',
                name: node.label,
                context: ''
            },
            initialTarget: {
                kind: resType,
                namespace: node.namespace || 'default',
                name: node.label,
                context: ''
            }
        });
        setContextMenu(null);
    }, [openDiagnostic]);

    const handleGoToResource = useCallback((node: any) => {
        const viewName = kindToView[node.kind];
        if (viewName) {
            const search = node.namespace
                ? `name:"${node.label}" namespace:"${node.namespace}"`
                : `name:"${node.label}"`;
            navigateWithSearch(viewName, search, true);
        }
        setContextMenu(null);
    }, [navigateWithSearch]);

    // Handle expanding a summary node
    const handleExpandNode = useCallback(async (summaryNode: any) => {
        setContextMenu(null);

        // Calculate the current offset (default 5 for initial expansion)
        const summaryId = `summary:${summaryNode.parentId}:${summaryNode.kind}`;
        const currentOffset = expansionOffsets[summaryId] || 5;

        try {
            Logger.info('Expanding summary node', { summaryId, offset: currentOffset }, 'k8s');
            const expandedGraph = await ExpandDependencyNode(
                resourceType,
                namespace || '',
                resourceName,
                summaryId,
                currentOffset
            );

            if (!expandedGraph || !expandedGraph.nodes) {
                Logger.warn('No expanded nodes returned', undefined, 'k8s');
                return;
            }

            // Convert new nodes to flow nodes
            const newFlowNodes = expandedGraph.nodes.map((node: any) => ({
                id: node.id,
                type: 'resource',
                data: {
                    label: node.name,
                    kind: node.kind,
                    namespace: node.namespace,
                    status: node.status,
                    metadata: node.metadata,
                    isSummary: node.isSummary || false,
                    remainingCount: node.remainingCount || 0,
                    parentId: node.parentId || '',
                    onContextMenu: (e: any) => handleNodeContextMenu(e, {
                        label: node.name,
                        kind: node.kind,
                        namespace: node.namespace,
                        isSummary: node.isSummary || false,
                        remainingCount: node.remainingCount || 0,
                        parentId: node.parentId || '',
                    }),
                },
                position: { x: 0, y: 0 },
            }));

            // Convert new edges
            const newFlowEdges = expandedGraph.edges.map((edge: any, idx: number) => ({
                id: `edge-expand-${currentOffset}-${idx}`,
                source: edge.source,
                target: edge.target,
                type: 'smoothstep',
                animated: edge.relation === 'uses',
                style: getEdgeStyle(edge.relation),
                markerEnd: {
                    type: MarkerType.ArrowClosed,
                    color: getEdgeStyle(edge.relation).stroke,
                },
                label: edge.relation,
                labelStyle: { fill: '#9ca3af', fontSize: 10 },
                labelBgStyle: { fill: '#1f2937', fillOpacity: 0.8 },
            }));

            // Merge new nodes and edges using refs to read current state
            const currentNodes = nodesRef.current;
            const currentEdges = edgesRef.current;

            // Filter out the old summary node and add new nodes
            const filteredNodes = currentNodes.filter((n: any) => n.id !== summaryId);
            const existingNodeIds = new Set(filteredNodes.map((n: any) => n.id));
            const uniqueNewNodes = newFlowNodes.filter((n: any) => !existingNodeIds.has(n.id));
            const allNodes = [...filteredNodes, ...uniqueNewNodes];

            // Filter edges and add new edges
            const filteredEdges = currentEdges.filter((e: any) => e.target !== summaryId && e.source !== summaryId);
            const existingEdgeIds = new Set(filteredEdges.map((e: any) => `${e.source}-${e.target}`));
            const uniqueNewEdges = newFlowEdges.filter((e: any) => !existingEdgeIds.has(`${e.source}-${e.target}`));
            const allEdges = [...filteredEdges, ...uniqueNewEdges];

            // Compute layout once with all data
            const { nodes: layoutedNodes, edges: layoutedEdges } = getLayoutedElements(allNodes, allEdges);

            // Single batch update - React 18 batches these automatically
            setNodes(layoutedNodes);
            setEdges(layoutedEdges);

            // Update offset for potential further expansion
            setExpansionOffsets(prev => ({
                ...prev,
                [summaryId]: currentOffset + 10,
            }));

        } catch (err: any) {
            Logger.error('Failed to expand summary node', err, 'k8s');
        }
    }, [resourceType, namespace, resourceName, expansionOffsets, handleNodeContextMenu]);

    // Close context menu when clicking elsewhere
    useEffect(() => {
        const handleClick = () => setContextMenu(null);
        document.addEventListener('click', handleClick);
        return () => document.removeEventListener('click', handleClick);
    }, []);

    // Stable context menu handler reference to avoid effect re-runs
    const contextMenuHandlerRef = useRef(handleNodeContextMenu);
    useEffect(() => {
        contextMenuHandlerRef.current = handleNodeContextMenu;
    }, [handleNodeContextMenu]);

    // Load dependency data
    useEffect(() => {
        let cancelled = false;

        const loadDependencies = async () => {
            setLoading(true);
            setError(null);

            try {
                Logger.info('Loading dependencies', { resourceType, namespace, resourceName }, 'k8s');
                const graph = await GetResourceDependencies(resourceType, namespace || '', resourceName);

                // Check if effect was cancelled during async operation
                if (cancelled) return;

                if (!graph || !graph.nodes || graph.nodes.length === 0) {
                    setError('No dependencies found for this resource');
                    setLoading(false);
                    return;
                }

                // Convert backend nodes to React Flow nodes
                const flowNodes = graph.nodes.map((node: any) => ({
                    id: node.id,
                    type: 'resource',
                    data: {
                        label: node.name,
                        kind: node.kind,
                        namespace: node.namespace,
                        status: node.status,
                        metadata: node.metadata,
                        isSummary: node.isSummary || false,
                        remainingCount: node.remainingCount || 0,
                        parentId: node.parentId || '',
                        onContextMenu: (e: any) => contextMenuHandlerRef.current(e, {
                            label: node.name,
                            kind: node.kind,
                            namespace: node.namespace,
                            isSummary: node.isSummary || false,
                            remainingCount: node.remainingCount || 0,
                            parentId: node.parentId || '',
                        }),
                    },
                    position: { x: 0, y: 0 },
                }));

                // Convert backend edges to React Flow edges
                const flowEdges = graph.edges.map((edge: any, idx: number) => ({
                    id: `edge-${idx}`,
                    source: edge.source,
                    target: edge.target,
                    type: 'smoothstep',
                    animated: edge.relation === 'uses',
                    style: getEdgeStyle(edge.relation),
                    markerEnd: {
                        type: MarkerType.ArrowClosed,
                        color: getEdgeStyle(edge.relation).stroke,
                    },
                    label: edge.relation,
                    labelStyle: { fill: '#9ca3af', fontSize: 10 },
                    labelBgStyle: { fill: '#1f2937', fillOpacity: 0.8 },
                }));

                // Apply layout
                const { nodes: layoutedNodes, edges: layoutedEdges } = getLayoutedElements(flowNodes, flowEdges);

                if (cancelled) return;

                setNodes(layoutedNodes);
                setEdges(layoutedEdges);
                setLoading(false);
            } catch (err: any) {
                if (cancelled) return;
                Logger.error('Failed to load dependencies', err, 'k8s');
                setError(err.message || 'Failed to load dependencies');
                setLoading(false);
            }
        };

        loadDependencies();

        return () => {
            cancelled = true;
        };
    }, [resourceType, namespace, resourceName]);

    if (loading) {
        return (
            <div className="flex items-center justify-center h-full bg-background">
                <div className="text-gray-400">Loading dependencies...</div>
            </div>
        );
    }

    if (error) {
        return (
            <div className="flex items-center justify-center h-full bg-background">
                <div className="text-red-400">{error}</div>
            </div>
        );
    }

    // Generate stable key for ReactFlow to force clean remount on resource change
    const flowKey = `${resourceType}-${namespace || ''}-${resourceName}`;

    return (
        <div className="h-full w-full bg-background relative">
            <ReactFlow
                key={flowKey}
                nodes={nodes}
                edges={edges}
                onNodesChange={onNodesChange}
                onEdgesChange={onEdgesChange}
                nodeTypes={nodeTypes}
                fitView
                fitViewOptions={{ padding: 0.2 }}
                minZoom={0.1}
                maxZoom={2}
                // Performance optimizations
                panOnScroll={true}
                nodesConnectable={false}
                elevateEdgesOnSelect={false}
            >
                <Controls className="!bg-surface-light !border-border" />
                <Background color="#3d3d3d" gap={16} />
            </ReactFlow>

            {/* Legend */}
            <div className="absolute bottom-4 left-4 bg-surface-light border border-border rounded-lg p-3 text-xs">
                <div className="text-gray-400 font-medium mb-2">Legend</div>
                <div className="grid grid-cols-2 gap-x-4 gap-y-1">
                    <div className="flex items-center gap-2">
                        <div className="w-4 h-0.5 bg-green-500"></div>
                        <span className="text-gray-300">owns</span>
                    </div>
                    <div className="flex items-center gap-2">
                        <div className="w-4 h-0.5 bg-blue-500" style={{ borderTop: '2px dashed' }}></div>
                        <span className="text-gray-300">uses</span>
                    </div>
                    <div className="flex items-center gap-2">
                        <div className="w-4 h-0.5 bg-purple-500"></div>
                        <span className="text-gray-300">binds</span>
                    </div>
                    <div className="flex items-center gap-2">
                        <div className="w-4 h-0.5 bg-teal-500" style={{ borderTop: '2px dashed' }}></div>
                        <span className="text-gray-300">selects</span>
                    </div>
                    <div className="flex items-center gap-2">
                        <div className="w-4 h-0.5 bg-pink-400"></div>
                        <span className="text-gray-300">routes-to</span>
                    </div>
                    <div className="flex items-center gap-2">
                        <div className="w-4 h-0.5 bg-cyan-400" style={{ borderTop: '2px dashed' }}></div>
                        <span className="text-gray-300">references</span>
                    </div>
                    <div className="flex items-center gap-2">
                        <div className="w-4 h-0.5 bg-orange-400" style={{ borderTop: '2px dashed' }}></div>
                        <span className="text-gray-300">applies-to</span>
                    </div>
                    <div className="flex items-center gap-2">
                        <div className="w-4 h-0.5 bg-teal-400"></div>
                        <span className="text-gray-300">scales</span>
                    </div>
                    <div className="flex items-center gap-2">
                        <div className="w-4 h-0.5 bg-amber-400" style={{ borderTop: '2px dashed' }}></div>
                        <span className="text-gray-300">protects</span>
                    </div>
                </div>
            </div>

            {/* Title */}
            <div className="absolute top-4 left-4 bg-surface-light border border-border rounded-lg px-3 py-2">
                <span className="text-gray-400 text-sm">
                    Dependencies for <span className="text-text font-medium">{resourceName}</span>
                </span>
            </div>

            {/* Context Menu */}
            {contextMenu && (
                <div
                    className="fixed bg-surface-light border border-border rounded-md shadow-lg z-50 py-1"
                    style={{ top: contextMenu.y, left: contextMenu.x }}
                    onClick={(e) => e.stopPropagation()}
                >
                    {contextMenu.node.isSummary ? (
                        <button
                            onClick={() => handleExpandNode(contextMenu.node)}
                            className="w-full text-left px-4 py-2 text-sm text-gray-300 hover:bg-surface-hover"
                        >
                            Expand ({contextMenu.node.remainingCount} more)
                        </button>
                    ) : (
                        <>
                            <button
                                onClick={() => handleEditYaml(contextMenu.node)}
                                className="w-full text-left px-4 py-2 text-sm text-gray-300 hover:bg-surface-hover"
                            >
                                Edit YAML
                            </button>
                            {DEPENDENCY_SUPPORTED_KINDS.has(contextMenu.node.kind) && (
                                <button
                                    onClick={() => handleShowDependencies(contextMenu.node)}
                                    className="w-full text-left px-4 py-2 text-sm text-gray-300 hover:bg-surface-hover"
                                >
                                    Show Dependencies
                                </button>
                            )}
                            <button
                                onClick={() => handleCompareResource(contextMenu.node)}
                                className="w-full text-left px-4 py-2 text-sm text-gray-300 hover:bg-surface-hover"
                            >
                                Compare to...
                            </button>
                            <button
                                onClick={() => handleGoToResource(contextMenu.node)}
                                className="w-full text-left px-4 py-2 text-sm text-gray-300 hover:bg-surface-hover"
                            >
                                Go to Resource
                            </button>
                        </>
                    )}
                </div>
            )}
        </div>
    );
}
