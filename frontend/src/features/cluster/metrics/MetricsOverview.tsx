import React, { useState, useMemo } from 'react';
import {
    ChartBarIcon,
    ServerIcon,
    CubeIcon,
    ArrowRightIcon,
    ExclamationTriangleIcon,
    ArrowPathIcon,
} from '@heroicons/react/24/outline';
import AggregateResourceBar from '~/components/shared/AggregateResourceBar';
import SourceSelect, { sourceOptions } from '~/components/shared/SourceSelect';
import { useClusterMetrics } from '~/hooks/useClusterMetrics';
import { useUI } from '~/context';
import { useConfig } from '~/context';
import { formatBytes, formatCpu } from '~/utils/formatting';

// Tooltip text for over-committed resources
const OVERCOMMIT_TOOLTIP = "Over-committed: Some containers are using more CPU/memory than their requests. This works because other pods aren't using their full reservations. Under contention, pods would be throttled to their guaranteed amounts.";

// Percentage display with tooltip for over-committed values (>100%)
const PercentValue = ({ value, className = "" }: any) => {
    const isOverCommitted = value > 100;
    return (
        <span
            className={`${className} ${isOverCommitted ? 'cursor-help' : ''}`}
            title={isOverCommitted ? OVERCOMMIT_TOOLTIP : undefined}
        >
            {value}%{isOverCommitted && <span className="text-yellow-500 ml-0.5">*</span>}
        </span>
    );
};

// Metric row in the summary table
const MetricRow = ({ label, usage, reserved, committed, capacity, formatFn, usagePercent, reservedPercent, committedPercent }: any) => (
    <tr className="border-b border-border/50 last:border-0">
        <td className="py-3 text-sm font-medium text-text">{label}</td>
        <td className="py-3 text-sm text-right text-blue-400">{formatFn(usage)}</td>
        <td className="py-3 text-sm text-right text-yellow-400">{reserved != null ? formatFn(reserved) : '-'}</td>
        <td className="py-3 text-sm text-right text-red-400">{committed != null ? formatFn(committed) : '-'}</td>
        <td className="py-3 text-sm text-right text-gray-400">{formatFn(capacity)}</td>
        <td className="py-3 pl-4 w-48">
            <AggregateResourceBar
                usagePercent={usagePercent}
                reservedPercent={reservedPercent}
                committedPercent={committedPercent}
                showPercent={false}
                barClassName="w-full h-4"
            />
        </td>
    </tr>
);

// Cluster summary card with comprehensive stats
const ClusterSummaryCard = ({ clusterTotals, loading }: any) => {
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
                                <PercentValue value={clusterTotals.cpuCommittedPercent} />
                            </div>
                        </div>
                        <div className="bg-background rounded-lg p-4">
                            <div className="flex items-center gap-2 text-gray-400 text-xs mb-1">
                                <ChartBarIcon className="h-4 w-4" />
                                Memory Committed
                            </div>
                            <div className="text-2xl font-semibold text-text">
                                <PercentValue value={clusterTotals.memCommittedPercent} />
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
                                    committedPercent={clusterTotals.cpuCommittedPercent}
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
                                    committedPercent={clusterTotals.memCommittedPercent}
                                />
                                <MetricRow
                                    label="Pods"
                                    usage={clusterTotals.podCount}
                                    capacity={clusterTotals.podCapacity}
                                    formatFn={(v: any) => v}
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
    { key: 'name', label: 'Node', align: 'left', getValue: (n: any) => n.name },
    { key: 'cpuPercent', label: 'CPU Used', align: 'right', color: 'text-blue-400', getValue: (n: any) => n.cpuPercent },
    { key: 'cpuReservedPercent', label: 'CPU Rsv', align: 'right', color: 'text-yellow-400', getValue: (n: any) => n.cpuReservedPercent },
    { key: 'cpuCommittedPercent', label: 'CPU Comm', align: 'right', color: 'text-red-400', getValue: (n: any) => n.cpuCommittedPercent },
    { key: 'memPercent', label: 'Mem Used', align: 'right', color: 'text-blue-400', getValue: (n: any) => n.memPercent },
    { key: 'memReservedPercent', label: 'Mem Rsv', align: 'right', color: 'text-yellow-400', getValue: (n: any) => n.memReservedPercent },
    { key: 'memCommittedPercent', label: 'Mem Comm', align: 'right', color: 'text-red-400', getValue: (n: any) => n.memCommittedPercent },
    { key: 'podPercent', label: 'Pods', align: 'right', color: 'text-gray-300', getValue: (n: any) => n.podPercent },
];

