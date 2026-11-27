import React from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import MenuBound from '~/components/shared/MenuBound';
import { formatAge } from '~/utils/formatting';
import { defineViewColumns, type ViewColumn } from '~/lib/views/viewColumns';
import PVCActionsMenu from './PVCActionsMenu';

export const getStatusColor = (phase: any) => {
    switch (phase) {
        case 'Bound':
            return 'text-green-400';
        case 'Pending':
            return 'text-yellow-400';
        case 'Lost':
            return 'text-red-400';
        default:
            return 'text-gray-400';
    }
};

const getAccessModeColor = (mode: any) => {
    switch (mode) {
        case 'ReadWriteOnce':
            return 'text-blue-400';
        case 'ReadOnlyMany':
            return 'text-yellow-400';
        case 'ReadWriteMany':
            return 'text-green-400';
        case 'ReadWriteOncePod':
            return 'text-purple-400';
        default:
            return 'text-gray-400';
    }
};

export const renderAccessModes = (modes: any) => {
    if (!modes || modes.length === 0) return '-';
    return (
        <span className="flex flex-wrap gap-1">
            {modes.map((mode: any, idx: number) => (
                <span key={idx} className={getAccessModeColor(mode)}>{mode}</span>
            ))}
        </span>
    );
};

/** Schema columns the list hides until the user shows them. */
export const PVC_HIDDEN_BY_DEFAULT = ['volumeMode', 'requested', 'selector'] as const;

export interface PVCViewColumnDeps {
    onEditYaml: (item: any) => void;
    onShowDependencies: (item: any) => void;
    onDelete: (item: any) => void;
}

/** Presentation of the windowed PersistentVolumeClaim list; data semantics come from the generated schema. */
export function buildPVCViewColumns(deps: PVCViewColumnDeps): ViewColumn[] {
    return defineViewColumns('persistentvolumeclaims', [
        { key: 'name', label: 'Name', render: item => item.metadata.name },
        { key: 'namespace', label: 'Namespace', render: item => item.metadata.namespace },
        {
            key: 'status',
            label: 'Status',
            render: item => <span className={getStatusColor(item.cells.status)}>{item.cells.status || 'Unknown'}</span>,
        },
        { key: 'volume', label: 'Volume', render: item => item.cells.volume || '-' },
        { key: 'capacity', label: 'Capacity', render: item => item.cells.capacity || '-' },
        // The cell is the modes joined with ", "; no access mode contains a comma.
        { key: 'accessModes', label: 'Access Modes', render: item => renderAccessModes(item.cells.accessModes ? item.cells.accessModes.split(', ') : []) },
        { key: 'storageClass', label: 'Storage Class', render: item => item.cells.storageClass || '-' },
        { key: 'age', label: 'Age', render: item => formatAge(item.cells.age ?? undefined) },
        // Hidden by default columns
        { key: 'volumeMode', label: 'Volume Mode', defaultHidden: true, render: item => item.cells.volumeMode },
        { key: 'requested', label: 'Requested', defaultHidden: true, render: item => item.cells.requested || '-' },
        {
            key: 'selector',
            label: 'Selector',
            defaultHidden: true,
            render: item => {
                // The cell is "k=v" pairs joined with commas; label keys and values never contain one.
                const entries: string[] = item.cells.selector ? item.cells.selector.split(',') : [];
                if (entries.length === 0) return <span className="text-gray-500">-</span>;
                return <span title={entries.join('\n')}>{entries.length} label{entries.length > 1 ? 's' : ''}</span>;
            },
        },
    ], [{
        key: 'actions',
        label: <EllipsisVerticalIcon className="h-5 w-5" />,
        align: 'center',
        render: (item: any) => (
            <MenuBound
                as={PVCActionsMenu}
                menuId={`pvc-${item.metadata.uid}`}
                pvc={item}
                onEditYaml={deps.onEditYaml}
                onShowDependencies={deps.onShowDependencies}
                onDelete={deps.onDelete}
            />
        ),
        isColumnSelector: true,
        disableSort: true,
    }]);
}
