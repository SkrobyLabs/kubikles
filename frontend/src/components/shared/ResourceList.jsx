import React, { useState, useRef, useEffect, useMemo, useCallback, forwardRef } from 'react';
import { TableVirtuoso } from 'react-virtuoso';
import { MagnifyingGlassIcon, EllipsisVerticalIcon, InformationCircleIcon, MinusIcon } from '@heroicons/react/24/outline';
import { CheckIcon } from '@heroicons/react/24/solid';
import SearchSelect from './SearchSelect';
import BulkActionBar from './BulkActionBar';
import { createFilter, getFieldsMetadata } from '../../utils/search';
import { useUI } from '../../context/UIContext';
import { useConfig } from '../../context/ConfigContext';

// Tri-state checkbox component for header (memoized to prevent re-renders)
const TriStateCheckbox = React.memo(({ state, onChange, disabled = false }) => {
    const handleClick = (e) => {
        e.stopPropagation();
        if (!disabled) onChange();
    };

    return (
        <button
            onClick={handleClick}
            disabled={disabled}
            className={`w-4 h-4 rounded border flex items-center justify-center transition-colors ${
                state === 'none'
                    ? 'border-gray-500 bg-transparent hover:border-gray-400'
                    : state === 'some'
                    ? 'border-primary bg-primary/20 hover:bg-primary/30'
                    : 'border-primary bg-primary hover:bg-primary/90'
            } ${disabled ? 'opacity-50 cursor-not-allowed' : 'cursor-pointer'}`}
        >
            {state === 'all' && <CheckIcon className="w-3 h-3 text-white" />}
            {state === 'some' && <MinusIcon className="w-3 h-3 text-primary" />}
        </button>
    );
});

// Row checkbox component (memoized to prevent re-renders on scroll)
const RowCheckbox = React.memo(({ checked, onChange, disabled = false }) => {
    const handleClick = (e) => {
        e.stopPropagation();
        if (!disabled) onChange(e);
    };

    return (
        <button
            onClick={handleClick}
            disabled={disabled}
            className={`w-4 h-4 rounded border flex items-center justify-center transition-colors ${
                checked
                    ? 'border-primary bg-primary hover:bg-primary/90'
                    : 'border-gray-500 bg-transparent hover:border-gray-400'
            } ${disabled ? 'opacity-50 cursor-not-allowed' : 'cursor-pointer'}`}
        >
            {checked && <CheckIcon className="w-3 h-3 text-white" />}
        </button>
    );
});

// Minimum column widths - ensure columns never get too narrow
const MIN_COLUMN_WIDTHS = {
    name: 150,
    namespace: 120,
    message: 200,
    cpu: 80,
    memory: 80,
    containers: 120,
    restarts: 80,
    age: 80,
    controlledBy: 120,
    pods: 80,
    ready: 80,
    desired: 80,
    current: 80,
    available: 80,
    condition: 120,
    conditions: 120,
    completions: 80,
    lastRun: 100,
    nextRun: 100,
    suspend: 80,
    schedule: 100,
    taints: 80,
    version: 80,
    count: 80,
    type: 80,
    last: 80,
    involvedObject: 200,
    actions: 50,
    status: 100,
    cluster: 100,
    clusterIP: 120,
    externalIP: 120,
    ports: 150,
    selector: 150,
    labels: 150,
    node: 120,
    ip: 100,
    image: 200,
    reason: 120,
    source: 100,
};

// Default column widths for common columns (fallback if not calculated)
const DEFAULT_COLUMN_WIDTHS = {
    cpu: 100,
    memory: 100,
    containers: 150,
    restarts: 100,
    age: 100,
    controlledBy: 150,
    pods: 100,
    ready: 100,
    desired: 100,
    current: 100,
    available: 100,
    condition: 150,
    conditions: 150,
    completions: 100,
    lastRun: 125,
    nextRun: 125,
    suspend: 100,
    schedule: 125,
    taints: 85,
    version: 85,
    count: 85,
    type: 100,
    last: 85,
    involvedObject: 250,
    actions: 50
};

