import React from 'react';
import { describe, expect, it, vi } from 'vitest';
import { renderToStaticMarkup } from 'react-dom/server';
import { buildCustomResourceViewColumns } from './viewColumns';
import { resolveSchema, schemaFromOpenResult, filterTypeIn, columnKeysIn, isDynamicType } from '~/lib/views/schema';
import type { DynamicRowSchema } from '~/lib/views/protocol';
import { FakeBackend } from '~/lib/views/testSupport';
import { createViewClient } from '~/lib/views/viewClient';
import { FakeFeed } from '~/lib/views/testSupport';

vi.mock('~/components/shared/MenuBound', () => ({ default: () => null }));

const schema: DynamicRowSchema = {
    type: 'crd:cert.io/v1/certs',
    namespaced: true,
    schemaVersion: 'abc',
    migrated: true,
    columns: [
        { key: 'name', kind: 'string', filterType: 'regex', predefined: [], volatile: false, label: 'Name' },
        { key: 'namespace', kind: 'string', filterType: 'regex', predefined: [], volatile: false, label: 'Namespace' },
        { key: 'ready', kind: 'string', filterType: 'select', predefined: [], volatile: false, label: 'Ready' },
        { key: 'replicas', kind: 'number', filterType: 'numeric', predefined: [], volatile: false, label: 'Replicas', hidden: true },
        { key: 'enabled', kind: 'bool', filterType: 'select', predefined: [], volatile: false, label: 'Enabled' },
        { key: 'not_after', kind: 'time', filterType: 'none', predefined: [], volatile: false, label: 'Not After' },
        { key: 'age', kind: 'time', filterType: 'numeric', predefined: [], volatile: false, label: 'Age' },
    ],
    search: [{ name: 'name', aliases: ['n'] }],
    presets: [],
};

const deps = {
    onShowDetails: vi.fn(), onEditYaml: vi.fn(), onDelete: vi.fn(), operatorActions: [], onOperatorAction: vi.fn(),
};

describe('buildCustomResourceViewColumns', () => {
    it('presents every schema column with the printer column name as label, then the actions column', () => {
        const cols = buildCustomResourceViewColumns(schema, deps);
        expect(cols.map(c => c.key)).toEqual(['name', 'namespace', 'ready', 'replicas', 'enabled', 'not_after', 'age', 'actions']);
        expect(cols.find(c => c.key === 'not_after')!.label).toBe('Not After');
        expect(cols.find(c => c.key === 'replicas')!.defaultHidden).toBe(true);
        expect(cols.find(c => c.key === 'ready')!.defaultHidden).toBe(false);
        expect(cols.find(c => c.key === 'not_after')!.filterable).toBe(false);
        expect(cols.find(c => c.key === 'replicas')!.filterable).toBe(true);
    });

    it('renders cells by kind: status colours, booleans, dates and numbers', () => {
        const cols = buildCustomResourceViewColumns(schema, deps);
        const item = {
            metadata: { uid: 'u', name: 'a', namespace: 'ns' },
            cells: { ready: 'True', replicas: 3, enabled: true, not_after: null, age: null },
        };
        const html = (key: string) => renderToStaticMarkup(<>{cols.find(c => c.key === key)!.render(item)}</>);
        expect(html('name')).toBe('a');
        expect(html('ready')).toContain('text-green-400');
        expect(html('replicas')).toBe('3');
        expect(html('enabled')).toContain('svg');
        expect(html('not_after')).toBe('-');
        expect(cols.find(c => c.key === 'ready')!.getValue!(item)).toBe('True');
    });

    it('falls back to name and age before the schema arrives', () => {
        const cols = buildCustomResourceViewColumns(null, deps);
        expect(cols.map(c => c.key)).toEqual(['name', 'age', 'actions']);
    });
});

describe('dynamic schema helpers', () => {
    it('prefers the schema from the open result over the generated one', () => {
        expect(resolveSchema('configmaps', null)?.type).toBe('configmaps');
        expect(resolveSchema('cr-cert.io-certs', schema)?.type).toBe('crd:cert.io/v1/certs');
        expect(resolveSchema('cr-cert.io-certs', null)).toBeUndefined();
        expect(schemaFromOpenResult({ schema })?.type).toBe('crd:cert.io/v1/certs');
        expect(schemaFromOpenResult({})).toBeUndefined();
    });

    it('answers filter types and column keys from a schema', () => {
        const s = resolveSchema(undefined, schema);
        expect(filterTypeIn(s, 'replicas')).toBe('numeric');
        expect(filterTypeIn(s, 'not_after')).toBe(false);
        expect(filterTypeIn(s, 'missing')).toBe(false);
        expect(columnKeysIn(s)).toContain('not_after');
        expect(isDynamicType('crd:a/b/c')).toBe(true);
        expect(isDynamicType('pods')).toBe(false);
    });
});

describe('view client with a dynamic schema', () => {
    it('publishes the schema from the open result in the snapshot', async () => {
        const backend = new FakeBackend();
        backend.schema = schema;
        backend.columns = ['name', 'age'];
        const client = createViewClient({ transport: backend, notices: new FakeFeed() });
        expect(client.getSnapshot().schema).toBeNull();
        await client.open({
            viewId: 'v', clientToken: 't', type: 'crd:cert.io/v1/certs', namespaces: ['*'], search: '', columnFilters: [],
            sort: { key: '', dir: '' }, columns: [], locale: '', hide: [],
            crd: { group: 'cert.io', version: 'v1', resource: 'certs', namespaced: true },
        });
        expect(client.getSnapshot().schema?.type).toBe('crd:cert.io/v1/certs');
        expect(backend.openCalls[0].crd?.resource).toBe('certs');
        client.close();
    });
});
