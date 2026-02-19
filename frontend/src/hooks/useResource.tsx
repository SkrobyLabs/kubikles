import { useState, useEffect, useCallback, useMemo, useRef } from 'react';
import { useK8s } from '../context';
import { optimizeNamespaceQuery } from './useNamespaceOptimization';
import { useResourceWatcher } from './useResourceWatcher';
import { createResourceEventHandler, createNamespacedResourceEventHandler } from './useResourceEventHandler';
import { CancelListRequest } from 'wailsjs/go/main/App';
import { EventsOn } from 'wailsjs/runtime/runtime';
import Logger from '../utils/Logger';

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
    stateName: string
) {
    return function useNamespacedResource(
        currentContext: string | null,
        selectedNamespaces: string | string[] | null | undefined,
        isVisible: boolean
    ): NamespacedResourceHookReturn<T> {
        const [dataMap, setDataMap] = useState<Map<string, T>>(new Map());
        const [loading, setLoading] = useState<boolean>(false);
        const [error, setError] = useState<Error | null>(null);
        const [loadingProgress, setLoadingProgress] = useState<LoadingProgress | null>(null);
        const { namespaces: allNamespaces, lastRefresh, checkConnectionError, reconcileToken } = useK8s();
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

        // Calculate optimized namespaces for watching
        const optimizedNamespaces = useMemo((): string[] => {
            const optimized = optimizeNamespaceQuery(selectedNamespaces || [], allNamespaces);
            if (optimized === null) return [];
            if (optimized === '') return ['']; // Watch all namespaces
            return optimized;
        }, [selectedNamespaces, allNamespaces]);

        // Fetch initial list with cancellation support
        useEffect(() => {
            if (!currentContext || selectedNamespaces === null || selectedNamespaces === undefined || !isVisible) return;

            // Prevent duplicate fetches from React StrictMode double-mount
            // Check if we're already fetching for the same query
            const queryKey = `${resourceType}-${createNamespaceKey(selectedNamespaces)}-${currentContext}`;
            if (fetchInProgressRef.current === queryKey) {
                return; // Already fetching for this exact query
            }

            // Track if this effect instance is still current
            let isCancelled = false;

            // Generate unique request ID for this effect instance
            // Using incrementing counter guarantees uniqueness even within same millisecond
            const newRequestId = createNamespacedRequestId(resourceType, selectedNamespaces);

            // Cancel previous request if it's different
            if (requestIdRef.current && requestIdRef.current !== newRequestId) {
                CancelListRequest(requestIdRef.current).catch(() => {});
            }
            requestIdRef.current = newRequestId;
            fetchInProgressRef.current = queryKey;

            const fetchData = async (): Promise<void> => {
                setLoading(true);
                let skipLoadingReset = false;
                try {
                    const optimized = optimizeNamespaceQuery(selectedNamespaces, allNamespaces);

                    if (optimized === null) {
                        if (!isCancelled) setDataMap(new Map());
                    } else if (optimized === '') {
                        const list = await listFn(newRequestId, '');
                        if (!isCancelled) setDataMap(arrayToMap(list || []));
                    } else {
                        const allResults = await Promise.all(
                            optimized.map((ns: any) => listFn(newRequestId, ns).catch((err: any) => {
                                // Don't log cancelled request errors
                                if (!err?.message?.includes('cancelled')) {
                                    console.error(`Failed to fetch ${resourceType} from namespace ${ns}`, err);
                                }
                                return [] as T[];
                            }))
                        );
                        // Check cancellation after all async operations complete
                        if (isCancelled) return;

                        // O(n) deduplication using Map
                        const merged = allResults.flat().filter(Boolean);
                        setDataMap(arrayToMap(merged));
                    }
                    if (!isCancelled) setError(null);
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
                    if (!isCancelled && !skipLoadingReset) setLoading(false);
                    // Clear fetch-in-progress flag when done
                    if (fetchInProgressRef.current === queryKey) {
                        fetchInProgressRef.current = null;
                    }
                }
            };

            fetchData();

            // Cleanup: mark as cancelled and cancel backend request
            return () => {
                isCancelled = true;
                // Clear fetch-in-progress flag on cleanup to allow re-fetch
                fetchInProgressRef.current = null;
                if (requestIdRef.current) {
                    CancelListRequest(requestIdRef.current).catch(() => {});
                }
            };
        }, [currentContext, selectedNamespaces, isVisible, allNamespaces, lastRefresh, checkConnectionError]);

        // Create selected namespaces array for event filtering
        const selectedNamespacesList = useMemo((): string[] => {
            if (!selectedNamespaces) return [];
            return Array.isArray(selectedNamespaces) ? selectedNamespaces : [selectedNamespaces];
        }, [selectedNamespaces]);

        // Subscribe to resource events
        const handleEvent = useCallback(
            createNamespacedResourceEventHandler(setDataMap as any, selectedNamespacesList),
            [selectedNamespacesList]
        );

        useResourceWatcher(
            resourceType,
            optimizedNamespaces,
            handleEvent as any,
            Boolean(currentContext && isVisible && optimizedNamespaces.length > 0)
        );

        // Silent reconciliation after watcher reconnection.
        // Only removes ghost resources (items deleted during disconnect window).
        // No loading flash, no scroll jump, no selection loss.
        useGhostReconciliation(
            resourceType, reconcileToken, setDataMap,
            listFn as any,
            currentContext, selectedNamespaces, allNamespaces, isVisible
        );

        // Return with dynamic key name for backwards compatibility
        const result: NamespacedResourceHookReturn<T> = {
            loading,
            error,
            loadingProgress,
            [stateName]: data
        };
        return result;
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
        const { lastRefresh, checkConnectionError, reconcileToken } = useK8s();
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

            const fetchData = async (): Promise<void> => {
                setLoading(true);
                let skipLoadingReset = false;
                try {
                    const list = await listFn(requestId);
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
                    if (!isCancelled && !skipLoadingReset) setLoading(false);
                    // Clear fetch-in-progress flag when done
                    if (fetchInProgressRef.current === queryKey) {
                        fetchInProgressRef.current = null;
                    }
                }
            };

            fetchData();

            return () => {
                isCancelled = true;
                // Clear fetch-in-progress flag on cleanup to allow re-fetch
                fetchInProgressRef.current = null;
                if (requestIdRef.current) {
                    CancelListRequest(requestIdRef.current).catch(() => {});
                }
            };
        }, [currentContext, isVisible, lastRefresh, checkConnectionError]);

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
                const optimized = optimizeNamespaceQuery(selectedNamespaces ?? [], allNamespaces);

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
                    const results = await Promise.all(
                        optimized.map(ns => listFn(requestId, ns).catch(() => [] as T[]))
                    );
                    freshItems = results.flat().filter(Boolean);
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
