import { useState, useEffect, useRef, useMemo, useCallback } from 'react';
import { stripAnsiCodes } from './logUtils';

/**
 * Hook for managing log search and filter functionality.
 */
export function useLogSearch({
    logs,
    getConfig,
    getSafeConfig
}: { logs: any; getConfig: any; getSafeConfig: any }) {
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

    // Calculate which lines match and filtered view with context
    const { displayLogs, matchCount, matchIndices } = useMemo(() => {
        if (!logs || logs.length === 0) {
            return { displayLogs: [], matchCount: 0, matchIndices: new Set<any>() };
        }

        // If no search term, show all logs
        if (!searchTerm || !searchRegex) {
            return { displayLogs: logs.map((entry: any, i: number) => ({ ...entry, originalIndex: i })), matchCount: 0, matchIndices: new Set<any>() };
        }

        // Find all matching line indices (search in stripped content without ANSI codes)
        const matches = new Set<any>();
        logs.forEach((entry: any, index: number) => {
            searchRegex.lastIndex = 0; // Reset regex state
            const strippedContent = stripAnsiCodes(entry.content);
            if (searchRegex.test(strippedContent)) {
                matches.add(index);
            }
        });

        // If not filtering, return all logs with match info
        if (!filterOnly) {
            return {
                displayLogs: logs.map((entry: any, index: number) => ({ ...entry, originalIndex: index, isMatch: matches.has(index) })),
                matchCount: matches.size,
                matchIndices: matches
            };
        }

        // Filter mode: include matching lines with context
        const includedIndices = new Set<any>();
        matches.forEach((matchIndex: any) => {
            // Add context lines before
            for (let i = Math.max(0, (matchIndex as number) - contextLinesBefore); i < (matchIndex as number); i++) {
                includedIndices.add(i);
            }
            // Add matching line
            includedIndices.add(matchIndex);
            // Add context lines after
            for (let i = (matchIndex as number) + 1; i <= Math.min(logs.length - 1, (matchIndex as number) + contextLinesAfter); i++) {
                includedIndices.add(i);
            }
        });

        // Build display with skipped line indicators
        const result: any[] = [];
        const sortedIndices = Array.from(includedIndices).sort((a: any, b: any) => a - b);
        let lastIndex = -1;

        sortedIndices.forEach((index: any) => {
            // Check if we need to show skipped lines indicator
            if (lastIndex !== -1 && (index as number) > lastIndex + 1) {
                const skipped = (index as number) - lastIndex - 1;
                result.push({
                    isSkipIndicator: true,
                    skippedCount: skipped,
                    key: `skip-${lastIndex}-${index}`
                });
            }
            result.push({
                ...logs[index as number],
                originalIndex: index,
                isMatch: matches.has(index)
            });
            lastIndex = index as number;
        });

        return {
            displayLogs: result,
            matchCount: matches.size,
            matchIndices: matches
        };
    }, [logs, searchTerm, searchRegex, filterOnly, contextLinesBefore, contextLinesAfter]);

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
        displayLogs,
        matchCount,
        matchIndices,
        handleSearchKeyDown,
        openSearch,
        closeSearch,
        clearSearchInput
    };
}
