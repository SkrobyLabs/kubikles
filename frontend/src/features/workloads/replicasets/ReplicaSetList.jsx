import React, { useMemo } from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ResourceList from '../../../components/shared/ResourceList';
import ReplicaSetActionsMenu from './ReplicaSetActionsMenu';
import { useReplicaSets } from '../../../hooks/useReplicaSets';
import { useReplicaSetActions } from './useReplicaSetActions';
import { useK8s } from '../../../context/K8sContext';
import { useUI } from '../../../context/UIContext';
import { formatAge } from '../../../utils/formatting';

export default function ReplicaSetList({ isVisible }) {
    const { currentContext, currentNamespace, setCurrentNamespace, namespaces } = useK8s();
    const { activeMenuId, setActiveMenuId } = useUI();
    const { replicaSets, loading } = useReplicaSets(currentContext, currentNamespace, isVisible);
    const { handleEditYaml, handleDelete, handleViewLogs } = useReplicaSetActions();

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item) => item.metadata?.name, getValue: (item) => item.metadata?.name, initialSort: 'asc' },
        { key: 'namespace', label: 'Namespace', render: (item) => item.metadata?.namespace, getValue: (item) => item.metadata?.namespace },
        {
            key: 'desired',
            label: 'Desired',
            render: (item) => item.spec?.replicas || 0,
            getValue: (item) => item.spec?.replicas || 0
        },
        {
            key: 'current',
            label: 'Current',
            render: (item) => item.status?.replicas || 0,
            getValue: (item) => item.status?.replicas || 0
        },
        {
            key: 'ready',
            label: 'Ready',
            render: (item) => item.status?.readyReplicas || 0,
            getValue: (item) => item.status?.readyReplicas || 0
        },
        { key: 'age', label: 'Age', render: (item) => formatAge(item.metadata?.creationTimestamp), getValue: (item) => item.metadata?.creationTimestamp },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item) => (
                <ReplicaSetActionsMenu
                    replicaSet={item}
                    isOpen={activeMenuId === `rs-${item.metadata.uid}`}
                    onOpenChange={(isOpen) => setActiveMenuId(isOpen ? `rs-${item.metadata.uid}` : null)}
                    onEditYaml={() => handleEditYaml(item)}
                    onDelete={() => handleDelete(item)}
                    onViewLogs={() => handleViewLogs(item)}
                />
            ),
            isColumnSelector: true,
            disableSort: true
        },
    ], [activeMenuId, setActiveMenuId, handleEditYaml, handleDelete, handleViewLogs]);

    return (
        <ResourceList
            title="ReplicaSets"
            columns={columns}
            data={replicaSets}
            isLoading={loading}
            namespaces={namespaces}
            currentNamespace={currentNamespace}
            onNamespaceChange={setCurrentNamespace}
            showNamespaceSelector={true}
            highlightedUid={activeMenuId}
            initialSort={{ key: 'age', direction: 'asc' }}
        />
    );
}
