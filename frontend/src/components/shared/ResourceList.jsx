import React, { useState, useRef, useEffect, useLayoutEffect, useMemo, useCallback, forwardRef } from 'react';
import { createPortal } from 'react-dom';
import { TableVirtuoso } from 'react-virtuoso';
import { MagnifyingGlassIcon, InformationCircleIcon, FunnelIcon } from '@heroicons/react/24/outline';
import SearchSelect from './SearchSelect';
import BulkActionBar from './BulkActionBar';
import ColumnConfigurator from './ColumnConfigurator';
import SavedViewsDropdown from './SavedViewsDropdown';
import { createFilter, getFieldsMetadata } from '../../utils/search';
import { useSavedViews } from '../../hooks/useSavedViews';
import { useUI } from '../../context';
import { useConfig } from '../../context';

// Tri-state checkbox component for header (memoized to prevent re-renders)
const TriStateCheckbox = React.memo(({ state, onChange, disabled = false }) => {
    const handleChange = (e) => {
        e.stopPropagation();
        if (!disabled) onChange();
    };

    return (
        <input
            type="checkbox"
            checked={state === 'all'}
            ref={(el) => { if (el) el.indeterminate = state === 'some'; }}
            onChange={handleChange}
            disabled={disabled}
            className={disabled ? 'opacity-50 cursor-not-allowed' : ''}
        />
    );
});

// Row checkbox component (memoized to prevent re-renders on scroll)
const RowCheckbox = React.memo(({ checked, onChange, disabled = false }) => {
    const handleChange = (e) => {
        e.stopPropagation();
        if (!disabled) onChange(e);
    };

    return (
        <input
            type="checkbox"
            checked={checked}
            onChange={handleChange}
            disabled={disabled}
            className={disabled ? 'opacity-50 cursor-not-allowed' : ''}
        />
    );
});

