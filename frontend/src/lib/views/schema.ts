import type { DynamicRowSchema, ViewOpenResult } from './protocol';
import { rowSchemas, type RowSchema, type RowSchemaColumn, type RowSchemaType, type RowCellTypes } from '~/generated/rowSchemas';

export type { RowSchemaType, RowCellTypes, RowSchemaColumn };

const schemas = rowSchemas as unknown as Record<string, RowSchema>;

export function hasRowSchema(resourceType: string): resourceType is RowSchemaType {
    return Object.prototype.hasOwnProperty.call(schemas, resourceType);
}

/**
 * Lists whose ResourceList type predates the store type name: the list keeps its old id (saved
 * layouts and views are keyed by it) while the schema belongs to the store type.
 */
const LIST_TYPE_ALIASES: Readonly<Record<string, string>> = { pvs: 'persistentvolumes', pvcs: 'persistentvolumeclaims' };

export function getRowSchema(resourceType: string): RowSchema | undefined {
    const type = LIST_TYPE_ALIASES[resourceType] ?? resourceType;
    return hasRowSchema(type) ? schemas[type] : undefined;
}

export function migratedTypes(): string[] {
    return Object.keys(schemas);
}

export function schemaColumnKeys(resourceType: string): string[] {
    return getRowSchema(resourceType)?.columns.map(c => c.key) ?? [];
}

export function schemaColumn(resourceType: string, key: string): RowSchemaColumn | undefined {
    return getRowSchema(resourceType)?.columns.find(c => c.key === key);
}

/** Filter type of a column as the schema declares it; false when it has no filter. */
export function schemaFilterType(resourceType: string, key: string): 'regex' | 'select' | 'numeric' | false {
    const t = schemaColumn(resourceType, key)?.filterType;
    return t && t !== 'none' ? t : false;
}

/** Predefined select values per column, in the shape PREDEFINED_COLUMN_VALUES uses. */
export function schemaPredefined(resourceType: string): Record<string, string[]> {
    const out: Record<string, string[]> = {};
    for (const c of getRowSchema(resourceType)?.columns ?? []) {
        if (c.predefined.length > 0) out[c.key] = [...c.predefined];
    }
    return out;
}

/** Search fields for query autocomplete and highlighting, from the generated schema. */
export function schemaSearchFields(resourceType: string): Array<{ name: string; aliases: string[] }> | undefined {
    const s = getRowSchema(resourceType);
    return s ? s.search.map(f => ({ name: f.name, aliases: [...f.aliases] })) : undefined;
}

export function schemaPresets(resourceType: string): string[] {
    return [...(getRowSchema(resourceType)?.presets ?? [])];
}

/** True for the type strings of custom resources ("crd:group/version/resource"). */
export function isDynamicType(resourceType: string): boolean {
    return resourceType.startsWith('crd:');
}

/**
 * The row schema a view's open result carries, in the shape of a generated one. Dynamic types
 * (custom resources) have no entry in the generated file; their columns come from the CRD.
 */
export function schemaFromOpenResult(res: Pick<ViewOpenResult, 'schema'> | null | undefined): RowSchema | undefined {
    return res?.schema ? (res.schema as unknown as RowSchema) : undefined;
}

/** Schema for a view: the dynamic one from its snapshot when present, else the generated one. */
export function resolveSchema(resourceType: string | undefined, dynamic: DynamicRowSchema | null | undefined): RowSchema | undefined {
    if (dynamic) return dynamic as unknown as RowSchema;
    return resourceType ? getRowSchema(resourceType) : undefined;
}

export function filterTypeIn(schema: RowSchema | undefined, key: string): 'regex' | 'select' | 'numeric' | false {
    const t = schema?.columns.find(c => c.key === key)?.filterType;
    return t && t !== 'none' ? t : false;
}

export function columnKeysIn(schema: RowSchema | undefined): string[] {
    return schema?.columns.map(c => c.key) ?? [];
}
