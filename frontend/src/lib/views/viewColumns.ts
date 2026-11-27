import { getRowSchema, type RowCellTypes, type RowSchemaType } from './schema';
import type { ColumnFilterSpec } from './protocol';
import type { RowItem } from './rowItem';

/** What a list still declares for a view column: presentation only. */
export interface ViewColumnPresentation<T extends RowSchemaType> {
    key: keyof RowCellTypes[T] & string;
    label: React.ReactNode;
    render: (item: RowItem & { cells: RowCellTypes[T] }) => React.ReactNode;
    defaultHidden?: boolean;
    align?: 'left' | 'center' | 'right';
    width?: number;
    numericHint?: string;
}

/** A column that exists only in the UI (actions menu, metrics overlay). */
export interface ViewUiColumn {
    key: string;
    label: React.ReactNode;
    render: (item: any) => React.ReactNode;
    align?: 'left' | 'center' | 'right';
    isColumnSelector?: boolean;
    disableSort?: boolean;
    filterable?: false;
    defaultHidden?: boolean;
    /** Insert after this schema column instead of at the end. */
    after?: string;
}

export interface ViewColumn {
    key: string;
    label: React.ReactNode;
    render: (item: any) => React.ReactNode;
    getValue?: (item: any) => any;
    defaultHidden?: boolean;
    align?: 'left' | 'center' | 'right';
    isColumnSelector?: boolean;
    disableSort?: boolean;
    filterable?: boolean;
    numericHint?: string;
}

/**
 * Merges presentation with the generated schema. Every schema column must be presented and every
 * presented key must exist in the schema; both mismatches throw, and the type parameter makes a
 * misspelt key a compile error. Sorting, filtering and search are the backend's job, so no
 * getValue/getFilterValue exist here beyond a plain cell read for width estimation.
 */
export function defineViewColumns<T extends RowSchemaType>(
    resourceType: T,
    presentation: ReadonlyArray<ViewColumnPresentation<T>>,
    uiColumns: ReadonlyArray<ViewUiColumn> = [],
): ViewColumn[] {
    const schema = getRowSchema(resourceType);
    if (!schema) throw new Error(`no row schema for ${resourceType}`);
    const known = new Set(schema.columns.map(c => c.key));
    const seen = new Set<string>();
    for (const p of presentation) {
        if (!known.has(p.key)) throw new Error(`${resourceType}: column "${p.key}" is not in the generated schema`);
        seen.add(p.key);
    }
    const missing = [...known].filter(k => !seen.has(k));
    if (missing.length > 0) throw new Error(`${resourceType}: schema columns without presentation: ${missing.join(', ')}`);

    const out: ViewColumn[] = presentation.map(p => {
        const sc = schema.columns.find(c => c.key === p.key)!;
        const key = p.key as string;
        return {
            key,
            label: p.label,
            render: p.render as (item: any) => React.ReactNode,
            getValue: (item: any) => item?.cells?.[key],
            defaultHidden: p.defaultHidden,
            align: p.align,
            numericHint: p.numericHint,
            filterable: sc.filterType !== 'none',
        };
    });
    for (const u of uiColumns) {
        if (known.has(u.key)) throw new Error(`${resourceType}: UI column "${u.key}" collides with a schema column`);
        const { after, ...col } = u;
        const at = after ? out.findIndex(c => c.key === after) : -1;
        if (at >= 0) out.splice(at + 1, 0, { ...col });
        else out.push({ ...col });
    }
    return out;
}

/** Keys of the schema columns shown by default (the seed for the view's projection). */
export function defaultViewColumns(resourceType: string, hiddenByDefault: readonly string[] = []): string[] {
    const hidden = new Set(hiddenByDefault);
    return (getRowSchema(resourceType)?.columns ?? []).map(c => c.key).filter(k => !hidden.has(k));
}

/** Column-filter map (Sets) as the Go view takes it. */
export function toColumnFilterSpecs(
    filters: Record<string, { type: 'select' | 'regex' | 'numeric'; values?: Set<string>; conditions?: any[]; pattern?: string; logic?: 'and' | 'or'; operator?: string; value?: number }>,
    knownColumns: ReadonlySet<string>,
): ColumnFilterSpec[] {
    const out: ColumnFilterSpec[] = [];
    for (const [col, f] of Object.entries(filters)) {
        if (!knownColumns.has(col)) continue;
        const spec: ColumnFilterSpec = { col, type: f.type };
        if (f.type === 'select') spec.values = Array.from(f.values ?? []);
        if (f.conditions) spec.conditions = f.conditions;
        if (f.logic) spec.logic = f.logic;
        if (f.pattern !== undefined) spec.pattern = f.pattern;
        if (f.operator !== undefined) spec.operator = f.operator;
        if (f.value !== undefined && !Number.isNaN(f.value)) spec.value = f.value;
        out.push(spec);
    }
    return out;
}
