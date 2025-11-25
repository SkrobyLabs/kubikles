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
import { GetResourceDependencies } from '../../../wailsjs/go/main/App';
import { useUI } from '../../context/UIContext';
import YamlEditor from './YamlEditor';
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
} from '@heroicons/react/24/outline';

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
};

// Custom node component
function ResourceNode({ data }) {
    const style = resourceStyles[data.kind] || { icon: CubeIcon, color: '#6b7280', bgColor: '#6b728020' };
    const Icon = style.icon;

    const getStatusColor = (status) => {
        if (!status) return 'text-gray-400';
        const s = status.toLowerCase();
        if (s === 'running' || s === 'bound' || s === 'available' || s === 'active') return 'text-green-400';
        if (s === 'pending' || s === 'released') return 'text-yellow-400';
        if (s === 'failed' || s === 'lost' || s === 'terminated') return 'text-red-400';
        return 'text-gray-400';
    };

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
        default:
            return { stroke: '#6b7280', strokeWidth: 1 };
    }
};

export default function DependencyGraph({ resourceType, namespace, resourceName, onClose }) {
    const { openTab, closeTab } = useUI();
    const [nodes, setNodes, onNodesChange] = useNodesState([]);
    const [edges, setEdges, onEdgesChange] = useEdgesState([]);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState(null);
    const [contextMenu, setContextMenu] = useState(null);

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
            title: `Edit: ${node.label}`,
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
            title: `Deps: ${node.label}`,
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
                        onContextMenu: (e) => handleNodeContextMenu(e, {
                            label: node.name,
                            kind: node.kind,
                            namespace: node.namespace,
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
            <div className="flex items-center justify-center h-full bg-[#1e1e1e]">
                <div className="text-gray-400">Loading dependencies...</div>
            </div>
        );
    }

    if (error) {
        return (
            <div className="flex items-center justify-center h-full bg-[#1e1e1e]">
                <div className="text-red-400">{error}</div>
            </div>
        );
    }

    return (
        <div className="h-full w-full bg-[#1e1e1e] relative">
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
            >
                <Controls className="!bg-[#2d2d2d] !border-[#3d3d3d]" />
                <Background color="#3d3d3d" gap={16} />
            </ReactFlow>

            {/* Legend */}
            <div className="absolute bottom-4 left-4 bg-[#2d2d2d] border border-[#3d3d3d] rounded-lg p-3 text-xs">
                <div className="text-gray-400 font-medium mb-2">Legend</div>
                <div className="space-y-1">
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
                </div>
            </div>

            {/* Title */}
            <div className="absolute top-4 left-4 bg-[#2d2d2d] border border-[#3d3d3d] rounded-lg px-3 py-2">
                <span className="text-gray-400 text-sm">
                    Dependencies for <span className="text-white font-medium">{resourceName}</span>
                </span>
            </div>

            {/* Context Menu */}
            {contextMenu && (
                <div
                    className="fixed bg-[#2d2d2d] border border-[#3d3d3d] rounded-md shadow-lg z-50 py-1"
                    style={{ top: contextMenu.y, left: contextMenu.x }}
                    onClick={(e) => e.stopPropagation()}
                >
                    <button
                        onClick={() => handleEditYaml(contextMenu.node)}
                        className="w-full text-left px-4 py-2 text-sm text-gray-300 hover:bg-[#3d3d3d]"
                    >
                        Edit YAML
                    </button>
                    <button
                        onClick={() => handleShowDependencies(contextMenu.node)}
                        className="w-full text-left px-4 py-2 text-sm text-gray-300 hover:bg-[#3d3d3d]"
                    >
                        Show Dependencies
                    </button>
                </div>
            )}
        </div>
    );
}
