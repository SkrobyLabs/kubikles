import React, { useMemo, useState, useCallback, memo } from 'react';
import ResourceList from '../../../components/shared/ResourceList';
import AggregateResourceBar from '../../../components/shared/AggregateResourceBar';
import ResourceBar from '../../../components/shared/ResourceBar';
import { useNodes } from '../../../hooks/resources';
import { useNodeMetrics } from '../../../hooks/useNodeMetrics';
import { useK8s } from '../../../context';
import { useUI } from '../../../context';
import { useMenu } from '../../../context';
import { formatAge, formatBytes, formatCpu } from '../../../utils/formatting';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import NodeActionsMenu from './NodeActionsMenu';
import { useNodeActions } from './useNodeActions';
import { useMenuPosition } from '../../../hooks/useMenuPosition';

// Helper to get node conditions summary
const getConditionsSummary = (node) => {
    const conditions = node.status?.conditions || [];
    const isUnschedulable = node.spec?.unschedulable === true;

    const issues = [];

    // Check Ready condition
    const readyCondition = conditions.find(c => c.type === 'Ready');
    const isReady = readyCondition?.status === 'True';

    if (!isReady) {
        issues.push({ type: 'NotReady', severity: 'error' });
    }

    if (isUnschedulable) {
        issues.push({ type: 'SchedulingDisabled', severity: 'warning' });
    }

    // Check pressure conditions (these are problems when True)
    const pressureConditions = ['MemoryPressure', 'DiskPressure', 'PIDPressure'];
    pressureConditions.forEach(condType => {
        const cond = conditions.find(c => c.type === condType);
        if (cond?.status === 'True') {
            issues.push({ type: condType, severity: 'error' });
        }
    });

    // Check NetworkUnavailable (problem when True)
    const networkCond = conditions.find(c => c.type === 'NetworkUnavailable');
    if (networkCond?.status === 'True') {
        issues.push({ type: 'NetworkUnavailable', severity: 'error' });
    }

    return { isReady, isUnschedulable, issues };
};

// Conditions cell component
const ConditionsCell = memo(function ConditionsCell({ node }) {
    const { isReady, issues } = getConditionsSummary(node);

    if (issues.length === 0) {
        return <span className="text-green-400">Ready</span>;
    }

    return (
        <div className="flex flex-wrap gap-1">
            {issues.map((issue, idx) => (
                <span
                    key={idx}
                    className={`px-1.5 py-0.5 text-xs rounded ${
                        issue.severity === 'error'
                            ? 'bg-red-500/20 text-red-400'
                            : 'bg-yellow-500/20 text-yellow-400'
                    }`}
                >
                    {issue.type}
                </span>
            ))}
        </div>
    );
});

// Taints cell component with tooltip
const TaintsCell = memo(function TaintsCell({ node }) {
    const taints = node.spec?.taints || [];
    const count = taints.length;

    if (count === 0) {
        return <span className="text-gray-500">-</span>;
    }

    const tooltipLines = taints.map(t =>
        `${t.key}${t.value ? '=' + t.value : ''}:${t.effect}`
    );

    return (
        <span className="relative group cursor-help text-gray-300 underline decoration-dotted">
            {count}
            <div className="absolute z-50 invisible opacity-0 group-hover:visible group-hover:opacity-100 transition-opacity delay-500 bottom-full left-1/2 -translate-x-1/2 mb-1 px-2 py-1 text-xs bg-background border border-border rounded shadow-lg whitespace-pre text-left">
                {tooltipLines.map((line, idx) => (
                    <div key={idx}>{line}</div>
                ))}
            </div>
        </span>
    );
});

