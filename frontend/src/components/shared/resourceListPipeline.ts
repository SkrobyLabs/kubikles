// Pure sort / filter / unique-value logic for ResourceList. Kept free of React so it can be
// unit-tested directly and so the component only decides WHEN to run it.

export interface ColumnFilterCondition {
    operator?: string;
    value?: number;
}

export interface ColumnFilter {
    type: 'select' | 'regex' | 'numeric';
    values?: Set<string>;
    conditions?: any[];
    pattern?: string;
    logic?: 'and' | 'or';
    operator?: string;
    value?: number;
}

export type ColumnFiltersMap = Record<string, ColumnFilter>;
export type ColumnFilterType = 'regex' | 'select' | 'numeric' | false;

export interface SortConfig {
    key: string | null;
    direction: string;
}

export interface UniqueValues {
    values: string[];
    counts: Record<string, number>;
}

// Select dropdowns with more distinct values than this are not useful and get no filter button.
export const MAX_SELECT_FILTER_VALUES = 200;

// Predefined values for select-type columns by resource type. They are listed (with count 0)
// even when no row currently has them, so users can pre-filter on a state that has not happened.
export const PREDEFINED_COLUMN_VALUES: Record<string, Record<string, string[]>> = {
    pods: {
        status: [
            'Running', 'Pending', 'Failed', 'Succeeded', 'CrashLoopBackOff', 'Terminating',
            'Unknown', 'ErrImagePull', 'ImagePullBackOff', 'ContainerCreating', 'PodInitializing',
            'Init:Error', 'Init:CrashLoopBackOff', 'Init:Running',
        ],
    },
    deployments: { status: ['Available', 'Progressing', 'Unavailable'] },
    services: { type: ['ClusterIP', 'NodePort', 'LoadBalancer', 'ExternalName'] },
    jobs: { status: ['Complete', 'Failed', 'Running', 'Suspended'] },
    cronjobs: { suspend: ['true', 'false'] },
    nodes: { status: ['Ready', 'NotReady', 'SchedulingDisabled'] },
    events: { type: ['Normal', 'Warning'] },
};

// One collator for every comparison. `new Intl.Collator()` with no arguments resolves exactly
// like String.prototype.localeCompare called with no locale/options, so ordering is unchanged
// while avoiding the per-call locale resolution that makes localeCompare slow in hot sorts.
const collator = new Intl.Collator();
export const compareNames = (a: string, b: string): number => collator.compare(a, b);

/** Numeric value used by numeric column filters (NaN means "no value", which never matches). */
export function getNumericValue(col: any, item: any): number {
    if (col.getNumericValue) return col.getNumericValue(item);
    // Auto-detect age column: convert creationTimestamp to hours
    if (col.key === 'age') {
        const ts = item.metadata?.creationTimestamp;
        if (!ts) return NaN;
        return (Date.now() - new Date(ts).getTime()) / 3600000;
    }
    if (col.getValue) {
        const v = col.getValue(item);
        return typeof v === 'number' ? v : NaN;
    }
    return NaN;
}

/** String value used by select/regex column filters (prefers getFilterValue over getValue). */
export function getColumnValue(col: any, item: any): string {
    if (col.getFilterValue) return String(col.getFilterValue(item) ?? '');
    if (col.key === 'name') return item.metadata?.name || '';
    if (col.key === 'namespace') return item.metadata?.namespace || '';
    if (col.getValue) {
        const v = col.getValue(item);
        return typeof v === 'string' ? v : '';
    }
    return String(item[col.key] ?? '');
}

/** Filter type per column: 'regex' | 'select' | 'numeric' | false (not filterable). */
export function detectColumnFilterTypes(columns: any[]): Record<string, ColumnFilterType> {
    const types: Record<string, ColumnFilterType> = {};
    for (const col of columns) {
        if (col.filterable === false || col.isColumnSelector || col.isSelectionColumn) {
            types[col.key] = false;
        } else if (col.filterType) {
            types[col.key] = col.filterType;
        } else if (col.key === 'name' || col.key === 'namespace') {
            types[col.key] = 'regex';
        } else if (col.key === 'age') {
            types[col.key] = 'numeric';
        } else {
            types[col.key] = 'select';
        }
    }
    return types;
}

/**
 * Unique values and counts for ONE select column over all rows (not just filtered ones),
 * predefined values first in declared order, then dynamic values alphabetically. Returns
 * undefined when there are no values at all or more than MAX_SELECT_FILTER_VALUES.
 */
export function computeColumnUniqueValues(col: any, data: any[], resourceType: string | null): UniqueValues | undefined {
    const counts: Record<string, number> = {};
    const predefined: string[] = (resourceType && PREDEFINED_COLUMN_VALUES[resourceType]?.[col.key]) || [];
    for (const v of predefined) counts[v] = 0;
    for (const item of data) {
        const val = getColumnValue(col, item);
        if (val) counts[val] = (counts[val] || 0) + 1;
    }
    const allValues = Object.keys(counts);
    if (allValues.length === 0 || allValues.length > MAX_SELECT_FILTER_VALUES) return undefined;
    const predefinedSet = new Set(predefined);
    const dynamic = allValues.filter(v => !predefinedSet.has(v)).sort(compareNames);
    return { values: [...predefined, ...dynamic], counts };
}

