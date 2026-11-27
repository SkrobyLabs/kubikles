import React from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import MenuBound from '~/components/shared/MenuBound';
import { formatAge } from '~/utils/formatting';
import { defineViewColumns, type ViewColumn } from '~/lib/views/viewColumns';
import CSINodeActionsMenu from './CSINodeActionsMenu';

export interface CSINodeViewColumnDeps {
    onEditYaml: (item: any) => void;
    onDelete: (item: any) => void;
}

/** Presentation of the windowed CSINode list. */
export function buildCSINodeViewColumns(deps: CSINodeViewColumnDeps): ViewColumn[] {
    return defineViewColumns('csinodes', [
        { key: 'name', label: 'Name', render: item => item.metadata.name },
        {
            key: 'driverCount',
            label: 'Drivers',
            render: item => (
                <span className={item.cells.driverCount > 0 ? 'text-green-400' : 'text-gray-500'}>
                    {item.cells.driverCount}
                </span>
            ),
        },
        { key: 'driverNames', label: 'Driver Names', render: item => item.cells.driverNames },
        { key: 'age', label: 'Age', render: item => formatAge(item.cells.age ?? undefined) },
    ], [{
        key: 'actions',
        label: <EllipsisVerticalIcon className="h-5 w-5" />,
        align: 'center',
        render: (item: any) => (
            <MenuBound
                as={CSINodeActionsMenu}
                menuId={`csinode-${item.metadata.uid}`}
                csiNode={item}
                onEditYaml={deps.onEditYaml}
                onDelete={deps.onDelete}
            />
        ),
        isColumnSelector: true,
        disableSort: true,
    }]);
}
