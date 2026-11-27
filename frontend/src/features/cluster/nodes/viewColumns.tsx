import React from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import MenuBound from '~/components/shared/MenuBound';
import { formatAge } from '~/utils/formatting';
import { defineViewColumns, type ViewColumn } from '~/lib/views/viewColumns';
import NodeActionsMenu from './NodeActionsMenu';
import { ConditionsTextCell, TaintsTextCell } from './NodeCells';

/** Columns whose cells other columns render from; always projected. */
export const NODE_PINNED_COLUMNS = ['taintList', 'unschedulable', 'metrics'] as const;

/** Schema columns the list hides until the user shows them. */
export const NODE_HIDDEN_BY_DEFAULT = [
    'roles', 'internalIP', 'externalIP', 'osImage', 'kernelVersion', 'containerRuntime', 'architecture', 'os',
    'taintList', 'unschedulable', 'metrics',
] as const;

export interface NodeViewColumnDeps {
    /** The metric bars read the row's metrics cell and the overlay availability, so the list supplies the cell. */
    renderMetric: (item: any, kind: 'cpu' | 'memory' | 'pods') => React.ReactNode;
    onEditYaml: (item: any) => void;
    onCordonUncordon: (item: any) => void;
    onShell: (item: any) => void;
    onDelete: (item: any) => void;
}

const dash = <span className="text-gray-500">-</span>;
const orDash = (v: string | null | undefined) => (v ? v : dash);

/** The node as the actions menu and handlers read it: identity plus the cordon state. */
export function menuNode(item: any): any {
    return { metadata: item.metadata, spec: { unschedulable: item.cells?.unschedulable === 'true' ? true : undefined } };
}

/** Presentation of the windowed Node list. CPU, memory and pods are overlay columns. */
export function buildNodeViewColumns(deps: NodeViewColumnDeps): ViewColumn[] {
    return defineViewColumns('nodes', [
        { key: 'name', label: 'Name', render: item => item.metadata.name },
        { key: 'cpu', label: 'CPU', numericHint: 'CPU usage % (0-100)', render: item => deps.renderMetric(item, 'cpu') },
        { key: 'memory', label: 'Memory', numericHint: 'Memory usage % (0-100)', render: item => deps.renderMetric(item, 'memory') },
        { key: 'pods', label: 'Pods', render: item => deps.renderMetric(item, 'pods') },
        { key: 'conditions', label: 'Conditions', render: item => <ConditionsTextCell text={item.cells.conditions} /> },
        { key: 'taints', label: 'Taints', align: 'center', render: item => <TaintsTextCell count={item.cells.taints} list={item.cells.taintList} /> },
        { key: 'version', label: 'Version', render: item => item.cells.version },
        // Hidden by default columns
        { key: 'roles', label: 'Roles', defaultHidden: true, render: item => orDash(item.cells.roles) },
        { key: 'internalIP', label: 'Internal IP', defaultHidden: true, render: item => orDash(item.cells.internalIP) },
        { key: 'externalIP', label: 'External IP', defaultHidden: true, render: item => orDash(item.cells.externalIP) },
        { key: 'osImage', label: 'OS Image', defaultHidden: true, render: item => item.cells.osImage || '-' },
        { key: 'kernelVersion', label: 'Kernel', defaultHidden: true, render: item => item.cells.kernelVersion || '-' },
        { key: 'containerRuntime', label: 'Container Runtime', defaultHidden: true, render: item => item.cells.containerRuntime || '-' },
        { key: 'architecture', label: 'Arch', defaultHidden: true, render: item => item.cells.architecture || '-' },
        { key: 'os', label: 'OS', defaultHidden: true, render: item => item.cells.os || '-' },
        { key: 'age', label: 'Age', numericHint: 'Age in hours', render: item => formatAge(item.cells.age ?? undefined) },
        { key: 'taintList', label: 'Taint List', defaultHidden: true, render: item => item.cells.taintList },
        { key: 'unschedulable', label: 'Unschedulable', defaultHidden: true, render: item => item.cells.unschedulable },
        { key: 'metrics', label: 'Metrics', defaultHidden: true, render: item => item.cells.metrics },
    ], [{
        key: 'actions',
        label: <EllipsisVerticalIcon className="h-5 w-5" />,
        align: 'center',
        render: (item: any) => (
            <MenuBound
                as={NodeActionsMenu}
                menuId={`node-${item.metadata.uid}`}
                node={menuNode(item)}
                onEditYaml={() => deps.onEditYaml(item)}
                onCordonUncordon={() => deps.onCordonUncordon(menuNode(item))}
                onShell={() => deps.onShell(item)}
                onDelete={() => deps.onDelete(item)}
            />
        ),
        isColumnSelector: true,
        disableSort: true,
    }]);
}
