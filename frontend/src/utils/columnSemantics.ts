// Shared shape for the pure, UI-free part of a list column: how a value is read for sorting,
// for select/regex filters and for numeric filters. Each list type keeps these in a plain
// semantics module that both the List component and the golden-parity generator import, so
// the Go row specs can be proven against the code that really runs.

export interface ColumnSemantics {
    getValue?: (item: any) => any;
    getFilterValue?: (item: any) => any;
    getNumericValue?: (item: any) => number;
    /** false hides the column's filter entirely (detectColumnFilterTypes). */
    filterable?: boolean;
    filterType?: 'regex' | 'select' | 'numeric' | false;
}

export type SemanticsMap = Record<string, ColumnSemantics>;

/** Column definitions (key plus semantics, no presentation) in declaration order. */
export function semanticColumns(map: SemanticsMap): Array<ColumnSemantics & { key: string }> {
    return Object.entries(map).map(([key, sem]) => ({ key, ...sem }));
}
