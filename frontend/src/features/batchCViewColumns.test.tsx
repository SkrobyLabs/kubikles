import React from 'react';
import { describe, expect, it, vi } from 'vitest';

// Batch C lists (replica sets, jobs, daemon sets, persistent volume claims): column builders must
// cover the generated schema exactly, render from cells only (a recording proxy catches reads of
// anything the cell shape does not provide), and hand actions the row identity.
vi.mock('~/components/shared/MenuBound', () => ({ default: () => null }));

import { schemaColumnKeys } from '~/lib/views/schema';
import { buildReplicaSetViewColumns } from './workloads/replicasets/viewColumns';
import { buildJobViewColumns } from './workloads/jobs/viewColumns';
import { buildDaemonSetViewColumns } from './workloads/daemonsets/viewColumns';
import { buildPVCViewColumns } from './storage/pvc/viewColumns';
import { formatJobDuration } from './workloads/jobs/semantics';
import { getJobConditionColor } from '~/utils/k8s-helpers';
import { text, recordingItem } from './viewColumnTestSupport';

const noop = vi.fn();
const navigate = vi.fn();
const base = { onEditYaml: noop, onShowDependencies: noop, onDelete: noop };
const ownerDeps = { ...base, crds: [], navigateWithSearch: navigate, onViewLogs: noop };
const controllerRef = JSON.stringify({ kind: 'Deployment', name: 'web', uid: 'u-1', apiVersion: 'apps/v1' });

const sets: Array<[string, any[], Record<string, any>]> = [
    ['replicasets', buildReplicaSetViewColumns({ ...ownerDeps, onScale: noop }),
        { name: 'x', namespace: 'ns', desired: 3, current: 3, ready: 2, age: null, controlledBy: 'Deployment', controllerRef }],
    ['jobs', buildJobViewColumns(ownerDeps),
        {
            name: null, namespace: null, completions: null, condition: null, age: null, controlledBy: '', parallelism: 1, backoffLimit: 6, activeDeadline: 0,
            startTime: '2025-06-01T11:00:00Z', duration: 3600000, image: 'nginx:1', completionsText: '1/3', conditionText: 'Complete', completionTime: '2025-06-01T12:00:00Z',
            imageList: 'nginx:1\nenvoy:2', controllerRef: '',
        }],
    ['daemonsets', buildDaemonSetViewColumns({ ...base, onRestart: noop, onViewLogs: noop }),
        { name: 'x', namespace: 'ns', desired: 3, current: 3, ready: 3, available: 3, age: null, upToDate: 3, updateStrategy: 'RollingUpdate', image: 'nginx:1', selector: 'app=web', nodeSelector: 'disk=ssd,zone=a', imageList: 'nginx:1' }],
    ['persistentvolumeclaims', buildPVCViewColumns(base),
        { name: 'x', namespace: 'ns', status: 'Bound', volume: 'pv-1', capacity: '1Gi', accessModes: 'ReadWriteOnce, ReadOnlyMany', storageClass: 'gp3', age: null, volumeMode: 'Filesystem', requested: '1Gi', selector: 'app=web' }],
];

