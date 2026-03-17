import React, { useMemo, useState, useCallback, useEffect, memo } from 'react';
import ResourceList from '~/components/shared/ResourceList';
import AggregateResourceBar from '~/components/shared/AggregateResourceBar';
import ResourceBar from '~/components/shared/ResourceBar';
import BulkActionModal from '~/components/shared/BulkActionModal';
import { useNodes } from '~/hooks/resources';
import { useNodeMetrics } from '~/hooks/useNodeMetrics';
import { useSelection } from '~/hooks/useSelection';
import { useBulkActions } from '~/hooks/useBulkActions';
import { useK8s } from '~/context';
import { useUI } from '~/context';
import { useMenu } from '~/context';
import { formatAge, formatBytes, formatCpu } from '~/utils/formatting';
import { EllipsisVerticalIcon, TableCellsIcon, Squares2X2Icon, NoSymbolIcon, CheckCircleIcon } from '@heroicons/react/24/outline';
import { SetNodeSchedulable, DeleteNode, GetNodeYaml } from 'wailsjs/go/main/App';
import NodeActionsMenu from './NodeActionsMenu';
import { useNodeActions } from './useNodeActions';
import { useMenuPosition } from '~/hooks/useMenuPosition';
import NodeTopology from '../topology/NodeTopology';

// Helper to get node conditions summary
const getConditionsSummary = (node: any) => {
    const conditions = node.status?.conditions || [];
    const isUnschedulable = node.spec?.unschedulable === true;

    const issues: { type: string; severity: string }[] = [];

    // Check Ready condition
    const readyCondition = conditions.find((c: any) => c.type === 'Ready');
    const isReady = readyCondition?.status === 'True';

    if (!isReady) {
        issues.push({ type: 'NotReady', severity: 'error' });
    }

    if (isUnschedulable) {
        issues.push({ type: 'SchedulingDisabled', severity: 'warning' });
    }

    // Check pressure conditions (these are problems when True)
    const pressureConditions = ['MemoryPressure', 'DiskPressure', 'PIDPressure'];
    pressureConditions.forEach((condType: any) => {
        const cond = conditions.find((c: any) => c.type === condType);
        if (cond?.status === 'True') {
            issues.push({ type: condType, severity: 'error' });
        }
    });

    // Check NetworkUnavailable (problem when True)
    const networkCond = conditions.find((c: any) => c.type === 'NetworkUnavailable');
    if (networkCond?.status === 'True') {
        issues.push({ type: 'NetworkUnavailable', severity: 'error' });
    }

    return { isReady, isUnschedulable, issues };
};

