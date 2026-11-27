import React from 'react';
import { describe, expect, it, vi } from 'vitest';

// Batch B1 lists (namespaced configuration and policy types): column builders must cover the
// generated schema exactly, render from cells only (a recording proxy catches reads of anything
// the cell shape does not provide), and hand actions the row identity.
vi.mock('~/components/shared/MenuBound', () => ({ default: () => null }));

import { schemaColumnKeys } from '~/lib/views/schema';
import { buildHPAViewColumns } from './config/hpas/viewColumns';
import { buildPDBViewColumns } from './config/pdbs/viewColumns';
import { buildResourceQuotaViewColumns } from './config/resourcequotas/viewColumns';
import { buildLimitRangeViewColumns } from './config/limitranges/viewColumns';
import { buildLeaseViewColumns } from './config/leases/viewColumns';
import { buildNetworkPolicyViewColumns } from './network/networkpolicies/viewColumns';
import { buildServiceAccountViewColumns } from './access-control/serviceaccounts/viewColumns';
import { text, recordingItem } from './viewColumnTestSupport';

const noop = vi.fn();
const deps = { onEditYaml: noop, onShowDependencies: noop, onDelete: noop };
const sets: Array<[string, any[], Record<string, any>]> = [
    ['hpas', buildHPAViewColumns(deps),
        { name: 'x', namespace: 'ns', reference: 'Deployment/web', minmax: '1/5', replicas: 3, age: '2025-01-01T00:00:00Z' }],
    ['pdbs', buildPDBViewColumns(deps),
        { name: 'x', namespace: 'ns', minAvailable: '50%', maxUnavailable: '-', allowed: 1, age: null }],
    ['resourcequotas', buildResourceQuotaViewColumns(deps),
        { name: 'x', namespace: 'ns', resources: 4, age: null }],
    ['limitranges', buildLimitRangeViewColumns(deps),
        { name: 'x', namespace: 'ns', limits: 2, age: null }],
    ['leases', buildLeaseViewColumns(deps),
        { name: 'x', namespace: 'ns', holder: 'node-1', duration: 15, renewTime: '2025-06-01T11:59:30.123456Z', age: null }],
    ['networkpolicies', buildNetworkPolicyViewColumns(deps),
        { name: 'x', namespace: 'ns', podSelector: 'app=web, +1', policyTypes: 'Ingress, Egress', age: null }],
    ['serviceaccounts', buildServiceAccountViewColumns({ onEditYaml: noop, onDelete: noop }),
        { name: 'x', namespace: 'ns', secrets: 2, age: null }],
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

describe('hpas', () => {
    it('shows the replica count or the dash', () => {
        const col = buildHPAViewColumns(deps).find(c => c.key === 'replicas')!;
        expect(text(col.render(recordingItem({ replicas: 3 }).item))).toBe('3');
        expect(text(col.render(recordingItem({ replicas: '-' }).item))).toBe('-');
    });
});

describe('pdbs', () => {
    it('renders numbers, percentages and the dash as given', () => {
        const col = buildPDBViewColumns(deps).find(c => c.key === 'minAvailable')!;
        expect(text(col.render(recordingItem({ minAvailable: 0 }).item))).toBe('0');
        expect(text(col.render(recordingItem({ minAvailable: '50%' }).item))).toBe('50%');
        expect(text(col.render(recordingItem({ minAvailable: '-' }).item))).toBe('-');
    });
});

describe('leases', () => {
    const cols = buildLeaseViewColumns(deps);
    it('formats the duration in seconds with a dash for none', () => {
        const col = cols.find(c => c.key === 'duration')!;
        expect(text(col.render(recordingItem({ duration: 15 }).item))).toBe('15s');
        expect(text(col.render(recordingItem({ duration: 0 }).item))).toBe('-');
    });
    it('shows the renew time as an age with a dash when never renewed', () => {
        const col = cols.find(c => c.key === 'renewTime')!;
        expect(text(col.render(recordingItem({ renewTime: '' }).item))).toBe('-');
        expect(text(col.render(recordingItem({ renewTime: '2000-01-01T00:00:00.000000Z' }).item))).not.toBe('-');
    });
});

describe('serviceaccounts', () => {
    it('centres the secrets count', () => {
        const col = buildServiceAccountViewColumns({ onEditYaml: noop, onDelete: noop }).find(c => c.key === 'secrets')!;
        expect(col.align).toBe('center');
        expect(text(col.render(recordingItem({ secrets: 0 }).item))).toBe('0');
    });
});
