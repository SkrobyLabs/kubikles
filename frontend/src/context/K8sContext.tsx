import React, { createContext, useContext, useState, useEffect, useCallback, useMemo, useRef } from 'react';
import { ListContexts, GetCurrentContext, SwitchContext, TestConnection, ListNamespaces, StartAutoStartPortForwards, ListCRDs, GetK8sInitError } from 'wailsjs/go/main/App';
import { EventsOn } from 'wailsjs/runtime/runtime';
import Logger from '../utils/Logger';

// ============================================================================
// Type Definitions
// ============================================================================

interface Namespace {
    metadata?: {
        name?: string;
    };
}

interface CRDSpec {
    group?: string;
    names?: {
        kind?: string;
        plural?: string;
    };
    versions?: Array<{ name?: string; storage?: boolean }>;
    scope?: string;
}

interface CRD {
    spec?: CRDSpec;
}

interface ConnectionError {
    title: string;
    message: string;
    suggestion: string;
    provider: 'aws' | 'azure' | 'gcloud' | 'network' | 'unknown';
    raw: string;
}

interface ContextState {
    namespaces?: string[];
    namespace?: string; // Legacy single namespace
}

interface ContextAccessTimes {
    [context: string]: number;
}

interface WatcherErrorEvent {
    resourceType: string;
    namespace?: string;
    error: string;
    recoverable?: boolean;
    context?: string;
}

interface WatcherStatusEvent {
    resourceType: string;
    namespace?: string;
    status: 'running' | 'connected' | 'error' | 'stopped';
    context?: string;
}

interface K8sContextValue {
    // Context management
    contexts: string[];
    sortedContexts: string[];
    currentContext: string;
    switchContext: (newContext: string) => Promise<void>;
    refreshContexts: () => Promise<void>;
    refreshContextsIfChanged: () => Promise<void>;

    // Namespace management
    namespaces: string[];
    currentNamespace: string;
    selectedNamespaces: string[];
    setSelectedNamespaces: (namespaces: string[]) => void;
    refreshNamespaces: () => Promise<void>;

    // Resource refresh trigger
    lastRefresh: number;
    triggerRefresh: () => void;

    // Silent reconciliation token: incremented when watchers reconnect after
    // error to remove ghost resources without disrupting UI (no loading flash,
    // no scroll jump, no selection loss)
    reconcileToken: number;

    // CRD lookup for owner reference resolution
    crds: CRD[];
    crdsLoading: boolean;
    ensureCRDsLoaded: () => Promise<CRD[]>;

    // Connection state
    connectionError: ConnectionError | null;
    isConnecting: boolean;
    retryConnection: () => void;
    checkConnectionError: (error: unknown) => boolean;
}

// ============================================================================
// Helper Functions
// ============================================================================

// Helper to get connection test timeout (seconds) from settings
const getConnectionTestTimeout = (): number => {
    try {
        const saved = localStorage.getItem('kubikles_settings');
        if (saved) {
            const settings = JSON.parse(saved);
            const timeout = settings?.kubernetes?.connectionTestTimeoutSeconds;
            if (typeof timeout === 'number' && timeout > 0) {
                return timeout;
            }
        }
    } catch (e: any) {
        Logger.error('Failed to read connection test timeout setting', e, 'k8s');
    }
    return 5; // Default 5 seconds
};

const K8sContext = createContext<K8sContextValue | undefined>(undefined);

export const useK8s = (): K8sContextValue => {
    const context = useContext(K8sContext);
    if (!context) {
        throw new Error('useK8s must be used within a K8sProvider');
    }
    return context;
};

