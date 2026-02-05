import { useState, useEffect, useRef, useMemo, useCallback } from 'react';
import { stripAnsiCodes } from './logUtils';

/**
 * Hook for managing log search and filter functionality.
 */
export function useLogSearch({
    logs,
    getConfig,
    getSafeConfig
}) {
    const [showSearch, setShowSearch] = useState(false);
    const [searchTerm, setSearchTerm] = useState(''); // The actual search term used for filtering
    const [searchInput, setSearchInput] = useState(''); // The input field value
    const [isRegex, setIsRegex] = useState(() => getSafeConfig('logs.search.useRegex', false, v => typeof v === 'boolean'));
    const [filterOnly, setFilterOnly] = useState(() => getSafeConfig('logs.search.filterOnly', false, v => typeof v === 'boolean'));
    const [searchOnEnter, setSearchOnEnter] = useState(() => getSafeConfig('logs.search.searchOnEnter', true, v => typeof v === 'boolean'));
    const [contextLinesBefore, setContextLinesBefore] = useState(() => getSafeConfig('logs.search.contextLinesBefore', 1, v => typeof v === 'number' && v >= 0));
    const [contextLinesAfter, setContextLinesAfter] = useState(() => getSafeConfig('logs.search.contextLinesAfter', 5, v => typeof v === 'number' && v >= 0));
    const [regexError, setRegexError] = useState('');

    const searchInputRef = useRef(null);
    const searchDebounceRef = useRef(null);

    // Create search regex with validation
    const { searchRegex, searchRegexError } = useMemo(() => {
        if (!searchTerm) {
            return { searchRegex: null, searchRegexError: '' };
        }
        try {
            const pattern = isRegex ? searchTerm : searchTerm.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
            const regex = new RegExp(pattern, 'gi');
            return { searchRegex: regex, searchRegexError: '' };
        } catch (e) {
            return { searchRegex: null, searchRegexError: e.message };
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

        const debounceMs = getSafeConfig('logs.search.debounceMs', 200, v => typeof v === 'number' && v >= 0 && v <= 2000);
        searchDebounceRef.current = setTimeout(() => {
            setSearchTerm(searchInput);
        }, debounceMs);

        return () => {
            if (searchDebounceRef.current) {
                clearTimeout(searchDebounceRef.current);
            }
        };
    }, [searchInput, searchOnEnter, getSafeConfig]);

    // Handle search input Enter key
    const handleSearchKeyDown = useCallback((e) => {
        if (e.key === 'Enter' && searchOnEnter) {
            setSearchTerm(searchInput);
        }
    }, [searchOnEnter, searchInput]);

    // Open search
    const openSearch = useCallback(() => {
        setShowSearch(true);
        setTimeout(() => searchInputRef.current?.focus(), 0);
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
            return { displayLogs: [], matchCount: 0, matchIndices: new Set() };
        }

        // If no search term, show all logs
        if (!searchTerm || !searchRegex) {
            return { displayLogs: logs.map((entry, i) => ({ ...entry, originalIndex: i })), matchCount: 0, matchIndices: new Set() };
        }

        // Find all matching line indices (search in stripped content without ANSI codes)
        const matches = new Set();
        logs.forEach((entry, index) => {
            searchRegex.lastIndex = 0; // Reset regex state
            const strippedContent = stripAnsiCodes(entry.content);
            if (searchRegex.test(strippedContent)) {
                matches.add(index);
            }
        });

        // If not filtering, return all logs with match info
        if (!filterOnly) {
            return {
                displayLogs: logs.map((entry, i) => ({ ...entry, originalIndex: i, isMatch: matches.has(i) })),
                matchCount: matches.size,
                matchIndices: matches
            };
        }

        // Filter mode: include matching lines with context
        const includedIndices = new Set();
        matches.forEach(matchIndex => {
            // Add context lines before
            for (let i = Math.max(0, matchIndex - contextLinesBefore); i < matchIndex; i++) {
                includedIndices.add(i);
            }
            // Add matching line
            includedIndices.add(matchIndex);
            // Add context lines after
            for (let i = matchIndex + 1; i <= Math.min(logs.length - 1, matchIndex + contextLinesAfter); i++) {
                includedIndices.add(i);
            }
        });

        // Build display with skipped line indicators
        const result = [];
        const sortedIndices = Array.from(includedIndices).sort((a, b) => a - b);
        let lastIndex = -1;

        sortedIndices.forEach(index => {
            // Check if we need to show skipped lines indicator
            if (lastIndex !== -1 && index > lastIndex + 1) {
                const skipped = index - lastIndex - 1;
                result.push({
                    isSkipIndicator: true,
                    skippedCount: skipped,
                    key: `skip-${lastIndex}-${index}`
                });
            }
            result.push({
                ...logs[index],
                originalIndex: index,
                isMatch: matches.has(index)
            });
            lastIndex = index;
        });

        return {
            displayLogs: result,
            matchCount: matches.size,
            matchIndices: matches
        };
    }, [logs, searchTerm, searchRegex, filterOnly, contextLinesBefore, contextLinesAfter]);

    // Keyboard shortcuts
    useEffect(() => {
        const handleKeyDown = (e) => {
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
