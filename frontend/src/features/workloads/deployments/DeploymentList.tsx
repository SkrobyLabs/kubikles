import React, { useMemo, useState, useCallback } from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ResourceList from '~/components/shared/ResourceList';
import { useStableCallbacks } from '~/hooks/useStableCallbacks';
import MenuBound from '~/components/shared/MenuBound';
import BulkActionModal from '~/components/shared/BulkActionModal';
import DeploymentActionsMenu from './DeploymentActionsMenu';
import { useDeployments, usePodsLegacy } from '~/hooks/resources';
import type { K8sDeployment, K8sPod } from '~/types/k8s';
import { useDeploymentActions } from './useDeploymentActions';
import { useK8s } from '~/context';
import { useNotification } from '~/context';
import { useSelection } from '~/hooks/useSelection';
import { useBulkActions } from '~/hooks/useBulkActions';
import { DeleteDeployment, RestartDeployment, GetDeploymentYaml, ScaleDeployment } from 'wailsjs/go/main/App';
import ScaleModal from '~/components/shared/ScaleModal';
import { formatAge } from '~/utils/formatting';
import { getEffectivePodStatus, getPodStatusColor } from '~/utils/k8s-helpers';
import { defaultViewColumns } from '~/lib/views/viewColumns';
import { useWithFullObject } from '~/hooks/useViewObject';
import { useOverlayAvailability } from '~/hooks/useViewOverlay';
import { createDeploymentSemantics, deploymentSemantics } from './semantics';
import { deploymentPodMaps, deploymentPodsOf } from '../podDerivation';
import { buildDeploymentViewColumns, DEPLOYMENT_HIDDEN_BY_DEFAULT, DEPLOYMENT_PINNED_COLUMNS, DEPLOYMENT_CARRIER_COLUMNS } from './viewColumns';

const DEPLOYMENT_INITIAL_SORT = { key: 'age', direction: 'desc' };
const DEPLOYMENT_VIEW_OPTIONS = { sort: DEPLOYMENT_INITIAL_SORT, columns: defaultViewColumns('deployments', DEPLOYMENT_HIDDEN_BY_DEFAULT) };
const NO_DATA: K8sDeployment[] = [];

type DeploymentListProps = {
    isVisible: boolean
}

