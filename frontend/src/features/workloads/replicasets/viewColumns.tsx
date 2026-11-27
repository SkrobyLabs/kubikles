import React from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import MenuBound from '~/components/shared/MenuBound';
import { formatAge } from '~/utils/formatting';
import { defineViewColumns, type ViewColumn } from '~/lib/views/viewColumns';
import ReplicaSetActionsMenu from './ReplicaSetActionsMenu';
import { renderControlledBy, parseControllerRef } from '../ControlledBy';

/** Columns whose cells other columns render from; always projected. */
export const REPLICASET_PINNED_COLUMNS = ['controllerRef'] as const;

/** Schema columns the list hides until the user shows them. */
export const REPLICASET_HIDDEN_BY_DEFAULT = ['controllerRef'] as const;

export interface ReplicaSetViewColumnDeps {
    crds: any;
    navigateWithSearch: (viewId: string, search: string) => void;
    onEditYaml: (item: any) => void;
    onShowDependencies: (item: any) => void;
    onDelete: (item: any) => void;
    onViewLogs: (item: any) => void;
    onScale: (item: any) => void;
}

/** Presentation of the windowed ReplicaSet list; data semantics come from the generated schema. */
export function buildReplicaSetViewColumns(deps: ReplicaSetViewColumnDeps): ViewColumn[] {
    return defineViewColumns('replicasets', [
        { key: 'name', label: 'Name', render: item => item.metadata.name },
        { key: 'namespace', label: 'Namespace', render: item => item.metadata.namespace },
        { key: 'desired', label: 'Desired', render: item => item.cells.desired },
        { key: 'current', label: 'Current', render: item => item.cells.current },
        { key: 'ready', label: 'Ready', render: item => item.cells.ready },
        { key: 'age', label: 'Age', render: item => formatAge(item.cells.age ?? undefined) },
        {
            key: 'controlledBy',
            label: 'Controlled By',
            render: item => renderControlledBy(parseControllerRef(item.cells.controllerRef), deps.crds, deps.navigateWithSearch),
        },
        { key: 'controllerRef', label: 'Controller Reference', defaultHidden: true, render: item => item.cells.controllerRef },
    ], [{
        key: 'actions',
        label: <EllipsisVerticalIcon className="h-5 w-5" />,
        align: 'center',
        render: (item: any) => (
            <MenuBound
                as={ReplicaSetActionsMenu}
                menuId={`rs-${item.metadata.uid}`}
                replicaSet={item}
                onEditYaml={() => deps.onEditYaml(item)}
                onShowDependencies={() => deps.onShowDependencies(item)}
                onDelete={() => deps.onDelete(item)}
                onViewLogs={() => deps.onViewLogs(item)}
                onScale={() => deps.onScale(item)}
            />
        ),
        isColumnSelector: true,
        disableSort: true,
    }]);
}
