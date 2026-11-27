import React from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import MenuBound from '~/components/shared/MenuBound';
import { formatAge } from '~/utils/formatting';
import { defineViewColumns, type ViewColumn } from '~/lib/views/viewColumns';
import ClusterRoleActionsMenu from './ClusterRoleActionsMenu';

export interface ClusterRoleViewColumnDeps {
    onEditYaml: (item: any) => void;
    onDelete: (item: any) => void;
}

/** Presentation of the windowed ClusterRole list. */
export function buildClusterRoleViewColumns(deps: ClusterRoleViewColumnDeps): ViewColumn[] {
    return defineViewColumns('clusterroles', [
        { key: 'name', label: 'Name', render: item => item.metadata.name },
        { key: 'rules', label: 'Rules', align: 'center', render: item => item.cells.rules },
        {
            key: 'aggregation',
            label: 'Aggregation',
            render: item => item.cells.aggregation === 'Yes'
                ? <span className="text-blue-400">Aggregated</span>
                : <span className="text-gray-500">-</span>,
        },
        { key: 'age', label: 'Age', render: item => formatAge(item.cells.age ?? undefined) },
    ], [{
        key: 'actions',
        label: <EllipsisVerticalIcon className="h-5 w-5" />,
        align: 'center',
        render: (item: any) => (
            <MenuBound
                as={ClusterRoleActionsMenu}
                menuId={`clusterrole-${item.metadata.uid}`}
                clusterRole={item}
                onEditYaml={deps.onEditYaml}
                onDelete={deps.onDelete}
            />
        ),
        isColumnSelector: true,
        disableSort: true,
    }]);
}
