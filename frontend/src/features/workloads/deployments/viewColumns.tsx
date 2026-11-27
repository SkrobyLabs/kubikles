import React from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import MenuBound from '~/components/shared/MenuBound';
import type { ValueStore } from '~/utils/valueStore';
import { formatAge } from '~/utils/formatting';
import { defineViewColumns, type ViewColumn } from '~/lib/views/viewColumns';
import DeploymentActionsMenu from './DeploymentActionsMenu';
import { PodSquares } from '../PodSquares';

/** Columns whose cells other columns render from; always projected (replicas sizes the loading placeholders). */
export const DEPLOYMENT_PINNED_COLUMNS = ['podRefs', 'readyText', 'imageList', 'replicas'] as const;

/** Pinned columns that only carry cells; Replicas is pinned and also a column of its own. */
export const DEPLOYMENT_CARRIER_COLUMNS = ['podRefs', 'readyText', 'imageList'] as const;

/** Schema columns the list hides until the user shows them. */
export const DEPLOYMENT_HIDDEN_BY_DEFAULT = [
    'strategy', 'replicas', 'updatedReplicas', 'availableReplicas', 'image', 'selector', 'readyText', 'imageList', 'podRefs',
] as const;

export interface DeploymentViewColumnDeps {
    /** Whether the pod source of the view has loaded (the view state's overlay "pods"). */
    podsAvailability: ValueStore<boolean | null>;
    onEditYaml: (item: any) => void;
    onShowDependencies: (item: any) => void;
    onRestart: (item: any) => void;
    onDelete: (item: any) => void;
    onViewLogs: (item: any) => void;
    onScale: (item: any) => void;
}

/** "k=v" pairs joined with commas; label keys and values never contain a comma. */
const labelEntries = (cell: string | null | undefined): string[] => (cell ? cell.split(',') : []);

/** Presentation of the windowed Deployment list; data semantics come from the generated schema. */
export function buildDeploymentViewColumns(deps: DeploymentViewColumnDeps): ViewColumn[] {
    return defineViewColumns('deployments', [
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
        { key: 'strategy', label: 'Strategy', defaultHidden: true, render: item => item.cells.strategy },
        { key: 'replicas', label: 'Replicas', defaultHidden: true, render: item => item.cells.replicas },
        { key: 'updatedReplicas', label: 'Updated', defaultHidden: true, render: item => item.cells.updatedReplicas },
        { key: 'availableReplicas', label: 'Available', defaultHidden: true, render: item => item.cells.availableReplicas },
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
        { key: 'readyText', label: 'Ready (text)', defaultHidden: true, render: item => item.cells.readyText },
        { key: 'imageList', label: 'Images', defaultHidden: true, render: item => item.cells.imageList },
        { key: 'podRefs', label: 'Pod list', defaultHidden: true, render: item => item.cells.podRefs },
    ], [{
        key: 'actions',
        label: <EllipsisVerticalIcon className="h-5 w-5" />,
        align: 'center',
        render: (item: any) => (
            <MenuBound
                as={DeploymentActionsMenu}
                menuId={`deployment-${item.metadata.uid}`}
                deployment={item}
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
