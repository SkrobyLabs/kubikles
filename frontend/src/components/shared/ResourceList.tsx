import ResourceLoadingStatus from './ResourceLoadingStatus';
import type { ResourceLoadState } from '~/hooks/useResource';
import React, { useState, useRef, useEffect, useLayoutEffect, useMemo, useCallback, useDeferredValue } from 'react';
import { createPortal } from 'react-dom';
import { TableVirtuoso } from 'react-virtuoso';
import { MagnifyingGlassIcon, InformationCircleIcon, FunnelIcon, ExclamationTriangleIcon, XMarkIcon } from '@heroicons/react/24/outline';
import { EventsOn } from 'wailsjs/runtime/runtime';
import SearchSelect from './SearchSelect';
import BulkActionBar from './BulkActionBar';
import RefreshControls from './RefreshControls';
import SavedViewsDropdown from './SavedViewsDropdown';
import { createFilter, getFieldsMetadata } from '~/utils/search';
import { namespaceSelection } from '~/utils/namespaceQuery';
import { useSavedViews } from '~/hooks/useSavedViews';
import type { SavedView } from '~/hooks/useSavedViews';
import { useConfig, useUI } from '~/context';
import { useK8sActions } from '~/context/K8sContext';
import { type ColumnDef, type ColumnWidths, MIN_COLUMN_WIDTHS, DEFAULT_COLUMN_WIDTHS, calculateColumnWidths, formatCount } from './resourceListColumns';
import { SearchHighlightOverlay } from './ResourceListControls';
import ResourceListHeader from './ResourceListHeader';
import { RowCells, SkeletonCells, virtuosoComponents } from './ResourceListRows';
import type { ViewHandle, ViewSnapshot } from '~/lib/views/viewClient';
import { resolveSchema, filterTypeIn, columnKeysIn } from '~/lib/views/schema';
import { toColumnFilterSpecs } from '~/lib/views/viewColumns';
import { refItem } from '~/lib/views/rowItem';
import type { Facet } from '~/lib/views/protocol';
import {
    type ColumnFilter,
    type ColumnFiltersMap,
    type SortConfig,
    PREDEFINED_COLUMN_VALUES,
    compileColumnFilters,
    computeColumnUniqueValues,
    detectColumnFilterTypes,
    selectColumnHasFilterValues,
    sortItems,
} from './resourceListPipeline';

// Heartbeat for time-dependent cells (age) now that rows are memoised.
const ROW_CLOCK_MS = 5000;

// Pixels rendered beyond the viewport.
const VIRTUOSO_OVERSCAN_PX = 400;

// Rows are keyed by identity, not index, so per-row state (an open actions menu, hover) stays
// attached to the same resource across re-sorts. Non-Kubernetes rows (Helm releases) have no
// uid and fall back to namespace/name.
/**
 * View-mode scroller. The app styles scrollbars as an 8px classic bar, which appears and disappears
 * as a search shrinks the list below one screen; without a reserved gutter every column then shifts
 * sideways by the bar width. scrollbar-gutter keeps the width constant (WebKit 18.2+, Chromium);
 * overflow-y: scroll covers older WebKit builds where the property is not honoured.
 */
export const VIEW_SCROLLER_STYLE: React.CSSProperties = { height: '100%', overflowY: 'scroll', scrollbarGutter: 'stable' };

/** Noun for the bulk action bar. Custom resource lists use an internal id ("cr-group-plural"), which is never shown. */
export const bulkResourceLabel = (resourceType: string | null | undefined, title: string): string =>
    resourceType && !resourceType.startsWith('cr-') ? resourceType : title.toLowerCase();

const computeRowKey = (index: number, item: any): string | number =>
    item?.metadata?.uid ?? (item?.name ? `${item.namespace ?? ''}/${item.name}` : index);

// Position state for the column filter dropdown
interface ColumnFilterPosition {
    top: number;
    left: number;
    anchorTop?: number;
    anchorBottom?: number;
}

// Selection object passed from parent
interface SelectionState {
    isSelected: (uid: string) => boolean;
    getSelectionState: (data: any[]) => 'none' | 'some' | 'all';
    toggleAll: (data: any[]) => void;
    toggleItem: (uid: string, index: number, data: any[], shiftKey: boolean) => void;
    getSelectedItems: (data: any[]) => any[];
    deselectAll: () => void;
    pruneSelection: (data: any[]) => void;
    selectedCount: number;
    // Windowed (view) lists select by identity; these are only used with a `source`.
    selectedIds?: Set<string>;
    toggleRef?: (ref: any, index: number) => void;
    addRefs?: (refs: any[], index?: number) => void;
    setRefs?: (refs: any[]) => void;
    getRefItems?: () => any[];
    getLastIndex?: () => number | null;
    pruneByUids?: (uids: Iterable<string>) => void;
}

// Resizing ref type
interface ResizeState {
    columnKey: string;
    startX: number;
    startWidth: number;
}

// Loading progress from paginated listing
interface LoadingProgress {
    loaded: number;
    total: number;
}

// ResourceList props
interface ResourceListProps {
    title: string;
    columns: any[];
    data: any[];
    isLoading: boolean;
    loadState?: ResourceLoadState;
    loadingProgress?: LoadingProgress | null;
    namespaces?: string[];
    currentNamespace?: any;
    onNamespaceChange?: ((...args: any[]) => void) | null;
    showNamespaceSelector?: boolean;
    multiSelectNamespaces?: boolean;
    highlightedUid?: string | null;
    initialSort?: any;
    resourceType?: string | null;
    /** Plural noun for the selection bar ("3 prometheusrules selected"). Defaults to the resource type, or the title for internal ids. */
    selectionLabel?: string;
    onRowClick?: ((item: any) => void) | null;
    onRefresh?: (() => void) | null;
    customHeaderActions?: React.ReactNode | null;
    customBody?: React.ReactNode | null;
    selection?: SelectionState | null;
    selectable?: boolean;
    onBulkDelete?: ((items: any[]) => void) | null;
    onBulkRestart?: ((items: any[]) => void) | null;
    onBulkExportYaml?: ((items: any[]) => void) | null;
    bulkCustomActions?: React.ReactNode | null;
    onFilteredUidsChange?: ((uids: Set<string>) => void) | null;
    /**
     * Changes whenever values read by columns marked `volatile` (e.g. live metrics) change.
     * Sorting or filtering by a volatile column re-runs only while that column is in use;
     * otherwise changes here never re-sort or re-filter the list.
     */
    volatileInputs?: unknown;
    /**
     * A backend view serving this list. When set, the rows come from the view (windowed,
     * sorted, filtered and searched by the backend) and `data` is ignored.
     */
    source?: ViewHandle | null;
    /** Schema columns always projected in view mode even when hidden (cells a visible column's render reads). */
    viewPinnedColumns?: readonly string[];
    /** Pinned columns that are data carriers only and never appear as columns. Defaults to all pinned columns. */
    viewCarrierColumns?: readonly string[];
}

const EMPTY_ROWS: any[] = [];
const STATIC_PIPELINE = Object.freeze({
    data: EMPTY_ROWS, searchTerm: '', columnFilters: {} as ColumnFiltersMap,
    sortConfig: { key: null, direction: 'asc' } as SortConfig, sortVolatile: undefined, filterVolatile: undefined,
});
const SEARCH_FILTER_PUSH_MS = 0;
const FACET_REFRESH_MS = 1500;

// Minimum column widths - ensure columns never get too narrow

