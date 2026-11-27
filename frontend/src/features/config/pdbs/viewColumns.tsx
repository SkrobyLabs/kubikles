import React from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import MenuBound from '~/components/shared/MenuBound';
import { formatAge } from '~/utils/formatting';
import { defineViewColumns, type ViewColumn } from '~/lib/views/viewColumns';
import PDBActionsMenu from './PDBActionsMenu';

export interface PDBViewColumnDeps {
    onEditYaml: (item: any) => void;
    onShowDependencies: (item: any) => void;
    onDelete: (item: any) => void;
}

/** Presentation of the windowed PodDisruptionBudget list; data semantics come from the generated schema. */
export function buildPDBViewColumns(deps: PDBViewColumnDeps): ViewColumn[] {
    return defineViewColumns('pdbs', [
        { key: 'name', label: 'Name', render: item => item.metadata.name },
        { key: 'namespace', label: 'Namespace', render: item => item.metadata.namespace },
        { key: 'minAvailable', label: 'Min Available', render: item => item.cells.minAvailable },
        { key: 'maxUnavailable', label: 'Max Unavailable', render: item => item.cells.maxUnavailable },
        { key: 'allowed', label: 'Allowed Disruptions', render: item => item.cells.allowed },
        { key: 'age', label: 'Age', render: item => formatAge(item.cells.age ?? undefined) },
    ], [{
        key: 'actions',
        label: <EllipsisVerticalIcon className="h-5 w-5" />,
        align: 'center',
        render: (item: any) => (
            <MenuBound
                as={PDBActionsMenu}
                menuId={`pdb-${item.metadata.uid}`}
                pdb={item}
                onEditYaml={deps.onEditYaml}
                onShowDependencies={deps.onShowDependencies}
                onDelete={deps.onDelete}
            />
        ),
        isColumnSelector: true,
        disableSort: true,
    }]);
}
