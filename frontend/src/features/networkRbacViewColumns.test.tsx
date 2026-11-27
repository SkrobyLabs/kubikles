import React from 'react';
import { describe, expect, it, vi } from 'vitest';

// Namespaced network and RBAC types: column builders must cover the generated
// schema exactly, render from cells only (a recording proxy catches reads of anything the cell
// shape does not provide), and hand actions the row identity.
vi.mock('~/components/shared/MenuBound', () => ({ default: () => null }));

import { schemaColumnKeys } from '~/lib/views/schema';
import { buildEndpointsViewColumns } from './network/endpoints/viewColumns';
import { buildEndpointSliceViewColumns } from './network/endpointslices/viewColumns';
import { buildIngressViewColumns } from './network/ingresses/viewColumns';
import { buildServiceViewColumns } from './network/services/viewColumns';
import { buildRoleViewColumns } from './access-control/roles/viewColumns';
import { buildRoleBindingViewColumns } from './access-control/rolebindings/viewColumns';
import { text, recordingItem } from './viewColumnTestSupport';

const noop = vi.fn();
const rbacDeps = { onEditYaml: noop, onDelete: noop };
const deps = { onEditYaml: noop, onShowDependencies: noop, onDelete: noop };
const sets: Array<[string, any[], Record<string, any>]> = [
    ['endpoints', buildEndpointsViewColumns(deps),
        { name: 'x', namespace: 'ns', endpoints: 3, ports: '80/TCP', readyCount: 2, age: null }],
    ['endpointslices', buildEndpointSliceViewColumns(deps),
        { name: 'x', namespace: 'ns', service: 'web', addressType: 'IPv4', endpoints: 3, ports: '80/TCP', readyCount: 2, age: null }],
    ['services', buildServiceViewColumns(deps),
        { name: 'x', namespace: 'ns', type: 'ClusterIP', clusterIP: '10.0.0.1', ports: '80/TCP', age: null, externalIP: '', externalAddresses: '', selector: 'app=web', sessionAffinity: 'None', externalName: '' }],
    ['ingresses', buildIngressViewColumns(deps),
        { name: 'x', namespace: 'ns', status: 'Active', class: 'nginx', hosts: 'a.example.com, b.example.com', address: '-', age: null, paths: '/', tls: 'Yes', tlsCount: 1, defaultBackend: '', defaultBackendTarget: '', rules: 1 }],
    ['roles', buildRoleViewColumns(rbacDeps),
        { name: 'x', namespace: 'ns', rules: 3, age: '2025-01-01T00:00:00Z' }],
    ['rolebindings', buildRoleBindingViewColumns(rbacDeps),
        { name: 'x', namespace: 'ns', roleRef: 'ClusterRole/admin', subjects: 2, age: null }],
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

describe('roles', () => {
    it('centres the rules count', () => {
        const col = buildRoleViewColumns(rbacDeps).find(c => c.key === 'rules')!;
        expect(col.align).toBe('center');
        expect(text(col.render(recordingItem({ rules: 0 }).item))).toBe('0');
    });
});

describe('rolebindings', () => {
    it('splits the role ref into a dimmed kind and the name', () => {
        const col = buildRoleBindingViewColumns(rbacDeps).find(c => c.key === 'roleRef')!;
        expect(text(col.render(recordingItem({ roleRef: 'Role/admin' }).item))).toBe('Role/admin');
    });
});

describe('endpoints and endpoint slices', () => {
    it('show ready over total in the endpoints column', () => {
        const a = buildEndpointsViewColumns(deps).find(c => c.key === 'endpoints')!;
        expect(text(a.render(recordingItem({ endpoints: 5, readyCount: 2 }).item))).toBe('2/5');
        const b = buildEndpointSliceViewColumns(deps).find(c => c.key === 'endpoints')!;
        expect(text(b.render(recordingItem({ endpoints: 0, readyCount: 0 }).item))).toBe('0/0');
    });
    it('hides the ready count by default but keeps it projected', () => {
        expect(buildEndpointsViewColumns(deps).find(c => c.key === 'readyCount')!.defaultHidden).toBe(true);
        expect(buildEndpointSliceViewColumns(deps).find(c => c.key === 'readyCount')!.defaultHidden).toBe(true);
    });
    it('shows the dash for a slice without an address type', () => {
        const col = buildEndpointSliceViewColumns(deps).find(c => c.key === 'addressType')!;
        expect(text(col.render(recordingItem({ addressType: '' }).item))).toBe('-');
    });
});

describe('services', () => {
    const cols = buildServiceViewColumns(deps);
    const col = (k: string) => cols.find(c => c.key === k)!;
    it('lists every load balancer address in the external IP column', () => {
        expect(text(col('externalIP').render(recordingItem({ externalIP: '1.1.1.1', externalAddresses: '1.1.1.1, lb.example.com' }).item))).toBe('1.1.1.1, lb.example.com');
        expect(text(col('externalIP').render(recordingItem({ externalIP: '', externalAddresses: 'lb.example.com' }).item))).toBe('lb.example.com');
        expect(text(col('externalIP').render(recordingItem({ externalIP: '', externalAddresses: '' }).item))).toBe('-');
    });
    it('summarises the selector as a label count', () => {
        expect(text(col('selector').render(recordingItem({ selector: 'a=1' }).item))).toBe('1 label');
        expect(text(col('selector').render(recordingItem({ selector: 'a=1,b=2' }).item))).toBe('2 labels');
        expect(text(col('selector').render(recordingItem({ selector: '' }).item))).toBe('-');
    });
    it('shows the dash for no external name', () => {
        expect(text(col('externalName').render(recordingItem({ externalName: '' }).item))).toBe('-');
        expect(text(col('externalName').render(recordingItem({ externalName: 'db.example.com' }).item))).toBe('db.example.com');
    });
    it('hides the optional columns by default', () => {
        for (const k of ['externalIP', 'externalAddresses', 'selector', 'sessionAffinity', 'externalName']) expect(col(k).defaultHidden).toBe(true);
        for (const k of ['type', 'clusterIP', 'ports', 'age']) expect(col(k).defaultHidden).toBeFalsy();
    });
});

describe('ingresses', () => {
    const cols = buildIngressViewColumns(deps);
    const col = (k: string) => cols.find(c => c.key === k)!;
    it('colours the status', () => {
        const el: any = col('status').render(recordingItem({ status: 'No Rules' }).item);
        expect(el.props.className).toBe('text-red-400');
        expect((col('status').render(recordingItem({ status: 'Active' }).item) as any).props.className).toBe('text-green-400');
        expect((col('status').render(recordingItem({ status: 'Pending' }).item) as any).props.className).toBe('text-yellow-400');
    });
    it('renders one label per host, or the wildcard', () => {
        const el: any = col('hosts').render(recordingItem({ hosts: 'a.example.com, b.example.com' }).item);
        expect(React.Children.count(el.props.children)).toBe(2);
        expect(col('hosts').render(recordingItem({ hosts: '*' }).item)).toBe('*');
    });
    it('shows the TLS count and the default backend target', () => {
        expect(text(col('tls').render(recordingItem({ tlsCount: 0 }).item))).toBe('No');
        expect(text(col('tls').render(recordingItem({ tlsCount: 2 }).item))).toBe('Yes (2)');
        expect(text(col('defaultBackend').render(recordingItem({ defaultBackendTarget: '' }).item))).toBe('-');
        expect(text(col('defaultBackend').render(recordingItem({ defaultBackendTarget: 'svc:80' }).item))).toBe('svc:80');
    });
});
