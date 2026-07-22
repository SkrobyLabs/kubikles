import React, { createContext, useContext, useState, useEffect, useCallback, useMemo, useRef } from 'react';
import { ListContexts, GetCurrentContext, SwitchContext, TestConnection, ListNamespaces, StartAutoStartPortForwards, ListCRDs, GetK8sInitError, SubscribeResourceWatcher, UnsubscribeWatcher, StopAllWatchers } from 'wailsjs/go/main/App';
import { EventsOn } from 'wailsjs/runtime/runtime';
import Logger from '../utils/Logger';
import { isImmediateWatchClosure, isStreamTransportError, isStreamingWarningDismissed, restoreConnectionMode, streamingWarningDismissalKey } from '../utils/streamingCompatibility';

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
    premature?: boolean;
    receivedAny?: boolean;
}

interface WatcherStatusEvent {
    resourceType: string;
    namespace?: string;
    status: 'running' | 'connected' | 'error' | 'stopped';
    context?: string;
}

interface ResourceEvent {
    type: 'ADDED' | 'MODIFIED' | 'DELETED';
    resourceType: string;
    namespace?: string;
    resource?: any;
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
    connectionMode: 'streaming' | 'polling';
    setConnectionMode: (mode: 'streaming' | 'polling') => void;
    streamingUnsupported: boolean;
    dismissStreamingWarning: () => void;
    reportStreamingFailure: (error: unknown) => void;
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

const normalizeNamespaceNames = (list: any[]): string[] => {
    const names = new Set<string>();

    for (const item of list || []) {
        const name = typeof item === 'string' ? item : item?.metadata?.name;
        if (name) names.add(name);
    }

    return ['', ...Array.from(names).sort((a, b) => a.localeCompare(b))];
};

const areStringArraysEqual = (a: string[], b: string[]): boolean => (
    a.length === b.length && a.every((value, index) => value === b[index])
);

const pruneSelectedNamespaces = (selected: string[], available: string[]): string[] => {
    const availableSet = new Set(available.filter((namespace) => namespace !== ''));
    return (selected || []).filter((namespace) => (
        namespace === '*' || namespace === '' || availableSet.has(namespace)
    ));
};

export const K8sProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
    const [contexts, setContexts] = useState<string[]>([]);
    const [currentContext, setCurrentContext] = useState<string>('');
    const currentContextRef = useRef<string>(''); // Ref for event handlers to avoid stale closures
    const [namespaces, setNamespaces] = useState<string[]>([]);
    const namespacesRef = useRef<string[]>([]);
    const [selectedNamespaces, setSelectedNamespaces] = useState<string[]>(['default']);
    const [lastRefresh, setLastRefresh] = useState<number>(Date.now());
    const [isLoadingNamespaces, setIsLoadingNamespaces] = useState<boolean>(false);
    const [reconcileToken, setReconcileToken] = useState(0);
    const watcherPrevStatusRef = useRef<Record<string, string>>({});
    const reconnectRefreshTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
    const namespaceRefreshPromiseRef = useRef<{ context: string; promise: Promise<string[] | void> } | null>(null);
    const [connectionError, setConnectionError] = useState<ConnectionError | null>(null); // { title, message, suggestion, provider, raw }
    const [isConnecting, setIsConnecting] = useState<boolean>(true); // Initial loading state
    const [retryToken, setRetryToken] = useState<number>(0); // Incremented to force data loading effect to re-run
    const [connectionMode, setConnectionModeState] = useState<'streaming' | 'polling'>('streaming');
    const [streamingUnsupported, setStreamingUnsupported] = useState(false);
    const streamingWarningDismissedRef = useRef(false);
    const watcherFailureCountRef = useRef<Record<string, number>>({});

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

    const setConnectionMode = useCallback((mode: 'streaming' | 'polling') => {
        setConnectionModeState(mode);
        setStreamingUnsupported(false);
        if (currentContextRef.current) {
            localStorage.setItem(`kubikles_connection_mode_${currentContextRef.current}`, mode);
        }
        if (mode === 'polling') StopAllWatchers().catch(() => {});
    }, []);

    const dismissStreamingWarning = useCallback(() => {
        setStreamingUnsupported(false);
        streamingWarningDismissedRef.current = true;
        if (currentContextRef.current) {
            localStorage.setItem(streamingWarningDismissalKey(currentContextRef.current), 'true');
        }
    }, []);

    const reportStreamingFailure = useCallback((error: unknown) => {
        if (connectionMode !== 'streaming') return;
        if (isStreamTransportError(error) && !streamingWarningDismissedRef.current) {
            setStreamingUnsupported(true);
        }
    }, [connectionMode]);

    useEffect(() => {
        const restoredMode = restoreConnectionMode(currentContext);
        setConnectionModeState(restoredMode);
        if (restoredMode === 'polling') StopAllWatchers().catch(() => {});
        streamingWarningDismissedRef.current = isStreamingWarningDismissed(currentContext);
        setStreamingUnsupported(false);
        watcherFailureCountRef.current = {};
    }, [currentContext]);

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

