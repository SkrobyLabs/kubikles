import { useState, useEffect, useCallback, useMemo, useRef } from 'react';
import { useK8s } from '../context';
import { optimizeNamespaceQuery } from './useNamespaceOptimization';
import { useResourceWatcher } from './useResourceWatcher';
import { createResourceEventHandler, createNamespacedResourceEventHandler } from './useResourceEventHandler';
import { CancelListRequest } from 'wailsjs/go/main/App';

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

// Hook return types for namespaced resources
interface NamespacedResourceHookReturn<T extends K8sResource> {
    loading: boolean;
    error: Error | null;
    [key: string]: T[] | boolean | Error | null | ((data: T[]) => void);
}

// Hook return types for cluster-scoped resources
interface ClusterScopedResourceHookReturn<T extends K8sResource> {
    loading: boolean;
    error: Error | null;
    refetch: () => Promise<void>;
    [key: string]: T[] | boolean | Error | null | (() => Promise<void>);
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
 * Factory function to create a hook for namespaced Kubernetes resources.
 * Handles namespace optimization, multi-namespace fetching, and real-time updates.
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
        const [data, setData] = useState<T[]>([]);
        const [loading, setLoading] = useState<boolean>(false);
        const [error, setError] = useState<Error | null>(null);
        const { namespaces: allNamespaces, lastRefresh, checkConnectionError } = useK8s();
        const requestIdRef = useRef<string | null>(null);
        const fetchInProgressRef = useRef<string | null>(null); // Prevent duplicate fetches from StrictMode

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
                        if (!isCancelled) setData([]);
                    } else if (optimized === '') {
                        const list = await listFn(newRequestId, '');
                        if (!isCancelled) setData(list || []);
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

                        // O(n) deduplication using Map instead of O(n²) filter/findIndex
                        const merged = allResults.flat().filter(Boolean);
                        const seen = new Map<string, T>();
                        for (const item of merged) {
                            const uid = item.metadata?.uid;
                            if (uid && !seen.has(uid)) {
                                seen.set(uid, item);
                            }
                        }
                        setData(Array.from(seen.values()));
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
            createNamespacedResourceEventHandler(setData as any, selectedNamespacesList),
            [selectedNamespacesList]
        );

        useResourceWatcher(
            resourceType,
            optimizedNamespaces,
            handleEvent as any,
            Boolean(currentContext && isVisible && optimizedNamespaces.length > 0)
        );

        // Return with dynamic key name for backwards compatibility
        const result: NamespacedResourceHookReturn<T> = {
            loading,
            error,
            [stateName]: data,
            [`set${stateName.charAt(0).toUpperCase() + stateName.slice(1)}`]: setData
        };
        return result;
    };
}

/**
 * Factory function to create a hook for cluster-scoped Kubernetes resources.
 * These resources don't have namespaces (e.g., nodes, PVs, StorageClasses).
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
        const [data, setData] = useState<T[]>([]);
        const [loading, setLoading] = useState<boolean>(false);
        const [error, setError] = useState<Error | null>(null);
        const { lastRefresh, checkConnectionError } = useK8s();
        const requestIdRef = useRef<string | null>(null);
        const fetchInProgressRef = useRef<string | null>(null); // Prevent duplicate fetches from StrictMode

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
                        setData(list || []);
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
                setData(list || []);
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
        const handleEvent = useCallback(createResourceEventHandler(setData as any), []);
        useResourceWatcher(resourceType, "", handleEvent as any, Boolean(currentContext && isVisible));

        const result: ClusterScopedResourceHookReturn<T> = {
            loading,
            error,
            refetch,
            [stateName]: data
        };
        return result;
    };
}
