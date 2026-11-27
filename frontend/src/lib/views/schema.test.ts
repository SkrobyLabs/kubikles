import { describe, expect, it } from 'vitest';
import { rowSchemas } from '~/generated/rowSchemas';
import { defineViewColumns, defaultViewColumns, toColumnFilterSpecs } from './viewColumns';
import {
    getRowSchema, hasRowSchema, migratedTypes, schemaColumnKeys, schemaFilterType, schemaPredefined,
    schemaPresets, schemaSearchFields,
} from './schema';
import { getFieldsMetadata } from '~/utils/search';

describe('schema accessors', () => {
    it('knows exactly the generated types', () => {
        expect(migratedTypes().sort()).toEqual(Object.keys(rowSchemas).sort());
        expect(hasRowSchema('configmaps')).toBe(true);
        expect(hasRowSchema('cronjobs')).toBe(false);
        expect(getRowSchema('cronjobs')).toBeUndefined();
    });

    it('resolves the persistent volume list type through its alias', () => {
        expect(getRowSchema('pvs')?.type).toBe('persistentvolumes');
        expect(hasRowSchema('pvs')).toBe(false);
        expect(schemaColumnKeys('pvs')).toEqual(schemaColumnKeys('persistentvolumes'));
    });

    it('maps filter types, predefined values and presets', () => {
        expect(schemaFilterType('events', 'type')).toBe('select');
        expect(schemaFilterType('events', 'age')).toBe('numeric');
        expect(schemaFilterType('secrets', 'keys')).toBe(false);
        expect(schemaFilterType('secrets', 'nope')).toBe(false);
        expect(schemaPredefined('events')).toEqual({ type: ['Normal', 'Warning'] });
        expect(schemaPresets('secrets')).toEqual(['helm']);
        expect(schemaColumnKeys('clusterroles')).toEqual(['name', 'rules', 'aggregation', 'age']);
    });

    it('search autocomplete reads the generated fields only in view mode', () => {
        const generated = schemaSearchFields('configmaps')!;
        expect(getFieldsMetadata('configmaps', true)).toEqual(generated);
        expect(getFieldsMetadata('configmaps').map(f => f.name)).toContain('name');
        // Unmigrated types always use the legacy table.
        expect(getFieldsMetadata('cronjobs', true)).toEqual(getFieldsMetadata('cronjobs'));
    });
});

describe('defineViewColumns', () => {
    const noop = () => null;

    it('merges presentation with the schema and appends UI-only columns', () => {
        const cols = defineViewColumns('clusterroles', [
            { key: 'name', label: 'Name', render: noop },
            { key: 'rules', label: 'Rules', render: noop },
            { key: 'aggregation', label: 'Aggregation', render: noop },
            { key: 'age', label: 'Age', render: noop },
        ], [{ key: 'actions', label: '', render: noop, isColumnSelector: true, disableSort: true }]);
        expect(cols.map(c => c.key)).toEqual(['name', 'rules', 'aggregation', 'age', 'actions']);
        expect(cols[0].getValue?.({ cells: { name: 'a' } })).toBe('a');
        expect(cols[3].filterable).toBe(true);
    });

    it('places UI columns after a named column', () => {
        const cols = defineViewColumns('clusterroles', [
            { key: 'name', label: 'Name', render: noop },
            { key: 'rules', label: 'Rules', render: noop },
            { key: 'aggregation', label: 'Aggregation', render: noop },
            { key: 'age', label: 'Age', render: noop },
        ], [{ key: 'cpu', label: 'CPU', render: noop, after: 'name' }]);
        expect(cols.map(c => c.key)).toEqual(['name', 'cpu', 'rules', 'aggregation', 'age']);
    });

    it('fails when a schema column has no presentation', () => {
        expect(() => defineViewColumns('clusterroles', [
            { key: 'name', label: 'Name', render: noop },
        ])).toThrow(/without presentation: rules, aggregation, age/);
    });

    it('fails when a presented key is not in the schema', () => {
        expect(() => defineViewColumns('clusterroles', [
            { key: 'name', label: 'Name', render: noop },
            { key: 'rules', label: 'Rules', render: noop },
            { key: 'aggregation', label: 'Aggregation', render: noop },
            { key: 'age', label: 'Age', render: noop },
            // @ts-expect-error a key outside the generated cell type is a compile error too
            { key: 'bogus', label: 'Bogus', render: noop },
        ])).toThrow(/"bogus" is not in the generated schema/);
    });

    it('rejects a UI column that shadows a schema column', () => {
        expect(() => defineViewColumns('clusterroles', [
            { key: 'name', label: 'Name', render: noop },
            { key: 'rules', label: 'Rules', render: noop },
            { key: 'aggregation', label: 'Aggregation', render: noop },
            { key: 'age', label: 'Age', render: noop },
        ], [{ key: 'age', label: 'x', render: noop }])).toThrow(/collides/);
    });

    it('the default projection drops the hidden-by-default keys', () => {
        expect(defaultViewColumns('configmaps', ['keyCount', 'binaryKeys', 'size'])).toEqual(['name', 'namespace', 'keys', 'age']);
        expect(defaultViewColumns('nope')).toEqual([]);
    });
});

describe('toColumnFilterSpecs', () => {
    const known = new Set(['type', 'name', 'age']);

    it('serialises Sets and keeps regex and numeric conditions as the backend takes them', () => {
        const specs = toColumnFilterSpecs({
            type: { type: 'select', values: new Set(['Warning']) },
            name: { type: 'regex', conditions: ['^a', 'b$'], logic: 'or' },
            age: { type: 'numeric', conditions: [{ operator: '>', value: 2 }], logic: 'and' },
            other: { type: 'select', values: new Set(['x']) },
        }, known);
        expect(specs).toEqual([
            { col: 'type', type: 'select', values: ['Warning'] },
            { col: 'name', type: 'regex', conditions: ['^a', 'b$'], logic: 'or' },
            { col: 'age', type: 'numeric', conditions: [{ operator: '>', value: 2 }], logic: 'and' },
        ]);
    });

    it('drops NaN values and filters on unknown columns', () => {
        expect(toColumnFilterSpecs({ age: { type: 'numeric', operator: '>', value: NaN } }, known))
            .toEqual([{ col: 'age', type: 'numeric', operator: '>' }]);
    });
});

describe('every list column key exists in the schema and vice versa', () => {
    // The lists build their columns with defineViewColumns at render time, which throws on any
    // mismatch; this guards the key sets the lists seed their projections from.
    for (const type of migratedTypes()) {
        it(`${type}: every key resolves in both directions`, () => {
            const keys = schemaColumnKeys(type);
            expect(new Set(keys).size).toBe(keys.length);
            const noop = () => null;
            const cols = defineViewColumns(type as any, keys.map(key => ({ key, label: key, render: noop })) as any);
            expect(cols.map(c => c.key)).toEqual(keys);
        });
    }
});
