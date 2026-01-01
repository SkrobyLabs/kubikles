import React, { useMemo, useState, useCallback } from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ResourceList from '../../../components/shared/ResourceList';
import DeploymentActionsMenu from './DeploymentActionsMenu';
import { useDeployments, usePods } from '../../../hooks/resources';
import { useDeploymentActions } from './useDeploymentActions';
import { useK8s } from '../../../context/K8sContext';
import { useUI } from '../../../context/UIContext';
import { formatAge } from '../../../utils/formatting';
import { getDeploymentPods, getEffectivePodStatus, getPodStatusColor } from '../../../utils/k8s-helpers';

export default function DeploymentList({ isVisible }) {
    const { currentContext, selectedNamespaces, setSelectedNamespaces, namespaces } = useK8s();
    const { activeMenuId, setActiveMenuId } = useUI();
    const [menuPosition, setMenuPosition] = useState({ top: 0, left: 0 });

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
    // console.log("DeploymentList rendering");
    const { deployments, loading: deploymentsLoading } = useDeployments(currentContext, selectedNamespaces, isVisible);
    const { pods: allPods, loading: podsLoading } = usePods(currentContext, selectedNamespaces, isVisible); // Fetch pods for status
    const { handleShowDetails, handleEditYaml, handleShowDependencies, handleRestart, handleDelete, handleViewLogs } = useDeploymentActions();

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item) => item.metadata?.name, getValue: (item) => item.metadata?.name, initialSort: 'asc' },
        { key: 'namespace', label: 'Namespace', render: (item) => item.metadata?.namespace, getValue: (item) => item.metadata?.namespace },
        {
            key: 'pods',
            label: 'Pods',
            render: (item) => {
                if (podsLoading && allPods.length === 0) { // Only show loading if we have no pods yet
                    // Show placeholders based on replicas count
                    const count = item.spec?.replicas ?? 1;
                    if (count === 0) return null;
                    return (
                        <div className="flex gap-1">
                            {Array.from({ length: Math.min(count, 5) }).map((_, i) => (
                                <div
                                    key={i}
                                    className="w-3 h-3 rounded-sm bg-gray-700 animate-pulse"
                                    title="Loading pods..."
                                />
                            ))}
                            {count > 5 && <span className="text-xs text-gray-500">...</span>}
                        </div>
                    );
                }
                return (
                    <div className="flex gap-1">
                        {getDeploymentPods(item, allPods).map((pod) => {
                            const status = getEffectivePodStatus(pod);
                            const colorClass = getPodStatusColor(status).replace('text-', 'bg-');
                            return (
                                <div
                                    key={pod.metadata.uid}
                                    className={`w-3 h-3 rounded-sm ${colorClass}`}
                                    title={`${pod.metadata.name}: ${status}`}
                                />
                            );
                        })}
                    </div>
                );
            },
            getValue: (item) => getDeploymentPods(item, allPods).length
        },
        { key: 'ready', label: 'Ready', render: (item) => `${item.status?.readyReplicas || 0}/${item.status?.replicas || 0}`, getValue: (item) => item.status?.readyReplicas || 0 },
        { key: 'age', label: 'Age', render: (item) => formatAge(item.metadata?.creationTimestamp), getValue: (item) => item.metadata?.creationTimestamp },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item) => (
                <DeploymentActionsMenu
                    deployment={item}
                    isOpen={activeMenuId === `deployment-${item.metadata.uid}`}
                    menuPosition={menuPosition}
                    onOpenChange={(isOpen, buttonElement) => handleMenuOpenChange(isOpen, `deployment-${item.metadata.uid}`, buttonElement)}
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
    ], [activeMenuId, menuPosition, handleMenuOpenChange, handleEditYaml, handleShowDependencies, handleRestart, handleDelete, handleViewLogs, podsLoading, allPods]);

    return (
        <ResourceList
            title="Deployments"
            columns={columns}
            data={deployments}
            isLoading={deploymentsLoading}
            namespaces={namespaces}
            currentNamespace={selectedNamespaces}
            onNamespaceChange={setSelectedNamespaces}
            showNamespaceSelector={true}
            multiSelectNamespaces={true}
            highlightedUid={activeMenuId}
            initialSort={{ key: 'age', direction: 'desc' }}
            resourceType="deployments"
            onRowClick={handleShowDetails}
        />
    );
}
