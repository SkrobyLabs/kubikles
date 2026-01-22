import { useState, useEffect, useCallback, useMemo, useRef } from 'react';
import { useK8s } from '../context/K8sContext';
import { optimizeNamespaceQuery } from './useNamespaceOptimization';
import { useResourceWatcher } from './useResourceWatcher';
import { createResourceEventHandler, createNamespacedResourceEventHandler } from './useResourceEventHandler';
import { CancelListRequest } from '../../wailsjs/go/main/App';

/**
 * Factory function to create a hook for namespaced Kubernetes resources.
 * Handles namespace optimization, multi-namespace fetching, and real-time updates.
 *
 * @param {string} resourceType - The K8s resource type (e.g., 'pods', 'deployments')
 * @param {Function} listFn - The Wails function to list resources (e.g., ListPods)
 * @param {string} stateName - The name for the state variable (e.g., 'pods', 'deployments')
 * @returns {Function} A React hook for the resource
 */
export function createNamespacedResourceHook(resourceType, listFn, stateName) {
    return function useNamespacedResource(currentContext, selectedNamespaces, isVisible) {
        const [data, setData] = useState([]);
        const [loading, setLoading] = useState(false);
        const [error, setError] = useState(null);
        const { namespaces: allNamespaces, lastRefresh, checkConnectionError } = useK8s();
        const requestIdRef = useRef(null);

        // Calculate optimized namespaces for watching
        const optimizedNamespaces = useMemo(() => {
            const optimized = optimizeNamespaceQuery(selectedNamespaces, allNamespaces);
            if (optimized === null) return [];
            if (optimized === '') return ['']; // Watch all namespaces
            return optimized;
        }, [selectedNamespaces, allNamespaces]);

        // Fetch initial list with cancellation support
        useEffect(() => {
            if (!currentContext || selectedNamespaces === null || selectedNamespaces === undefined || !isVisible) return;

            // Track if this effect instance is still current
            let isCancelled = false;

            // Generate request ID based on resource type and namespaces
            const nsKey = Array.isArray(selectedNamespaces)
                ? selectedNamespaces.sort().join(',') || 'all'
                : selectedNamespaces || 'all';
            const newRequestId = `list-${resourceType}-${nsKey}`;

            // Cancel previous request if it's different
            if (requestIdRef.current && requestIdRef.current !== newRequestId) {
                CancelListRequest(requestIdRef.current).catch(() => {});
            }
            requestIdRef.current = newRequestId;

            const fetchData = async () => {
                setLoading(true);
                try {
                    const optimized = optimizeNamespaceQuery(selectedNamespaces, allNamespaces);

                    if (optimized === null) {
                        if (!isCancelled) setData([]);
                    } else if (optimized === '') {
                        const list = await listFn(newRequestId, '');
                        if (!isCancelled) setData(list || []);
                    } else {
                        const allResults = await Promise.all(
                            optimized.map(ns => listFn(newRequestId, ns).catch(err => {
                                // Don't log cancelled request errors
                                if (!err?.message?.includes('cancelled')) {
                                    console.error(`Failed to fetch ${resourceType} from namespace ${ns}`, err);
                                }
                                return [];
                            }))
                        );
                        // Check cancellation after all async operations complete
                        if (isCancelled) return;

                        // O(n) deduplication using Map instead of O(n²) filter/findIndex
                        const merged = allResults.flat();
                        const seen = new Map();
                        for (const item of merged) {
                            const uid = item.metadata?.uid;
                            if (uid && !seen.has(uid)) {
                                seen.set(uid, item);
                            }
                        }
                        setData(Array.from(seen.values()));
                    }
                    if (!isCancelled) setError(null);
                } catch (err) {
                    // Don't show error for cancelled requests
                    if (!isCancelled && !err?.message?.includes('cancelled')) {
                        console.error(`Failed to fetch ${resourceType}`, err);
                        setError(err);
                        // Check if this is a connection/auth error
                        checkConnectionError(err);
                    }
                } finally {
                    if (!isCancelled) setLoading(false);
                }
            };

            fetchData();

            // Cleanup: mark as cancelled and cancel backend request
            return () => {
                isCancelled = true;
                if (requestIdRef.current) {
                    CancelListRequest(requestIdRef.current).catch(() => {});
                }
            };
        }, [currentContext, selectedNamespaces, isVisible, allNamespaces, lastRefresh, checkConnectionError]);

        // Create selected namespaces array for event filtering
        const selectedNamespacesList = useMemo(() => {
            if (!selectedNamespaces) return [];
            return Array.isArray(selectedNamespaces) ? selectedNamespaces : [selectedNamespaces];
        }, [selectedNamespaces]);

        // Subscribe to resource events
        const handleEvent = useCallback(
            createNamespacedResourceEventHandler(setData, selectedNamespacesList),
            [selectedNamespacesList]
        );

        useResourceWatcher(
            resourceType,
            optimizedNamespaces,
            handleEvent,
            currentContext && isVisible && optimizedNamespaces.length > 0
        );

        // Return with dynamic key name for backwards compatibility
        return { [stateName]: data, loading, error, [`set${stateName.charAt(0).toUpperCase() + stateName.slice(1)}`]: setData };
    };
}

