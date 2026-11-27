import React from 'react';
import { describe, expect, it, vi } from 'vitest';

// The windowed pilot lists (namespaces, configmaps, secrets, events): their column builders must
// cover the generated schema exactly, render from cells only, hand actions the row identity,
// and fetch the full object before anything needs spec or status.
vi.mock('~/components/shared/MenuBound', () => ({ default: () => null }));
vi.mock('~/components/shared/ComparisonMenuItems', () => ({ default: () => null }));

import { schemaColumnKeys } from '~/lib/views/schema';
import type { RowItem } from '~/lib/views/rowItem';
import { buildNamespaceViewColumns } from './cluster/namespaces/viewColumns';
import { buildConfigMapViewColumns } from './config/configmaps/viewColumns';
import { buildSecretViewColumns } from './config/secrets/viewColumns';
import { buildEventViewColumns, openInvolvedObject, getEventTypeColor } from './cluster/events/viewColumns';
import MenuBound from '~/components/shared/MenuBound';
import { text, byKey, rowItem } from './viewColumnTestSupport';

const item = rowItem;

describe('column sets', () => {
    const noop = vi.fn();
    const cases: Array<[string, any[]]> = [
        ['namespaces', buildNamespaceViewColumns({ renderMetric: () => null, onEditYaml: noop, onDelete: noop })],
        ['configmaps', buildConfigMapViewColumns({ onEditYaml: noop, onShowDependencies: noop, onDelete: noop })],
        ['secrets', buildSecretViewColumns({ onEditYaml: noop, onShowDependencies: noop, onDelete: noop })],
        ['events', buildEventViewColumns({ view: null, crds: [], navigateWithSearch: noop, onEditYaml: noop, onDelete: noop })],
    ];

    it.each(cases)('%s presents every schema column exactly once, plus UI-only columns', (type, cols) => {
        const keys = cols.map(c => c.key);
        for (const k of schemaColumnKeys(type)) expect(keys.filter(x => x === k)).toHaveLength(1);
        const extra = keys.filter(k => !schemaColumnKeys(type).includes(k));
        expect(extra.every(k => k === 'actions')).toBe(true);
        expect(byKey(cols, 'actions').isColumnSelector).toBe(true);
    });

    it.each(cases)('%s never defines client-side sort or filter semantics for schema columns', (type, cols) => {
        for (const c of cols.filter(c => schemaColumnKeys(type).includes(c.key))) {
            expect(c.getFilterValue).toBeUndefined();
            expect(c.getNumericValue).toBeUndefined();
        }
    });
});

describe('namespaces', () => {
    const noop = vi.fn();
    const cols = buildNamespaceViewColumns({ renderMetric: (_i, kind) => `metric:${kind}`, onEditYaml: noop, onDelete: noop });

    it('renders status from cells and sorts and filters the metrics columns like the legacy list', () => {
        expect(text(byKey(cols, 'status').render(item({ status: 'Terminating' })))).toBe('Terminating');
        expect(byKey(cols, 'status').render(item({ status: 'Active' })).props.className).toContain('green');
        expect(byKey(cols, 'cpu').render(item({}))).toBe('metric:cpu');
        // The overlay columns are schema columns: the backend sorts them, so they are not display-only.
        expect(byKey(cols, 'cpu').disableSort).toBeUndefined();
        expect(cols.map(c => c.key)).toEqual(['name', 'cpu', 'memory', 'status', 'age', 'metrics', 'actions']);
    });

    it('wires the row actions with the row itself', () => {
        const onEditYaml = vi.fn();
        const onDelete = vi.fn();
        const c = buildNamespaceViewColumns({ renderMetric: () => null, onEditYaml, onDelete });
        const row = item({ status: 'Active' });
        const el: any = byKey(c, 'actions').render(row);
        expect(el.type).toBe(MenuBound);
        expect(el.props.namespace).toBe(row);
        el.props.onEditYaml();
        el.props.onDelete();
        expect(onEditYaml).toHaveBeenCalledWith(row);
        expect(onDelete).toHaveBeenCalledWith(row);
    });
});

