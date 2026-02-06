import React, { useMemo, useState, useCallback } from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ResourceList from '~/components/shared/ResourceList';
import BulkActionModal from '~/components/shared/BulkActionModal';
import DeploymentActionsMenu from './DeploymentActionsMenu';
import { useDeployments, usePods } from '~/hooks/resources';
import type { K8sDeployment, K8sPod } from '~/types/k8s';
import { useDeploymentActions } from './useDeploymentActions';
import { useK8s } from '~/context';
import { useSelection } from '~/hooks/useSelection';
import { useBulkActions } from '~/hooks/useBulkActions';
import { DeleteDeployment, RestartDeployment, GetDeploymentYaml } from 'wailsjs/go/main/App';
import { formatAge } from '~/utils/formatting';
import { getEffectivePodStatus, getPodStatusColor } from '~/utils/k8s-helpers';
import { useMenuPosition } from '~/hooks/useMenuPosition';

type DeploymentListProps = {
    isVisible: boolean
}

export default function DeploymentList({ isVisible }: DeploymentListProps) {
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
    });
    // console.log("DeploymentList rendering");
    const { deployments, loading: deploymentsLoading } = useDeployments(currentContext, selectedNamespaces, isVisible) as unknown as {
        deployments: K8sDeployment[];
        loading: boolean;
    };
    // Defer pods fetch until deployments are loaded to prioritize showing deployment list first
    const deploymentsReady = !deploymentsLoading && deployments.length > 0;
    const { pods: allPods, loading: podsLoading } = usePods(currentContext, selectedNamespaces, isVisible && deploymentsReady) as unknown as {
        pods: K8sPod[];
        loading: boolean;
    };
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
            const matchingPods = allPods.filter((pod: any) => {
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
    const getPodsForDeployment = useCallback((deployment: K8sDeployment): K8sPod[] => {
        const key = `${deployment.metadata?.namespace}/${deployment.metadata?.name}`;
        return podsByDeployment.get(key) || [];
    }, [podsByDeployment]);

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item: K8sDeployment) => item.metadata?.name, getValue: (item: K8sDeployment) => item.metadata?.name, initialSort: 'asc' },
        { key: 'namespace', label: 'Namespace', render: (item: K8sDeployment) => item.metadata?.namespace, getValue: (item: K8sDeployment) => item.metadata?.namespace },
        {
            key: 'pods',
            label: 'Pods',
            filterable: false,
            render: (item: K8sDeployment) => {
                if (podsLoading && allPods.length === 0) { // Only show loading if we have no pods yet
                    // Show placeholders based on replicas count
                    const count = item.spec?.replicas ?? 1;
                    if (count === 0) return null;
                    return (
                        <div className="flex gap-1">
                            {Array.from({ length: Math.min(count, 5) }).map((_: any, i: number) => (
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
            getValue: (item: K8sDeployment) => podCountsByUid.get(item.metadata?.uid) ?? 0
        },
        { key: 'ready', label: 'Ready', render: (item: K8sDeployment) => `${item.status?.readyReplicas || 0}/${item.status?.replicas || 0}`, getValue: (item: K8sDeployment) => item.status?.readyReplicas || 0 },
        { key: 'age', label: 'Age', render: (item: K8sDeployment) => formatAge(item.metadata?.creationTimestamp), getValue: (item: K8sDeployment) => item.metadata?.creationTimestamp },
        // Hidden by default columns
        {
            key: 'strategy',
            label: 'Strategy',
            defaultHidden: true,
            render: (item: K8sDeployment) => item.spec?.strategy?.type || 'RollingUpdate',
            getValue: (item: K8sDeployment) => item.spec?.strategy?.type || 'RollingUpdate',
        },
        {
            key: 'replicas',
            label: 'Replicas',
            defaultHidden: true,
            render: (item: K8sDeployment) => item.spec?.replicas ?? 1,
            getValue: (item: K8sDeployment) => item.spec?.replicas ?? 1,
        },
        {
            key: 'updatedReplicas',
            label: 'Updated',
            defaultHidden: true,
            render: (item: K8sDeployment) => item.status?.updatedReplicas ?? 0,
            getValue: (item: K8sDeployment) => item.status?.updatedReplicas ?? 0,
        },
        {
            key: 'availableReplicas',
            label: 'Available',
            defaultHidden: true,
            render: (item: K8sDeployment) => item.status?.availableReplicas ?? 0,
            getValue: (item: K8sDeployment) => item.status?.availableReplicas ?? 0,
        },
        {
            key: 'image',
            label: 'Image',
            defaultHidden: true,
            render: (item: K8sDeployment) => {
                const containers = item.spec?.template?.spec?.containers || [];
                if (containers.length === 0) return '-';
                if (containers.length === 1) return <span title={containers[0].image}>{containers[0].image?.split('/').pop()}</span>;
                return <span title={containers.map((c: any) => c.image).join('\n')}>{containers.length} images</span>;
            },
            getValue: (item: K8sDeployment) => item.spec?.template?.spec?.containers?.[0]?.image || '',
        },
        {
            key: 'selector',
            label: 'Selector',
            defaultHidden: true,
            render: (item: K8sDeployment) => {
                const labels = item.spec?.selector?.matchLabels || {};
                const entries = Object.entries(labels);
                if (entries.length === 0) return '-';
                return <span title={entries.map(([k, v]) => `${k}=${v}`).join('\n')}>{entries.length} label{entries.length > 1 ? 's' : ''}</span>;
            },
            getValue: (item: K8sDeployment) => Object.entries(item.spec?.selector?.matchLabels || {}).map(([k, v]) => `${k}=${v}`).join(','),
        },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item: K8sDeployment) => (
                <DeploymentActionsMenu
                    deployment={item}
                    isOpen={activeMenuId === `deployment-${item.metadata.uid}`}
                    menuPosition={menuPosition}
                    onOpenChange={(isOpen: any, buttonElement: any) => handleMenuOpenChange(isOpen, `deployment-${item.metadata.uid}`, buttonElement)}
                    onEditYaml={() => handleEditYaml(item)}
                    onShowDependencies={() => handleShowDependencies?.(item)}
                    onRestart={() => openBulkRestart?.([item])}
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
            {/* eslint-disable-next-line @typescript-eslint/no-explicit-any */}
            <ResourceList
                title="Deployments"
                columns={columns as any}
                data={deployments}
                isLoading={deploymentsLoading}
                namespaces={namespaces as any}
                currentNamespace={selectedNamespaces}
                onNamespaceChange={setSelectedNamespaces}
                showNamespaceSelector={true}
                multiSelectNamespaces={true}
                highlightedUid={activeMenuId as any}
                initialSort={{ key: 'age', direction: 'desc' } as any}
                resourceType={"deployments" as any}
                onRowClick={handleShowDetails as any}
                selectable={true}
                selection={selection as any}
                onBulkDelete={openBulkDelete as any}
                onBulkRestart={openBulkRestart as any}
            />

            <BulkActionModal
                isOpen={bulkActionModal.isOpen}
                onClose={closeBulkAction}
                action={bulkActionModal.action || ''}
                actionLabel={bulkActionModal.action === 'delete' ? 'Delete' : 'Restart'}
                items={bulkActionModal.items as any}
                onConfirm={confirmBulkAction as any}
                onExportYaml={bulkActionModal.action === 'delete' ? exportYaml : undefined}
                progress={bulkProgress as any}
            />
        </>
    );
}
