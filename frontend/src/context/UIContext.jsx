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

    // Track active detail tab per resource type (e.g., { pod: 'metrics', node: 'basic' })
    const [detailTabByResourceType, setDetailTabByResourceType] = useState({});

    // Use ref to store pending search for reliable consumption
    // This avoids timing issues with React 18's batching
    const pendingSearchRef = useRef(null);

    // Navigate to a view with a pre-filled search term
    const navigateWithSearch = useCallback((view, searchTerm) => {
        pendingSearchRef.current = searchTerm;
        setPendingSearch(searchTerm); // Trigger re-render
        setActiveView(view);
    }, []);

    // Consume pending search (called by ResourceList when it mounts/updates)
    const consumePendingSearch = useCallback(() => {
        const result = pendingSearchRef.current;
        pendingSearchRef.current = null;
        setPendingSearch(null);
        return result;
    }, []);

    // Persistence for activeView
    // We want to save activeView per context, similar to namespace
    useEffect(() => {
        if (!currentContext) return;

        const saved = localStorage.getItem(`kubikles_state_${currentContext}`);
        if (saved) {
            try {
                const state = JSON.parse(saved);
                if (state.view) {
                    setActiveView(state.view);
                }
            } catch (e) {
                console.error("Failed to parse saved view state", e);
            }
        }
    }, [currentContext]);

    useEffect(() => {
        if (!currentContext) return;

        const existing = localStorage.getItem(`kubikles_state_${currentContext}`);
        let state = {};
        if (existing) {
            try {
                state = JSON.parse(existing);
            } catch (e) { }
        }
        state.view = activeView;
        localStorage.setItem(`kubikles_state_${currentContext}`, JSON.stringify(state));
    }, [currentContext, activeView]);

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

    // Close all stale tabs
    const closeAllStaleTabs = useCallback(() => {
        setBottomTabs(prev => prev.filter(t => !(t.context && t.context !== currentContext)));
        setActiveTabId(prev => prev); // Will be cleaned up by effect if needed
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
        closeAllStaleTabs,
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
        setDetailTab
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
        closeAllStaleTabs,
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
        setDetailTab
    ]);

    return (
        <UIContext.Provider value={value}>
            {children}
        </UIContext.Provider>
    );
};
