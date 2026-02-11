import { useState, useEffect, useCallback } from 'react';
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
 * Hook to fetch custom resource instances for a given CRD,
 * with real-time updates via CRD watcher subscription.
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
    const [resources, setResources] = useState<K8sResource[]>([]);
    const [loading, setLoading] = useState<boolean>(false);
    const [error, setError] = useState<Error | null>(null);
    const { namespaces: allNamespaces, lastRefresh } = useK8s();

    useEffect(() => {
        if (!currentContext || !isVisible || !group || !version || !resource) return;

        const fetchResources = async (): Promise<void> => {
            setLoading(true);
            try {
                if (!isNamespaced) {
                    // Cluster-scoped: fetch all
                    const list = await ListCustomResources(group, version, resource, '');
                    setResources(list || []);
                } else {
                    // Namespaced: use optimization logic
                    const optimized = optimizeNamespaceQuery(selectedNamespaces, allNamespaces);

                    if (optimized === null) {
                        // No namespaces selected - return empty
                        setResources([]);
                    } else if (optimized === '') {
                        // Fetch from all namespaces in a single query
                        const list = await ListCustomResources(group, version, resource, '');
                        setResources(list || []);
                    } else {
                        // Fetch from each namespace and merge results
                        const allResources = await Promise.all(
                            optimized.map((ns: string) => ListCustomResources(group, version, resource, ns).catch((err: Error) => {
                                console.error(`Failed to fetch custom resources from namespace ${ns}`, err);
                                return [];
                            }))
                        );
                        // Flatten and deduplicate by UID
                        const merged = allResources.flat();
                        const unique = merged.filter((item, index, self) =>
                            index === self.findIndex((r: any) => r.metadata?.uid === item.metadata?.uid)
                        );
                        setResources(unique);
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

    // Handle real-time watcher events
    const handleWatcherEvent = useCallback((event: any) => {
        const { type, resource: updatedResource } = event;
        if (!updatedResource?.metadata?.uid) return;

        // For namespaced resources, check namespace filtering
        if (isNamespaced && selectedNamespaces.length > 0) {
            const ns = updatedResource.metadata?.namespace;
            if (ns && !selectedNamespaces.includes(ns)) return;
        }

        setResources(prev => {
            const uid = updatedResource.metadata.uid;
            switch (type) {
                case 'ADDED': {
                    if (prev.find((r: any) => r.metadata?.uid === uid)) {
                        // Already exists, treat as modification
                        return prev.map((r: any) => r.metadata?.uid === uid ? updatedResource : r);
                    }
                    return [...prev, updatedResource];
                }
                case 'MODIFIED':
                    return prev.map((r: any) => r.metadata?.uid === uid ? updatedResource : r);
                case 'DELETED':
                    return prev.filter((r: any) => r.metadata?.uid !== uid);
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