// Helper to parse connection errors and provide user-friendly messages
const parseConnectionError = (error: unknown): Omit<ConnectionError, 'raw'> => {
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

    // AWS STS AssumeRole denied
    if (errorStr.includes('AssumeRole') && errorStr.includes('AccessDenied')) {
        // Extract the IAM user and target role from the error for a helpful message
        const userMatch = errorStr.match(/User: (arn:aws:iam::\S+)/);
        const roleMatch = errorStr.match(/resource: (arn:aws:iam::\S+)/);
        const user = userMatch ? userMatch[1] : 'your IAM identity';
        const role = roleMatch ? roleMatch[1] : 'the target role';
        return {
            title: 'AWS Role Assumption Denied',
            message: `${user} is not authorized to assume ${role}. The IAM user/role does not have permission to call sts:AssumeRole on this EKS cluster's role.`,
            suggestion: 'Verify the IAM trust policy on the target role allows your user/role to assume it. Check that your AWS credentials are correct and not expired.',
            provider: 'aws'
        };
    }

    // AWS CLI auth failure (exit code from aws credential plugin)
    if (errorStr.includes('executable aws failed with exit code')) {
        return {
            title: 'AWS Authentication Failed',
            message: 'The AWS CLI credential plugin returned an error. This usually means AWS authentication failed.',
            suggestion: 'Check your AWS credentials (aws sts get-caller-identity), verify your AWS profile configuration, and ensure your session/token is not expired.',
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

    // Azure CLI auth failure
    if (errorStr.includes('executable az failed with exit code') || errorStr.includes('executable kubelogin failed with exit code')) {
        return {
            title: 'Azure Authentication Failed',
            message: 'The Azure credential plugin returned an error. This usually means your Azure session has expired.',
            suggestion: 'Run "az login" to re-authenticate, or check your Azure subscription access.',
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

    // GKE auth plugin failure
    if (errorStr.includes('executable gke-gcloud-auth-plugin failed with exit code') || errorStr.includes('executable gcloud failed with exit code')) {
        return {
            title: 'Google Cloud Authentication Failed',
            message: 'The GKE credential plugin returned an error. This usually means your Google Cloud session has expired.',
            suggestion: 'Run "gcloud auth login" to re-authenticate, and ensure the gke-gcloud-auth-plugin is installed.',
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

    // Generic credential exec plugin failure (catch-all for exec-based auth)
    const execMatch = errorStr.match(/executable (\S+) failed with exit code (\d+)/);
    if (execMatch || errorStr.includes('getting credentials: exec:')) {
        const execName = execMatch ? execMatch[1] : 'credential plugin';
        return {
            title: 'Credential Plugin Failed',
            message: `The kubeconfig exec-based credential plugin "${execName}" returned an error. Authentication could not be completed.`,
            suggestion: `Verify that "${execName}" is properly configured, your credentials are valid, and you have the required permissions.`,
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

export const K8sProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
    const [contexts, setContexts] = useState<string[]>([]);
    const [currentContext, setCurrentContext] = useState<string>('');
    const currentContextRef = useRef<string>(''); // Ref for event handlers to avoid stale closures
    const [namespaces, setNamespaces] = useState<string[]>([]);
    const [selectedNamespaces, setSelectedNamespaces] = useState<string[]>(['default']);
    const [lastRefresh, setLastRefresh] = useState<number>(Date.now());
    const [isLoadingNamespaces, setIsLoadingNamespaces] = useState<boolean>(false);
    const [reconcileToken, setReconcileToken] = useState(0);
    const watcherPrevStatusRef = useRef<Record<string, string>>({});
    const reconnectRefreshTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
    const [connectionError, setConnectionError] = useState<ConnectionError | null>(null); // { title, message, suggestion, provider, raw }
    const [isConnecting, setIsConnecting] = useState<boolean>(true); // Initial loading state
    const [retryToken, setRetryToken] = useState<number>(0); // Incremented to force data loading effect to re-run

    // Track when each context was last accessed (for sorting)
    const [contextAccessTimes, setContextAccessTimes] = useState<ContextAccessTimes>(() => {
        try {
            const saved = localStorage.getItem('kubikles_context_access_times');
            return saved ? JSON.parse(saved) : {};
        } catch {
            return {};
        }
    });

    // CRD state for owner reference resolution
    const [crds, setCRDs] = useState<CRD[]>([]);
    const [crdsLoading, setCRDsLoading] = useState(false);
    const crdsRef = useRef<CRD[]>([]);
    const crdsLoadedForContext = useRef<string | null>(null);
    const crdsFetchPromise = useRef<{ context: string; promise: Promise<CRD[]> } | null>(null);

    // Derived single-namespace value for components using single-namespace API
    const currentNamespace = selectedNamespaces.length === 1 ? selectedNamespaces[0] : '';

    // Persistence Helpers
    const loadContextState = (ctx: string): ContextState => {
        const saved = localStorage.getItem(`kubikles_state_${ctx}`);
        if (saved) {
            try {
                const parsed: ContextState = JSON.parse(saved);
                // Backward compatibility: migrate old single namespace to array
                if (parsed.namespace && typeof parsed.namespace === 'string') {
                    return { namespaces: [parsed.namespace] };
                }
                if (parsed.namespaces && Array.isArray(parsed.namespaces)) {
                    return { namespaces: parsed.namespaces };
                }
            } catch (e: any) {
                Logger.error("Failed to parse saved state", e, 'k8s');
            }
        }
        return { namespaces: ['default'] };
    };

    const saveContextState = (ctx: string, ns: string | string[]): void => {
        if (!ctx) return;
        const existing = localStorage.getItem(`kubikles_state_${ctx}`);
        let state: ContextState = {};
        if (existing) {
            try {
                state = JSON.parse(existing);
            } catch (e: any) { }
        }
        // Save as array for multi-namespace support
        state.namespaces = Array.isArray(ns) ? ns : [ns];
        delete state.namespace; // Remove old single namespace format
        localStorage.setItem(`kubikles_state_${ctx}`, JSON.stringify(state));
    };

    // Update context access time (call when switching to a context)
    const updateContextAccessTime = useCallback((ctx: string): void => {
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

    const fetchContexts = useCallback(async (): Promise<void> => {
        setIsConnecting(true);
        // Don't clear connectionError here - only clear it when we have confirmed connectivity
        try {
            Logger.debug("Fetching contexts...", undefined, 'k8s');
            const list: string[] = await ListContexts();
            const curr: string = await GetCurrentContext();
            const sortedList = (list || []).sort((a, b) => a.localeCompare(b));
            setContexts(sortedList);

            // Check if there's a saved context preference
            const savedContext = localStorage.getItem('kubikles_last_context');
            Logger.debug("Context restoration check", {
                savedContext,
                currentKubectlContext: curr,
                availableContexts: sortedList
            }, 'k8s');

            let contextToUse = curr;

            // If we have a saved context and it exists in the list, switch to it
            if (savedContext && sortedList.includes(savedContext) && savedContext !== curr) {
                Logger.info("Restoring last used context", { saved: savedContext, current: curr }, 'k8s');
                try {
                    await SwitchContext(savedContext);
                    contextToUse = savedContext;
                    Logger.info("Successfully restored context", { context: contextToUse }, 'k8s');
                } catch (err: any) {
                    Logger.error("Failed to restore saved context, using current", err, 'k8s');
                }
            } else if (savedContext === curr) {
                Logger.debug("Saved context matches current kubectl context, no switch needed", undefined, 'k8s');
            } else if (!savedContext) {
                Logger.debug("No saved context found, using current kubectl context", undefined, 'k8s');
            }

            setCurrentContext(contextToUse);
            updateContextAccessTime(contextToUse);
            Logger.info("Contexts fetched", { count: sortedList.length, current: contextToUse }, 'k8s');

            // Load saved namespaces for this context (will be overwritten by data loading effect)
            const savedState = loadContextState(contextToUse);
            if (savedState.namespaces && savedState.namespaces.length > 0) {
                setSelectedNamespaces(savedState.namespaces);
                Logger.debug("Restored namespaces from saved state", { namespaces: savedState.namespaces }, 'k8s');
            }
            // Connection test happens in the data loading effect when currentContext changes
        } catch (err: any) {
            Logger.error("Failed to fetch contexts", err, 'k8s');
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
    const refreshContextsIfChanged = useCallback(async (): Promise<void> => {
        try {
            const list: string[] = await ListContexts();
            const sortedList = (list || []).sort((a, b) => a.localeCompare(b));

            // Only update if the list actually changed
            setContexts(prev => {
                const lengthChanged = prev.length !== sortedList.length;
                const contentChanged = sortedList.some((ctx, i) => ctx !== prev[i]);
                if (lengthChanged || contentChanged) {
                    Logger.debug("Kubeconfig contexts changed", { previous: prev, current: sortedList }, 'k8s');
                    return sortedList;
                }
                return prev;
            });
        } catch (err: any) {
            Logger.error("Failed to refresh contexts", err, 'k8s');
        }
    }, []);

    const fetchNamespaces = useCallback(async (): Promise<void> => {
        if (!currentContext) return;
        try {
            Logger.debug("Fetching namespaces...", { context: currentContext }, 'k8s');
            const list: Namespace[] = await ListNamespaces(currentContext);
            // Extract namespace names from objects
            const namespaceNames = (list || []).map((ns: any) => ns.metadata?.name || ns).filter(Boolean) as string[];
            // Prepend "All Namespaces" option (empty string value)
            const namespacesWithAll = ['', ...namespaceNames];
            setNamespaces(namespacesWithAll);
            Logger.info("Namespaces fetched", { count: namespaceNames.length }, 'k8s');
            // Note: Don't clear connectionError here - other API calls might still be failing.
            // Error is only cleared when a watcher successfully connects.
            setIsConnecting(false);
        } catch (err: any) {
            Logger.error("Failed to fetch namespaces", err, 'k8s');
            // Set connection error - this is the first actual cluster call
            const parsed = parseConnectionError(err);
            setConnectionError({
                ...parsed,
                raw: String(err)
            });
            setIsConnecting(false);
        }
    }, [currentContext]);

    const switchContext = useCallback(async (newContext: string): Promise<void> => {
        // No-op when clicking the same context we're already on.
        // Without this guard, watchers are stopped but the useEffect keyed on
        // currentContext won't re-run (value unchanged), leaving the UI stuck
        // in an infinite "connecting" state.
        if (newContext === currentContext) return;

        try {
            Logger.info("Switching context...", { from: currentContext, to: newContext }, 'k8s');

            // Update ref IMMEDIATELY so any in-flight requests see the new context
            // and ignore their stale results. This must happen before any async calls.
            currentContextRef.current = newContext;

            // Clear previous errors and show connecting state
            setConnectionError(null);
            setIsConnecting(true);

            // Clear state immediately to prevent stale data display
            setNamespaces([]);
            setSelectedNamespaces([]);

            // Set loading flag to prevent saves during switch
            setIsLoadingNamespaces(true);

            // This cancels pending connection tests and switches context
            await SwitchContext(newContext);

            // Update UI state - connection test happens in the data loading effect
            setCurrentContext(newContext);
            updateContextAccessTime(newContext);
            localStorage.setItem('kubikles_last_context', newContext);
            Logger.debug("Context switched, data loading will follow", { context: newContext }, 'k8s');
        } catch (err: any) {
            Logger.error("Failed to switch context", err, 'k8s');
            setIsConnecting(false);
            setIsLoadingNamespaces(false);
        }
    }, [currentContext, updateContextAccessTime]);

    // Initial Load
    useEffect(() => {
        // Check for K8s client initialization errors first
        const checkInitError = async (): Promise<void> => {
            try {
                const initError: string = await GetK8sInitError();
                if (initError) {
                    Logger.error("K8s client initialization failed", { error: initError }, 'k8s');
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
            } catch (err: any) {
                Logger.error("Failed to check K8s init error", err, 'k8s');
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

        const loadNamespacesAndRestoreState = async (): Promise<void> => {
            setIsLoadingNamespaces(true);

            // Quick connectivity check - fail fast if cluster unreachable
            const connectionTimeout = getConnectionTestTimeout();
            Logger.debug("Testing connection to cluster...", { context: contextForThisEffect, timeoutSeconds: connectionTimeout }, 'k8s');
            try {
                await TestConnection(connectionTimeout);
                // Check if context changed while we were waiting
                if (cancelled || currentContextRef.current !== contextForThisEffect) {
                    Logger.debug("Connection test completed but context changed, ignoring", {
                        testedContext: contextForThisEffect,
                        currentContext: currentContextRef.current
                    }, 'k8s');
                    return;
                }
                Logger.info("Connection test passed", { context: contextForThisEffect }, 'k8s');
            } catch (connErr) {
                // Check if context changed while we were waiting
                if (cancelled || currentContextRef.current !== contextForThisEffect) {
                    Logger.debug("Connection test failed but context changed, ignoring", {
                        testedContext: contextForThisEffect,
                        currentContext: currentContextRef.current
                    }, 'k8s');
                    return;
                }
                Logger.error("Connection test failed", { context: contextForThisEffect, error: connErr }, 'k8s');
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
                Logger.debug("Namespace fetch completed but context changed, ignoring", undefined, 'k8s');
                return;
            }

            // After namespaces are loaded, restore saved state
            const savedState = loadContextState(contextForThisEffect);
            if (savedState.namespaces && savedState.namespaces.length > 0) {
                setSelectedNamespaces(savedState.namespaces);
                Logger.debug("Restored namespaces after context switch", { namespaces: savedState.namespaces }, 'k8s');
            }
            setIsLoadingNamespaces(false);
            setIsConnecting(false);

            // Start port forwards with AutoStart=true for this context
            try {
                await StartAutoStartPortForwards(contextForThisEffect);
                Logger.debug("Started auto-start port forwards", { context: contextForThisEffect }, 'k8s');
            } catch (err: any) {
                Logger.error("Failed to start port forwards", err, 'k8s');
            }
        };

        loadNamespacesAndRestoreState();
        localStorage.setItem('kubikles_context', currentContext);

        // Cleanup: mark this effect as cancelled if context changes
        return () => {
            cancelled = true;
        };
    }, [currentContext, retryToken]);

    // Save namespaces when they change (but not while loading)
    useEffect(() => {
        if (currentContext && !isLoadingNamespaces) {
            saveContextState(currentContext, selectedNamespaces);
            Logger.debug("Namespaces changed", { context: currentContext, namespaces: selectedNamespaces }, 'k8s');
        }
    }, [currentContext, selectedNamespaces, isLoadingNamespaces]);

    // Listen for watcher error and status events
    useEffect(() => {
        if (!(window as any).runtime) return;

        const handleWatcherError = (event: WatcherErrorEvent): void => {
            const { resourceType, namespace, error, recoverable, context } = event;

            // Ignore errors from a different context (stale events after context switch)
            if (context && context !== currentContextRef.current) {
                Logger.debug("Ignoring stale watcher error from old context", { context, currentContext: currentContextRef.current }, 'k8s');
                return;
            }

            Logger.warn("Watcher error received", { resourceType, namespace, error, recoverable }, 'k8s');

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

        const handleWatcherStatus = (event: WatcherStatusEvent): void => {
            const { resourceType, namespace, status, context } = event;

            // Ignore status from a different context (stale events after context switch)
            if (context && context !== currentContextRef.current) {
                Logger.debug("Ignoring stale watcher status from old context", { context, currentContext: currentContextRef.current }, 'k8s');
                return;
            }

            Logger.debug("Watcher status changed", { resourceType, namespace, status }, 'k8s');

            // Detect reconnection: if a watcher transitions reconnecting → connected,
            // the watch restarted from "now" after resourceVersion expired, so any
            // resources deleted during the gap are now ghosts. Trigger a full re-fetch
            // to reconcile frontend state. Debounce so multiple watchers reconnecting
            // simultaneously only trigger one refresh cycle.
            const prevStatus = watcherPrevStatusRef.current[resourceType];
            watcherPrevStatusRef.current[resourceType] = status;
            if (prevStatus === 'reconnecting' && status === 'connected') {
                if (reconnectRefreshTimerRef.current) clearTimeout(reconnectRefreshTimerRef.current);
                reconnectRefreshTimerRef.current = setTimeout(() => {
                    Logger.info("Watcher reconnected after error, triggering silent reconciliation", undefined, 'k8s');
                    setReconcileToken(t => t + 1);
                    reconnectRefreshTimerRef.current = null;
                }, 500);
            }

            // Clear connection error if a watcher successfully connects
            if (status === 'running' || status === 'connected') {
                setConnectionError(null);
                setIsConnecting(false);
            }
        };

        const cancelError = EventsOn("watcher-error", handleWatcherError);
        const cancelStatus = EventsOn("watcher-status", handleWatcherStatus);

        return () => {
            cancelError();
            cancelStatus();
        };
    }, []);

    // Keep context ref in sync for event handlers (avoids stale closures)
    useEffect(() => {
        currentContextRef.current = currentContext;
    }, [currentContext]);

    // Clear watcher reconnection tracking on context switch
    useEffect(() => {
        watcherPrevStatusRef.current = {};
        if (reconnectRefreshTimerRef.current) {
            clearTimeout(reconnectRefreshTimerRef.current);
            reconnectRefreshTimerRef.current = null;
        }
    }, [currentContext]);

    const triggerRefresh = useCallback((): void => {
        setLastRefresh(Date.now());
        Logger.debug("Triggered resource refresh", undefined, 'k8s');
    }, []);

    const retryConnection = useCallback((): void => {
        Logger.info("Retrying connection...", undefined, 'k8s');
        setConnectionError(null);
        setIsConnecting(true);
        setRetryToken(prev => prev + 1);
    }, []);

    // Check if an error is an auth/connection error and set connectionError if so
    const checkConnectionError = useCallback((error: unknown): boolean => {
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

    // Keep crds ref in sync for stable callback
    useEffect(() => { crdsRef.current = crds; }, [crds]);

    // Fetch CRDs lazily when needed (for owner reference resolution)
    // Only depends on currentContext — uses refs for crds to stay stable
    const ensureCRDsLoaded = useCallback(async (): Promise<CRD[]> => {
        if (!currentContext) return [];
        if (crdsLoadedForContext.current === currentContext && crdsRef.current.length > 0) {
            return crdsRef.current;
        }
        // Dedup: if a fetch is already in flight for this context, reuse it
        if (crdsFetchPromise.current?.context === currentContext) {
            return crdsFetchPromise.current.promise;
        }

        const fetchPromise = (async () => {
            try {
                setCRDsLoading(true);
                Logger.debug("Fetching CRDs...", undefined, 'k8s');
                const list: CRD[] = await ListCRDs();
                setCRDs(list || []);
                crdsRef.current = list || [];
                crdsLoadedForContext.current = currentContext;
                return list || [];
            } catch (err: any) {
                Logger.error("Failed to fetch CRDs", err, 'k8s');
                return [];
            } finally {
                setCRDsLoading(false);
                crdsFetchPromise.current = null;
            }
        })();
        crdsFetchPromise.current = { context: currentContext, promise: fetchPromise };
        return fetchPromise;
    }, [currentContext]);

    // Clear CRDs on context switch
    // Note: don't clear crdsFetchPromise — the context tag handles staleness
    useEffect(() => {
        setCRDs([]);
        crdsRef.current = [];
        crdsLoadedForContext.current = null;
    }, [currentContext]);

    const value: K8sContextValue = useMemo(() => ({
        contexts,
        sortedContexts,
        currentContext,
        namespaces,
        currentNamespace,
        selectedNamespaces,
        setSelectedNamespaces,
        switchContext,
        refreshContexts: fetchContexts,
        refreshContextsIfChanged,
        refreshNamespaces: fetchNamespaces,
        lastRefresh,
        triggerRefresh,
        reconcileToken,
        // CRD lookup for owner reference resolution
        crds,
        crdsLoading,
        ensureCRDsLoaded,
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
        reconcileToken,
        crds,
        crdsLoading,
        ensureCRDsLoaded,
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
