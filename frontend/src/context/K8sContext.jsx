import React, { createContext, useContext, useState, useEffect, useCallback, useMemo, useRef } from 'react';
import { ListContexts, GetCurrentContext, SwitchContext, TestConnection, ListNamespaces, StartPortForwardsWithMode, ListCRDs, GetK8sInitError } from '../../wailsjs/go/main/App';
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime';
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

// Helper to get connection test timeout (seconds) from settings
const getConnectionTestTimeout = () => {
    try {
        const saved = localStorage.getItem('kubikles_settings');
        if (saved) {
            const settings = JSON.parse(saved);
            const timeout = settings?.kubernetes?.connectionTestTimeoutSeconds;
            if (typeof timeout === 'number' && timeout > 0) {
                return timeout;
            }
        }
    } catch (e) {
        Logger.error('Failed to read connection test timeout setting', e);
    }
    return 5; // Default 5 seconds
};

const K8sContext = createContext();

export const useK8s = () => {
    const context = useContext(K8sContext);
    if (!context) {
        throw new Error('useK8s must be used within a K8sProvider');
    }
    return context;
};

// Helper to parse connection errors and provide user-friendly messages
const parseConnectionError = (error) => {
    const errorStr = String(error);

    // AWS CLI not found
    if (errorStr.includes('executable aws not found') || errorStr.includes('aws: executable file not found')) {
        return {
            title: 'AWS CLI Not Found',
            message: 'Your kubeconfig requires AWS CLI for authentication, but it was not found in PATH.',
            suggestion: 'Install AWS CLI: brew install awscli (macOS) or visit https://aws.amazon.com/cli/',
            provider: 'aws'
        };
    }

    // Azure CLI not found
    if (errorStr.includes('executable az not found') || errorStr.includes('az: executable file not found')) {
        return {
            title: 'Azure CLI Not Found',
            message: 'Your kubeconfig requires Azure CLI for authentication, but it was not found in PATH.',
            suggestion: 'Install Azure CLI: brew install azure-cli (macOS) or visit https://docs.microsoft.com/cli/azure/install-azure-cli',
            provider: 'azure'
        };
    }

    // Google Cloud CLI not found
    if (errorStr.includes('executable gcloud not found') || errorStr.includes('gcloud: executable file not found')) {
        return {
            title: 'Google Cloud CLI Not Found',
            message: 'Your kubeconfig requires gcloud CLI for authentication, but it was not found in PATH.',
            suggestion: 'Install gcloud: https://cloud.google.com/sdk/docs/install',
            provider: 'gcloud'
        };
    }

    // K8s client not initialized
    if (errorStr.includes('k8s client not initialized')) {
        return {
            title: 'Kubernetes Client Failed',
            message: 'Failed to initialize the Kubernetes client. This may be due to an invalid kubeconfig or authentication issue.',
            suggestion: 'Check your kubeconfig file (~/.kube/config) and ensure your credentials are valid.',
            provider: 'unknown'
        };
    }

    // Generic connection errors
    if (errorStr.includes('connection refused') || errorStr.includes('no such host')) {
        return {
            title: 'Connection Failed',
            message: 'Could not connect to the Kubernetes cluster.',
            suggestion: 'Verify the cluster is running and accessible from your network.',
            provider: 'network'
        };
    }

    // Certificate/TLS errors
    if (errorStr.includes('x509') || errorStr.includes('certificate')) {
        return {
            title: 'Certificate Error',
            message: 'There was a problem with the cluster certificate.',
            suggestion: 'Check that your kubeconfig has the correct certificate authority, or the cluster certificate is valid.',
            provider: 'unknown'
        };
    }

    // Unauthorized/Forbidden errors
    if (errorStr.includes('Unauthorized') || errorStr.includes('forbidden')) {
        return {
            title: 'Authentication Failed',
            message: 'Your credentials were rejected by the cluster.',
            suggestion: 'Your token may have expired. Try refreshing your credentials or re-authenticating.',
            provider: 'unknown'
        };
    }

    // Default fallback
    return {
        title: 'Connection Error',
        message: errorStr,
        suggestion: 'Check your kubeconfig and cluster connectivity.',
        provider: 'unknown'
    };
};

