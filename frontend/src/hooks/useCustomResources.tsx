import { useState, useEffect, useCallback, useMemo } from 'react';
import { ListCustomResources } from 'wailsjs/go/main/App';
import { useK8s } from '../context';
import { optimizeNamespaceQuery } from './useNamespaceOptimization';
import { useCRDWatcher } from './useResourceWatcher';
import { K8sResource } from '../types/k8s';

interface UseCustomResourcesResult {
    resources: K8sResource[];
    loading: boolean;
    error: Error | null;
}

/**
 * Converts an array of K8s resources to a UID-keyed Map.
 */
function arrayToMap(items: K8sResource[]): Map<string, K8sResource> {
    const map = new Map<string, K8sResource>();
    for (const item of items) {
        const uid = item.metadata?.uid;
        if (uid) map.set(uid, item);
    }
    return map;
}

/**
 * Hook to fetch custom resource instances for a given CRD,
 * with real-time updates via CRD watcher subscription.
 * Uses Map<string, K8sResource> internally for O(1) event processing.
 */
export const useCustomResources = (
    currentContext: string | null,
    group: string,
    version: string,
    resource: string,
    selectedNamespaces: string[],
    isVisible: boolean,
    isNamespaced: boolean
): UseCustomResourcesResult => {
    const [resourceMap, setResourceMap] = useState<Map<string, K8sResource>>(new Map());
    const [loading, setLoading] = useState<boolean>(false);
    const [error, setError] = useState<Error | null>(null);
    const { namespaces: allNamespaces, lastRefresh } = useK8s();

    // Derive array from map for consumers
    const resources = useMemo(() => Array.from(resourceMap.values()), [resourceMap]);

    useEffect(() => {
        if (!currentContext || !isVisible || !group || !version || !resource) return;

        const fetchResources = async (): Promise<void> => {
            setLoading(true);
            try {
                if (!isNamespaced) {
                    // Cluster-scoped: fetch all
                    const list = await ListCustomResources('', group, version, resource, '');
                    setResourceMap(arrayToMap(list || []));
                } else {
                    // Namespaced: use optimization logic
                    const optimized = optimizeNamespaceQuery(selectedNamespaces, allNamespaces);

                    if (optimized === null) {
                        // No namespaces selected - return empty
                        setResourceMap(new Map());
                    } else if (optimized === '') {
                        // Fetch from all namespaces in a single query
                        const list = await ListCustomResources('', group, version, resource, '');
                        setResourceMap(arrayToMap(list || []));
                    } else {
                        // Fetch from each namespace and merge results
                        const allResources = await Promise.all(
                            optimized.map((ns: string) => ListCustomResources('', group, version, resource, ns).catch((err: Error) => {
                                console.error(`Failed to fetch custom resources from namespace ${ns}`, err);
                                return [];
                            }))
                        );
                        // Flatten and deduplicate via Map
                        setResourceMap(arrayToMap(allResources.flat()));
                    }
                }
                setError(null);
            } catch (err: any) {
                console.error("Failed to fetch custom resources", err);
                setError(err as Error);
            } finally {
                setLoading(false);
            }
        };

        fetchResources();
    }, [currentContext, group, version, resource, selectedNamespaces, isVisible, isNamespaced, allNamespaces, lastRefresh]);

    // Handle real-time watcher events with O(1) Map operations
    const handleWatcherEvent = useCallback((event: any) => {
        const { type, resource: updatedResource } = event;
        if (!updatedResource?.metadata?.uid) return;

        // For namespaced resources, check namespace filtering
        if (isNamespaced && selectedNamespaces.length > 0) {
            const ns = updatedResource.metadata?.namespace;
            if (ns && !selectedNamespaces.includes(ns)) return;
        }

        setResourceMap(prev => {
            const uid = updatedResource.metadata.uid;
            switch (type) {
                case 'ADDED': {
                    if (prev.has(uid)) {
                        // Already exists, treat as modification
                        const next = new Map(prev);
                        next.set(uid, updatedResource);
                        return next;
                    }
                    const next = new Map(prev);
                    next.set(uid, updatedResource);
                    return next;
                }
                case 'MODIFIED': {
                    const next = new Map(prev);
                    next.set(uid, updatedResource);
                    return next;
                }
                case 'DELETED': {
                    if (!prev.has(uid)) return prev;
                    const next = new Map(prev);
                    next.delete(uid);
                    return next;
                }
                default:
                    return prev;
            }
        });
    }, [isNamespaced, selectedNamespaces]);

    // Determine namespaces to watch
    const watchNamespaces = isNamespaced
        ? (selectedNamespaces.length > 0 ? selectedNamespaces : [''])
        : [''];

    // Subscribe to CRD watcher for real-time updates
    useCRDWatcher(
        group,
        version,
        resource,
        watchNamespaces,
        handleWatcherEvent,
        Boolean(currentContext && isVisible && group && version && resource)
    );

    return { resources, loading, error };
};
