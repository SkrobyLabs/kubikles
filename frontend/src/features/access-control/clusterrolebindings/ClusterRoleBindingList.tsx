import React, { useMemo, useState, useCallback } from 'react';
import ResourceList from '~/components/shared/ResourceList';
import { useStableCallbacks } from '~/hooks/useStableCallbacks';
import MenuBound from '~/components/shared/MenuBound';
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
import { clusterRoleBindingSemantics } from './semantics';
import { defaultViewColumns } from '~/lib/views/viewColumns';
import { buildClusterRoleBindingViewColumns } from './viewColumns';
import { useWithFullObject } from '~/hooks/useViewObject';

const CLUSTERROLEBINDING_INITIAL_SORT = { key: 'age', direction: 'desc' };
const CLUSTERROLEBINDING_VIEW_OPTIONS = { sort: CLUSTERROLEBINDING_INITIAL_SORT, columns: defaultViewColumns('clusterrolebindings') };
const NO_DATA: any[] = [];

export default function ClusterRoleBindingList({ isVisible }: { isVisible: boolean }) {
    const { currentContext } = useK8s();
    const { clusterRoleBindings, loading, loadState, view } = useClusterRoleBindings(currentContext, isVisible, CLUSTERROLEBINDING_VIEW_OPTIONS) as any;
    const { handleShowDetails, handleEditYaml } = useStableCallbacks(useClusterRoleBindingActions());
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
        resourceLabel: 'ClusterRoleBinding',
        resourceType: 'clusterrolebindings',
        isNamespaced: false,
        deleteApi: (context, name) => DeleteClusterRoleBinding(name),
        getYamlApi: GetClusterRoleBindingYaml,

    });

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item: any) => item.metadata?.name, getValue: clusterRoleBindingSemantics.name.getValue },
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
            getValue: clusterRoleBindingSemantics.roleRef.getValue
        },
        {
            key: 'subjects',
            label: 'Subjects',
            align: 'center',
            render: (item: any) => (item.subjects || []).length,
            getValue: clusterRoleBindingSemantics.subjects.getValue
        },
        { key: 'age', label: 'Age', render: (item: any) => formatAge(item.metadata?.creationTimestamp), getValue: clusterRoleBindingSemantics.age.getValue },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item: any) => (
                <MenuBound
                    as={ClusterRoleBindingActionsMenu}
                    menuId={`clusterrolebinding-${item.metadata.uid}`}
                    clusterRoleBinding={item}
                    onEditYaml={handleEditYaml}
                    onDelete={(clusterRoleBinding: any) => openBulkDelete([clusterRoleBinding])}
                />
            ),
            getValue: () => '',
            isColumnSelector: true,
            disableSort: true
        }
    ], [handleEditYaml, openBulkDelete]);

    const viewColumns = useMemo(
        () => buildClusterRoleBindingViewColumns({
            onEditYaml: handleEditYaml,
            onDelete: item => openBulkDelete([item]),
        }),
        [handleEditYaml, openBulkDelete]);

    return (
        <>
            <ResourceList loadState={loadState}
                title="Cluster Role Bindings"
                columns={view ? viewColumns : columns}
                source={view ?? null}
                data={view ? NO_DATA : clusterRoleBindings}
                isLoading={loading}
                showNamespaceSelector={false}
                initialSort={CLUSTERROLEBINDING_INITIAL_SORT}
                resourceType="clusterrolebindings"
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
