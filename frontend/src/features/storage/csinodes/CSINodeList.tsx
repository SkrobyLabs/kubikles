import React, { useMemo, useState, useCallback } from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ResourceList from '~/components/shared/ResourceList';
import { useStableCallbacks } from '~/hooks/useStableCallbacks';
import MenuBound from '~/components/shared/MenuBound';
import BulkActionModal from '~/components/shared/BulkActionModal';
import CSINodeActionsMenu from './CSINodeActionsMenu';
import { useCSINodes } from '~/hooks/resources';
import { useCSINodeActions } from './useCSINodeActions';
import { useK8s } from '~/context';
import { useSelection } from '~/hooks/useSelection';
import { useBulkActions } from '~/hooks/useBulkActions';
import { DeleteCSINode, GetCSINodeYaml } from 'wailsjs/go/main/App';
import { formatAge } from '~/utils/formatting';
import { csiNodeSemantics, getDriverCount, getDriverNames } from './semantics';
import { defaultViewColumns } from '~/lib/views/viewColumns';
import { buildCSINodeViewColumns } from './viewColumns';
import { useWithFullObject } from '~/hooks/useViewObject';

const CSINODE_INITIAL_SORT = { key: 'age', direction: 'desc' };
const CSINODE_VIEW_OPTIONS = { sort: CSINODE_INITIAL_SORT, columns: defaultViewColumns('csinodes') };
const NO_DATA: any[] = [];

export default function CSINodeList({ isVisible }: { isVisible: boolean }) {
    const { currentContext } = useK8s();
    const { csiNodes, loading, loadState, view } = useCSINodes(currentContext, isVisible, CSINODE_VIEW_OPTIONS) as any;
    const { handleShowDetails, handleEditYaml } = useStableCallbacks(useCSINodeActions());
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
        resourceLabel: 'CSINode',
        resourceType: 'csinodes',
        isNamespaced: false,
        deleteApi: DeleteCSINode,
        getYamlApi: GetCSINodeYaml,

    });

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item: any) => item.metadata?.name, getValue: csiNodeSemantics.name.getValue },
        {
            key: 'driverCount',
            label: 'Drivers',
            render: (item: any) => (
                <span className={getDriverCount(item) > 0 ? 'text-green-400' : 'text-gray-500'}>
                    {getDriverCount(item)}
                </span>
            ),
            getValue: csiNodeSemantics.driverCount.getValue
        },
        { key: 'driverNames', label: 'Driver Names', render: (item: any) => getDriverNames(item), getValue: csiNodeSemantics.driverNames.getValue },
        { key: 'age', label: 'Age', render: (item: any) => formatAge(item.metadata?.creationTimestamp), getValue: csiNodeSemantics.age.getValue },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item: any) => (
                <MenuBound
                    as={CSINodeActionsMenu}
                    menuId={`csinode-${item.metadata.uid}`}
                    csiNode={item}
                    onEditYaml={handleEditYaml}
                    onDelete={(csiNode: any) => openBulkDelete([csiNode])}
                />
            ),
            getValue: () => '',
            isColumnSelector: true,
            disableSort: true
        }
    ], [handleEditYaml, openBulkDelete]);

    const viewColumns = useMemo(
        () => buildCSINodeViewColumns({
            onEditYaml: handleEditYaml,
            onDelete: item => openBulkDelete([item]),
        }),
        [handleEditYaml, openBulkDelete]);

    return (
        <>
            <ResourceList loadState={loadState}
                title="CSI Nodes"
                columns={view ? viewColumns : columns}
                source={view ?? null}
                data={view ? NO_DATA : csiNodes}
                isLoading={loading}
                showNamespaceSelector={false}
                initialSort={CSINODE_INITIAL_SORT}
                resourceType="csinodes"
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
