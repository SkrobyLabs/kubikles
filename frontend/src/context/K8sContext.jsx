import React, { createContext, useContext, useState, useEffect } from 'react';
import { ListContexts, GetCurrentContext, SwitchContext, ListNamespaces, StartPortForwardsWithMode } from '../../wailsjs/go/main/App';
import Logger from '../utils/Logger';

// Helper to get port forward auto-start mode from settings
const getPortForwardAutoStartMode = () => {
    try {
        const saved = localStorage.getItem('kubikles_settings');
        if (saved) {
            const settings = JSON.parse(saved);
            return settings?.portForwards?.autoStartMode || 'favorites';
        }
    } catch (e) {
        Logger.error('Failed to read port forward settings', e);
    }
    return 'favorites'; // Default
};

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
    const [selectedNamespaces, setSelectedNamespaces] = useState(['default']);
    const [lastRefresh, setLastRefresh] = useState(Date.now());
    const [isLoadingNamespaces, setIsLoadingNamespaces] = useState(false);

    // Backward compatibility: expose currentNamespace for components not yet updated
    const currentNamespace = selectedNamespaces.length === 1 ? selectedNamespaces[0] : '';
    const setCurrentNamespace = (ns) => {
        setSelectedNamespaces(Array.isArray(ns) ? ns : [ns]);
    };

    // Persistence Helpers
    const loadContextState = (ctx) => {
        const saved = localStorage.getItem(`kubikles_state_${ctx}`);
        if (saved) {
            try {
                const parsed = JSON.parse(saved);
                // Backward compatibility: migrate old single namespace to array
                if (parsed.namespace && typeof parsed.namespace === 'string') {
                    return { namespaces: [parsed.namespace] };
                }
                if (parsed.namespaces && Array.isArray(parsed.namespaces)) {
                    return { namespaces: parsed.namespaces };
                }
            } catch (e) {
                Logger.error("Failed to parse saved state", e);
            }
        }
        return { namespaces: ['default'] };
    };

    const saveContextState = (ctx, ns) => {
        if (!ctx) return;
        const existing = localStorage.getItem(`kubikles_state_${ctx}`);
        let state = {};
        if (existing) {
            try {
                state = JSON.parse(existing);
            } catch (e) { }
        }
        // Save as array for multi-namespace support
        state.namespaces = Array.isArray(ns) ? ns : [ns];
        delete state.namespace; // Remove old single namespace format
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

            // Load saved namespaces for this context
            const savedState = loadContextState(contextToUse);
            if (savedState.namespaces && savedState.namespaces.length > 0) {
                setSelectedNamespaces(savedState.namespaces);
                Logger.debug("Restored namespaces from saved state", { namespaces: savedState.namespaces });
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
            // Prepend "All Namespaces" option (empty string value)
            const namespacesWithAll = ['', ...namespaceNames];
            setNamespaces(namespacesWithAll);
            Logger.info("Namespaces fetched", { count: namespaceNames.length });
        } catch (err) {
            Logger.error("Failed to fetch namespaces", err);
        }
    };

    const switchContext = async (newContext) => {
        try {
            Logger.info("Switching context...", { from: currentContext, to: newContext });

            // Set loading flag first to prevent saves during switch
            setIsLoadingNamespaces(true);

            await SwitchContext(newContext);

            // Clear namespaces immediately to prevent stale data
            setNamespaces([]);

            // Clear selected namespaces to prevent unnecessary fetches
            // We'll restore saved state after namespaces are fetched
            setSelectedNamespaces([]);

            setCurrentContext(newContext);

            // Save the context preference
            localStorage.setItem('kubikles_last_context', newContext);
            Logger.debug("Saved context to localStorage", { context: newContext });

            Logger.info("Context switched successfully", { context: newContext });

            // We need to trigger namespace fetch after switch
            // fetchNamespaces will be called by useEffect when currentContext changes
        } catch (err) {
            Logger.error("Failed to switch context", err);
            setIsLoadingNamespaces(false);
        }
    };

    // Initial Load
    useEffect(() => {
        fetchContexts();
    }, []);

    // Fetch namespaces when context changes
    useEffect(() => {
        if (currentContext) {
            const loadNamespacesAndRestoreState = async () => {
                setIsLoadingNamespaces(true);
                await fetchNamespaces();

                // After namespaces are loaded, restore saved state
                const savedState = loadContextState(currentContext);
                if (savedState.namespaces && savedState.namespaces.length > 0) {
                    setSelectedNamespaces(savedState.namespaces);
                    Logger.debug("Restored namespaces after context switch", { namespaces: savedState.namespaces });
                }
                setIsLoadingNamespaces(false);

                // Start port forwards based on auto-start mode setting
                const autoStartMode = getPortForwardAutoStartMode();
                try {
                    await StartPortForwardsWithMode(currentContext, autoStartMode);
                    Logger.debug("Started port forwards", { context: currentContext, mode: autoStartMode });
                } catch (err) {
                    Logger.error("Failed to start port forwards", err);
                }
            };

            loadNamespacesAndRestoreState();
            localStorage.setItem('kubikles_context', currentContext);
        }
    }, [currentContext]);

    // Save namespaces when they change (but not while loading)
    useEffect(() => {
        if (currentContext && !isLoadingNamespaces) {
            saveContextState(currentContext, selectedNamespaces);
            Logger.debug("Namespaces changed", { context: currentContext, namespaces: selectedNamespaces });
        }
    }, [currentContext, selectedNamespaces, isLoadingNamespaces]);

    const triggerRefresh = () => {
        setLastRefresh(Date.now());
        Logger.debug("Triggered resource refresh");
    };

    const value = {
        contexts,
        currentContext,
        namespaces,
        currentNamespace,
        setCurrentNamespace,
        selectedNamespaces,
        setSelectedNamespaces,
        switchContext,
        refreshContexts: fetchContexts,
        refreshNamespaces: fetchNamespaces,
        lastRefresh,
        triggerRefresh
    };

    return (
        <K8sContext.Provider value={value}>
            {children}
        </K8sContext.Provider>
    );
};