describe.each(sets)('%s view columns', (type, cols, cells) => {
    it('presents every schema column exactly once plus the actions column', () => {
        const keys = cols.map(c => c.key);
        for (const k of schemaColumnKeys(type)) expect(keys.filter(x => x === k)).toHaveLength(1);
        expect(keys.filter(k => !schemaColumnKeys(type).includes(k))).toEqual(['actions']);
        expect(cols.find(c => c.key === 'actions')!.isColumnSelector).toBe(true);
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

    it('shows identity from the row metadata', () => {
        const { item } = recordingItem(cells);
        expect(text(cols.find(c => c.key === 'name')!.render(item))).toBe('thing');
        expect(text(cols.find(c => c.key === 'namespace')!.render(item))).toBe('ns');
    });
});

const col = (cols: any[], k: string) => cols.find(c => c.key === k)!;

describe('replicasets', () => {
    const cols = buildReplicaSetViewColumns({ ...ownerDeps, onScale: noop });
    it('links the controller of a built-in kind to its list', () => {
        const el: any = col(cols, 'controlledBy').render(recordingItem({ controllerRef }).item);
        expect(el.type).toBe('button');
        expect(text(el)).toBe('Deployment');
        el.props.onClick({ stopPropagation: noop });
        expect(navigate).toHaveBeenCalledWith('deployments', 'uid:"u-1"');
    });
    it('shows a dash without a controller and plain text for an unknown kind', () => {
        expect(text(col(cols, 'controlledBy').render(recordingItem({ controllerRef: '' }).item))).toBe('-');
        const unknown = JSON.stringify({ kind: 'Mystery', name: 'm', uid: 'u-2', apiVersion: 'x.io/v1' });
        const el: any = col(cols, 'controlledBy').render(recordingItem({ controllerRef: unknown }).item);
        expect(el.type).toBe('span');
        expect(el.props.title).toBe('m');
    });
    it('hides the controller reference by default', () => {
        expect(col(cols, 'controllerRef').defaultHidden).toBe(true);
        expect(col(cols, 'controlledBy').defaultHidden).toBeFalsy();
    });
});

describe('jobs', () => {
    const cols = buildJobViewColumns(ownerDeps);
    it('shows the completion and condition texts that the empty sort cells do not carry', () => {
        expect(text(col(cols, 'completions').render(recordingItem({ completionsText: '2/5' }).item))).toBe('2/5');
        const cond: any = col(cols, 'condition').render(recordingItem({ conditionText: 'Failed' }).item);
        expect(text(cond)).toBe('Failed');
        expect(cond.props.className).toBe(getJobConditionColor('Failed'));
    });
    it('counts the duration from the start and completion times like the legacy list', () => {
        const start = '2025-06-01T11:00:00Z';
        const end = '2025-06-01T12:05:30Z';
        expect(text(col(cols, 'duration').render(recordingItem({ startTime: start, completionTime: end }).item))).toBe(formatJobDuration(start, end));
        expect(formatJobDuration(start, end)).toBe('1h 5m');
        expect(formatJobDuration('2025-06-01T11:59:50Z', '2025-06-01T12:00:00Z')).toBe('10s');
        expect(formatJobDuration('2025-06-01T11:55:00Z', '2025-06-01T12:00:05Z')).toBe('5m 5s');
        expect(text(col(cols, 'duration').render(recordingItem({ startTime: '', completionTime: '' }).item))).toBe('-');
    });
    it('shows dashes for the optional spec fields', () => {
        expect(text(col(cols, 'activeDeadline').render(recordingItem({ activeDeadline: 0 }).item))).toBe('-');
        expect(text(col(cols, 'activeDeadline').render(recordingItem({ activeDeadline: 90 }).item))).toBe('90');
        expect(text(col(cols, 'startTime').render(recordingItem({ startTime: '' }).item))).toBe('-');
    });
    it('summarises images as the last path segment, a count for several, a dash for none', () => {
        expect(text(col(cols, 'image').render(recordingItem({ imageList: 'registry.io/team/app:v1' }).item))).toBe('app:v1');
        expect(text(col(cols, 'image').render(recordingItem({ imageList: 'a:1\nb:2' }).item))).toBe('2 images');
        expect(text(col(cols, 'image').render(recordingItem({ imageList: '' }).item))).toBe('-');
    });
    it('keeps the display-only columns hidden and the visible ones shown', () => {
        for (const k of ['completionsText', 'conditionText', 'completionTime', 'imageList', 'controllerRef', 'duration', 'image']) expect(col(cols, k).defaultHidden).toBe(true);
        for (const k of ['name', 'namespace', 'completions', 'condition', 'age', 'controlledBy']) expect(col(cols, k).defaultHidden).toBeFalsy();
    });
});

describe('daemonsets', () => {
    const cols = buildDaemonSetViewColumns({ ...base, onRestart: noop, onViewLogs: noop });
    it('summarises selectors as a label count', () => {
        expect(text(col(cols, 'selector').render(recordingItem({ selector: 'a=1' }).item))).toBe('1 label');
        expect(text(col(cols, 'selector').render(recordingItem({ selector: 'a=1,b=2' }).item))).toBe('2 labels');
        expect(text(col(cols, 'selector').render(recordingItem({ selector: '' }).item))).toBe('-');
        expect(text(col(cols, 'nodeSelector').render(recordingItem({ nodeSelector: 'a=1,b=2,c=3' }).item))).toBe('3 labels');
    });
    it('summarises images', () => {
        expect(text(col(cols, 'image').render(recordingItem({ imageList: 'nginx:1' }).item))).toBe('nginx:1');
        expect(text(col(cols, 'image').render(recordingItem({ imageList: 'a\nb\nc' }).item))).toBe('3 images');
    });
});

describe('persistentvolumeclaims', () => {
    const cols = buildPVCViewColumns(base);
    it('shows Unknown for a claim without a phase and colours the phase', () => {
        const el: any = col(cols, 'status').render(recordingItem({ status: null }).item);
        expect(text(el)).toBe('Unknown');
        expect(el.props.className).toBe('text-gray-400');
        expect((col(cols, 'status').render(recordingItem({ status: 'Bound' }).item) as any).props.className).toBe('text-green-400');
    });
    it('shows dashes for missing values and one label per access mode', () => {
        expect(text(col(cols, 'volume').render(recordingItem({ volume: '' }).item))).toBe('-');
        expect(text(col(cols, 'capacity').render(recordingItem({ capacity: '' }).item))).toBe('-');
        expect(text(col(cols, 'storageClass').render(recordingItem({ storageClass: '' }).item))).toBe('-');
        expect(text(col(cols, 'requested').render(recordingItem({ requested: '' }).item))).toBe('-');
        expect(text(col(cols, 'accessModes').render(recordingItem({ accessModes: '' }).item))).toBe('-');
        const modes: any = col(cols, 'accessModes').render(recordingItem({ accessModes: 'ReadWriteOnce, ReadOnlyMany' }).item);
        expect(React.Children.count(modes.props.children)).toBe(2);
    });
    it('summarises the selector as a label count', () => {
        expect(text(col(cols, 'selector').render(recordingItem({ selector: 'a=1,b=2' }).item))).toBe('2 labels');
        expect(text(col(cols, 'selector').render(recordingItem({ selector: '' }).item))).toBe('-');
    });
    it('hides the optional columns by default', () => {
        for (const k of ['volumeMode', 'requested', 'selector']) expect(col(cols, k).defaultHidden).toBe(true);
    });
});
