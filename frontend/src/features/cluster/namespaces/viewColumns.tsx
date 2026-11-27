import React from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import MenuBound from '~/components/shared/MenuBound';
import { formatAge } from '~/utils/formatting';
import { defineViewColumns, type ViewColumn } from '~/lib/views/viewColumns';
import NamespaceActionsMenu from './NamespaceActionsMenu';

export function getStatusColor(status: any) {
    switch (status) {
        case 'Active':
            return 'text-green-400';
        case 'Terminating':
            return 'text-yellow-400';
        default:
            return 'text-gray-400';
    }
}

/** Columns whose cells other columns render from; always projected. */
export const NAMESPACE_PINNED_COLUMNS = ['metrics'] as const;

/** Schema columns the list hides until the user shows them. */
export const NAMESPACE_HIDDEN_BY_DEFAULT = ['metrics'] as const;

export interface NamespaceViewColumnDeps {
    /** The metric bars read the row's metrics cell and the overlay availability, so the list supplies the cell. */
    renderMetric: (item: any, kind: 'cpu' | 'memory') => React.ReactNode;
    onEditYaml: (item: any) => void;
    onDelete: (item: any) => void;
}

/** Presentation of the windowed Namespace list. CPU and memory are overlay columns: sortable like the legacy ones. */
export function buildNamespaceViewColumns(deps: NamespaceViewColumnDeps): ViewColumn[] {
    return defineViewColumns('namespaces', [
        { key: 'name', label: 'Name', render: item => item.metadata.name },
        { key: 'cpu', label: 'CPU', render: item => deps.renderMetric(item, 'cpu') },
        { key: 'memory', label: 'Memory', render: item => deps.renderMetric(item, 'memory') },
        {
            key: 'status',
            label: 'Status',
            render: item => <span className={getStatusColor(item.cells.status)}>{item.cells.status}</span>,
        },
        { key: 'age', label: 'Age', render: item => formatAge(item.cells.age ?? undefined) },
        { key: 'metrics', label: 'Metrics', defaultHidden: true, render: item => item.cells.metrics },
    ], [
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item: any) => (
                <MenuBound
                    as={NamespaceActionsMenu}
                    menuId={`namespace-${item.metadata.uid}`}
                    namespace={item}
                    onEditYaml={() => deps.onEditYaml(item)}
                    onDelete={() => deps.onDelete(item)}
                />
            ),
            isColumnSelector: true,
            disableSort: true,
        },
    ]);
}
