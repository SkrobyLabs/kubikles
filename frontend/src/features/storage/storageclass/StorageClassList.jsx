import React, { useMemo } from 'react';
import ResourceList from '../../../components/shared/ResourceList';
import { useStorageClasses } from '../../../hooks/useStorageClasses';
import { useK8s } from '../../../context/K8sContext';
import { useUI } from '../../../context/UIContext';
import { formatAge } from '../../../utils/formatting';
import { EllipsisVerticalIcon, CheckCircleIcon } from '@heroicons/react/24/outline';
import StorageClassActionsMenu from './StorageClassActionsMenu';
import { useStorageClassActions } from './useStorageClassActions';

export default function StorageClassList({ isVisible }) {
    const { currentContext } = useK8s();
    const { activeMenuId, setActiveMenuId } = useUI();
    const { storageClasses, loading } = useStorageClasses(currentContext, isVisible);
    const { handleEditYaml, handleDelete } = useStorageClassActions();

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
        { key: 'reclaimPolicy', label: 'Reclaim Policy', render: (item) => item.reclaimPolicy || '-', getValue: (item) => item.reclaimPolicy || '' },
        { key: 'volumeBindingMode', label: 'Volume Binding Mode', render: (item) => item.volumeBindingMode || '-', getValue: (item) => item.volumeBindingMode || '' },
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
                    onOpenChange={(isOpen) => setActiveMenuId(isOpen ? `storageclass-${item.metadata.uid}` : null)}
                    onEditYaml={handleEditYaml}
                    onDelete={handleDelete}
                />
            ),
            getValue: () => '',
            isColumnSelector: true,
            disableSort: true
        }
    ], [activeMenuId, setActiveMenuId, handleEditYaml, handleDelete]);

    return (
        <ResourceList
            title="Storage Classes"
            columns={columns}
            data={storageClasses}
            isLoading={loading}
            showNamespaceSelector={false}
            initialSort={{ key: 'age', direction: 'desc' }}
            resourceType="storageclasses"
        />
    );
}
