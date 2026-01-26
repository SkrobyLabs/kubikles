import React, { useMemo, useState, useCallback } from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ResourceList from '../../../components/shared/ResourceList';
import BulkActionModal from '../../../components/shared/BulkActionModal';
import DeploymentActionsMenu from './DeploymentActionsMenu';
import { useDeployments, usePods } from '../../../hooks/resources';
import { useDeploymentActions } from './useDeploymentActions';
import { useK8s } from '../../../context/K8sContext';
import { useMenu } from '../../../context/MenuContext';
import { useSelection } from '../../../hooks/useSelection';
import { useBulkActions } from '../../../hooks/useBulkActions';
import { DeleteDeployment, RestartDeployment, GetDeploymentYaml } from '../../../../wailsjs/go/main/App';
import { formatAge } from '../../../utils/formatting';
import { getEffectivePodStatus, getPodStatusColor } from '../../../utils/k8s-helpers';
import { useMenuPosition } from '../../../hooks/useMenuPosition';

export default function DeploymentList({ isVisible }) {
    const { currentContext, selectedNamespaces, setSelectedNamespaces, namespaces } = useK8s();
    const { activeMenuId, menuPosition, handleMenuOpenChange } = useMenuPosition();
    const selection = useSelection();

    // Unified bulk actions (also used for single delete/restart)
    const {
        bulkActionModal,
        bulkProgress,
        openBulkDelete,
        openBulkRestart,
        closeBulkAction,
        confirmBulkAction,
        exportYaml,
    } = useBulkActions({
        resourceLabel: 'Deployment',
        resourceType: 'deployments',
        isNamespaced: true,
        deleteApi: DeleteDeployment,
        restartApi: RestartDeployment,
        getYamlApi: GetDeploymentYaml,
        currentContext,
    });
    // console.log("DeploymentList rendering");
    const { deployments, loading: deploymentsLoading } = useDeployments(currentContext, selectedNamespaces, isVisible);
    // Defer pods fetch until deployments are loaded to prioritize showing deployment list first
    const deploymentsReady = !deploymentsLoading && deployments.length > 0;
    const { pods: allPods, loading: podsLoading } = usePods(currentContext, selectedNamespaces, isVisible && deploymentsReady); // Fetch pods for status
    const { handleShowDetails, handleEditYaml, handleShowDependencies, handleViewLogs } = useDeploymentActions();

    // Pre-compute deployment -> pods mapping and counts (O(n+m) instead of O(n*m))
    const { podsByDeployment, podCountsByUid } = useMemo(() => {
        const podsMap = new Map();
        const countsMap = new Map();
        if (!deployments || !allPods) return { podsByDeployment: podsMap, podCountsByUid: countsMap };

        for (const deployment of deployments) {
            const selector = deployment.spec?.selector?.matchLabels;
            if (!selector) continue;

            const deploymentKey = `${deployment.metadata?.namespace}/${deployment.metadata?.name}`;
            const matchingPods = allPods.filter(pod => {
                if (pod.metadata.namespace !== deployment.metadata.namespace) return false;
                for (const [key, value] of Object.entries(selector)) {
                    if (pod.metadata.labels?.[key] !== value) return false;
                }
                return true;
            });
            podsMap.set(deploymentKey, matchingPods);
            // Pre-compute count by UID for O(1) sort lookups
            countsMap.set(deployment.metadata?.uid, matchingPods.length);
        }
        return { podsByDeployment: podsMap, podCountsByUid: countsMap };
    }, [deployments, allPods]);

    // Helper to get pods for a deployment from the pre-computed map
    const getPodsForDeployment = useCallback((deployment) => {
        const key = `${deployment.metadata?.namespace}/${deployment.metadata?.name}`;
        return podsByDeployment.get(key) || [];
    }, [podsByDeployment]);

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
                        {getPodsForDeployment(item).map((pod) => {
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
            getValue: (item) => podCountsByUid.get(item.metadata?.uid) ?? 0
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
                    onRestart={() => openBulkRestart([item])}
                    onDelete={() => openBulkDelete([item])}
                    onViewLogs={() => handleViewLogs(item)}
                />
            ),
            isColumnSelector: true,
            disableSort: true
        },
    ], [activeMenuId, menuPosition, handleMenuOpenChange, handleEditYaml, handleShowDependencies, openBulkRestart, openBulkDelete, handleViewLogs, podsLoading, allPods, getPodsForDeployment, podCountsByUid]);

    return (
        <>
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
                selectable={true}
                selection={selection}
                onBulkDelete={openBulkDelete}
                onBulkRestart={openBulkRestart}
            />

            <BulkActionModal
                isOpen={bulkActionModal.isOpen}
                onClose={closeBulkAction}
                action={bulkActionModal.action}
                actionLabel={bulkActionModal.action === 'delete' ? 'Delete' : 'Restart'}
                items={bulkActionModal.items}
                onConfirm={confirmBulkAction}
                onExportYaml={bulkActionModal.action === 'delete' ? exportYaml : null}
                progress={bulkProgress}
            />
        </>
    );
}
