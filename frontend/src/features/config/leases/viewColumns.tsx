import React from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import MenuBound from '~/components/shared/MenuBound';
import { formatAge } from '~/utils/formatting';
import { defineViewColumns, type ViewColumn } from '~/lib/views/viewColumns';
import LeaseActionsMenu from './LeaseActionsMenu';

export interface LeaseViewColumnDeps {
    onEditYaml: (item: any) => void;
    onShowDependencies: (item: any) => void;
    onDelete: (item: any) => void;
}

/** Presentation of the windowed Lease list; data semantics come from the generated schema. */
export function buildLeaseViewColumns(deps: LeaseViewColumnDeps): ViewColumn[] {
    return defineViewColumns('leases', [
        { key: 'name', label: 'Name', render: item => item.metadata.name },
        { key: 'namespace', label: 'Namespace', render: item => item.metadata.namespace },
        { key: 'holder', label: 'Holder Identity', render: item => item.cells.holder },
        { key: 'duration', label: 'Duration', render: item => (item.cells.duration ? `${item.cells.duration}s` : '-') },
        { key: 'renewTime', label: 'Last Renewed', render: item => (item.cells.renewTime ? formatAge(item.cells.renewTime) : '-') },
        { key: 'age', label: 'Age', render: item => formatAge(item.cells.age ?? undefined) },
    ], [{
        key: 'actions',
        label: <EllipsisVerticalIcon className="h-5 w-5" />,
        align: 'center',
        render: (item: any) => (
            <MenuBound
                as={LeaseActionsMenu}
                menuId={`lease-${item.metadata.uid}`}
                lease={item}
                onEditYaml={deps.onEditYaml}
                onShowDependencies={deps.onShowDependencies}
                onDelete={deps.onDelete}
            />
        ),
        isColumnSelector: true,
        disableSort: true,
    }]);
}