// Average character width in pixels (approximate for our font)
const CHAR_WIDTH_PX = 7.5;
// Padding for cells (px on each side)
const CELL_PADDING_PX = 24;

// Calculate width needed for a string value
const calculateTextWidth = (text) => {
    if (!text) return 0;
    const str = String(text);
    return Math.ceil(str.length * CHAR_WIDTH_PX) + CELL_PADDING_PX;
};

// Calculate column widths based on data - finds width that covers ~95% of values
const calculateColumnWidths = (columns, data, savedWidths) => {
    if (!data || data.length === 0) return {};

    const calculatedWidths = {};

    for (const col of columns) {
        // Skip columns that already have saved user widths
        if (savedWidths[col.key]) continue;
        // Skip special columns
        if (col.isColumnSelector || col.isSelectionColumn) continue;

        const minWidth = MIN_COLUMN_WIDTHS[col.key] || 80;

        // Calculate header width
        const headerWidth = calculateTextWidth(col.label) + 20; // extra for sort indicator

        // Sample data to find content widths
        const contentWidths = [];
        const sampleSize = Math.min(data.length, 500); // Sample up to 500 rows
        const step = Math.max(1, Math.floor(data.length / sampleSize));

        for (let i = 0; i < data.length; i += step) {
            const item = data[i];
            let value;

            // Get the display value
            if (col.getValue) {
                value = col.getValue(item);
            } else if (col.key === 'name') {
                value = item.metadata?.name;
            } else if (col.key === 'namespace') {
                value = item.metadata?.namespace;
            } else {
                value = item[col.key];
            }

            // For rendered content, try to extract text
            if (col.render) {
                // For rendered columns, use the raw value or a reasonable estimate
                if (typeof value === 'string') {
                    contentWidths.push(calculateTextWidth(value));
                } else if (col.key === 'name' && item.metadata?.name) {
                    contentWidths.push(calculateTextWidth(item.metadata.name));
                }
            } else if (value !== null && value !== undefined) {
                contentWidths.push(calculateTextWidth(value));
            }
        }

        if (contentWidths.length === 0) {
            calculatedWidths[col.key] = Math.max(minWidth, headerWidth);
            continue;
        }

        // Sort widths and find 95th percentile
        contentWidths.sort((a, b) => a - b);
        const p95Index = Math.floor(contentWidths.length * 0.95);
        const p95Width = contentWidths[p95Index] || contentWidths[contentWidths.length - 1];

        // Use the larger of: min width, header width, or 95th percentile content width
        // Cap at a reasonable max to prevent extremely wide columns
        const maxWidth = col.key === 'message' ? 500 : 350;
        calculatedWidths[col.key] = Math.min(maxWidth, Math.max(minWidth, headerWidth, p95Width));
    }

    return calculatedWidths;
};

