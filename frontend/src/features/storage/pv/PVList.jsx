import React, { useMemo } from 'react';
import ResourceList from '../../../components/shared/ResourceList';
import { usePVs } from '../../../hooks/usePVs';
import { useK8s } from '../../../context/K8sContext';
import { useUI } from '../../../context/UIContext';
import { formatAge } from '../../../utils/formatting';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import PVActionsMenu from './PVActionsMenu';
import { usePVActions } from './usePVActions';

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

export default function PVList({ isVisible }) {
    const { currentContext } = useK8s();
    const { activeMenuId, setActiveMenuId } = useUI();
    const { pvs, loading } = usePVs(currentContext, isVisible);
    const { handleEditYaml, handleShowDependencies, handleDelete } = usePVActions();

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item) => item.metadata?.name, getValue: (item) => item.metadata?.name },
        {
            key: 'capacity',
            label: 'Capacity',
            render: (item) => item.spec?.capacity?.storage || '-',
            getValue: (item) => item.spec?.capacity?.storage || ''
        },
        { key: 'accessModes', label: 'Access Modes', render: (item) => item.spec?.accessModes?.join(', ') || '-', getValue: (item) => item.spec?.accessModes?.join(', ') || '' },
        { key: 'reclaimPolicy', label: 'Reclaim Policy', render: (item) => item.spec?.persistentVolumeReclaimPolicy || '-', getValue: (item) => item.spec?.persistentVolumeReclaimPolicy || '' },
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
                    onOpenChange={(isOpen) => setActiveMenuId(isOpen ? `pv-${item.metadata.uid}` : null)}
                    onEditYaml={handleEditYaml}
                    onShowDependencies={handleShowDependencies}
                    onDelete={handleDelete}
                />
            ),
            getValue: () => '',
            isColumnSelector: true,
            disableSort: true
        }
    ], [activeMenuId, setActiveMenuId, handleEditYaml, handleShowDependencies, handleDelete]);

    return (
        <ResourceList
            title="Persistent Volumes"
            columns={columns}
            data={pvs}
            isLoading={loading}
            showNamespaceSelector={false}
            initialSort={{ key: 'age', direction: 'asc' }}
            resourceType="pvs"
        />
    );
}
