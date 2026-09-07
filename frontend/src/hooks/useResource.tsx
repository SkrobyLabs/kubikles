import { useState, useEffect, useCallback, useMemo, useRef } from 'react';
import { useK8s } from '../context';
import { optimizeNamespaceQuery } from './useNamespaceOptimization';
import { useResourceWatcher } from './useResourceWatcher';
import { createResourceEventHandler } from './useResourceEventHandler';
import { CancelListRequest } from 'wailsjs/go/main/App';
import { EventsOn } from 'wailsjs/runtime/runtime';
import Logger from '../utils/Logger';
import { runWithPollingBudget } from '../utils/pollingBudget';
import { chooseNamespaceScope, namespaceSelection, observeNamespaceList, listNamespaceScope, isNamespaceForbidden, type NamespaceQueryObservation } from '../utils/namespaceQuery';

// K8s resource with metadata
interface K8sResource {
    metadata?: {
        uid?: string;
        [key: string]: any;
    };
    [key: string]: any;
}

// List function types
type NamespacedListFn<T extends K8sResource> = (requestId: string, namespace: string) => Promise<T[]>;
type ClusterScopedListFn<T extends K8sResource> = (requestId: string) => Promise<T[]>;

// Loading progress from paginated backend list
export interface LoadingProgress {
    loaded: number;
    total: number;
}

// Hook return types for namespaced resources
interface NamespacedResourceHookReturn<T extends K8sResource> {
    loading: boolean;
    error: Error | null;
    loadingProgress: LoadingProgress | null;
    [key: string]: T[] | boolean | Error | null | LoadingProgress | null | ((data: T[]) => void);
}

// Hook return types for cluster-scoped resources
interface ClusterScopedResourceHookReturn<T extends K8sResource> {
    loading: boolean;
    error: Error | null;
    refetch: () => Promise<void>;
    loadingProgress: LoadingProgress | null;
    [key: string]: T[] | boolean | Error | null | LoadingProgress | null | (() => Promise<void>);
}

// Global counter for unique request IDs - Date.now() can return same value within same millisecond
let requestCounter = 0;

/**
 * Creates a namespace key from selected namespaces for use in request IDs.
 * Sorts namespaces for stable keys.
 */
export function createNamespaceKey(selectedNamespaces: string | string[] | null | undefined): string {
    if (Array.isArray(selectedNamespaces)) {
        return [...selectedNamespaces].sort().join(',') || 'all';
    }
    return selectedNamespaces || 'all';
}

/**
 * Creates a unique request ID for namespaced resource list requests.
 * Uses incrementing counter to guarantee uniqueness even within same millisecond.
 */
export function createNamespacedRequestId(resourceType: string, selectedNamespaces: string | string[] | null | undefined): string {
    const nsKey = createNamespaceKey(selectedNamespaces);
    return `list-${resourceType}-${nsKey}-${++requestCounter}`;
}

/**
 * Creates a unique request ID for cluster-scoped resource list requests.
 * Uses incrementing counter to guarantee uniqueness even within same millisecond.
 */
export function createClusterScopedRequestId(resourceType: string): string {
    return `list-${resourceType}-cluster-${++requestCounter}`;
}

/**
 * Converts an array of K8s resources to a UID-keyed Map.
 */
function arrayToMap<T extends K8sResource>(items: T[]): Map<string, T> {
    const map = new Map<string, T>();
    for (const item of items) {
        const uid = item.metadata?.uid;
        if (uid) map.set(uid, item);
    }
    return map;
}

/**
 * Factory function to create a hook for namespaced Kubernetes resources.
 * Handles namespace optimization, multi-namespace fetching, and real-time updates.
 * Internally uses Map<string, T> for O(1) event processing, exposes T[] for consumers.
 */
