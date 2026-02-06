import React, { useMemo, useState, useCallback } from 'react';
import ResourceList from '~/components/shared/ResourceList';
import BulkActionModal from '~/components/shared/BulkActionModal';
import { useRoles } from '~/hooks/resources';
import { useK8s } from '~/context';
import { useSelection } from '~/hooks/useSelection';
import { useBulkActions } from '~/hooks/useBulkActions';
import { DeleteRole, GetRoleYaml } from 'wailsjs/go/main/App';
import { formatAge } from '~/utils/formatting';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import RoleActionsMenu from './RoleActionsMenu';
import { useRoleActions } from './useRoleActions';
import { useMenuPosition } from '~/hooks/useMenuPosition';

export default function RoleList({ isVisible }) {
    const { currentContext, selectedNamespaces, setSelectedNamespaces, namespaces } = useK8s();
    const { activeMenuId, menuPosition, handleMenuOpenChange } = useMenuPosition();
    const { roles, loading } = useRoles(currentContext, selectedNamespaces, isVisible);
    const { handleEditYaml } = useRoleActions();
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
        resourceLabel: 'Role',
        resourceType: 'roles',
        isNamespaced: true,
        deleteApi: (context, namespace, name) => DeleteRole(namespace, name),
        getYamlApi: GetRoleYaml,

    });

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item) => item.metadata?.name, getValue: (item) => item.metadata?.name },
        { key: 'namespace', label: 'Namespace', render: (item) => item.metadata?.namespace, getValue: (item) => item.metadata?.namespace },
        {
            key: 'rules',
            label: 'Rules',
            align: 'center',
            render: (item) => (item.rules || []).length,
            getValue: (item) => (item.rules || []).length
        },
        { key: 'age', label: 'Age', render: (item) => formatAge(item.metadata?.creationTimestamp), getValue: (item) => item.metadata?.creationTimestamp },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item) => (
                <RoleActionsMenu
                    role={item}
                    isOpen={activeMenuId === `role-${item.metadata.uid}`}
                    menuPosition={menuPosition}
                    onOpenChange={(isOpen, buttonElement) => handleMenuOpenChange(isOpen, `role-${item.metadata.uid}`, buttonElement)}
                    onEditYaml={handleEditYaml}
                    onDelete={(role) => openBulkDelete([role])}
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
                title="Roles"
                columns={columns}
                data={roles}
                isLoading={loading}
                namespaces={namespaces}
                currentNamespace={selectedNamespaces}
                onNamespaceChange={setSelectedNamespaces}
                showNamespaceSelector={true}
                multiSelectNamespaces={true}
                initialSort={{ key: 'age', direction: 'desc' }}
                resourceType="roles"
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
