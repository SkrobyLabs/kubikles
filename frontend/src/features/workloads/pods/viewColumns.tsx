import React from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import MenuBound from '~/components/shared/MenuBound';
import { formatAge } from '~/utils/formatting';
import { defineViewColumns, type ViewColumn } from '~/lib/views/viewColumns';
import type { ValueStore } from '~/utils/valueStore';
import PodActionsMenu from './PodActionsMenu';
import { ContainerSquares, PodControllerLink, PodStatusText, renderImages, statusesFromCell, type PodController } from './PodCells';

/** Columns whose cells other columns render from; always projected. */
export const POD_PINNED_COLUMNS = ['statusText', 'phase', 'containerStates', 'controllerRef', 'imageList', 'metrics'] as const;

/** Schema columns the list hides until the user shows them. */
export const POD_HIDDEN_BY_DEFAULT = [
    'qos', 'node', 'podIP', 'hostIP', 'serviceAccount', 'priorityClass', 'image',
    'statusText', 'phase', 'containerStates', 'controllerRef', 'imageList', 'metrics',
] as const;

export interface PodViewColumnDeps {
    /** The metric bars read the row's metrics cell and the overlay availability, so the list supplies the cell. */
    renderMetric: (item: any, kind: 'cpu' | 'memory') => React.ReactNode;
    crdsStore: ValueStore<any>;
    navigateWithSearch: (viewId: string, search: string) => void;
    onLogs: (item: any) => void;
    onShell: (item: any) => void;
    onFiles: (item: any) => void;
    onDelete: (item: any) => void;
    onEvict: (item: any) => void;
    onEditYaml: (item: any) => void;
    onShowDependencies: (item: any) => void;
    onShowDetails: (item: any) => void;
    onPortForward: (item: any) => void;
}

const dash = <span className="text-gray-500">-</span>;
const orDash = (v: string | null | undefined) => (v ? v : dash);

function parseController(text: string | null | undefined): PodController | null {
    if (!text) return null;
    try { return JSON.parse(text) as PodController; } catch { return null; }
}

/** The pod as the actions menu reads it: identity, whether it is terminating, and its phase. */
export function menuPod(item: any): any {
    return { metadata: item.metadata, status: { phase: item.cells?.phase || undefined } };
}

/** Presentation of the windowed Pod list. CPU and memory are overlay columns. */
export function buildPodViewColumns(deps: PodViewColumnDeps): ViewColumn[] {
    return defineViewColumns('pods', [
        { key: 'name', label: 'Name', render: item => item.metadata.name },
        { key: 'namespace', label: 'Namespace', render: item => item.metadata.namespace },
        { key: 'cpu', label: 'CPU', numericHint: 'CPU usage % (0-100)', render: item => deps.renderMetric(item, 'cpu') },
        { key: 'memory', label: 'Memory', numericHint: 'Memory usage % (0-100)', render: item => deps.renderMetric(item, 'memory') },
        { key: 'containers', label: 'Containers', render: item => <ContainerSquares statuses={statusesFromCell(item.cells.containerStates)} /> },
        { key: 'status', label: 'Status', render: item => <PodStatusText status={item.cells.statusText} /> },
        { key: 'restarts', label: 'Restarts', numericHint: 'Total restart count', render: item => item.cells.restarts },
        { key: 'age', label: 'Age', numericHint: 'Age in hours', render: item => formatAge(item.cells.age ?? undefined) },
        {
            key: 'controlledBy',
            label: 'Controlled By',
            render: item => <PodControllerLink controller={parseController(item.cells.controllerRef)} crdsStore={deps.crdsStore} navigateWithSearch={deps.navigateWithSearch} />,
        },
        // Hidden by default columns
        { key: 'qos', label: 'QoS', defaultHidden: true, render: item => item.cells.qos || '-' },
        { key: 'node', label: 'Node', defaultHidden: true, render: item => orDash(item.cells.node) },
        { key: 'podIP', label: 'Pod IP', defaultHidden: true, render: item => orDash(item.cells.podIP) },
        { key: 'hostIP', label: 'Host IP', defaultHidden: true, render: item => orDash(item.cells.hostIP) },
        { key: 'serviceAccount', label: 'Service Account', defaultHidden: true, render: item => item.cells.serviceAccount },
        { key: 'priorityClass', label: 'Priority Class', defaultHidden: true, render: item => orDash(item.cells.priorityClass) },
        { key: 'image', label: 'Image', defaultHidden: true, render: item => renderImages(item.cells.imageList ? item.cells.imageList.split('\n') : []) },
        { key: 'statusText', label: 'Status Text', defaultHidden: true, render: item => item.cells.statusText },
        { key: 'phase', label: 'Phase', defaultHidden: true, render: item => item.cells.phase },
        { key: 'containerStates', label: 'Container States', defaultHidden: true, render: item => item.cells.containerStates },
        { key: 'controllerRef', label: 'Controller', defaultHidden: true, render: item => item.cells.controllerRef },
        { key: 'imageList', label: 'Images', defaultHidden: true, render: item => item.cells.imageList },
        { key: 'metrics', label: 'Metrics', defaultHidden: true, render: item => item.cells.metrics },
    ], [{
        key: 'actions',
        label: <EllipsisVerticalIcon className="h-5 w-5" />,
        align: 'center',
        render: (item: any) => (
            <MenuBound
                as={PodActionsMenu}
                menuId={`pod-${item.metadata.uid}`}
                pod={menuPod(item)}
                onLogs={() => deps.onLogs(item)}
                onShell={() => deps.onShell(item)}
                onFiles={() => deps.onFiles(item)}
                onDelete={() => deps.onDelete(item)}
                onForceDelete={() => deps.onDelete(item)}
                onEvict={() => deps.onEvict(item)}
                onEditYaml={() => deps.onEditYaml(item)}
                onShowDependencies={() => deps.onShowDependencies(item)}
                onShowDetails={() => deps.onShowDetails(item)}
                onPortForward={() => deps.onPortForward(item)}
            />
        ),
        isColumnSelector: true,
        disableSort: true,
    }]);
}
