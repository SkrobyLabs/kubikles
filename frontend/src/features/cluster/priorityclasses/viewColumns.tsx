import React from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import MenuBound from '~/components/shared/MenuBound';
import { formatAge } from '~/utils/formatting';
import { defineViewColumns, type ViewColumn } from '~/lib/views/viewColumns';
import PriorityClassActionsMenu from './PriorityClassActionsMenu';

export function formatPriorityValue(value: any): string {
    if (value >= 1000000000) return `${(value / 1000000000).toFixed(1)}B`;
    if (value >= 1000000) return `${(value / 1000000).toFixed(1)}M`;
    if (value >= 1000) return `${(value / 1000).toFixed(1)}K`;
    return value?.toString() || '0';
}

export interface PriorityClassViewColumnDeps {
    onEditYaml: (item: any) => void;
    onShowDependencies: (item: any) => void;
    onDelete: (item: any) => void;
}

/** Presentation of the windowed PriorityClass list. */
export function buildPriorityClassViewColumns(deps: PriorityClassViewColumnDeps): ViewColumn[] {
    return defineViewColumns('priorityclasses', [
        { key: 'name', label: 'Name', render: item => item.metadata.name },
        { key: 'value', label: 'Value', render: item => <span className="font-mono">{formatPriorityValue(item.cells.value)}</span> },
        {
            key: 'globalDefault',
            label: 'Global Default',
            render: item => (
                <span className={item.cells.globalDefault === 'Yes' ? 'text-green-400' : 'text-gray-500'}>{item.cells.globalDefault}</span>
            ),
        },
        { key: 'preemption', label: 'Preemption', render: item => item.cells.preemption },
        { key: 'age', label: 'Age', render: item => formatAge(item.cells.age ?? undefined) },
    ], [{
        key: 'actions',
        label: <EllipsisVerticalIcon className="h-5 w-5" />,
        align: 'center',
        render: (item: any) => (
            <MenuBound
                as={PriorityClassActionsMenu}
                menuId={`priorityclass-${item.metadata.uid}`}
                priorityClass={item}
                onEditYaml={deps.onEditYaml}
                onShowDependencies={deps.onShowDependencies}
                onDelete={deps.onDelete}
            />
        ),
        isColumnSelector: true,
        disableSort: true,
    }]);
}
