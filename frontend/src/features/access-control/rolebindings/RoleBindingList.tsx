import React, { useMemo, useState, useCallback } from 'react';
import ResourceList from '~/components/shared/ResourceList';
import { useStableCallbacks } from '~/hooks/useStableCallbacks';
import MenuBound from '~/components/shared/MenuBound';
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
import { roleBindingSemantics, getRoleBindingSubjectCount } from './semantics';
import { defaultViewColumns } from '~/lib/views/viewColumns';
import { buildRoleBindingViewColumns } from './viewColumns';
import { useWithFullObject } from '~/hooks/useViewObject';

const ROLEBINDING_INITIAL_SORT = { key: 'age', direction: 'desc' };
const ROLEBINDING_VIEW_OPTIONS = { sort: ROLEBINDING_INITIAL_SORT, columns: defaultViewColumns('rolebindings') };
const NO_DATA: any[] = [];

export default function RoleBindingList({ isVisible }: { isVisible: boolean }) {
    const { currentContext, selectedNamespaces, setSelectedNamespaces, namespaces } = useK8s();
    const { roleBindings, loading, loadState, view } = useRoleBindings(currentContext, selectedNamespaces, isVisible, ROLEBINDING_VIEW_OPTIONS) as any;
    const { handleShowDetails, handleEditYaml } = useStableCallbacks(useRoleBindingActions());
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
        resourceLabel: 'RoleBinding',
        resourceType: 'rolebindings',
        isNamespaced: true,
        deleteApi: ((namespace: string, name: string) => DeleteRoleBinding(namespace, name)) as any,
        getYamlApi: GetRoleBindingYaml,

    });

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item: any) => item.metadata?.name, getValue: roleBindingSemantics.name.getValue },
        { key: 'namespace', label: 'Namespace', render: (item: any) => item.metadata?.namespace, getValue: roleBindingSemantics.namespace.getValue },
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
            getValue: roleBindingSemantics.roleRef.getValue
        },
        {
            key: 'subjects',
            label: 'Subjects',
            align: 'center',
            render: (item: any) => getRoleBindingSubjectCount(item),
            getValue: roleBindingSemantics.subjects.getValue
        },
        { key: 'age', label: 'Age', render: (item: any) => formatAge(item.metadata?.creationTimestamp), getValue: roleBindingSemantics.age.getValue },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item: any) => (
                <MenuBound
                    as={RoleBindingActionsMenu}
                    menuId={`rolebinding-${item.metadata.uid}`}
                    roleBinding={item}
                    onEditYaml={handleEditYaml}
                    onDelete={(roleBinding: any) => openBulkDelete([roleBinding])}
                />
            ),
            getValue: () => '',
            isColumnSelector: true,
            disableSort: true
        }
    ], [handleEditYaml, openBulkDelete]);

    const viewColumns = useMemo(
        () => buildRoleBindingViewColumns({
            onEditYaml: handleEditYaml,
            onDelete: item => openBulkDelete([item]),
        }),
        [handleEditYaml, openBulkDelete]);

    return (
        <>
            <ResourceList loadState={loadState}
                title="Role Bindings"
                columns={view ? viewColumns : columns}
                source={view ?? null}
                data={view ? NO_DATA : roleBindings}
                isLoading={loading}
                namespaces={namespaces}
                currentNamespace={selectedNamespaces}
                onNamespaceChange={setSelectedNamespaces}
                showNamespaceSelector={true}
                multiSelectNamespaces={true}
                initialSort={ROLEBINDING_INITIAL_SORT}
                resourceType="rolebindings"
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
