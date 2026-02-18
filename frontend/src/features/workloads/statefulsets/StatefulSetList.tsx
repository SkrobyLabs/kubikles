import React, { useMemo, useState, useCallback } from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ResourceList from '~/components/shared/ResourceList';
import BulkActionModal from '~/components/shared/BulkActionModal';
import StatefulSetActionsMenu from './StatefulSetActionsMenu';
import { useStatefulSets, usePods } from '~/hooks/resources';
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
import { useMenuPosition } from '~/hooks/useMenuPosition';

export default function StatefulSetList({ isVisible }: { isVisible: boolean }) {
    const { currentContext, selectedNamespaces, setSelectedNamespaces, namespaces } = useK8s();
    const { activeMenuId, menuPosition, handleMenuOpenChange } = useMenuPosition();
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
    const { statefulSets, loading: statefulSetsLoading } = useStatefulSets(currentContext, selectedNamespaces, isVisible) as any;
    // Defer pods fetch until statefulsets are loaded to prioritize showing the list first
    const statefulSetsReady = !statefulSetsLoading && statefulSets.length > 0;
    const { pods: allPods, loading: podsLoading } = usePods(currentContext, selectedNamespaces, isVisible && statefulSetsReady) as any;
    const { handleShowDetails, handleEditYaml, handleShowDependencies, handleViewLogs } = useStatefulSetActions();
    const { addNotification } = useNotification();
    const [scaleTarget, setScaleTarget] = useState<any>(null);

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item: any) => item.metadata?.name, getValue: (item: any) => item.metadata?.name, initialSort: 'asc' },
        { key: 'namespace', label: 'Namespace', render: (item: any) => item.metadata?.namespace, getValue: (item: any) => item.metadata?.namespace },
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
            getValue: (item: any) => getDeploymentPods(item, allPods).length
        },
        { key: 'ready', label: 'Ready', render: (item: any) => `${item.status?.readyReplicas || 0}/${item.status?.replicas || 0}`, getValue: (item: any) => item.status?.readyReplicas || 0 },
        { key: 'age', label: 'Age', render: (item: any) => formatAge(item.metadata?.creationTimestamp), getValue: (item: any) => item.metadata?.creationTimestamp },
        // Hidden by default columns
        {
            key: 'replicas',
            label: 'Replicas',
            defaultHidden: true,
            render: (item: any) => item.spec?.replicas ?? 1,
            getValue: (item: any) => item.spec?.replicas ?? 1,
        },
        {
            key: 'serviceName',
            label: 'Service Name',
            defaultHidden: true,
            render: (item: any) => item.spec?.serviceName || <span className="text-gray-500">-</span>,
            getValue: (item: any) => item.spec?.serviceName || '',
        },
        {
            key: 'updateStrategy',
            label: 'Update Strategy',
            defaultHidden: true,
            render: (item: any) => item.spec?.updateStrategy?.type || 'RollingUpdate',
            getValue: (item: any) => item.spec?.updateStrategy?.type || 'RollingUpdate',
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
            getValue: (item: any) => item.spec?.template?.spec?.containers?.[0]?.image || '',
        },
        {
            key: 'volumeClaims',
            label: 'Volume Claims',
            defaultHidden: true,
            render: (item: any) => {
                const claims = item.spec?.volumeClaimTemplates || [];
                return claims.length > 0 ? claims.length : <span className="text-gray-500">-</span>;
            },
            getValue: (item: any) => (item.spec?.volumeClaimTemplates || []).length,
        },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item: any) => (
                <StatefulSetActionsMenu
                    statefulSet={item}
                    isOpen={activeMenuId === `statefulset-${item.metadata.uid}`}
                    menuPosition={menuPosition}
                    onOpenChange={(isOpen: any, buttonElement: any) => handleMenuOpenChange(isOpen, `statefulset-${item.metadata.uid}`, buttonElement)}
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
    ], [activeMenuId, menuPosition, handleMenuOpenChange, handleEditYaml, handleShowDependencies, openBulkRestart, openBulkDelete, handleViewLogs, podsLoading, allPods]);

    return (
        <>
            <ResourceList
                title="StatefulSets"
                columns={columns}
                data={statefulSets}
                isLoading={statefulSetsLoading}
                namespaces={namespaces}
                currentNamespace={selectedNamespaces}
                onNamespaceChange={setSelectedNamespaces}
                showNamespaceSelector={true}
                multiSelectNamespaces={true}
                highlightedUid={activeMenuId}
                initialSort={{ key: 'age', direction: 'desc' }}
                resourceType="statefulsets"
                onRowClick={handleShowDetails}
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
