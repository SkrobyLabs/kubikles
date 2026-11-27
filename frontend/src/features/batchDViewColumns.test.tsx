import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it, vi } from 'vitest';

// Batch D lists (pods, nodes and the namespace metrics columns): the column builders cover the
// generated schema exactly, render from cells only, hand actions the row identity, and the
// metrics cells show what the legacy cells show for the same sample.
vi.mock('~/components/shared/MenuBound', () => ({ default: () => null }));
vi.mock('~/components/shared/ComparisonMenuItems', () => ({ default: () => null }));

import { schemaColumnKeys } from '~/lib/views/schema';
import { createValueStore } from '~/utils/valueStore';
import { metricsRecordText, recordOf, POD_RECORD_FIELDS, NODE_RECORD_FIELDS, NAMESPACE_RECORD_FIELDS, parseMetricsRecord, fromRecord, transformPodMetrics } from '~/utils/metricsTransform';
import MenuBound from '~/components/shared/MenuBound';
import { buildPodViewColumns, menuPod } from './workloads/pods/viewColumns';
import { buildNodeViewColumns, menuNode } from './cluster/nodes/viewColumns';
import { buildNamespaceViewColumns } from './cluster/namespaces/viewColumns';
import { PodMetricCell, PodMetricViewCell } from './workloads/pods/PodMetricCells';
import { NodeMetricCell, NodeMetricViewCell } from './cluster/nodes/NodeMetricCells';
import { NamespaceMetricCell, NamespaceMetricViewCell } from './cluster/namespaces/NamespaceMetricCells';
import { podContainerStatesText } from './workloads/pods/semantics';
import { statusesFromCell } from './workloads/pods/PodCells';
import { text, recordingItem, rowItem, byKey } from './viewColumnTestSupport';

const noop = vi.fn();
const navigate = vi.fn();
const crdsStore = createValueStore<any>([]);
const availability = createValueStore<boolean | null>(true);

const podCols = buildPodViewColumns({
    renderMetric: (item, kind) => `metric:${kind}:${item.cells.metrics}`,
    crdsStore, navigateWithSearch: navigate, onLogs: noop, onShell: noop, onFiles: noop, onDelete: noop, onEvict: noop,
    onEditYaml: noop, onShowDependencies: noop, onShowDetails: noop, onPortForward: noop,
});
const nodeCols = buildNodeViewColumns({
    renderMetric: (_i, kind) => `metric:${kind}`, onEditYaml: noop, onCordonUncordon: noop, onShell: noop, onDelete: noop,
});
const nsCols = buildNamespaceViewColumns({ renderMetric: () => null, onEditYaml: noop, onDelete: noop });

const podCells = {
    name: 'p', namespace: 'ns', cpu: 12, memory: 3, containers: 8, status: 8, restarts: 4, age: null, controlledBy: 'Deployment',
    qos: 'Burstable', node: 'node-a', podIP: '10.0.0.1', hostIP: '192.168.0.1', serviceAccount: 'default', priorityClass: '', image: 'nginx:1',
    statusText: 'Running', phase: 'Running', containerStates: JSON.stringify([{ n: 'app', s: 'running', w: '', e: null }]),
    controllerRef: JSON.stringify({ kind: 'Deployment', name: 'web', uid: 'u-1', apiVersion: 'apps/v1' }), imageList: 'nginx:1\nenvoy:2', metrics: '1,2,3',
};
const nodeCells = {
    name: 'n', cpu: 10, memory: 20, pods: 5, conditions: 'NotReady,SchedulingDisabled', taints: 2, version: 'v1.29', roles: 'control-plane, etcd',
    internalIP: '10.0.0.5', externalIP: '', osImage: 'Ubuntu', kernelVersion: '5.15', containerRuntime: 'containerd://1', architecture: 'amd64', os: 'linux',
    age: null, taintList: 'a=b:NoSchedule\nc:NoExecute', unschedulable: 'true', metrics: '',
};