describe('configmaps', () => {
    const noop = vi.fn();
    const cols = buildConfigMapViewColumns({ onEditYaml: noop, onShowDependencies: noop, onDelete: noop });
    const row = item({ name: 'a', namespace: 'ns', keys: 'x, y', age: null, keyCount: 2, binaryKeys: 0, size: 2048 });

    it('renders every column from cells', () => {
        expect(byKey(cols, 'keys').render(row)).toBe('x, y');
        expect(byKey(cols, 'keyCount').render(row)).toBe(2);
        expect(text(byKey(cols, 'binaryKeys').render(row))).toBe('-');
        expect(byKey(cols, 'binaryKeys').render(item({ binaryKeys: 3 }))).toBe(3);
        expect(byKey(cols, 'size').render(row)).toBe('2.0 KB');
        expect(byKey(cols, 'size').render(item({ size: 12 }))).toBe('12 B');
        expect(byKey(cols, 'namespace').render(row)).toBe('ns');
    });

    it('hides the optional columns by default and gives actions only the row', () => {
        expect(cols.filter(c => c.defaultHidden).map(c => c.key)).toEqual(['keyCount', 'binaryKeys', 'size']);
        const el: any = byKey(cols, 'actions').render(row);
        expect(el.props.configMap).toBe(row);
    });
});

describe('secrets', () => {
    const noop = vi.fn();
    const cols = buildSecretViewColumns({ onEditYaml: noop, onShowDependencies: noop, onDelete: noop });

    it('shows a key count and no size column: data never reaches the list', () => {
        expect(cols.map(c => c.key)).not.toContain('size');
        expect(text(byKey(cols, 'keys').render(item({ keys: 0 })))).toBe('-');
        expect(text(byKey(cols, 'keys').render(item({ keys: 1 })))).toBe('1 key');
        expect(text(byKey(cols, 'keys').render(item({ keys: 3 })))).toBe('3 keys');
        expect(byKey(cols, 'type').render(item({ type: 'Opaque' }))).toBe('Opaque');
    });

    it('passes the identity item to every action', () => {
        const onDelete = vi.fn();
        const c = buildSecretViewColumns({ onEditYaml: noop, onShowDependencies: noop, onDelete });
        const row = item({ keys: 1 });
        const el: any = byKey(c, 'actions').render(row);
        expect(el.props.secret).toBe(row);
        el.props.onDelete(row);
        expect(onDelete).toHaveBeenCalledWith(row);
    });
});

describe('events', () => {
    const base = { crds: [] as any[], navigateWithSearch: vi.fn(), onEditYaml: vi.fn(), onDelete: vi.fn() };

    it('colours the type and renders the rest from cells', () => {
        const cols = buildEventViewColumns({ ...base, view: null });
        const row = item({ type: 'Warning', message: '', count: 0, reason: '', source: '', objectKind: '', age: null, last: null });
        expect(byKey(cols, 'type').render(row).props.className).toBe(getEventTypeColor('Warning'));
        expect(byKey(cols, 'message').render(row)).toBe('-');
        expect(byKey(cols, 'count').render(row)).toBe(1);
        expect(byKey(cols, 'reason').render(row)).toBe('-');
        expect(text(byKey(cols, 'source').render(row))).toBe('-');
        expect(byKey(cols, 'type').render(item({ type: '' })).props.children).toBe('Unknown');
    });

    it('links built-in kinds and known custom kinds; other kinds are plain text', () => {
        const cols = buildEventViewColumns({ ...base, view: null, crds: [{ spec: { names: { kind: 'Widget' } } }] });
        const link = (kind: string) => byKey(cols, 'involvedObject').render(item({ involvedObject: `${kind}/x` })).type;
        expect(link('Deployment')).toBe('button');
        expect(link('Widget')).toBe('button');
        expect(link('Mystery')).toBe('span');
        expect(text(byKey(cols, 'involvedObject').render(item({ involvedObject: '' })))).toBe('-');
    });

    it('resolves the link target from the event object fetched at click time', async () => {
        const navigateWithSearch = vi.fn();
        const getObject = vi.fn().mockResolvedValue({ involvedObject: { kind: 'Deployment', apiVersion: 'apps/v1', uid: 'dep-uid' } });
        const row = item({ involvedObject: 'Deployment/web' });
        await openInvolvedObject({ ...base, navigateWithSearch, view: { getObject } }, row);
        expect(getObject).toHaveBeenCalledWith(row);
        expect(navigateWithSearch).toHaveBeenCalledWith('deployments', 'uid:"dep-uid"');
    });

    it('does nothing when the object cannot be fetched or has no resolvable target', async () => {
        const navigateWithSearch = vi.fn();
        const row = item({ involvedObject: 'Deployment/web' });
        await openInvolvedObject({ ...base, navigateWithSearch, view: { getObject: vi.fn().mockRejectedValue(new Error('gone')) } }, row);
        await openInvolvedObject({ ...base, navigateWithSearch, view: { getObject: vi.fn().mockResolvedValue({}) } }, row);
        await openInvolvedObject({ ...base, navigateWithSearch, view: null }, row);
        expect(navigateWithSearch).not.toHaveBeenCalled();
    });
});