export function createNamespacedResourceHook<T extends K8sResource>(
    resourceType: string,
    listFn: NamespacedListFn<T>,
    stateName: string,
    useWatcher = useResourceWatcher,
) {
    // Share lightweight measurements across mounts of this resource hook, never resource data.
    const observations = new Map<string, NamespaceQueryObservation>();
    const scopedLatency = new Map<string, number>();
    return function useNamespacedResource(
        currentContext: string | null,
        selectedNamespaces: string | string[] | null | undefined,
        isVisible: boolean
    ): NamespacedResourceHookReturn<T> {
        const dataKey = JSON.stringify([currentContext, resourceType]);
        const [snapshot, setSnapshot] = useState({ context: dataKey, scopeKey: '', map: new Map<string, T>() });
        const [loading, setLoading] = useState(false);
        const [error, setError] = useState<Error | null>(null);
        const [loadingProgress, setLoadingProgress] = useState<LoadingProgress | null>(null);
        const { namespaces: allNamespaces, lastRefresh, checkConnectionError, reconcileToken, connectionMode, registerPolling } = useK8s();
        const [broadDenied, setBroadDenied] = useState<string | null>(null);
        const loadedScope = useRef<{ context: string | null; key: string } | null>(null);
        const eventJournal = useRef<Map<string, any> | null>(null);
        const selectionKey = JSON.stringify(namespaceSelection(selectedNamespaces));
        const selected: string[] = useMemo(() => JSON.parse(selectionKey), [selectionKey]);
        const availableKey = JSON.stringify([...new Set(allNamespaces.filter(Boolean))].sort());
        // Explicit refresh retries permissions; polling does not repeatedly probe a denied scope.
        const permissionKey = JSON.stringify([dataKey, lastRefresh]);
        const scope = useMemo(() => chooseNamespaceScope(selected, JSON.parse(availableKey),
            observations.get(dataKey), {
                broadDenied: broadDenied === permissionKey,
                reuseBroad: loadedScope.current?.context === dataKey && loadedScope.current?.key === '[""]',
                scopedDurationMs: scopedLatency.get(dataKey),
            }), [selectionKey, availableKey, dataKey, broadDenied, permissionKey]);
        const scopeKey = JSON.stringify(scope);
        const broad = scope.length === 1 && scope[0] === '';
        const selectedSet = useMemo(() => new Set(selected), [selectionKey]);
        const selectionRef = useRef(selectedSet);
        selectionRef.current = selectedSet;
        const data = useMemo(() => {
            if (snapshot.context !== dataKey || !selected.length) return [];
            return Array.from(snapshot.map.values()).filter(item => selectedSet.has('*') || selectedSet.has(item.metadata?.namespace));
        }, [snapshot, dataKey, selectedSet]);

        useEffect(() => {
            if (!loading) return;
            const cancel = EventsOn('list-progress', (event: any) => {
                if (event?.resourceType === resourceType) setLoadingProgress({ loaded: event.loaded, total: event.total });
            });
            return () => { cancel(); setLoadingProgress(null); };
        }, [loading]);

        // Selection changes that retain the same fetch scope only update the local filter.
        useEffect(() => {
            if (!currentContext || !isVisible) return;
            let cancelled = false;
            let running = false;
            const activeRequests = new Set<string>();
            const namespaces: string[] = JSON.parse(scopeKey);
            const fetchData = async (): Promise<void> => {
                if (cancelled || running) return;
                running = true;
                const journal = new Map<string, any>();
                eventJournal.current = journal;
                const sameScope = loadedScope.current?.context === dataKey && loadedScope.current?.key === scopeKey;
                if (!sameScope) setLoading(true);
                try {
                    const started = performance.now();
                    const items = await listNamespaceScope(namespaces, async namespace => {
                        const requestId = createNamespacedRequestId(resourceType, namespace);
                        const request = async () => {
                            if (cancelled) return [];
                            activeRequests.add(requestId);
                            const requestStarted = performance.now();
                            try {
                                const result = await listFn(requestId, namespace);
                                if (namespace && !cancelled) {
                                    const duration = performance.now() - requestStarted;
                                    const previous = scopedLatency.get(dataKey);
                                    scopedLatency.set(dataKey, previous === undefined ? duration : previous * 0.75 + duration * 0.25);
                                    if (scopedLatency.size > 64) scopedLatency.delete(scopedLatency.keys().next().value!);
                                }
                                return result || [];
                            } finally {
                                activeRequests.delete(requestId);
                            }
                        };
                        return connectionMode === 'polling' ? runWithPollingBudget(request) : request();
                    }, () => !cancelled);
                    if (cancelled) return;
                    if (broad) {
                        observations.set(dataKey, observeNamespaceList(items, performance.now() - started));
                        if (observations.size > 64) observations.delete(observations.keys().next().value!);
                    }
                    let map = arrayToMap(items);
                    // Preserve updates/deletions received while the list was in flight.
                    const apply = createResourceEventHandler<T>(update => { map = typeof update === 'function' ? update(map) : update; });
                    for (const event of journal.values()) apply(event);
                    setSnapshot({ context: dataKey, scopeKey, map });
                    loadedScope.current = { context: dataKey, key: scopeKey };
                    setError(null);
                } catch (err) {
                    if (cancelled) return;
                    if (broad && !selectionRef.current.has('*') && isNamespaceForbidden(err)) {
                        setBroadDenied(permissionKey);
                    } else {
                        setError(err instanceof Error ? err : new Error(String(err)));
                        checkConnectionError(err);
                    }
                } finally {
                    if (eventJournal.current === journal) eventJournal.current = null;
                    running = false;
                    if (!cancelled) setLoading(false);
                }
            };
            void fetchData();
            const unregister = connectionMode === 'polling' ? registerPolling(fetchData) : undefined;
            return () => {
                cancelled = true;
                activeRequests.forEach(id => { void CancelListRequest(id).catch(() => {}); });
                unregister?.();
            };
        }, [currentContext, scopeKey, isVisible, lastRefresh, reconcileToken, connectionMode, registerPolling, checkConnectionError, resourceType, listFn]);

        const handleEvent = useCallback((event: any) => {
            if (event.context && event.context !== currentContext) return;
            const namespace = event.resource?.metadata?.namespace ?? event.namespace;
            if (!broad && !scope.includes(namespace)) return;
            const uid = event.resource?.metadata?.uid;
            if (!uid) return;
            eventJournal.current?.set(uid, event);
            setSnapshot(previous => {
                if (previous.context !== dataKey || previous.scopeKey !== scopeKey) return previous;
                let map = previous.map;
                createResourceEventHandler<T>(update => { map = typeof update === 'function' ? update(map) : update; })(event);
                return map === previous.map ? previous : { ...previous, map };
            });
        }, [dataKey, scopeKey]);

        const handleWatchError = useCallback((err: unknown, namespace: string) => {
            if (broad && namespace === '' && !selectedSet.has('*') && isNamespaceForbidden(err)) {
                setBroadDenied(permissionKey);
            } else {
                setError(err instanceof Error ? err : new Error(String(err)));
            }
        }, [broad, selectedSet, permissionKey]);
        useEffect(() => {
            if (!isVisible || !currentContext || connectionMode !== 'streaming') return;
            return EventsOn('watcher-error', (event: any) => {
                if (event?.resourceType !== resourceType || (event.context && event.context !== currentContext)) return;
                if (!scope.includes(event.namespace || '')) return;
                if (isNamespaceForbidden(event.error)) handleWatchError(event.error, event.namespace || '');
            });
        }, [currentContext, scopeKey, handleWatchError, isVisible, connectionMode]);
        useWatcher(resourceType, scope, handleEvent, Boolean(currentContext && isVisible && scope.length), handleWatchError);

        return { loading, error, loadingProgress, [stateName]: data };
    };
}