const sets: Array<[string, any[], Record<string, any>]> = [
    ['pods', podCols, podCells],
    ['nodes', nodeCols, nodeCells],
];

describe.each(sets)('%s view columns', (type, cols, cells) => {
    it('presents every schema column exactly once plus the actions column', () => {
        const keys = cols.map(c => c.key);
        for (const k of schemaColumnKeys(type)) expect(keys.filter(x => x === k)).toHaveLength(1);
        expect(keys.filter(k => !schemaColumnKeys(type).includes(k))).toEqual(['actions']);
        expect(byKey(cols, 'actions').isColumnSelector).toBe(true);
    });

    it('renders only from cells that the schema provides', () => {
        const allowed = new Set(schemaColumnKeys(type).map(k => `cells.${k}`));
        for (const col of cols.filter(c => c.key !== 'actions')) {
            const { item, reads } = recordingItem(cells);
            col.render(item);
            for (const r of reads) {
                if (r.startsWith('cells.')) expect(allowed.has(r), `${type}.${col.key} read ${r}`).toBe(true);
                else expect(['metadata.name', 'metadata.namespace', 'metadata.uid']).toContain(r);
            }
        }
    });

    it('has no client-side sort or filter semantics for schema columns', () => {
        for (const c of cols.filter(c => schemaColumnKeys(type).includes(c.key))) {
            expect(c.getFilterValue).toBeUndefined();
            expect(c.getNumericValue).toBeUndefined();
        }
    });
});

describe('pod view columns', () => {
    it('draws status, containers and the controller link from cells', () => {
        const item = rowItem(podCells);
        expect(renderToStaticMarkup(byKey(podCols, 'status').render(item))).toContain('>Running<');
        const squares = renderToStaticMarkup(byKey(podCols, 'containers').render(item));
        expect(squares).toContain('bg-success');
        expect(squares).toContain('app: running ()');
        expect(text(byKey(podCols, 'restarts').render(item))).toBe('4');
        expect(byKey(podCols, 'image').render(item).props.title).toBe('nginx:1\nenvoy:2');
        expect(text(byKey(podCols, 'node').render(rowItem({ ...podCells, node: '' })))).toBe('-');
    });

    it('shows a dash without a controller and the kind with one', () => {
        const withOwner = renderToStaticMarkup(byKey(podCols, 'controlledBy').render(rowItem(podCells)));
        expect(withOwner).toContain('Deployment');
        const without = renderToStaticMarkup(byKey(podCols, 'controlledBy').render(rowItem({ ...podCells, controllerRef: '', controlledBy: '' })));
        expect(without).toContain('-');
    });

    it('rebuilds the container statuses the legacy squares read, including the unset state', () => {
        const pod = { status: { containerStatuses: [
            { name: 'a', state: { running: { startedAt: null } } },
            { name: 'b', state: { waiting: { reason: 'CrashLoopBackOff' } } },
            { name: 'c', state: { terminated: { exitCode: 3 } } },
            { name: 'd', state: {} },
        ] } };
        const cell = JSON.parse(podContainerStatesText(pod));
        expect(cell).toEqual([
            { n: 'a', s: 'running', w: '', e: null }, { n: 'b', s: 'waiting', w: 'CrashLoopBackOff', e: null },
            { n: 'c', s: 'terminated', w: '', e: 3 }, { n: 'd', s: '', w: '', e: null },
        ]);
        const rebuilt = statusesFromCell(JSON.stringify(cell));
        expect(rebuilt.map(s => Object.keys(s.state)[0])).toEqual(['running', 'waiting', 'terminated', undefined]);
        expect(rebuilt[1].state.waiting.reason).toBe('CrashLoopBackOff');
        expect(rebuilt[2].state.terminated.exitCode).toBe(3);
    });

    it('hands the actions the row and the menu a pod that knows its phase', () => {
        const onShell = vi.fn();
        const onEvict = vi.fn();
        const cols = buildPodViewColumns({
            renderMetric: () => null, crdsStore, navigateWithSearch: navigate, onLogs: noop, onShell, onFiles: noop, onDelete: noop, onEvict,
            onEditYaml: noop, onShowDependencies: noop, onShowDetails: noop, onPortForward: noop,
        });
        const row = rowItem(podCells, { deletionTimestamp: 'deleting' });
        const el: any = byKey(cols, 'actions').render(row);
        expect(el.type).toBe(MenuBound);
        expect(el.props.pod.status.phase).toBe('Running');
        expect(el.props.pod.metadata.deletionTimestamp).toBe('deleting');
        el.props.onShell();
        el.props.onEvict();
        expect(onShell).toHaveBeenCalledWith(row);
        expect(onEvict).toHaveBeenCalledWith(row);
        expect(menuPod(rowItem({ phase: '' })).status.phase).toBeUndefined();
    });
});

