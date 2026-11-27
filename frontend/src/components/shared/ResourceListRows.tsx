import { LiveAge, ageTimestampFor } from './LiveAge';
import { formatWhen } from '~/utils/formatting';
import React, { forwardRef } from 'react';
import { MIN_COLUMN_WIDTHS, type ColumnWidths } from './resourceListColumns';
import { RowCheckbox } from './ResourceListControls';

export interface SelectionLike {
    isSelected: (uid: string) => boolean;
}

export interface ResourceTableContext {
    highlightedUid?: string | null;
    selectable: boolean;
    selection?: SelectionLike | null;
    onRowClick?: ((item: any) => void) | null;
    /** Windowed lists render by index: the row object is looked up here instead of arriving as `item`. */
    rowAt?: ((index: number) => any) | null;
    /** Window revision of a windowed list; changing it re-renders the rows. */
    epoch?: number;
}

function shallowEqual(a: Record<string, any>, b: Record<string, any>, skip?: string): boolean {
    const aKeys = Object.keys(a);
    if (aKeys.length !== Object.keys(b).length) return false;
    for (const key of aKeys) {
        if (key === skip) continue;
        if (!Object.is(a[key], b[key])) return false;
    }
    return true;
}

// The <tr> children are always a single <RowCells> element created fresh by itemContent, so
// element identity alone would defeat memoisation. Compare the element's type and props
// instead: if those are equal the rendered output is equal.
function tableRowPropsEqual(prev: any, next: any): boolean {
    if (!shallowEqual(prev, next, 'children')) return false;
    const a = prev.children;
    const b = next.children;
    if (a === b) return true;
    if (!a || !b || a.type !== b.type) return false;
    return shallowEqual(a.props, b.props);
}

const TableRow = React.memo(({ item, context, ...props }: any) => {
    const { highlightedUid, selectable, selection, onRowClick, rowAt } = context as ResourceTableContext;
    if (item === undefined && rowAt) {
        const index = props['data-index'];
        item = typeof index === 'number' ? rowAt(index) : undefined;
    }
    const isHighlighted = highlightedUid === item?.metadata?.uid;
    const isSelected = selectable && selection?.isSelected(item?.metadata?.uid);
    return (
        <tr
            {...props}
            className={`transition-colors ${
                isSelected
                    ? 'bg-[color-mix(in_srgb,var(--color-primary)_12%,transparent)] hover:bg-[color-mix(in_srgb,var(--color-primary)_24%,transparent)]'
                    : isHighlighted
                        ? 'bg-white/5'
                        : 'hover:bg-white/5'
            } ${onRowClick ? 'cursor-pointer' : ''}`}
            // A row whose data has not arrived yet (a placeholder) has nothing to open.
            onClick={() => item && onRowClick && onRowClick(item)}
        />
    );
}, tableRowPropsEqual);

// Component types must stay stable across resource updates. Replacing them remounts
// the table and resets the hovered row (including any native cell tooltip).
export const virtuosoComponents = {
    Table: ({ style, context, ...props }: any) => (
        <table
            {...props}
            className="text-left border-collapse w-full"
            style={{ ...style, tableLayout: 'fixed' as const }}
        />
    ),
    TableHead: forwardRef<HTMLTableSectionElement>(({ context, ...props }: any, ref) => (
        <thead {...props} ref={ref} className="bg-surface sticky top-0 z-10" />
    )),
    TableBody: forwardRef<HTMLTableSectionElement>(({ context, ...props }: any, ref) => (
        <tbody {...props} ref={ref} className="divide-y divide-border" />
    )),
    TableRow,
};

export interface RowCellsProps {
    item: any;
    index: number;
    columns: any[];
    columnWidths: ColumnWidths;
    /** Whether this row is selected; only read for the selection column. */
    selected: boolean;
    onToggleSelect?: (e: any, item: any, index: number) => void;
    onNamespaceChange?: ((value: any) => void) | null;
    /** Heartbeat; changing it re-renders the row so time-dependent cells (age) stay current. */
    clock?: number;
}

// Cells of one row. Memoised so a ResourceList re-render (filter UI, metrics tick, menu
// state) does not re-render rows whose item, column set and widths are unchanged.
export const RowCells = React.memo(function RowCells({ item, index, columns, columnWidths, selected, onToggleSelect, onNamespaceChange }: RowCellsProps) {
    return (
        <>
            {columns.map((col: any) => {
                // Selection column cell
                if (col.isSelectionColumn) {
                    return (
                        <td
                            key={col.key}
                            className="p-3 text-sm whitespace-nowrap cursor-pointer"
                            style={{ width: `${col.width}px`, minWidth: `${col.width}px` }}
                            onClick={(e: any) => {
                                e.stopPropagation();
                                onToggleSelect?.(e, item, index);
                            }}
                        >
                            <div className="flex items-center justify-center pointer-events-none">
                                <RowCheckbox checked={selected} onChange={noop} />
                            </div>
                        </td>
                    );
                }

                const content = col.render ? col.render(item) : item[col.key];
                const isNamespaceColumn = col.key === 'namespace' && onNamespaceChange;
                const ageTimestamp = ageTimestampFor(item, col.key, content);
                const namespaceValue = item.metadata?.namespace;
                // All columns get a width for table-layout: fixed
                const width = columnWidths[col.key] || MIN_COLUMN_WIDTHS[col.key] || 100;

                return (
                    <td
                        key={col.key}
                        className={`p-3 text-sm text-text whitespace-nowrap ${col.isColumnSelector ? 'sticky right-0 bg-background overflow-visible' : 'overflow-hidden text-ellipsis'} ${col.align === 'center' ? 'text-center' : ''}`}
                        style={{ width: `${width}px`, minWidth: `${width}px` }}
                        title={ageTimestamp ? formatWhen(ageTimestamp) : typeof content === 'string' ? content : undefined}
                    >
                        {isNamespaceColumn && namespaceValue ? (
                            <button
                                onClick={(e) => {
                                    e.stopPropagation();
                                    onNamespaceChange([namespaceValue]);
                                }}
                                className="text-primary hover:text-primary/80 hover:underline transition-colors"
                                title={`Filter to namespace: ${namespaceValue}`}
                            >
                                {content}
                            </button>
                        ) : ageTimestamp ? <LiveAge timestamp={ageTimestamp} /> : content}
                    </td>
                );
            })}
        </>
    );
});

function noop() {}

export interface SkeletonCellsProps {
    columns: any[];
    columnWidths: ColumnWidths;
}

// Placeholder for a windowed-list row whose data is still in flight. The cells keep the real
// padding and text height so the row has the height of a loaded one.
export const SkeletonCells = React.memo(function SkeletonCells({ columns, columnWidths }: SkeletonCellsProps) {
    return (
        <>
            {columns.map((col: any) => {
                const width = col.isSelectionColumn ? col.width : (columnWidths[col.key] || MIN_COLUMN_WIDTHS[col.key] || 100);
                return (
                    <td
                        key={col.key}
                        className="p-3 text-sm whitespace-nowrap"
                        style={{ width: `${width}px`, minWidth: `${width}px` }}
                    >
                        {col.isSelectionColumn || col.isColumnSelector ? (
                            <div className="h-5" />
                        ) : (
                            <div className="h-5 flex items-center"><div className="h-3 w-2/3 rounded bg-white/5 animate-pulse" /></div>
                        )}
                    </td>
                );
            })}
        </>
    );
});
