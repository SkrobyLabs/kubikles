import React, { useMemo, useState, useCallback } from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ResourceList from '../../../components/shared/ResourceList';
import ResourceBar from '../../../components/shared/ResourceBar';
import BulkActionModal from '../../../components/shared/BulkActionModal';
import NamespaceActionsMenu from './NamespaceActionsMenu';
import { useNamespacesList } from '../../../hooks/resources';
import { useNamespaceActions } from './useNamespaceActions';
import { useNamespaceMetrics } from '../../../hooks/useNamespaceMetrics';
import { useK8s } from '../../../context/K8sContext';
import { useMenu } from '../../../context/MenuContext';
import { useSelection } from '../../../hooks/useSelection';
import { useBulkActions } from '../../../hooks/useBulkActions';
import { DeleteNamespace, GetNamespaceYAML } from '../../../../wailsjs/go/main/App';
import { formatAge } from '../../../utils/formatting';
import { useMenuPosition } from '../../../hooks/useMenuPosition';

// Get namespace status from conditions
function getNamespaceStatus(namespace) {
    const phase = namespace.status?.phase;
    if (phase === 'Active') return 'Active';
    if (phase === 'Terminating') return 'Terminating';
    return phase || 'Unknown';
}

function getStatusColor(status) {
    switch (status) {
        case 'Active':
            return 'text-green-400';
        case 'Terminating':
            return 'text-yellow-400';
        default:
            return 'text-gray-400';
    }
}

export default function NamespaceList({ isVisible }) {
    const { currentContext } = useK8s();
    const { activeMenuId, menuPosition, handleMenuOpenChange } = useMenuPosition();
    const selection = useSelection();

    // Unified bulk actions (also used for single delete)
    const {
        bulkActionModal,
        bulkProgress,
        openBulkDelete,
        closeBulkAction,
        confirmBulkAction,
        exportYaml,
    } = useBulkActions({
        resourceLabel: 'Namespace',
        resourceType: 'namespaces',
        isNamespaced: false,
        deleteApi: DeleteNamespace,
        getYamlApi: GetNamespaceYAML,
        currentContext,
    });
    const { namespaces, loading } = useNamespacesList(currentContext, isVisible);
    const { handleShowDetails, handleEditYaml } = useNamespaceActions();

    // Load metrics after namespaces are loaded
    const { metrics, available: metricsAvailable } = useNamespaceMetrics(isVisible, !loading && namespaces.length > 0);

    const columns = useMemo(() => [
        {
            key: 'name',
            label: 'Name',
            render: (item) => item.metadata?.name,
            getValue: (item) => item.metadata?.name
        },
        {
            key: 'cpu',
            label: 'CPU',
            render: (item) => {
                const name = item.metadata?.name;
                const m = metrics[name];
                if (metricsAvailable === false) return <span className="text-gray-500 italic text-xs">N/A</span>;
                if (!m) return <span className="text-gray-500 text-xs">--</span>;
                return (
                    <div className="flex flex-col gap-0.5">
                        <ResourceBar percent={m.cpuUsagePercent} label="" tooltipLabel="Used" color="bg-blue-500" />
                        <ResourceBar percent={m.cpuCommittedPercent} label="" tooltipLabel="Committed" color="bg-red-500" fixedColor />
                    </div>
                );
            },
            getValue: (item) => {
                const name = item.metadata?.name;
                return metrics[name]?.cpuCommittedPercent ?? -1;
            }
        },
        {
            key: 'memory',
            label: 'Memory',
            render: (item) => {
                const name = item.metadata?.name;
                const m = metrics[name];
                if (metricsAvailable === false) return <span className="text-gray-500 italic text-xs">N/A</span>;
                if (!m) return <span className="text-gray-500 text-xs">--</span>;
                return (
                    <div className="flex flex-col gap-0.5">
                        <ResourceBar percent={m.memUsagePercent} label="" tooltipLabel="Used" color="bg-purple-500" />
                        <ResourceBar percent={m.memCommittedPercent} label="" tooltipLabel="Committed" color="bg-red-500" fixedColor />
                    </div>
                );
            },
            getValue: (item) => {
                const name = item.metadata?.name;
                return metrics[name]?.memCommittedPercent ?? -1;
            }
        },
        {
            key: 'status',
            label: 'Status',
            render: (item) => {
                const status = getNamespaceStatus(item);
                return <span className={getStatusColor(status)}>{status}</span>;
            },
            getValue: (item) => getNamespaceStatus(item)
        },
        {
            key: 'age',
            label: 'Age',
            render: (item) => formatAge(item.metadata?.creationTimestamp),
            getValue: (item) => item.metadata?.creationTimestamp
        },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item) => (
                <NamespaceActionsMenu
                    namespace={item}
                    isOpen={activeMenuId === `namespace-${item.metadata.uid}`}
                    menuPosition={menuPosition}
                    onOpenChange={(isOpen, buttonElement) => handleMenuOpenChange(isOpen, `namespace-${item.metadata.uid}`, buttonElement)}
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
                isOpen={bulkActionModal.isOpen}
                onClose={closeBulkAction}
                action={bulkActionModal.action}
                actionLabel="Delete"
                items={bulkActionModal.items}
                onConfirm={confirmBulkAction}
                onExportYaml={exportYaml}
                progress={bulkProgress}
            />
        </>
    );
}