describe('node view columns', () => {
    it('draws the conditions badges, taints tooltip and roles from cells', () => {
        const item = rowItem(nodeCells);
        const badges = renderToStaticMarkup(byKey(nodeCols, 'conditions').render(item));
        expect(badges).toContain('NotReady');
        expect(badges).toContain('bg-red-500/20');
        expect(badges).toContain('SchedulingDisabled');
        expect(badges).toContain('bg-yellow-500/20');
        expect(renderToStaticMarkup(byKey(nodeCols, 'conditions').render(rowItem({ ...nodeCells, conditions: 'Ready' })))).toContain('Ready');
        const taints = renderToStaticMarkup(byKey(nodeCols, 'taints').render(item));
        expect(taints).toContain('a=b:NoSchedule');
        expect(taints).toContain('c:NoExecute');
        expect(renderToStaticMarkup(byKey(nodeCols, 'taints').render(rowItem({ ...nodeCells, taints: 0, taintList: '' })))).toContain('-');
        expect(text(byKey(nodeCols, 'roles').render(item))).toBe('control-plane, etcd');
        expect(renderToStaticMarkup(byKey(nodeCols, 'externalIP').render(item))).toContain('-');
    });

    it('gives the actions menu a node that knows whether it is cordoned', () => {
        const row = rowItem(nodeCells);
        expect(menuNode(row).spec.unschedulable).toBe(true);
        expect(menuNode(rowItem({ ...nodeCells, unschedulable: '' })).spec.unschedulable).toBeUndefined();
        const onCordon = vi.fn();
        const cols = buildNodeViewColumns({ renderMetric: () => null, onEditYaml: noop, onCordonUncordon: onCordon, onShell: noop, onDelete: noop });
        const el: any = byKey(cols, 'actions').render(row);
        el.props.onCordonUncordon();
        expect(onCordon.mock.calls[0][0].spec.unschedulable).toBe(true);
        expect(onCordon.mock.calls[0][0].metadata.name).toBe('thing');
    });
});

describe('namespace view columns', () => {
    it('sorts the overlay columns in the backend like the legacy list', () => {
        const keys = nsCols.map(c => c.key);
        expect(keys).toEqual(['name', 'cpu', 'memory', 'status', 'age', 'metrics', 'actions']);
        expect(byKey(nsCols, 'cpu').disableSort).toBeUndefined();
        expect(byKey(nsCols, 'metrics').defaultHidden).toBe(true);
    });
});

