import React from 'react';
import { describe, expect, it, vi } from 'vitest';

// Batch A lists (cluster-scoped: classes, webhook configurations, CSI, volumes, RBAC): column builders must cover the
// generated schema exactly, render from cells only (a recording proxy catches reads of anything
// the cell shape does not provide), and hand actions the row identity.
vi.mock('~/components/shared/MenuBound', () => ({ default: () => null }));

import { schemaColumnKeys } from '~/lib/views/schema';
import { buildPriorityClassViewColumns, formatPriorityValue } from './cluster/priorityclasses/viewColumns';
import { buildIngressClassViewColumns } from './network/ingressclasses/viewColumns';
import { buildStorageClassViewColumns, STORAGE_CLASS_PINNED_COLUMNS } from './storage/storageclass/viewColumns';
import { buildValidatingWebhookViewColumns, buildMutatingWebhookViewColumns } from './cluster/webhooks/viewColumns';
import { buildCSIDriverViewColumns } from './storage/csidrivers/viewColumns';
import { buildCSINodeViewColumns } from './storage/csinodes/viewColumns';
import { buildPVViewColumns } from './storage/pv/viewColumns';
import { buildClusterRoleViewColumns } from './access-control/clusterroles/viewColumns';
import { buildClusterRoleBindingViewColumns } from './access-control/clusterrolebindings/viewColumns';
import { text, recordingItem } from './viewColumnTestSupport';