/**
 * Factory function to create a hook for cluster-scoped Kubernetes resources.
 * These resources don't have namespaces (e.g., nodes, PVs, StorageClasses).
 * Internally uses Map<string, T> for O(1) event processing, exposes T[] for consumers.
 */
export function createClusterScopedResourceHook<T extends K8sResource>(
    resourceType: string,
    listFn: ClusterScopedListFn<T>,
    stateName: string
) {
    return function useClusterScopedResource(
        currentContext: string | null,
        isVisible: boolean
    ): ClusterScopedResourceHookReturn<T> {
        const [dataMap, setDataMap] = useState<Map<string, T>>(new Map());
        const [loading, setLoading] = useState<boolean>(false);
        const [error, setError] = useState<Error | null>(null);
        const [loadingProgress, setLoadingProgress] = useState<LoadingProgress | null>(null);
        const { lastRefresh, checkConnectionError, reconcileToken, connectionMode, registerPolling } = useK8s();
        const requestIdRef = useRef<string | null>(null);
        const fetchInProgressRef = useRef<string | null>(null); // Prevent duplicate fetches from StrictMode

        // Derive array from map for consumers
        const data = useMemo(() => Array.from(dataMap.values()), [dataMap]);

        // Listen for paginated list progress events
        useEffect(() => {
            if (!loading) return;
            const cancel = EventsOn("list-progress", (event: any) => {
                if (event?.resourceType === resourceType) {
                    setLoadingProgress({ loaded: event.loaded, total: event.total });
                }
            });
            return () => { cancel(); setLoadingProgress(null); };
        }, [loading]);

        useEffect(() => {
            if (!currentContext || !isVisible) return;

            // Prevent duplicate fetches from React StrictMode double-mount
            const queryKey = `${resourceType}-cluster-${currentContext}`;
            if (fetchInProgressRef.current === queryKey) {
                return; // Already fetching for this exact query
            }

            let isCancelled = false;

            // Generate unique request ID for this effect instance
            const requestId = createClusterScopedRequestId(resourceType);

            // Cancel previous request if needed
            if (requestIdRef.current && requestIdRef.current !== requestId) {
                CancelListRequest(requestIdRef.current).catch(() => {});
            }
            requestIdRef.current = requestId;
            fetchInProgressRef.current = queryKey;

            let running = false;
            const fetchData = async (): Promise<void> => {
                if (running || isCancelled) return;
                running = true;
                setLoading(true);
                let skipLoadingReset = false;
                try {
                    const list = await (connectionMode === 'polling'
                        ? runWithPollingBudget(() => isCancelled ? Promise.resolve([]) : listFn(requestId))
                        : listFn(requestId));
                    if (!isCancelled) {
                        setDataMap(arrayToMap(list || []));
                        setError(null);
                    }
                } catch (err: any) {
                    // Don't show error for cancelled requests
                    const wasCancelledByBackend = (err as any)?.message?.includes('cancelled');
                    if (!isCancelled && !wasCancelledByBackend) {
                        console.error(`Failed to fetch ${resourceType}`, err);
                        setError(err instanceof Error ? err : new Error(String(err)));
                        // Check if this is a connection/auth error
                        checkConnectionError(err);
                    }
                    // If cancelled, don't reset loading - another request is likely pending
                    if (isCancelled || wasCancelledByBackend) {
                        skipLoadingReset = true;
                    }
                } finally {
                    running = false;
                    if (!isCancelled && !skipLoadingReset) setLoading(false);
                    // Clear fetch-in-progress flag when done
                    if (fetchInProgressRef.current === queryKey) {
                        fetchInProgressRef.current = null;
                    }
                }
            };

            fetchData();
            const unregister = connectionMode === 'polling' ? registerPolling(fetchData) : undefined;

            return () => {
                isCancelled = true;
                // Clear fetch-in-progress flag on cleanup to allow re-fetch
                fetchInProgressRef.current = null;
                if (requestIdRef.current) {
                    CancelListRequest(requestIdRef.current).catch(() => {});
                }
                unregister?.();
            };
        }, [currentContext, isVisible, lastRefresh, connectionMode, registerPolling, checkConnectionError]);

        const refetch = useCallback(async (): Promise<void> => {
            if (!currentContext || !isVisible) return;
            const refetchRequestId = createClusterScopedRequestId(resourceType);
            setLoading(true);
            try {
                const list = await listFn(refetchRequestId);
                setDataMap(arrayToMap(list || []));
                setError(null);
            } catch (err: any) {
                if (!(err as any)?.message?.includes('cancelled')) {
                    console.error(`Failed to fetch ${resourceType}`, err);
                    setError(err instanceof Error ? err : new Error(String(err)));
                    checkConnectionError(err);
                }
            } finally {
                setLoading(false);
            }
        }, [currentContext, isVisible, checkConnectionError]);

        // Subscribe to resource events (cluster-scoped, so namespace = "")
        const handleEvent = useCallback(createResourceEventHandler(setDataMap as any), []);
        useResourceWatcher(resourceType, "", handleEvent as any, Boolean(currentContext && isVisible));

        // Silent reconciliation after watcher reconnection (cluster-scoped variant)
        useGhostReconciliation(
            resourceType, reconcileToken, setDataMap,
            ((rid: string, _ns: string) => listFn(rid)) as any,
            currentContext, null, [], isVisible
        );

        const result: ClusterScopedResourceHookReturn<T> = {
            loading,
            error,
            refetch,
            loadingProgress,
            [stateName]: data
        };
        return result;
    };
}

