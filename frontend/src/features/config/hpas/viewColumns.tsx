import React from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import MenuBound from '~/components/shared/MenuBound';
import { formatAge } from '~/utils/formatting';
import { defineViewColumns, type ViewColumn } from '~/lib/views/viewColumns';
import HPAActionsMenu from './HPAActionsMenu';

export interface HPAViewColumnDeps {
    onEditYaml: (item: any) => void;
    onShowDependencies: (item: any) => void;
    onDelete: (item: any) => void;
}

/** Presentation of the windowed HorizontalPodAutoscaler list; data semantics come from the generated schema. */
export function buildHPAViewColumns(deps: HPAViewColumnDeps): ViewColumn[] {
    return defineViewColumns('hpas', [
        { key: 'name', label: 'Name', render: item => item.metadata.name },
        { key: 'namespace', label: 'Namespace', render: item => item.metadata.namespace },
        { key: 'reference', label: 'Reference', render: item => item.cells.reference },
        { key: 'minmax', label: 'Min/Max', render: item => item.cells.minmax },
        { key: 'replicas', label: 'Replicas', render: item => item.cells.replicas },
        { key: 'age', label: 'Age', render: item => formatAge(item.cells.age ?? undefined) },
    ], [{
        key: 'actions',
        label: <EllipsisVerticalIcon className="h-5 w-5" />,
        align: 'center',
        render: (item: any) => (
            <MenuBound
                as={HPAActionsMenu}
                menuId={`hpa-${item.metadata.uid}`}
                hpa={item}
                onEditYaml={deps.onEditYaml}
                onShowDependencies={deps.onShowDependencies}
                onDelete={deps.onDelete}
            />
        ),
        isColumnSelector: true,
        disableSort: true,
    }]);
}
