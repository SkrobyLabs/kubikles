import React from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import MenuBound from '~/components/shared/MenuBound';
import { formatAge } from '~/utils/formatting';
import { defineViewColumns, type ViewColumn } from '~/lib/views/viewColumns';
import SecretActionsMenu from './SecretActionsMenu';

export interface SecretViewColumnDeps {
    onEditYaml: (item: any) => void;
    onShowDependencies: (item: any) => void;
    onDelete: (item: any) => void;
}

/**
 * Presentation of the windowed Secret list. Rows come from a metadata-only listing, so Keys shows
 * the number of data keys and there is no Size column.
 */
export function buildSecretViewColumns(deps: SecretViewColumnDeps): ViewColumn[] {
    return defineViewColumns('secrets', [
        { key: 'name', label: 'Name', render: item => item.metadata.name },
        { key: 'namespace', label: 'Namespace', render: item => item.metadata.namespace },
        { key: 'type', label: 'Type', render: item => item.cells.type },
        { key: 'age', label: 'Age', render: item => formatAge(item.cells.age ?? undefined) },
        {
            key: 'keys',
            label: 'Keys',
            defaultHidden: true,
            render: item => {
                const n = item.cells.keys;
                if (!n) return <span className="text-gray-500">-</span>;
                return <span>{n} key{n > 1 ? 's' : ''}</span>;
            },
        },
    ], [{
        key: 'actions',
        label: <EllipsisVerticalIcon className="h-5 w-5" />,
        align: 'center',
        render: (item: any) => (
            <MenuBound
                as={SecretActionsMenu}
                menuId={`secret-${item.metadata.uid}`}
                secret={item}
                onEditYaml={deps.onEditYaml}
                onShowDependencies={deps.onShowDependencies}
                onDelete={deps.onDelete}
            />
        ),
        isColumnSelector: true,
        disableSort: true,
    }]);
}
