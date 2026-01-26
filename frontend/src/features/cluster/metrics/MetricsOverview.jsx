import React, { useState, useMemo } from 'react';
import {
    ChartBarIcon,
    ServerIcon,
    CubeIcon,
    ArrowRightIcon,
    ExclamationTriangleIcon,
    ArrowPathIcon
} from '@heroicons/react/24/outline';
import { useClusterMetrics } from '../../../hooks/useClusterMetrics';
import { useUI } from '../../../context/UIContext';
import { formatBytes } from '../../../utils/formatting';

// Format CPU millicores for display
const formatCpu = (millicores) => {
    if (millicores == null || millicores === 0) return '0m';
    if (millicores >= 1000) {
        return `${(millicores / 1000).toFixed(1)} cores`;
    }
    return `${Math.round(millicores)}m`;
};

// Stacked bar showing usage and reserved-excess as distinct segments
const ResourceBar = ({ usagePercent, reservedPercent }) => {
    // Reserved excess is the portion of reserved that exceeds usage
    const reservedExcess = Math.max(0, reservedPercent - usagePercent);

    return (
        <div className="relative h-4 bg-gray-700 rounded overflow-hidden flex">
            {/* Usage (blue) */}
            <div
                className="h-full bg-primary shrink-0"
                style={{ width: `${Math.min(100, usagePercent)}%` }}
            />
            {/* Reserved excess (yellow) - only shows if reserved > usage */}
            {reservedExcess > 0 && (
                <div
                    className="h-full bg-yellow-500 shrink-0"
                    style={{ width: `${Math.min(100 - usagePercent, reservedExcess)}%` }}
                />
            )}
            {/* Committed marker (red line) at the committed boundary */}
            {(usagePercent > 0 || reservedPercent > 0) && (
                <div
                    className="absolute top-0 bottom-0 w-0.5 bg-red-500"
                    style={{ left: `${Math.min(100, Math.max(usagePercent, reservedPercent))}%` }}
                />
            )}
        </div>
    );
};

// Metric row in the summary table
const MetricRow = ({ label, usage, reserved, committed, capacity, formatFn, usagePercent, reservedPercent }) => (
    <tr className="border-b border-border/50 last:border-0">
        <td className="py-3 text-sm font-medium text-text">{label}</td>
        <td className="py-3 text-sm text-right text-blue-400">{formatFn(usage)}</td>
        <td className="py-3 text-sm text-right text-yellow-400">{reserved != null ? formatFn(reserved) : '-'}</td>
        <td className="py-3 text-sm text-right text-red-400">{committed != null ? formatFn(committed) : '-'}</td>
        <td className="py-3 text-sm text-right text-gray-400">{formatFn(capacity)}</td>
        <td className="py-3 pl-4 w-48">
            <ResourceBar
                usagePercent={usagePercent}
                reservedPercent={reservedPercent ?? 0}
            />
        </td>
    </tr>
);

