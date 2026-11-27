import React, { useMemo, useState, useCallback } from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ResourceList from '~/components/shared/ResourceList';
import { useStableCallbacks } from '~/hooks/useStableCallbacks';
import MenuBound from '~/components/shared/MenuBound';
import BulkActionModal from '~/components/shared/BulkActionModal';
import StatefulSetActionsMenu from './StatefulSetActionsMenu';
import { useStatefulSets, usePodsLegacy } from '~/hooks/resources';
import { useStatefulSetActions } from './useStatefulSetActions';
import { useK8s } from '~/context';
import { useMenu } from '~/context';
import { useNotification } from '~/context';
import { useSelection } from '~/hooks/useSelection';
import { useBulkActions } from '~/hooks/useBulkActions';
import { DeleteStatefulSet, RestartStatefulSet, GetStatefulSetYaml, ScaleStatefulSet } from 'wailsjs/go/main/App';
import ScaleModal from '~/components/shared/ScaleModal';
import { formatAge } from '~/utils/formatting';
import { getDeploymentPods, getEffectivePodStatus, getPodStatusColor } from '~/utils/k8s-helpers';
import { defaultViewColumns } from '~/lib/views/viewColumns';
import { useWithFullObject } from '~/hooks/useViewObject';
import { useOverlayAvailability } from '~/hooks/useViewOverlay';
import { createStatefulSetSemantics, statefulSetSemantics } from './semantics';
import { buildStatefulSetViewColumns, STATEFULSET_HIDDEN_BY_DEFAULT, STATEFULSET_PINNED_COLUMNS, STATEFULSET_CARRIER_COLUMNS } from './viewColumns';

const STATEFULSET_INITIAL_SORT = { key: 'age', direction: 'desc' };
const STATEFULSET_VIEW_OPTIONS = { sort: STATEFULSET_INITIAL_SORT, columns: defaultViewColumns('statefulsets', STATEFULSET_HIDDEN_BY_DEFAULT) };
const NO_DATA: any[] = [];

