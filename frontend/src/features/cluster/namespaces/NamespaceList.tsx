import React, { useMemo, useState, useCallback } from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ResourceList from '~/components/shared/ResourceList';
import AggregateResourceBar from '~/components/shared/AggregateResourceBar';
import BulkActionModal from '~/components/shared/BulkActionModal';
import NamespaceActionsMenu from './NamespaceActionsMenu';
import { useNamespacesList } from '~/hooks/resources';
import { useNamespaceActions } from './useNamespaceActions';
import { useNamespaceMetrics } from '~/hooks/useNamespaceMetrics';
import { useK8s } from '~/context';
import { useMenu } from '~/context';
import { useSelection } from '~/hooks/useSelection';
import { useBulkActions } from '~/hooks/useBulkActions';
import { DeleteNamespace, GetNamespaceYAML } from 'wailsjs/go/main/App';
import { formatAge, formatBytes, formatCpu } from '~/utils/formatting';
import { useMenuPosition } from '~/hooks/useMenuPosition';

// Get namespace status from conditions
function getNamespaceStatus(namespace: any) {
    const phase = namespace.status?.phase;
    if (phase === 'Active') return 'Active';
    if (phase === 'Terminating') return 'Terminating';
    return phase || 'Unknown';
}

function getStatusColor(status: any) {
    switch (status) {
        case 'Active':
            return 'text-green-400';
        case 'Terminating':
            return 'text-yellow-400';
        default:
            return 'text-gray-400';
    }
}

export default function NamespaceList({ isVisible }: { isVisible: boolean }) {
    const { currentContext } = useK8s();
    const { activeMenuId, menuPosition, handleMenuOpenChange } = useMenuPosition();
    const selection = useSelection();

    // Unified bulk actions (also used for single delete)
    const {
        bulkModalProps,
        openBulkDelete,
        exportYaml,
    } = useBulkActions({
        resourceLabel: 'Namespace',
        resourceType: 'namespaces',
        isNamespaced: false,
        deleteApi: DeleteNamespace,
        getYamlApi: GetNamespaceYAML,

    });
    const { namespaces, loading } = useNamespacesList(currentContext, isVisible) as any;
    const { handleShowDetails, handleEditYaml } = useNamespaceActions();

    // Load metrics after namespaces are loaded
    const { metrics, available: metricsAvailable } = useNamespaceMetrics(isVisible, !loading && (namespaces as any)?.length > 0);

    const columns = useMemo(() => [
        {
            key: 'name',
            label: 'Name',
            render: (item: any) => item.metadata?.name,
            getValue: (item: any) => item.metadata?.name
        },
        {
            key: 'cpu',
            label: 'CPU',
            render: (item: any) => {
                const name = item.metadata?.name;
                const m = metrics[name];
                if (metricsAvailable === false) return <span className="text-gray-500 italic text-xs">N/A</span>;
                if (!m) return <span className="text-gray-500 text-xs">--</span>;
                return (
                    <AggregateResourceBar
                        usagePercent={m.cpuUsagePercent}
                        reservedPercent={m.cpuReservedPercent}
                        committedPercent={m.cpuCommittedPercent}
                        type="cpu"
                        label="CPU"
                        usageValue={m.cpuUsage}
                        reservedValue={m.cpuRequested}
                        committedValue={m.cpuCommitted}
                        formatValue={formatCpu}
                    />
                );
            },
            getValue: (item: any) => {
                const name = item.metadata?.name;
                return metrics[name]?.cpuCommittedPercent ?? -1;
            }
        },
        {
            key: 'memory',
            label: 'Memory',
            render: (item: any) => {
                const name = item.metadata?.name;
                const m = metrics[name];
                if (metricsAvailable === false) return <span className="text-gray-500 italic text-xs">N/A</span>;
                if (!m) return <span className="text-gray-500 text-xs">--</span>;
                return (
                    <AggregateResourceBar
                        usagePercent={m.memUsagePercent}
                        reservedPercent={m.memReservedPercent}
                        committedPercent={m.memCommittedPercent}
                        type="memory"
                        label="Memory"
                        usageValue={m.memUsage}
                        reservedValue={m.memRequested}
                        committedValue={m.memCommitted}
                        formatValue={formatBytes}
                    />
                );
            },
            getValue: (item: any) => {
                const name = item.metadata?.name;
                return metrics[name]?.memCommittedPercent ?? -1;
            }
        },
        {
            key: 'status',
            label: 'Status',
            render: (item: any) => {
                const status = getNamespaceStatus(item);
                return <span className={getStatusColor(status)}>{status}</span>;
            },
            getValue: (item: any) => getNamespaceStatus(item)
        },
        {
            key: 'age',
            label: 'Age',
            render: (item: any) => formatAge(item.metadata?.creationTimestamp),
            getValue: (item: any) => item.metadata?.creationTimestamp
        },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item: any) => (
                <NamespaceActionsMenu
                    namespace={item}
                    isOpen={activeMenuId === `namespace-${item.metadata.uid}`}
                    menuPosition={menuPosition}
                    onOpenChange={(isOpen: any, buttonElement: any) => handleMenuOpenChange(isOpen, `namespace-${item.metadata.uid}`, buttonElement)}
                    onEditYaml={() => handleEditYaml(item)}
                    onDelete={() => openBulkDelete([item])}
                />
            ),
            isColumnSelector: true,
            disableSort: true
        },
    ], [activeMenuId, menuPosition, handleMenuOpenChange, handleEditYaml, openBulkDelete, metrics, metricsAvailable]);

    return (
        <>
            <ResourceList
                title="Namespaces"
                columns={columns}
                data={namespaces}
                isLoading={loading}
                showNamespaceSelector={false}
                initialSort={{ key: 'age', direction: 'desc' }}
                resourceType="namespaces"
                onRowClick={handleShowDetails}
                selectable={true}
                selection={selection}
                onBulkDelete={openBulkDelete}
            />
            <BulkActionModal
                {...bulkModalProps}
                action="delete"
                actionLabel="Delete"
                onExportYaml={exportYaml}
            />
        </>
    );
}
