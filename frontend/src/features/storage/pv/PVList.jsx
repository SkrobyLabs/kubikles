import React, { useMemo, useState, useCallback } from 'react';
import ResourceList from '../../../components/shared/ResourceList';
import BulkActionModal from '../../../components/shared/BulkActionModal';
import { usePVs } from '../../../hooks/resources';
import { useK8s } from '../../../context/K8sContext';
import { useMenu } from '../../../context/MenuContext';
import { useSelection } from '../../../hooks/useSelection';
import { DeletePV, GetPVYaml, SaveYamlBackup } from '../../../../wailsjs/go/main/App';
import { formatAge } from '../../../utils/formatting';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import PVActionsMenu from './PVActionsMenu';
import { usePVActions } from './usePVActions';
import Logger from '../../../utils/Logger';

const getStatusColor = (phase) => {
    switch (phase) {
        case 'Bound':
            return 'text-green-400';
        case 'Available':
            return 'text-blue-400';
        case 'Released':
            return 'text-yellow-400';
        case 'Failed':
            return 'text-red-400';
        default:
            return 'text-gray-400';
    }
};

const getReclaimPolicyColor = (policy) => {
    switch (policy) {
        case 'Delete':
            return 'text-red-400';
        case 'Retain':
            return 'text-green-400';
        case 'Recycle':
            return 'text-yellow-400';
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

export default function PVList({ isVisible }) {
    const { currentContext } = useK8s();
    const { activeMenuId, setActiveMenuId } = useMenu();
    const { pvs, loading } = usePVs(currentContext, isVisible);
    const { handleShowDetails, handleEditYaml, handleShowDependencies, handleDelete } = usePVActions();
    const [menuPosition, setMenuPosition] = useState({ top: 0, left: 0 });
    const selection = useSelection();

    const [bulkActionModal, setBulkActionModal] = useState({ isOpen: false, action: null, items: [] });
    const [bulkProgress, setBulkProgress] = useState({ current: 0, total: 0, status: 'idle', results: [] });

    const handleBulkDeleteClick = useCallback((selectedItems) => {
        setBulkActionModal({ isOpen: true, action: 'delete', items: selectedItems });
        setBulkProgress({ current: 0, total: selectedItems.length, status: 'idle', results: [] });
    }, []);

    const handleBulkActionConfirm = useCallback(async (items) => {
        Logger.info('Bulk delete started', { count: items.length });
        setBulkProgress(prev => ({ ...prev, status: 'inProgress', results: [] }));
        const results = [];
        for (let i = 0; i < items.length; i++) {
            const item = items[i];
            const name = item.metadata?.name;
            try {
                await DeletePV(name);
                results.push({ name, namespace: '', success: true, message: '' });
            } catch (err) {
                results.push({ name, namespace: '', success: false, message: err.toString() });
            }
            setBulkProgress(prev => ({ ...prev, current: i + 1, results: [...results] }));
        }
        setBulkProgress(prev => ({ ...prev, status: 'complete' }));
    }, []);

    const handleBulkActionClose = useCallback(() => {
        setBulkActionModal({ isOpen: false, action: null, items: [] });
        setBulkProgress({ current: 0, total: 0, status: 'idle', results: [] });
    }, []);

    const handleExportYaml = useCallback(async (items) => {
        const entries = [];
        for (const item of items) {
            try {
                const yaml = await GetPVYaml(item.metadata?.name);
                entries.push({ namespace: '', name: item.metadata?.name, kind: 'PersistentVolume', yaml });
            } catch (err) {
                entries.push({ namespace: '', name: item.metadata?.name, kind: 'PersistentVolume', yaml: `# Failed: ${err}` });
            }
        }
        const timestamp = new Date().toISOString().replace(/[:.]/g, '-').slice(0, 19);
        try { await SaveYamlBackup(entries, `pvs-backup-${timestamp}.zip`); } catch (err) { if (err?.toString()) alert('Failed: ' + err); }
    }, []);

    const handleMenuOpenChange = useCallback((isOpen, menuId, buttonElement) => {
        if (isOpen && buttonElement) {
            const rect = buttonElement.getBoundingClientRect();
            setMenuPosition({
                top: rect.bottom + 4,
                left: rect.right - 192
            });
        }
        setActiveMenuId(isOpen ? menuId : null);
    }, [setActiveMenuId]);

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item) => item.metadata?.name, getValue: (item) => item.metadata?.name },
        {
            key: 'capacity',
            label: 'Capacity',
            render: (item) => item.spec?.capacity?.storage || '-',
            getValue: (item) => item.spec?.capacity?.storage || ''
        },
        {
            key: 'accessModes',
            label: 'Access Modes',
            render: (item) => renderAccessModes(item.spec?.accessModes),
            getValue: (item) => item.spec?.accessModes?.join(', ') || ''
        },
        {
            key: 'reclaimPolicy',
            label: 'Reclaim Policy',
            render: (item) => item.spec?.persistentVolumeReclaimPolicy ? (
                <span className={getReclaimPolicyColor(item.spec.persistentVolumeReclaimPolicy)}>
                    {item.spec.persistentVolumeReclaimPolicy}
                </span>
            ) : '-',
            getValue: (item) => item.spec?.persistentVolumeReclaimPolicy || ''
        },
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
        {
            key: 'claim',
            label: 'Claim',
            render: (item) => item.spec?.claimRef ? `${item.spec.claimRef.namespace}/${item.spec.claimRef.name}` : '-',
            getValue: (item) => item.spec?.claimRef ? `${item.spec.claimRef.namespace}/${item.spec.claimRef.name}` : ''
        },
        { key: 'storageClass', label: 'Storage Class', render: (item) => item.spec?.storageClassName || '-', getValue: (item) => item.spec?.storageClassName || '' },
        { key: 'age', label: 'Age', render: (item) => formatAge(item.metadata?.creationTimestamp), getValue: (item) => item.metadata?.creationTimestamp },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item) => (
                <PVActionsMenu
                    pv={item}
                    isOpen={activeMenuId === `pv-${item.metadata.uid}`}
                    menuPosition={menuPosition}
                    onOpenChange={(isOpen, buttonElement) => handleMenuOpenChange(isOpen, `pv-${item.metadata.uid}`, buttonElement)}
                    onEditYaml={handleEditYaml}
                    onShowDependencies={handleShowDependencies}
                    onDelete={handleDelete}
                />
            ),
            getValue: () => '',
            isColumnSelector: true,
            disableSort: true
        }
    ], [activeMenuId, menuPosition, handleMenuOpenChange, handleEditYaml, handleShowDependencies, handleDelete]);

    return (
        <>
            <ResourceList
                title="Persistent Volumes"
                columns={columns}
                data={pvs}
                isLoading={loading}
                showNamespaceSelector={false}
                initialSort={{ key: 'age', direction: 'desc' }}
                resourceType="pvs"
                onRowClick={handleShowDetails}
                selectable={true}
                selection={selection}
                onBulkDelete={handleBulkDeleteClick}
            />
            <BulkActionModal isOpen={bulkActionModal.isOpen} onClose={handleBulkActionClose} action={bulkActionModal.action} actionLabel="Delete" items={bulkActionModal.items} onConfirm={handleBulkActionConfirm} onExportYaml={handleExportYaml} progress={bulkProgress} />
        </>
    );
}
