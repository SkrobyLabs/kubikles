import React, { useMemo, useState, useCallback } from 'react';
import ResourceList from '~/components/shared/ResourceList';
import BulkActionModal from '~/components/shared/BulkActionModal';
import { useStorageClasses } from '~/hooks/resources';
import { useK8s } from '~/context';
import { useSelection } from '~/hooks/useSelection';
import { useBulkActions } from '~/hooks/useBulkActions';
import { DeleteStorageClass, GetStorageClassYaml } from 'wailsjs/go/main/App';
import { formatAge } from '~/utils/formatting';
import { EllipsisVerticalIcon, CheckCircleIcon } from '@heroicons/react/24/outline';
import StorageClassActionsMenu from './StorageClassActionsMenu';
import { useStorageClassActions } from './useStorageClassActions';
import { useMenuPosition } from '~/hooks/useMenuPosition';

// Get color for reclaim policy
const getReclaimPolicyColor = (policy: any) => {
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
const getBindingModeColor = (mode: any) => {
    switch (mode) {
        case 'Immediate':
            return 'text-green-400';
        case 'WaitForFirstConsumer':
            return 'text-orange-400';
        default:
            return 'text-gray-400';
    }
};

export default function StorageClassList({ isVisible }: { isVisible: boolean }) {
    const { currentContext } = useK8s();
    const { activeMenuId, menuPosition, handleMenuOpenChange } = useMenuPosition();
    const { storageClasses, loading } = useStorageClasses(currentContext, isVisible) as any;
    const { handleShowDetails, handleEditYaml } = useStorageClassActions();
    const selection = useSelection();

    // Unified bulk actions (also used for single delete)
    const {
        bulkModalProps,
        openBulkDelete,
        exportYaml,
    } = useBulkActions({
        resourceLabel: 'StorageClass',
        resourceType: 'storageclasses',
        isNamespaced: false,
        deleteApi: DeleteStorageClass,
        getYamlApi: GetStorageClassYaml,

    });

    const columns = useMemo(() => [
        {
            key: 'name',
            label: 'Name',
            render: (item: any) => (
                <div className="flex items-center gap-2">
                    {item.metadata?.name}
                    {item.metadata?.annotations?.['storageclass.kubernetes.io/is-default-class'] === 'true' && (
                        <span className="text-xs bg-blue-500/20 text-blue-400 px-1.5 py-0.5 rounded">default</span>
                    )}
                </div>
            ),
            getValue: (item: any) => item.metadata?.name
        },
        { key: 'provisioner', label: 'Provisioner', render: (item: any) => item.provisioner || '-', getValue: (item: any) => item.provisioner || '' },
        {
            key: 'reclaimPolicy',
            label: 'Reclaim Policy',
            render: (item: any) => item.reclaimPolicy ? (
                <span className={getReclaimPolicyColor(item.reclaimPolicy)}>{item.reclaimPolicy}</span>
            ) : '-',
            getValue: (item: any) => item.reclaimPolicy || ''
        },
        {
            key: 'volumeBindingMode',
            label: 'Volume Binding Mode',
            render: (item: any) => item.volumeBindingMode ? (
                <span className={getBindingModeColor(item.volumeBindingMode)}>{item.volumeBindingMode}</span>
            ) : '-',
            getValue: (item: any) => item.volumeBindingMode || ''
        },
        {
            key: 'allowVolumeExpansion',
            label: 'Allow Expansion',
            render: (item: any) => item.allowVolumeExpansion ? (
                <CheckCircleIcon className="h-5 w-5 text-green-400" />
            ) : (
                <span className="text-gray-500">-</span>
            ),
            getValue: (item: any) => item.allowVolumeExpansion ? 'Yes' : 'No'
        },
        { key: 'age', label: 'Age', render: (item: any) => formatAge(item.metadata?.creationTimestamp), getValue: (item: any) => item.metadata?.creationTimestamp },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item: any) => (
                <StorageClassActionsMenu
                    storageClass={item}
                    isOpen={activeMenuId === `storageclass-${item.metadata.uid}`}
                    menuPosition={menuPosition}
                    onOpenChange={(isOpen: any, buttonElement: any) => handleMenuOpenChange(isOpen, `storageclass-${item.metadata.uid}`, buttonElement)}
                    onEditYaml={handleEditYaml}
                    onDelete={(storageClass: any) => openBulkDelete([storageClass])}
                />
            ),
            getValue: () => '',
            isColumnSelector: true,
            disableSort: true
        }
    ], [activeMenuId, menuPosition, handleMenuOpenChange, handleEditYaml, openBulkDelete]);

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
