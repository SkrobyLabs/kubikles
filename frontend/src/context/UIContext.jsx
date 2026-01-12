import React, { createContext, useContext, useState, useEffect, useMemo, useCallback } from 'react';
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
    const [activeMenuId, setActiveMenuId] = useState(null);
    const [pendingSearch, setPendingSearch] = useState(null);
    const [modal, setModal] = useState(null);

    // Navigate to a view with a pre-filled search term
    const navigateWithSearch = useCallback((view, searchTerm) => {
        setPendingSearch(searchTerm);
        setActiveView(view);
    }, []);

    // Consume pending search (called by ResourceList when it mounts/updates)
    const consumePendingSearch = useCallback(() => {
        let result = null;
        setPendingSearch(prev => {
            result = prev;
            return null;
        });
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

    const closeTab = useCallback((tabId) => {
        setBottomTabs(prev => {
            const newTabs = prev.filter(t => t.id !== tabId);
            return newTabs;
        });
        setActiveTabId(prev => {
            // Need to check current tabs to determine new active
            // This is handled via effect or inline check
            return prev;
        });
    }, []);

    const closeOtherTabs = useCallback((tabId) => {
        setBottomTabs(prev => prev.filter(t => t.id === tabId));
        setActiveTabId(tabId);
    }, []);

    const closeTabsToRight = useCallback((tabId) => {
        setBottomTabs(prev => {
            const index = prev.findIndex(t => t.id === tabId);
            if (index === -1) return prev;
            return prev.slice(0, index + 1);
        });
    }, []);

    const closeAllTabs = useCallback(() => {
        setBottomTabs([]);
        setActiveTabId(null);
    }, []);

    const openModal = useCallback((config) => {
        setModal(config);
    }, []);

    const closeModal = useCallback(() => {
        setModal(null);
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
        isTabStale,
        panelHeight,
        setPanelHeight,
        activeMenuId,
        setActiveMenuId,
        pendingSearch,
        navigateWithSearch,
        consumePendingSearch,
        modal,
        openModal,
        closeModal
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
        isTabStale,
        panelHeight,
        activeMenuId,
        pendingSearch,
        navigateWithSearch,
        consumePendingSearch,
        modal,
        openModal,
        closeModal
    ]);

    return (
        <UIContext.Provider value={value}>
            {children}
        </UIContext.Provider>
    );
};
