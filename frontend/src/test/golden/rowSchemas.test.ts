import { describe, expect, it } from 'vitest';
import { rowSchemas } from '~/generated/rowSchemas';
import { semanticColumns } from '~/utils/columnSemantics';
import { namespaceSemantics } from '~/features/cluster/namespaces/semantics';
import { configMapSemantics } from '~/features/config/configmaps/semantics';
import { secretSemantics } from '~/features/config/secrets/semantics';
import { eventSemantics } from '~/features/cluster/events/semantics';
import { PREDEFINED_COLUMN_VALUES, detectColumnFilterTypes } from '~/components/shared/resourceListPipeline';
import { getFieldsMetadata } from '~/utils/search/fieldMappings';

// The generated schema (from the Go row specs) must stay in step with the legacy list code it
// replaces for migrated types: same column keys, same filter types, same predefined values and
// the same search fields and aliases. Secrets differ on purpose in keys and size (see
// spec_secrets.go).
const legacy: Record<string, Record<string, any>> = {
    namespaces: namespaceSemantics,
    configmaps: configMapSemantics,
    secrets: secretSemantics,
    events: eventSemantics,
};
const intentional: Record<string, string[]> = { secrets: ['keys', 'size'] };

describe('generated row schemas', () => {
    it.each(Object.keys(legacy))('%s matches the legacy semantics module', type => {
        const schema = (rowSchemas as any)[type];
        expect(schema, `no generated schema for ${type}`).toBeTruthy();
        expect(schema.migrated).toBe(true);
        const skip = new Set(intentional[type] ?? []);
        const legacyColumns = semanticColumns(legacy[type]);
        const legacyKeys = legacyColumns.map(c => c.key).filter(k => !skip.has(k));
        const schemaKeys = schema.columns.map((c: any) => c.key).filter((k: string) => !skip.has(k));
        expect(schemaKeys).toEqual(legacyKeys);

        const types = detectColumnFilterTypes(legacyColumns);
        for (const col of schema.columns) {
            if (skip.has(col.key)) continue;
            expect(col.filterType, `${type}.${col.key}`).toBe(types[col.key] === false ? 'none' : types[col.key]);
            expect([...col.predefined], `${type}.${col.key}`).toEqual(PREDEFINED_COLUMN_VALUES[type]?.[col.key] ?? []);
        }
    });

    it.each(Object.keys(legacy))('%s search fields equal the legacy field mappings', type => {
        const schema = (rowSchemas as any)[type];
        const fromSchema = schema.search.map((f: any) => ({ name: f.name, aliases: [...f.aliases] }));
        expect(fromSchema).toEqual(getFieldsMetadata(type));
    });
});