// Large dataset banner — shows progress during loading or warning after load
function LargeDatasetBanner({ isLoading, progress, dataCount, threshold }: {
    isLoading: boolean;
    progress: LoadingProgress | null;
    dataCount: number;
    threshold: number;
}) {
    const [dismissed, setDismissed] = useState(false);

    // Reset dismissed state when loading starts
    useEffect(() => {
        if (isLoading) setDismissed(false);
    }, [isLoading]);

    // Mode 1: Loading with progress (only for large datasets worth tracking)
    if (isLoading && progress && progress.total > threshold) {
        const pct = Math.min(100, Math.round((progress.loaded / progress.total) * 100));
        return (
            <div className="px-4 py-2 border-b border-border bg-blue-500/10 shrink-0 flex items-center gap-3 text-sm text-blue-400">
                <div className="shrink-0">
                    Loading... {formatCount(progress.loaded)} / ~{formatCount(progress.total)}
                </div>
                <div className="flex-1 h-1.5 bg-blue-500/20 rounded-full overflow-hidden">
                    <div
                        className="h-full bg-blue-500 rounded-full transition-all duration-300"
                        style={{ width: `${pct}%` }}
                    />
                </div>
                <div className="shrink-0 text-xs text-blue-400/70">{pct}%</div>
            </div>
        );
    }

    // Mode 2: Post-load warning
    if (!isLoading && dataCount > threshold && !dismissed) {
        return (
            <div className="px-4 py-1.5 border-b border-border bg-yellow-500/10 shrink-0 flex items-center gap-2 text-sm text-yellow-400">
                <ExclamationTriangleIcon className="w-4 h-4 shrink-0" />
                <span>Large dataset: {formatCount(dataCount)} items. Filtering may be slower.</span>
                <button
                    onClick={() => setDismissed(true)}
                    className="ml-auto p-0.5 hover:bg-yellow-500/20 rounded transition-colors"
                    title="Dismiss"
                >
                    <XMarkIcon className="w-4 h-4" />
                </button>
            </div>
        );
    }

    return null;
}

