import React from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import MenuBound from '~/components/shared/MenuBound';
import { formatAge } from '~/utils/formatting';
import { getJobConditionColor } from '~/utils/k8s-helpers';
import { defineViewColumns, type ViewColumn } from '~/lib/views/viewColumns';
import JobActionsMenu from './JobActionsMenu';
import { renderControlledBy, parseControllerRef } from '../ControlledBy';
import { formatJobDuration } from './semantics';

/** Columns whose cells other columns render from; always projected. */
export const JOB_PINNED_COLUMNS = ['completionsText', 'conditionText', 'startTime', 'completionTime', 'imageList', 'controllerRef'] as const;

/** Pinned columns that only carry cells. Start Time and Completion Time are pinned because Duration is counted from them, and are columns of their own. */
export const JOB_CARRIER_COLUMNS = ['completionsText', 'conditionText', 'imageList', 'controllerRef'] as const;

/** Schema columns the list hides until the user shows them. */
export const JOB_HIDDEN_BY_DEFAULT = [
    'parallelism', 'backoffLimit', 'activeDeadline', 'startTime', 'duration', 'image',
    'completionsText', 'conditionText', 'completionTime', 'imageList', 'controllerRef',
] as const;

export interface JobViewColumnDeps {
    crds: any;
    navigateWithSearch: (viewId: string, search: string) => void;
    onEditYaml: (item: any) => void;
    onShowDependencies: (item: any) => void;
    onDelete: (item: any) => void;
    onViewLogs: (item: any) => void;
}

const dash = <span className="text-gray-500">-</span>;

/** Presentation of the windowed Job list; data semantics come from the generated schema. */
export function buildJobViewColumns(deps: JobViewColumnDeps): ViewColumn[] {
    return defineViewColumns('jobs', [
        { key: 'name', label: 'Name', render: item => item.metadata.name },
        { key: 'namespace', label: 'Namespace', render: item => item.metadata.namespace },
        // The Completions and Condition cells are empty (as in the legacy list); the text is display-only.
        { key: 'completions', label: 'Completions', render: item => item.cells.completionsText },
        {
            key: 'condition',
            label: 'Condition',
            render: item => <span className={getJobConditionColor(item.cells.conditionText)}>{item.cells.conditionText}</span>,
        },
        { key: 'age', label: 'Age', render: item => formatAge(item.cells.age ?? undefined) },
        {
            key: 'controlledBy',
            label: 'Controlled By',
            render: item => renderControlledBy(parseControllerRef(item.cells.controllerRef), deps.crds, deps.navigateWithSearch),
        },
        // Hidden by default columns
        { key: 'parallelism', label: 'Parallelism', defaultHidden: true, render: item => item.cells.parallelism },
        { key: 'backoffLimit', label: 'Backoff Limit', defaultHidden: true, render: item => item.cells.backoffLimit },
        {
            key: 'activeDeadline',
            label: 'Deadline (s)',
            defaultHidden: true,
            // The cell is 0 without a deadline (the legacy sort value), and a deadline of 0 is not valid.
            render: item => item.cells.activeDeadline ? item.cells.activeDeadline : dash,
        },
        {
            key: 'startTime',
            label: 'Start Time',
            defaultHidden: true,
            render: item => item.cells.startTime ? formatAge(item.cells.startTime) + ' ago' : dash,
        },
        {
            key: 'duration',
            label: 'Duration',
            defaultHidden: true,
            // Counted from the start and completion times at render, as the legacy list does.
            render: item => formatJobDuration(item.cells.startTime || undefined, item.cells.completionTime || undefined) ?? dash,
        },
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
        { key: 'completionsText', label: 'Completions Text', defaultHidden: true, render: item => item.cells.completionsText },
        { key: 'conditionText', label: 'Condition Text', defaultHidden: true, render: item => item.cells.conditionText },
        { key: 'completionTime', label: 'Completion Time', defaultHidden: true, render: item => item.cells.completionTime },
        { key: 'imageList', label: 'Images', defaultHidden: true, render: item => item.cells.imageList },
        { key: 'controllerRef', label: 'Controller Reference', defaultHidden: true, render: item => item.cells.controllerRef },
    ], [{
        key: 'actions',
        label: <EllipsisVerticalIcon className="h-5 w-5" />,
        align: 'center',
        render: (item: any) => (
            <MenuBound
                as={JobActionsMenu}
                menuId={`job-${item.metadata.uid}`}
                job={item}
                onEditYaml={deps.onEditYaml}
                onShowDependencies={deps.onShowDependencies}
                onDelete={deps.onDelete}
                onViewLogs={deps.onViewLogs}
            />
        ),
        isColumnSelector: true,
        disableSort: true,
    }]);
}
