import React, { createContext, useContext, useState, useEffect, useMemo, useCallback, useRef } from 'react';
import { useK8s } from './K8sContext';

const UIContext = createContext();

export const useUI = () => {
    const context = useContext(UIContext);
    if (!context) {
        throw new Error('useUI must be used within a UIProvider');
    }
    return context;
};

export const UIProvider = ({ children }) => {
    const { currentContext } = useK8s();

    const [activeView, setActiveView] = useState('pods');
    const [bottomTabs, setBottomTabs] = useState([]);
    const [activeTabId, setActiveTabId] = useState(null);
    const [panelHeight, setPanelHeight] = useState(40);
    const [pendingSearch, setPendingSearch] = useState(null);
    const [modal, setModal] = useState(null);

    // Diagnostic view params - used when navigating to diagnostic views with pre-filled data
    const [diagnosticParams, setDiagnosticParams] = useState(null);

    // Comparison source for resource diff feature
    // Format: { kind: 'pod', namespace: 'default', name: 'nginx-xyz', context: 'my-cluster' }
    // Persists across context switches for cross-cluster comparison
    const [comparisonSource, setComparisonSourceState] = useState(null);

    // Track active detail tab per resource type (e.g., { pod: 'metrics', node: 'basic' })
    const [detailTabByResourceType, setDetailTabByResourceType] = useState({});

    // Track last active tab per K8s context for restoration on context switch
    const activeTabByContextRef = useRef({});
    const previousContextRef = useRef(currentContext);

    // Use ref to store pending search for reliable consumption
    // This avoids timing issues with React 18's batching
    const pendingSearchRef = useRef(null);
    const pendingAutoOpenRef = useRef(false);

    // Navigate to a view with a pre-filled search term
    // If autoOpenDetails is true, the first matching row will be auto-clicked
    const navigateWithSearch = useCallback((view, searchTerm, autoOpenDetails = false) => {
        pendingSearchRef.current = searchTerm;
        pendingAutoOpenRef.current = autoOpenDetails;
        setPendingSearch(searchTerm); // Trigger re-render
        setActiveView(view);
    }, []);

    // Navigate to a diagnostic view with initial params
    const openDiagnostic = useCallback((view, params = {}) => {
        setDiagnosticParams(params);
        setActiveView(view);
    }, []);

    // Consume diagnostic params (called by diagnostic component when it mounts)
    const consumeDiagnosticParams = useCallback(() => {
        const result = diagnosticParams;
        setDiagnosticParams(null);
        return result;
    }, [diagnosticParams]);

    // Set comparison source for resource diff (includes current context for cross-cluster comparison)
    const setComparisonSource = useCallback((kind, namespace, name) => {
        setComparisonSourceState({ kind, namespace, name, context: currentContext });
    }, [currentContext]);

    // Clear comparison source
    const clearComparisonSource = useCallback(() => {
        setComparisonSourceState(null);
    }, []);

    // Compare current resource with the comparison source
    const compareWithSource = useCallback((targetKind, targetNamespace, targetName) => {
        if (!comparisonSource) {
            return;
        }
        const params = {
            initialSource: {
                kind: comparisonSource.kind,
                namespace: comparisonSource.namespace,
                name: comparisonSource.name,
                context: comparisonSource.context
            },
            initialTarget: {
                kind: targetKind,
                namespace: targetNamespace,
                name: targetName,
                context: currentContext
            }
        };
        openDiagnostic('resource-diff', params);
        // Clear comparison source after use
        setComparisonSourceState(null);
    }, [comparisonSource, currentContext, openDiagnostic]);

    // Consume pending search (called by ResourceList when it mounts/updates)
    const consumePendingSearch = useCallback(() => {
        const result = pendingSearchRef.current;
        const autoOpen = pendingAutoOpenRef.current;
        pendingSearchRef.current = null;
        pendingAutoOpenRef.current = false;
        setPendingSearch(null);
        return { search: result, autoOpen };
    }, []);

    // Handle context switches - save/restore active tab per context
    useEffect(() => {
        const prevContext = previousContextRef.current;

        // Skip if context hasn't actually changed
        if (prevContext === currentContext) return;

        // Save current active tab for the previous context
        if (prevContext && activeTabId) {
            activeTabByContextRef.current[prevContext] = activeTabId;
        }

        // Find visible tabs for the new context
        // A tab is visible if: it's not stale (belongs to current context or is context-independent) OR it's pinned
        const visibleTabs = bottomTabs.filter(tab => {
            const isStale = tab.context && tab.context !== currentContext;
            return !isStale || tab.pinned;
        });

        // Try to restore last active tab for this context
        const savedTabId = activeTabByContextRef.current[currentContext];
        const savedTabStillVisible = savedTabId && visibleTabs.some(t => t.id === savedTabId);

        if (savedTabStillVisible) {
            // Restore the last active tab for this context
            setActiveTabId(savedTabId);
        } else if (visibleTabs.length > 0) {
            // Fall back to first visible tab
            setActiveTabId(visibleTabs[0].id);
        } else {
            // No visible tabs
            setActiveTabId(null);
        }

        // Update previous context ref
        previousContextRef.current = currentContext;
    }, [currentContext, bottomTabs, activeTabId]);

    // Persistence for activeView - save globally (not per-context)
    useEffect(() => {
        const saved = localStorage.getItem('kubikles_active_view');
        if (saved) {
            setActiveView(saved);
        }
    }, []);

    useEffect(() => {
        localStorage.setItem('kubikles_active_view', activeView);
    }, [activeView]);

    // Tab Management (all callbacks memoized to prevent re-renders)
    const openTab = useCallback((tab) => {
        setBottomTabs(prev => {
            const existingIndex = prev.findIndex(t => t.id === tab.id);
            if (existingIndex >= 0) {
                // Update existing tab - preserve original context
                const newTabs = [...prev];
                newTabs[existingIndex] = {
                    ...prev[existingIndex],
                    ...tab,
                    context: prev[existingIndex].context // Keep original context
                };
                return newTabs;
            }
            // Add new tab - use explicit context if provided, otherwise current context
            // (context: null means context-independent, context: undefined means use current)
            const tabContext = 'context' in tab ? tab.context : currentContext;
            return [...prev, { ...tab, context: tabContext }];
        });
        setActiveTabId(tab.id);
    }, [currentContext]);

    const updateTab = useCallback((tabId, updates) => {
        setBottomTabs(prev => prev.map(t =>
            t.id === tabId ? { ...t, ...updates } : t
        ));
    }, []);

    const togglePinTab = useCallback((tabId) => {
        setBottomTabs(prev => {
            const newTabs = prev.map(t =>
                t.id === tabId ? { ...t, pinned: !t.pinned } : t
            );
            // Sort: pinned tabs first, then unpinned (preserve order within each group)
            const pinned = newTabs.filter(t => t.pinned);
            const unpinned = newTabs.filter(t => !t.pinned);
            return [...pinned, ...unpinned];
        });
    }, []);

    const closeTab = useCallback((tabId) => {
        setBottomTabs(prev => {
            // Don't close pinned tabs
            const tab = prev.find(t => t.id === tabId);
            if (tab?.pinned) return prev;

            const closingIndex = prev.findIndex(t => t.id === tabId);
            const newTabs = prev.filter(t => t.id !== tabId);

            // Update active tab if we're closing the active one
            setActiveTabId(currentActive => {
                if (currentActive !== tabId) return currentActive;
                if (newTabs.length === 0) return null;
                // Try to select the tab to the left, otherwise the one that took its place
                const newIndex = Math.min(closingIndex - 1, newTabs.length - 1);
                return newTabs[Math.max(0, newIndex)]?.id || null;
            });

            return newTabs;
        });
    }, []);

    const closeOtherTabs = useCallback((tabId) => {
        setBottomTabs(prev => prev.filter(t => t.id === tabId || t.pinned));
        setActiveTabId(tabId);
    }, []);

    const closeTabsToRight = useCallback((tabId) => {
        setBottomTabs(prev => {
            const index = prev.findIndex(t => t.id === tabId);
            if (index === -1) return prev;
            // Keep tabs to the left (including current) + any pinned tabs to the right
            const leftTabs = prev.slice(0, index + 1);
            const rightPinned = prev.slice(index + 1).filter(t => t.pinned);
            return [...leftTabs, ...rightPinned];
        });
    }, []);

    const closeAllTabs = useCallback(() => {
        setBottomTabs(prev => prev.filter(t => t.pinned));
        setActiveTabId(prev => prev);
    }, []);

    const openModal = useCallback((config) => {
        setModal(config);
    }, []);

    const closeModal = useCallback(() => {
        setModal(null);
    }, []);

    // Get the active detail tab for a resource type, with fallback to default
    const getDetailTab = useCallback((resourceType, defaultTab) => {
        return detailTabByResourceType[resourceType] || defaultTab;
    }, [detailTabByResourceType]);

    // Set the active detail tab for a resource type
    const setDetailTab = useCallback((resourceType, tab) => {
        setDetailTabByResourceType(prev => ({
            ...prev,
            [resourceType]: tab
        }));
    }, []);

    const reorderTabs = useCallback((fromIndex, toIndex) => {
        setBottomTabs(prev => {
            const newTabs = [...prev];
            const [removed] = newTabs.splice(fromIndex, 1);
            newTabs.splice(toIndex, 0, removed);
            return newTabs;
        });
    }, []);

    // Check if a tab is stale (belongs to a different context)
    const isTabStale = useCallback((tab) => {
        return tab.context && tab.context !== currentContext;
    }, [currentContext]);

    // Memoize context value to prevent unnecessary re-renders of consumers
    // Note: activeMenuId moved to MenuContext for better performance
    const value = useMemo(() => ({
        activeView,
        setActiveView,
        bottomTabs,
        setBottomTabs,
        activeTabId,
        setActiveTabId,
        openTab,
        updateTab,
        closeTab,
        closeOtherTabs,
        closeTabsToRight,
        closeAllTabs,
        reorderTabs,
        togglePinTab,
        isTabStale,
        panelHeight,
        setPanelHeight,
        pendingSearch,
        navigateWithSearch,
        consumePendingSearch,
        modal,
        openModal,
        closeModal,
        getDetailTab,
        setDetailTab,
        diagnosticParams,
        openDiagnostic,
        consumeDiagnosticParams,
        comparisonSource,
        setComparisonSource,
        clearComparisonSource,
        compareWithSource
    }), [
        activeView,
        bottomTabs,
        activeTabId,
        openTab,
        updateTab,
        closeTab,
        closeOtherTabs,
        closeTabsToRight,
        closeAllTabs,
        reorderTabs,
        togglePinTab,
        isTabStale,
        panelHeight,
        pendingSearch,
        navigateWithSearch,
        consumePendingSearch,
        modal,
        openModal,
        closeModal,
        getDetailTab,
        setDetailTab,
        diagnosticParams,
        openDiagnostic,
        consumeDiagnosticParams,
        comparisonSource,
        setComparisonSource,
        clearComparisonSource,
        compareWithSource
    ]);

    return (
        <UIContext.Provider value={value}>
            {children}
        </UIContext.Provider>
    );
};
