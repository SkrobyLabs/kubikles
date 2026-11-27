import React, { useMemo, useState, useCallback } from 'react';
import ResourceList from '~/components/shared/ResourceList';
import { useStableCallbacks } from '~/hooks/useStableCallbacks';
import MenuBound from '~/components/shared/MenuBound';
import BulkActionModal from '~/components/shared/BulkActionModal';
import { useClusterRoles } from '~/hooks/resources';
import { useK8s } from '~/context';
import { useSelection } from '~/hooks/useSelection';
import { useBulkActions } from '~/hooks/useBulkActions';
import { DeleteClusterRole, GetClusterRoleYaml } from 'wailsjs/go/main/App';
import { formatAge } from '~/utils/formatting';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ClusterRoleActionsMenu from './ClusterRoleActionsMenu';
import { useClusterRoleActions } from './useClusterRoleActions';
import { clusterRoleSemantics } from './semantics';
import { defaultViewColumns } from '~/lib/views/viewColumns';
import { buildClusterRoleViewColumns } from './viewColumns';
import { useWithFullObject } from '~/hooks/useViewObject';

const CLUSTERROLE_INITIAL_SORT = { key: 'age', direction: 'desc' };
const CLUSTERROLE_VIEW_OPTIONS = { sort: CLUSTERROLE_INITIAL_SORT, columns: defaultViewColumns('clusterroles') };
const NO_DATA: any[] = [];

export default function ClusterRoleList({ isVisible }: { isVisible: boolean }) {
    const { currentContext } = useK8s();
    const { clusterRoles, loading, loadState, view } = useClusterRoles(currentContext, isVisible, CLUSTERROLE_VIEW_OPTIONS) as any;
    const { handleShowDetails, handleEditYaml } = useStableCallbacks(useClusterRoleActions());
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
        resourceLabel: 'ClusterRole',
        resourceType: 'clusterroles',
        isNamespaced: false,
        deleteApi: (context, name) => DeleteClusterRole(name),
        getYamlApi: GetClusterRoleYaml,

    });

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item: any) => item.metadata?.name, getValue: clusterRoleSemantics.name.getValue },
        {
            key: 'rules',
            label: 'Rules',
            align: 'center',
            render: (item: any) => (item.rules || []).length,
            getValue: clusterRoleSemantics.rules.getValue
        },
        {
            key: 'aggregation',
            label: 'Aggregation',
            render: (item: any) => {
                const selectors = item.aggregationRule?.clusterRoleSelectors || [];
                return selectors.length > 0 ? (
                    <span className="text-blue-400">Aggregated</span>
                ) : (
                    <span className="text-gray-500">-</span>
                );
            },
            getValue: clusterRoleSemantics.aggregation.getValue
        },
        { key: 'age', label: 'Age', render: (item: any) => formatAge(item.metadata?.creationTimestamp), getValue: clusterRoleSemantics.age.getValue },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item: any) => (
                <MenuBound
                    as={ClusterRoleActionsMenu}
                    menuId={`clusterrole-${item.metadata.uid}`}
                    clusterRole={item}
                    onEditYaml={handleEditYaml}
                    onDelete={(clusterRole: any) => openBulkDelete([clusterRole])}
                />
            ),
            getValue: () => '',
            isColumnSelector: true,
            disableSort: true
        }
    ], [handleEditYaml, openBulkDelete]);

    const viewColumns = useMemo(
        () => buildClusterRoleViewColumns({
            onEditYaml: handleEditYaml,
            onDelete: item => openBulkDelete([item]),
        }),
        [handleEditYaml, openBulkDelete]);

    return (
        <>
            <ResourceList loadState={loadState}
                title="Cluster Roles"
                columns={view ? viewColumns : columns}
                source={view ?? null}
                data={view ? NO_DATA : clusterRoles}
                isLoading={loading}
                showNamespaceSelector={false}
                initialSort={CLUSTERROLE_INITIAL_SORT}
                resourceType="clusterroles"
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