/**
 * Factory function to create a hook for cluster-scoped Kubernetes resources.
 * These resources don't have namespaces (e.g., nodes, PVs, StorageClasses).
 *
 * @param {string} resourceType - The K8s resource type for watching (e.g., 'nodes', 'persistentvolumes')
 * @param {Function} listFn - The Wails function to list resources
 * @param {string} stateName - The name for the state variable
 * @returns {Function} A React hook for the resource
 */
export function createClusterScopedResourceHook(resourceType, listFn, stateName) {
    return function useClusterScopedResource(currentContext, isVisible) {
        const [data, setData] = useState([]);
        const [loading, setLoading] = useState(false);
        const [error, setError] = useState(null);
        const { lastRefresh, checkConnectionError } = useK8s();
        const requestIdRef = useRef(null);

        // Generate request ID for cluster-scoped resources
        const requestId = `list-${resourceType}-cluster`;

        useEffect(() => {
            if (!currentContext || !isVisible) return;

            let isCancelled = false;

            // Cancel previous request if needed
            if (requestIdRef.current && requestIdRef.current !== requestId) {
                CancelListRequest(requestIdRef.current).catch(() => {});
            }
            requestIdRef.current = requestId;

            const fetchData = async () => {
                setLoading(true);
                try {
                    const list = await listFn(requestId);
                    if (!isCancelled) {
                        setData(list || []);
                        setError(null);
                    }
                } catch (err) {
                    // Don't show error for cancelled requests
                    if (!isCancelled && !err?.message?.includes('cancelled')) {
                        console.error(`Failed to fetch ${resourceType}`, err);
                        setError(err);
                        // Check if this is a connection/auth error
                        checkConnectionError(err);
                    }
                } finally {
                    if (!isCancelled) setLoading(false);
                }
            };

            fetchData();

            return () => {
                isCancelled = true;
                if (requestIdRef.current) {
                    CancelListRequest(requestIdRef.current).catch(() => {});
                }
            };
        }, [currentContext, isVisible, lastRefresh, requestId, checkConnectionError]);

        const refetch = useCallback(async () => {
            if (!currentContext || !isVisible) return;
            setLoading(true);
            try {
                const list = await listFn(requestId);
                setData(list || []);
                setError(null);
            } catch (err) {
                if (!err?.message?.includes('cancelled')) {
                    console.error(`Failed to fetch ${resourceType}`, err);
                    setError(err);
                    checkConnectionError(err);
                }
            } finally {
                setLoading(false);
            }
        }, [currentContext, isVisible, requestId, checkConnectionError]);

        // Subscribe to resource events (cluster-scoped, so namespace = "")
        const handleEvent = useCallback(createResourceEventHandler(setData), []);
        useResourceWatcher(resourceType, "", handleEvent, currentContext && isVisible);

        return { [stateName]: data, loading, error, refetch };
    };
}