// Minimum column widths - ensure columns never get too narrow
const MIN_COLUMN_WIDTHS = {
    _selection: 44,
    name: 150,
    namespace: 120,
    message: 200,
    cpu: 100,
    memory: 100,
    containers: 120,
    restarts: 80,
    age: 80,
    controlledBy: 120,
    pods: 100,
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
    restarts: 140,
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

    // Columns that render visual components with fixed widths - skip auto-calculation
    const fixedWidthColumns = new Set(['cpu', 'memory', 'pods']);

    for (const col of columns) {
        // Skip columns that already have saved user widths
        if (savedWidths[col.key]) continue;
        // Skip special columns
        if (col.isColumnSelector || col.isSelectionColumn) continue;
        // Skip columns that render fixed-width visual components (e.g., resource bars)
        if (fixedWidthColumns.has(col.key)) continue;

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
    selection = null,
    selectable = false,
    // Bulk action callbacks
    onBulkDelete = null,
    onBulkRestart = null,
    onBulkExportYaml = null,
    onFilteredUidsChange = null,
}) {
    const { pendingSearch, consumePendingSearch } = useUI();
    const { getConfig } = useConfig();
    const [sortConfig, setSortConfig] = useState(initialSort || { key: null, direction: 'asc' });
    const [searchInput, setSearchInput] = useState(''); // Immediate input value
    const [searchTerm, setSearchTerm] = useState('');   // Debounced value for filtering
    const [hiddenColumns, setHiddenColumns] = useState(() => {
        // Initialize with columns marked as defaultHidden
        return new Set(columns.filter(col => col.defaultHidden).map(col => col.key));
    });
    const [activeViewId, setActiveViewId] = useState(null);

    // Saved views
    const { views, saveView, loadView, updateView, deleteView, renameView, duplicateView, setDefaultView, getDefaultView } = useSavedViews(resourceType);

    // Debounce search input to avoid filtering on every keystroke
    const searchDebounceMs = getConfig('ui.searchDebounceMs') ?? 150;
    useEffect(() => {
        const timer = setTimeout(() => setSearchTerm(searchInput), searchDebounceMs);
        return () => clearTimeout(timer);
    }, [searchInput, searchDebounceMs]);
    const [columnFilters, setColumnFilters] = useState({}); // { [columnKey]: { type, values|pattern } }
    const [openColumnFilter, setOpenColumnFilter] = useState(null); // column key of open filter dropdown
    const [columnFilterSearch, setColumnFilterSearch] = useState(''); // search within filter dropdown
    const [regexDraft, setRegexDraft] = useState(''); // draft regex pattern (applied on Enter)
    const [numericDraft, setNumericDraft] = useState({ operator: '>', value: '', unit: 'h' }); // draft numeric filter
    const [columnFilterPos, setColumnFilterPos] = useState(null); // { top, left } for fixed-position dropdown
    const columnFilterRef = useRef(null); // ref for the filter button wrapper
    const columnFilterDropdownRef = useRef(null); // ref for the fixed-position dropdown
    const [showSearchHelp, setShowSearchHelp] = useState(false);
    const searchHelpRef = useRef(null);
    const tableRef = useRef(null);

    // Get current view config for saving (defined after columnFilters state)
    const getCurrentViewConfig = useCallback(() => {
        // Serialize columnFilters — convert Sets to arrays for JSON storage
        const serializedFilters = {};
        for (const [key, filter] of Object.entries(columnFilters)) {
            if (filter.type === 'select') {
                serializedFilters[key] = { ...filter, values: Array.from(filter.values) };
            } else {
                serializedFilters[key] = filter;
            }
        }
        return {
            query: searchInput,
            namespace: currentNamespace,
            hiddenColumns: Array.from(hiddenColumns),
            sortConfig,
            columnFilters: serializedFilters,
            resourceType,
        };
    }, [searchInput, currentNamespace, hiddenColumns, sortConfig, columnFilters, resourceType]);

    // Load a saved view (or reset to defaults if viewId is null)
    const handleLoadView = useCallback((viewId) => {
        if (!viewId) {
            // Reset to application defaults
            setActiveViewId(null);
            setSearchInput('');
            setSearchTerm('');
            setHiddenColumns(new Set(columns.filter(col => col.defaultHidden).map(col => col.key)));
            setSortConfig(initialSort || { key: null, direction: 'asc' });
            setColumnFilters({});
            return;
        }
        const view = loadView(viewId);
        if (!view) return;

        setActiveViewId(viewId);
        setSearchInput(view.query || '');
        setSearchTerm(view.query || '');
        if (view.hiddenColumns?.length > 0) {
            setHiddenColumns(new Set(view.hiddenColumns));
        } else {
            setHiddenColumns(new Set());
        }
        setSortConfig(view.sortConfig || { key: null, direction: 'asc' });
        // Restore column filters — deserialize arrays back to Sets
        if (view.columnFilters && Object.keys(view.columnFilters).length > 0) {
            const restored = {};
            for (const [key, filter] of Object.entries(view.columnFilters)) {
                if (filter.type === 'select' && Array.isArray(filter.values)) {
                    restored[key] = { ...filter, values: new Set(filter.values) };
                } else {
                    restored[key] = filter;
                }
            }
            setColumnFilters(restored);
        } else {
            setColumnFilters({});
        }
        if (view.namespace && onNamespaceChange) {
            onNamespaceChange(view.namespace);
        }
    }, [loadView, onNamespaceChange, columns, initialSort]);

    // Handle updating an existing view with current config
    const handleUpdateView = useCallback((viewId, config) => {
        // Serialize Sets to arrays for storage
        const serializedFilters = {};
        for (const [key, filter] of Object.entries(config.columnFilters || {})) {
            if (filter.type === 'select' && filter.values instanceof Set) {
                serializedFilters[key] = { ...filter, values: Array.from(filter.values) };
            } else {
                serializedFilters[key] = filter;
            }
        }
        updateView(viewId, {
            query: config.query,
            namespace: config.namespace,
            hiddenColumns: config.hiddenColumns,
            sortConfig: config.sortConfig,
            columnFilters: serializedFilters,
        });
    }, [updateView]);

    // Handle deleting a view - reset to defaults if deleting the active view
    const handleDeleteView = useCallback((viewId) => {
        deleteView(viewId);
        if (viewId === activeViewId) {
            handleLoadView(null);
        }
    }, [deleteView, activeViewId, handleLoadView]);

    // Auto-load default view on mount
    const defaultViewLoadedRef = useRef(false);
    useEffect(() => {
        if (defaultViewLoadedRef.current || !resourceType) return;
        const defaultView = getDefaultView();
        if (defaultView) {
            defaultViewLoadedRef.current = true;
            handleLoadView(defaultView.id);
        }
    }, [resourceType, getDefaultView, handleLoadView]);

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

    // Track whether to auto-open details for the first matching row
    const autoOpenDetailsRef = useRef(false);

    // Consume pending search when navigating to this view
    useEffect(() => {
        if (pendingSearch) {
            const { search, autoOpen } = consumePendingSearch();
            if (search) {
                setSearchInput(search);
                setSearchTerm(search); // Skip debounce for programmatic search
                if (autoOpen) {
                    autoOpenDetailsRef.current = true;
                }
            }
        }
    }, [pendingSearch, consumePendingSearch]);

    useEffect(() => {
        const handleClickOutside = (event) => {
            if (searchHelpRef.current && !searchHelpRef.current.contains(event.target)) {
                setShowSearchHelp(false);
            }
            if (columnFilterRef.current && !columnFilterRef.current.contains(event.target) &&
                (!columnFilterDropdownRef.current || !columnFilterDropdownRef.current.contains(event.target))) {
                setOpenColumnFilter(null);
                setColumnFilterSearch('');
            }
        };

        document.addEventListener('mousedown', handleClickOutside);
        return () => {
            document.removeEventListener('mousedown', handleClickOutside);
        };
    }, []);

    // Reposition the column filter dropdown after render to prevent viewport overflow
    useLayoutEffect(() => {
        if (!openColumnFilter || !columnFilterDropdownRef.current || !columnFilterPos) return;
        const dropdown = columnFilterDropdownRef.current;
        const dropdownRect = dropdown.getBoundingClientRect();
        const vw = window.innerWidth;
        const vh = window.innerHeight;
        let { top, left, anchorTop, anchorBottom } = columnFilterPos;
        // Clamp right edge
        if (left + dropdownRect.width > vw - 8) {
            left = vw - dropdownRect.width - 8;
        }
        if (left < 8) left = 8;
        // If dropdown overflows bottom, flip above the anchor button
        if (top + dropdownRect.height > vh - 8 && anchorTop !== undefined) {
            const flippedTop = anchorTop - dropdownRect.height - 4;
            top = flippedTop < 8 ? 8 : flippedTop;
        }
        if (top !== columnFilterPos.top || left !== columnFilterPos.left) {
            setColumnFilterPos(prev => ({ ...prev, top, left }));
        }
    }, [openColumnFilter, columnFilterPos]);

    // Extract numeric value for numeric filters
    const getNumericValue = useCallback((col, item) => {
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
    }, []);

    // Extract column value for filtering (prefers getFilterValue over getValue)
    const getColumnValue = useCallback((col, item) => {
        if (col.getFilterValue) return String(col.getFilterValue(item) ?? '');
        if (col.key === 'name') return item.metadata?.name || '';
        if (col.key === 'namespace') return item.metadata?.namespace || '';
        if (col.getValue) {
            const v = col.getValue(item);
            return typeof v === 'string' ? v : '';
        }
        return String(item[col.key] ?? '');
    }, []);

    // Determine filter type per column: 'regex' | 'select' | 'numeric' | false
    const columnFilterTypes = useMemo(() => {
        const types = {};
        for (const col of columns) {
            if (col.filterable === false || col.isColumnSelector || col.isSelectionColumn) {
                types[col.key] = false;
                continue;
            }
            if (col.filterType) { types[col.key] = col.filterType; continue; }
            // Auto-detect
            if (col.key === 'name' || col.key === 'namespace') { types[col.key] = 'regex'; continue; }
            if (col.key === 'age') { types[col.key] = 'numeric'; continue; }
            types[col.key] = 'select';
        }
        return types;
    }, [columns]);

    // Predefined values for select-type columns by resource type
    const PREDEFINED_COLUMN_VALUES = useMemo(() => ({
        pods: {
            status: [
                'Running', 'Pending', 'Failed', 'Succeeded', 'CrashLoopBackOff', 'Terminating',
                'Unknown', 'ErrImagePull', 'ImagePullBackOff', 'ContainerCreating', 'PodInitializing',
                'Init:Error', 'Init:CrashLoopBackOff', 'Init:Running',
            ],
        },
        deployments: {
            status: ['Available', 'Progressing', 'Unavailable'],
        },
        services: {
            type: ['ClusterIP', 'NodePort', 'LoadBalancer', 'ExternalName'],
        },
        jobs: {
            status: ['Complete', 'Failed', 'Running', 'Suspended'],
        },
        cronjobs: {
            suspend: ['true', 'false'],
        },
        nodes: {
            status: ['Ready', 'NotReady', 'SchedulingDisabled'],
        },
        events: {
            type: ['Normal', 'Warning'],
        },
    }), []);

    // Compute unique values + counts for select-type filter columns from data, merged with predefined
    const columnUniqueValues = useMemo(() => {
        const result = {};
        const selectCols = columns.filter(c => columnFilterTypes[c.key] === 'select');

        for (const col of selectCols) {
            const counts = {};
            // Seed with predefined values (count = 0)
            const predefined = PREDEFINED_COLUMN_VALUES[resourceType]?.[col.key] || [];
            for (const v of predefined) counts[v] = 0;
            // Count actual data values
            for (const item of data) {
                const val = getColumnValue(col, item);
                if (val) counts[val] = (counts[val] || 0) + 1;
            }
            const allValues = Object.keys(counts);
            if (allValues.length > 0 && allValues.length <= 200) {
                // Sort: predefined first (in order), then dynamic values alphabetically
                const predefinedSet = new Set(predefined);
                const dynamic = allValues.filter(v => !predefinedSet.has(v)).sort((a, b) => a.localeCompare(b));
                result[col.key] = { values: [...predefined, ...dynamic], counts };
            }
        }
        return result;
    }, [data, columns, getColumnValue, columnFilterTypes, resourceType, PREDEFINED_COLUMN_VALUES]);

    // Toggle a value in a select-type column filter
    const toggleColumnFilter = useCallback((colKey, value) => {
        setColumnFilters(prev => {
            const current = prev[colKey]?.type === 'select' ? new Set(prev[colKey].values) : new Set();
            if (current.has(value)) {
                current.delete(value);
            } else {
                current.add(value);
            }
            const next = { ...prev };
            if (current.size === 0) {
                delete next[colKey];
            } else {
                next[colKey] = { type: 'select', values: current };
            }
            return next;
        });
    }, []);

    // Add a regex pattern condition to a column filter
    const addRegexCondition = useCallback((colKey, pattern) => {
        if (!pattern) return;
        setColumnFilters(prev => {
            const existing = prev[colKey];
            const conditions = existing?.type === 'regex' ? [...existing.conditions] : [];
            conditions.push(pattern);
            return { ...prev, [colKey]: { type: 'regex', conditions, logic: existing?.logic || 'and' } };
        });
    }, []);

    // Remove a regex condition by index
    const removeRegexCondition = useCallback((colKey, index) => {
        setColumnFilters(prev => {
            const existing = prev[colKey];
            if (!existing || existing.type !== 'regex') return prev;
            const conditions = existing.conditions.filter((_, i) => i !== index);
            if (conditions.length === 0) {
                const next = { ...prev };
                delete next[colKey];
                return next;
            }
            return { ...prev, [colKey]: { ...existing, conditions } };
        });
    }, []);

    // Add a numeric condition to a column filter
    const addNumericCondition = useCallback((colKey, operator, value) => {
        if (value === '' || value === null || isNaN(Number(value))) return;
        setColumnFilters(prev => {
            const existing = prev[colKey];
            const conditions = existing?.type === 'numeric' ? [...existing.conditions] : [];
            conditions.push({ operator, value: Number(value) });
            return { ...prev, [colKey]: { type: 'numeric', conditions, logic: existing?.logic || 'and' } };
        });
    }, []);

    // Remove a numeric condition by index
    const removeNumericCondition = useCallback((colKey, index) => {
        setColumnFilters(prev => {
            const existing = prev[colKey];
            if (!existing || existing.type !== 'numeric') return prev;
            const conditions = existing.conditions.filter((_, i) => i !== index);
            if (conditions.length === 0) {
                const next = { ...prev };
                delete next[colKey];
                return next;
            }
            return { ...prev, [colKey]: { ...existing, conditions } };
        });
    }, []);

    // Toggle AND/OR logic for a column filter
    const toggleFilterLogic = useCallback((colKey) => {
        setColumnFilters(prev => {
            const existing = prev[colKey];
            if (!existing) return prev;
            return { ...prev, [colKey]: { ...existing, logic: existing.logic === 'and' ? 'or' : 'and' } };
        });
    }, []);

    // Clear all filters for a column
    const clearColumnFilter = useCallback((colKey) => {
        setColumnFilters(prev => {
            const next = { ...prev };
            delete next[colKey];
            return next;
        });
    }, []);

    // Clear all column filters
    const clearAllColumnFilters = useCallback(() => {
        setColumnFilters({});
    }, []);

    // Check if any column has active filters
    const hasActiveColumnFilters = Object.keys(columnFilters).length > 0;

    const toggleColumn = (key) => {
        const newHidden = new Set(hiddenColumns);
        if (newHidden.has(key)) {
            newHidden.delete(key);
        } else {
            newHidden.add(key);
        }
        setHiddenColumns(newHidden);
    };

    // Show all columns
    const showAllColumns = useCallback(() => {
        setHiddenColumns(new Set());
    }, []);

    // Reset columns to default visibility (determined by column.defaultHidden property)
    const resetColumnDefaults = useCallback(() => {
        const defaultHidden = new Set(
            columns.filter(col => col.defaultHidden).map(col => col.key)
        );
        setHiddenColumns(defaultHidden);
    }, [columns]);

    // Compute default hidden columns set for ColumnConfigurator
    const defaultHiddenColumns = useMemo(() => {
        return new Set(columns.filter(col => col.defaultHidden).map(col => col.key));
    }, [columns]);

    const baseVisibleColumns = columns.filter(col => !hiddenColumns.has(col.key));

    const handleSort = (key) => {
        let direction = 'asc';
        if (sortConfig.key === key && sortConfig.direction === 'asc') {
            direction = 'desc';
        }
        setSortConfig({ key, direction });
    };

    const filteredData = useMemo(() => {
        let result = data;

        // Apply search term filter
        if (searchTerm) {
            const filterFn = createFilter(resourceType, searchTerm);
            result = result.filter(filterFn);
        }

        // Apply per-column filters (select and regex)
        if (hasActiveColumnFilters) {
            result = result.filter(item => {
                for (const [colKey, filter] of Object.entries(columnFilters)) {
                    const col = columns.find(c => c.key === colKey);
                    if (!col) continue;
                    const val = getColumnValue(col, item);
                    if (filter.type === 'select') {
                        if (!filter.values.has(val)) return false;
                    } else if (filter.type === 'regex') {
                        const conditions = filter.conditions || [filter.pattern];
                        const isAnd = filter.logic !== 'or';
                        const results = conditions.map(pattern => {
                            try {
                                return new RegExp(pattern, 'i').test(val);
                            } catch { return true; }
                        });
                        if (isAnd ? results.some(r => !r) : !results.some(r => r)) return false;
                    } else if (filter.type === 'numeric') {
                        const numVal = getNumericValue(col, item);
                        if (isNaN(numVal)) return false;
                        const conditions = filter.conditions || [{ operator: filter.operator, value: filter.value }];
                        const isAnd = filter.logic !== 'or';
                        const checkCondition = (cond) => {
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
                        const results = conditions.map(checkCondition);
                        if (isAnd ? results.some(r => !r) : !results.some(r => r)) return false;
                    }
                }
                return true;
            });
        }

        return result;
    }, [data, searchTerm, resourceType, columnFilters, hasActiveColumnFilters, columns, getColumnValue, getNumericValue]);

    // Report filtered UIDs to parent (for notifications scoping, etc.)
    // Use a ref to avoid re-calling when the actual UIDs haven't changed
    const prevFilteredUidsRef = useRef(null);
    useEffect(() => {
        if (!onFilteredUidsChange) return;
        const uidArray = filteredData.map(item => item.metadata?.uid).filter(Boolean);
        // Quick check: same length and same UIDs as last time?
        const prev = prevFilteredUidsRef.current;
        if (prev && prev.size === uidArray.length && uidArray.every(uid => prev.has(uid))) {
            return; // No change
        }
        const uids = new Set(uidArray);
        prevFilteredUidsRef.current = uids;
        onFilteredUidsChange(uids);
    }, [filteredData, onFilteredUidsChange]);

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

    // Auto-open details for the first matching row after search + filter settles
    useEffect(() => {
        if (autoOpenDetailsRef.current && sortedData.length > 0 && onRowClick) {
            autoOpenDetailsRef.current = false;
            // Defer to let the UI render the list first
            requestAnimationFrame(() => onRowClick(sortedData[0]));
        }
    }, [sortedData, onRowClick]);

    // Add selection checkbox column if selection is enabled
    const visibleColumns = useMemo(() => {
        if (!selectable || !selection) return baseVisibleColumns;

        const checkboxColumn = {
            key: '_selection',
            label: '',
            isSelectionColumn: true,
            width: MIN_COLUMN_WIDTHS._selection,
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
                            ? 'bg-[color-mix(in_srgb,var(--color-primary)_12%,transparent)] hover:bg-[color-mix(in_srgb,var(--color-primary)_24%,transparent)]'
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
            {/* Header - split into draggable and non-draggable regions as siblings */}
            <div className="h-14 border-b border-border flex items-center bg-surface shrink-0">
                {/* Left side - draggable titlebar region */}
                <div className="flex items-center gap-4 flex-1 px-4 titlebar-drag h-full">
                    <h1 className="text-lg font-semibold text-text shrink-0">{title}</h1>

                    {/* Search Bar */}
                    <div className="relative max-w-md w-full flex items-center gap-1 no-drag">
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
                                            <div>
                                                <span className="text-gray-300">OR groups:</span> condition OR condition
                                                <div className="text-xs text-gray-500 ml-2">name:/^web-/ OR name:/^api-/</div>
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

                {/* Right side controls - NOT a child of titlebar-drag, so right-click works */}
                <div className="flex items-center gap-3 shrink-0 px-4">
                    {/* Custom Header Actions */}
                    {customHeaderActions}

                    {/* Saved Views */}
                    {resourceType && (
                        <SavedViewsDropdown
                            views={views}
                            activeViewId={activeViewId}
                            onSave={saveView}
                            onLoad={handleLoadView}
                            onUpdate={handleUpdateView}
                            onDelete={handleDeleteView}
                            onRename={renameView}
                            onDuplicate={duplicateView}
                            onSetDefault={setDefaultView}
                            getCurrentConfig={getCurrentViewConfig}
                        />
                    )}

                    {/* Namespace Selector */}
                    {showNamespaceSelector && (
                        <div className="w-64 no-drag">
                            <SearchSelect
                                options={namespaces}
                                value={currentNamespace}
                                onChange={onNamespaceChange}
                                placeholder="Select Namespace..."
                                multiSelect={multiSelectNamespaces}
                            />
                        </div>
                    )}
                </div>
            </div>

            {/* Active Filter Chips */}
            {resourceType && hasActiveColumnFilters && (
                <div className="px-4 py-1.5 border-b border-border bg-surface shrink-0 flex items-center gap-2 flex-wrap">
                    <FunnelIcon className="w-4 h-4 text-gray-500 shrink-0" />
                    {Object.entries(columnFilters).map(([colKey, filter]) => {
                        const col = columns.find(c => c.key === colKey);
                        return (
                            <span key={colKey} className="inline-flex items-center gap-1 px-2 py-0.5 text-xs bg-primary/10 text-primary border border-primary/15 rounded">
                                <span className="text-gray-400">{col?.label || colKey}:</span>
                                <span>
                                    {filter.type === 'regex'
                                        ? <span className="font-mono">{(filter.conditions || [filter.pattern]).map((p, i) => (
                                            <span key={i}>{i > 0 && <span className="text-gray-500 mx-0.5">{filter.logic === 'or' ? '|' : '&'}</span>}/{p}/</span>
                                        ))}</span>
                                        : filter.type === 'numeric'
                                            ? <span className="font-mono">{(filter.conditions || [{ operator: filter.operator, value: filter.value }]).map((c, i) => (
                                                <span key={i}>{i > 0 && <span className="text-gray-500 mx-0.5">{filter.logic === 'or' ? '|' : '&'}</span>}{c.operator} {c.value}</span>
                                            ))}</span>
                                            : `${filter.values.size} selected`
                                    }
                                </span>
                                <button
                                    onClick={() => clearColumnFilter(colKey)}
                                    className="hover:text-white transition-colors ml-0.5"
                                >
                                    ×
                                </button>
                            </span>
                        );
                    })}
                    <button
                        onClick={clearAllColumnFilters}
                        className="text-xs text-gray-500 hover:text-gray-300 transition-colors ml-2"
                    >
                        Clear all
                    </button>
                </div>
            )}

            {/* Table Content */}
            <div className="flex-1 overflow-hidden">
                {isLoading ? (
                    <div className="flex items-center justify-center h-full text-gray-500">
                        Loading...
                    </div>
                ) : sortedData.length === 0 ? (
                    <div className="flex flex-col items-center justify-center h-full text-gray-500 gap-3">
                        {/* Message and action buttons based on why no results */}
                        {(() => {
                            const namespaceArray = Array.isArray(currentNamespace) ? currentNamespace : (currentNamespace ? [currentNamespace] : []);
                            const hasNoNamespace = namespaceArray.length === 0;
                            const hasSearchFilter = searchTerm && data.length > 0;
                            const hasPartialNamespaces = namespaceArray.length > 0 && namespaceArray.length < namespaces.length;

                            // Priority 1: No namespace selected (namespace-scoped resources only)
                            if (hasNoNamespace && namespaces.length > 0 && onNamespaceChange) {
                                return (
                                    <>
                                        <span>No namespace selected</span>
                                        <button
                                            onClick={() => onNamespaceChange(multiSelectNamespaces ? ['*'] : namespaces[0])}
                                            className="px-3 py-1.5 text-xs font-medium text-primary hover:text-white bg-primary/10 hover:bg-primary/20 rounded transition-colors"
                                        >
                                            View all namespaces
                                        </button>
                                    </>
                                );
                            }

                            // Priority 2: Search is filtering everything out
                            if (hasSearchFilter) {
                                return (
                                    <>
                                        <span>No matching resources</span>
                                        <button
                                            onClick={() => { setSearchInput(''); setSearchTerm(''); }}
                                            className="px-3 py-1.5 text-xs font-medium text-primary hover:text-white bg-primary/10 hover:bg-primary/20 rounded transition-colors"
                                        >
                                            Clear search
                                        </button>
                                    </>
                                );
                            }

                            // Priority 3: Less than all namespaces selected (and not using '*' marker)
                            if (hasPartialNamespaces && !namespaceArray.includes('*') && onNamespaceChange) {
                                return (
                                    <>
                                        <span>No resources found in selected namespaces</span>
                                        <button
                                            onClick={() => onNamespaceChange(multiSelectNamespaces ? ['*'] : namespaces[0])}
                                            className="px-3 py-1.5 text-xs font-medium text-primary hover:text-white bg-primary/10 hover:bg-primary/20 rounded transition-colors"
                                        >
                                            View all namespaces
                                        </button>
                                    </>
                                );
                            }

                            return <span>No resources found</span>;
                        })()}
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
                                            className={`p-3 text-xs font-medium text-gray-400 uppercase tracking-wider border-b border-border select-none relative whitespace-nowrap group ${col.isColumnSelector ? 'sticky right-0 bg-surface z-20' : 'cursor-pointer hover:text-text'}`}
                                            style={{ width: `${width}px`, minWidth: `${width}px` }}
                                            onClick={() => !col.isColumnSelector && handleSort(col.key)}
                                        >
                                            {col.isColumnSelector ? (
                                                <div className="flex justify-center">
                                                    <ColumnConfigurator
                                                        columns={columns}
                                                        hiddenColumns={hiddenColumns}
                                                        onToggleColumn={toggleColumn}
                                                        onShowAll={showAllColumns}
                                                        onResetDefaults={resetColumnDefaults}
                                                        defaultHiddenColumns={defaultHiddenColumns}
                                                    />
                                                </div>
                                            ) : (
                                                <div className={`flex items-center gap-1 ${col.align === 'center' ? 'justify-center' : ''}`}>
                                                    {col.label}
                                                    {sortConfig.key === col.key && (
                                                        <span className="text-primary">
                                                            {sortConfig.direction === 'asc' ? '↑' : '↓'}
                                                        </span>
                                                    )}
                                                    {/* Column filter button */}
                                                    {(columnFilterTypes[col.key] === 'regex' || columnFilterTypes[col.key] === 'numeric' || (columnFilterTypes[col.key] === 'select' && columnUniqueValues[col.key])) && (
                                                        <div className="relative" ref={openColumnFilter === col.key ? columnFilterRef : undefined}>
                                                            <button
                                                                onClick={(e) => {
                                                                    e.stopPropagation();
                                                                    const opening = openColumnFilter !== col.key;
                                                                    setOpenColumnFilter(opening ? col.key : null);
                                                                    setColumnFilterSearch('');
                                                                    if (opening) {
                                                                        const rect = e.currentTarget.getBoundingClientRect();
                                                                        const dropdownWidth = 224; // w-56 = 14rem = 224px
                                                                        const left = Math.max(8, Math.min(rect.left, window.innerWidth - dropdownWidth - 8));
                                                                        // Place below the button initially; useLayoutEffect will correct if it overflows
                                                                        setColumnFilterPos({ top: rect.bottom + 4, left, anchorTop: rect.top, anchorBottom: rect.bottom });
                                                                        if (columnFilterTypes[col.key] === 'regex') {
                                                                            setRegexDraft('');
                                                                        } else if (columnFilterTypes[col.key] === 'numeric') {
                                                                            setNumericDraft({ operator: '>', value: '', unit: 'h' });
                                                                        }
                                                                    }
                                                                }}
                                                                className={`p-0.5 rounded transition-colors ${
                                                                    columnFilters[col.key]
                                                                        ? 'text-primary'
                                                                        : 'text-gray-600 opacity-0 group-hover:opacity-100 hover:text-gray-400'
                                                                }`}
                                                                title={columnFilters[col.key]
                                                                    ? columnFilters[col.key].type === 'regex'
                                                                        ? `Filtering: ${(columnFilters[col.key].conditions || []).map(p => `/${p}/`).join(` ${columnFilters[col.key].logic || 'and'} `)}`
                                                                        : columnFilters[col.key].type === 'numeric'
                                                                            ? `Filtering: ${(columnFilters[col.key].conditions || []).map(c => `${c.operator} ${c.value}`).join(` ${columnFilters[col.key].logic || 'and'} `)}`
                                                                            : `Filtering: ${columnFilters[col.key].values.size} value(s)`
                                                                    : `Filter by ${col.label}`
                                                                }
                                                            >
                                                                <FunnelIcon className="w-3 h-3" />
                                                            </button>
                                                            {/* Column filter dropdown rendered via portal below */}
                                                        </div>
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

            {/* Column filter dropdown — rendered via portal to escape overflow:hidden ancestors */}
            {openColumnFilter && columnFilterPos && (() => {
                const col = visibleColumns.find(c => c.key === openColumnFilter);
                if (!col) return null;
                return createPortal(
                    <div
                        ref={columnFilterDropdownRef}
                        className="fixed bg-surface border border-border rounded-lg shadow-xl z-[9999] w-56"
                        style={{ top: columnFilterPos.top, left: columnFilterPos.left }}
                        onClick={(e) => e.stopPropagation()}
                    >
                        {columnFilterTypes[col.key] === 'regex' ? (
                            <div className="p-2">
                                {/* Existing regex conditions */}
                                {columnFilters[col.key]?.conditions?.length > 0 && (
                                    <div className="mb-2 space-y-1">
                                        {columnFilters[col.key].conditions.map((pattern, idx) => (
                                            <div key={idx} className="flex items-center gap-1">
                                                {idx > 0 && (
                                                    <button
                                                        onClick={() => toggleFilterLogic(col.key)}
                                                        className="text-[10px] font-medium px-1 py-0.5 rounded bg-primary/20 text-primary hover:bg-primary/30 transition-colors shrink-0"
                                                    >
                                                        {columnFilters[col.key].logic === 'or' ? 'OR' : 'AND'}
                                                    </button>
                                                )}
                                                <span className="flex-1 text-xs font-mono text-gray-300 truncate">/{pattern}/</span>
                                                <button
                                                    onClick={() => removeRegexCondition(col.key, idx)}
                                                    className="text-gray-500 hover:text-red-400 transition-colors shrink-0"
                                                >
                                                    ×
                                                </button>
                                            </div>
                                        ))}
                                    </div>
                                )}
                                <input
                                    type="text"
                                    value={regexDraft}
                                    onChange={(e) => setRegexDraft(e.target.value)}
                                    placeholder="Regex pattern... (e.g. ^nginx)"
                                    className="w-full px-2 py-1.5 bg-background border border-border rounded text-xs font-mono focus:outline-none focus:border-primary"
                                    autoFocus
                                    autoComplete="off"
                                    onKeyDown={(e) => {
                                        if (e.key === 'Enter') {
                                            addRegexCondition(col.key, regexDraft);
                                            setRegexDraft('');
                                        } else if (e.key === 'Escape') {
                                            setOpenColumnFilter(null);
                                        }
                                    }}
                                />
                                <div className="text-[10px] text-gray-500 mt-1">Press Enter to add condition &middot; Escape to close</div>
                            </div>
                        ) : columnFilterTypes[col.key] === 'numeric' ? (
                            <div className="p-2">
                                {/* Existing numeric conditions */}
                                {columnFilters[col.key]?.conditions?.length > 0 && (
                                    <div className="mb-2 space-y-1">
                                        {columnFilters[col.key].conditions.map((cond, idx) => (
                                            <div key={idx} className="flex items-center gap-1">
                                                {idx > 0 && (
                                                    <button
                                                        onClick={() => toggleFilterLogic(col.key)}
                                                        className="text-[10px] font-medium px-1 py-0.5 rounded bg-primary/20 text-primary hover:bg-primary/30 transition-colors shrink-0"
                                                    >
                                                        {columnFilters[col.key].logic === 'or' ? 'OR' : 'AND'}
                                                    </button>
                                                )}
                                                <span className="flex-1 text-xs font-mono text-gray-300">{cond.operator} {cond.value}</span>
                                                <button
                                                    onClick={() => removeNumericCondition(col.key, idx)}
                                                    className="text-gray-500 hover:text-red-400 transition-colors shrink-0"
                                                >
                                                    ×
                                                </button>
                                            </div>
                                        ))}
                                    </div>
                                )}
                                <div className="flex gap-1.5">
                                    <select
                                        value={numericDraft.operator}
                                        onChange={(e) => setNumericDraft(d => ({ ...d, operator: e.target.value }))}
                                        className="px-1.5 py-1.5 bg-background border border-border rounded text-xs focus:outline-none focus:border-primary"
                                    >
                                        <option value=">">{'>'}</option>
                                        <option value=">=">{'>='}</option>
                                        <option value="<">{'<'}</option>
                                        <option value="<=">{'<='}</option>
                                        <option value="=">{'='}</option>
                                        <option value="!=">{'!='}</option>
                                    </select>
                                    <input
                                        type="number"
                                        value={numericDraft.value}
                                        onChange={(e) => setNumericDraft(d => ({ ...d, value: e.target.value }))}
                                        placeholder="0"
                                        className="flex-1 min-w-0 px-2 py-1.5 bg-background border border-border rounded text-xs font-mono focus:outline-none focus:border-primary"
                                        autoFocus
                                        autoComplete="off"
                                        onKeyDown={(e) => {
                                            if (e.key === 'Enter') {
                                                let filterValue = numericDraft.value;
                                                if (col.key === 'age' && filterValue !== '') {
                                                    const multipliers = { s: 1/3600, m: 1/60, h: 1, d: 24 };
                                                    filterValue = String(Number(filterValue) * (multipliers[numericDraft.unit] || 1));
                                                }
                                                addNumericCondition(col.key, numericDraft.operator, filterValue);
                                                setNumericDraft(d => ({ ...d, value: '' }));
                                            } else if (e.key === 'Escape') {
                                                setOpenColumnFilter(null);
                                            }
                                        }}
                                    />
                                    {col.key === 'age' && (
                                        <select
                                            value={numericDraft.unit}
                                            onChange={(e) => setNumericDraft(d => ({ ...d, unit: e.target.value }))}
                                            className="px-1.5 py-1.5 bg-background border border-border rounded text-xs focus:outline-none focus:border-primary"
                                        >
                                            <option value="s">sec</option>
                                            <option value="m">min</option>
                                            <option value="h">hours</option>
                                            <option value="d">days</option>
                                        </select>
                                    )}
                                </div>
                                {col.numericHint && (
                                    <div className="text-[10px] text-gray-500 mt-1">{col.numericHint}</div>
                                )}
                                <div className="text-[10px] text-gray-500 mt-0.5">Press Enter to add condition &middot; Escape to close</div>
                            </div>
                        ) : (
                            <>
                                <div className="p-2 border-b border-border">
                                    <input
                                        type="text"
                                        value={columnFilterSearch}
                                        onChange={(e) => setColumnFilterSearch(e.target.value)}
                                        placeholder="Search values..."
                                        className="w-full px-2 py-1 bg-background border border-border rounded text-xs focus:outline-none focus:border-primary"
                                        autoFocus
                                        autoComplete="off"
                                    />
                                </div>
                                <div className="max-h-48 overflow-auto py-1">
                                    {columnUniqueValues[col.key]?.values
                                        ?.filter(v => !columnFilterSearch || v.toLowerCase().includes(columnFilterSearch.toLowerCase()))
                                        .map(value => {
                                            const isChecked = columnFilters[col.key]?.values?.has(value);
                                            const count = columnUniqueValues[col.key]?.counts?.[value] ?? 0;
                                            const isZero = count === 0;
                                            return (
                                                <label
                                                    key={value}
                                                    className={`flex items-center gap-2 px-3 py-1 text-xs hover:bg-white/5 cursor-pointer ${isZero ? 'opacity-50' : ''}`}
                                                >
                                                    <input
                                                        type="checkbox"
                                                        checked={!!isChecked}
                                                        onChange={() => toggleColumnFilter(col.key, value)}
                                                        className="w-3 h-3 rounded border-gray-600 bg-background text-primary focus:ring-primary"
                                                    />
                                                    <span className="text-gray-300 truncate flex-1" title={value}>{value}</span>
                                                    <span className={`tabular-nums text-xs ml-auto ${isChecked ? 'text-primary' : isZero ? 'text-gray-600' : 'text-gray-400'}`}>{count}</span>
                                                </label>
                                            );
                                        })
                                    }
                                </div>
                            </>
                        )}
                        {columnFilters[col.key] && (
                            <div className="px-3 py-1.5 border-t border-border">
                                <button
                                    onClick={() => clearColumnFilter(col.key)}
                                    className="text-xs text-gray-500 hover:text-gray-300 transition-colors"
                                >
                                    Clear filter
                                </button>
                            </div>
                        )}
                    </div>,
                    document.body
                );
            })()}
        </div>
    );
}