export const K8sProvider = ({ children }) => {
    const [contexts, setContexts] = useState([]);
    const [currentContext, setCurrentContext] = useState('');
    const currentContextRef = useRef(''); // Ref for event handlers to avoid stale closures
    const [namespaces, setNamespaces] = useState([]);
    const [selectedNamespaces, setSelectedNamespaces] = useState(['default']);
    const [lastRefresh, setLastRefresh] = useState(Date.now());
    const [isLoadingNamespaces, setIsLoadingNamespaces] = useState(false);
    const [watcherStatus, setWatcherStatus] = useState({}); // { resourceType: { status, error } }
    const [connectionError, setConnectionError] = useState(null); // { title, message, suggestion, provider, raw }
    const [isConnecting, setIsConnecting] = useState(true); // Initial loading state

    // Track when each context was last accessed (for sorting)
    const [contextAccessTimes, setContextAccessTimes] = useState(() => {
        try {
            const saved = localStorage.getItem('kubikles_context_access_times');
            return saved ? JSON.parse(saved) : {};
        } catch {
            return {};
        }
    });

    // CRD state for owner reference resolution
    const [crds, setCRDs] = useState([]);
    const crdsLoadedForContext = useRef(null);

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

    // Update context access time (call when switching to a context)
    const updateContextAccessTime = useCallback((ctx) => {
        if (!ctx) return;
        setContextAccessTimes(prev => {
            const updated = { ...prev, [ctx]: Date.now() };
            localStorage.setItem('kubikles_context_access_times', JSON.stringify(updated));
            return updated;
        });
    }, []);

    // Sorted contexts: by last accessed (descending), then by name
    const sortedContexts = useMemo(() => {
        return [...contexts].sort((a, b) => {
            const timeA = contextAccessTimes[a] || 0;
            const timeB = contextAccessTimes[b] || 0;
            // Sort by access time descending (most recent first)
            if (timeA !== timeB) {
                return timeB - timeA;
            }
            // Then by name ascending
            return a.localeCompare(b);
        });
    }, [contexts, contextAccessTimes]);

    const fetchContexts = useCallback(async () => {
        setIsConnecting(true);
        // Don't clear connectionError here - only clear it when we have confirmed connectivity
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
            updateContextAccessTime(contextToUse);
            Logger.info("Contexts fetched", { count: sortedList.length, current: contextToUse });

            // Load saved namespaces for this context (will be overwritten by data loading effect)
            const savedState = loadContextState(contextToUse);
            if (savedState.namespaces && savedState.namespaces.length > 0) {
                setSelectedNamespaces(savedState.namespaces);
                Logger.debug("Restored namespaces from saved state", { namespaces: savedState.namespaces });
            }
            // Connection test happens in the data loading effect when currentContext changes
        } catch (err) {
            Logger.error("Failed to fetch contexts", err);
            const parsed = parseConnectionError(err);
            setConnectionError({
                ...parsed,
                raw: String(err)
            });
            setIsConnecting(false);
        }
        // Note: isConnecting will be cleared by the data loading effect after connection test
    }, [updateContextAccessTime]);

    // Lightweight refresh that only updates if contexts changed (avoids UI flicker)
    const refreshContextsIfChanged = useCallback(async () => {
        try {
            const list = await ListContexts();
            const sortedList = (list || []).sort((a, b) => a.localeCompare(b));

            // Only update if the list actually changed
            setContexts(prev => {
                const lengthChanged = prev.length !== sortedList.length;
                const contentChanged = sortedList.some((ctx, i) => ctx !== prev[i]);
                if (lengthChanged || contentChanged) {
                    Logger.debug("Kubeconfig contexts changed", { previous: prev, current: sortedList });
                    return sortedList;
                }
                return prev;
            });
        } catch (err) {
            Logger.error("Failed to refresh contexts", err);
        }
    }, []);

    const fetchNamespaces = useCallback(async () => {
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
            // Note: Don't clear connectionError here - other API calls might still be failing.
            // Error is only cleared when a watcher successfully connects.
            setIsConnecting(false);
        } catch (err) {
            Logger.error("Failed to fetch namespaces", err);
            // Set connection error - this is the first actual cluster call
            const parsed = parseConnectionError(err);
            setConnectionError({
                ...parsed,
                raw: String(err)
            });
            setIsConnecting(false);
        }
    }, [currentContext]);

    const switchContext = useCallback(async (newContext) => {
        try {
            Logger.info("Switching context...", { from: currentContext, to: newContext });

            // Update ref IMMEDIATELY so any in-flight requests see the new context
            // and ignore their stale results. This must happen before any async calls.
            currentContextRef.current = newContext;

            // Clear previous errors and show connecting state
            setConnectionError(null);
            setIsConnecting(true);

            // Clear state immediately to prevent stale data display
            setNamespaces([]);
            setSelectedNamespaces([]);
            setWatcherStatus({});

            // Set loading flag to prevent saves during switch
            setIsLoadingNamespaces(true);

            // This cancels pending connection tests and switches context
            await SwitchContext(newContext);

            // Update UI state - connection test happens in the data loading effect
            setCurrentContext(newContext);
            updateContextAccessTime(newContext);
            localStorage.setItem('kubikles_last_context', newContext);
            Logger.debug("Context switched, data loading will follow", { context: newContext });
        } catch (err) {
            Logger.error("Failed to switch context", err);
            setIsConnecting(false);
            setIsLoadingNamespaces(false);
        }
    }, [currentContext, updateContextAccessTime]);

    // Initial Load
    useEffect(() => {
        // Check for K8s client initialization errors first
        const checkInitError = async () => {
            try {
                const initError = await GetK8sInitError();
                if (initError) {
                    Logger.error("K8s client initialization failed", { error: initError });
                    setConnectionError({
                        title: 'Kubernetes Client Failed to Initialize',
                        message: initError,
                        suggestion: 'Check your kubeconfig file (~/.kube/config) and ensure it is valid.',
                        provider: 'unknown',
                        raw: initError
                    });
                    setIsConnecting(false);
                    return;
                }
            } catch (err) {
                Logger.error("Failed to check K8s init error", err);
            }
            // If no init error, proceed with normal loading
            fetchContexts();
        };
        checkInitError();
    }, []);

    // Fetch namespaces when context changes
    useEffect(() => {
        if (!currentContext) return;

        // Capture the context this effect is for - used to detect stale results
        const contextForThisEffect = currentContext;
        let cancelled = false;

        const loadNamespacesAndRestoreState = async () => {
            setIsLoadingNamespaces(true);

            // Quick connectivity check - fail fast if cluster unreachable
            const connectionTimeout = getConnectionTestTimeout();
            Logger.debug("Testing connection to cluster...", { context: contextForThisEffect, timeoutSeconds: connectionTimeout });
            try {
                await TestConnection(connectionTimeout);
                // Check if context changed while we were waiting
                if (cancelled || currentContextRef.current !== contextForThisEffect) {
                    Logger.debug("Connection test completed but context changed, ignoring", {
                        testedContext: contextForThisEffect,
                        currentContext: currentContextRef.current
                    });
                    return;
                }
                Logger.info("Connection test passed", { context: contextForThisEffect });
            } catch (connErr) {
                // Check if context changed while we were waiting
                if (cancelled || currentContextRef.current !== contextForThisEffect) {
                    Logger.debug("Connection test failed but context changed, ignoring", {
                        testedContext: contextForThisEffect,
                        currentContext: currentContextRef.current
                    });
                    return;
                }
                Logger.error("Connection test failed", { context: contextForThisEffect, error: connErr });
                const parsed = parseConnectionError(String(connErr));
                setConnectionError({
                    ...parsed,
                    raw: String(connErr)
                });
                setIsConnecting(false);
                setIsLoadingNamespaces(false);
                return; // Don't proceed with namespace loading if connection fails
            }

            await fetchNamespaces();

            // Check again after namespace fetch
            if (cancelled || currentContextRef.current !== contextForThisEffect) {
                Logger.debug("Namespace fetch completed but context changed, ignoring");
                return;
            }

            // After namespaces are loaded, restore saved state
            const savedState = loadContextState(contextForThisEffect);
            if (savedState.namespaces && savedState.namespaces.length > 0) {
                setSelectedNamespaces(savedState.namespaces);
                Logger.debug("Restored namespaces after context switch", { namespaces: savedState.namespaces });
            }
            setIsLoadingNamespaces(false);
            setIsConnecting(false);

            // Start port forwards based on auto-start mode setting
            const autoStartMode = getPortForwardAutoStartMode();
            try {
                await StartPortForwardsWithMode(contextForThisEffect, autoStartMode);
                Logger.debug("Started port forwards", { context: contextForThisEffect, mode: autoStartMode });
            } catch (err) {
                Logger.error("Failed to start port forwards", err);
            }
        };

        loadNamespacesAndRestoreState();
        localStorage.setItem('kubikles_context', currentContext);

        // Cleanup: mark this effect as cancelled if context changes
        return () => {
            cancelled = true;
        };
    }, [currentContext]);

    // Save namespaces when they change (but not while loading)
    useEffect(() => {
        if (currentContext && !isLoadingNamespaces) {
            saveContextState(currentContext, selectedNamespaces);
            Logger.debug("Namespaces changed", { context: currentContext, namespaces: selectedNamespaces });
        }
    }, [currentContext, selectedNamespaces, isLoadingNamespaces]);

    // Listen for watcher error and status events
    useEffect(() => {
        if (!window.runtime) return;

        const handleWatcherError = (event) => {
            const { resourceType, namespace, error, recoverable, context } = event;

            // Ignore errors from a different context (stale events after context switch)
            if (context && context !== currentContextRef.current) {
                Logger.debug("Ignoring stale watcher error from old context", { context, currentContext: currentContextRef.current });
                return;
            }

            Logger.warn("Watcher error received", { resourceType, namespace, error, recoverable });
            setWatcherStatus(prev => ({
                ...prev,
                [resourceType]: { status: 'error', error, namespace, recoverable }
            }));

            // Check if this is an auth/connection error that should show the connection error UI
            const errorStr = String(error);
            const isAuthError = errorStr.includes('executable aws not found') ||
                errorStr.includes('executable az not found') ||
                errorStr.includes('executable gcloud not found') ||
                errorStr.includes('aws: executable file not found') ||
                errorStr.includes('az: executable file not found') ||
                errorStr.includes('gcloud: executable file not found') ||
                errorStr.includes('credential plugin') ||
                errorStr.includes('getting credentials');

            if (isAuthError) {
                const parsed = parseConnectionError(error);
                setConnectionError({
                    ...parsed,
                    raw: errorStr
                });
                setIsConnecting(false);
            }
        };

        const handleWatcherStatus = (event) => {
            const { resourceType, namespace, status, context } = event;

            // Ignore status from a different context (stale events after context switch)
            if (context && context !== currentContextRef.current) {
                Logger.debug("Ignoring stale watcher status from old context", { context, currentContext: currentContextRef.current });
                return;
            }

            Logger.debug("Watcher status changed", { resourceType, namespace, status });
            setWatcherStatus(prev => ({
                ...prev,
                [resourceType]: { status, namespace, error: null }
            }));

            // Clear connection error if a watcher successfully connects
            if (status === 'running' || status === 'connected') {
                setConnectionError(null);
                setIsConnecting(false);
            }
        };

        EventsOn("watcher-error", handleWatcherError);
        EventsOn("watcher-status", handleWatcherStatus);

        return () => {
            EventsOff("watcher-error", handleWatcherError);
            EventsOff("watcher-status", handleWatcherStatus);
        };
    }, []);

    // Keep context ref in sync for event handlers (avoids stale closures)
    useEffect(() => {
        currentContextRef.current = currentContext;
    }, [currentContext]);

    // Clear watcher status on context switch
    useEffect(() => {
        setWatcherStatus({});
    }, [currentContext]);

    const triggerRefresh = useCallback(() => {
        setLastRefresh(Date.now());
        Logger.debug("Triggered resource refresh");
    }, []);

    const retryConnection = useCallback(() => {
        Logger.info("Retrying connection...");
        fetchContexts();
    }, [fetchContexts]);

    // Check if an error is an auth/connection error and set connectionError if so
    const checkConnectionError = useCallback((error) => {
        if (!error) return false;
        const errorStr = String(error);
        const isAuthError = errorStr.includes('executable aws not found') ||
            errorStr.includes('executable az not found') ||
            errorStr.includes('executable gcloud not found') ||
            errorStr.includes('aws: executable file not found') ||
            errorStr.includes('az: executable file not found') ||
            errorStr.includes('gcloud: executable file not found') ||
            errorStr.includes('credential plugin') ||
            errorStr.includes('getting credentials') ||
            errorStr.includes('Unauthorized') ||
            errorStr.includes('forbidden') ||
            errorStr.includes('certificate') ||
            errorStr.includes('x509');

        if (isAuthError) {
            const parsed = parseConnectionError(error);
            setConnectionError({
                ...parsed,
                raw: errorStr
            });
            return true;
        }
        return false;
    }, []);

    // Fetch CRDs lazily when needed (for owner reference resolution)
    const ensureCRDsLoaded = useCallback(async () => {
        if (!currentContext) return [];
        if (crdsLoadedForContext.current === currentContext && crds.length > 0) {
            return crds;
        }

        try {
            Logger.debug("Fetching CRDs for owner resolution...");
            const list = await ListCRDs();
            setCRDs(list || []);
            crdsLoadedForContext.current = currentContext;
            return list || [];
        } catch (err) {
            Logger.error("Failed to fetch CRDs", err);
            return [];
        }
    }, [currentContext, crds]);

    // Look up CRD by apiVersion and kind (for resolving owner references)
    const findCRD = useCallback((apiVersion, kind) => {
        if (!apiVersion || !kind) return null;

        // Parse apiVersion: "group/version" or just "version" for core API
        const parts = apiVersion.split('/');
        const group = parts.length === 2 ? parts[0] : '';
        const version = parts.length === 2 ? parts[1] : parts[0];

        // Find matching CRD
        return crds.find(crd => {
            const crdGroup = crd.spec?.group || '';
            const crdKind = crd.spec?.names?.kind || '';
            // Match group and kind
            return crdGroup === group && crdKind === kind;
        }) || null;
    }, [crds]);

    // Clear CRDs on context switch
    useEffect(() => {
        setCRDs([]);
        crdsLoadedForContext.current = null;
    }, [currentContext]);

    const value = useMemo(() => ({
        contexts,
        sortedContexts,
        currentContext,
        namespaces,
        currentNamespace,
        setCurrentNamespace,
        selectedNamespaces,
        setSelectedNamespaces,
        switchContext,
        refreshContexts: fetchContexts,
        refreshContextsIfChanged,
        refreshNamespaces: fetchNamespaces,
        lastRefresh,
        triggerRefresh,
        watcherStatus,
        // CRD lookup for owner reference resolution
        crds,
        ensureCRDsLoaded,
        findCRD,
        // Connection state
        connectionError,
        isConnecting,
        retryConnection,
        checkConnectionError,
    }), [
        contexts,
        sortedContexts,
        currentContext,
        namespaces,
        currentNamespace,
        selectedNamespaces,
        switchContext,
        fetchContexts,
        refreshContextsIfChanged,
        fetchNamespaces,
        lastRefresh,
        triggerRefresh,
        watcherStatus,
        crds,
        ensureCRDsLoaded,
        findCRD,
        connectionError,
        isConnecting,
        retryConnection,
        checkConnectionError,
    ]);

    return (
        <K8sContext.Provider value={value}>
            {children}
        </K8sContext.Provider>
    );
};
