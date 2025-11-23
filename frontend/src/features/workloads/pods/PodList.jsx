import React, { useMemo } from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ResourceList from '../../../components/shared/ResourceList';
import PodActionsMenu from './PodActionsMenu';
import { usePods } from '../../../hooks/usePods';
import { usePodActions } from './usePodActions';
import { useK8s } from '../../../context/K8sContext';
import { useUI } from '../../../context/UIContext';
import { formatAge } from '../../../utils/formatting';
import { getPodStatus, getPodStatusColor, getContainerStatusColor, getPodStatusPriority, getPodController } from '../../../utils/k8s-helpers';

export default function PodList({ isVisible }) {
    const { currentContext, currentNamespace, setCurrentNamespace, namespaces } = useK8s();
    const { activeMenuId, setActiveMenuId } = useUI();
    // console.log("PodList rendering");
    const { pods, loading } = usePods(currentContext, currentNamespace, isVisible);
    const { openLogs, handleShell, handleEditYaml, handleDelete } = usePodActions();

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item) => item.metadata?.name, getValue: (item) => item.metadata?.name, initialSort: 'asc' },
        {
            key: 'containers',
            label: 'Containers',
            render: (item) => (
                <div className="flex gap-1">
                    {(item.status?.containerStatuses || []).map((status, i) => (
                        <div
                            key={i}
                            className={`w-3 h-3 rounded-sm ${getContainerStatusColor(status)}`}
                            title={`${status.name}: ${Object.keys(status.state || {})[0]} (${status.state?.waiting?.reason || ''})`}
                        />
                    ))}
                </div>
            ),
            getValue: (item) => getPodStatusPriority(getPodStatus(item))
        },
        {
            key: 'status',
            label: 'Status',
            render: (item) => {
                const status = getPodStatus(item);
                const colorClass = getPodStatusColor(status);
                return <span className={`font-medium ${colorClass}`}>{status}</span>;
            },
            getValue: (item) => getPodStatusPriority(getPodStatus(item))
        },
        { key: 'restarts', label: 'Restarts', render: (item) => item.status?.containerStatuses?.reduce((acc, curr) => acc + curr.restartCount, 0) || 0, getValue: (item) => item.status?.containerStatuses?.reduce((acc, curr) => acc + curr.restartCount, 0) || 0 },
        { key: 'age', label: 'Age', render: (item) => formatAge(item.metadata?.creationTimestamp), getValue: (item) => item.metadata?.creationTimestamp },
        {
            key: 'controlledBy',
            label: 'Controlled By',
            render: (item) => {
                const controller = getPodController(item);
                return controller ? (
                    <span className="text-gray-400" title={controller.name}>
                        {controller.kind}
                    </span>
                ) : (
                    <span className="text-gray-600">-</span>
                );
            },
            getValue: (item) => getPodController(item)?.kind || ''
        },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item) => (
                <PodActionsMenu
                    pod={item}
                    isOpen={activeMenuId === `pod-${item.metadata.uid}`}
                    onOpenChange={(isOpen) => setActiveMenuId(isOpen ? `pod-${item.metadata.uid}` : null)}
                    onLogs={() => {
                        const containers = [
                            ...(item.spec?.initContainers || []).map(c => c.name),
                            ...(item.spec?.containers || []).map(c => c.name)
                        ];

                        const controller = getPodController(item);
                        let siblingPods = [];
                        if (controller) {
                            siblingPods = pods
                                .filter(p => {
                                    const c = getPodController(p);
                                    return c && c.uid === controller.uid;
                                })
                                .map(p => p.metadata.name);
                        } else {
                            // If no controller, just show itself
                            siblingPods = [item.metadata.name];
                        }

                        openLogs(item.metadata.namespace, item.metadata.name, containers, siblingPods);
                    }}
                    onShell={() => handleShell(item.metadata.namespace, item.metadata.name)}
                    onDelete={() => handleDelete(item.metadata.namespace, item.metadata.name, false)}
                    onForceDelete={() => handleDelete(item.metadata.namespace, item.metadata.name, true)}
                    onEditYaml={() => handleEditYaml(item)}
                />
            ),
            isColumnSelector: true,
            disableSort: true
        },
    ], [activeMenuId, setActiveMenuId, openLogs, handleShell, handleDelete, handleEditYaml]);

    return (
        <ResourceList
            title="Pods"
            columns={columns}
            data={pods}
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
