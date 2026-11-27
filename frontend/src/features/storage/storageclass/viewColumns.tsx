import React from 'react';
import { EllipsisVerticalIcon, CheckCircleIcon } from '@heroicons/react/24/outline';
import MenuBound from '~/components/shared/MenuBound';
import { formatAge } from '~/utils/formatting';
import { defineViewColumns, type ViewColumn } from '~/lib/views/viewColumns';
import StorageClassActionsMenu from './StorageClassActionsMenu';

export const getReclaimPolicyColor = (policy: any) => {
    switch (policy) {
        case 'Delete': return 'text-red-400';
        case 'Retain': return 'text-green-400';
        case 'Recycle': return 'text-yellow-400';
        default: return 'text-gray-400';
    }
};

export const getBindingModeColor = (mode: any) => {
    switch (mode) {
        case 'Immediate': return 'text-green-400';
        case 'WaitForFirstConsumer': return 'text-orange-400';
        default: return 'text-gray-400';
    }
};

/** Columns whose cells other columns render from; always projected. */
export const STORAGE_CLASS_PINNED_COLUMNS = ['isDefault'] as const;

export interface StorageClassViewColumnDeps {
    onEditYaml: (item: any) => void;
    onDelete: (item: any) => void;
}

/** Presentation of the windowed StorageClass list. */
export function buildStorageClassViewColumns(deps: StorageClassViewColumnDeps): ViewColumn[] {
    return defineViewColumns('storageclasses', [
        {
            key: 'name',
            label: 'Name',
            render: item => (
                <div className="flex items-center gap-2">
                    {item.metadata.name}
                    {item.cells.isDefault === 'Yes' && (
                        <span className="text-xs bg-blue-500/20 text-blue-400 px-1.5 py-0.5 rounded">default</span>
                    )}
                </div>
            ),
        },
        { key: 'provisioner', label: 'Provisioner', render: item => item.cells.provisioner || '-' },
        {
            key: 'reclaimPolicy',
            label: 'Reclaim Policy',
            render: item => item.cells.reclaimPolicy
                ? <span className={getReclaimPolicyColor(item.cells.reclaimPolicy)}>{item.cells.reclaimPolicy}</span>
                : '-',
        },
        {
            key: 'volumeBindingMode',
            label: 'Volume Binding Mode',
            render: item => item.cells.volumeBindingMode
                ? <span className={getBindingModeColor(item.cells.volumeBindingMode)}>{item.cells.volumeBindingMode}</span>
                : '-',
        },
        {
            key: 'allowVolumeExpansion',
            label: 'Allow Expansion',
            render: item => item.cells.allowVolumeExpansion === 'Yes'
                ? <CheckCircleIcon className="h-5 w-5 text-green-400" />
                : <span className="text-gray-500">-</span>,
        },
        { key: 'isDefault', label: 'Default', defaultHidden: true, render: item => item.cells.isDefault },
        { key: 'age', label: 'Age', render: item => formatAge(item.cells.age ?? undefined) },
    ], [{
        key: 'actions',
        label: <EllipsisVerticalIcon className="h-5 w-5" />,
        align: 'center',
        render: (item: any) => (
            <MenuBound
                as={StorageClassActionsMenu}
                menuId={`storageclass-${item.metadata.uid}`}
                storageClass={item}
                onEditYaml={deps.onEditYaml}
                onDelete={deps.onDelete}
            />
        ),
        isColumnSelector: true,
        disableSort: true,
    }]);
}