// Nodes table
const NodesTable = ({ nodes, onNodeClick, onViewAll }: any) => {
    const [sortConfig, setSortConfig] = useState({ key: 'cpuCommittedPercent', direction: 'desc' });

    const handleSort = (key: any) => {
        setSortConfig(prev => ({
            key,
            direction: prev.key === key && prev.direction === 'desc' ? 'asc' : 'desc'
        }));
    };

    const sortedNodes = useMemo(() => {
        const column = nodeColumns.find((c: any) => c.key === sortConfig.key);
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
                                {nodeColumns.map((col: any) => (
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
                            {sortedNodes.map((node: any) => (
                                <tr
                                    key={node.name}
                                    onClick={() => onNodeClick(node.name)}
                                    className="border-t border-border/50 hover:bg-white/5 cursor-pointer transition-colors"
                                >
                                    {nodeColumns.map((col: any) => {
                                        const value = col.getValue(node);
                                        const isCommitted = col.key === 'cpuCommittedPercent' || col.key === 'memCommittedPercent';
                                        return (
                                            <td
                                                key={col.key}
                                                className={`py-2 ${col.align === 'right' ? 'text-right' : ''} ${col.color || 'text-text'} ${col.key === 'name' ? 'truncate max-w-[180px]' : ''}`}
                                            >
                                                {col.key === 'name' ? node.name : (
                                                    isCommitted ? <PercentValue value={value} /> : `${value}%`
                                                )}
                                            </td>
                                        );
                                    })}
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

export default function MetricsOverview({ isVisible }: { isVisible: boolean }) {
    const { navigateWithSearch, setActiveView } = useUI();
    const { getConfig, setConfig } = useConfig();
    const {
        nodeMetrics,
        clusterTotals,
        available,
        loading,
        source,
        refresh
    } = useClusterMetrics(isVisible);

    // Get preferred source from config
    const preferredSource = getConfig('metrics.preferredSource') ?? 'auto';

    // Handle source change - useEffect in useNodeMetrics will auto-refresh when config changes
    const handleSourceChange = (newValue: any) => {
        setConfig('metrics.preferredSource', newValue);
    };

    // Navigation handlers
    const handleNodeClick = (nodeName: any) => {
        navigateWithSearch('nodes', `name:"${nodeName}"`);
    };

    const handleViewAllNodes = () => setActiveView('nodes');

    // Format source name for display
    const sourceLabel = source === 'prometheus' ? 'Prometheus' : 'K8s Metrics API';

    return (
        <div className="flex flex-col h-full bg-background">
            {/* Header */}
            <div className="h-14 border-b border-border flex items-center justify-between px-4 bg-surface shrink-0 titlebar-drag">
                <div className="flex items-center gap-3">
                    <ChartBarIcon className="h-6 w-6 text-primary" />
                    <h1 className="text-lg font-semibold text-text">Metrics Overview</h1>
                </div>
                <div className="flex items-center gap-3 text-sm text-gray-400">
                    {/* Source selector */}
                    <div className="flex items-center gap-2">
                        <span className="text-xs text-gray-500">Source:</span>
                        <SourceSelect
                            value={preferredSource}
                            onChange={handleSourceChange}
                            options={sourceOptions}
                        />
                    </div>

                    {/* Status indicator */}
                    {available === true && (
                        <div className="flex items-center gap-1.5" title={`Using ${sourceLabel}`}>
                            <span className="text-xs text-gray-500">({sourceLabel})</span>
                            <span className="h-2 w-2 rounded-full bg-green-500"></span>
                        </div>
                    )}
                    {available === false && (
                        <div className="flex items-center gap-1.5" title="Metrics unavailable">
                            <span className="text-xs text-gray-500">(unavailable)</span>
                            <span className="h-2 w-2 rounded-full bg-yellow-500"></span>
                        </div>
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
