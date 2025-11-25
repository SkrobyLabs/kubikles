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
        if (!bottomTabs.find(t => t.id === tab.id)) {
            setBottomTabs(prev => [...prev, tab]);
        }
        setActiveTabId(tab.id);
    };

    const closeTab = (tabId) => {
        const newTabs = bottomTabs.filter(t => t.id !== tabId);
        setBottomTabs(newTabs);
        if (activeTabId === tabId) {
            setActiveTabId(newTabs.length > 0 ? newTabs[newTabs.length - 1].id : null);
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
        closeTab,
        panelHeight,
        setPanelHeight,
        activeMenuId,
        setActiveMenuId,
        pendingSearch,
        navigateWithSearch,
        consumePendingSearch
    };

    return (
        <UIContext.Provider value={value}>
            {children}
        </UIContext.Provider>
    );
};