export default function ResourceList({
    title,
    columns,
    data,
    isLoading,
    namespaces = [],
    currentNamespace,
    onNamespaceChange,
    showNamespaceSelector = true,
    multiSelectNamespaces = false,
    highlightedUid = null,
    initialSort = null,
    resourceType = null,
    onRowClick = null,
    onRefresh = null,
    customHeaderActions = null,
    // Selection props
    selection = null,  // Selection hook result from useSelection
    selectable = false, // Enable selection mode
    // Bulk action callbacks
    onBulkDelete = null,
    onBulkRestart = null,
    onBulkExportYaml = null,
}) {
    const { pendingSearch, consumePendingSearch } = useUI();
    const { getConfig } = useConfig();
    const [sortConfig, setSortConfig] = useState(initialSort || { key: null, direction: 'asc' });
    const [searchInput, setSearchInput] = useState(''); // Immediate input value
    const [searchTerm, setSearchTerm] = useState('');   // Debounced value for filtering
    const [hiddenColumns, setHiddenColumns] = useState(new Set());

    // Debounce search input to avoid filtering on every keystroke
    const searchDebounceMs = getConfig('ui.searchDebounceMs') ?? 150;
    useEffect(() => {
        const timer = setTimeout(() => setSearchTerm(searchInput), searchDebounceMs);
        return () => clearTimeout(timer);
    }, [searchInput, searchDebounceMs]);
    const [showColumnMenu, setShowColumnMenu] = useState(false);
    const [showSearchHelp, setShowSearchHelp] = useState(false);
    const columnMenuRef = useRef(null);
    const searchHelpRef = useRef(null);
    const tableRef = useRef(null);

    // Column resizing state (user-saved widths)
    const [savedColumnWidths, setSavedColumnWidths] = useState(() => {
        if (!resourceType) return {};
        try {
            const saved = localStorage.getItem(`kubikles_colwidths_${resourceType}`);
            return saved ? JSON.parse(saved) : {};
        } catch {
            return {};
        }
    });
    const resizingRef = useRef(null);

    // Track data length to know when to recalculate widths
    // We use a ref to store the calculated widths so they persist across scrolls
    const calculatedWidthsRef = useRef({});
    const lastDataLengthRef = useRef(0);

    // Calculate column widths based on data - only recalculate when data changes significantly
    const calculatedWidths = useMemo(() => {
        // Recalculate if data length changed by more than 10% or is new
        const dataLengthChanged = Math.abs(data.length - lastDataLengthRef.current) > lastDataLengthRef.current * 0.1;
        const isNewData = lastDataLengthRef.current === 0 && data.length > 0;

        if (isNewData || dataLengthChanged) {
            calculatedWidthsRef.current = calculateColumnWidths(columns, data, savedColumnWidths);
            lastDataLengthRef.current = data.length;
        }

        return calculatedWidthsRef.current;
    }, [data, columns, savedColumnWidths]);

    // Effective column widths: calculated widths, then defaults, then saved widths (highest priority)
    const columnWidths = useMemo(() => ({
        ...DEFAULT_COLUMN_WIDTHS,
        ...calculatedWidths,
        ...savedColumnWidths
    }), [calculatedWidths, savedColumnWidths]);

    // Save column widths to localStorage
    useEffect(() => {
        if (resourceType && Object.keys(savedColumnWidths).length > 0) {
            localStorage.setItem(`kubikles_colwidths_${resourceType}`, JSON.stringify(savedColumnWidths));
        }
    }, [savedColumnWidths, resourceType]);

    // Column resize handlers
    const handleResizeStart = useCallback((e, columnKey) => {
        e.preventDefault();
        e.stopPropagation();

        const startX = e.clientX;
        const th = e.target.parentElement;
        const startWidth = th.offsetWidth;

        resizingRef.current = { columnKey, startX, startWidth };
        document.body.style.cursor = 'col-resize';
        document.body.style.userSelect = 'none';

        const handleMouseMove = (moveEvent) => {
            if (!resizingRef.current) return;
            const diff = moveEvent.clientX - resizingRef.current.startX;
            const newWidth = Math.max(50, resizingRef.current.startWidth + diff);
            setSavedColumnWidths(prev => ({
                ...prev,
                [resizingRef.current.columnKey]: newWidth
            }));
        };

        const handleMouseUp = () => {
            resizingRef.current = null;
            document.body.style.cursor = '';
            document.body.style.userSelect = '';
            document.removeEventListener('mousemove', handleMouseMove);
            document.removeEventListener('mouseup', handleMouseUp);
        };

        document.addEventListener('mousemove', handleMouseMove);
        document.addEventListener('mouseup', handleMouseUp);
    }, []);

    // Double-click to reset column width
    const handleResizeDoubleClick = useCallback((e, columnKey) => {
        e.preventDefault();
        e.stopPropagation();
        setSavedColumnWidths(prev => {
            const newWidths = { ...prev };
            delete newWidths[columnKey];
            return newWidths;
        });
    }, []);

    // Consume pending search when navigating to this view
    useEffect(() => {
        if (pendingSearch) {
            const search = consumePendingSearch();
            if (search) {
                setSearchInput(search);
                setSearchTerm(search); // Skip debounce for programmatic search
            }
        }
    }, [pendingSearch, consumePendingSearch]);

    useEffect(() => {
        const handleClickOutside = (event) => {
            if (columnMenuRef.current && !columnMenuRef.current.contains(event.target)) {
                setShowColumnMenu(false);
            }
            if (searchHelpRef.current && !searchHelpRef.current.contains(event.target)) {
                setShowSearchHelp(false);
            }
        };

        document.addEventListener('mousedown', handleClickOutside);
        return () => {
            document.removeEventListener('mousedown', handleClickOutside);
        };
    }, []);

    const toggleColumn = (key) => {
        const newHidden = new Set(hiddenColumns);
        if (newHidden.has(key)) {
            newHidden.delete(key);
        } else {
            newHidden.add(key);
        }
        setHiddenColumns(newHidden);
    };

    const baseVisibleColumns = columns.filter(col => !hiddenColumns.has(col.key));

    const handleSort = (key) => {
        let direction = 'asc';
        if (sortConfig.key === key && sortConfig.direction === 'asc') {
            direction = 'desc';
        }
        setSortConfig({ key, direction });
    };

    const filteredData = useMemo(() => {
        if (!searchTerm) return data;
        const filterFn = createFilter(resourceType, searchTerm);
        return data.filter(filterFn);
    }, [data, searchTerm, resourceType]);

    const sortedData = React.useMemo(() => {
        if (!sortConfig.key) return filteredData;

        return [...filteredData].sort((a, b) => {
            const column = columns.find(col => col.key === sortConfig.key);
            if (!column) return 0;

            const getValue = column.getValue || ((item) => item[column.key]);
            const aValue = getValue(a);
            const bValue = getValue(b);

            if (aValue < bValue) {
                return sortConfig.direction === 'asc' ? -1 : 1;
            }
            if (aValue > bValue) {
                return sortConfig.direction === 'asc' ? 1 : -1;
            }

            // Tie-breakers (always ascending)

            // 1. Age (if available and not primary sort)
            if (sortConfig.key !== 'age') {
                const aDate = a.metadata?.creationTimestamp;
                const bDate = b.metadata?.creationTimestamp;
                if (aDate && bDate && aDate !== bDate) {
                    // User wants "Ascending Age" (Newest to Oldest)
                    // Newest = Larger Timestamp
                    return aDate > bDate ? -1 : 1;
                }
            }

            // 2. Name (if not primary sort)
            if (sortConfig.key !== 'name') {
                const aName = a.metadata?.name || '';
                const bName = b.metadata?.name || '';
                return aName.localeCompare(bName);
            }

            return 0;
        });
    }, [filteredData, sortConfig, columns]);

    // Add selection checkbox column if selection is enabled
    const visibleColumns = useMemo(() => {
        if (!selectable || !selection) return baseVisibleColumns;

        const checkboxColumn = {
            key: '_selection',
            label: '',
            isSelectionColumn: true,
            width: 40,
        };

        return [checkboxColumn, ...baseVisibleColumns];
    }, [selectable, selection, baseVisibleColumns]);

    // Compute selection state for the current filtered/sorted data
    const selectionState = useMemo(() => {
        if (!selectable || !selection) return 'none';
        return selection.getSelectionState(sortedData);
    }, [selectable, selection, sortedData]);

    // Handle header checkbox click
    const handleHeaderCheckboxClick = useCallback(() => {
        if (!selection) return;
        selection.toggleAll(sortedData);
    }, [selection, sortedData]);

    // Handle row checkbox click
    const handleRowCheckboxClick = useCallback((e, item, index) => {
        if (!selection) return;
        const uid = item?.metadata?.uid;
        if (uid) {
            selection.toggleItem(uid, index, sortedData, e.shiftKey);
        }
    }, [selection, sortedData]);

    // Bulk action handlers - pass selected items to callbacks
    const handleBulkDelete = useCallback(() => {
        if (!selection || !onBulkDelete) return;
        const selectedItems = selection.getSelectedItems(sortedData);
        onBulkDelete(selectedItems);
    }, [selection, sortedData, onBulkDelete]);

    const handleBulkRestart = useCallback(() => {
        if (!selection || !onBulkRestart) return;
        const selectedItems = selection.getSelectedItems(sortedData);
        onBulkRestart(selectedItems);
    }, [selection, sortedData, onBulkRestart]);

    const handleBulkExportYaml = useCallback(() => {
        if (!selection || !onBulkExportYaml) return;
        const selectedItems = selection.getSelectedItems(sortedData);
        onBulkExportYaml(selectedItems);
    }, [selection, sortedData, onBulkExportYaml]);

    const handleClearSelection = useCallback(() => {
        if (!selection) return;
        selection.deselectAll();
    }, [selection]);

    // Memoize virtuoso components to prevent recreation on every render
    const virtuosoComponents = useMemo(() => ({
        Table: ({ style, ...props }) => (
            <table
                {...props}
                ref={tableRef}
                className="text-left border-collapse w-full"
                style={{ ...style, tableLayout: 'fixed' }}
            />
        ),
        TableHead: forwardRef((props, ref) => (
            <thead {...props} ref={ref} className="bg-surface sticky top-0 z-10" />
        )),
        TableBody: forwardRef((props, ref) => (
            <tbody {...props} ref={ref} className="divide-y divide-border" />
        )),
        TableRow: ({ item, ...props }) => {
            const isHighlighted = highlightedUid === item?.metadata?.uid;
            const isSelected = selectable && selection?.isSelected(item?.metadata?.uid);
            return (
                <tr
                    {...props}
                    className={`transition-colors ${
                        isSelected
                            ? 'bg-primary/10 hover:bg-primary/15'
                            : isHighlighted
                                ? 'bg-white/5'
                                : 'hover:bg-white/5'
                    } ${onRowClick ? 'cursor-pointer' : ''}`}
                    onClick={() => onRowClick && onRowClick(item)}
                />
            );
        }
    }), [highlightedUid, selectable, selection, onRowClick]);

    return (
        <div className="flex flex-col h-full bg-background relative">
            {/* Header */}
            <div className="h-14 border-b border-border flex items-center justify-between px-4 bg-surface shrink-0 gap-4 titlebar-drag">
                <div className="flex items-center gap-4 flex-1">
                    <h1 className="text-lg font-semibold text-text shrink-0">{title}</h1>

                    {/* Search Bar */}
                    <div className="relative max-w-md w-full flex items-center gap-1">
                        <div className="relative flex-1">
                            <MagnifyingGlassIcon className="h-4 w-4 text-gray-400 absolute left-3 top-1/2 transform -translate-y-1/2" />
                            <input
                                type="text"
                                placeholder={resourceType ? `Search... (name:"x" status:Running)` : `Search ${title}...`}
                                className="w-full bg-background border border-border rounded-md pl-9 pr-4 py-1.5 text-sm text-text focus:outline-none focus:border-primary transition-colors"
                                value={searchInput}
                                onChange={(e) => setSearchInput(e.target.value)}
                                autoComplete="off"
                                autoCorrect="off"
                                spellCheck="false"
                            />
                        </div>
                        {resourceType && (
                            <div className="relative" ref={searchHelpRef}>
                                <button
                                    onClick={() => setShowSearchHelp(!showSearchHelp)}
                                    className="p-1 text-gray-400 hover:text-gray-300 transition-colors"
                                    title="Search syntax help"
                                >
                                    <InformationCircleIcon className="h-5 w-5" />
                                </button>
                                {showSearchHelp && (
                                    <div className="absolute left-0 top-full mt-1 w-80 bg-surface border border-border rounded-md shadow-lg z-50 p-3 text-sm">
                                        <div className="font-medium text-text mb-2">Search Syntax</div>
                                        <div className="space-y-2 text-gray-400">
                                            <div>
                                                <span className="text-gray-300">Plain text:</span> matches name
                                                <div className="text-xs text-gray-500 ml-2">nginx</div>
                                            </div>
                                            <div>
                                                <span className="text-gray-300">Field search:</span> field:"value"
                                                <div className="text-xs text-gray-500 ml-2">name:"my-pod" status:Running</div>
                                            </div>
                                            <div>
                                                <span className="text-gray-300">Regex:</span> field:/pattern/
                                                <div className="text-xs text-gray-500 ml-2">name:/^nginx-/ name:/end$/</div>
                                            </div>
                                            <div className="border-t border-border pt-2 mt-2">
                                                <span className="text-gray-300">Available fields:</span>
                                                <div className="text-xs text-gray-500 mt-1 flex flex-wrap gap-1">
                                                    {getFieldsMetadata(resourceType).map(f => (
                                                        <span key={f.name} className="bg-background px-1.5 py-0.5 rounded">
                                                            {f.name}
                                                        </span>
                                                    ))}
                                                </div>
                                            </div>
                                        </div>
                                    </div>
                                )}
                            </div>
                        )}
                    </div>
                </div>

                {/* Namespace Selector */}
                {showNamespaceSelector && (
                    <div className="w-64">
                        <SearchSelect
                            options={namespaces}
                            value={currentNamespace}
                            onChange={onNamespaceChange}
                            placeholder="Select Namespace..."
                            multiSelect={multiSelectNamespaces}
                        />
                    </div>
                )}

                {/* Custom Header Actions */}
                {customHeaderActions}
            </div>

            {/* Table Content */}
            <div className="flex-1 overflow-hidden">
                {isLoading ? (
                    <div className="flex items-center justify-center h-full text-gray-500">
                        Loading...
                    </div>
                ) : sortedData.length === 0 ? (
                    <div className="flex items-center justify-center h-full text-gray-500">
                        No resources found
                    </div>
                ) : (
                    <TableVirtuoso
                        style={{ height: '100%' }}
                        data={sortedData}
                        overscan={50}
                        components={virtuosoComponents}
                        fixedHeaderContent={() => (
                            <tr>
                                {visibleColumns.map((col, colIndex) => {
                                    const isLastDataColumn = colIndex === visibleColumns.length - 2 && visibleColumns[visibleColumns.length - 1]?.isColumnSelector;
                                    const isResizable = !col.isColumnSelector && !col.isSelectionColumn;
                                    // All columns get a width for table-layout: fixed
                                    const width = col.isSelectionColumn
                                        ? col.width
                                        : (columnWidths[col.key] || MIN_COLUMN_WIDTHS[col.key] || 100);

                                    // Selection column header
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
                                                        onChange={handleHeaderCheckboxClick}
                                                    />
                                                </div>
                                            </th>
                                        );
                                    }

                                    return (
                                        <th
                                            key={col.key}
                                            className={`p-3 text-xs font-medium text-gray-400 uppercase tracking-wider border-b border-border select-none relative whitespace-nowrap ${col.isColumnSelector ? 'sticky right-0 bg-surface z-20' : 'cursor-pointer hover:text-text'}`}
                                            style={{ width: `${width}px`, minWidth: `${width}px` }}
                                            onClick={() => !col.isColumnSelector && handleSort(col.key)}
                                        >
                                            {col.isColumnSelector ? (
                                                <div className={`relative flex ${col.align === 'center' ? 'justify-center' : 'justify-end'}`} ref={columnMenuRef}>
                                                    <button
                                                        onClick={(e) => {
                                                            e.stopPropagation();
                                                            setShowColumnMenu(!showColumnMenu);
                                                        }}
                                                        className="p-1 hover:bg-white/10 rounded-full transition-colors"
                                                    >
                                                        <EllipsisVerticalIcon className="h-5 w-5" />
                                                    </button>
                                                    {showColumnMenu && (
                                                        <div className="absolute right-0 top-full mt-1 w-48 bg-surface border border-border rounded-md shadow-lg z-50 py-1">
                                                            {columns.filter(c => !c.isColumnSelector).map(c => (
                                                                <label
                                                                    key={c.key}
                                                                    className="flex items-center px-4 py-2 text-sm text-text hover:bg-white/5 cursor-pointer"
                                                                    onClick={(e) => e.stopPropagation()}
                                                                >
                                                                    <input
                                                                        type="checkbox"
                                                                        checked={!hiddenColumns.has(c.key)}
                                                                        onChange={() => toggleColumn(c.key)}
                                                                        className="mr-2 rounded border-gray-600 bg-background text-primary focus:ring-primary"
                                                                    />
                                                                    {c.label}
                                                                </label>
                                                            ))}
                                                        </div>
                                                    )}
                                                </div>
                                            ) : (
                                                <div className={`flex items-center gap-1 ${col.align === 'center' ? 'justify-center' : ''}`}>
                                                    {col.label}
                                                    {sortConfig.key === col.key && (
                                                        <span className="text-primary">
                                                            {sortConfig.direction === 'asc' ? '↑' : '↓'}
                                                        </span>
                                                    )}
                                                </div>
                                            )}
                                            {/* Resize handle */}
                                            {isResizable && !isLastDataColumn && (
                                                <div
                                                    className="absolute right-0 top-0 h-full w-1 cursor-col-resize hover:bg-primary/50 active:bg-primary group"
                                                    onMouseDown={(e) => handleResizeStart(e, col.key)}
                                                    onDoubleClick={(e) => handleResizeDoubleClick(e, col.key)}
                                                    onClick={(e) => e.stopPropagation()}
                                                >
                                                    <div className="absolute right-0 top-1/2 -translate-y-1/2 w-0.5 h-4 bg-gray-600 group-hover:bg-primary opacity-0 group-hover:opacity-100 transition-opacity" />
                                                </div>
                                            )}
                                        </th>
                                    );
                                })}
                            </tr>
                        )}
                        itemContent={(index, item) => (
                            <>
                                {visibleColumns.map((col) => {
                                    // Selection column cell
                                    if (col.isSelectionColumn) {
                                        const isSelected = selection?.isSelected(item?.metadata?.uid);
                                        return (
                                            <td
                                                key={col.key}
                                                className="p-3 text-sm whitespace-nowrap"
                                                style={{ width: `${col.width}px`, minWidth: `${col.width}px` }}
                                            >
                                                <div className="flex items-center justify-center">
                                                    <RowCheckbox
                                                        checked={isSelected}
                                                        onChange={(e) => handleRowCheckboxClick(e, item, index)}
                                                    />
                                                </div>
                                            </td>
                                        );
                                    }

                                    const content = col.render ? col.render(item) : item[col.key];
                                    const isNamespaceColumn = col.key === 'namespace' && onNamespaceChange;
                                    const namespaceValue = item.metadata?.namespace;
                                    // All columns get a width for table-layout: fixed
                                    const width = columnWidths[col.key] || MIN_COLUMN_WIDTHS[col.key] || 100;

                                    return (
                                        <td
                                            key={col.key}
                                            className={`p-3 text-sm text-text whitespace-nowrap ${col.isColumnSelector ? 'sticky right-0 bg-background overflow-visible' : 'overflow-hidden text-ellipsis'} ${col.align === 'center' ? 'text-center' : ''}`}
                                            style={{ width: `${width}px`, minWidth: `${width}px` }}
                                            title={typeof content === 'string' ? content : undefined}
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
                                            ) : content}
                                        </td>
                                    );
                                })}
                            </>
                        )}
                    />
                )}
            </div>

            {/* Bulk Action Bar - Bottom */}
            {selectable && selection && (
                <BulkActionBar
                    selectedCount={selection.selectedCount}
                    onClearSelection={handleClearSelection}
                    onDelete={onBulkDelete ? handleBulkDelete : null}
                    onRestart={onBulkRestart ? handleBulkRestart : null}
                    onExportYaml={onBulkExportYaml ? handleBulkExportYaml : null}
                    resourceType={resourceType || title.toLowerCase()}
                    position="bottom"
                />
            )}
        </div>
    );
}
