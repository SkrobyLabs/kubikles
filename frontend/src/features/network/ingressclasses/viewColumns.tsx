import React from 'react';
import { EllipsisVerticalIcon, CheckCircleIcon } from '@heroicons/react/24/outline';
import MenuBound from '~/components/shared/MenuBound';
import { formatAge } from '~/utils/formatting';
import { defineViewColumns, type ViewColumn } from '~/lib/views/viewColumns';
import IngressClassActionsMenu from './IngressClassActionsMenu';

export interface IngressClassViewColumnDeps {
    onEditYaml: (item: any) => void;
    onDelete: (item: any) => void;
}

/** Presentation of the windowed IngressClass list. */
export function buildIngressClassViewColumns(deps: IngressClassViewColumnDeps): ViewColumn[] {
    return defineViewColumns('ingressclasses', [
        { key: 'name', label: 'Name', render: item => item.metadata.name },
        { key: 'controller', label: 'Controller', render: item => item.cells.controller || '-' },
        {
            key: 'default',
            label: 'Default',
            render: item => item.cells.default === 'Yes'
                ? <CheckCircleIcon className="h-5 w-5 text-green-400" />
                : <span className="text-gray-500">-</span>,
        },
        { key: 'age', label: 'Age', render: item => formatAge(item.cells.age ?? undefined) },
    ], [{
        key: 'actions',
        label: <EllipsisVerticalIcon className="h-5 w-5" />,
        align: 'center',
        render: (item: any) => (
            <MenuBound
                as={IngressClassActionsMenu}
                menuId={`ingressclass-${item.metadata.uid}`}
                ingressClass={item}
                onEditYaml={deps.onEditYaml}
                onDelete={deps.onDelete}
            />
        ),
        isColumnSelector: true,
        disableSort: true,
    }]);
}
