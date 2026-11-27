import React from 'react';
import { describe, expect, it, vi } from 'vitest';
import { renderToStaticMarkup } from 'react-dom/server';

// Deployments, stateful sets: column builders must cover the generated schema
// exactly, render from cells only (a recording proxy catches reads of anything the cell shape does
// not provide), and draw the Pods squares from the podRefs cell.
vi.mock('~/components/shared/MenuBound', () => ({ default: () => null }));

import { schemaColumnKeys } from '~/lib/views/schema';
import { createValueStore } from '~/utils/valueStore';
import { buildDeploymentViewColumns } from './workloads/deployments/viewColumns';
import { buildStatefulSetViewColumns } from './workloads/statefulsets/viewColumns';
import { PodSquares, parsePodRefs } from './workloads/PodSquares';
import { getEffectivePodStatus, getPodStatusColor } from '~/utils/k8s-helpers';
import { podRefsText, placeholderSquares } from './workloads/podDerivation';
import { text, recordingItem } from './viewColumnTestSupport';

const noop = vi.fn();
const availability = createValueStore<boolean | null>(true);
const deps = { podsAvailability: availability, onEditYaml: noop, onShowDependencies: noop, onRestart: noop, onDelete: noop, onViewLogs: noop, onScale: noop };
const refs = JSON.stringify([['web-1', 'u1', 'Running'], ['web-2', 'u2', 'CrashLoopBackOff']]);

const sets: Array<[string, any[], Record<string, any>]> = [
    ['deployments', buildDeploymentViewColumns(deps),
        { name: 'x', namespace: 'ns', pods: 2, ready: 1, age: null, strategy: 'Recreate', replicas: 3, updatedReplicas: 1, availableReplicas: 1, image: 'nginx:1', selector: 'app=web', readyText: '1/3', imageList: 'nginx:1', podRefs: refs }],
    ['statefulsets', buildStatefulSetViewColumns(deps),
        { name: 'x', namespace: 'ns', pods: 2, ready: 1, age: null, replicas: 3, serviceName: 'db', updateStrategy: 'OnDelete', image: 'nginx:1', volumeClaims: 2, readyText: '1/3', imageList: 'nginx:1', podRefs: refs }],
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
            renderToStaticMarkup(<>{col.render(item)}</>);
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

    it('shows identity from the row metadata and the ready text', () => {
        const { item } = recordingItem(cells);
        expect(text(cols.find(c => c.key === 'name')!.render(item))).toBe('thing');
        expect(text(cols.find(c => c.key === 'namespace')!.render(item))).toBe('ns');
        expect(text(cols.find(c => c.key === 'ready')!.render(item))).toBe('1/3');
    });

    it('hides the optional and display-only columns by default', () => {
        for (const k of ['readyText', 'imageList', 'podRefs', 'replicas', 'image']) expect(cols.find(c => c.key === k)!.defaultHidden).toBe(true);
        for (const k of ['name', 'namespace', 'pods', 'ready', 'age']) expect(cols.find(c => c.key === k)!.defaultHidden).toBeFalsy();
    });

    it('draws one coloured square per selected pod with name and status in the tooltip', () => {
        const { item } = recordingItem(cells);
        const html = renderToStaticMarkup(<>{cols.find(c => c.key === 'pods')!.render(item)}</>);
        expect(html.match(/rounded-sm/g)).toHaveLength(2);
        expect(html).toContain('title="web-1: Running"');
        expect(html).toContain('title="web-2: CrashLoopBackOff"');
        expect(html).toContain(getPodStatusColor('Running').replace('text-', 'bg-'));
    });
});

describe('PodSquares', () => {
    const render = (available: boolean | null, podRefs: string, replicas: number | null) =>
        renderToStaticMarkup(<PodSquares availability={createValueStore<boolean | null>(available)} refs={podRefs} replicas={replicas} />);

    it('shows loading placeholders for the replicas while the pods have not loaded', () => {
        expect(render(false, '', 3).match(/animate-pulse/g)).toHaveLength(3);
        expect(render(null, '', 8).match(/animate-pulse/g)).toHaveLength(5);
        expect(render(false, '', 8)).toContain('...');
        expect(render(false, '', 0)).toBe('');
        // A workload without a replicas count is drawn as one replica, like the legacy list.
        expect(render(false, '', null).match(/animate-pulse/g)).toHaveLength(1);
    });

    it('shows nothing for a workload without pods once the pods have loaded', () => {
        expect(render(true, '', 3)).toBe('<div class="flex gap-1"></div>');
    });

    it('keeps the squares it has while loading', () => {
        expect(render(false, refs, 3).match(/rounded-sm/g)).toHaveLength(2);
        expect(render(false, refs, 3)).not.toContain('animate-pulse');
    });

    it('survives a malformed cell', () => {
        expect(parsePodRefs('not json')).toEqual([]);
        expect(parsePodRefs(null)).toEqual([]);
    });
});

describe('legacy helpers shared with the generator', () => {
    it('encodes the pod squares from the effective pod status', () => {
        const pod = { metadata: { name: 'p', uid: 'u' }, status: { phase: 'Running', containerStatuses: [{ state: { waiting: { reason: 'CrashLoopBackOff' } } }] } };
        expect(getEffectivePodStatus(pod)).toBe('CrashLoopBackOff');
        expect(JSON.parse(podRefsText([pod]))).toEqual([['p', 'u', 'CrashLoopBackOff']]);
        expect(podRefsText([])).toBe('');
    });
    it('sizes the placeholders like the legacy list', () => {
        expect(placeholderSquares(undefined)).toEqual({ count: 1, more: false });
        expect(placeholderSquares(0)).toEqual({ count: 0, more: false });
        expect(placeholderSquares(6)).toEqual({ count: 5, more: true });
    });
});
