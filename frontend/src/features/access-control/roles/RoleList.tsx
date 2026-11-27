import React, { useMemo, useState, useCallback } from 'react';
import ResourceList from '~/components/shared/ResourceList';
import { useStableCallbacks } from '~/hooks/useStableCallbacks';
import MenuBound from '~/components/shared/MenuBound';
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
import { roleSemantics, getRoleRuleCount } from './semantics';
import { defaultViewColumns } from '~/lib/views/viewColumns';
import { buildRoleViewColumns } from './viewColumns';
import { useWithFullObject } from '~/hooks/useViewObject';

const ROLE_INITIAL_SORT = { key: 'age', direction: 'desc' };
const ROLE_VIEW_OPTIONS = { sort: ROLE_INITIAL_SORT, columns: defaultViewColumns('roles') };
const NO_DATA: any[] = [];

export default function RoleList({ isVisible }: { isVisible: boolean }) {
    const { currentContext, selectedNamespaces, setSelectedNamespaces, namespaces } = useK8s();
    const { roles, loading, loadState, view } = useRoles(currentContext, selectedNamespaces, isVisible, ROLE_VIEW_OPTIONS) as any;
    const { handleShowDetails, handleEditYaml } = useStableCallbacks(useRoleActions());
    const selection = useSelection();
    const withObject = useWithFullObject(view ?? null);
    // Details need the object itself; windowed rows only carry identity and cells.
    const showDetails = useMemo(() => withObject(handleShowDetails), [withObject, handleShowDetails]);

    // Unified bulk actions (also used for single delete)
    const {
        bulkModalProps,
        openBulkDelete,
        exportYaml,
    } = useBulkActions({
        resourceLabel: 'Role',
        resourceType: 'roles',
        isNamespaced: true,
        deleteApi: ((namespace: string, name: string) => DeleteRole(namespace, name)) as any,
        getYamlApi: GetRoleYaml,

    });

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item: any) => item.metadata?.name, getValue: roleSemantics.name.getValue },
        { key: 'namespace', label: 'Namespace', render: (item: any) => item.metadata?.namespace, getValue: roleSemantics.namespace.getValue },
        {
            key: 'rules',
            label: 'Rules',
            align: 'center',
            render: (item: any) => getRoleRuleCount(item),
            getValue: roleSemantics.rules.getValue
        },
        { key: 'age', label: 'Age', render: (item: any) => formatAge(item.metadata?.creationTimestamp), getValue: roleSemantics.age.getValue },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item: any) => (
                <MenuBound
                    as={RoleActionsMenu}
                    menuId={`role-${item.metadata.uid}`}
                    role={item}
                    onEditYaml={handleEditYaml}
                    onDelete={(role: any) => openBulkDelete([role])}
                />
            ),
            getValue: () => '',
            isColumnSelector: true,
            disableSort: true
        }
    ], [handleEditYaml, openBulkDelete]);

    const viewColumns = useMemo(
        () => buildRoleViewColumns({
            onEditYaml: handleEditYaml,
            onDelete: item => openBulkDelete([item]),
        }),
        [handleEditYaml, openBulkDelete]);

    return (
        <>
            <ResourceList loadState={loadState}
                title="Roles"
                columns={view ? viewColumns : columns}
                source={view ?? null}
                data={view ? NO_DATA : roles}
                isLoading={loading}
                namespaces={namespaces}
                currentNamespace={selectedNamespaces}
                onNamespaceChange={setSelectedNamespaces}
                showNamespaceSelector={true}
                multiSelectNamespaces={true}
                initialSort={ROLE_INITIAL_SORT}
                resourceType="roles"
                onRowClick={view ? showDetails : handleShowDetails}
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
