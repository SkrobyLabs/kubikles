import { useState, useCallback, useEffect } from 'react';

export const STORAGE_KEY = 'kubikles_savedviews';

/**
 * Sort configuration for a saved view
 */
interface SortConfig {
    key: string;
    direction: 'asc' | 'desc';
}

/**
 * Column filter configuration
 */
interface ColumnFilters {
    [columnKey: string]: string | string[] | boolean | number;
}

/**
 * Saved view configuration
 */
export interface SavedView {
    id: string;
    name: string;
    resourceType: string;
    query: string;
    namespace: string | string[];
    hiddenColumns: string[];
    sortConfig: SortConfig | null;
    columnFilters: ColumnFilters;
    columnOrder?: string[];
    createdAt: number;
    isDefault?: boolean;
}

/**
 * Configuration for creating/updating a view
 */
export interface ViewConfig {
    query?: string;
    namespace: string | string[];
    hiddenColumns?: string[];
    sortConfig?: SortConfig | null;
    columnFilters?: ColumnFilters;
    columnOrder?: string[];
    resourceType?: string;
}

/**
 * Return type for useSavedViews hook
 */
export interface UseSavedViewsReturn {
    views: SavedView[];
    allViews: SavedView[];
    saveView: (name: string, config: ViewConfig) => SavedView;
    loadView: (viewId: string) => SavedView | null;
    updateView: (viewId: string, updates: Partial<SavedView>) => boolean;
    renameView: (viewId: string, newName: string) => void;
    deleteView: (viewId: string) => void;
    duplicateView: (viewId: string) => SavedView | null;
    setDefaultView: (viewId: string) => void;
    getDefaultView: () => SavedView | null;
}

/**
 * Generate a unique ID for a new view.
 * @exported for testing
 */
export function generateId(): string {
    return `view_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`;
}

/**
 * Load all saved views from localStorage.
 * @exported for testing
 */
export function loadViews(): SavedView[] {
    try {
        const saved = localStorage.getItem(STORAGE_KEY);
        return saved ? JSON.parse(saved) : [];
    } catch (e: any) {
        console.error('Failed to load saved views:', e);
        return [];
    }
}

/**
 * Save all views to localStorage.
 * @exported for testing
 */
export function persistViews(views: SavedView[]): void {
    try {
        localStorage.setItem(STORAGE_KEY, JSON.stringify(views));
    } catch (e: any) {
        console.error('Failed to save views:', e);
    }
}

/**
 * Hook for managing saved views (filter presets).
 *
 * @example
 * const { views, saveView, loadView, deleteView } = useSavedViews('pods');
 *
 * // Save current view
 * saveView('My View', { query: 'status:Running', namespace: ['default'] });
 *
 * // Load a saved view
 * const config = loadView(viewId);
 */
export function useSavedViews(resourceType: string | null = null): UseSavedViewsReturn {
    const [allViews, setAllViews] = useState<SavedView[]>(() => loadViews());

    // Filter views by resource type if specified
    const views = resourceType
        ? allViews.filter((v: any) => v.resourceType === resourceType)
        : allViews;

    // Persist changes to localStorage
    useEffect(() => {
        persistViews(allViews);
    }, [allViews]);

    /**
     * Save a new view.
     */
    const saveView = useCallback((name: string, config: ViewConfig): SavedView => {
        const newView: SavedView = {
            id: generateId(),
            name,
            resourceType: config.resourceType || resourceType || '',
            query: config.query || '',
            namespace: config.namespace,
            hiddenColumns: config.hiddenColumns || [],
            sortConfig: config.sortConfig || null,
            columnFilters: config.columnFilters || {},
            columnOrder: config.columnOrder,
            createdAt: Date.now(),
        };

        setAllViews(prev => [...prev, newView]);
        return newView;
    }, [resourceType]);

    /**
     * Get a view by ID.
     */
    const loadView = useCallback((viewId: string): SavedView | null => {
        return allViews.find((v: any) => v.id === viewId) || null;
    }, [allViews]);

    /**
     * Update an existing view.
     */
    const updateView = useCallback((viewId: string, updates: Partial<SavedView>): boolean => {
        setAllViews(prev => {
            const index = prev.findIndex((v: any) => v.id === viewId);
            if (index === -1) return prev;

            const updated = [...prev];
            updated[index] = { ...updated[index], ...updates };
            return updated;
        });
        return true;
    }, []);

    /**
     * Rename a view.
     */
    const renameView = useCallback((viewId: string, newName: string): void => {
        updateView(viewId, { name: newName });
    }, [updateView]);

    /**
     * Delete a view.
     */
    const deleteView = useCallback((viewId: string): void => {
        setAllViews(prev => prev.filter((v: any) => v.id !== viewId));
    }, []);

    /**
     * Duplicate a view.
     */
    const duplicateView = useCallback((viewId: string): SavedView | null => {
        const source = allViews.find((v: any) => v.id === viewId);
        if (!source) return null;

        const newView: SavedView = {
            ...source,
            id: generateId(),
            name: `${source.name} (copy)`,
            createdAt: Date.now(),
            isDefault: false, // Duplicates are never default
        };

        setAllViews(prev => [...prev, newView]);
        return newView;
    }, [allViews]);

    /**
     * Set or unset a view as the default for its resource type.
     * Only one view can be default per resource type.
     */
    const setDefaultView = useCallback((viewId: string): void => {
        setAllViews(prev => {
            const view = prev.find((v: any) => v.id === viewId);
            if (!view) return prev;

            const isCurrentlyDefault = view.isDefault;
            const viewResourceType = view.resourceType;

            return prev.map((v: any) => {
                // If this is the target view, toggle its default status
                if (v.id === viewId) {
                    return { ...v, isDefault: !isCurrentlyDefault };
                }
                // If setting a new default, unset any other defaults for this resource type
                if (!isCurrentlyDefault && v.resourceType === viewResourceType && v.isDefault) {
                    return { ...v, isDefault: false };
                }
                return v;
            });
        });
    }, []);

    /**
     * Get the default view for the current resource type.
     */
    const getDefaultView = useCallback((): SavedView | null => {
        return views.find((v: any) => v.isDefault) || null;
    }, [views]);

    return {
        views,
        allViews,
        saveView,
        loadView,
        updateView,
        renameView,
        deleteView,
        duplicateView,
        setDefaultView,
        getDefaultView,
    };
}

export default useSavedViews;
