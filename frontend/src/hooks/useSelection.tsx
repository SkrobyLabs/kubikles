import { useState, useCallback, useMemo, useRef } from 'react';

/**
 * Hook for managing selection state in list views
 * Supports single selection, multi-selection, and shift+click range selection
 *
 * @returns {Object} Selection state and methods
 */
export function useSelection() {
    const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set<any>());
    const lastSelectedIndexRef = useRef<number | null>(null);

    // Toggle selection of a single item
    const toggleItem = useCallback((uid: string, index: number, items: any[], shiftKey = false) => {
        setSelectedIds(prev => {
            const next = new Set(prev);

            // Shift+click for range selection
            if (shiftKey && lastSelectedIndexRef.current !== null && items) {
                const start = Math.min(lastSelectedIndexRef.current, index);
                const end = Math.max(lastSelectedIndexRef.current, index);

                // Select all items in range
                for (let i = start; i <= end; i++) {
                    const item = items[i];
                    if (item?.metadata?.uid) {
                        next.add(item.metadata.uid);
                    }
                }
            } else {
                // Regular toggle
                if (next.has(uid)) {
                    next.delete(uid);
                } else {
                    next.add(uid);
                }
            }

            return next;
        });

        // Always update last selected index for shift+click
        lastSelectedIndexRef.current = index;
    }, []);

    // Select a single item (without toggle)
    const selectItem = useCallback((uid: string) => {
        setSelectedIds(prev => {
            const next = new Set(prev);
            next.add(uid);
            return next;
        });
    }, []);

    // Deselect a single item
    const deselectItem = useCallback((uid: string) => {
        setSelectedIds(prev => {
            const next = new Set(prev);
            next.delete(uid);
            return next;
        });
    }, []);

    // Select all items
    const selectAll = useCallback((items: any[]) => {
        const uids = items
            .map((item: any) => item?.metadata?.uid)
            .filter(Boolean);
        setSelectedIds(new Set(uids));
    }, []);

    // Deselect all items
    const deselectAll = useCallback(() => {
        setSelectedIds(new Set<any>());
        lastSelectedIndexRef.current = null;
    }, []);

    // Toggle between select all and deselect all
    // If any items are selected, deselect all; otherwise select all
    const toggleAll = useCallback((items: any[]) => {
        setSelectedIds(prev => {
            if (prev.size > 0) {
                return new Set<string>();
            }
            const uids = items
                .map((item: any) => item?.metadata?.uid)
                .filter(Boolean);
            return new Set(uids);
        });
    }, []);

    // Check if an item is selected
    const isSelected = useCallback((uid: string) => {
        return selectedIds.has(uid);
    }, [selectedIds]);

    // Get selected count
    const selectedCount = selectedIds.size;

    // Get selection state relative to a list of items
    // Returns 'none', 'some', or 'all'
    const getSelectionState = useCallback((items: any[]) => {
        if (!items || items.length === 0) return 'none';

        const itemUids = items
            .map((item: any) => item?.metadata?.uid)
            .filter(Boolean);

        if (itemUids.length === 0) return 'none';

        const selectedInList = itemUids.filter((uid: string) => selectedIds.has(uid)).length;

        if (selectedInList === 0) return 'none';
        if (selectedInList === itemUids.length) return 'all';
        return 'some';
    }, [selectedIds]);

    // Get array of selected items from a list
    const getSelectedItems = useCallback((items: any[]) => {
        return items.filter((item: any) => selectedIds.has(item?.metadata?.uid));
    }, [selectedIds]);

    // Clear selection for items that no longer exist in the list
    const pruneSelection = useCallback((items: any[]) => {
        const validUids = new Set(
            items
                .map((item: any) => item?.metadata?.uid)
                .filter(Boolean)
        );

        setSelectedIds(prev => {
            const next = new Set<string>();
            for (const uid of prev) {
                if (validUids.has(uid)) {
                    next.add(uid);
                }
            }
            return next;
        });
    }, []);

    return useMemo(() => ({
        selectedIds,
        selectedCount,
        toggleItem,
        selectItem,
        deselectItem,
        selectAll,
        deselectAll,
        toggleAll,
        isSelected,
        getSelectionState,
        getSelectedItems,
        pruneSelection,
    }), [
        selectedIds,
        selectedCount,
        toggleItem,
        selectItem,
        deselectItem,
        selectAll,
        deselectAll,
        toggleAll,
        isSelected,
        getSelectionState,
        getSelectedItems,
        pruneSelection,
    ]);
}

export default useSelection;
