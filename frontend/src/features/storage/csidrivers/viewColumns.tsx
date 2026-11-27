import React from 'react';
import { EllipsisVerticalIcon, CheckCircleIcon, XCircleIcon } from '@heroicons/react/24/outline';
import MenuBound from '~/components/shared/MenuBound';
import { formatAge } from '~/utils/formatting';
import { defineViewColumns, type ViewColumn } from '~/lib/views/viewColumns';
import CSIDriverActionsMenu from './CSIDriverActionsMenu';

export const BooleanIcon = ({ value }: any) => {
    if (value === true) {
        return <CheckCircleIcon className="h-5 w-5 text-green-400" />;
    }
    return <XCircleIcon className="h-5 w-5 text-gray-500" />;
};

export interface CSIDriverViewColumnDeps {
    onEditYaml: (item: any) => void;
    onDelete: (item: any) => void;
}

/** Presentation of the windowed CSIDriver list. */
export function buildCSIDriverViewColumns(deps: CSIDriverViewColumnDeps): ViewColumn[] {
    return defineViewColumns('csidrivers', [
        { key: 'name', label: 'Name', render: item => item.metadata.name },
        { key: 'attachRequired', label: 'Attach Required', align: 'center', render: item => <BooleanIcon value={item.cells.attachRequired === 'Yes'} /> },
        { key: 'podInfoOnMount', label: 'Pod Info on Mount', align: 'center', render: item => <BooleanIcon value={item.cells.podInfoOnMount === 'Yes'} /> },
        { key: 'storageCapacity', label: 'Storage Capacity', align: 'center', render: item => <BooleanIcon value={item.cells.storageCapacity === 'Yes'} /> },
        { key: 'volumeModes', label: 'Volume Modes', render: item => item.cells.volumeModes },
        { key: 'age', label: 'Age', render: item => formatAge(item.cells.age ?? undefined) },
    ], [{
        key: 'actions',
        label: <EllipsisVerticalIcon className="h-5 w-5" />,
        align: 'center',
        render: (item: any) => (
            <MenuBound
                as={CSIDriverActionsMenu}
                menuId={`csidriver-${item.metadata.uid}`}
                csiDriver={item}
                onEditYaml={deps.onEditYaml}
                onDelete={deps.onDelete}
            />
        ),
        isColumnSelector: true,
        disableSort: true,
    }]);
}