export default function StatefulSetList({ isVisible }: { isVisible: boolean }) {
    const { currentContext, selectedNamespaces, setSelectedNamespaces, namespaces } = useK8s();
    const selection = useSelection();

    const {
        bulkActionModal,
        bulkModalProps,
        openBulkDelete,
        openBulkRestart,
        exportYaml,
    } = useBulkActions({
        resourceLabel: 'StatefulSet',
        resourceType: 'statefulsets',
        isNamespaced: true,
        deleteApi: DeleteStatefulSet,
        restartApi: RestartStatefulSet,
        getYamlApi: GetStatefulSetYaml,

    });
    const { statefulSets, loading: statefulSetsLoading, loadState, view, viewSnapshot } = useStatefulSets(currentContext, selectedNamespaces, isVisible, STATEFULSET_VIEW_OPTIONS) as any;
    // Defer pods fetch until statefulsets are loaded to prioritize showing the list first.
    // In view mode the backend derives the Pods column itself, so the pod list is not fetched.
    const statefulSetsReady = !statefulSetsLoading && statefulSets.length > 0;
    const { pods: allPods, loading: podsLoading } = usePodsLegacy(currentContext, selectedNamespaces, isVisible && statefulSetsReady && !view) as any;
    const { handleShowDetails, handleEditYaml, handleShowDependencies, handleViewLogs } = useStableCallbacks(useStatefulSetActions());
    const { addNotification } = useNotification();
    const [scaleTarget, setScaleTarget] = useState<any>(null);
    const withObject = useWithFullObject(view ?? null);
    // Details and scaling read spec; windowed rows only carry identity and cells.
    const showDetails = useMemo(() => withObject(handleShowDetails), [withObject, handleShowDetails]);
    const startScale = useMemo(() => withObject(setScaleTarget), [withObject]);
    const podsAvailability = useOverlayAvailability(viewSnapshot, 'pods');
    const sem = useMemo(() => createStatefulSetSemantics((item: any) => getDeploymentPods(item, allPods)), [allPods]);

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item: any) => item.metadata?.name, getValue: statefulSetSemantics.name.getValue, initialSort: 'asc' },
        { key: 'namespace', label: 'Namespace', render: (item: any) => item.metadata?.namespace, getValue: statefulSetSemantics.namespace.getValue },
        {
            key: 'pods',
            label: 'Pods',
            filterable: false,
            render: (item: any) => {
                if (podsLoading && allPods.length === 0) {
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
            getValue: sem.pods.getValue
        },
        { key: 'ready', label: 'Ready', render: (item: any) => `${item.status?.readyReplicas || 0}/${item.status?.replicas || 0}`, getValue: statefulSetSemantics.ready.getValue },
        { key: 'age', label: 'Age', render: (item: any) => formatAge(item.metadata?.creationTimestamp), getValue: statefulSetSemantics.age.getValue },
        // Hidden by default columns
        {
            key: 'replicas',
            label: 'Replicas',
            defaultHidden: true,
            render: (item: any) => item.spec?.replicas ?? 1,
            getValue: statefulSetSemantics.replicas.getValue,
        },
        {
            key: 'serviceName',
            label: 'Service Name',
            defaultHidden: true,
            render: (item: any) => item.spec?.serviceName || <span className="text-gray-500">-</span>,
            getValue: statefulSetSemantics.serviceName.getValue,
        },
        {
            key: 'updateStrategy',
            label: 'Update Strategy',
            defaultHidden: true,
            render: (item: any) => item.spec?.updateStrategy?.type || 'RollingUpdate',
            getValue: statefulSetSemantics.updateStrategy.getValue,
        },
        {
            key: 'image',
            label: 'Image',
            defaultHidden: true,
            render: (item: any) => {
                const containers = item.spec?.template?.spec?.containers || [];
                if (containers.length === 0) return '-';
                if (containers.length === 1) return <span title={containers[0].image}>{containers[0].image?.split('/').pop()}</span>;
                return <span title={containers.map((c: any) => c.image).join('\n')}>{containers.length} images</span>;
            },
            getValue: statefulSetSemantics.image.getValue,
        },
        {
            key: 'volumeClaims',
            label: 'Volume Claims',
            defaultHidden: true,
            render: (item: any) => {
                const claims = item.spec?.volumeClaimTemplates || [];
                return claims.length > 0 ? claims.length : <span className="text-gray-500">-</span>;
            },
            getValue: statefulSetSemantics.volumeClaims.getValue,
        },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item: any) => (
                <MenuBound
                    as={StatefulSetActionsMenu}
                    menuId={`statefulset-${item.metadata.uid}`}
                    statefulSet={item}
                    onEditYaml={() => handleEditYaml(item)}
                    onShowDependencies={() => handleShowDependencies(item)}
                    onRestart={() => openBulkRestart?.([ item])}
                    onDelete={() => openBulkDelete([item])}
                    onViewLogs={() => handleViewLogs(item)}
                    onScale={() => setScaleTarget(item)}
                />
            ),
            isColumnSelector: true,
            disableSort: true
        },
    ], [handleEditYaml, handleShowDependencies, openBulkRestart, openBulkDelete, handleViewLogs, podsLoading, allPods, sem]);

    const viewColumns = useMemo(
        () => buildStatefulSetViewColumns({
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
            <ResourceList loadState={loadState}
                title="StatefulSets"
                columns={view ? viewColumns : columns}
                source={view ?? null}
                viewPinnedColumns={STATEFULSET_PINNED_COLUMNS}
                viewCarrierColumns={STATEFULSET_CARRIER_COLUMNS}
                data={view ? NO_DATA : statefulSets}
                isLoading={statefulSetsLoading}
                namespaces={namespaces}
                currentNamespace={selectedNamespaces}
                onNamespaceChange={setSelectedNamespaces}
                showNamespaceSelector={true}
                multiSelectNamespaces={true}
                initialSort={STATEFULSET_INITIAL_SORT}
                resourceType="statefulsets"
                onRowClick={view ? showDetails : handleShowDetails}
                selectable={true}
                selection={selection}
                onBulkDelete={openBulkDelete}
                onBulkRestart={openBulkRestart}
            />

            <BulkActionModal
                {...bulkModalProps}
                action={bulkActionModal.action || ''}
                actionLabel={bulkActionModal.action === 'delete' ? 'Delete' : 'Restart'}
                onExportYaml={bulkActionModal.action === 'delete' ? exportYaml : undefined}
            />

            {scaleTarget && (
                <ScaleModal
                    resourceType="StatefulSet"
                    resourceName={scaleTarget.metadata?.name || ''}
                    namespace={scaleTarget.metadata?.namespace || ''}
                    currentReplicas={scaleTarget.spec?.replicas ?? 1}
                    selector={scaleTarget.spec?.selector?.matchLabels || {}}
                    onScale={async (replicas: number) => {
                        await ScaleStatefulSet(scaleTarget.metadata?.namespace || '', scaleTarget.metadata?.name || '', replicas);
                        addNotification({ type: 'success', message: `Scaled ${scaleTarget.metadata?.name} to ${replicas} replicas` });
                    }}
                    onClose={() => setScaleTarget(null)}
                />
            )}
        </>
    );
}
