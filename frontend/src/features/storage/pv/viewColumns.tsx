import React from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import MenuBound from '~/components/shared/MenuBound';
import { formatAge } from '~/utils/formatting';
import { defineViewColumns, type ViewColumn } from '~/lib/views/viewColumns';
import PVActionsMenu from './PVActionsMenu';

export const getStatusColor = (phase: any) => {
    switch (phase) {
        case 'Bound': return 'text-green-400';
        case 'Available': return 'text-blue-400';
        case 'Released': return 'text-yellow-400';
        case 'Failed': return 'text-red-400';
        default: return 'text-gray-400';
    }
};

export const getReclaimPolicyColor = (policy: any) => {
    switch (policy) {
        case 'Delete': return 'text-red-400';
        case 'Retain': return 'text-green-400';
        case 'Recycle': return 'text-yellow-400';
        default: return 'text-gray-400';
    }
};

export const getAccessModeColor = (mode: any) => {
    switch (mode) {
        case 'ReadWriteOnce': return 'text-blue-400';
        case 'ReadOnlyMany': return 'text-yellow-400';
        case 'ReadWriteMany': return 'text-green-400';
        case 'ReadWriteOncePod': return 'text-purple-400';
        default: return 'text-gray-400';
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

export interface PVViewColumnDeps {
    onEditYaml: (item: any) => void;
    onShowDependencies: (item: any) => void;
    onDelete: (item: any) => void;
}

/** Presentation of the windowed PersistentVolume list. */
export function buildPVViewColumns(deps: PVViewColumnDeps): ViewColumn[] {
    return defineViewColumns('persistentvolumes', [
        { key: 'name', label: 'Name', render: item => item.metadata.name },
        { key: 'capacity', label: 'Capacity', render: item => item.cells.capacity || '-' },
        // The cell is the modes joined with ", "; no access mode contains a comma.
        { key: 'accessModes', label: 'Access Modes', render: item => renderAccessModes(item.cells.accessModes ? item.cells.accessModes.split(', ') : []) },
        {
            key: 'reclaimPolicy',
            label: 'Reclaim Policy',
            render: item => item.cells.reclaimPolicy
                ? <span className={getReclaimPolicyColor(item.cells.reclaimPolicy)}>{item.cells.reclaimPolicy}</span>
                : '-',
        },
        {
            key: 'status',
            label: 'Status',
            render: item => <span className={getStatusColor(item.cells.status)}>{item.cells.status || 'Unknown'}</span>,
        },
        { key: 'claim', label: 'Claim', render: item => item.cells.claim || '-' },
        { key: 'storageClass', label: 'Storage Class', render: item => item.cells.storageClass || '-' },
        { key: 'age', label: 'Age', render: item => formatAge(item.cells.age ?? undefined) },
    ], [{
        key: 'actions',
        label: <EllipsisVerticalIcon className="h-5 w-5" />,
        align: 'center',
        render: (item: any) => (
            <MenuBound
                as={PVActionsMenu}
                menuId={`pv-${item.metadata.uid}`}
                pv={item}
                onEditYaml={deps.onEditYaml}
                onShowDependencies={deps.onShowDependencies}
                onDelete={deps.onDelete}
            />
        ),
        isColumnSelector: true,
        disableSort: true,
    }]);
}
