import { useState, useCallback, useEffect } from 'react';

export const STORAGE_KEY = 'kubikles_savedviews';

/**
 * Generate a unique ID for a new view.
 * @exported for testing
 */
export function generateId() {
    return `view_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`;
}

/**
 * Load all saved views from localStorage.
 * @exported for testing
 */
export function loadViews() {
    try {
        const saved = localStorage.getItem(STORAGE_KEY);
        return saved ? JSON.parse(saved) : [];
    } catch (e) {
        console.error('Failed to load saved views:', e);
        return [];
    }
}

/**
 * Save all views to localStorage.
 * @exported for testing
 */
export function persistViews(views) {
    try {
        localStorage.setItem(STORAGE_KEY, JSON.stringify(views));
    } catch (e) {
        console.error('Failed to save views:', e);
    }
}

/**
 * Hook for managing saved views (filter presets).
 *
 * @param {string} resourceType - Optional filter to only show views for this resource type
 * @returns {object} View management functions and state
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
export function useSavedViews(resourceType = null) {
    const [allViews, setAllViews] = useState(() => loadViews());

    // Filter views by resource type if specified
    const views = resourceType
        ? allViews.filter(v => v.resourceType === resourceType)
        : allViews;

    // Persist changes to localStorage
    useEffect(() => {
        persistViews(allViews);
    }, [allViews]);

    /**
     * Save a new view.
     *
     * @param {string} name - Display name for the view
     * @param {object} config - View configuration
     * @param {string} config.query - Search query string
     * @param {string|string[]} config.namespace - Namespace selection
     * @param {string[]} [config.hiddenColumns] - Hidden column keys
     * @param {object} [config.sortConfig] - Sort configuration
     * @param {string} [config.resourceType] - Resource type (required if not provided via hook)
     * @returns {object} The saved view
     */
    const saveView = useCallback((name, config) => {
        const newView = {
            id: generateId(),
            name,
            resourceType: config.resourceType || resourceType,
            query: config.query || '',
            namespace: config.namespace,
            hiddenColumns: config.hiddenColumns || [],
            sortConfig: config.sortConfig || null,
            columnFilters: config.columnFilters || {},
            createdAt: Date.now(),
        };

        setAllViews(prev => [...prev, newView]);
        return newView;
    }, [resourceType]);

    /**
     * Get a view by ID.
     *
     * @param {string} viewId - The view ID
     * @returns {object|null} The view configuration or null if not found
     */
    const loadView = useCallback((viewId) => {
        return allViews.find(v => v.id === viewId) || null;
    }, [allViews]);

    /**
     * Update an existing view.
     *
     * @param {string} viewId - The view ID to update
     * @param {object} updates - Fields to update
     * @returns {boolean} True if updated, false if not found
     */
    const updateView = useCallback((viewId, updates) => {
        setAllViews(prev => {
            const index = prev.findIndex(v => v.id === viewId);
            if (index === -1) return prev;

            const updated = [...prev];
            updated[index] = { ...updated[index], ...updates };
            return updated;
        });
        return true;
    }, []);

    /**
     * Rename a view.
     *
     * @param {string} viewId - The view ID
     * @param {string} newName - The new name
     */
    const renameView = useCallback((viewId, newName) => {
        updateView(viewId, { name: newName });
    }, [updateView]);

    /**
     * Delete a view.
     *
     * @param {string} viewId - The view ID to delete
     */
    const deleteView = useCallback((viewId) => {
        setAllViews(prev => prev.filter(v => v.id !== viewId));
    }, []);

    /**
     * Duplicate a view.
     *
     * @param {string} viewId - The view ID to duplicate
     * @returns {object|null} The new view or null if source not found
     */
    const duplicateView = useCallback((viewId) => {
        const source = allViews.find(v => v.id === viewId);
        if (!source) return null;

        const newView = {
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
     *
     * @param {string} viewId - The view ID to set as default (or unset if already default)
     */
    const setDefaultView = useCallback((viewId) => {
        setAllViews(prev => {
            const view = prev.find(v => v.id === viewId);
            if (!view) return prev;

            const isCurrentlyDefault = view.isDefault;
            const viewResourceType = view.resourceType;

            return prev.map(v => {
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
     *
     * @returns {object|null} The default view or null if none set
     */
    const getDefaultView = useCallback(() => {
        return views.find(v => v.isDefault) || null;
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