    const applyNamespaceOptions = useCallback((nextNamespaces: string[], reason: string): void => {
        namespacesRef.current = nextNamespaces;
        setNamespaces(prev => {
            if (areStringArraysEqual(prev, nextNamespaces)) {
                return prev;
            }

            Logger.debug("Namespace options changed", { reason, previous: prev, current: nextNamespaces }, 'k8s');
            return nextNamespaces;
        });

        setSelectedNamespaces(prev => {
            const pruned = pruneSelectedNamespaces(prev, nextNamespaces);
            return areStringArraysEqual(prev, pruned) ? prev : pruned;
        });
    }, []);

    const refreshNamespacesInternal = useCallback(async ({
        background = false,
        context = currentContext
    }: { background?: boolean; context?: string } = {}): Promise<string[] | void> => {
        if (!context) return;

        if (namespaceRefreshPromiseRef.current?.context === context) {
            return namespaceRefreshPromiseRef.current.promise;
        }

        let refreshPromise: Promise<string[] | void>;
        const runRefresh = async (): Promise<string[] | void> => {
            const contextForRequest = context;
            if (!background) {
                Logger.debug("Fetching namespaces...", { context: contextForRequest }, 'k8s');
            } else {
                Logger.debug("Refreshing namespaces in background...", { context: contextForRequest }, 'k8s');
            }

            try {
                const list: Namespace[] = await ListNamespaces(contextForRequest);

                if (currentContextRef.current !== contextForRequest) {
                    Logger.debug("Namespace refresh completed for stale context, ignoring", {
                        requestedContext: contextForRequest,
                        currentContext: currentContextRef.current
                    }, 'k8s');
                    return;
                }

                const nextNamespaces = normalizeNamespaceNames(list);
                applyNamespaceOptions(nextNamespaces, background ? 'background-refresh' : 'foreground-load');
                Logger.info("Namespaces refreshed", { count: nextNamespaces.length - 1, changedOnly: true, background }, 'k8s');

                if (!background) {
                    // Note: Don't clear connectionError here - other API calls might still be failing.
                    // Error is only cleared when a watcher successfully connects.
                    setIsConnecting(false);
                }
                return nextNamespaces;
            } catch (err: any) {
                Logger.error(background ? "Failed to refresh namespaces in background" : "Failed to fetch namespaces", err, 'k8s');
                if (!background) {
                    const parsed = parseConnectionError(err);
                    setConnectionError({
                        ...parsed,
                        raw: String(err)
                    });
                    setIsConnecting(false);
                }
            } finally {
                if (namespaceRefreshPromiseRef.current?.promise === refreshPromise) {
                    namespaceRefreshPromiseRef.current = null;
                }
            }
        };
        refreshPromise = runRefresh();

        namespaceRefreshPromiseRef.current = { context, promise: refreshPromise };
        return refreshPromise;
    }, [applyNamespaceOptions, currentContext]);

    const fetchNamespaces = useCallback(async (): Promise<void> => {
        await refreshNamespacesInternal({ background: true, context: currentContext });
    }, [currentContext, refreshNamespacesInternal]);

    useEffect(() => {
        if (!currentContext || connectionMode !== 'polling') return;
        let cancelled = false;
        let timer: number | undefined;
        const poll = async () => {
            await refreshNamespacesInternal({ background: true, context: currentContext });
            if (!cancelled) timer = window.setTimeout(poll, 9_000 + Math.random() * 2_000);
        };
        timer = window.setTimeout(poll, 9_000 + Math.random() * 2_000);
        return () => {
            cancelled = true;
            if (timer !== undefined) window.clearTimeout(timer);
        };
    }, [currentContext, connectionMode, refreshNamespacesInternal]);

    const loadNamespaces = useCallback(async (context: string): Promise<string[] | void> => {
        return refreshNamespacesInternal({ background: false, context });
    }, [refreshNamespacesInternal]);

    const applyNamespaceEvent = useCallback((event: ResourceEvent): void => {
        const namespaceName = typeof event.resource === 'string' ? event.resource : event.resource?.metadata?.name;
        if (!namespaceName) return;

        const previousNamespaces = namespacesRef.current;
        let nextNamespaces: string[];

        if (event.type === 'DELETED') {
            nextNamespaces = previousNamespaces.filter((namespace) => namespace !== namespaceName);
        } else {
            nextNamespaces = normalizeNamespaceNames([...previousNamespaces, namespaceName]);
        }

        if (areStringArraysEqual(previousNamespaces, nextNamespaces)) {
            return;
        }

        Logger.debug("Namespace watcher updated options", { type: event.type, namespace: namespaceName }, 'k8s');
        namespacesRef.current = nextNamespaces;
        setNamespaces(prev => {
            return areStringArraysEqual(prev, nextNamespaces) ? prev : nextNamespaces;
        });
        setSelectedNamespaces(selected => {
            const pruned = pruneSelectedNamespaces(selected, nextNamespaces);
            return areStringArraysEqual(selected, pruned) ? selected : pruned;
        });
    }, []);

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
            namespacesRef.current = [];
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