// Cluster summary card with comprehensive stats
const ClusterSummaryCard = ({ clusterTotals, loading }) => {
    return (
        <div className="bg-surface border border-border rounded-lg p-6">
            <h2 className="text-sm font-semibold text-gray-400 uppercase tracking-wider mb-4">
                Cluster Summary
            </h2>
            {loading ? (
                <div className="flex items-center gap-2 text-gray-400">
                    <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-primary"></div>
                    Loading metrics...
                </div>
            ) : (
                <>
                    {/* Quick stats */}
                    <div className="grid grid-cols-4 gap-4 mb-6">
                        <div className="bg-background rounded-lg p-4">
                            <div className="flex items-center gap-2 text-gray-400 text-xs mb-1">
                                <ServerIcon className="h-4 w-4" />
                                Nodes
                            </div>
                            <div className="text-2xl font-semibold text-text">
                                {clusterTotals.nodeCount}
                            </div>
                        </div>
                        <div className="bg-background rounded-lg p-4">
                            <div className="flex items-center gap-2 text-gray-400 text-xs mb-1">
                                <CubeIcon className="h-4 w-4" />
                                Pods
                            </div>
                            <div className="text-2xl font-semibold text-text">
                                {clusterTotals.podPercent}%
                            </div>
                            <div className="text-xs text-gray-500 mt-1">
                                {clusterTotals.podCount} / {clusterTotals.podCapacity}
                            </div>
                        </div>
                        <div className="bg-background rounded-lg p-4">
                            <div className="flex items-center gap-2 text-gray-400 text-xs mb-1">
                                <ChartBarIcon className="h-4 w-4" />
                                CPU Committed
                            </div>
                            <div className="text-2xl font-semibold text-text">
                                {clusterTotals.cpuCommittedPercent}%
                            </div>
                        </div>
                        <div className="bg-background rounded-lg p-4">
                            <div className="flex items-center gap-2 text-gray-400 text-xs mb-1">
                                <ChartBarIcon className="h-4 w-4" />
                                Memory Committed
                            </div>
                            <div className="text-2xl font-semibold text-text">
                                {clusterTotals.memCommittedPercent}%
                            </div>
                        </div>
                    </div>

                    {/* Detailed breakdown table */}
                    <div className="overflow-x-auto">
                        <table className="w-full">
                            <thead>
                                <tr className="text-xs text-gray-500 uppercase tracking-wider">
                                    <th className="text-left pb-2 font-medium">Resource</th>
                                    <th className="text-right pb-2 font-medium text-blue-400">Usage</th>
                                    <th className="text-right pb-2 font-medium text-yellow-400">Reserved</th>
                                    <th className="text-right pb-2 font-medium text-red-400">Committed</th>
                                    <th className="text-right pb-2 font-medium">Capacity</th>
                                    <th className="text-left pb-2 pl-4 font-medium">Distribution</th>
                                </tr>
                            </thead>
                            <tbody>
                                <MetricRow
                                    label="CPU"
                                    usage={clusterTotals.cpuUsage}
                                    reserved={clusterTotals.cpuReserved}
                                    committed={clusterTotals.cpuCommitted}
                                    capacity={clusterTotals.cpuCapacity}
                                    formatFn={formatCpu}
                                    usagePercent={clusterTotals.cpuUsagePercent}
                                    reservedPercent={clusterTotals.cpuReservedPercent}
                                />
                                <MetricRow
                                    label="Memory"
                                    usage={clusterTotals.memUsage}
                                    reserved={clusterTotals.memReserved}
                                    committed={clusterTotals.memCommitted}
                                    capacity={clusterTotals.memCapacity}
                                    formatFn={formatBytes}
                                    usagePercent={clusterTotals.memUsagePercent}
                                    reservedPercent={clusterTotals.memReservedPercent}
                                />
                                <MetricRow
                                    label="Pods"
                                    usage={clusterTotals.podCount}
                                    capacity={clusterTotals.podCapacity}
                                    formatFn={(v) => v}
                                    usagePercent={clusterTotals.podPercent}
                                />
                            </tbody>
                        </table>
                    </div>

                    {/* Legend */}
                    <div className="flex items-center gap-6 mt-4 text-xs text-gray-500">
                        <span className="flex items-center gap-1.5">
                            <span className="w-3 h-3 bg-primary rounded"></span>
                            Usage (actual)
                        </span>
                        <span className="flex items-center gap-1.5">
                            <span className="w-3 h-3 bg-yellow-500 rounded"></span>
                            Reserved excess
                        </span>
                        <span className="flex items-center gap-1.5">
                            <span className="w-3 h-3 bg-red-500 rounded"></span>
                            Committed
                        </span>
                    </div>
                </>
            )}
        </div>
    );
};

