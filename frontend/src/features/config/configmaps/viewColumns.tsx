import React from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import MenuBound from '~/components/shared/MenuBound';
import { formatAge } from '~/utils/formatting';
import { defineViewColumns, type ViewColumn } from '~/lib/views/viewColumns';
import ConfigMapActionsMenu from './ConfigMapActionsMenu';

export interface ConfigMapViewColumnDeps {
    onEditYaml: (item: any) => void;
    onShowDependencies: (item: any) => void;
    onDelete: (item: any) => void;
}

/** Presentation of the windowed ConfigMap list; data semantics come from the generated schema. */
export function buildConfigMapViewColumns(deps: ConfigMapViewColumnDeps): ViewColumn[] {
    return defineViewColumns('configmaps', [
        { key: 'name', label: 'Name', render: item => item.metadata.name },
        { key: 'namespace', label: 'Namespace', render: item => item.metadata.namespace },
        { key: 'keys', label: 'Keys', render: item => item.cells.keys },
        { key: 'age', label: 'Age', render: item => formatAge(item.cells.age ?? undefined) },
        { key: 'keyCount', label: 'Key Count', defaultHidden: true, render: item => item.cells.keyCount },
        {
            key: 'binaryKeys',
            label: 'Binary Keys',
            defaultHidden: true,
            render: item => (item.cells.binaryKeys > 0 ? item.cells.binaryKeys : <span className="text-gray-500">-</span>),
        },
        {
            key: 'size',
            label: 'Size',
            defaultHidden: true,
            render: item => {
                const total = item.cells.size;
                if (total < 1024) return `${total} B`;
                return `${(total / 1024).toFixed(1)} KB`;
            },
        },
    ], [{
        key: 'actions',
        label: <EllipsisVerticalIcon className="h-5 w-5" />,
        align: 'center',
        render: (item: any) => (
            <MenuBound
                as={ConfigMapActionsMenu}
                menuId={`configmap-${item.metadata.uid}`}
                configMap={item}
                onEditYaml={deps.onEditYaml}
                onShowDependencies={deps.onShowDependencies}
                onDelete={deps.onDelete}
            />
        ),
        isColumnSelector: true,
        disableSort: true,
    }]);
}