export default function ResourceList({
    title,
    columns: columnsProp,
    data,
    isLoading,
    loadState,
    loadingProgress: externalProgress = null,
    namespaces = [],
    currentNamespace,
    onNamespaceChange,
    showNamespaceSelector = true,
    multiSelectNamespaces = false,
    highlightedUid = null,
    initialSort = null,
    resourceType = null,
    selectionLabel,
    onRowClick = null,
    onRefresh = null,
    customHeaderActions = null,
    customBody = null,
    // Selection props
    selection = null,
    selectable = false,
    // Bulk action callbacks
    onBulkDelete = null,
    onBulkRestart = null,
    onBulkExportYaml = null,
    bulkCustomActions = null,
    onFilteredUidsChange = null,
    volatileInputs,
    source = null,
    viewPinnedColumns,
    viewCarrierColumns,
}: ResourceListProps) {
    // Pinned columns are always projected (see viewColumnKeys) because other columns render from
    // their cells. Those that exist only for that purpose (carriers) are never shown or offered in
    // the column picker; a pinned column that is also a real column (Replicas) stays available.
    // Without an explicit carrier list every pinned column is treated as a carrier.
    const columns = useMemo(() => {
        const hidden = viewCarrierColumns ?? viewPinnedColumns;
        if (!source || !hidden || hidden.length === 0) return columnsProp;
        const carriers = new Set<string>(hidden);
        return columnsProp.filter((col: any) => !carriers.has(col.key));
    }, [columnsProp, viewPinnedColumns, viewCarrierColumns, source]);
    const { pendingSearch, consumePendingSearch } = useUI();
    const { getConfig } = useConfig();
    const { refreshNamespaces } = useK8sActions();

    // View mode: the backend owns sorting, filtering, searching and windowing.
    const viewMode = !!source;
    // The snapshot is tracked with plain state so a row of the table never reads a different
    // source's counts: the stored pair is only used while it belongs to the current source.
    const [viewHeld, setViewHeld] = useState<{ source: ViewHandle | null; snap: ViewSnapshot | null }>(
        () => ({ source, snap: source ? source.getSnapshot() : null }));
    useEffect(() => {
        if (!source) { setViewHeld(held => (held.source === null ? held : { source: null, snap: null })); return; }
        setViewHeld({ source, snap: source.getSnapshot() });
        return source.subscribe(() => setViewHeld({ source, snap: source.getSnapshot() }));
    }, [source]);
    // A list mounting over a view that is already open (the node list after its topology diagram)
    // asks for its window again: the same row range as before would otherwise fetch nothing.
    useEffect(() => { source?.refresh(); }, [source]);
    const viewSnap = source ? (viewHeld.source === source ? viewHeld.snap : source.getSnapshot()) : null;
    // Custom resource views bring their own schema with the open result.
    const viewSchema = viewSnap?.schema ?? null;
    const activeSchema = useMemo(() => resolveSchema(resourceType || undefined, viewSchema), [resourceType, viewSchema]);
    const viewFiltered = viewSnap?.filtered ?? 0;
    const viewTotal = viewSnap?.total ?? 0;
    const viewEpoch = viewSnap?.epoch ?? 0;
    // Counts are zero until the view has opened and its first load has finished. The load state
    // arrives through another path and can report "complete" a render earlier than these counts,
    // which would show the empty message for a frame.
    const viewPending = !!viewSnap && (viewSnap.status === 'idle' || viewSnap.status === 'opening'
        || (viewSnap.status === 'open' && (viewSnap.state?.phase ?? 'loading') === 'loading'));
    const sourceRef = useRef(source);
    sourceRef.current = source;

    // Loading progress: prefer external prop, fall back to internal listener
    const [internalProgress, setInternalProgress] = useState<LoadingProgress | null>(null);
    useEffect(() => {
        if (!isLoading || !resourceType || loadState) {
            setInternalProgress(null);
            return;
        }
        const cancel = EventsOn("list-progress", (event: any) => {
            if (event?.resourceType === resourceType) {
                setInternalProgress({ loaded: event.loaded, total: event.total });
            }
        });
        return () => { cancel(); setInternalProgress(null); };
    }, [isLoading, resourceType, loadState]);
    const loadingProgress = loadState?.progress ?? externalProgress ?? internalProgress;
    const largeDatasetThreshold = getConfig('ui.largeDatasetThreshold') ?? 5000;
    const [sortConfig, setSortConfig] = useState<{ key: string | null; direction: string }>(initialSort || { key: null, direction: 'asc' });
    const [searchInput, setSearchInput] = useState(''); // Immediate input value
    const [searchTerm, setSearchTerm] = useState('');   // Debounced value for filtering
    const [hiddenColumns, setHiddenColumns] = useState(() => {
        // Initialize with columns marked as defaultHidden
        return new Set(columns.filter((col: any) => col.defaultHidden).map((col: any) => col.key));
    });
    // A windowed list swaps its columns when the view opens, and the swapped-in set can carry
    // columns the legacy set never had (cells another column renders from). Columns that appear
    // later keep their default visibility instead of showing up in the table.
    const seenColumnKeys = useRef<Set<string> | null>(null);
    if (seenColumnKeys.current === null) seenColumnKeys.current = new Set(columns.map((col: any) => col.key));
    useEffect(() => {
        const seen = seenColumnKeys.current!;
        const fresh = columns.filter((col: any) => !seen.has(col.key));
        if (fresh.length === 0) return;
        for (const col of fresh) seen.add(col.key);
        const hide = fresh.filter((col: any) => col.defaultHidden).map((col: any) => col.key);
        if (hide.length > 0) setHiddenColumns(prev => new Set([...prev, ...hide]));
    }, [columns]);
    const [activeViewId, setActiveViewId] = useState<string | null>(null);

    // Saved views
    const { views, saveView, loadView, updateView, deleteView, renameView, duplicateView, setDefaultView, getDefaultView } = useSavedViews(resourceType);

    // Debounce search input to avoid filtering on every keystroke
    const searchDebounceMs = getConfig('ui.searchDebounceMs') ?? 150;
    useEffect(() => {
        const timer = setTimeout(() => setSearchTerm(searchInput), searchDebounceMs);
        return () => clearTimeout(timer);
    }, [searchInput, searchDebounceMs]);
    const [columnFilters, setColumnFilters] = useState<ColumnFiltersMap>({}); // { [columnKey]: { type, values|pattern } }
    const [openColumnFilter, setOpenColumnFilter] = useState<string | null>(null); // column key of open filter dropdown
    const [columnFilterSearch, setColumnFilterSearch] = useState(''); // search within filter dropdown
    const [regexDraft, setRegexDraft] = useState(''); // draft regex pattern (applied on Enter)
    const [numericDraft, setNumericDraft] = useState<{ operator: string; value: string; unit: string }>({ operator: '>', value: '', unit: 'h' }); // draft numeric filter
    const [columnFilterPos, setColumnFilterPos] = useState<ColumnFilterPosition | null>(null); // { top, left } for fixed-position dropdown
    const columnFilterRef = useRef<HTMLDivElement>(null); // ref for the filter button wrapper
    const columnFilterDropdownRef = useRef<HTMLDivElement>(null); // ref for the fixed-position dropdown
    const [showSearchHelp, setShowSearchHelp] = useState(false);
    const searchHelpRef = useRef<HTMLDivElement>(null);

    // Column reordering state (user-saved order)
    const [columnOrder, setColumnOrder] = useState<string[]>(() => {
        if (!resourceType) return [];
        try {
            const saved = localStorage.getItem(`kubikles_colorder_${resourceType}`);
            return saved ? JSON.parse(saved) : [];
        } catch {
            return [];
        }
    });

    // Persist column order to localStorage
    useEffect(() => {
        if (!resourceType) return;
        if (columnOrder.length > 0) {
            localStorage.setItem(`kubikles_colorder_${resourceType}`, JSON.stringify(columnOrder));
        } else {
            localStorage.removeItem(`kubikles_colorder_${resourceType}`);
        }
    }, [columnOrder, resourceType]);

    // Get current view config for saving (defined after columnFilters state)
    const getCurrentViewConfig = useCallback(() => {
        // Serialize columnFilters — convert Sets to arrays for JSON storage
        const serializedFilters: Record<string, any> = {};
        for (const [key, filter] of Object.entries(columnFilters)) {
            if (filter.type === 'select') {
                serializedFilters[key] = { ...filter, values: Array.from(filter.values || []) };
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
            columnOrder: columnOrder.length > 0 ? columnOrder : undefined,
            resourceType,
        };
    }, [searchInput, currentNamespace, hiddenColumns, sortConfig, columnFilters, columnOrder, resourceType]);

    // Load a saved view (or reset to defaults if viewId is null)
    const handleLoadView = useCallback((viewId: string | null) => {
        if (!viewId) {
            // Reset to application defaults
            setActiveViewId(null);
            setSearchInput('');
            setSearchTerm('');
            setHiddenColumns(new Set(columns.filter((col: any) => col.defaultHidden).map((col: any) => col.key)));
            setSortConfig(initialSort || { key: null, direction: 'asc' });
            setColumnFilters({});
            setColumnOrder([]);
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
            setHiddenColumns(new Set<any>());
        }
        setSortConfig(view.sortConfig || { key: null, direction: 'asc' });
        // Restore column filters — deserialize arrays back to Sets
        if (view.columnFilters && Object.keys(view.columnFilters).length > 0) {
            const restored: ColumnFiltersMap = {};
            for (const [key, filter] of Object.entries(view.columnFilters)) {
                const f = filter as any;
                if (f.type === 'select' && Array.isArray(f.values)) {
                    restored[key] = { ...f, values: new Set(f.values) };
                } else {
                    restored[key] = f;
                }
            }
            setColumnFilters(restored);
        } else {
            setColumnFilters({});
        }
        setColumnOrder(view.columnOrder || []);
        if (view.namespace && onNamespaceChange) {
            onNamespaceChange(view.namespace);
        }
    }, [loadView, onNamespaceChange, columns, initialSort]);

    // Handle updating an existing view with current config
    const handleUpdateView = useCallback((viewId: string, config: any) => {
        // Serialize Sets to arrays for storage
        const serializedFilters: Record<string, any> = {};
        for (const [key, filter] of Object.entries(config.columnFilters || {})) {
            const f = filter as any;
            if (f.type === 'select' && f.values instanceof Set) {
                serializedFilters[key] = { ...f, values: Array.from(f.values) };
            } else {
                serializedFilters[key] = f;
            }
        }
        updateView(viewId, {
            query: config.query,
            namespace: config.namespace,
            hiddenColumns: config.hiddenColumns,
            sortConfig: config.sortConfig,
            columnFilters: serializedFilters,
            columnOrder: config.columnOrder,
        });
    }, [updateView]);

    // Handle deleting a view - reset to defaults if deleting the active view
    const handleDeleteView = useCallback((viewId: string) => {
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
    const resizingRef = useRef<ResizeState | null>(null);

    // Track data length to know when to recalculate widths
    // We use a ref to store the calculated widths so they persist across scrolls
    const calculatedWidthsRef = useRef<ColumnWidths>({});
    const lastDataLengthRef = useRef(0);

    // Calculate column widths based on data - only recalculate when data changes significantly
    // In view mode only the rows of the current window exist on this side, so widths come from them.
    const widthData = useMemo(() => (source ? source.loadedRows() : data),
        // eslint-disable-next-line react-hooks/exhaustive-deps
        [source, viewEpoch, data]);
    const calculatedWidths = useMemo(() => {
        // Recalculate if data length changed by more than 10% or is new
        const dataLengthChanged = Math.abs(widthData.length - lastDataLengthRef.current) > lastDataLengthRef.current * 0.1;
        const isNewData = lastDataLengthRef.current === 0 && widthData.length > 0;

        if (isNewData || dataLengthChanged) {
            const next = calculateColumnWidths(columns, widthData, savedColumnWidths);
            if (source) {
                // A view holds only the rows of its window, and a search or filter changes which
                // rows those are. Widths measured from each new set would move every column, so
                // in view mode a column only ever widens.
                const prev = calculatedWidthsRef.current;
                const merged: ColumnWidths = { ...next };
                for (const key of Object.keys(prev)) merged[key] = Math.max(prev[key] ?? 0, next[key] ?? 0);
                calculatedWidthsRef.current = merged;
            } else {
                calculatedWidthsRef.current = next;
            }
            lastDataLengthRef.current = widthData.length;
        }

        return calculatedWidthsRef.current;
    }, [widthData, columns, savedColumnWidths, source]);

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
    const handleResizeStart = useCallback((e: React.MouseEvent, columnKey: string) => {
        e.preventDefault();
        e.stopPropagation();

        const startX = e.clientX;
        // The handle contains a hover marker; a press on it must still measure the header cell,
        // not the handle (a few pixels wide), or the column collapses and trails the cursor.
        const th = (e.target as HTMLElement).closest('th');
        if (!th) return;
        const startWidth = th.offsetWidth;

        resizingRef.current = { columnKey, startX, startWidth };
        document.body.style.cursor = 'col-resize';
        document.body.style.userSelect = 'none';

        const handleMouseMove = (moveEvent: MouseEvent) => {
            if (!resizingRef.current) return;
            const diff = moveEvent.clientX - resizingRef.current.startX;
            const newWidth = Math.max(50, resizingRef.current.startWidth + diff);
            setSavedColumnWidths((prev: ColumnWidths) => ({
                ...prev,
                [resizingRef.current!.columnKey]: newWidth
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
    const handleResizeDoubleClick = useCallback((e: React.MouseEvent, columnKey: string) => {
        e.preventDefault();
        e.stopPropagation();
        setSavedColumnWidths((prev: ColumnWidths) => {
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
        const handleClickOutside = (event: MouseEvent) => {
            if (searchHelpRef.current && !searchHelpRef.current.contains(event.target as Node)) {
                setShowSearchHelp(false);
            }
            if (columnFilterRef.current && !columnFilterRef.current.contains(event.target as Node) &&
                (!columnFilterDropdownRef.current || !columnFilterDropdownRef.current.contains(event.target as Node))) {
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
        let { top, left, anchorTop } = columnFilterPos;
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
            setColumnFilterPos(prev => prev ? { ...prev, top, left } : prev);
        }
    }, [openColumnFilter, columnFilterPos]);

    // Determine filter type per column: 'regex' | 'select' | 'numeric' | false
    const columnFilterTypes = useMemo(() => {
        if (!viewMode || !resourceType) return detectColumnFilterTypes(columns);
        // The generated schema is the single source of filter types in view mode.
        const types: Record<string, any> = {};
        for (const col of columns) {
            types[col.key] = col.filterable === false || col.isColumnSelector ? false : filterTypeIn(activeSchema, col.key);
        }
        return types;
    }, [columns, viewMode, resourceType, activeSchema]);

    // Toggle a value in a select-type column filter
    const toggleColumnFilter = useCallback((colKey: string, value: string) => {
        setColumnFilters(prev => {
            const current: Set<string> = prev[colKey]?.type === 'select' ? new Set(prev[colKey].values) : new Set<any>();
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
    const addRegexCondition = useCallback((colKey: string, pattern: string) => {
        if (!pattern) return;
        setColumnFilters(prev => {
            const existing = prev[colKey];
            const conditions = existing?.type === 'regex' ? [...(existing.conditions || [])] : [];
            conditions.push(pattern);
            return { ...prev, [colKey]: { type: 'regex', conditions, logic: existing?.logic || 'and' } };
        });
    }, []);

    // Remove a regex condition by index
    const removeRegexCondition = useCallback((colKey: string, index: number) => {
        setColumnFilters(prev => {
            const existing = prev[colKey];
            if (!existing || existing.type !== 'regex') return prev;
            const conditions = (existing.conditions || []).filter((_: any, i: number) => i !== index);
            if (conditions.length === 0) {
                const next = { ...prev };
                delete next[colKey];
                return next;
            }
            return { ...prev, [colKey]: { ...existing, conditions } };
        });
    }, []);

    // Add a numeric condition to a column filter
    const addNumericCondition = useCallback((colKey: string, operator: string, value: string) => {
        if (value === '' || value === null || isNaN(Number(value))) return;
        setColumnFilters(prev => {
            const existing = prev[colKey];
            const conditions = existing?.type === 'numeric' ? [...(existing.conditions || [])] : [];
            conditions.push({ operator, value: Number(value) });
            return { ...prev, [colKey]: { type: 'numeric', conditions, logic: existing?.logic || 'and' } };
        });
    }, []);

    // Remove a numeric condition by index
    const removeNumericCondition = useCallback((colKey: string, index: number) => {
        setColumnFilters(prev => {
            const existing = prev[colKey];
            if (!existing || existing.type !== 'numeric') return prev;
            const conditions = (existing.conditions || []).filter((_: any, i: number) => i !== index);
            if (conditions.length === 0) {
                const next = { ...prev };
                delete next[colKey];
                return next;
            }
            return { ...prev, [colKey]: { ...existing, conditions } };
        });
    }, []);

    // Toggle AND/OR logic for a column filter
    const toggleFilterLogic = useCallback((colKey: string) => {
        setColumnFilters(prev => {
            const existing = prev[colKey];
            if (!existing) return prev;
            return { ...prev, [colKey]: { ...existing, logic: existing.logic === 'and' ? 'or' : 'and' } };
        });
    }, []);

    // Clear all filters for a column
    const clearColumnFilter = useCallback((colKey: string) => {
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

    const toggleColumn = useCallback((key: string) => {
        setHiddenColumns((prev: Set<string>) => {
            const next = new Set(prev);
            if (next.has(key)) {
                next.delete(key);
            } else {
                next.add(key);
            }
            return next;
        });
    }, []);

    // Show all columns
    const showAllColumns = useCallback(() => {
        setHiddenColumns(new Set<any>());
    }, []);

    // Reset columns to default visibility (determined by column.defaultHidden property)
    const resetColumnDefaults = useCallback(() => {
        const defaultHidden = new Set(
            columns.filter((col: any) => col.defaultHidden).map((col: any) => col.key)
        );
        setHiddenColumns(defaultHidden);
    }, [columns]);

    // Compute default hidden columns set for ColumnConfigurator
    const defaultHiddenColumns = useMemo(() => {
        return new Set(columns.filter((col: any) => col.defaultHidden).map((col: any) => col.key));
    }, [columns]);

    const baseVisibleColumns = useMemo(() => columns.filter((col: any) => !hiddenColumns.has(col.key)), [columns, hiddenColumns]);

    // Apply column order — reorder data columns while keeping column-selector at end
    const orderedVisibleColumns = useMemo(() => {
        if (columnOrder.length === 0) return baseVisibleColumns;

        // Separate column-selector (always last) from data columns
        const colSelectorIdx = baseVisibleColumns.findIndex((c: any) => c.isColumnSelector);
        const colSelector = colSelectorIdx >= 0 ? baseVisibleColumns[colSelectorIdx] : null;
        const dataCols = baseVisibleColumns.filter((c: any) => !c.isColumnSelector);

        // Build order map for known keys
        const orderMap = new Map(columnOrder.map((key, idx) => [key, idx]));

        // Partition into ordered and unordered columns
        const ordered = dataCols.filter((c: any) => orderMap.has(c.key));
        const unordered = dataCols.filter((c: any) => !orderMap.has(c.key));

        // Sort ordered columns by their position in columnOrder
        ordered.sort((a: any, b: any) => orderMap.get(a.key)! - orderMap.get(b.key)!);

        const result = [...ordered, ...unordered];
        if (colSelector) result.push(colSelector);
        return result;
    }, [baseVisibleColumns, columnOrder]);

    const handleSort = useCallback((key: string) => {
        setSortConfig((prev: SortConfig) => ({
            key,
            direction: prev.key === key && prev.direction === 'asc' ? 'desc' : 'asc',
        }));
    }, []);

    // Sorting or filtering by a volatile column (live metrics) must follow value changes;
    // for every other column those changes are ignored so a metrics tick costs nothing here.
    const sortColumn = sortConfig.key ? columns.find((c: any) => c.key === sortConfig.key) : undefined;
    const sortVolatile = sortColumn?.volatile ? volatileInputs : undefined;
    const filterVolatile = Object.keys(columnFilters).some(k => columns.find((c: any) => c.key === k)?.volatile)
        ? volatileInputs
        : undefined;

    // The heavy derived data below is computed from a deferred snapshot of its inputs. Urgent
    // updates (typing, header clicks, menus) render first and the filter/sort pass follows at
    // low priority, so a burst of watch batches cannot block input. Rows are keyed by uid, so
    // in-place item updates still show once the deferred render lands.
    const pipelineInputs = useMemo(() => (viewMode ? STATIC_PIPELINE : {
        data, searchTerm, columnFilters, sortConfig, sortVolatile, filterVolatile,
    }), [viewMode, data, searchTerm, columnFilters, sortConfig, sortVolatile, filterVolatile]);
    const lagging = useDeferredValue(pipelineInputs);
    // A reset to empty (context or scope switch) must not keep serving the previous
    // cluster's rows to selection and bulk actions while the low-priority render is pending.
    const deferredInputs = data.length === 0 && lagging.data.length > 0 ? pipelineInputs : lagging;
    const deferredData = deferredInputs.data;

    const filteredData = useMemo(() => {
        if (viewMode) return EMPTY_ROWS;
        let result = deferredData;

        // Apply search term filter
        if (deferredInputs.searchTerm) {
            const filterFn = createFilter(resourceType as string, deferredInputs.searchTerm);
            result = result.filter(filterFn);
        }

        // Apply per-column filters (select, regex and numeric), compiled once per pass
        if (Object.keys(deferredInputs.columnFilters).length > 0) {
            const predicate = compileColumnFilters(columns, deferredInputs.columnFilters);
            if (predicate) result = result.filter(predicate);
        }

        return result;
        // filterVolatile is a deliberate dependency: it re-runs the pass when live values change
        // while a filter on a volatile column is active.
    }, [viewMode, deferredData, deferredInputs.searchTerm, deferredInputs.columnFilters, deferredInputs.filterVolatile, resourceType, columns]);

    // Report filtered UIDs to parent (for notifications scoping, etc.)
    // Compare against the previously reported set without allocating, and only build a new
    // Set (and call the parent) when the UIDs really changed.
    const prevFilteredUidsRef = useRef<Set<string> | null>(null);
    useEffect(() => {
        if (!onFilteredUidsChange || viewMode) return;
        const prev = prevFilteredUidsRef.current;
        let count = 0;
        let same = prev !== null;
        for (const item of filteredData) {
            const uid = item.metadata?.uid;
            if (!uid) continue;
            count++;
            if (same && !prev!.has(uid)) same = false;
        }
        if (same && prev!.size === count) return; // No change
        const uids = new Set<string>();
        for (const item of filteredData) {
            const uid = item.metadata?.uid;
            if (uid) uids.add(uid);
        }
        prevFilteredUidsRef.current = uids;
        onFilteredUidsChange(uids);
    }, [filteredData, onFilteredUidsChange, viewMode]);

    const sortedData = useMemo(
        () => (viewMode ? EMPTY_ROWS : sortItems(filteredData, columns, deferredInputs.sortConfig)),
        // sortVolatile re-sorts live while sorted by a volatile column (see above).
        [viewMode, filteredData, columns, deferredInputs.sortConfig, deferredInputs.sortVolatile],
    );
    // Rows the list currently shows, whichever side produced them.
    const shownCount = viewMode ? viewFiltered : sortedData.length;
    const knownCount = viewMode ? viewTotal : data.length;

    // Auto-open details for the first matching row after search + filter settles
    useEffect(() => {
        if (viewMode) {
            const first = source?.rowAt(0);
            if (autoOpenDetailsRef.current && first && onRowClick) {
                autoOpenDetailsRef.current = false;
                requestAnimationFrame(() => onRowClick(first));
            }
            return;
        }
        if (autoOpenDetailsRef.current && sortedData.length > 0 && onRowClick) {
            autoOpenDetailsRef.current = false;
            // Defer to let the UI render the list first
            requestAnimationFrame(() => onRowClick(sortedData[0]));
        }
    }, [sortedData, onRowClick, viewMode, source, viewEpoch]);

    // Add selection checkbox column if selection is enabled
    const visibleColumns = useMemo(() => {
        if (!selectable || !selection) return orderedVisibleColumns;

        const checkboxColumn = {
            key: '_selection',
            label: '',
            isSelectionColumn: true,
            width: MIN_COLUMN_WIDTHS._selection,
        };

        return [checkboxColumn, ...orderedVisibleColumns];
    }, [selectable, selection, orderedVisibleColumns]);

    // Prune stale selections when data changes (e.g. after bulk delete/restart)
    useEffect(() => {
        if (viewMode) return;
        if (selectable && selection && selection.selectedCount > 0) {
            selection.pruneSelection(sortedData);
        }
    }, [sortedData, viewMode]);

    // View mode: the backend tracks the selected uids and reports the ones that left the scope.
    const selectedIds = selection?.selectedIds;
    useEffect(() => {
        if (!source || !selectable || !selection || !selectedIds) return;
        source.setSelection(selectedIds);
    }, [source, selectable, selectedIds]);
    useEffect(() => {
        if (!source || !selection?.pruneByUids) return;
        return source.onSelectionStale(uids => selectionRef.current?.pruneByUids?.(uids));
    }, [source]);

    // Compute selection state for the current filtered/sorted data
    const selectionState = useMemo(() => {
        if (!selectable || !selection) return 'none';
        if (viewMode) {
            // Without the rows at hand, "all" means at least as many selected as rows shown.
            if (selection.selectedCount === 0) return 'none';
            return viewFiltered > 0 && selection.selectedCount >= viewFiltered ? 'all' : 'some';
        }
        return selection.getSelectionState(sortedData);
    }, [selectable, selection, sortedData, viewMode, viewFiltered]);

    // Click handlers read the latest rows/selection from refs so their identity (and with it
    // the memoised header and row props) does not change on every data update.
    const sortedDataRef = useRef(sortedData);
    sortedDataRef.current = sortedData;
    const selectionRef = useRef(selection);
    selectionRef.current = selection;

    // Handle header checkbox click
    const handleHeaderCheckboxClick = useCallback(() => {
        const src = sourceRef.current;
        const sel = selectionRef.current;
        if (src && sel) {
            // Same rule as the legacy toggle: anything selected clears, otherwise select every filtered row.
            if (sel.selectedCount > 0) {
                sel.deselectAll();
            } else {
                void src.selectAllRefs().then(refs => sel.setRefs?.(refs.map(r => refItem(r.uid, r.ns, r.name)))).catch(() => {});
            }
            return;
        }
        sel?.toggleAll(sortedDataRef.current);
    }, []);

    // Handle row checkbox click
    const handleRowCheckboxClick = useCallback((e: any, item: any, index: number) => {
        const uid = item?.metadata?.uid;
        if (!uid) return;
        const src = sourceRef.current;
        const sel = selectionRef.current;
        if (src && sel) {
            const last = sel.getLastIndex?.() ?? null;
            if (e.shiftKey && last !== null) {
                const from = Math.min(last, index);
                const to = Math.max(last, index);
                void src.refs(from, to).then(refs => sel.addRefs?.(refs.map(r => refItem(r.uid, r.ns, r.name)), index)).catch(() => {});
            } else {
                sel.toggleRef?.(refItem(uid, item.metadata.namespace ?? '', item.metadata.name), index);
            }
            return;
        }
        selectionRef.current?.toggleItem(uid, index, sortedDataRef.current, e.shiftKey);
    }, []);

    const selectedForBulk = useCallback((): any[] => {
        if (!selection) return [];
        return viewMode && selection.getRefItems ? selection.getRefItems() : selection.getSelectedItems(sortedData);
    }, [selection, sortedData, viewMode]);

    // Bulk action handlers - pass selected items to callbacks
    const handleBulkDelete = useCallback(() => {
        if (!selection || !onBulkDelete) return;
        onBulkDelete(selectedForBulk());
    }, [selection, selectedForBulk, onBulkDelete]);

    const handleBulkRestart = useCallback(() => {
        if (!selection || !onBulkRestart) return;
        onBulkRestart(selectedForBulk());
    }, [selection, selectedForBulk, onBulkRestart]);

    const handleBulkExportYaml = useCallback(() => {
        if (!selection || !onBulkExportYaml) return;
        onBulkExportYaml(selectedForBulk());
    }, [selection, selectedForBulk, onBulkExportYaml]);

    const handleClearSelection = useCallback(() => {
        if (!selection) return;
        selection.deselectAll();
    }, [selection]);

    // Detect whether the current state has unsaved changes (vs active view or vs app defaults)
    const defaultSortConfig = initialSort || { key: null, direction: 'asc' };
    const isViewDirty = useMemo(() => {
        if (activeViewId) {
            // Compare against the loaded saved view
            const view = loadView(activeViewId);
            if (!view) return false;

            if ((view.query || '') !== searchInput) return true;
            if (JSON.stringify(view.namespace) !== JSON.stringify(currentNamespace)) return true;
            if (JSON.stringify(view.sortConfig || { key: null, direction: 'asc' }) !== JSON.stringify(sortConfig)) return true;

            const savedHidden = new Set(view.hiddenColumns || []);
            if (savedHidden.size !== hiddenColumns.size || [...savedHidden].some(k => !hiddenColumns.has(k))) return true;

            const savedOrder = view.columnOrder || [];
            if (JSON.stringify(savedOrder) !== JSON.stringify(columnOrder)) return true;

            const currentFilters: Record<string, any> = {};
            for (const [key, filter] of Object.entries(columnFilters)) {
                if (filter.type === 'select') {
                    currentFilters[key] = { ...filter, values: Array.from(filter.values || []) };
                } else {
                    currentFilters[key] = filter;
                }
            }
            if (JSON.stringify(view.columnFilters || {}) !== JSON.stringify(currentFilters)) return true;

            return false;
        }

        // No active view — compare against application defaults
        if (searchInput) return true;
        if (JSON.stringify(sortConfig) !== JSON.stringify(defaultSortConfig)) return true;
        if (columnOrder.length > 0) return true;
        if (Object.keys(columnFilters).length > 0) return true;

        // Hidden columns differ from column defaults
        if (hiddenColumns.size !== defaultHiddenColumns.size || [...defaultHiddenColumns].some(k => !hiddenColumns.has(k))) return true;

        return false;
    }, [activeViewId, loadView, searchInput, currentNamespace, sortConfig, defaultSortConfig, hiddenColumns, defaultHiddenColumns, columnOrder, columnFilters]);

    // Select-type columns whose funnel icon is worth showing. Only visible columns are
    // considered, and columns with predefined values (status, type, ...) are always usable,
    // so the common cases cost nothing; the rest scan the deferred data lazily.
    const prevSelectFilterColumnsRef = useRef<ReadonlySet<string>>(new Set());
    const selectFilterColumns = useMemo(() => {
        const result = new Set<string>();
        for (const col of visibleColumns) {
            if (columnFilterTypes[col.key] !== 'select') continue;
            if (viewMode) {
                // The backend says which select columns currently have a usable set of values.
                if ((viewSnap?.state?.filterable ?? viewSnap?.filterable)?.[col.key]) result.add(col.key);
                continue;
            }
            const predefined = resourceType ? PREDEFINED_COLUMN_VALUES[resourceType]?.[col.key] : undefined;
            if ((predefined && predefined.length > 0) || selectColumnHasFilterValues(col, deferredData, resourceType)) {
                result.add(col.key);
            }
        }
        // Keep the previous Set when nothing changed so the memoised header is not invalidated by data bursts.
        const prev = prevSelectFilterColumnsRef.current;
        if (prev.size === result.size && [...result].every(k => prev.has(k))) return prev;
        prevSelectFilterColumnsRef.current = result;
        return result;
    }, [visibleColumns, columnFilterTypes, deferredData, resourceType, viewMode, viewSnap?.filterable, viewSnap?.state?.filterable]);

    // Unique values and counts exist only for the open select dropdown, recomputed while it
    // is open as data changes. Nothing is scanned when no dropdown is open.
    const [facet, setFacet] = useState<Facet | null>(null);
    const [facetLoading, setFacetLoading] = useState(false);
    useEffect(() => {
        if (!source || !openColumnFilter || columnFilterTypes[openColumnFilter] !== 'select') {
            setFacet(null);
            setFacetLoading(false);
            return;
        }
        let cancelled = false;
        let timer: ReturnType<typeof setTimeout> | undefined;
        const load = () => {
            setFacetLoading(true);
            source.facet(openColumnFilter).then(f => {
                if (!cancelled) setFacet(f);
            }).catch(() => {
                if (!cancelled) setFacet(null);
            }).finally(() => {
                if (!cancelled) setFacetLoading(false);
            });
        };
        // Counts follow the data while the dropdown is open, at a gentle rate.
        timer = setTimeout(load, facet && facet.column === openColumnFilter ? FACET_REFRESH_MS : 0);
        return () => { cancelled = true; if (timer) clearTimeout(timer); };
        // facet is read only to pick the delay
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [source, openColumnFilter, columnFilterTypes, viewTotal, viewFiltered]);
    const openUniqueValues = useMemo(() => {
        if (viewMode) {
            if (!facet || facet.column !== openColumnFilter || facet.suppressed) return undefined;
            return { values: facet.values, counts: facet.counts };
        }
        if (!openColumnFilter || columnFilterTypes[openColumnFilter] !== 'select') return undefined;
        const col = columns.find((c: any) => c.key === openColumnFilter);
        return col ? computeColumnUniqueValues(col, deferredData, resourceType) : undefined;
    }, [openColumnFilter, columnFilterTypes, columns, deferredData, resourceType, viewMode, facet]);

    // Search, column filters and sort go to the backend view. Search arrives already debounced
    // (searchTerm), sort and filter toggles are pushed at once. The client drops no-op patches,
    // so re-pushing everything when the view (re)opens is cheap.
    const schemaKeys = useMemo(() => new Set(columnKeysIn(activeSchema)), [activeSchema]);
    const filtersWire = useMemo(() => toColumnFilterSpecs(columnFilters as any, schemaKeys), [columnFilters, schemaKeys]);
    // A filter on a column the view does not have (restored from a saved view made in the legacy
    // list) cannot be applied. Leaving it would show a filter chip over unfiltered rows, and a
    // select-all would then cover rows the user believes are filtered out, so it is removed.
    useEffect(() => {
        if (!source || schemaKeys.size === 0) return;
        const unknown = Object.keys(columnFilters).filter(k => !schemaKeys.has(k));
        if (unknown.length === 0) return;
        setColumnFilters((prev: any) => {
            const next = { ...prev };
            for (const k of unknown) delete next[k];
            return next;
        });
    }, [source, schemaKeys, columnFilters]);
    const filtersWireKey = JSON.stringify(filtersWire);
    const sortWireKey = sortConfig.key && schemaKeys.has(sortConfig.key) ? `${sortConfig.key}|${sortConfig.direction}` : '';
    const viewColumnKeys = useMemo(
        () => {
            const keys = visibleColumns.map((c: any) => c.key).filter((k: string) => schemaKeys.has(k));
            for (const k of viewPinnedColumns ?? []) if (schemaKeys.has(k) && !keys.includes(k)) keys.push(k);
            return keys;
        },
        [visibleColumns, schemaKeys, viewPinnedColumns]);
    const viewColumnKeysKey = viewColumnKeys.join(',');
    useEffect(() => {
        if (!source) return;
        const [key, dir] = sortWireKey ? sortWireKey.split('|') : ['', ''];
        source.update({
            sort: { key, dir: dir === 'desc' ? 'desc' : dir === 'asc' ? 'asc' : '' },
            columnFilters: JSON.parse(filtersWireKey),
        });
    }, [source, sortWireKey, filtersWireKey]);
    useEffect(() => {
        if (!source) return;
        source.update({ search: searchTerm });
    }, [source, searchTerm]);
    useEffect(() => {
        if (!source || viewColumnKeys.length === 0) return;
        source.update({ columns: viewColumnKeys });
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [source, viewColumnKeysKey]);

    // Consumers that asked for the filtered uids get them over the refs stream, lazily.
    useEffect(() => {
        if (!source || !onFilteredUidsChange) return;
        // A 'transitions' feed from the backend can replace this refetch later; for now the set is
        // re-read after the counts settle.
        const timer = setTimeout(() => {
            void source.selectAllRefs().then(refs => onFilteredUidsChange(new Set(refs.map(r => r.uid)))).catch(() => {});
        }, 300);
        return () => clearTimeout(timer);
    }, [source, onFilteredUidsChange, viewFiltered, viewTotal, viewEpoch]);

    const handleFilterButtonClick = useCallback((e: React.MouseEvent<HTMLButtonElement>, colKey: string) => {
        e.stopPropagation();
        const opening = openColumnFilter !== colKey;
        setOpenColumnFilter(opening ? colKey : null);
        setColumnFilterSearch('');
        if (opening) {
            const rect = e.currentTarget.getBoundingClientRect();
            const dropdownWidth = 224; // w-56 = 14rem = 224px
            const left = Math.max(8, Math.min(rect.left, window.innerWidth - dropdownWidth - 8));
            // Place below the button initially; useLayoutEffect will correct if it overflows
            setColumnFilterPos({ top: rect.bottom + 4, left, anchorTop: rect.top, anchorBottom: rect.bottom });
            if (columnFilterTypes[colKey] === 'regex') {
                setRegexDraft('');
            } else if (columnFilterTypes[colKey] === 'numeric') {
                setNumericDraft({ operator: '>', value: '', unit: 'h' });
            }
        }
    }, [openColumnFilter, columnFilterTypes]);

    const headerProps = useMemo(() => ({
        columns, visibleColumns, columnWidths, sortConfig, onSort: handleSort,
        columnFilters, columnFilterTypes, selectFilterColumns, openColumnFilter, columnFilterRef,
        onFilterButtonClick: handleFilterButtonClick,
        selectionState: selectionState as 'none' | 'some' | 'all',
        onHeaderCheckboxClick: handleHeaderCheckboxClick,
        hiddenColumns, defaultHiddenColumns,
        onToggleColumn: toggleColumn, onShowAllColumns: showAllColumns, onResetColumnDefaults: resetColumnDefaults,
        onColumnOrderChange: setColumnOrder,
        onResizeStart: handleResizeStart, onResizeDoubleClick: handleResizeDoubleClick,
    }), [
        columns, visibleColumns, columnWidths, sortConfig, handleSort,
        columnFilters, columnFilterTypes, selectFilterColumns, openColumnFilter,
        handleFilterButtonClick, selectionState, handleHeaderCheckboxClick,
        hiddenColumns, defaultHiddenColumns, toggleColumn, showAllColumns, resetColumnDefaults,
        handleResizeStart, handleResizeDoubleClick,
    ]);

    const fixedHeaderContent = useCallback(() => <ResourceListHeader {...headerProps} />, [headerProps]);

    // Rows read the live click handler through a ref so a parent that recreates its callback
    // each render does not invalidate the table context (and with it every visible row).
    const onRowClickRef = useRef(onRowClick);
    onRowClickRef.current = onRowClick;
    const hasRowClick = !!onRowClick;
    const rowClick = useMemo(
        () => (hasRowClick ? (item: any) => onRowClickRef.current?.(item) : null),
        [hasRowClick],
    );
    const rowAt = useMemo(() => (source ? (index: number) => source.rowAt(index) : null), [source]);
    // Virtuoso only re-renders rows when its context changes, so a window update (viewEpoch)
    // has to travel through it for rows to leave the skeleton state.
    const tableContext = useMemo(() => ({
        highlightedUid, selectable, selection, onRowClick: rowClick, rowAt, epoch: viewEpoch,
    }), [highlightedUid, selectable, selection, rowClick, rowAt, viewEpoch]);

    // Age cells format "now - creationTimestamp", so memoised rows need a heartbeat to keep
    // them current. It only runs while this list is actually on screen.
    const rootRef = useRef<HTMLDivElement>(null);
    const [clock, setClock] = useState(0);
    useEffect(() => {
        const id = setInterval(() => {
            const el = rootRef.current;
            if (document.hidden || !el || el.getClientRects().length === 0) return;
            setClock(c => c + 1);
        }, ROW_CLOCK_MS);
        return () => clearInterval(id);
    }, []);

    const itemContent = useCallback((index: number, item: any) => (
        <RowCells
            item={item}
            index={index}
            columns={visibleColumns}
            columnWidths={columnWidths}
            selected={selectable && selection ? !!selection.isSelected(item?.metadata?.uid) : false}
            onToggleSelect={handleRowCheckboxClick}
            onNamespaceChange={onNamespaceChange}
            clock={clock}
        />
    ), [visibleColumns, columnWidths, selectable, selection, handleRowCheckboxClick, onNamespaceChange, clock]);

    // Windowed rows: an index whose row has not arrived yet renders as a skeleton.
    const viewItemContent = useCallback((index: number) => {
        const item = source?.rowAt(index);
        if (!item) return <SkeletonCells columns={visibleColumns} columnWidths={columnWidths} />;
        return (
            <RowCells
                item={item}
                index={index}
                columns={visibleColumns}
                columnWidths={columnWidths}
                selected={selectable && selection ? !!selection.isSelected(item.metadata.uid) : false}
                onToggleSelect={handleRowCheckboxClick}
                onNamespaceChange={onNamespaceChange}
                clock={clock}
            />
        );
        // viewEpoch is a deliberate dependency: it re-renders the visible rows when the window changes.
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [source, viewEpoch, visibleColumns, columnWidths, selectable, selection, handleRowCheckboxClick, onNamespaceChange, clock]);
    const viewRowKey = useCallback((index: number) => source?.rowAt(index)?.metadata.uid ?? `pending-${index}`, [source, viewEpoch]); // eslint-disable-line react-hooks/exhaustive-deps
    const handleRangeChanged = useCallback((range: { startIndex: number; endIndex: number }) => {
        sourceRef.current?.setRange(range.startIndex, range.endIndex + 1);
    }, []);

    return (
        <div ref={rootRef} className="flex flex-col h-full bg-background relative">
            {/* Header - split into draggable and non-draggable regions as siblings */}
            <div className="h-14 border-b border-border flex items-center bg-surface shrink-0">
                {/* Left side - draggable titlebar region */}
                <div className="flex items-center gap-4 flex-1 px-4 titlebar-drag h-full">
                    <h1 className="text-lg font-semibold text-text shrink-0">{title}</h1>

                    {/* Search Bar */}
                    <div className="relative max-w-md w-full flex items-center gap-1 no-drag">
                        <div className="relative flex-1 rounded-md bg-background">
                            <MagnifyingGlassIcon className="h-4 w-4 text-gray-400 absolute left-3 top-1/2 transform -translate-y-1/2 z-30" />
                            <SearchHighlightOverlay query={searchInput} />
                            <input
                                autoCapitalize="off"
                                type="text"
                                placeholder={resourceType ? `Search... (name:"x" status:Running)` : `Search ${title}...`}
                                className={`relative z-10 w-full bg-transparent border border-border rounded-md pl-9 pr-4 py-1.5 text-sm caret-text focus:outline-none focus:border-primary transition-colors ${
                                    searchInput ? 'text-transparent' : 'text-text'
                                }`}
                                value={searchInput}
                                onChange={(e: any) => setSearchInput(e.target.value)}
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
                                                <span className="text-gray-300">Regex:</span> /pattern/ or field:/pattern/
                                                <div className="text-xs text-gray-500 ml-2">/^nginx-/ name:/end$/</div>
                                            </div>
                                            <div>
                                                <span className="text-gray-300">OR groups:</span> condition OR condition
                                                <div className="text-xs text-gray-500 ml-2">name:/^web-/ OR name:/^api-/</div>
                                            </div>
                                            <div className="border-t border-border pt-2 mt-2">
                                                <span className="text-gray-300">Available fields:</span>
                                                <div className="text-xs text-gray-500 mt-1 flex flex-wrap gap-1">
                                                    {getFieldsMetadata(resourceType, viewMode).map((f: any) => (
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
                            views={views as any}
                            activeViewId={activeViewId as any}
                            isDirty={isViewDirty}
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
                                onOpen={refreshNamespaces}
                            />
                        </div>
                    )}
                    <RefreshControls />
                </div>
            </div>

            {/* Active Filter Chips */}
            {resourceType && hasActiveColumnFilters && (
                <div className="px-4 py-1.5 border-b border-border bg-surface shrink-0 flex items-center gap-2 flex-wrap">
                    <FunnelIcon className="w-4 h-4 text-gray-500 shrink-0" />
                    {(Object.entries(columnFilters) as [string, ColumnFilter][]).map(([colKey, filter]) => {
                        const col = columns.find((c: any) => c.key === colKey);
                        return (
                            <span key={colKey} className="inline-flex items-center gap-1 px-2 py-0.5 text-xs bg-primary/10 text-primary border border-primary/15 rounded">
                                <span className="text-gray-400">{col?.label || colKey}:</span>
                                <span>
                                    {filter.type === 'regex'
                                        ? <span className="font-mono">{(filter.conditions || [filter.pattern]).map((p: any, i: number) => (
                                            <span key={i}>{i > 0 && <span className="text-gray-500 mx-0.5">{filter.logic === 'or' ? '|' : '&'}</span>}/{p}/</span>
                                        ))}</span>
                                        : filter.type === 'numeric'
                                            ? <span className="font-mono">{(filter.conditions || [{ operator: filter.operator, value: filter.value }]).map((c: any, i: number) => (
                                                <span key={i}>{i > 0 && <span className="text-gray-500 mx-0.5">{filter.logic === 'or' ? '|' : '&'}</span>}{c.operator} {c.value}</span>
                                            ))}</span>
                                            : `${filter.values?.size} selected`
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

            {/* Large Dataset Banner */}
            <LargeDatasetBanner
                isLoading={isLoading}
                progress={loadState ? null : loadingProgress}
                dataCount={viewMode ? 0 : data.length}
                threshold={largeDatasetThreshold}
            />

            {/* Table Content — loading feedback floats above rows and the bulk action bar. */}
            <div className="relative min-h-0 flex-1 overflow-hidden">
                {customBody ? customBody : (isLoading || viewPending || loadState?.phase === 'incomplete') && knownCount === 0 ? (
                    <div className="flex items-center justify-center h-full text-gray-500">
                        {loadState?.phase === 'incomplete' ? 'Resource list unavailable' : 'Loading…'}
                    </div>
                ) : shownCount === 0 && loadState && loadState.phase !== 'complete' ? (
                    <div className="flex items-center justify-center h-full text-gray-500">
                        No matching resources loaded yet. The resource list is not complete.
                    </div>
                ) : shownCount === 0 ? (
                    <div className="flex flex-col items-center justify-center h-full text-gray-500 gap-3">
                        {(() => {
                            const namespaceArray = namespaceSelection(currentNamespace);
                            const hasSearchFilter = Boolean(searchTerm.trim());
                            const hasNamespaceFilter = showNamespaceSelector && onNamespaceChange && !namespaceArray.includes('*');

                            return (
                                <>
                                    {hasSearchFilter && (
                                        <div className="flex flex-col items-center gap-3">
                                            <span>No resources match the current query</span>
                                            <button
                                                onClick={() => { setSearchInput(''); setSearchTerm(''); }}
                                                className="px-3 py-1.5 text-xs font-medium text-primary hover:text-white bg-primary/10 hover:bg-primary/20 rounded transition-colors"
                                            >
                                                Clear query
                                            </button>
                                        </div>
                                    )}
                                    {hasNamespaceFilter && (
                                        <div className="flex flex-col items-center gap-3">
                                            <span>{namespaceArray.length === 0 ? 'No namespace selected' : 'No resources found in selected namespaces'}</span>
                                            <button
                                                onClick={() => onNamespaceChange(multiSelectNamespaces ? ['*'] : '')}
                                                className="px-3 py-1.5 text-xs font-medium text-primary hover:text-white bg-primary/10 hover:bg-primary/20 rounded transition-colors"
                                            >
                                                Reset namespace filter
                                            </button>
                                        </div>
                                    )}
                                    {!hasSearchFilter && !hasNamespaceFilter && <span>No resources found</span>}
                                </>
                            );
                        })()}
                    </div>
                ) : viewMode ? (
                    <TableVirtuoso
                        style={VIEW_SCROLLER_STYLE}
                        totalCount={viewFiltered}
                        overscan={VIRTUOSO_OVERSCAN_PX}
                        computeItemKey={viewRowKey}
                        components={virtuosoComponents}
                        context={tableContext}
                        fixedHeaderContent={fixedHeaderContent}
                        itemContent={viewItemContent}
                        rangeChanged={handleRangeChanged}
                    />
                ) : (
                    <TableVirtuoso
                        style={{ height: '100%' }}
                        data={sortedData}
                        // A few rows beyond the viewport: enough that fast scrolling in WKWebView does not
                        // flash blank rows, small enough that memoised rows keep the extra DOM cheap.
                        overscan={VIRTUOSO_OVERSCAN_PX}
                        computeItemKey={computeRowKey}
                        components={virtuosoComponents}
                        context={tableContext}
                        fixedHeaderContent={fixedHeaderContent}
                        itemContent={itemContent}
                    />
                )}
                <ResourceLoadingStatus state={loadState} displayed={shownCount} />
            </div>

            {/* Bulk Action Bar - Bottom */}
            {selectable && selection && (
                <BulkActionBar
                    selectedCount={selection.selectedCount}
                    onClearSelection={handleClearSelection}
                    onDelete={onBulkDelete ? handleBulkDelete : undefined}
                    onRestart={onBulkRestart ? handleBulkRestart : undefined}
                    onExportYaml={onBulkExportYaml ? handleBulkExportYaml : undefined}
                    customActions={bulkCustomActions}
                    resourceType={selectionLabel || bulkResourceLabel(resourceType, title)}
                    position="bottom"
                />
            )}

            {/* Column filter dropdown — rendered via portal to escape overflow:hidden ancestors */}
            {openColumnFilter && columnFilterPos && (() => {
                const col = visibleColumns.find((c: any) => c.key === openColumnFilter);
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
                                {(columnFilters[col.key]?.conditions?.length ?? 0) > 0 && (
                                    <div className="mb-2 space-y-1">
                                        {(columnFilters[col.key].conditions || []).map((pattern: any, idx: number) => (
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
                                    autoCorrect="off"
                                    autoCapitalize="off"
                                    spellCheck={false}
                                    type="text"
                                    value={regexDraft}
                                    onChange={(e: any) => setRegexDraft(e.target.value)}
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
                                {(columnFilters[col.key]?.conditions?.length ?? 0) > 0 && (
                                    <div className="mb-2 space-y-1">
                                        {(columnFilters[col.key].conditions || []).map((cond: any, idx: number) => (
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
                                        onChange={(e: any) => setNumericDraft(d => ({ ...d, operator: e.target.value }))}
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
                                        onChange={(e: any) => setNumericDraft(d => ({ ...d, value: e.target.value }))}
                                        placeholder="0"
                                        className="flex-1 min-w-0 px-2 py-1.5 bg-background border border-border rounded text-xs font-mono focus:outline-none focus:border-primary"
                                        autoFocus
                                        autoComplete="off"
                                        onKeyDown={(e) => {
                                            if (e.key === 'Enter') {
                                                let filterValue = numericDraft.value;
                                                if (col.key === 'age' && filterValue !== '') {
                                                    const multipliers: Record<string, number> = { s: 1/3600, m: 1/60, h: 1, d: 24 };
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
                                            onChange={(e: any) => setNumericDraft(d => ({ ...d, unit: e.target.value }))}
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
                                        autoCorrect="off"
                                        autoCapitalize="off"
                                        spellCheck={false}
                                        type="text"
                                        value={columnFilterSearch}
                                        onChange={(e: any) => setColumnFilterSearch(e.target.value)}
                                        placeholder="Search values..."
                                        className="w-full px-2 py-1 bg-background border border-border rounded text-xs focus:outline-none focus:border-primary"
                                        autoFocus
                                        autoComplete="off"
                                    />
                                </div>
                                <div className="max-h-48 overflow-auto py-1">
                                    {viewMode && facetLoading && !openUniqueValues && (
                                        <div className="px-3 py-2 text-xs text-gray-500 flex items-center gap-2">
                                            <span className="inline-block h-3 w-3 rounded-full border-2 border-gray-500 border-t-transparent animate-spin" />
                                            Loading values...
                                        </div>
                                    )}
                                    {openUniqueValues?.values
                                        ?.filter((v: string) => !columnFilterSearch || v.toLowerCase().includes(columnFilterSearch.toLowerCase()))
                                        .map((value: string) => {
                                            const isChecked = columnFilters[col.key]?.values?.has(value);
                                            const count = openUniqueValues?.counts?.[value] ?? 0;
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