            const loadedNamespaces = await loadNamespaces(contextForThisEffect);

            // Check again after namespace fetch
            if (cancelled || currentContextRef.current !== contextForThisEffect) {
                Logger.debug("Namespace fetch completed but context changed, ignoring", undefined, 'k8s');
                return;
            }

            // After namespaces are loaded, restore saved state
            const savedState = loadContextState(contextForThisEffect);
            if (savedState.namespaces && savedState.namespaces.length > 0) {
                const availableNamespaces = Array.isArray(loadedNamespaces) ? loadedNamespaces : namespacesRef.current;
                const restoredNamespaces = pruneSelectedNamespaces(savedState.namespaces, availableNamespaces);
                setSelectedNamespaces(prev => areStringArraysEqual(prev, restoredNamespaces) ? prev : restoredNamespaces);
                Logger.debug("Restored namespaces after context switch", { namespaces: restoredNamespaces }, 'k8s');
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
    }, [currentContext, retryToken, loadNamespaces]);

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
            const { resourceType, namespace, error, recoverable, context, premature, receivedAny } = event;

            // Ignore errors from a different context (stale events after context switch)
            if (context && context !== currentContextRef.current) {
                Logger.debug("Ignoring stale watcher error from old context", { context, currentContext: currentContextRef.current }, 'k8s');
                return;
            }

            Logger.warn("Watcher error received", { resourceType, namespace, error, recoverable }, 'k8s');

            const errorStr = String(error);
            const streamError = isStreamTransportError(errorStr);
            if (isImmediateWatchClosure({ premature, receivedAny }) && connectionMode === 'streaming' && !streamingWarningDismissedRef.current) {
                setStreamingUnsupported(true);
            } else if (streamError && connectionMode === 'streaming') {
                const key = `${resourceType}:${namespace || ''}`;
                const failures = (watcherFailureCountRef.current[key] || 0) + 1;
                watcherFailureCountRef.current[key] = failures;
                if (failures >= 2 && !streamingWarningDismissedRef.current) setStreamingUnsupported(true);
            }

            // Check if this is an auth/connection error that should show the connection error UI
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
                watcherFailureCountRef.current[`${resourceType}:${namespace || ''}`] = 0;
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
    }, [connectionMode]);

    // Keep namespace selector options current with a cluster-scoped namespace watcher.
    useEffect(() => {
        if (!(window as any).runtime || !currentContext || connectionMode === 'polling') return;

        const contextForWatcher = currentContext;
        const subscribedKeys: string[] = [];
        let isMounted = true;

        const subscribe = async (): Promise<void> => {
            try {
                const key = await SubscribeResourceWatcher('namespaces', '');
                if (key && isMounted) {
                    subscribedKeys.push(key);
                } else if (key) {
                    UnsubscribeWatcher(key).catch(() => {});
                }
            } catch (err: any) {
                Logger.error("Failed to subscribe to namespace watcher", err, 'k8s');
            }
        };

        const handleEvent = (event: ResourceEvent): void => {
            if (!isMounted || event?.resourceType !== 'namespaces') return;
            if (event.context && event.context !== currentContextRef.current) {
                Logger.debug("Ignoring stale namespace event from old context", { context: event.context, currentContext: currentContextRef.current }, 'k8s');
                return;
            }
            if (contextForWatcher !== currentContextRef.current) {
                Logger.debug("Ignoring namespace event for inactive watcher context", { contextForWatcher, currentContext: currentContextRef.current }, 'k8s');
                return;
            }
            applyNamespaceEvent(event);
        };

        const handleBatchEvents = (events: ResourceEvent[]): void => {
            if (!isMounted || !Array.isArray(events)) return;
            for (const event of events) {
                handleEvent(event);
            }
        };

        subscribe();
        const cancelEvent = EventsOn("resource-event", handleEvent);
        const cancelBatch = EventsOn("resource-events-batch", handleBatchEvents);

        return () => {
            isMounted = false;
            cancelEvent();
            cancelBatch();
            subscribedKeys.forEach((key) => {
                UnsubscribeWatcher(key).catch((err: any) => {
                    Logger.error("Failed to unsubscribe namespace watcher", err, 'k8s');
                });
            });
        };
    }, [currentContext, applyNamespaceEvent, connectionMode]);

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
        connectionMode,
        setConnectionMode,
        streamingUnsupported,
        dismissStreamingWarning,
        reportStreamingFailure,
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
        connectionMode,
        setConnectionMode,
        streamingUnsupported,
        dismissStreamingWarning,
        reportStreamingFailure,
    ]);

    return (
        <K8sContext.Provider value={value}>
            {children}
        </K8sContext.Provider>
    );
};
