import React, { useState, useEffect, useCallback, useMemo } from 'react';
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
import { GetResourceDependencies, ExpandDependencyNode } from '../../../wailsjs/go/main/App';
import { useUI } from '../../context/UIContext';
import { LazyYamlEditor as YamlEditor } from '../lazy';
import Logger from '../../utils/Logger';
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

// Custom node component
function ResourceNode({ data }) {
    const style = resourceStyles[data.kind] || { icon: CubeIcon, color: '#6b7280', bgColor: '#6b728020' };
    const Icon = data.isSummary ? EllipsisHorizontalIcon : style.icon;

    const getStatusColor = (status) => {
        if (!status) return 'text-gray-400';
        const s = status.toLowerCase();
        if (s === 'running' || s === 'bound' || s === 'available' || s === 'active') return 'text-green-400';
        if (s === 'pending' || s === 'released') return 'text-yellow-400';
        if (s === 'failed' || s === 'lost' || s === 'terminated') return 'text-red-400';
        return 'text-gray-400';
    };

    // Summary nodes have a dashed border and different styling
    if (data.isSummary) {
        return (
            <div
                className="px-3 py-2 rounded-lg border-2 border-dashed min-w-[140px] cursor-pointer transition-all hover:scale-105"
                style={{
                    backgroundColor: '#374151',
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
        <div
            className="px-3 py-2 rounded-lg border-2 min-w-[140px] cursor-pointer transition-all hover:scale-105"
            style={{
                backgroundColor: style.bgColor,
                borderColor: style.color,
            }}
            onContextMenu={data.onContextMenu}
        >
            <Handle type="target" position={Position.Top} className="!bg-gray-500" />
            <div className="flex items-center gap-2">
                <Icon className="h-5 w-5" style={{ color: style.color }} />
                <div className="flex flex-col">
                    <span className="text-xs text-gray-400">{data.kind}</span>
                    <span className="text-sm font-medium text-white truncate max-w-[120px]" title={data.label}>
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
    );
}

const nodeTypes = {
    resource: ResourceNode,
};

// Layout the graph using dagre
function getLayoutedElements(nodes, edges) {
    const dagreGraph = new dagre.graphlib.Graph();
    dagreGraph.setDefaultEdgeLabel(() => ({}));
    dagreGraph.setGraph({ rankdir: 'TB', nodesep: 60, ranksep: 80 });

    nodes.forEach((node) => {
        dagreGraph.setNode(node.id, { width: 160, height: 70 });
    });

    edges.forEach((edge) => {
        dagreGraph.setEdge(edge.source, edge.target);
    });

    dagre.layout(dagreGraph);

    const layoutedNodes = nodes.map((node) => {
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
const getEdgeStyle = (relation) => {
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

export default function DependencyGraph({ resourceType, namespace, resourceName, onClose }) {
    const { openTab, closeTab, openDiagnostic } = useUI();
    const [nodes, setNodes, onNodesChange] = useNodesState([]);
    const [edges, setEdges, onEdgesChange] = useEdgesState([]);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState(null);
    const [contextMenu, setContextMenu] = useState(null);
    const [expansionOffsets, setExpansionOffsets] = useState({}); // Track offset per summary node

    // Map resource type to kind for context menu actions
    const getResourceTypeFromKind = (kind) => {
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
        return mapping[kind] || kind.toLowerCase();
    };

    const handleNodeContextMenu = useCallback((event, nodeData) => {
        event.preventDefault();
        setContextMenu({
            x: event.clientX,
            y: event.clientY,
            node: nodeData,
        });
    }, []);

    const handleEditYaml = useCallback((node) => {
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

    const handleShowDependencies = useCallback((node) => {
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

    const handleCompareResource = useCallback((node) => {
        const resType = getResourceTypeFromKind(node.kind);
        // Pre-fill both source and target with the same resource
        // User typically just needs to change the target context
        openDiagnostic('resource-diff', {
            initialSource: {
                kind: resType,
                namespace: node.namespace || 'default',
                name: node.label
            },
            initialTarget: {
                kind: resType,
                namespace: node.namespace || 'default',
                name: node.label
            }
        });
        setContextMenu(null);
    }, [openDiagnostic]);

    // Handle expanding a summary node
    const handleExpandNode = useCallback(async (summaryNode) => {
        setContextMenu(null);

        // Calculate the current offset (default 5 for initial expansion)
        const summaryId = `summary:${summaryNode.parentId}:${summaryNode.kind}`;
        const currentOffset = expansionOffsets[summaryId] || 5;

        try {
            Logger.info('Expanding summary node', { summaryId, offset: currentOffset });
            const expandedGraph = await ExpandDependencyNode(
                resourceType,
                namespace || '',
                resourceName,
                summaryId,
                currentOffset
            );

            if (!expandedGraph || !expandedGraph.nodes) {
                Logger.warn('No expanded nodes returned');
                return;
            }

            // Convert new nodes to flow nodes
            const newFlowNodes = expandedGraph.nodes.map((node) => ({
                id: node.id,
                type: 'resource',
                data: {
                    label: node.name,
                    kind: node.kind,
                    namespace: node.namespace,
                    status: node.status,
                    isSummary: node.isSummary || false,
                    remainingCount: node.remainingCount || 0,
                    parentId: node.parentId || '',
                    onContextMenu: (e) => handleNodeContextMenu(e, {
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
            const newFlowEdges = expandedGraph.edges.map((edge, idx) => ({
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

            // Merge new nodes and edges, then re-layout the entire graph
            setNodes((currentNodes) => {
                // Filter out the old summary node
                const filteredNodes = currentNodes.filter(n => n.id !== summaryId);
                // Add new nodes (avoiding duplicates)
                const existingIds = new Set(filteredNodes.map(n => n.id));
                const uniqueNewNodes = newFlowNodes.filter(n => !existingIds.has(n.id));
                const allNodes = [...filteredNodes, ...uniqueNewNodes];

                // Get current edges to include in layout
                setEdges((currentEdges) => {
                    // Remove edges pointing to/from old summary node
                    const filteredEdges = currentEdges.filter(e => e.target !== summaryId && e.source !== summaryId);
                    // Add new edges (avoiding duplicates)
                    const existingEdgeIds = new Set(filteredEdges.map(e => `${e.source}-${e.target}`));
                    const uniqueNewEdges = newFlowEdges.filter(e => !existingEdgeIds.has(`${e.source}-${e.target}`));
                    const allEdges = [...filteredEdges, ...uniqueNewEdges];

                    // Re-layout the graph with dagre
                    const { nodes: layoutedNodes, edges: layoutedEdges } = getLayoutedElements(allNodes, allEdges);

                    // Update nodes with new positions (need to do this via setTimeout to avoid setState in setState)
                    setTimeout(() => {
                        setNodes(layoutedNodes);
                    }, 0);

                    return layoutedEdges;
                });

                return allNodes;
            });

            // Update offset for potential further expansion
            setExpansionOffsets(prev => ({
                ...prev,
                [summaryId]: currentOffset + 10,
            }));

        } catch (err) {
            Logger.error('Failed to expand summary node', err);
        }
    }, [resourceType, namespace, resourceName, expansionOffsets, handleNodeContextMenu]);

    // Close context menu when clicking elsewhere
    useEffect(() => {
        const handleClick = () => setContextMenu(null);
        document.addEventListener('click', handleClick);
        return () => document.removeEventListener('click', handleClick);
    }, []);

    // Load dependency data
    useEffect(() => {
        const loadDependencies = async () => {
            setLoading(true);
            setError(null);

            try {
                Logger.info('Loading dependencies', { resourceType, namespace, resourceName });
                const graph = await GetResourceDependencies(resourceType, namespace || '', resourceName);

                if (!graph || !graph.nodes || graph.nodes.length === 0) {
                    setError('No dependencies found for this resource');
                    setLoading(false);
                    return;
                }

                // Convert backend nodes to React Flow nodes
                const flowNodes = graph.nodes.map((node) => ({
                    id: node.id,
                    type: 'resource',
                    data: {
                        label: node.name,
                        kind: node.kind,
                        namespace: node.namespace,
                        status: node.status,
                        isSummary: node.isSummary || false,
                        remainingCount: node.remainingCount || 0,
                        parentId: node.parentId || '',
                        onContextMenu: (e) => handleNodeContextMenu(e, {
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
                const flowEdges = graph.edges.map((edge, idx) => ({
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

                setNodes(layoutedNodes);
                setEdges(layoutedEdges);
                setLoading(false);
            } catch (err) {
                Logger.error('Failed to load dependencies', err);
                setError(err.message || 'Failed to load dependencies');
                setLoading(false);
            }
        };

        loadDependencies();
    }, [resourceType, namespace, resourceName, handleNodeContextMenu]);

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

    return (
        <div className="h-full w-full bg-background relative">
            <ReactFlow
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
                    Dependencies for <span className="text-white font-medium">{resourceName}</span>
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
                        </>
                    )}
                </div>
            )}
        </div>
    );
}