describe('metrics cells', () => {
    const podSample = transformPodMetrics({ metrics: [{ namespace: 'ns', name: 'p', nodeName: 'n', cpuUsage: 250, memoryUsage: 1 << 30, cpuRequested: 500, memRequested: 1 << 29, cpuCommitted: 500, memCommitted: 1 << 30, nodeCpuCapacity: 4000, nodeMemCapacity: 8 << 30 }] });
    const sample = podSample['ns/p'];
    const record = metricsRecordText(recordOf(POD_RECORD_FIELDS, sample));
    const html = (el: any) => renderToStaticMarkup(el);

    it('round-trips a record through its cell text', () => {
        expect(fromRecord(POD_RECORD_FIELDS, parseMetricsRecord(record))).toEqual({ ...sample, nodeName: undefined });
        expect(parseMetricsRecord('')).toBeUndefined();
        expect(parseMetricsRecord(null)).toBeUndefined();
        expect(fromRecord(NODE_RECORD_FIELDS, [1, 2])).toBeUndefined();
        expect(NAMESPACE_RECORD_FIELDS).toHaveLength(13);
    });

    it('pod cells render the same markup in view mode as in the legacy list', () => {
        const item = { metadata: { name: 'p', namespace: 'ns' }, cells: { metrics: record } };
        for (const kind of ['cpu', 'memory'] as const) {
            const legacy = html(<PodMetricCell store={createValueStore({ metrics: podSample, available: true })} item={item} kind={kind} />);
            const view = html(<PodMetricViewCell availability={createValueStore<boolean | null>(true)} item={item} kind={kind} />);
            expect(view).toBe(legacy);
        }
    });

    it('shows N/A when the source is unavailable and a dash without a sample, in both paths', () => {
        const item = { metadata: { name: 'p', namespace: 'ns' }, cells: { metrics: '' } };
        const store = (a: boolean | null) => createValueStore<boolean | null>(a);
        expect(html(<PodMetricViewCell availability={store(false)} item={{ ...item, cells: { metrics: record } }} kind="cpu" />)).toContain('N/A');
        expect(html(<PodMetricViewCell availability={store(true)} item={item} kind="cpu" />)).toContain('--');
        expect(html(<PodMetricViewCell availability={store(null)} item={item} kind="memory" />)).toContain('--');
        expect(html(<NodeMetricViewCell availability={store(false)} item={item} kind="pods" />)).toContain('N/A');
        expect(html(<NodeMetricViewCell availability={store(true)} item={item} kind="pods" />)).toContain('--');
        expect(html(<NamespaceMetricViewCell availability={store(false)} item={item} kind="cpu" />)).toContain('N/A');
        expect(html(<NamespaceMetricViewCell availability={store(true)} item={item} kind="memory" />)).toContain('--');
        void availability;
    });

    it('node and namespace cells match their legacy cells', () => {
        const node = { cpuPercent: 40, memPercent: 20, cpuReservedPercent: 30, memReservedPercent: 10, cpuCommittedPercent: 50, memCommittedPercent: 25, podPercent: 5, cpuUsage: 1600, memoryUsage: 100, cpuCapacity: 4000, memCapacity: 500, cpuRequested: 1200, memRequested: 50, cpuCommitted: 2000, memCommitted: 125, podCount: 6, podCapacity: 110 };
        const nodeItem = { metadata: { name: 'n' }, cells: { metrics: metricsRecordText(recordOf(NODE_RECORD_FIELDS, node)) } };
        for (const kind of ['cpu', 'memory', 'pods'] as const) {
            expect(html(<NodeMetricViewCell availability={createValueStore<boolean | null>(true)} item={nodeItem} kind={kind} />))
                .toBe(html(<NodeMetricCell store={createValueStore({ metrics: { n: node }, available: true })} item={nodeItem} kind={kind} />));
        }
        const ns = { cpuUsagePercent: 4, memUsagePercent: 3, cpuReservedPercent: 2, memReservedPercent: 1, cpuCommittedPercent: 5, memCommittedPercent: 6, cpuUsage: 10, memUsage: 20, cpuRequested: 30, memRequested: 40, cpuCommitted: 50, memCommitted: 60, podCount: 7 };
        const nsItem = { metadata: { name: 'default' }, cells: { metrics: metricsRecordText(recordOf(NAMESPACE_RECORD_FIELDS, ns)) } };
        for (const kind of ['cpu', 'memory'] as const) {
            expect(html(<NamespaceMetricViewCell availability={createValueStore<boolean | null>(true)} item={nsItem} kind={kind} />))
                .toBe(html(<NamespaceMetricCell store={createValueStore({ metrics: { default: ns }, available: true })} item={nsItem} kind={kind} />));
        }
    });
});
