import { useState, useEffect, useRef, useMemo, useCallback } from 'react';
import { stripAnsiCodes } from './logUtils';

/**
 * Hook for managing log search and filter functionality.
 */
export interface DisplayRow {
    /** The underlying entry. Identity is stable, so per-entry caches and React.memo keep working. */
    entry?: any;
    /** Stable line id (firstId + position); survives head trimming. */
    originalIndex?: number;
    isMatch?: boolean;
    isSkipIndicator?: boolean;
    skippedCount?: number;
    key?: string;
}

const EMPTY_SET = new Set<number>();
const NO_ROWS: DisplayRow[] = [];

interface ScanState {
    logs: any;
    firstId: number;
    regexKey: string;
    scanned: number;
    matches: Set<number>;
    matchList: number[];
}

// Entries only contain escape codes when the source coloured them, so skip the regex for the common case.
const searchableText = (content: string) => (content.indexOf('\x1b') === -1 ? content : stripAnsiCodes(content));

/**
 * Hook for managing log search and filter functionality.
 *
 * `logs` may be a buffer that is appended to in place: pass `logCount` (and `firstId`,
 * the id of position 0) so changes are noticed. Matching is incremental over appended
 * lines and restarts only when the term/options change or the head was trimmed/prepended.
 */
export function useLogSearch({
    logs,
    logCount,
    firstId = 0,
    getConfig,
    getSafeConfig
}: { logs: any; logCount?: number; firstId?: number; getConfig: any; getSafeConfig: any }) {
    const [showSearch, setShowSearch] = useState(false);
    const [searchTerm, setSearchTerm] = useState(''); // The actual search term used for filtering
    const [searchInput, setSearchInput] = useState(''); // The input field value
    const [isRegex, setIsRegex] = useState(() => getSafeConfig('logs.search.useRegex', false, (v: any) => typeof v === 'boolean'));
    const [filterOnly, setFilterOnly] = useState(() => getSafeConfig('logs.search.filterOnly', false, (v: any) => typeof v === 'boolean'));
    const [searchOnEnter, setSearchOnEnter] = useState(() => getSafeConfig('logs.search.searchOnEnter', true, (v: any) => typeof v === 'boolean'));
    const [contextLinesBefore, setContextLinesBefore] = useState(() => getSafeConfig('logs.search.contextLinesBefore', 1, (v: any) => typeof v === 'number' && v >= 0));
    const [contextLinesAfter, setContextLinesAfter] = useState(() => getSafeConfig('logs.search.contextLinesAfter', 5, (v: any) => typeof v === 'number' && v >= 0));
    const [regexError, setRegexError] = useState('');

    const searchInputRef = useRef<any>(null);
    const searchDebounceRef = useRef<any>(null);

    // Create search regex with validation
    const { searchRegex, searchRegexError } = useMemo(() => {
        if (!searchTerm) {
            return { searchRegex: null, searchRegexError: '' };
        }
        try {
            const pattern = isRegex ? searchTerm : searchTerm.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
            const regex = new RegExp(pattern, 'gi');
            return { searchRegex: regex, searchRegexError: '' };
        } catch (e: any) {
            return { searchRegex: null, searchRegexError: (e as any).message };
        }
    }, [searchTerm, isRegex]);

    // Update error state when regex error changes
    useEffect(() => {
        setRegexError(searchRegexError);
    }, [searchRegexError]);

    // Debounced search when typing (only in as-you-type mode)
    useEffect(() => {
        if (searchOnEnter) return;

        if (searchDebounceRef.current) {
            clearTimeout(searchDebounceRef.current);
        }

        const debounceMs = getSafeConfig('logs.search.debounceMs', 200, (v: any) => typeof v === 'number' && v >= 0 && v <= 2000);
        (searchDebounceRef as any).current = setTimeout(() => {
            setSearchTerm(searchInput);
        }, debounceMs);

        return () => {
            if (searchDebounceRef.current) {
                clearTimeout(searchDebounceRef.current);
            }
        };
    }, [searchInput, searchOnEnter, getSafeConfig]);

    // Handle search input Enter key
    const handleSearchKeyDown = useCallback((e: any) => {
        if (e.key === 'Enter' && searchOnEnter) {
            setSearchTerm(searchInput);
        }
    }, [searchOnEnter, searchInput]);

    // Open search
    const openSearch = useCallback(() => {
        setShowSearch(true);
        setTimeout(() => (searchInputRef as any).current?.focus(), 0);
    }, []);

    // Close search
    const closeSearch = useCallback(() => {
        setShowSearch(false);
        setSearchTerm('');
        setSearchInput('');
        setRegexError('');
    }, []);

    // Clear search input
    const clearSearchInput = useCallback(() => {
        setSearchInput('');
        setSearchTerm('');
    }, []);

    const scanRef = useRef<ScanState | null>(null);
    const count = logCount ?? (logs ? logs.length : 0);

    // Calculate which lines match and the filtered view with context.
    const model = useMemo(() => {
        if (!logs || count === 0) {
            return { mode: 'empty' as const, rows: NO_ROWS, matchCount: 0, matchIndices: EMPTY_SET };
        }

        // No (valid) search: show the entries themselves, nothing derived.
        if (!searchTerm || !searchRegex) {
            return { mode: 'plain' as const, rows: NO_ROWS, matchCount: 0, matchIndices: EMPTY_SET };
        }

        const regexKey = `${searchRegex.source}/${searchRegex.flags}`;
        let state = scanRef.current;
        if (!state || state.logs !== logs || state.firstId !== firstId || state.regexKey !== regexKey || state.scanned > count) {
            state = { logs, firstId, regexKey, scanned: 0, matches: new Set<number>(), matchList: [] };
            scanRef.current = state;
        }
        // Search in stripped content (no ANSI codes); only lines appended since the last pass.
        for (let index = state.scanned; index < count; index++) {
            searchRegex.lastIndex = 0; // Reset regex state
            if (searchRegex.test(searchableText(logs[index].content))) {
                state.matches.add(index);
                state.matchList.push(index);
            }
        }
        state.scanned = count;
        const matches = state.matches;

        if (!filterOnly) {
            return { mode: 'marked' as const, rows: NO_ROWS, matchCount: matches.size, matchIndices: matches };
        }

        // Filter mode: include matching lines with context
        const rows: DisplayRow[] = [];
        let lastIndex = -1; // last position emitted
        let nextFree = 0; // first position not yet covered by an earlier window
        for (const matchIndex of state.matchList) {
            const from = Math.max(nextFree, matchIndex - contextLinesBefore);
            const to = Math.min(count - 1, matchIndex + contextLinesAfter);
            if (to < from) continue;
            if (lastIndex !== -1 && from > lastIndex + 1) {
                rows.push({
                    isSkipIndicator: true,
                    skippedCount: from - lastIndex - 1,
                    key: `skip-${lastIndex}-${from}`
                });
            }
            for (let i = from; i <= to; i++) {
                rows.push({ entry: logs[i], originalIndex: firstId + i, isMatch: matches.has(i) });
            }
            lastIndex = to;
            nextFree = to + 1;
        }
        return { mode: 'filtered' as const, rows, matchCount: matches.size, matchIndices: matches };
    // logs is mutated in place by appends, so count is part of the identity.
    }, [logs, count, firstId, searchTerm, searchRegex, filterOnly, contextLinesBefore, contextLinesAfter]);

    const displayCount = model.mode === 'filtered' ? model.rows.length : model.mode === 'empty' ? 0 : count;

    const getRow = useCallback((index: number): DisplayRow | null => {
        if (index < 0) return null;
        if (model.mode === 'filtered') return model.rows[index] ?? null;
        if (model.mode === 'empty' || index >= count) return null;
        const entry = logs[index];
        if (model.mode === 'marked') return { entry, originalIndex: firstId + index, isMatch: model.matchIndices.has(index) };
        return { entry, originalIndex: firstId + index };
    }, [model, logs, count, firstId]);

    // Legacy shape (entry fields spread onto each row). Built on demand: the viewer uses getRow.
    const displayLogsRef = useRef<{ key: any; value: any[] } | null>(null);
    const materializeDisplayLogs = () => {
        if (displayLogsRef.current && displayLogsRef.current.key === getRow) return displayLogsRef.current.value;
        const value: any[] = [];
        for (let i = 0; i < displayCount; i++) {
            const row = getRow(i) as DisplayRow;
            value.push(row.isSkipIndicator ? row : { ...row.entry, originalIndex: row.originalIndex, ...(row.isMatch === undefined ? {} : { isMatch: row.isMatch }) });
        }
        displayLogsRef.current = { key: getRow, value };
        return value;
    };
    const { matchCount, matchIndices } = model;

    // Keyboard shortcuts
    useEffect(() => {
        const handleKeyDown = (e: any) => {
            // Cmd+F / Ctrl+F to open search
            if ((e.metaKey || e.ctrlKey) && e.key === 'f') {
                e.preventDefault();
                openSearch();
            }
            // Escape to close search
            if (e.key === 'Escape' && showSearch) {
                closeSearch();
            }
        };

        window.addEventListener('keydown', handleKeyDown);
        return () => window.removeEventListener('keydown', handleKeyDown);
    }, [showSearch, openSearch, closeSearch]);

    return {
        showSearch,
        setShowSearch,
        searchTerm,
        searchInput,
        setSearchInput,
        isRegex,
        setIsRegex,
        filterOnly,
        setFilterOnly,
        searchOnEnter,
        setSearchOnEnter,
        contextLinesBefore,
        setContextLinesBefore,
        contextLinesAfter,
        setContextLinesAfter,
        regexError,
        searchInputRef,
        searchRegex,
        get displayLogs() { return materializeDisplayLogs(); },
        displayCount,
        displayIsDerived: model.mode === 'filtered',
        getRow,
        matchCount,
        matchIndices,
        handleSearchKeyDown,
        openSearch,
        closeSearch,
        clearSearchInput
    };
}
