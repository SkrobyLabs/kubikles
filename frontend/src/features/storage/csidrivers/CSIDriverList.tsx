import React, { useMemo, useState, useCallback } from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ResourceList from '~/components/shared/ResourceList';
import { useStableCallbacks } from '~/hooks/useStableCallbacks';
import MenuBound from '~/components/shared/MenuBound';
import BulkActionModal from '~/components/shared/BulkActionModal';
import CSIDriverActionsMenu from './CSIDriverActionsMenu';
import { useCSIDrivers } from '~/hooks/resources';
import { useCSIDriverActions } from './useCSIDriverActions';
import { useK8s } from '~/context';
import { useSelection } from '~/hooks/useSelection';
import { useBulkActions } from '~/hooks/useBulkActions';
import { DeleteCSIDriver, GetCSIDriverYaml } from 'wailsjs/go/main/App';
import { formatAge } from '~/utils/formatting';
import { csiDriverSemantics, getAttachRequired, getPodInfoOnMount, getStorageCapacity, getVolumeModes } from './semantics';
import { defaultViewColumns } from '~/lib/views/viewColumns';
import { buildCSIDriverViewColumns, BooleanIcon } from './viewColumns';
import { useWithFullObject } from '~/hooks/useViewObject';

const CSIDRIVER_INITIAL_SORT = { key: 'age', direction: 'desc' };
const CSIDRIVER_VIEW_OPTIONS = { sort: CSIDRIVER_INITIAL_SORT, columns: defaultViewColumns('csidrivers') };
const NO_DATA: any[] = [];

export default function CSIDriverList({ isVisible }: { isVisible: boolean }) {
    const { currentContext } = useK8s();
    const { csiDrivers, loading, loadState, view } = useCSIDrivers(currentContext, isVisible, CSIDRIVER_VIEW_OPTIONS) as any;
    const { handleShowDetails, handleEditYaml } = useStableCallbacks(useCSIDriverActions());
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
        resourceLabel: 'CSIDriver',
        resourceType: 'csidrivers',
        isNamespaced: false,
        deleteApi: DeleteCSIDriver,
        getYamlApi: GetCSIDriverYaml,

    });

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item: any) => item.metadata?.name, getValue: csiDriverSemantics.name.getValue },
        {
            key: 'attachRequired',
            label: 'Attach Required',
            render: (item: any) => <BooleanIcon value={getAttachRequired(item)} />,
            getValue: csiDriverSemantics.attachRequired.getValue,
            align: 'center'
        },
        {
            key: 'podInfoOnMount',
            label: 'Pod Info on Mount',
            render: (item: any) => <BooleanIcon value={getPodInfoOnMount(item)} />,
            getValue: csiDriverSemantics.podInfoOnMount.getValue,
            align: 'center'
        },
        {
            key: 'storageCapacity',
            label: 'Storage Capacity',
            render: (item: any) => <BooleanIcon value={getStorageCapacity(item)} />,
            getValue: csiDriverSemantics.storageCapacity.getValue,
            align: 'center'
        },
        { key: 'volumeModes', label: 'Volume Modes', render: (item: any) => getVolumeModes(item), getValue: csiDriverSemantics.volumeModes.getValue },
        { key: 'age', label: 'Age', render: (item: any) => formatAge(item.metadata?.creationTimestamp), getValue: csiDriverSemantics.age.getValue },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item: any) => (
                <MenuBound
                    as={CSIDriverActionsMenu}
                    menuId={`csidriver-${item.metadata.uid}`}
                    csiDriver={item}
                    onEditYaml={handleEditYaml}
                    onDelete={(csiDriver: any) => openBulkDelete([csiDriver])}
                />
            ),
            getValue: () => '',
            isColumnSelector: true,
            disableSort: true
        }
    ], [handleEditYaml, openBulkDelete]);

    const viewColumns = useMemo(
        () => buildCSIDriverViewColumns({
            onEditYaml: handleEditYaml,
            onDelete: item => openBulkDelete([item]),
        }),
        [handleEditYaml, openBulkDelete]);

    return (
        <>
            <ResourceList loadState={loadState}
                title="CSI Drivers"
                columns={view ? viewColumns : columns}
                source={view ?? null}
                data={view ? NO_DATA : csiDrivers}
                isLoading={loading}
                showNamespaceSelector={false}
                initialSort={CSIDRIVER_INITIAL_SORT}
                resourceType="csidrivers"
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
