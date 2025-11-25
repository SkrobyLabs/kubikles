import React, { useMemo } from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ResourceList from '../../../components/shared/ResourceList';
import DaemonSetActionsMenu from './DaemonSetActionsMenu';
import { useDaemonSets } from '../../../hooks/useDaemonSets';
import { useDaemonSetActions } from './useDaemonSetActions';
import { useK8s } from '../../../context/K8sContext';
import { useUI } from '../../../context/UIContext';
import { formatAge } from '../../../utils/formatting';

export default function DaemonSetList({ isVisible }) {
    const { currentContext, selectedNamespaces, setSelectedNamespaces, namespaces } = useK8s();
    const { activeMenuId, setActiveMenuId } = useUI();
    const { daemonSets, loading } = useDaemonSets(currentContext, selectedNamespaces, isVisible);
    const { handleEditYaml, handleShowDependencies, handleRestart, handleDelete, handleViewLogs } = useDaemonSetActions();

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item) => item.metadata?.name, getValue: (item) => item.metadata?.name, initialSort: 'asc' },
        { key: 'namespace', label: 'Namespace', render: (item) => item.metadata?.namespace, getValue: (item) => item.metadata?.namespace },
        {
            key: 'desired',
            label: 'Desired',
            render: (item) => item.status?.desiredNumberScheduled || 0,
            getValue: (item) => item.status?.desiredNumberScheduled || 0
        },
        {
            key: 'current',
            label: 'Current',
            render: (item) => item.status?.currentNumberScheduled || 0,
            getValue: (item) => item.status?.currentNumberScheduled || 0
        },
        {
            key: 'ready',
            label: 'Ready',
            render: (item) => item.status?.numberReady || 0,
            getValue: (item) => item.status?.numberReady || 0
        },
        {
            key: 'available',
            label: 'Available',
            render: (item) => item.status?.numberAvailable || 0,
            getValue: (item) => item.status?.numberAvailable || 0
        },
        { key: 'age', label: 'Age', render: (item) => formatAge(item.metadata?.creationTimestamp), getValue: (item) => item.metadata?.creationTimestamp },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item) => (
                <DaemonSetActionsMenu
                    daemonSet={item}
                    isOpen={activeMenuId === `ds-${item.metadata.uid}`}
                    onOpenChange={(isOpen) => setActiveMenuId(isOpen ? `ds-${item.metadata.uid}` : null)}
                    onEditYaml={() => handleEditYaml(item)}
                    onShowDependencies={() => handleShowDependencies(item)}
                    onRestart={() => handleRestart(item)}
                    onDelete={() => handleDelete(item)}
                    onViewLogs={() => handleViewLogs(item)}
                />
            ),
            isColumnSelector: true,
            disableSort: true
        },
    ], [activeMenuId, setActiveMenuId, handleEditYaml, handleShowDependencies, handleRestart, handleDelete, handleViewLogs]);

    return (
        <ResourceList
            title="DaemonSets"
            columns={columns}
            data={daemonSets}
            isLoading={loading}
            namespaces={namespaces}
            currentNamespace={selectedNamespaces}
            onNamespaceChange={setSelectedNamespaces}
            showNamespaceSelector={true}
            multiSelectNamespaces={true}
            highlightedUid={activeMenuId}
            initialSort={{ key: 'age', direction: 'asc' }}
            resourceType="daemonsets"
        />
    );
}
