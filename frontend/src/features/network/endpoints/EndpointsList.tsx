import React, { useMemo, useState, useCallback } from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ResourceList from '~/components/shared/ResourceList';
import { useStableCallbacks } from '~/hooks/useStableCallbacks';
import MenuBound from '~/components/shared/MenuBound';
import BulkActionModal from '~/components/shared/BulkActionModal';
import EndpointsActionsMenu from './EndpointsActionsMenu';
import { useEndpoints } from '~/hooks/resources';
import { useEndpointsActions } from './useEndpointsActions';
import { useK8s } from '~/context';
import { useSelection } from '~/hooks/useSelection';
import { useBulkActions } from '~/hooks/useBulkActions';
import { DeleteEndpoints, GetEndpointsYaml } from 'wailsjs/go/main/App';
import { formatAge } from '~/utils/formatting';
import { endpointsSemantics, endpointsAddressCount, endpointsReadyCount, endpointsPorts } from './semantics';
import { defaultViewColumns } from '~/lib/views/viewColumns';
import { buildEndpointsViewColumns, ENDPOINTS_HIDDEN_BY_DEFAULT, ENDPOINTS_PINNED_COLUMNS } from './viewColumns';
import { useWithFullObject } from '~/hooks/useViewObject';

const ENDPOINTS_INITIAL_SORT = { key: 'age', direction: 'desc' };
const ENDPOINTS_VIEW_OPTIONS = { sort: ENDPOINTS_INITIAL_SORT, columns: defaultViewColumns('endpoints', ENDPOINTS_HIDDEN_BY_DEFAULT) };
const NO_DATA: any[] = [];

export default function EndpointsList({ isVisible }: { isVisible: boolean }) {
    const { currentContext, selectedNamespaces, setSelectedNamespaces, namespaces } = useK8s();
    const { endpoints, loading, loadState, view } = useEndpoints(currentContext, selectedNamespaces, isVisible, ENDPOINTS_VIEW_OPTIONS) as any;
    const { handleShowDetails, handleEditYaml, handleShowDependencies } = useStableCallbacks(useEndpointsActions());
    const selection = useSelection();
    const withObject = useWithFullObject(view ?? null);
    // Details need the object itself; windowed rows only carry identity and cells.
    const showDetails = useMemo(() => withObject(handleShowDetails), [withObject, handleShowDetails]);

    const {
        bulkModalProps,
        openBulkDelete,
        exportYaml,
    } = useBulkActions({
        resourceLabel: 'Endpoints',
        resourceType: 'endpoints',
        isNamespaced: true,
        deleteApi: DeleteEndpoints,
        getYamlApi: GetEndpointsYaml,

    });

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item: any) => item.metadata?.name, getValue: endpointsSemantics.name.getValue },
        { key: 'namespace', label: 'Namespace', render: (item: any) => item.metadata?.namespace, getValue: endpointsSemantics.namespace.getValue },
        {
            key: 'endpoints',
            label: 'Endpoints',
            render: (item: any) => {
                const ready = endpointsReadyCount(item);
                const total = endpointsAddressCount(item);
                return `${ready}/${total}`;
            },
            getValue: endpointsSemantics.endpoints.getValue
        },
        { key: 'ports', label: 'Ports', render: (item: any) => endpointsPorts(item), getValue: endpointsSemantics.ports.getValue },
        { key: 'age', label: 'Age', render: (item: any) => formatAge(item.metadata?.creationTimestamp), getValue: endpointsSemantics.age.getValue },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item: any) => (
                <MenuBound
                    as={EndpointsActionsMenu}
                    menuId={`endpoints-${item.metadata.uid}`}
                    endpoints={item}
                    onEditYaml={handleEditYaml}
                    onShowDependencies={handleShowDependencies}
                    onDelete={(ep: any) => openBulkDelete([ep])}
                />
            ),
            getValue: () => '',
            isColumnSelector: true,
            disableSort: true
        }
    ], [handleEditYaml, handleShowDependencies, openBulkDelete]);

    const viewColumns = useMemo(
        () => buildEndpointsViewColumns({
            onEditYaml: handleEditYaml,
            onShowDependencies: handleShowDependencies,
            onDelete: item => openBulkDelete([item]),
        }),
        [handleEditYaml, handleShowDependencies, openBulkDelete]);

    return (
        <>
            <ResourceList loadState={loadState}
                title="Endpoints"
                columns={view ? viewColumns : columns}
                source={view ?? null}
                viewPinnedColumns={ENDPOINTS_PINNED_COLUMNS}
                data={view ? NO_DATA : endpoints}
                isLoading={loading}
                namespaces={namespaces}
                currentNamespace={selectedNamespaces}
                onNamespaceChange={setSelectedNamespaces}
                showNamespaceSelector={true}
                multiSelectNamespaces={true}
                initialSort={ENDPOINTS_INITIAL_SORT}
                resourceType="endpoints"
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
