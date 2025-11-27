import React, { createContext, useContext, useState, useEffect } from 'react';
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
    const navigateWithSearch = (view, searchTerm) => {
        setPendingSearch(searchTerm);
        setActiveView(view);
    };

    // Consume pending search (called by ResourceList when it mounts/updates)
    const consumePendingSearch = () => {
        const search = pendingSearch;
        setPendingSearch(null);
        return search;
    };

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

    // Tab Management
    const openTab = (tab) => {
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
    };

    const updateTab = (tabId, updates) => {
        setBottomTabs(prev => prev.map(t =>
            t.id === tabId ? { ...t, ...updates } : t
        ));
    };

    const closeTab = (tabId) => {
        const newTabs = bottomTabs.filter(t => t.id !== tabId);
        setBottomTabs(newTabs);
        if (activeTabId === tabId) {
            setActiveTabId(newTabs.length > 0 ? newTabs[newTabs.length - 1].id : null);
        }
    };

    const closeOtherTabs = (tabId) => {
        const newTabs = bottomTabs.filter(t => t.id === tabId);
        setBottomTabs(newTabs);
        setActiveTabId(tabId);
    };

    const closeTabsToRight = (tabId) => {
        const index = bottomTabs.findIndex(t => t.id === tabId);
        if (index === -1) return;
        const newTabs = bottomTabs.slice(0, index + 1);
        setBottomTabs(newTabs);
        if (!newTabs.find(t => t.id === activeTabId)) {
            setActiveTabId(tabId);
        }
    };

    const closeAllTabs = () => {
        setBottomTabs([]);
        setActiveTabId(null);
    };

    const openModal = (config) => {
        setModal(config);
    };

    const closeModal = () => {
        setModal(null);
    };

    const reorderTabs = (fromIndex, toIndex) => {
        const newTabs = [...bottomTabs];
        const [removed] = newTabs.splice(fromIndex, 1);
        newTabs.splice(toIndex, 0, removed);
        setBottomTabs(newTabs);
    };

    // Check if a tab is stale (belongs to a different context)
    const isTabStale = (tab) => {
        return tab.context && tab.context !== currentContext;
    };

    // Close all stale tabs
    const closeAllStaleTabs = () => {
        const freshTabs = bottomTabs.filter(t => !isTabStale(t));
        setBottomTabs(freshTabs);
        if (activeTabId && !freshTabs.find(t => t.id === activeTabId)) {
            setActiveTabId(freshTabs.length > 0 ? freshTabs[freshTabs.length - 1].id : null);
        }
    };

    const value = {
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
    };

    return (
        <UIContext.Provider value={value}>
            {children}
        </UIContext.Provider>
    );
};