export default function NodeList({ isVisible }) {
    const { currentContext } = useK8s();
    const { activeMenuId, menuPosition, handleMenuOpenChange } = useMenuPosition();
    const { nodes, loading, refetch } = useNodes(currentContext, isVisible);
    // Delay metrics fetch until nodes are loaded to prioritize showing node list first
    const { metrics, available: metricsAvailable } = useNodeMetrics(isVisible, !loading && nodes.length > 0);
    const { handleShowDetails, handleEditYaml, handleCordonUncordon, handleShell, handleDelete } = useNodeActions(refetch);

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item) => item.metadata?.name, getValue: (item) => item.metadata?.name },
        {
            key: 'cpu',
            label: 'CPU',
            filterType: 'numeric',
            numericHint: 'CPU usage % (0-100)',
            numericUnit: '%',
            render: (item) => {
                const m = metrics[item.metadata?.name];
                if (metricsAvailable === false) return <span className="text-gray-500 italic text-xs">N/A</span>;
                if (!m) return <span className="text-gray-500 text-xs">--</span>;
                return (
                    <AggregateResourceBar
                        usagePercent={m.cpuPercent}
                        reservedPercent={m.cpuReservedPercent}
                        committedPercent={m.cpuCommittedPercent}
                        type="cpu"
                        label="CPU"
                        usageValue={m.cpuUsage}
                        reservedValue={m.cpuRequested}
                        committedValue={m.cpuCommitted}
                        capacityValue={m.cpuCapacity}
                        formatValue={formatCpu}
                    />
                );
            },
            getValue: (item) => metrics[item.metadata?.name]?.cpuCommittedPercent ?? -1,
            getNumericValue: (item) => metrics[item.metadata?.name]?.cpuPercent ?? NaN
        },
        {
            key: 'memory',
            label: 'Memory',
            filterType: 'numeric',
            numericHint: 'Memory usage % (0-100)',
            numericUnit: '%',
            render: (item) => {
                const m = metrics[item.metadata?.name];
                if (metricsAvailable === false) return <span className="text-gray-500 italic text-xs">N/A</span>;
                if (!m) return <span className="text-gray-500 text-xs">--</span>;
                return (
                    <AggregateResourceBar
                        usagePercent={m.memPercent}
                        reservedPercent={m.memReservedPercent}
                        committedPercent={m.memCommittedPercent}
                        type="memory"
                        label="Memory"
                        usageValue={m.memoryUsage}
                        reservedValue={m.memRequested}
                        committedValue={m.memCommitted}
                        capacityValue={m.memCapacity}
                        formatValue={formatBytes}
                    />
                );
            },
            getValue: (item) => metrics[item.metadata?.name]?.memCommittedPercent ?? -1,
            getNumericValue: (item) => metrics[item.metadata?.name]?.memPercent ?? NaN
        },
        {
            key: 'pods',
            label: 'Pods',
            render: (item) => {
                const m = metrics[item.metadata?.name];
                if (metricsAvailable === false) return <span className="text-gray-500 italic text-xs">N/A</span>;
                if (!m) return <span className="text-gray-500 text-xs">--</span>;
                return <ResourceBar percent={m.podPercent} label="" tooltipLabel={`${m.podCount}/${m.podCapacity}`} color="bg-green-500" />;
            },
            getValue: (item) => metrics[item.metadata?.name]?.podPercent ?? -1
        },
        {
            key: 'conditions',
            label: 'Conditions',
            render: (item) => <ConditionsCell node={item} />,
            getValue: (item) => {
                const { issues } = getConditionsSummary(item);
                return issues.length === 0 ? 'Ready' : issues.map(i => i.type).join(',');
            }
        },
        {
            key: 'taints',
            label: 'Taints',
            align: 'center',
            filterable: false,
            render: (item) => <TaintsCell node={item} />,
            getValue: (item) => (item.spec?.taints || []).length
        },
        { key: 'version', label: 'Version', render: (item) => item.status?.nodeInfo?.kubeletVersion, getValue: (item) => item.status?.nodeInfo?.kubeletVersion },
        // Hidden by default columns
        {
            key: 'roles',
            label: 'Roles',
            defaultHidden: true,
            render: (item) => {
                const labels = item.metadata?.labels || {};
                const roles = Object.keys(labels)
                    .filter(k => k.startsWith('node-role.kubernetes.io/'))
                    .map(k => k.replace('node-role.kubernetes.io/', ''));
                return roles.length > 0 ? roles.join(', ') : <span className="text-gray-500">-</span>;
            },
            getValue: (item) => {
                const labels = item.metadata?.labels || {};
                return Object.keys(labels)
                    .filter(k => k.startsWith('node-role.kubernetes.io/'))
                    .map(k => k.replace('node-role.kubernetes.io/', ''))
                    .join(', ');
            },
        },
        {
            key: 'internalIP',
            label: 'Internal IP',
            defaultHidden: true,
            render: (item) => {
                const addr = (item.status?.addresses || []).find(a => a.type === 'InternalIP');
                return addr?.address || <span className="text-gray-500">-</span>;
            },
            getValue: (item) => (item.status?.addresses || []).find(a => a.type === 'InternalIP')?.address || '',
        },
        {
            key: 'externalIP',
            label: 'External IP',
            defaultHidden: true,
            render: (item) => {
                const addr = (item.status?.addresses || []).find(a => a.type === 'ExternalIP');
                return addr?.address || <span className="text-gray-500">-</span>;
            },
            getValue: (item) => (item.status?.addresses || []).find(a => a.type === 'ExternalIP')?.address || '',
        },
        {
            key: 'osImage',
            label: 'OS Image',
            defaultHidden: true,
            render: (item) => item.status?.nodeInfo?.osImage || '-',
            getValue: (item) => item.status?.nodeInfo?.osImage || '',
        },
        {
            key: 'kernelVersion',
            label: 'Kernel',
            defaultHidden: true,
            render: (item) => item.status?.nodeInfo?.kernelVersion || '-',
            getValue: (item) => item.status?.nodeInfo?.kernelVersion || '',
        },
        {
            key: 'containerRuntime',
            label: 'Container Runtime',
            defaultHidden: true,
            render: (item) => item.status?.nodeInfo?.containerRuntimeVersion || '-',
            getValue: (item) => item.status?.nodeInfo?.containerRuntimeVersion || '',
        },
        {
            key: 'architecture',
            label: 'Arch',
            defaultHidden: true,
            render: (item) => item.status?.nodeInfo?.architecture || '-',
            getValue: (item) => item.status?.nodeInfo?.architecture || '',
        },
        {
            key: 'os',
            label: 'OS',
            defaultHidden: true,
            render: (item) => item.status?.nodeInfo?.operatingSystem || '-',
            getValue: (item) => item.status?.nodeInfo?.operatingSystem || '',
        },
        {
            key: 'age',
            label: 'Age',
            filterType: 'numeric',
            numericHint: 'Age in hours',
            numericUnit: 'h',
            render: (item) => formatAge(item.metadata?.creationTimestamp),
            getValue: (item) => item.metadata?.creationTimestamp,
            getNumericValue: (item) => {
                if (!item.metadata?.creationTimestamp) return NaN;
                return (Date.now() - new Date(item.metadata.creationTimestamp).getTime()) / 3600000;
            }
        },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item) => (
                <NodeActionsMenu
                    node={item}
                    isOpen={activeMenuId === `node-${item.metadata.uid}`}
                    menuPosition={menuPosition}
                    onOpenChange={(isOpen, buttonElement) => handleMenuOpenChange(isOpen, `node-${item.metadata.uid}`, buttonElement)}
                    onEditYaml={handleEditYaml}
                    onCordonUncordon={handleCordonUncordon}
                    onShell={handleShell}
                    onDelete={handleDelete}
                />
            ),
            getValue: () => '',
            isColumnSelector: true,
            disableSort: true
        }
    ], [activeMenuId, menuPosition, handleMenuOpenChange, handleEditYaml, handleCordonUncordon, handleShell, handleDelete, metrics, metricsAvailable]);

    return (
        <ResourceList
            title="Nodes"
            columns={columns}
            data={nodes}
            isLoading={loading}
            showNamespaceSelector={false}
            initialSort={{ key: 'age', direction: 'desc' }}
            resourceType="nodes"
            onRowClick={handleShowDetails}
        />
    );
}
