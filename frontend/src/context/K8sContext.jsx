import React, { createContext, useContext, useState, useEffect } from 'react';
import { ListContexts, GetCurrentContext, SwitchContext, ListNamespaces } from '../../wailsjs/go/main/App';

const K8sContext = createContext();

export const useK8s = () => {
    const context = useContext(K8sContext);
    if (!context) {
        throw new Error('useK8s must be used within a K8sProvider');
    }
    return context;
};

export const K8sProvider = ({ children }) => {
    const [contexts, setContexts] = useState([]);
    const [currentContext, setCurrentContext] = useState('');
    const [namespaces, setNamespaces] = useState([]);
    const [currentNamespace, setCurrentNamespace] = useState('default');

    // Persistence Helpers
    const loadContextState = (ctx) => {
        const saved = localStorage.getItem(`kubikles_state_${ctx}`);
        if (saved) {
            try {
                return JSON.parse(saved);
            } catch (e) {
                console.error("Failed to parse saved state", e);
            }
        }
        return { namespace: 'default' };
    };

    const saveContextState = (ctx, ns) => {
        if (!ctx) return;
        // We only save namespace here. View state is UI concern.
        // We might need to coordinate this if we want to save view per context too.
        // For now, let's save namespace.
        // Wait, the original code saved view AND namespace.
        // Let's allow passing extra state or handle view in UIContext but save it keyed by context?
        // Or maybe K8sContext just exposes a "saveState" function?
        // Let's keep it simple: K8sContext manages namespace persistence.
        // UIContext can manage view persistence if needed, or we pass view to saveContextState.

        // Actually, let's just save namespace for now. 
        // If we want to persist view per context, we might need to expose a way to update that.
        const existing = localStorage.getItem(`kubikles_state_${ctx}`);
        let state = {};
        if (existing) {
            try {
                state = JSON.parse(existing);
            } catch (e) { }
        }
        state.namespace = ns;
        localStorage.setItem(`kubikles_state_${ctx}`, JSON.stringify(state));
    };

    const fetchContexts = async () => {
        try {
            const list = await ListContexts();
            const curr = await GetCurrentContext();
            const sortedList = (list || []).sort((a, b) => a.localeCompare(b));
            setContexts(sortedList);
            setCurrentContext(curr);

            // Load saved namespace for this context
            const savedState = loadContextState(curr);
            if (savedState.namespace) {
                setCurrentNamespace(savedState.namespace);
            }
        } catch (err) {
            console.error("Failed to fetch contexts", err);
        }
    };

    const fetchNamespaces = async () => {
        if (!currentContext) return;
        try {
            const list = await ListNamespaces(currentContext);
            // Extract namespace names from objects
            const namespaceNames = (list || []).map(ns => ns.metadata?.name || ns).filter(Boolean);
            setNamespaces(namespaceNames);
        } catch (err) {
            console.error("Failed to fetch namespaces", err);
        }
    };

    const switchContext = async (newContext) => {
        try {
            await SwitchContext(newContext);
            setCurrentContext(newContext);

            const savedState = loadContextState(newContext);
            setCurrentNamespace(savedState.namespace || 'default');

            // We need to trigger namespace fetch after switch
            // fetchNamespaces will be called by useEffect when currentContext changes
        } catch (err) {
            console.error("Failed to switch context", err);
        }
    };

    // Initial Load
    useEffect(() => {
        fetchContexts();
    }, []);

    // Fetch namespaces when context changes
    useEffect(() => {
        if (currentContext) {
            fetchNamespaces();
            localStorage.setItem('kubikles_context', currentContext);
        }
    }, [currentContext]);

    // Save namespace when it changes
    useEffect(() => {
        if (currentContext) {
            saveContextState(currentContext, currentNamespace);
        }
    }, [currentContext, currentNamespace]);

    const value = {
        contexts,
        currentContext,
        namespaces,
        currentNamespace,
        setCurrentNamespace,
        switchContext,
        refreshContexts: fetchContexts,
        refreshNamespaces: fetchNamespaces
    };

    return (
        <K8sContext.Provider value={value}>
            {children}
        </K8sContext.Provider>
    );
};
