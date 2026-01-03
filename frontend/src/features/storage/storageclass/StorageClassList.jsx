import React, { useMemo, useState, useCallback } from 'react';
import ResourceList from '../../../components/shared/ResourceList';
import BulkActionModal from '../../../components/shared/BulkActionModal';
import { useStorageClasses } from '../../../hooks/resources';
import { useK8s } from '../../../context/K8sContext';
import { useUI } from '../../../context/UIContext';
import { useSelection } from '../../../hooks/useSelection';
import { DeleteStorageClass, GetStorageClassYaml, SaveYamlBackup } from '../../../../wailsjs/go/main/App';
import { formatAge } from '../../../utils/formatting';
import { EllipsisVerticalIcon, CheckCircleIcon } from '@heroicons/react/24/outline';
import StorageClassActionsMenu from './StorageClassActionsMenu';
import { useStorageClassActions } from './useStorageClassActions';
import Logger from '../../../utils/Logger';

// Get color for reclaim policy
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

// Get color for volume binding mode
const getBindingModeColor = (mode) => {
    switch (mode) {
        case 'Immediate':
            return 'text-green-400';
        case 'WaitForFirstConsumer':
            return 'text-orange-400';
        default:
            return 'text-gray-400';
    }
};

export default function StorageClassList({ isVisible }) {
    const { currentContext } = useK8s();
    const { activeMenuId, setActiveMenuId } = useUI();
    const { storageClasses, loading } = useStorageClasses(currentContext, isVisible);
    const { handleShowDetails, handleEditYaml, handleDelete } = useStorageClassActions();
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
                await DeleteStorageClass(name);
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
                const yaml = await GetStorageClassYaml(item.metadata?.name);
                entries.push({ namespace: '', name: item.metadata?.name, kind: 'StorageClass', yaml });
            } catch (err) {
                entries.push({ namespace: '', name: item.metadata?.name, kind: 'StorageClass', yaml: `# Failed: ${err}` });
            }
        }
        const timestamp = new Date().toISOString().replace(/[:.]/g, '-').slice(0, 19);
        try { await SaveYamlBackup(entries, `storageclasses-backup-${timestamp}.zip`); } catch (err) { if (err?.toString()) alert('Failed: ' + err); }
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
        {
            key: 'name',
            label: 'Name',
            render: (item) => (
                <div className="flex items-center gap-2">
                    {item.metadata?.name}
                    {item.metadata?.annotations?.['storageclass.kubernetes.io/is-default-class'] === 'true' && (
                        <span className="text-xs bg-blue-500/20 text-blue-400 px-1.5 py-0.5 rounded">default</span>
                    )}
                </div>
            ),
            getValue: (item) => item.metadata?.name
        },
        { key: 'provisioner', label: 'Provisioner', render: (item) => item.provisioner || '-', getValue: (item) => item.provisioner || '' },
        {
            key: 'reclaimPolicy',
            label: 'Reclaim Policy',
            render: (item) => item.reclaimPolicy ? (
                <span className={getReclaimPolicyColor(item.reclaimPolicy)}>{item.reclaimPolicy}</span>
            ) : '-',
            getValue: (item) => item.reclaimPolicy || ''
        },
        {
            key: 'volumeBindingMode',
            label: 'Volume Binding Mode',
            render: (item) => item.volumeBindingMode ? (
                <span className={getBindingModeColor(item.volumeBindingMode)}>{item.volumeBindingMode}</span>
            ) : '-',
            getValue: (item) => item.volumeBindingMode || ''
        },
        {
            key: 'allowVolumeExpansion',
            label: 'Allow Expansion',
            render: (item) => item.allowVolumeExpansion ? (
                <CheckCircleIcon className="h-5 w-5 text-green-400" />
            ) : (
                <span className="text-gray-500">-</span>
            ),
            getValue: (item) => item.allowVolumeExpansion ? 'Yes' : 'No'
        },
        { key: 'age', label: 'Age', render: (item) => formatAge(item.metadata?.creationTimestamp), getValue: (item) => item.metadata?.creationTimestamp },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item) => (
                <StorageClassActionsMenu
                    storageClass={item}
                    isOpen={activeMenuId === `storageclass-${item.metadata.uid}`}
                    menuPosition={menuPosition}
                    onOpenChange={(isOpen, buttonElement) => handleMenuOpenChange(isOpen, `storageclass-${item.metadata.uid}`, buttonElement)}
                    onEditYaml={handleEditYaml}
                    onDelete={handleDelete}
                />
            ),
            getValue: () => '',
            isColumnSelector: true,
            disableSort: true
        }
    ], [activeMenuId, menuPosition, handleMenuOpenChange, handleEditYaml, handleDelete]);

    return (
        <>
            <ResourceList
                title="Storage Classes"
                columns={columns}
                data={storageClasses}
                isLoading={loading}
                showNamespaceSelector={false}
                initialSort={{ key: 'age', direction: 'desc' }}
                resourceType="storageclasses"
                onRowClick={handleShowDetails}
                selectable={true}
                selection={selection}
                onBulkDelete={handleBulkDeleteClick}
            />
            <BulkActionModal isOpen={bulkActionModal.isOpen} onClose={handleBulkActionClose} action={bulkActionModal.action} actionLabel="Delete" items={bulkActionModal.items} onConfirm={handleBulkActionConfirm} onExportYaml={handleExportYaml} progress={bulkProgress} />
        </>
    );
}
