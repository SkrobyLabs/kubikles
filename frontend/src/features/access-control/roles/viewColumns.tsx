import React from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import MenuBound from '~/components/shared/MenuBound';
import { formatAge } from '~/utils/formatting';
import { defineViewColumns, type ViewColumn } from '~/lib/views/viewColumns';
import RoleActionsMenu from './RoleActionsMenu';

export interface RoleViewColumnDeps {
    onEditYaml: (item: any) => void;
    onDelete: (item: any) => void;
}

/** Presentation of the windowed Role list; data semantics come from the generated schema. */
export function buildRoleViewColumns(deps: RoleViewColumnDeps): ViewColumn[] {
    return defineViewColumns('roles', [
        { key: 'name', label: 'Name', render: item => item.metadata.name },
        { key: 'namespace', label: 'Namespace', render: item => item.metadata.namespace },
        { key: 'rules', label: 'Rules', align: 'center', render: item => item.cells.rules },
        { key: 'age', label: 'Age', render: item => formatAge(item.cells.age ?? undefined) },
    ], [{
        key: 'actions',
        label: <EllipsisVerticalIcon className="h-5 w-5" />,
        align: 'center',
        render: (item: any) => (
            <MenuBound
                as={RoleActionsMenu}
                menuId={`role-${item.metadata.uid}`}
                role={item}
                onEditYaml={deps.onEditYaml}
                onDelete={deps.onDelete}
            />
        ),
        isColumnSelector: true,
        disableSort: true,
    }]);
}