/**
 * Silent ghost reconciliation after watcher reconnection.
 * When the watcher disconnects and reconnects, resources deleted during the gap
 * become "ghosts" — they exist in local state but not on the cluster.
 * This hook fetches fresh data and removes any items not present in the fresh list.
 *
 * Key properties:
 * - No loading state changes (no flash)
 * - Only removes ghosts; never replaces existing items (preserves selections, scroll)
 * - Skips initial mount (reconcileToken starts at 0)
 * - Silently ignores fetch errors (watcher will catch up)
 */
function useGhostReconciliation<T extends K8sResource>(
    resourceType: string,
    reconcileToken: number,
    setDataMap: React.Dispatch<React.SetStateAction<Map<string, T>>>,
    listFn: NamespacedListFn<T>,
    currentContext: string | null,
    selectedNamespaces: string | string[] | null | undefined,
    allNamespaces: string[],
    isVisible: boolean
): void {
    const prevTokenRef = useRef(reconcileToken);

    useEffect(() => {
        // Skip if token hasn't changed (including initial mount)
        if (reconcileToken === prevTokenRef.current) return;
        prevTokenRef.current = reconcileToken;

        if (!currentContext || !isVisible) return;

        let cancelled = false;

        const reconcile = async () => {
            Logger.debug(`[ghost-reconcile] Starting for ${resourceType} (token=${reconcileToken})`);

            try {
                const optimized = selectedNamespaces == null ? '' : optimizeNamespaceQuery(selectedNamespaces, allNamespaces);

                let freshItems: T[];
                const requestId = `reconcile-${resourceType}-${++requestCounter}`;

                if (optimized === null) {
                    // No valid namespaces selected — nothing to reconcile against
                    Logger.debug(`[ghost-reconcile] ${resourceType}: no namespaces, skipping`);
                    return;
                } else if (optimized === '') {
                    // All namespaces
                    freshItems = await listFn(requestId, '');
                } else {
                    freshItems = await listNamespaceScope(optimized, ns => listFn(`${requestId}-${ns}`, ns), () => !cancelled);
                }

                if (cancelled) return;

                // Build set of UIDs that exist on the cluster right now
                const freshUids = new Set<string>();
                for (const item of freshItems) {
                    const uid = item.metadata?.uid;
                    if (uid) freshUids.add(uid);
                }

                // Remove ghosts: items in local map whose UID is not in the fresh set
                setDataMap(prev => {
                    let removed = 0;
                    const next = new Map<string, T>();
                    for (const [uid, resource] of prev) {
                        if (freshUids.has(uid)) {
                            next.set(uid, resource);
                        } else {
                            removed++;
                        }
                    }
                    if (removed > 0) {
                        Logger.debug(`[ghost-reconcile] ${resourceType}: removed ${removed} ghost(s)`);
                        return next;
                    }
                    Logger.debug(`[ghost-reconcile] ${resourceType}: no ghosts found (${prev.size} items verified)`);
                    // Return same reference if nothing changed to avoid re-render
                    return prev;
                });
            } catch (err) {
                // Silent failure — watcher events will eventually correct state
                Logger.debug(`[ghost-reconcile] ${resourceType}: fetch failed, skipping`, err);
            }
        };

        reconcile();

        return () => { cancelled = true; };
    }, [reconcileToken, resourceType, currentContext, selectedNamespaces, allNamespaces, isVisible, listFn, setDataMap]);
}
