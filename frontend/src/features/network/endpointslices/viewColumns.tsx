import React from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import MenuBound from '~/components/shared/MenuBound';
import { formatAge } from '~/utils/formatting';
import { defineViewColumns, type ViewColumn } from '~/lib/views/viewColumns';
import EndpointSliceActionsMenu from './EndpointSliceActionsMenu';

/** Columns whose cells other columns render from; always projected. */
export const ENDPOINTSLICE_PINNED_COLUMNS = ['readyCount'] as const;

/** Schema columns the list hides until the user shows them. */
export const ENDPOINTSLICE_HIDDEN_BY_DEFAULT = ['readyCount'] as const;

export interface EndpointSliceViewColumnDeps {
    onEditYaml: (item: any) => void;
    onShowDependencies: (item: any) => void;
    onDelete: (item: any) => void;
}

/** Presentation of the windowed EndpointSlice list; data semantics come from the generated schema. */
export function buildEndpointSliceViewColumns(deps: EndpointSliceViewColumnDeps): ViewColumn[] {
    return defineViewColumns('endpointslices', [
        { key: 'name', label: 'Name', render: item => item.metadata.name },
        { key: 'namespace', label: 'Namespace', render: item => item.metadata.namespace },
        { key: 'service', label: 'Service', render: item => item.cells.service },
        { key: 'addressType', label: 'Type', render: item => item.cells.addressType || '-' },
        { key: 'endpoints', label: 'Endpoints', render: item => `${item.cells.readyCount}/${item.cells.endpoints}` },
        { key: 'ports', label: 'Ports', render: item => item.cells.ports },
        { key: 'readyCount', label: 'Ready', defaultHidden: true, render: item => item.cells.readyCount },
        { key: 'age', label: 'Age', render: item => formatAge(item.cells.age ?? undefined) },
    ], [{
        key: 'actions',
        label: <EllipsisVerticalIcon className="h-5 w-5" />,
        align: 'center',
        render: (item: any) => (
            <MenuBound
                as={EndpointSliceActionsMenu}
                menuId={`endpointslice-${item.metadata.uid}`}
                endpointSlice={item}
                onEditYaml={deps.onEditYaml}
                onShowDependencies={deps.onShowDependencies}
                onDelete={deps.onDelete}
            />
        ),
        isColumnSelector: true,
        disableSort: true,
    }]);
}
