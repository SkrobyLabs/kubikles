import React from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import MenuBound from '~/components/shared/MenuBound';
import type { ValueStore } from '~/utils/valueStore';
import { formatAge } from '~/utils/formatting';
import { defineViewColumns, type ViewColumn } from '~/lib/views/viewColumns';
import StatefulSetActionsMenu from './StatefulSetActionsMenu';
import { PodSquares } from '../PodSquares';

/** Columns whose cells other columns render from; always projected (replicas sizes the loading placeholders). */
export const STATEFULSET_PINNED_COLUMNS = ['podRefs', 'readyText', 'imageList', 'replicas'] as const;

/** Pinned columns that only carry cells; Replicas is pinned and also a column of its own. */
export const STATEFULSET_CARRIER_COLUMNS = ['podRefs', 'readyText', 'imageList'] as const;

/** Schema columns the list hides until the user shows them. */
export const STATEFULSET_HIDDEN_BY_DEFAULT = [
    'replicas', 'serviceName', 'updateStrategy', 'image', 'volumeClaims', 'readyText', 'imageList', 'podRefs',
] as const;

export interface StatefulSetViewColumnDeps {
    /** Whether the pod source of the view has loaded (the view state's overlay "pods"). */
    podsAvailability: ValueStore<boolean | null>;
    onEditYaml: (item: any) => void;
    onShowDependencies: (item: any) => void;
    onRestart: (item: any) => void;
    onDelete: (item: any) => void;
    onViewLogs: (item: any) => void;
    onScale: (item: any) => void;
}

const dash = <span className="text-gray-500">-</span>;

/** Presentation of the windowed StatefulSet list; data semantics come from the generated schema. */
export function buildStatefulSetViewColumns(deps: StatefulSetViewColumnDeps): ViewColumn[] {
    return defineViewColumns('statefulsets', [
        { key: 'name', label: 'Name', render: item => item.metadata.name },
        { key: 'namespace', label: 'Namespace', render: item => item.metadata.namespace },
        {
            key: 'pods',
            label: 'Pods',
            render: item => <PodSquares availability={deps.podsAvailability} refs={item.cells.podRefs} replicas={item.cells.replicas} />,
        },
        { key: 'ready', label: 'Ready', render: item => item.cells.readyText },
        { key: 'age', label: 'Age', render: item => formatAge(item.cells.age ?? undefined) },
        // Hidden by default columns
        { key: 'replicas', label: 'Replicas', defaultHidden: true, render: item => item.cells.replicas },
        { key: 'serviceName', label: 'Service Name', defaultHidden: true, render: item => item.cells.serviceName || dash },
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
        { key: 'volumeClaims', label: 'Volume Claims', defaultHidden: true, render: item => (item.cells.volumeClaims > 0 ? item.cells.volumeClaims : dash) },
        { key: 'readyText', label: 'Ready (text)', defaultHidden: true, render: item => item.cells.readyText },
        { key: 'imageList', label: 'Images', defaultHidden: true, render: item => item.cells.imageList },
        { key: 'podRefs', label: 'Pod list', defaultHidden: true, render: item => item.cells.podRefs },
    ], [{
        key: 'actions',
        label: <EllipsisVerticalIcon className="h-5 w-5" />,
        align: 'center',
        render: (item: any) => (
            <MenuBound
                as={StatefulSetActionsMenu}
                menuId={`statefulset-${item.metadata.uid}`}
                statefulSet={item}
                onEditYaml={() => deps.onEditYaml(item)}
                onShowDependencies={() => deps.onShowDependencies(item)}
                onRestart={() => deps.onRestart(item)}
                onDelete={() => deps.onDelete(item)}
                onViewLogs={() => deps.onViewLogs(item)}
                onScale={() => deps.onScale(item)}
            />
        ),
        isColumnSelector: true,
        disableSort: true,
    }]);
}
