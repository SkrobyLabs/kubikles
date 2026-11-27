import React from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import MenuBound from '~/components/shared/MenuBound';
import { formatAge } from '~/utils/formatting';
import { defineViewColumns, type ViewColumn } from '~/lib/views/viewColumns';
import ServiceActionsMenu from './ServiceActionsMenu';

/** Columns whose cells other columns render from; always projected. */
export const SERVICE_PINNED_COLUMNS = ['externalAddresses'] as const;

/** Schema columns the list hides until the user shows them. */
export const SERVICE_HIDDEN_BY_DEFAULT = ['externalIP', 'externalAddresses', 'selector', 'sessionAffinity', 'externalName'] as const;

export interface ServiceViewColumnDeps {
    onEditYaml: (item: any) => void;
    onShowDependencies: (item: any) => void;
    onDelete: (item: any) => void;
}

/** Presentation of the windowed Service list; data semantics come from the generated schema. */
export function buildServiceViewColumns(deps: ServiceViewColumnDeps): ViewColumn[] {
    return defineViewColumns('services', [
        { key: 'name', label: 'Name', render: item => item.metadata.name },
        { key: 'namespace', label: 'Namespace', render: item => item.metadata.namespace },
        { key: 'type', label: 'Type', render: item => item.cells.type },
        { key: 'clusterIP', label: 'Cluster IP', render: item => item.cells.clusterIP },
        { key: 'ports', label: 'Ports', render: item => item.cells.ports },
        { key: 'age', label: 'Age', render: item => formatAge(item.cells.age ?? undefined) },
        // Hidden by default columns
        {
            key: 'externalIP',
            label: 'External IP',
            defaultHidden: true,
            // The cell of this column is the first IP only (its sort and filter value); the display lists every address.
            render: item => item.cells.externalAddresses
                ? item.cells.externalAddresses
                : <span className="text-gray-500">-</span>,
        },
        { key: 'externalAddresses', label: 'External Addresses', defaultHidden: true, render: item => item.cells.externalAddresses },
        {
            key: 'selector',
            label: 'Selector',
            defaultHidden: true,
            render: item => {
                // The cell is "k=v" pairs joined with commas; label keys and values never contain one.
                const entries: string[] = item.cells.selector ? item.cells.selector.split(',') : [];
                if (entries.length === 0) return '-';
                return <span title={entries.join('\n')}>{entries.length} label{entries.length > 1 ? 's' : ''}</span>;
            },
        },
        { key: 'sessionAffinity', label: 'Session Affinity', defaultHidden: true, render: item => item.cells.sessionAffinity },
        {
            key: 'externalName',
            label: 'External Name',
            defaultHidden: true,
            render: item => item.cells.externalName || <span className="text-gray-500">-</span>,
        },
    ], [{
        key: 'actions',
        label: <EllipsisVerticalIcon className="h-5 w-5" />,
        align: 'center',
        render: (item: any) => (
            <MenuBound
                as={ServiceActionsMenu}
                menuId={`service-${item.metadata.uid}`}
                service={item}
                onEditYaml={deps.onEditYaml}
                onShowDependencies={deps.onShowDependencies}
                onDelete={deps.onDelete}
            />
        ),
        isColumnSelector: true,
        disableSort: true,
    }]);
}
