import React, { useMemo, useState, useCallback } from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ResourceList from '~/components/shared/ResourceList';
import BulkActionModal from '~/components/shared/BulkActionModal';
import CSINodeActionsMenu from './CSINodeActionsMenu';
import { useCSINodes } from '~/hooks/resources';
import { useCSINodeActions } from './useCSINodeActions';
import { useK8s } from '~/context';
import { useSelection } from '~/hooks/useSelection';
import { useBulkActions } from '~/hooks/useBulkActions';
import { DeleteCSINode, GetCSINodeYaml } from 'wailsjs/go/main/App';
import { formatAge } from '~/utils/formatting';
import { useMenuPosition } from '~/hooks/useMenuPosition';

export default function CSINodeList({ isVisible }: { isVisible: boolean }) {
    const { currentContext } = useK8s();
    const { activeMenuId, menuPosition, handleMenuOpenChange } = useMenuPosition();
    const { csiNodes, loading } = useCSINodes(currentContext, isVisible) as any;
    const { handleShowDetails, handleEditYaml } = useCSINodeActions();
    const selection = useSelection();

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

    const getDriverCount = (csiNode: any) => {
        const drivers = csiNode.spec?.drivers || [];
        return drivers.length;
    };

    const getDriverNames = (csiNode: any) => {
        const drivers = csiNode.spec?.drivers || [];
        if (drivers.length === 0) return '-';
        if (drivers.length <= 2) {
            return drivers.map((d: any) => d.name).join(', ');
        }
        return `${drivers[0].name}, +${drivers.length - 1} more`;
    };

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item: any) => item.metadata?.name, getValue: (item: any) => item.metadata?.name },
        {
            key: 'driverCount',
            label: 'Drivers',
            render: (item: any) => (
                <span className={getDriverCount(item) > 0 ? 'text-green-400' : 'text-gray-500'}>
                    {getDriverCount(item)}
                </span>
            ),
            getValue: (item: any) => getDriverCount(item)
        },
        { key: 'driverNames', label: 'Driver Names', render: (item: any) => getDriverNames(item), getValue: (item: any) => getDriverNames(item) },
        { key: 'age', label: 'Age', render: (item: any) => formatAge(item.metadata?.creationTimestamp), getValue: (item: any) => item.metadata?.creationTimestamp },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item: any) => (
                <CSINodeActionsMenu
                    csiNode={item}
                    isOpen={activeMenuId === `csinode-${item.metadata.uid}`}
                    menuPosition={menuPosition}
                    onOpenChange={(isOpen: any, buttonElement: any) => handleMenuOpenChange(isOpen, `csinode-${item.metadata.uid}`, buttonElement)}
                    onEditYaml={handleEditYaml}
                    onDelete={(csiNode: any) => openBulkDelete([csiNode])}
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
                title="CSI Nodes"
                columns={columns}
                data={csiNodes}
                isLoading={loading}
                showNamespaceSelector={false}
                initialSort={{ key: 'age', direction: 'desc' }}
                resourceType="csinodes"
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
