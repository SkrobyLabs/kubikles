import React from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import MenuBound from '~/components/shared/MenuBound';
import { formatAge } from '~/utils/formatting';
import { defineViewColumns, type ViewColumn } from '~/lib/views/viewColumns';
import DaemonSetActionsMenu from './DaemonSetActionsMenu';

/** Columns whose cells other columns render from; always projected. */
export const DAEMONSET_PINNED_COLUMNS = ['imageList'] as const;

/** Schema columns the list hides until the user shows them. */
export const DAEMONSET_HIDDEN_BY_DEFAULT = ['upToDate', 'updateStrategy', 'image', 'selector', 'nodeSelector', 'imageList'] as const;

export interface DaemonSetViewColumnDeps {
    onEditYaml: (item: any) => void;
    onShowDependencies: (item: any) => void;
    onRestart: (item: any) => void;
    onDelete: (item: any) => void;
    onViewLogs: (item: any) => void;
}

/** "k=v" pairs joined with commas; label keys and values never contain a comma. */
const labelEntries = (cell: string | null | undefined): string[] => (cell ? cell.split(',') : []);

/** Presentation of the windowed DaemonSet list; data semantics come from the generated schema. */
export function buildDaemonSetViewColumns(deps: DaemonSetViewColumnDeps): ViewColumn[] {
    return defineViewColumns('daemonsets', [
        { key: 'name', label: 'Name', render: item => item.metadata.name },
        { key: 'namespace', label: 'Namespace', render: item => item.metadata.namespace },
        { key: 'desired', label: 'Desired', render: item => item.cells.desired },
        { key: 'current', label: 'Current', render: item => item.cells.current },
        { key: 'ready', label: 'Ready', render: item => item.cells.ready },
        { key: 'available', label: 'Available', render: item => item.cells.available },
        { key: 'age', label: 'Age', render: item => formatAge(item.cells.age ?? undefined) },
        // Hidden by default columns
        { key: 'upToDate', label: 'Up-to-date', defaultHidden: true, render: item => item.cells.upToDate },
        { key: 'updateStrategy', label: 'Update Strategy', defaultHidden: true, render: item => item.cells.updateStrategy },
        {
            key: 'image',
            label: 'Image',
            defaultHidden: true,
            render: item => {
                const images: string[] = item.cells.imageList ? item.cells.imageList.split('\n') : [];
                if (images.length === 0) return '-';
                if (images.length === 1) return <span title={images[0]}>{images[0].split('/').pop()}</span>;
                return <span title={images.join('\n')}>{images.length} images</span>;
            },
        },
        {
            key: 'selector',
            label: 'Selector',
            defaultHidden: true,
            render: item => {
                const entries = labelEntries(item.cells.selector);
                if (entries.length === 0) return '-';
                return <span title={entries.join('\n')}>{entries.length} label{entries.length > 1 ? 's' : ''}</span>;
            },
        },
        {
            key: 'nodeSelector',
            label: 'Node Selector',
            defaultHidden: true,
            render: item => {
                const entries = labelEntries(item.cells.nodeSelector);
                if (entries.length === 0) return <span className="text-gray-500">-</span>;
                return <span title={entries.join('\n')}>{entries.length} label{entries.length > 1 ? 's' : ''}</span>;
            },
        },
        { key: 'imageList', label: 'Images', defaultHidden: true, render: item => item.cells.imageList },
    ], [{
        key: 'actions',
        label: <EllipsisVerticalIcon className="h-5 w-5" />,
        align: 'center',
        render: (item: any) => (
            <MenuBound
                as={DaemonSetActionsMenu}
                menuId={`ds-${item.metadata.uid}`}
                daemonSet={item}
                onEditYaml={() => deps.onEditYaml(item)}
                onShowDependencies={() => deps.onShowDependencies(item)}
                onRestart={() => deps.onRestart(item)}
                onDelete={() => deps.onDelete(item)}
                onViewLogs={() => deps.onViewLogs(item)}
            />
        ),
        isColumnSelector: true,
        disableSort: true,
    }]);
}