// Column definitions for nodes table
const nodeColumns = [
    { key: 'name', label: 'Node', align: 'left', getValue: (n) => n.name },
    { key: 'cpuPercent', label: 'CPU Used', align: 'right', color: 'text-blue-400', getValue: (n) => n.cpuPercent },
    { key: 'cpuReservedPercent', label: 'CPU Rsv', align: 'right', color: 'text-yellow-400', getValue: (n) => n.cpuReservedPercent },
    { key: 'cpuCommittedPercent', label: 'CPU Comm', align: 'right', color: 'text-red-400', getValue: (n) => n.cpuCommittedPercent },
    { key: 'memPercent', label: 'Mem Used', align: 'right', color: 'text-blue-400', getValue: (n) => n.memPercent },
    { key: 'memReservedPercent', label: 'Mem Rsv', align: 'right', color: 'text-yellow-400', getValue: (n) => n.memReservedPercent },
    { key: 'memCommittedPercent', label: 'Mem Comm', align: 'right', color: 'text-red-400', getValue: (n) => n.memCommittedPercent },
    { key: 'podPercent', label: 'Pods', align: 'right', color: 'text-gray-300', getValue: (n) => n.podPercent },
];

// Nodes table
const NodesTable = ({ nodes, onNodeClick, onViewAll }) => {
    const [sortConfig, setSortConfig] = useState({ key: 'cpuCommittedPercent', direction: 'desc' });

    const handleSort = (key) => {
        setSortConfig(prev => ({
            key,
            direction: prev.key === key && prev.direction === 'desc' ? 'asc' : 'desc'
        }));
    };

    const sortedNodes = useMemo(() => {
        const column = nodeColumns.find(c => c.key === sortConfig.key);
        if (!column) return nodes;

        return [...nodes].sort((a, b) => {
            const aVal = column.getValue(a);
            const bVal = column.getValue(b);

            // String comparison for name
            if (sortConfig.key === 'name') {
                const cmp = String(aVal).localeCompare(String(bVal));
                return sortConfig.direction === 'asc' ? cmp : -cmp;
            }

            // Numeric comparison for percentages
            if (aVal < bVal) return sortConfig.direction === 'asc' ? -1 : 1;
            if (aVal > bVal) return sortConfig.direction === 'asc' ? 1 : -1;
            return 0;
        });
    }, [nodes, sortConfig]);

    return (
        <div className="bg-surface border border-border rounded-lg p-6">
            <div className="flex items-center justify-between mb-4">
                <h2 className="text-sm font-semibold text-gray-400 uppercase tracking-wider flex items-center gap-2">
                    <ServerIcon className="h-4 w-4" />
                    Nodes
                </h2>
                <button
                    onClick={onViewAll}
                    className="flex items-center gap-1 text-xs text-primary hover:text-primary/80 transition-colors"
                >
                    View All
                    <ArrowRightIcon className="h-3 w-3" />
                </button>
            </div>

            {nodes.length === 0 ? (
                <div className="text-sm text-gray-500">No node metrics available</div>
            ) : (
                <div className="overflow-x-auto">
                    <table className="w-full text-sm">
                        <thead>
                            <tr className="text-xs text-gray-500 uppercase tracking-wider">
                                {nodeColumns.map(col => (
                                    <th
                                        key={col.key}
                                        className={`pb-2 font-medium cursor-pointer hover:text-gray-300 transition-colors ${col.align === 'right' ? 'text-right' : 'text-left'}`}
                                        onClick={() => handleSort(col.key)}
                                    >
                                        <div className={`flex items-center gap-1 ${col.align === 'right' ? 'justify-end' : ''}`}>
                                            {col.label}
                                            {sortConfig.key === col.key && (
                                                <span className="text-primary">
                                                    {sortConfig.direction === 'asc' ? '↑' : '↓'}
                                                </span>
                                            )}
                                        </div>
                                    </th>
                                ))}
                                <th className="w-6"></th>
                            </tr>
                        </thead>
                        <tbody>
                            {sortedNodes.map((node) => (
                                <tr
                                    key={node.name}
                                    onClick={() => onNodeClick(node.name)}
                                    className="border-t border-border/50 hover:bg-white/5 cursor-pointer transition-colors"
                                >
                                    {nodeColumns.map(col => (
                                        <td
                                            key={col.key}
                                            className={`py-2 ${col.align === 'right' ? 'text-right' : ''} ${col.color || 'text-text'} ${col.key === 'name' ? 'truncate max-w-[180px]' : ''}`}
                                        >
                                            {col.key === 'name' ? node.name : `${col.getValue(node)}%`}
                                        </td>
                                    ))}
                                    <td className="py-2">
                                        <ArrowRightIcon className="h-3 w-3 text-gray-500" />
                                    </td>
                                </tr>
                            ))}
                        </tbody>
                    </table>
                </div>
            )}
        </div>
    );
};

