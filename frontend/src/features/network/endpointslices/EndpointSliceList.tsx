import React, { useMemo, useState, useCallback } from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ResourceList from '~/components/shared/ResourceList';
import { useStableCallbacks } from '~/hooks/useStableCallbacks';
import MenuBound from '~/components/shared/MenuBound';
import BulkActionModal from '~/components/shared/BulkActionModal';
import EndpointSliceActionsMenu from './EndpointSliceActionsMenu';
import { useEndpointSlices } from '~/hooks/resources';
import { useEndpointSliceActions } from './useEndpointSliceActions';
import { useK8s } from '~/context';
import { useSelection } from '~/hooks/useSelection';
import { useBulkActions } from '~/hooks/useBulkActions';
import { DeleteEndpointSlice, GetEndpointSliceYaml } from 'wailsjs/go/main/App';
import { formatAge } from '~/utils/formatting';
import { endpointSliceSemantics, endpointSliceEndpointCount, endpointSliceReadyCount, endpointSlicePorts, endpointSliceServiceName } from './semantics';
import { defaultViewColumns } from '~/lib/views/viewColumns';
import { buildEndpointSliceViewColumns, ENDPOINTSLICE_HIDDEN_BY_DEFAULT, ENDPOINTSLICE_PINNED_COLUMNS } from './viewColumns';
import { useWithFullObject } from '~/hooks/useViewObject';

const ENDPOINTSLICE_INITIAL_SORT = { key: 'age', direction: 'desc' };
const ENDPOINTSLICE_VIEW_OPTIONS = { sort: ENDPOINTSLICE_INITIAL_SORT, columns: defaultViewColumns('endpointslices', ENDPOINTSLICE_HIDDEN_BY_DEFAULT) };
const NO_DATA: any[] = [];

export default function EndpointSliceList({ isVisible }: { isVisible: boolean }) {
    const { currentContext, selectedNamespaces, setSelectedNamespaces, namespaces } = useK8s();
    const { endpointSlices, loading, loadState, view } = useEndpointSlices(currentContext, selectedNamespaces, isVisible, ENDPOINTSLICE_VIEW_OPTIONS) as any;
    const { handleShowDetails, handleEditYaml, handleShowDependencies } = useStableCallbacks(useEndpointSliceActions());
    const selection = useSelection();
    const withObject = useWithFullObject(view ?? null);
    // Details need the object itself; windowed rows only carry identity and cells.
    const showDetails = useMemo(() => withObject(handleShowDetails), [withObject, handleShowDetails]);

    const {
        bulkModalProps,
        openBulkDelete,
        exportYaml,
    } = useBulkActions({
        resourceLabel: 'EndpointSlice',
        resourceType: 'endpointslices',
        isNamespaced: true,
        deleteApi: DeleteEndpointSlice,
        getYamlApi: GetEndpointSliceYaml,

    });

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item: any) => item.metadata?.name, getValue: endpointSliceSemantics.name.getValue },
        { key: 'namespace', label: 'Namespace', render: (item: any) => item.metadata?.namespace, getValue: endpointSliceSemantics.namespace.getValue },
        { key: 'service', label: 'Service', render: (item: any) => endpointSliceServiceName(item), getValue: endpointSliceSemantics.service.getValue },
        { key: 'addressType', label: 'Type', render: (item: any) => item.addressType || '-', getValue: endpointSliceSemantics.addressType.getValue },
        {
            key: 'endpoints',
            label: 'Endpoints',
            render: (item: any) => {
                const ready = endpointSliceReadyCount(item);
                const total = endpointSliceEndpointCount(item);
                return `${ready}/${total}`;
            },
            getValue: endpointSliceSemantics.endpoints.getValue
        },
        { key: 'ports', label: 'Ports', render: (item: any) => endpointSlicePorts(item), getValue: endpointSliceSemantics.ports.getValue },
        { key: 'age', label: 'Age', render: (item: any) => formatAge(item.metadata?.creationTimestamp), getValue: endpointSliceSemantics.age.getValue },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item: any) => (
                <MenuBound
                    as={EndpointSliceActionsMenu}
                    menuId={`endpointslice-${item.metadata.uid}`}
                    endpointSlice={item}
                    onEditYaml={handleEditYaml}
                    onShowDependencies={handleShowDependencies}
                    onDelete={(slice: any) => openBulkDelete([slice])}
                />
            ),
            getValue: () => '',
            isColumnSelector: true,
            disableSort: true
        }
    ], [handleEditYaml, handleShowDependencies, openBulkDelete]);

    const viewColumns = useMemo(
        () => buildEndpointSliceViewColumns({
            onEditYaml: handleEditYaml,
            onShowDependencies: handleShowDependencies,
            onDelete: item => openBulkDelete([item]),
        }),
        [handleEditYaml, handleShowDependencies, openBulkDelete]);

    return (
        <>
            <ResourceList loadState={loadState}
                title="Endpoint Slices"
                columns={view ? viewColumns : columns}
                source={view ?? null}
                viewPinnedColumns={ENDPOINTSLICE_PINNED_COLUMNS}
                data={view ? NO_DATA : endpointSlices}
                isLoading={loading}
                namespaces={namespaces}
                currentNamespace={selectedNamespaces}
                onNamespaceChange={setSelectedNamespaces}
                showNamespaceSelector={true}
                multiSelectNamespaces={true}
                initialSort={ENDPOINTSLICE_INITIAL_SORT}
                resourceType="endpointslices"
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
