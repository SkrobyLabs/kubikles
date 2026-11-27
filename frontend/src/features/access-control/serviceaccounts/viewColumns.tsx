import React from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import MenuBound from '~/components/shared/MenuBound';
import { formatAge } from '~/utils/formatting';
import { defineViewColumns, type ViewColumn } from '~/lib/views/viewColumns';
import ServiceAccountActionsMenu from './ServiceAccountActionsMenu';

export interface ServiceAccountViewColumnDeps {
    onEditYaml: (item: any) => void;
    onDelete: (item: any) => void;
}

/** Presentation of the windowed ServiceAccount list; data semantics come from the generated schema. */
export function buildServiceAccountViewColumns(deps: ServiceAccountViewColumnDeps): ViewColumn[] {
    return defineViewColumns('serviceaccounts', [
        { key: 'name', label: 'Name', render: item => item.metadata.name },
        { key: 'namespace', label: 'Namespace', render: item => item.metadata.namespace },
        { key: 'secrets', label: 'Secrets', align: 'center', render: item => item.cells.secrets },
        { key: 'age', label: 'Age', render: item => formatAge(item.cells.age ?? undefined) },
    ], [{
        key: 'actions',
        label: <EllipsisVerticalIcon className="h-5 w-5" />,
        align: 'center',
        render: (item: any) => (
            <MenuBound
                as={ServiceAccountActionsMenu}
                menuId={`serviceaccount-${item.metadata.uid}`}
                serviceAccount={item}
                onEditYaml={deps.onEditYaml}
                onDelete={deps.onDelete}
            />
        ),
        isColumnSelector: true,
        disableSort: true,
    }]);
}
