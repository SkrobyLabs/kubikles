import React, { createContext, useContext, useState, useEffect } from 'react';
import { ListContexts, GetCurrentContext, SwitchContext, ListNamespaces } from '../../wailsjs/go/main/App';
import Logger from '../utils/Logger';

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
                Logger.error("Failed to parse saved state", e);
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
            Logger.debug("Fetching contexts...");
            const list = await ListContexts();
            const curr = await GetCurrentContext();
            const sortedList = (list || []).sort((a, b) => a.localeCompare(b));
            setContexts(sortedList);

            // Check if there's a saved context preference
            const savedContext = localStorage.getItem('kubikles_last_context');
            Logger.debug("Context restoration check", {
                savedContext,
                currentKubectlContext: curr,
                availableContexts: sortedList
            });

            let contextToUse = curr;

            // If we have a saved context and it exists in the list, switch to it
            if (savedContext && sortedList.includes(savedContext) && savedContext !== curr) {
                Logger.info("Restoring last used context", { saved: savedContext, current: curr });
                try {
                    await SwitchContext(savedContext);
                    contextToUse = savedContext;
                    Logger.info("Successfully restored context", { context: contextToUse });
                } catch (err) {
                    Logger.error("Failed to restore saved context, using current", err);
                }
            } else if (savedContext === curr) {
                Logger.debug("Saved context matches current kubectl context, no switch needed");
            } else if (!savedContext) {
                Logger.debug("No saved context found, using current kubectl context");
            }

            setCurrentContext(contextToUse);
            Logger.info("Contexts fetched", { count: sortedList.length, current: contextToUse });

            // Load saved namespace for this context
            const savedState = loadContextState(contextToUse);
            if (savedState.namespace) {
                setCurrentNamespace(savedState.namespace);
                Logger.debug("Restored namespace from saved state", { namespace: savedState.namespace });
            }
        } catch (err) {
            Logger.error("Failed to fetch contexts", err);
        }
    };

    const fetchNamespaces = async () => {
        if (!currentContext) return;
        try {
            Logger.debug("Fetching namespaces...", { context: currentContext });
            const list = await ListNamespaces(currentContext);
            // Extract namespace names from objects
            const namespaceNames = (list || []).map(ns => ns.metadata?.name || ns).filter(Boolean);
            setNamespaces(namespaceNames);
            Logger.info("Namespaces fetched", { count: namespaceNames.length });
        } catch (err) {
            Logger.error("Failed to fetch namespaces", err);
        }
    };

    const switchContext = async (newContext) => {
        try {
            Logger.info("Switching context...", { from: currentContext, to: newContext });
            await SwitchContext(newContext);
            setCurrentContext(newContext);

            // Save the context preference
            localStorage.setItem('kubikles_last_context', newContext);
            Logger.debug("Saved context to localStorage", { context: newContext });

            const savedState = loadContextState(newContext);
            const newNs = savedState.namespace || 'default';
            setCurrentNamespace(newNs);
            Logger.info("Context switched successfully", { context: newContext, namespace: newNs });

            // We need to trigger namespace fetch after switch
            // fetchNamespaces will be called by useEffect when currentContext changes
        } catch (err) {
            Logger.error("Failed to switch context", err);
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
            Logger.debug("Namespace changed", { context: currentContext, namespace: currentNamespace });
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
