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
import { useSelection } from '~/hooks/useSelection';
import { useBulkActions } from '~/hooks/useBulkActions';
import { DeleteReplicaSet, GetReplicaSetYaml } from 'wailsjs/go/main/App';
import { formatAge } from '~/utils/formatting';
import { getOwnerViewId } from '~/utils/owner-navigation';
import { useMenuPosition } from '~/hooks/useMenuPosition';

// Get controller from owner references
function getController(item) {
    const owners = item.metadata?.ownerReferences || [];
    const controller = owners.find(owner => owner.controller);
    return controller ? { kind: controller.kind, name: controller.name, uid: controller.uid, apiVersion: controller.apiVersion } : null;
}

export default function ReplicaSetList({ isVisible }) {
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
        bulkActionModal,
        bulkProgress,
        openBulkDelete,
        closeBulkAction,
        confirmBulkAction,
        exportYaml,
    } = useBulkActions({
        resourceLabel: 'ReplicaSet',
        resourceType: 'replicasets',
        isNamespaced: true,
        deleteApi: DeleteReplicaSet,
        getYamlApi: GetReplicaSetYaml,

    });
    const { replicaSets, loading } = useReplicaSets(currentContext, selectedNamespaces, isVisible);
    const { handleShowDetails, handleEditYaml, handleShowDependencies, handleViewLogs } = useReplicaSetActions();

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item) => item.metadata?.name, getValue: (item) => item.metadata?.name, initialSort: 'asc' },
        { key: 'namespace', label: 'Namespace', render: (item) => item.metadata?.namespace, getValue: (item) => item.metadata?.namespace },
        {
            key: 'desired',
            label: 'Desired',
            render: (item) => item.spec?.replicas || 0,
            getValue: (item) => item.spec?.replicas || 0
        },
        {
            key: 'current',
            label: 'Current',
            render: (item) => item.status?.replicas || 0,
            getValue: (item) => item.status?.replicas || 0
        },
        {
            key: 'ready',
            label: 'Ready',
            render: (item) => item.status?.readyReplicas || 0,
            getValue: (item) => item.status?.readyReplicas || 0
        },
        { key: 'age', label: 'Age', render: (item) => formatAge(item.metadata?.creationTimestamp), getValue: (item) => item.metadata?.creationTimestamp },
        {
            key: 'controlledBy',
            label: 'Controlled By',
            render: (item) => {
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
            getValue: (item) => getController(item)?.kind || ''
        },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item) => (
                <ReplicaSetActionsMenu
                    replicaSet={item}
                    isOpen={activeMenuId === `rs-${item.metadata.uid}`}
                    menuPosition={menuPosition}
                    onOpenChange={(isOpen, buttonElement) => handleMenuOpenChange(isOpen, `rs-${item.metadata.uid}`, buttonElement)}
                    onEditYaml={() => handleEditYaml(item)}
                    onShowDependencies={() => handleShowDependencies(item)}
                    onDelete={() => openBulkDelete([item])}
                    onViewLogs={() => handleViewLogs(item)}
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
                isOpen={bulkActionModal.isOpen}
                onClose={closeBulkAction}
                action={bulkActionModal.action}
                actionLabel="Delete"
                items={bulkActionModal.items}
                onConfirm={confirmBulkAction}
                onExportYaml={exportYaml}
                progress={bulkProgress}
            />
        </>
    );
}