export default function DeploymentList({ isVisible }: DeploymentListProps) {
    const { currentContext, selectedNamespaces, setSelectedNamespaces, namespaces } = useK8s();
    const selection = useSelection();

    // Unified bulk actions (also used for single delete/restart)
    const {
        bulkActionModal,
        bulkModalProps,
        openBulkDelete,
        openBulkRestart,
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
    const { deployments, loading: deploymentsLoading, loadState, view, viewSnapshot } = useDeployments(currentContext, selectedNamespaces, isVisible, DEPLOYMENT_VIEW_OPTIONS) as unknown as {
        deployments: K8sDeployment[];
        loadState: import('~/hooks/useResource').ResourceLoadState;
        loading: boolean;
        view: import('~/lib/views/viewClient').ViewHandle | null;
        viewSnapshot: import('~/lib/views/viewClient').ViewSnapshot | null;
    };
    // Defer pods fetch until deployments are loaded to prioritize showing deployment list first.
    // In view mode the backend derives the Pods column itself, so the pod list is not fetched.
    const deploymentsReady = !deploymentsLoading && deployments.length > 0;
    const { pods: allPods, loading: podsLoading } = usePodsLegacy(currentContext, selectedNamespaces, isVisible && deploymentsReady && !view) as unknown as {
        pods: K8sPod[];
        loading: boolean;
    };
    const { handleShowDetails, handleEditYaml, handleShowDependencies, handleViewLogs } = useStableCallbacks(useDeploymentActions());
    const { addNotification } = useNotification();
    const [scaleTarget, setScaleTarget] = useState<K8sDeployment | null>(null);
    const withObject = useWithFullObject(view ?? null);
    // Details and scaling read spec; windowed rows only carry identity and cells.
    const showDetails = useMemo(() => withObject(handleShowDetails as any), [withObject, handleShowDetails]);
    const startScale = useMemo(() => withObject(setScaleTarget as any), [withObject]);
    const podsAvailability = useOverlayAvailability(viewSnapshot, 'pods');

    // Pre-compute deployment -> pods mapping and counts (O(n+m) instead of O(n*m))
    const podMaps = useMemo(() => deploymentPodMaps(deployments, allPods), [deployments, allPods]);
    const sem = useMemo(() => createDeploymentSemantics(podMaps), [podMaps]);

    // Helper to get pods for a deployment from the pre-computed map
    const getPodsForDeployment = useCallback((deployment: K8sDeployment): K8sPod[] => deploymentPodsOf(podMaps, deployment), [podMaps]);

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item: K8sDeployment) => item.metadata?.name, getValue: deploymentSemantics.name.getValue, initialSort: 'asc' },
        { key: 'namespace', label: 'Namespace', render: (item: K8sDeployment) => item.metadata?.namespace, getValue: deploymentSemantics.namespace.getValue },
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
            getValue: sem.pods.getValue
        },
        { key: 'ready', label: 'Ready', render: (item: K8sDeployment) => `${item.status?.readyReplicas || 0}/${item.status?.replicas || 0}`, getValue: deploymentSemantics.ready.getValue },
        { key: 'age', label: 'Age', render: (item: K8sDeployment) => formatAge(item.metadata?.creationTimestamp), getValue: deploymentSemantics.age.getValue },
        // Hidden by default columns
        {
            key: 'strategy',
            label: 'Strategy',
            defaultHidden: true,
            render: (item: K8sDeployment) => item.spec?.strategy?.type || 'RollingUpdate',
            getValue: deploymentSemantics.strategy.getValue,
        },
        {
            key: 'replicas',
            label: 'Replicas',
            defaultHidden: true,
            render: (item: K8sDeployment) => item.spec?.replicas ?? 1,
            getValue: deploymentSemantics.replicas.getValue,
        },
        {
            key: 'updatedReplicas',
            label: 'Updated',
            defaultHidden: true,
            render: (item: K8sDeployment) => item.status?.updatedReplicas ?? 0,
            getValue: deploymentSemantics.updatedReplicas.getValue,
        },
        {
            key: 'availableReplicas',
            label: 'Available',
            defaultHidden: true,
            render: (item: K8sDeployment) => item.status?.availableReplicas ?? 0,
            getValue: deploymentSemantics.availableReplicas.getValue,
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
            getValue: deploymentSemantics.image.getValue,
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
            getValue: deploymentSemantics.selector.getValue,
        },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item: K8sDeployment) => (
                <MenuBound
                    as={DeploymentActionsMenu}
                    menuId={`deployment-${item.metadata.uid}`}
                    deployment={item}
                    onEditYaml={() => handleEditYaml(item)}
                    onShowDependencies={() => handleShowDependencies?.(item)}
                    onRestart={() => openBulkRestart?.([item])}
                    onDelete={() => openBulkDelete([item])}
                    onViewLogs={() => handleViewLogs(item)}
                    onScale={() => setScaleTarget(item)}
                />
            ),
            isColumnSelector: true,
            disableSort: true
        },
    ], [handleEditYaml, handleShowDependencies, openBulkRestart, openBulkDelete, handleViewLogs, podsLoading, allPods, getPodsForDeployment, sem]);

    const viewColumns = useMemo(
        () => buildDeploymentViewColumns({
            podsAvailability,
            onEditYaml: handleEditYaml as (item: any) => void,
            onShowDependencies: handleShowDependencies as (item: any) => void,
            onRestart: item => openBulkRestart?.([item]),
            onDelete: item => openBulkDelete([item]),
            onViewLogs: handleViewLogs as (item: any) => void,
            onScale: startScale,
        }),
        [podsAvailability, handleEditYaml, handleShowDependencies, openBulkRestart, openBulkDelete, handleViewLogs, startScale]);

    return (
        <>
            {/* eslint-disable-next-line @typescript-eslint/no-explicit-any */}
            <ResourceList loadState={loadState}
                title="Deployments"
                columns={(view ? viewColumns : columns) as any}
                source={view ?? null}
                viewPinnedColumns={DEPLOYMENT_PINNED_COLUMNS}
                viewCarrierColumns={DEPLOYMENT_CARRIER_COLUMNS}
                data={view ? NO_DATA : deployments}
                isLoading={deploymentsLoading}
                namespaces={namespaces as any}
                currentNamespace={selectedNamespaces}
                onNamespaceChange={setSelectedNamespaces}
                showNamespaceSelector={true}
                multiSelectNamespaces={true}
                initialSort={DEPLOYMENT_INITIAL_SORT as any}
                resourceType={"deployments" as any}
                onRowClick={(view ? showDetails : handleShowDetails) as any}
                selectable={true}
                selection={selection as any}
                onBulkDelete={openBulkDelete as any}
                onBulkRestart={openBulkRestart as any}
            />

            <BulkActionModal
                {...bulkModalProps}
                action={bulkActionModal.action || ''}
                actionLabel={bulkActionModal.action === 'delete' ? 'Delete' : 'Restart'}
                onExportYaml={bulkActionModal.action === 'delete' ? exportYaml : undefined}
            />

            {scaleTarget && (
                <ScaleModal
                    resourceType="Deployment"
                    resourceName={scaleTarget.metadata?.name || ''}
                    namespace={scaleTarget.metadata?.namespace || ''}
                    currentReplicas={scaleTarget.spec?.replicas ?? 1}
                    selector={scaleTarget.spec?.selector?.matchLabels || {}}
                    onScale={async (replicas: number) => {
                        await ScaleDeployment(scaleTarget.metadata?.namespace || '', scaleTarget.metadata?.name || '', replicas);
                        addNotification({ type: 'success', message: `Scaled ${scaleTarget.metadata?.name} to ${replicas} replicas` });
                    }}
                    onClose={() => setScaleTarget(null)}
                />
            )}
        </>
    );
}
