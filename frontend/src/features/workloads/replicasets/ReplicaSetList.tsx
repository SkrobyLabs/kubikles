import React, { useMemo, useState, useCallback, useEffect } from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ResourceList from '~/components/shared/ResourceList';
import BulkActionModal from '~/components/shared/BulkActionModal';
import ReplicaSetActionsMenu from './ReplicaSetActionsMenu';
import { useReplicaSets } from '~/hooks/resources';
import { useReplicaSetActions } from './useReplicaSetActions';
import { useK8s } from '~/context';
import { useUI } from '~/context';
import { useMenu } from '~/context';
import { useNotification } from '~/context';
import { useSelection } from '~/hooks/useSelection';
import { useBulkActions } from '~/hooks/useBulkActions';
import { DeleteReplicaSet, GetReplicaSetYaml, ScaleReplicaSet } from 'wailsjs/go/main/App';
import ScaleModal from '~/components/shared/ScaleModal';
import { formatAge } from '~/utils/formatting';
import { getOwnerViewId } from '~/utils/owner-navigation';
import { useMenuPosition } from '~/hooks/useMenuPosition';

// Get controller from owner references
function getController(item: any) {
    const owners = item.metadata?.ownerReferences || [];
    const controller = owners.find((owner: any) => owner.controller);
    return controller ? { kind: controller.kind, name: controller.name, uid: controller.uid, apiVersion: controller.apiVersion } : null;
}

export default function ReplicaSetList({ isVisible }: { isVisible: boolean }) {
    const { currentContext, selectedNamespaces, setSelectedNamespaces, namespaces, crds, ensureCRDsLoaded } = useK8s();
    const { navigateWithSearch } = useUI();

    // Load CRDs for owner reference resolution (lazy load)
    useEffect(() => {
        if (isVisible) {
            ensureCRDsLoaded();
        }
    }, [isVisible, ensureCRDsLoaded]);
    const { activeMenuId, menuPosition, handleMenuOpenChange } = useMenuPosition();
    const selection = useSelection();

    const {
        bulkModalProps,
        openBulkDelete,
        exportYaml,
    } = useBulkActions({
        resourceLabel: 'ReplicaSet',
        resourceType: 'replicasets',
        isNamespaced: true,
        deleteApi: DeleteReplicaSet,
        getYamlApi: GetReplicaSetYaml,

    });
    const { replicaSets, loading } = useReplicaSets(currentContext, selectedNamespaces, isVisible) as any;
    const { handleShowDetails, handleEditYaml, handleShowDependencies, handleViewLogs } = useReplicaSetActions();
    const { addNotification } = useNotification();
    const [scaleTarget, setScaleTarget] = useState<any>(null);

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item: any) => item.metadata?.name, getValue: (item: any) => item.metadata?.name, initialSort: 'asc' },
        { key: 'namespace', label: 'Namespace', render: (item: any) => item.metadata?.namespace, getValue: (item: any) => item.metadata?.namespace },
        {
            key: 'desired',
            label: 'Desired',
            render: (item: any) => item.spec?.replicas || 0,
            getValue: (item: any) => item.spec?.replicas || 0
        },
        {
            key: 'current',
            label: 'Current',
            render: (item: any) => item.status?.replicas || 0,
            getValue: (item: any) => item.status?.replicas || 0
        },
        {
            key: 'ready',
            label: 'Ready',
            render: (item: any) => item.status?.readyReplicas || 0,
            getValue: (item: any) => item.status?.readyReplicas || 0
        },
        { key: 'age', label: 'Age', render: (item: any) => formatAge(item.metadata?.creationTimestamp), getValue: (item: any) => item.metadata?.creationTimestamp },
        {
            key: 'controlledBy',
            label: 'Controlled By',
            render: (item: any) => {
                const controller = getController(item);
                if (!controller) {
                    return <span className="text-gray-600">-</span>;
                }

                const viewId = getOwnerViewId(controller, crds);

                if (viewId) {
                    return (
                        <button
                            onClick={(e) => {
                                e.stopPropagation();
                                navigateWithSearch(viewId, `uid:"${controller.uid}"`);
                            }}
                            className="text-primary hover:text-primary/80 hover:underline transition-colors"
                            title={`Go to ${controller.kind}: ${controller.name}`}
                        >
                            {controller.kind}
                        </button>
                    );
                }

                return (
                    <span className="text-gray-400" title={controller.name}>
                        {controller.kind}
                    </span>
                );
            },
            getValue: (item: any) => getController(item)?.kind || ''
        },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item: any) => (
                <ReplicaSetActionsMenu
                    replicaSet={item}
                    isOpen={activeMenuId === `rs-${item.metadata.uid}`}
                    menuPosition={menuPosition}
                    onOpenChange={(isOpen: any, buttonElement: any) => handleMenuOpenChange(isOpen, `rs-${item.metadata.uid}`, buttonElement)}
                    onEditYaml={() => handleEditYaml(item)}
                    onShowDependencies={() => handleShowDependencies(item)}
                    onDelete={() => openBulkDelete([item])}
                    onViewLogs={() => handleViewLogs(item)}
                    onScale={() => setScaleTarget(item)}
                />
            ),
            isColumnSelector: true,
            disableSort: true
        },
    ], [activeMenuId, menuPosition, handleMenuOpenChange, handleEditYaml, handleShowDependencies, openBulkDelete, handleViewLogs, navigateWithSearch, crds]);

    return (
        <>
            <ResourceList
                title="ReplicaSets"
                columns={columns}
                data={replicaSets}
                isLoading={loading}
                namespaces={namespaces}
                currentNamespace={selectedNamespaces}
                onNamespaceChange={setSelectedNamespaces}
                showNamespaceSelector={true}
                multiSelectNamespaces={true}
                highlightedUid={activeMenuId}
                initialSort={{ key: 'age', direction: 'desc' }}
                resourceType="replicasets"
                onRowClick={handleShowDetails}
                selectable={true}
                selection={selection}
                onBulkDelete={openBulkDelete}
            />
            <BulkActionModal
                {...bulkModalProps}
                action="delete"
                actionLabel="Delete"
                onExportYaml={exportYaml}
            />

            {scaleTarget && (
                <ScaleModal
                    resourceType="ReplicaSet"
                    resourceName={scaleTarget.metadata?.name || ''}
                    namespace={scaleTarget.metadata?.namespace || ''}
                    currentReplicas={scaleTarget.spec?.replicas ?? 1}
                    selector={scaleTarget.spec?.selector?.matchLabels || {}}
                    onScale={async (replicas: number) => {
                        await ScaleReplicaSet(scaleTarget.metadata?.namespace || '', scaleTarget.metadata?.name || '', replicas);
                        addNotification({ type: 'success', message: `Scaled ${scaleTarget.metadata?.name} to ${replicas} replicas` });
                    }}
                    onClose={() => setScaleTarget(null)}
                />
            )}
        </>
    );
}
