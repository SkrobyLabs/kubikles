import React, { useCallback, useMemo } from 'react';
import { FunnelIcon } from '@heroicons/react/24/outline';
import { DndContext, PointerSensor, useSensor, useSensors, type DragEndEvent } from '@dnd-kit/core';
import { arrayMove, SortableContext, horizontalListSortingStrategy } from '@dnd-kit/sortable';
import ColumnConfigurator from './ColumnConfigurator';
import { MIN_COLUMN_WIDTHS, type ColumnWidths } from './resourceListColumns';
import { TriStateCheckbox, SortableHeader } from './ResourceListControls';
import type { ColumnFilter, ColumnFiltersMap, ColumnFilterType, SortConfig } from './resourceListPipeline';

export interface ResourceListHeaderProps {
    /** All columns, including hidden ones (for the column configurator). */
    columns: any[];
    visibleColumns: any[];
    columnWidths: ColumnWidths;
    sortConfig: SortConfig;
    onSort: (key: string) => void;
    columnFilters: ColumnFiltersMap;
    columnFilterTypes: Record<string, ColumnFilterType>;
    /** Select-type columns that currently have a usable value list (decides the funnel icon). */
    selectFilterColumns: ReadonlySet<string>;
    openColumnFilter: string | null;
    columnFilterRef: React.RefObject<HTMLDivElement>;
    onFilterButtonClick: (e: React.MouseEvent<HTMLButtonElement>, columnKey: string) => void;
    selectionState: 'none' | 'some' | 'all';
    onHeaderCheckboxClick: () => void;
    hiddenColumns: Set<string>;
    defaultHiddenColumns: Set<string>;
    onToggleColumn: (key: string) => void;
    onShowAllColumns: () => void;
    onResetColumnDefaults: () => void;
    onColumnOrderChange: (order: string[]) => void;
    onResizeStart: (e: React.MouseEvent, columnKey: string) => void;
    onResizeDoubleClick: (e: React.MouseEvent, columnKey: string) => void;
}

function filterTitle(filter: ColumnFilter | undefined, label: string): string {
    if (!filter) return `Filter by ${label}`;
    if (filter.type === 'regex') {
        return `Filtering: ${(filter.conditions || []).map((p: any) => `/${p}/`).join(` ${filter.logic || 'and'} `)}`;
    }
    if (filter.type === 'numeric') {
        return `Filtering: ${(filter.conditions || []).map((c: any) => `${c.operator} ${c.value}`).join(` ${filter.logic || 'and'} `)}`;
    }
    return `Filtering: ${filter.values?.size} value(s)`;
}

