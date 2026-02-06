import React, { useMemo, useState, useCallback } from 'react';
import ResourceList from '~/components/shared/ResourceList';
import BulkActionModal from '~/components/shared/BulkActionModal';
import { useRoleBindings } from '~/hooks/resources';
import { useK8s } from '~/context';
import { useSelection } from '~/hooks/useSelection';
import { useBulkActions } from '~/hooks/useBulkActions';
import { DeleteRoleBinding, GetRoleBindingYaml } from 'wailsjs/go/main/App';
import { formatAge } from '~/utils/formatting';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import RoleBindingActionsMenu from './RoleBindingActionsMenu';
import { useRoleBindingActions } from './useRoleBindingActions';
import { useMenuPosition } from '~/hooks/useMenuPosition';

export default function RoleBindingList({ isVisible }) {
    const { currentContext, selectedNamespaces, setSelectedNamespaces, namespaces } = useK8s();
    const { activeMenuId, menuPosition, handleMenuOpenChange } = useMenuPosition();
    const { roleBindings, loading } = useRoleBindings(currentContext, selectedNamespaces, isVisible);
    const { handleEditYaml } = useRoleBindingActions();
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
        resourceLabel: 'RoleBinding',
        resourceType: 'rolebindings',
        isNamespaced: true,
        deleteApi: (context, namespace, name) => DeleteRoleBinding(namespace, name),
        getYamlApi: GetRoleBindingYaml,

    });

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item) => item.metadata?.name, getValue: (item) => item.metadata?.name },
        { key: 'namespace', label: 'Namespace', render: (item) => item.metadata?.namespace, getValue: (item) => item.metadata?.namespace },
        {
            key: 'roleRef',
            label: 'Role Ref',
            render: (item) => {
                const ref = item.roleRef || {};
                return (
                    <span>
                        <span className="text-gray-400">{ref.kind}/</span>
                        {ref.name}
                    </span>
                );
            },
            getValue: (item) => `${item.roleRef?.kind}/${item.roleRef?.name}`
        },
        {
            key: 'subjects',
            label: 'Subjects',
            align: 'center',
            render: (item) => (item.subjects || []).length,
            getValue: (item) => (item.subjects || []).length
        },
        { key: 'age', label: 'Age', render: (item) => formatAge(item.metadata?.creationTimestamp), getValue: (item) => item.metadata?.creationTimestamp },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item) => (
                <RoleBindingActionsMenu
                    roleBinding={item}
                    isOpen={activeMenuId === `rolebinding-${item.metadata.uid}`}
                    menuPosition={menuPosition}
                    onOpenChange={(isOpen, buttonElement) => handleMenuOpenChange(isOpen, `rolebinding-${item.metadata.uid}`, buttonElement)}
                    onEditYaml={handleEditYaml}
                    onDelete={(roleBinding) => openBulkDelete([roleBinding])}
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
                title="Role Bindings"
                columns={columns}
                data={roleBindings}
                isLoading={loading}
                namespaces={namespaces}
                currentNamespace={selectedNamespaces}
                onNamespaceChange={setSelectedNamespaces}
                showNamespaceSelector={true}
                multiSelectNamespaces={true}
                initialSort={{ key: 'age', direction: 'desc' }}
                resourceType="rolebindings"
                onRowClick={handleEditYaml}
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
