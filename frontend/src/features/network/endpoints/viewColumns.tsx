import React from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import MenuBound from '~/components/shared/MenuBound';
import { formatAge } from '~/utils/formatting';
import { defineViewColumns, type ViewColumn } from '~/lib/views/viewColumns';
import EndpointsActionsMenu from './EndpointsActionsMenu';

/** Columns whose cells other columns render from; always projected. */
export const ENDPOINTS_PINNED_COLUMNS = ['readyCount'] as const;

/** Schema columns the list hides until the user shows them. */
export const ENDPOINTS_HIDDEN_BY_DEFAULT = ['readyCount'] as const;

export interface EndpointsViewColumnDeps {
    onEditYaml: (item: any) => void;
    onShowDependencies: (item: any) => void;
    onDelete: (item: any) => void;
}

/** Presentation of the windowed Endpoints list; data semantics come from the generated schema. */
export function buildEndpointsViewColumns(deps: EndpointsViewColumnDeps): ViewColumn[] {
    return defineViewColumns('endpoints', [
        { key: 'name', label: 'Name', render: item => item.metadata.name },
        { key: 'namespace', label: 'Namespace', render: item => item.metadata.namespace },
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
                as={EndpointsActionsMenu}
                menuId={`endpoints-${item.metadata.uid}`}
                endpoints={item}
                onEditYaml={deps.onEditYaml}
                onShowDependencies={deps.onShowDependencies}
                onDelete={deps.onDelete}
            />
        ),
        isColumnSelector: true,
        disableSort: true,
    }]);
}