// Memoised: header props only change with sort/filter/column state, so a burst of watch
// batches (which changes rows, not the header) never re-renders it.
const ResourceListHeader = React.memo(function ResourceListHeader(props: ResourceListHeaderProps) {
    const {
        columns, visibleColumns, columnWidths, sortConfig, onSort, columnFilters, columnFilterTypes, selectFilterColumns,
        openColumnFilter, columnFilterRef, onFilterButtonClick, selectionState, onHeaderCheckboxClick,
        hiddenColumns, defaultHiddenColumns, onToggleColumn, onShowAllColumns, onResetColumnDefaults,
        onColumnOrderChange, onResizeStart, onResizeDoubleClick,
    } = props;

    // 8px distance distinguishes click (sort) from drag (reorder)
    const dndSensors = useSensors(
        useSensor(PointerSensor, { activationConstraint: { distance: 8 } })
    );

    // Sortable IDs = data column keys only (no selection, no column-selector)
    const sortableColumnIds = useMemo(() =>
        visibleColumns
            .filter((c: any) => !c.isSelectionColumn && !c.isColumnSelector)
            .map((c: any) => c.key),
    [visibleColumns]);

    const handleColumnDragEnd = useCallback((event: DragEndEvent) => {
        const { active, over } = event;
        if (!over || active.id === over.id) return;

        const oldIndex = sortableColumnIds.indexOf(active.id as string);
        const newIndex = sortableColumnIds.indexOf(over.id as string);
        if (oldIndex === -1 || newIndex === -1) return;

        onColumnOrderChange(arrayMove(sortableColumnIds, oldIndex, newIndex));
    }, [sortableColumnIds, onColumnOrderChange]);

    return (
        <DndContext sensors={dndSensors} onDragEnd={handleColumnDragEnd}>
            <SortableContext items={sortableColumnIds} strategy={horizontalListSortingStrategy}>
                <tr>
                    {visibleColumns.map((col: any) => {
                        const isResizable = !col.isColumnSelector && !col.isSelectionColumn;
                        const isDraggable = isResizable;
                        // All columns get a width for table-layout: fixed
                        const width = col.isSelectionColumn
                            ? col.width
                            : (columnWidths[col.key] || MIN_COLUMN_WIDTHS[col.key] || 100);

                        // Selection column header (not sortable/draggable)
                        if (col.isSelectionColumn) {
                            return (
                                <th
                                    key={col.key}
                                    className="p-3 text-xs font-medium text-gray-400 border-b border-border select-none whitespace-nowrap"
                                    style={{ width: `${col.width}px`, minWidth: `${col.width}px` }}
                                >
                                    <div className="flex items-center justify-center">
                                        <TriStateCheckbox
                                            state={selectionState}
                                            label="Select all displayed resources"
                                            onChange={onHeaderCheckboxClick}
                                        />
                                    </div>
                                </th>
                            );
                        }

                        // Column-selector header (not sortable/draggable)
                        if (col.isColumnSelector) {
                            return (
                                <th
                                    key={col.key}
                                    className="p-3 text-xs font-medium text-gray-400 uppercase tracking-wider border-b border-border select-none relative whitespace-nowrap group sticky right-0 bg-surface z-20"
                                    style={{ width: `${width}px`, minWidth: `${width}px` }}
                                >
                                    <div className="flex justify-center">
                                        <ColumnConfigurator
                                            columns={columns}
                                            hiddenColumns={hiddenColumns}
                                            onToggleColumn={onToggleColumn}
                                            onShowAll={onShowAllColumns}
                                            onResetDefaults={onResetColumnDefaults}
                                            defaultHiddenColumns={defaultHiddenColumns}
                                        />
                                    </div>
                                </th>
                            );
                        }

                        const filterType = columnFilterTypes[col.key];
                        const showFilterButton = filterType === 'regex' || filterType === 'numeric' || (filterType === 'select' && selectFilterColumns.has(col.key));
                        const activeFilter = columnFilters[col.key];

                        // Data column — sortable + draggable
                        return (
                            <SortableHeader
                                key={col.key}
                                id={col.key}
                                disabled={!isDraggable}
                                className="p-3 text-xs font-medium text-gray-400 uppercase tracking-wider border-b border-border select-none relative whitespace-nowrap group cursor-pointer hover:text-text"
                                style={{ width: `${width}px`, minWidth: `${width}px` }}
                            >
                                <div
                                    className={`flex items-center gap-1 ${col.align === 'center' ? 'justify-center' : ''}`}
                                    onClick={() => onSort(col.key)}
                                >
                                    {col.label}
                                    {sortConfig.key === col.key && (
                                        <span className="text-primary">
                                            {sortConfig.direction === 'asc' ? '↑' : '↓'}
                                        </span>
                                    )}
                                    {/* Column filter button */}
                                    {showFilterButton && (
                                        <div className="relative" ref={openColumnFilter === col.key ? columnFilterRef : undefined}>
                                            <button
                                                onPointerDown={(e) => e.stopPropagation()}
                                                onClick={(e) => onFilterButtonClick(e, col.key)}
                                                className={`p-0.5 rounded transition-colors ${
                                                    activeFilter
                                                        ? 'text-primary'
                                                        : 'text-gray-600 opacity-0 group-hover:opacity-100 hover:text-gray-400'
                                                }`}
                                                title={filterTitle(activeFilter, col.label)}
                                            >
                                                <FunnelIcon className="w-3 h-3" />
                                            </button>
                                            {/* Column filter dropdown rendered via portal by ResourceList */}
                                        </div>
                                    )}
                                </div>
                                {/* Resize handle */}
                                {isResizable && (
                                    <div
                                        className="absolute right-0 top-0 h-full w-1 cursor-col-resize hover:bg-primary/50 active:bg-primary group"
                                        onPointerDown={(e) => e.stopPropagation()}
                                        onMouseDown={(e) => onResizeStart(e, col.key)}
                                        onDoubleClick={(e) => onResizeDoubleClick(e, col.key)}
                                        onClick={(e) => e.stopPropagation()}
                                    >
                                        <div className="absolute right-0 top-1/2 -translate-y-1/2 w-0.5 h-4 bg-gray-600 group-hover:bg-primary opacity-0 group-hover:opacity-100 transition-opacity" />
                                    </div>
                                )}
                            </SortableHeader>
                        );
                    })}
                </tr>
            </SortableContext>
        </DndContext>
    );
});

export default ResourceListHeader;
