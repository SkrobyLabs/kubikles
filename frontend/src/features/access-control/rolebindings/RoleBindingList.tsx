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

export default function RoleBindingList({ isVisible }: { isVisible: boolean }) {
    const { currentContext, selectedNamespaces, setSelectedNamespaces, namespaces } = useK8s();
    const { activeMenuId, menuPosition, handleMenuOpenChange } = useMenuPosition();
    const { roleBindings, loading } = useRoleBindings(currentContext, selectedNamespaces, isVisible) as any;
    const { handleShowDetails, handleEditYaml } = useRoleBindingActions();
    const selection = useSelection();

    // Unified bulk actions (also used for single delete)
    const {
        bulkModalProps,
        openBulkDelete,
        exportYaml,
    } = useBulkActions({
        resourceLabel: 'RoleBinding',
        resourceType: 'rolebindings',
        isNamespaced: true,
        deleteApi: ((namespace: string, name: string) => DeleteRoleBinding(namespace, name)) as any,
        getYamlApi: GetRoleBindingYaml,

    });

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item: any) => item.metadata?.name, getValue: (item: any) => item.metadata?.name },
        { key: 'namespace', label: 'Namespace', render: (item: any) => item.metadata?.namespace, getValue: (item: any) => item.metadata?.namespace },
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
                <RoleBindingActionsMenu
                    roleBinding={item}
                    isOpen={activeMenuId === `rolebinding-${item.metadata.uid}`}
                    menuPosition={menuPosition}
                    onOpenChange={(isOpen: any, buttonElement: any) => handleMenuOpenChange(isOpen, `rolebinding-${item.metadata.uid}`, buttonElement)}
                    onEditYaml={handleEditYaml}
                    onDelete={(roleBinding: any) => openBulkDelete([roleBinding])}
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
        </>
    );
}