// Message shown when metrics-server is not available
const MetricsUnavailableMessage = () => (
    <div className="bg-surface border border-border rounded-lg p-6">
        <div className="flex items-start gap-4">
            <div className="p-2 bg-yellow-500/20 rounded-lg">
                <ExclamationTriangleIcon className="h-6 w-6 text-yellow-400" />
            </div>
            <div className="flex-1">
                <h3 className="text-lg font-medium text-text mb-2">Metrics Server Not Available</h3>
                <p className="text-sm text-gray-400 mb-4">
                    The Kubernetes Metrics Server is required to display resource usage data.
                    Install it to see CPU and memory metrics for your cluster.
                </p>
                <div className="bg-background rounded p-3 text-sm font-mono text-gray-300">
                    kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml
                </div>
                <p className="text-xs text-gray-500 mt-3">
                    Note: For local clusters like minikube or Docker Desktop, the metrics server may
                    need additional configuration. Check your cluster documentation.
                </p>
            </div>
        </div>
    </div>
);

export default function MetricsOverview({ isVisible }) {
    const { navigateWithSearch, setActiveView } = useUI();
    const {
        nodeMetrics,
        clusterTotals,
        available,
        loading,
        refresh
    } = useClusterMetrics(isVisible);

    // Navigation handlers
    const handleNodeClick = (nodeName) => {
        navigateWithSearch('nodes', `name:"${nodeName}"`);
    };

    const handleViewAllNodes = () => setActiveView('nodes');

    return (
        <div className="flex flex-col h-full bg-background">
            {/* Header */}
            <div className="h-14 border-b border-border flex items-center justify-between px-4 bg-surface shrink-0 titlebar-drag">
                <div className="flex items-center gap-3">
                    <ChartBarIcon className="h-6 w-6 text-primary" />
                    <h1 className="text-lg font-semibold text-text">Metrics Overview</h1>
                </div>
                <div className="flex items-center gap-3 text-sm text-gray-400">
                    <span>Source: K8s Metrics API</span>
                    {available === true && (
                        <span className="h-2 w-2 rounded-full bg-green-500" title="Metrics available"></span>
                    )}
                    {available === false && (
                        <span className="h-2 w-2 rounded-full bg-yellow-500" title="Metrics unavailable"></span>
                    )}
                    <button
                        onClick={refresh}
                        disabled={loading}
                        className="p-1.5 rounded hover:bg-white/10 transition-colors disabled:opacity-50"
                        title="Refresh metrics"
                    >
                        <ArrowPathIcon className={`h-4 w-4 ${loading ? 'animate-spin' : ''}`} />
                    </button>
                </div>
            </div>

            {/* Content */}
            <div className="flex-1 overflow-auto p-6">
                <div className="max-w-5xl mx-auto space-y-6">
                    {/* Not yet checked - show initial loading */}
                    {available === null && (
                        <div className="bg-surface border border-border rounded-lg p-6">
                            <div className="flex items-center gap-3 text-gray-400">
                                <div className="animate-spin rounded-full h-5 w-5 border-b-2 border-primary"></div>
                                Checking metrics availability...
                            </div>
                        </div>
                    )}

                    {/* Metrics unavailable */}
                    {available === false && (
                        <MetricsUnavailableMessage />
                    )}

                    {/* Metrics available - show dashboard */}
                    {available === true && (
                        <>
                            {/* Cluster Summary */}
                            <ClusterSummaryCard clusterTotals={clusterTotals} loading={loading} />

                            {/* Nodes Table */}
                            <NodesTable
                                nodes={nodeMetrics}
                                onNodeClick={handleNodeClick}
                                onViewAll={handleViewAllNodes}
                            />
                        </>
                    )}
                </div>
            </div>
        </div>
    );
}