/**
 * Cheap check used to decide whether a select column's funnel icon is worth showing,
 * without building counts: true when the column has at least one value and at most the cap.
 * Stops scanning as soon as the cap is exceeded.
 */
export function selectColumnHasFilterValues(col: any, data: any[], resourceType: string | null): boolean {
    const seen = new Set<string>((resourceType && PREDEFINED_COLUMN_VALUES[resourceType]?.[col.key]) || []);
    if (seen.size > MAX_SELECT_FILTER_VALUES) return false;
    for (const item of data) {
        const val = getColumnValue(col, item);
        if (val) {
            seen.add(val);
            if (seen.size > MAX_SELECT_FILTER_VALUES) return false;
        }
    }
    return seen.size > 0;
}

const checkNumericCondition = (numVal: number, cond: any): boolean => {
    const target = cond.value;
    switch (cond.operator) {
        case '>': return numVal > target;
        case '>=': return numVal >= target;
        case '<': return numVal < target;
        case '<=': return numVal <= target;
        case '=': return numVal === target;
        case '!=': return numVal !== target;
        default: return true;
    }
};

/**
 * Compiles column filters once into a single predicate. Regexes are built once per filter
 * rather than once per row; an invalid pattern matches everything (it is ignored), exactly
 * as before. Filters whose column is not in `columns` are skipped.
 */
export function compileColumnFilters(columns: any[], columnFilters: ColumnFiltersMap): ((item: any) => boolean) | null {
    const compiled: Array<(item: any) => boolean> = [];
    for (const [colKey, filter] of Object.entries(columnFilters)) {
        const col = columns.find((c: any) => c.key === colKey);
        if (!col) continue;
        if (filter.type === 'select') {
            compiled.push(item => !!filter.values?.has(getColumnValue(col, item)));
        } else if (filter.type === 'regex') {
            const conditions = filter.conditions || [filter.pattern];
            const isAnd = filter.logic !== 'or';
            const regexes = conditions.map((pattern: any) => {
                try { return new RegExp(pattern, 'i'); } catch { return null; }
            });
            compiled.push(item => {
                const val = getColumnValue(col, item);
                // Same semantics as evaluating every condition: AND fails on any miss, OR needs one hit.
                const results = regexes.map(re => (re ? re.test(val) : true));
                return isAnd ? !results.some(r => !r) : results.some(r => r);
            });
        } else if (filter.type === 'numeric') {
            const conditions = filter.conditions || [{ operator: filter.operator, value: filter.value }];
            const isAnd = filter.logic !== 'or';
            compiled.push(item => {
                const numVal = getNumericValue(col, item);
                if (isNaN(numVal)) return false;
                const results = conditions.map((c: any) => checkNumericCondition(numVal, c));
                return isAnd ? !results.some(r => !r) : results.some(r => r);
            });
        }
    }
    if (compiled.length === 0) return null;
    return item => {
        for (const test of compiled) if (!test(item)) return false;
        return true;
    };
}

export function applyColumnFilters(items: any[], columns: any[], columnFilters: ColumnFiltersMap): any[] {
    const predicate = compileColumnFilters(columns, columnFilters);
    return predicate ? items.filter(predicate) : items;
}

interface SortEntry { item: any; value: any; ts: any; name: string }

/**
 * Sorts a copy of `items` by the column named in `sortConfig`. Sort keys are computed once
 * per item (decorate-sort-undecorate) instead of twice per comparison. Ordering is identical
 * to the previous inline comparator: primary key by < / > on raw values, then newest first by
 * creationTimestamp (unless sorting by age), then name ascending (unless sorting by name).
 * Array.prototype.sort is stable, so full ties keep incoming order.
 */
export function sortItems(items: any[], columns: any[], sortConfig: SortConfig): any[] {
    if (!sortConfig.key) return items;
    const column = columns.find((col: any) => col.key === sortConfig.key);
    if (!column) return items;

    const getValue = column.getValue || ((item: any) => item[column.key]);
    const asc = sortConfig.direction === 'asc';
    const tieAge = sortConfig.key !== 'age';
    const tieName = sortConfig.key !== 'name';

    const entries: SortEntry[] = new Array(items.length);
    for (let i = 0; i < items.length; i++) {
        const item = items[i];
        entries[i] = {
            item,
            value: getValue(item),
            ts: tieAge ? item.metadata?.creationTimestamp : undefined,
            name: tieName ? (item.metadata?.name || '') : '',
        };
    }

    entries.sort((a, b) => {
        if (a.value < b.value) return asc ? -1 : 1;
        if (a.value > b.value) return asc ? 1 : -1;

        // Tie-breakers (always ascending). "Ascending Age" means newest first: larger timestamp wins.
        if (tieAge && a.ts && b.ts && a.ts !== b.ts) return a.ts > b.ts ? -1 : 1;
        if (tieName) return compareNames(a.name, b.name);
        return 0;
    });

    const result: any[] = new Array(entries.length);
    for (let i = 0; i < entries.length; i++) result[i] = entries[i].item;
    return result;
}
