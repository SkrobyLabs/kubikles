import React from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import MenuBound from '~/components/shared/MenuBound';
import { formatAge } from '~/utils/formatting';
import { defineViewColumns, type ViewColumn } from '~/lib/views/viewColumns';
import RoleBindingActionsMenu from './RoleBindingActionsMenu';

export interface RoleBindingViewColumnDeps {
    onEditYaml: (item: any) => void;
    onDelete: (item: any) => void;
}

/** Presentation of the windowed RoleBinding list; data semantics come from the generated schema. */
export function buildRoleBindingViewColumns(deps: RoleBindingViewColumnDeps): ViewColumn[] {
    return defineViewColumns('rolebindings', [
        { key: 'name', label: 'Name', render: item => item.metadata.name },
        { key: 'namespace', label: 'Namespace', render: item => item.metadata.namespace },
        {
            key: 'roleRef',
            label: 'Role Ref',
            render: item => {
                // The cell is "Kind/Name"; a kind never contains a slash.
                const text: string = item.cells.roleRef;
                const at = text.indexOf('/');
                return (
                    <span>
                        <span className="text-gray-400">{text.slice(0, at)}/</span>
                        {text.slice(at + 1)}
                    </span>
                );
            },
        },
        { key: 'subjects', label: 'Subjects', align: 'center', render: item => item.cells.subjects },
        { key: 'age', label: 'Age', render: item => formatAge(item.cells.age ?? undefined) },
    ], [{
        key: 'actions',
        label: <EllipsisVerticalIcon className="h-5 w-5" />,
        align: 'center',
        render: (item: any) => (
            <MenuBound
                as={RoleBindingActionsMenu}
                menuId={`rolebinding-${item.metadata.uid}`}
                roleBinding={item}
                onEditYaml={deps.onEditYaml}
                onDelete={deps.onDelete}
            />
        ),
        isColumnSelector: true,
        disableSort: true,
    }]);
}
