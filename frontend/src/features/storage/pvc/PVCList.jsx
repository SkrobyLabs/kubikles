import React, { useMemo, useState, useCallback } from 'react';
import ResourceList from '../../../components/shared/ResourceList';
import BulkActionModal from '../../../components/shared/BulkActionModal';
import { usePVCs } from '../../../hooks/resources';
import { useK8s } from '../../../context/K8sContext';
import { useSelection } from '../../../hooks/useSelection';
import { useBulkActions } from '../../../hooks/useBulkActions';
import { DeletePVC, GetPVCYaml } from '../../../../wailsjs/go/main/App';
import { formatAge } from '../../../utils/formatting';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import PVCActionsMenu from './PVCActionsMenu';
import { usePVCActions } from './usePVCActions';
import { useMenuPosition } from '../../../hooks/useMenuPosition';

const getStatusColor = (phase) => {
    switch (phase) {
        case 'Bound':
            return 'text-green-400';
        case 'Pending':
            return 'text-yellow-400';
        case 'Lost':
            return 'text-red-400';
        default:
            return 'text-gray-400';
    }
};

const getAccessModeColor = (mode) => {
    switch (mode) {
        case 'ReadWriteOnce':
            return 'text-blue-400';
        case 'ReadOnlyMany':
            return 'text-yellow-400';
        case 'ReadWriteMany':
            return 'text-green-400';
        case 'ReadWriteOncePod':
            return 'text-purple-400';
        default:
            return 'text-gray-400';
    }
};

const renderAccessModes = (modes) => {
    if (!modes || modes.length === 0) return '-';
    return (
        <span className="flex flex-wrap gap-1">
            {modes.map((mode, idx) => (
                <span key={idx} className={getAccessModeColor(mode)}>{mode}</span>
            ))}
        </span>
    );
};

export default function PVCList({ isVisible }) {
    const { currentContext, selectedNamespaces, setSelectedNamespaces, namespaces } = useK8s();
    const { activeMenuId, menuPosition, handleMenuOpenChange } = useMenuPosition();
    const { pvcs, loading } = usePVCs(currentContext, selectedNamespaces, isVisible);
    const { handleShowDetails, handleEditYaml, handleShowDependencies } = usePVCActions();
    const selection = useSelection();

    const {
        bulkActionModal,
        bulkProgress,
        openBulkDelete,
        closeBulkAction,
        confirmBulkAction,
        exportYaml,
    } = useBulkActions({
        resourceLabel: 'PersistentVolumeClaim',
        resourceType: 'pvcs',
        isNamespaced: true,
        deleteApi: DeletePVC,
        getYamlApi: GetPVCYaml,
        currentContext,
    });

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item) => item.metadata?.name, getValue: (item) => item.metadata?.name },
        { key: 'namespace', label: 'Namespace', render: (item) => item.metadata?.namespace, getValue: (item) => item.metadata?.namespace },
        {
            key: 'status',
            label: 'Status',
            render: (item) => (
                <span className={getStatusColor(item.status?.phase)}>
                    {item.status?.phase || 'Unknown'}
                </span>
            ),
            getValue: (item) => item.status?.phase
        },
        { key: 'volume', label: 'Volume', render: (item) => item.spec?.volumeName || '-', getValue: (item) => item.spec?.volumeName || '' },
        {
            key: 'capacity',
            label: 'Capacity',
            render: (item) => item.status?.capacity?.storage || item.spec?.resources?.requests?.storage || '-',
            getValue: (item) => item.status?.capacity?.storage || item.spec?.resources?.requests?.storage || ''
        },
        { key: 'accessModes', label: 'Access Modes', render: (item) => renderAccessModes(item.spec?.accessModes), getValue: (item) => item.spec?.accessModes?.join(', ') || '' },
        { key: 'storageClass', label: 'Storage Class', render: (item) => item.spec?.storageClassName || '-', getValue: (item) => item.spec?.storageClassName || '' },
        { key: 'age', label: 'Age', render: (item) => formatAge(item.metadata?.creationTimestamp), getValue: (item) => item.metadata?.creationTimestamp },
        // Hidden by default columns
        {
            key: 'volumeMode',
            label: 'Volume Mode',
            defaultHidden: true,
            render: (item) => item.spec?.volumeMode || 'Filesystem',
            getValue: (item) => item.spec?.volumeMode || 'Filesystem',
        },
        {
            key: 'requested',
            label: 'Requested',
            defaultHidden: true,
            render: (item) => item.spec?.resources?.requests?.storage || '-',
            getValue: (item) => item.spec?.resources?.requests?.storage || '',
        },
        {
            key: 'selector',
            label: 'Selector',
            defaultHidden: true,
            render: (item) => {
                const labels = item.spec?.selector?.matchLabels || {};
                const entries = Object.entries(labels);
                if (entries.length === 0) return <span className="text-gray-500">-</span>;
                return <span title={entries.map(([k, v]) => `${k}=${v}`).join('\n')}>{entries.length} label{entries.length > 1 ? 's' : ''}</span>;
            },
            getValue: (item) => Object.entries(item.spec?.selector?.matchLabels || {}).map(([k, v]) => `${k}=${v}`).join(','),
        },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item) => (
                <PVCActionsMenu
                    pvc={item}
                    isOpen={activeMenuId === `pvc-${item.metadata.uid}`}
                    menuPosition={menuPosition}
                    onOpenChange={(isOpen, buttonElement) => handleMenuOpenChange(isOpen, `pvc-${item.metadata.uid}`, buttonElement)}
                    onEditYaml={handleEditYaml}
                    onShowDependencies={handleShowDependencies}
                    onDelete={(item) => openBulkDelete([item])}
                />
            ),
            getValue: () => '',
            isColumnSelector: true,
            disableSort: true
        }
    ], [activeMenuId, menuPosition, handleMenuOpenChange, handleEditYaml, handleShowDependencies, openBulkDelete]);

    return (
        <>
            <ResourceList
                title="Persistent Volume Claims"
                columns={columns}
                data={pvcs}
                isLoading={loading}
                namespaces={namespaces}
                currentNamespace={selectedNamespaces}
                onNamespaceChange={setSelectedNamespaces}
                showNamespaceSelector={true}
                multiSelectNamespaces={true}
                initialSort={{ key: 'age', direction: 'desc' }}
                resourceType="pvcs"
                onRowClick={handleShowDetails}
                selectable={true}
                selection={selection}
                onBulkDelete={openBulkDelete}
            />
            <BulkActionModal isOpen={bulkActionModal.isOpen} onClose={closeBulkAction} action={bulkActionModal.action} actionLabel="Delete" items={bulkActionModal.items} onConfirm={confirmBulkAction} onExportYaml={exportYaml} progress={bulkProgress} />
        </>
    );
}