// Conditions cell component
const ConditionsCell = memo(function ConditionsCell({ node }: { node: any }) {
    const { isReady, issues } = getConditionsSummary(node);

    if (issues.length === 0) {
        return <span className="text-green-400">Ready</span>;
    }

    return (
        <div className="flex flex-wrap gap-1">
            {issues.map((issue: any, idx: number) => (
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
const TaintsCell = memo(function TaintsCell({ node }: { node: any }) {
    const taints = node.spec?.taints || [];
    const count = taints.length;

    if (count === 0) {
        return <span className="text-gray-500">-</span>;
    }

    const tooltipLines = taints.map((t: any) =>
        `${t.key}${t.value ? '=' + t.value : ''}:${t.effect}`
    );

    return (
        <span className="relative group cursor-help text-gray-300 underline decoration-dotted">
            {count}
            <div className="absolute z-50 invisible opacity-0 group-hover:visible group-hover:opacity-100 transition-opacity delay-500 bottom-full left-1/2 -translate-x-1/2 mb-1 px-2 py-1 text-xs bg-background border border-border rounded shadow-lg whitespace-pre text-left">
                {tooltipLines.map((line: any, idx: number) => (
                    <div key={idx}>{line}</div>
                ))}
            </div>
        </span>
    );
});

const STORAGE_KEY = 'kubikles-nodes-view';

export default function NodeList({ isVisible }: { isVisible: boolean }) {
    const { currentContext } = useK8s();
    const { activeMenuId, menuPosition, handleMenuOpenChange } = useMenuPosition();
    const { nodes, loading, refetch } = useNodes(currentContext, isVisible) as any;
    // Delay metrics fetch until nodes are loaded to prioritize showing node list first
    const { metrics, available: metricsAvailable } = useNodeMetrics(isVisible, !loading && nodes.length > 0);
    const nodeActions = useNodeActions(refetch);
    const { handleShowDetails, handleEditYaml, handleCordonUncordon, handleShell, handleDelete } = nodeActions;

    // Multi-select
    const selection = useSelection();
    const cordonNode = useCallback((name: string) => SetNodeSchedulable(name, false), []);
    const uncordonNode = useCallback((name: string) => SetNodeSchedulable(name, true), []);
    const {
        bulkModalProps,
        bulkActionModal,
        openBulkCustomAction,
        exportYaml,
    } = useBulkActions({
        resourceLabel: 'Node',
        resourceType: 'nodes',
        isNamespaced: false,
        deleteApi: DeleteNode,
        customApis: { cordon: cordonNode, uncordon: uncordonNode },
        getYamlApi: GetNodeYaml,
    });

    // Compute cordon/uncordon button visibility from selected items
    const selectedItems = selection.getSelectedItems(nodes);
    const hasSchedulable = selectedItems.some((n: any) => !n.spec?.unschedulable);
    const hasCordoned = selectedItems.some((n: any) => n.spec?.unschedulable === true);

    const bulkCustomActions = selectedItems.length > 0 ? (
        <>
            {hasSchedulable && (
                <button
                    onClick={() => openBulkCustomAction(selectedItems, 'cordon')}
                    className="flex items-center gap-1.5 px-3 py-1.5 text-sm text-yellow-400 hover:text-yellow-300 hover:bg-yellow-500/10 rounded transition-colors"
                    title="Cordon selected nodes"
                >
                    <NoSymbolIcon className="h-4 w-4" />
                    <span>Cordon</span>
                </button>
            )}
            {hasCordoned && (
                <button
                    onClick={() => openBulkCustomAction(selectedItems, 'uncordon')}
                    className="flex items-center gap-1.5 px-3 py-1.5 text-sm text-green-400 hover:text-green-300 hover:bg-green-500/10 rounded transition-colors"
                    title="Uncordon selected nodes"
                >
                    <CheckCircleIcon className="h-4 w-4" />
                    <span>Uncordon</span>
                </button>
            )}
        </>
    ) : null;

    // View mode: list or topology, persisted to localStorage
    const [viewMode, setViewMode] = useState<'list' | 'topology'>(() =>
        (localStorage.getItem(STORAGE_KEY) as 'list' | 'topology') || 'list'
    );

    useEffect(() => {
        localStorage.setItem(STORAGE_KEY, viewMode);
    }, [viewMode]);

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item: any) => item.metadata?.name, getValue: (item: any) => item.metadata?.name },
        {
            key: 'cpu',
            label: 'CPU',
            filterType: 'numeric',
            numericHint: 'CPU usage % (0-100)',
            numericUnit: '%',
            render: (item: any) => {
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
            getValue: (item: any) => metrics[item.metadata?.name]?.cpuCommittedPercent ?? -1,
            getNumericValue: (item: any) => metrics[item.metadata?.name]?.cpuPercent ?? NaN
        },
        {
            key: 'memory',
            label: 'Memory',
            filterType: 'numeric',
            numericHint: 'Memory usage % (0-100)',
            numericUnit: '%',
            render: (item: any) => {
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
            getValue: (item: any) => metrics[item.metadata?.name]?.memCommittedPercent ?? -1,
            getNumericValue: (item: any) => metrics[item.metadata?.name]?.memPercent ?? NaN
        },
        {
            key: 'pods',
            label: 'Pods',
            render: (item: any) => {
                const m = metrics[item.metadata?.name];
                if (metricsAvailable === false) return <span className="text-gray-500 italic text-xs">N/A</span>;
                if (!m) return <span className="text-gray-500 text-xs">--</span>;
                return <ResourceBar percent={m.podPercent} label="" tooltipLabel={`${m.podCount}/${m.podCapacity}`} color="bg-green-500" />;
            },
            getValue: (item: any) => metrics[item.metadata?.name]?.podPercent ?? -1
        },
        {
            key: 'conditions',
            label: 'Conditions',
            render: (item: any) => <ConditionsCell node={item} />,
            getValue: (item: any) => {
                const { issues } = getConditionsSummary(item);
                return issues.length === 0 ? 'Ready' : issues.map((i: any) => i.type).join(',');
            }
        },
        {
            key: 'taints',
            label: 'Taints',
            align: 'center',
            filterable: false,
            render: (item: any) => <TaintsCell node={item} />,
            getValue: (item: any) => (item.spec?.taints || []).length
        },
        { key: 'version', label: 'Version', render: (item: any) => item.status?.nodeInfo?.kubeletVersion, getValue: (item: any) => item.status?.nodeInfo?.kubeletVersion },
        // Hidden by default columns
        {
            key: 'roles',
            label: 'Roles',
            defaultHidden: true,
            render: (item: any) => {
                const labels = item.metadata?.labels || {};
                const roles = Object.keys(labels)
                    .filter((k: any) => k.startsWith('node-role.kubernetes.io/'))
                    .map((k: any) => k.replace('node-role.kubernetes.io/', ''));
                return roles.length > 0 ? roles.join(', ') : <span className="text-gray-500">-</span>;
            },
            getValue: (item: any) => {
                const labels = item.metadata?.labels || {};
                return Object.keys(labels)
                    .filter((k: any) => k.startsWith('node-role.kubernetes.io/'))
                    .map((k: any) => k.replace('node-role.kubernetes.io/', ''))
                    .join(', ');
            },
        },
        {
            key: 'internalIP',
            label: 'Internal IP',
            defaultHidden: true,
            render: (item: any) => {
                const addr = (item.status?.addresses || []).find((a: any) => a.type === 'InternalIP');
                return addr?.address || <span className="text-gray-500">-</span>;
            },
            getValue: (item: any) => (item.status?.addresses || []).find((a: any) => a.type === 'InternalIP')?.address || '',
        },
        {
            key: 'externalIP',
            label: 'External IP',
            defaultHidden: true,
            render: (item: any) => {
                const addr = (item.status?.addresses || []).find((a: any) => a.type === 'ExternalIP');
                return addr?.address || <span className="text-gray-500">-</span>;
            },
            getValue: (item: any) => (item.status?.addresses || []).find((a: any) => a.type === 'ExternalIP')?.address || '',
        },
        {
            key: 'osImage',
            label: 'OS Image',
            defaultHidden: true,
            render: (item: any) => item.status?.nodeInfo?.osImage || '-',
            getValue: (item: any) => item.status?.nodeInfo?.osImage || '',
        },
        {
            key: 'kernelVersion',
            label: 'Kernel',
            defaultHidden: true,
            render: (item: any) => item.status?.nodeInfo?.kernelVersion || '-',
            getValue: (item: any) => item.status?.nodeInfo?.kernelVersion || '',
        },
        {
            key: 'containerRuntime',
            label: 'Container Runtime',
            defaultHidden: true,
            render: (item: any) => item.status?.nodeInfo?.containerRuntimeVersion || '-',
            getValue: (item: any) => item.status?.nodeInfo?.containerRuntimeVersion || '',
        },
        {
            key: 'architecture',
            label: 'Arch',
            defaultHidden: true,
            render: (item: any) => item.status?.nodeInfo?.architecture || '-',
            getValue: (item: any) => item.status?.nodeInfo?.architecture || '',
        },
        {
            key: 'os',
            label: 'OS',
            defaultHidden: true,
            render: (item: any) => item.status?.nodeInfo?.operatingSystem || '-',
            getValue: (item: any) => item.status?.nodeInfo?.operatingSystem || '',
        },
        {
            key: 'age',
            label: 'Age',
            filterType: 'numeric',
            numericHint: 'Age in hours',
            numericUnit: 'h',
            render: (item: any) => formatAge(item.metadata?.creationTimestamp),
            getValue: (item: any) => item.metadata?.creationTimestamp,
            getNumericValue: (item: any) => {
                if (!item.metadata?.creationTimestamp) return NaN;
                return (Date.now() - new Date(item.metadata.creationTimestamp).getTime()) / 3600000;
            }
        },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item: any) => (
                <NodeActionsMenu
                    node={item}
                    isOpen={activeMenuId === `node-${item.metadata.uid}`}
                    menuPosition={menuPosition}
                    onOpenChange={(isOpen: any, buttonElement: any) => handleMenuOpenChange(isOpen, `node-${item.metadata.uid}`, buttonElement)}
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
    ] as any[], [activeMenuId, menuPosition, handleMenuOpenChange, handleEditYaml, handleCordonUncordon, handleShell, handleDelete, metrics, metricsAvailable]);

    const viewToggle = (
        <div className="flex items-center gap-0.5 bg-surface rounded-md p-0.5 border border-border">
            <button
                onClick={() => setViewMode('list')}
                className={`p-1.5 rounded transition-colors ${viewMode === 'list' ? 'bg-surface-light text-white' : 'text-gray-500 hover:text-gray-300'}`}
                title="List view"
            >
                <TableCellsIcon className="h-4 w-4" />
            </button>
            <button
                onClick={() => setViewMode('topology')}
                className={`p-1.5 rounded transition-colors ${viewMode === 'topology' ? 'bg-surface-light text-white' : 'text-gray-500 hover:text-gray-300'}`}
                title="Topology view"
            >
                <Squares2X2Icon className="h-4 w-4" />
            </button>
        </div>
    );

    if (viewMode === 'topology') {
        return (
            <ResourceList
                title="Nodes"
                columns={columns}
                data={nodes}
                isLoading={false}
                showNamespaceSelector={false}
                initialSort={{ key: 'age', direction: 'desc' }}
                resourceType="nodes"
                onRowClick={handleShowDetails}
                customHeaderActions={viewToggle}
                customBody={
                    <NodeTopology
                        isVisible={isVisible}
                        nodes={nodes}
                        nodesLoading={loading}
                        metrics={metrics}
                        metricsAvailable={metricsAvailable}
                        nodeActions={nodeActions}
                    />
                }
            />
        );
    }

    return (
        <>
            <ResourceList
                title="Nodes"
                columns={columns}
                data={nodes}
                isLoading={loading}
                showNamespaceSelector={false}
                initialSort={{ key: 'age', direction: 'desc' }}
                resourceType="nodes"
                onRowClick={handleShowDetails}
                customHeaderActions={viewToggle}
                selectable={true}
                selection={selection}
                onBulkExportYaml={exportYaml}
                bulkCustomActions={bulkCustomActions}
            />
            <BulkActionModal
                {...bulkModalProps}
                action={bulkActionModal.action || 'cordon'}
                actionLabel={bulkActionModal.action === 'uncordon' ? 'Uncordon' : 'Cordon'}
                onExportYaml={exportYaml}
            />
        </>
    );
}