const noop = vi.fn();
const sets: Array<[string, any[], Record<string, any>]> = [
    ['priorityclasses', buildPriorityClassViewColumns({ onEditYaml: noop, onShowDependencies: noop, onDelete: noop }),
        { name: 'x', value: 1500000, globalDefault: 'Yes', preemption: 'Never', age: '2025-01-01T00:00:00Z' }],
    ['ingressclasses', buildIngressClassViewColumns({ onEditYaml: noop, onDelete: noop }),
        { name: 'x', controller: 'k8s.io/ingress-nginx', default: 'Yes', age: null }],
    ['storageclasses', buildStorageClassViewColumns({ onEditYaml: noop, onDelete: noop }),
        { name: 'x', provisioner: 'p', reclaimPolicy: 'Delete', volumeBindingMode: 'Immediate', allowVolumeExpansion: 'Yes', isDefault: 'Yes', age: null }],
    ['validatingwebhookconfigurations', buildValidatingWebhookViewColumns({ onEditYaml: noop, onShowDependencies: noop, onDelete: noop }),
        { name: 'x', webhooks: 2, failurePolicy: 'Fail, Ignore', age: null }],
    ['mutatingwebhookconfigurations', buildMutatingWebhookViewColumns({ onEditYaml: noop, onShowDependencies: noop, onDelete: noop }),
        { name: 'x', webhooks: 2, failurePolicy: 'Fail', reinvocation: 'Never, IfNeeded', age: null }],
    ['csidrivers', buildCSIDriverViewColumns({ onEditYaml: noop, onDelete: noop }),
        { name: 'x', attachRequired: 'Yes', podInfoOnMount: 'No', storageCapacity: 'Yes', volumeModes: 'Persistent, Ephemeral', age: null }],
    ['csinodes', buildCSINodeViewColumns({ onEditYaml: noop, onDelete: noop }),
        { name: 'x', driverCount: 2, driverNames: 'a, b', age: null }],
    ['persistentvolumes', buildPVViewColumns({ onEditYaml: noop, onShowDependencies: noop, onDelete: noop }),
        { name: 'x', capacity: '10Gi', accessModes: 'ReadWriteOnce, ReadOnlyMany', reclaimPolicy: 'Retain', status: 'Bound', claim: 'ns/pvc', storageClass: 'fast', age: null }],
    ['clusterroles', buildClusterRoleViewColumns({ onEditYaml: noop, onDelete: noop }),
        { name: 'x', rules: 3, aggregation: 'Yes', age: null }],
    ['clusterrolebindings', buildClusterRoleBindingViewColumns({ onEditYaml: noop, onDelete: noop }),
        { name: 'x', roleRef: 'ClusterRole/admin', subjects: 2, age: null }],
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
                else expect(['metadata.name', 'metadata.uid']).toContain(r);
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

describe('priorityclasses', () => {
    const cols = buildPriorityClassViewColumns({ onEditYaml: noop, onShowDependencies: noop, onDelete: noop });
    it('abbreviates values like the legacy list', () => {
        expect(formatPriorityValue(0)).toBe('0');
        expect(formatPriorityValue(1500)).toBe('1.5K');
        expect(formatPriorityValue(2000000000)).toBe('2.0B');
        const { item } = recordingItem({ value: 1000000 });
        expect(text(cols.find(c => c.key === 'value')!.render(item))).toBe('1.0M');
    });
});

describe('storageclasses', () => {
    const cols = buildStorageClassViewColumns({ onEditYaml: noop, onDelete: noop });
    it('shows the default badge from the pinned isDefault cell', () => {
        const name = cols.find(c => c.key === 'name')!;
        expect(text(name.render(recordingItem({ isDefault: 'Yes' }).item))).toContain('default');
        expect(text(name.render(recordingItem({ isDefault: 'No' }).item))).not.toContain('default');
        expect(STORAGE_CLASS_PINNED_COLUMNS).toContain('isDefault');
        expect(cols.find(c => c.key === 'isDefault')!.defaultHidden).toBe(true);
    });
    it('renders dashes for empty policies', () => {
        const { item } = recordingItem({ reclaimPolicy: '', volumeBindingMode: '', allowVolumeExpansion: 'No' });
        expect(text(cols.find(c => c.key === 'reclaimPolicy')!.render(item))).toBe('-');
        expect(text(cols.find(c => c.key === 'volumeBindingMode')!.render(item))).toBe('-');
    });
});

describe('ingressclasses', () => {
    it('renders a dash for a missing controller', () => {
        const cols = buildIngressClassViewColumns({ onEditYaml: noop, onDelete: noop });
        expect(text(cols.find(c => c.key === 'controller')!.render(recordingItem({ controller: '' }).item))).toBe('-');
    });
});

describe('webhook configurations', () => {
    it('shows counts and policies from cells', () => {
        const v = buildValidatingWebhookViewColumns({ onEditYaml: noop, onShowDependencies: noop, onDelete: noop });
        const m = buildMutatingWebhookViewColumns({ onEditYaml: noop, onShowDependencies: noop, onDelete: noop });
        const cells = { webhooks: 0, failurePolicy: '-', reinvocation: 'Never' };
        expect(text(v.find(c => c.key === 'webhooks')!.render(recordingItem(cells).item))).toBe('0');
        expect(text(v.find(c => c.key === 'failurePolicy')!.render(recordingItem(cells).item))).toBe('-');
        expect(text(m.find(c => c.key === 'reinvocation')!.render(recordingItem(cells).item))).toBe('Never');
    });
});

describe('csidrivers', () => {
    it('draws the boolean icons from Yes and No cells', () => {
        const cols = buildCSIDriverViewColumns({ onEditYaml: noop, onDelete: noop });
        const yes: any = cols.find(c => c.key === 'attachRequired')!.render(recordingItem({ attachRequired: 'Yes' }).item);
        const no: any = cols.find(c => c.key === 'attachRequired')!.render(recordingItem({ attachRequired: 'No' }).item);
        expect(yes.props.value).toBe(true);
        expect(no.props.value).toBe(false);
        expect(cols.find(c => c.key === 'attachRequired')!.align).toBe('center');
    });
});

describe('csinodes', () => {
    it('colours the driver count by whether any driver exists', () => {
        const cols = buildCSINodeViewColumns({ onEditYaml: noop, onDelete: noop });
        const some: any = cols.find(c => c.key === 'driverCount')!.render(recordingItem({ driverCount: 2 }).item);
        const none: any = cols.find(c => c.key === 'driverCount')!.render(recordingItem({ driverCount: 0 }).item);
        expect(some.props.className).toContain('green');
        expect(none.props.className).toContain('gray');
        expect(text(none)).toBe('0');
    });
});

describe('persistentvolumes', () => {
    const cols = buildPVViewColumns({ onEditYaml: noop, onShowDependencies: noop, onDelete: noop });
    it('splits the access mode cell into one badge per mode, a dash when empty', () => {
        const col = cols.find(c => c.key === 'accessModes')!;
        const el: any = col.render(recordingItem({ accessModes: 'ReadWriteOnce, ReadOnlyMany' }).item);
        expect(el.props.children).toHaveLength(2);
        expect(text(el)).toBe('ReadWriteOnceReadOnlyMany');
        expect(col.render(recordingItem({ accessModes: '' }).item)).toBe('-');
    });
    it('renders Unknown for a missing phase and dashes for empty cells', () => {
        expect(text(cols.find(c => c.key === 'status')!.render(recordingItem({ status: null }).item))).toBe('Unknown');
        const empty = { capacity: '', claim: '', storageClass: '', reclaimPolicy: '' };
        for (const k of ['capacity', 'claim', 'storageClass', 'reclaimPolicy']) {
            expect(text(cols.find(c => c.key === k)!.render(recordingItem(empty).item))).toBe('-');
        }
    });
});

describe('clusterroles', () => {
    it('marks aggregated roles', () => {
        const cols = buildClusterRoleViewColumns({ onEditYaml: noop, onDelete: noop });
        const col = cols.find(c => c.key === 'aggregation')!;
        expect(text(col.render(recordingItem({ aggregation: 'Yes' }).item))).toBe('Aggregated');
        expect(text(col.render(recordingItem({ aggregation: 'No' }).item))).toBe('-');
    });
});

describe('clusterrolebindings', () => {
    it('splits the role reference into a muted kind and the name', () => {
        const col = buildClusterRoleBindingViewColumns({ onEditYaml: noop, onDelete: noop }).find(c => c.key === 'roleRef')!;
        expect(text(col.render(recordingItem({ roleRef: 'ClusterRole/cluster-admin' }).item))).toBe('ClusterRole/cluster-admin');
        const el: any = col.render(recordingItem({ roleRef: 'ClusterRole/x' }).item);
        expect(text(el.props.children[0])).toBe('ClusterRole/');
        expect(text(col.render(recordingItem({ roleRef: '/' }).item))).toBe('/');
    });
});
