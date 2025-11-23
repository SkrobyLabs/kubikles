import React, { useMemo } from 'react';
import { EllipsisHorizontalIcon } from '@heroicons/react/24/outline';
import ResourceList from '../../../components/shared/ResourceList';
import DeploymentActionsMenu from './DeploymentActionsMenu';
import { useDeployments } from '../../../hooks/useDeployments';
import { usePods } from '../../../hooks/usePods';
import { useDeploymentActions } from './useDeploymentActions';
import { useK8s } from '../../../context/K8sContext';
import { useUI } from '../../../context/UIContext';
import { formatAge } from '../../../utils/formatting';
import { getDeploymentPods, getEffectivePodStatus, getPodStatusColor } from '../../../utils/k8s-helpers';

export default function DeploymentList({ isVisible }) {
    const { currentContext, currentNamespace, setCurrentNamespace, namespaces } = useK8s();
    const { activeMenuId, setActiveMenuId } = useUI();
    const { deployments, loading: deploymentsLoading } = useDeployments(currentContext, currentNamespace, isVisible);
    const { pods: allPods, loading: podsLoading } = usePods(currentContext, currentNamespace, isVisible); // Fetch pods for status
    const { handleEditYaml, handleRestart, handleDelete } = useDeploymentActions();

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item) => item.metadata?.name, getValue: (item) => item.metadata?.name, initialSort: 'asc' },
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
            label: <EllipsisHorizontalIcon className="h-5 w-5" />,
            render: (item) => (
                <DeploymentActionsMenu
                    deployment={item}
                    isOpen={activeMenuId === `deploy-${item.metadata.uid}`}
                    onOpenChange={(isOpen) => setActiveMenuId(isOpen ? `deploy-${item.metadata.uid}` : null)}
                    onEditYaml={() => handleEditYaml(item)}
                    onRestart={() => handleRestart(item)}
                    onDelete={() => handleDelete(item)}
                />
            ),
            isColumnSelector: true,
            disableSort: true
        },
    ], [activeMenuId, setActiveMenuId, handleEditYaml, handleRestart, handleDelete, podsLoading, allPods]);

    return (
        <ResourceList
            title="Deployments"
            columns={columns}
            data={deployments}
            isLoading={deploymentsLoading}
            namespaces={namespaces}
            currentNamespace={currentNamespace}
            onNamespaceChange={setCurrentNamespace}
            showNamespaceSelector={true}
            highlightedUid={activeMenuId}
        />
    );
}
