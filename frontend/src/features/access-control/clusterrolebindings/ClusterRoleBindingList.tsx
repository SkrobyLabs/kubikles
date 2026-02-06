import React, { useMemo, useState, useCallback } from 'react';
import ResourceList from '~/components/shared/ResourceList';
import BulkActionModal from '~/components/shared/BulkActionModal';
import { useClusterRoleBindings } from '~/hooks/resources';
import { useK8s } from '~/context';
import { useSelection } from '~/hooks/useSelection';
import { useBulkActions } from '~/hooks/useBulkActions';
import { DeleteClusterRoleBinding, GetClusterRoleBindingYaml } from 'wailsjs/go/main/App';
import { formatAge } from '~/utils/formatting';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ClusterRoleBindingActionsMenu from './ClusterRoleBindingActionsMenu';
import { useClusterRoleBindingActions } from './useClusterRoleBindingActions';
import { useMenuPosition } from '~/hooks/useMenuPosition';

export default function ClusterRoleBindingList({ isVisible }: { isVisible: boolean }) {
    const { currentContext } = useK8s();
    const { activeMenuId, menuPosition, handleMenuOpenChange } = useMenuPosition();
    const { clusterRoleBindings, loading } = useClusterRoleBindings(currentContext, isVisible) as any;
    const { handleEditYaml } = useClusterRoleBindingActions();
    const selection = useSelection();

    // Unified bulk actions (also used for single delete)
    const {
        bulkActionModal,
        bulkProgress,
        openBulkDelete,
        closeBulkAction,
        confirmBulkAction,
        exportYaml,
    } = useBulkActions({
        resourceLabel: 'ClusterRoleBinding',
        resourceType: 'clusterrolebindings',
        isNamespaced: false,
        deleteApi: (context, name) => DeleteClusterRoleBinding(name),
        getYamlApi: GetClusterRoleBindingYaml,

    });

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item: any) => item.metadata?.name, getValue: (item: any) => item.metadata?.name },
        {
            key: 'roleRef',
            label: 'Role Ref',
            render: (item: any) => {
                const ref = item.roleRef || {};
                return (
                    <span>
                        <span className="text-gray-400">{ref.kind}/</span>
                        {ref.name}
                    </span>
                );
            },
            getValue: (item: any) => `${item.roleRef?.kind}/${item.roleRef?.name}`
        },
        {
            key: 'subjects',
            label: 'Subjects',
            align: 'center',
            render: (item: any) => (item.subjects || []).length,
            getValue: (item: any) => (item.subjects || []).length
        },
        { key: 'age', label: 'Age', render: (item: any) => formatAge(item.metadata?.creationTimestamp), getValue: (item: any) => item.metadata?.creationTimestamp },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item: any) => (
                <ClusterRoleBindingActionsMenu
                    clusterRoleBinding={item}
                    isOpen={activeMenuId === `clusterrolebinding-${item.metadata.uid}`}
                    menuPosition={menuPosition}
                    onOpenChange={(isOpen: any, buttonElement: any) => handleMenuOpenChange(isOpen, `clusterrolebinding-${item.metadata.uid}`, buttonElement)}
                    onEditYaml={handleEditYaml}
                    onDelete={(clusterRoleBinding: any) => openBulkDelete([clusterRoleBinding])}
                />
            ),
            getValue: () => '',
            isColumnSelector: true,
            disableSort: true
        }
    ], [activeMenuId, menuPosition, handleMenuOpenChange, handleEditYaml, openBulkDelete]);

    return (
        <>
            <ResourceList
                title="Cluster Role Bindings"
                columns={columns}
                data={clusterRoleBindings}
                isLoading={loading}
                showNamespaceSelector={false}
                initialSort={{ key: 'age', direction: 'desc' }}
                resourceType="clusterrolebindings"
                onRowClick={handleEditYaml}
                selectable={true}
                selection={selection}
                onBulkDelete={openBulkDelete}
            />
            <BulkActionModal
                isOpen={bulkActionModal.isOpen}
                onClose={closeBulkAction}
                action={bulkActionModal.action || ''}
                actionLabel="Delete"
                items={bulkActionModal.items}
                onConfirm={confirmBulkAction}
                onExportYaml={exportYaml}
                progress={bulkProgress}
            />
        </>
    );
}
