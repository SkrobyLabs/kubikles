import React from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import MenuBound from '~/components/shared/MenuBound';
import { CopyableLabel } from '~/components/shared/DetailComponents';
import { formatAge } from '~/utils/formatting';
import { defineViewColumns, type ViewColumn } from '~/lib/views/viewColumns';
import IngressActionsMenu from './IngressActionsMenu';

/** Status text to colour, as the legacy list colours it. */
export const INGRESS_STATUS_COLORS: Record<string, string> = {
    'Active': 'text-green-400',
    'No Rules': 'text-red-400',
    'Pending': 'text-yellow-400',
};

/** Columns whose cells other columns render from; always projected. */
export const INGRESS_PINNED_COLUMNS = ['tlsCount', 'defaultBackendTarget'] as const;

/** Schema columns the list hides until the user shows them. */
export const INGRESS_HIDDEN_BY_DEFAULT = ['paths', 'tls', 'tlsCount', 'defaultBackend', 'defaultBackendTarget', 'rules'] as const;

export interface IngressViewColumnDeps {
    onEditYaml: (item: any) => void;
    onShowDependencies: (item: any) => void;
    onDelete: (item: any) => void;
}

/** Presentation of the windowed Ingress list; data semantics come from the generated schema. */
export function buildIngressViewColumns(deps: IngressViewColumnDeps): ViewColumn[] {
    return defineViewColumns('ingresses', [
        { key: 'name', label: 'Name', render: item => item.metadata.name },
        { key: 'namespace', label: 'Namespace', render: item => item.metadata.namespace },
        {
            key: 'status',
            label: 'Status',
            render: item => <span className={INGRESS_STATUS_COLORS[item.cells.status]}>{item.cells.status}</span>,
        },
        { key: 'class', label: 'Class', render: item => item.cells.class },
        {
            key: 'hosts',
            label: 'Hosts',
            render: item => {
                // The cell is the hosts joined with ", ", or "*" when no rule names a host.
                const text: string = item.cells.hosts;
                if (text === '*') return '*';
                return <span className="flex flex-wrap gap-1">{text.split(', ').map(h => <CopyableLabel key={h} value={h} />)}</span>;
            },
        },
        { key: 'address', label: 'Address', render: item => item.cells.address },
        { key: 'age', label: 'Age', render: item => formatAge(item.cells.age ?? undefined) },
        // Hidden by default columns
        { key: 'paths', label: 'Paths', defaultHidden: true, render: item => item.cells.paths },
        {
            key: 'tls',
            label: 'TLS',
            defaultHidden: true,
            render: item => item.cells.tlsCount === 0
                ? <span className="text-gray-500">No</span>
                : <span className="text-green-400">Yes ({item.cells.tlsCount})</span>,
        },
        { key: 'tlsCount', label: 'TLS Count', defaultHidden: true, render: item => item.cells.tlsCount },
        {
            key: 'defaultBackend',
            label: 'Default Backend',
            defaultHidden: true,
            // The cell of this column is the service name only (its sort and filter value); the display adds the port.
            render: item => item.cells.defaultBackendTarget || <span className="text-gray-500">-</span>,
        },
        { key: 'defaultBackendTarget', label: 'Default Backend Target', defaultHidden: true, render: item => item.cells.defaultBackendTarget },
        { key: 'rules', label: 'Rules', defaultHidden: true, render: item => item.cells.rules },
    ], [{
        key: 'actions',
        label: <EllipsisVerticalIcon className="h-5 w-5" />,
        align: 'center',
        render: (item: any) => (
            <MenuBound
                as={IngressActionsMenu}
                menuId={`ingress-${item.metadata.uid}`}
                ingress={item}
                onEditYaml={deps.onEditYaml}
                onShowDependencies={deps.onShowDependencies}
                onDelete={deps.onDelete}
            />
        ),
        isColumnSelector: true,
        disableSort: true,
    }]);
}
