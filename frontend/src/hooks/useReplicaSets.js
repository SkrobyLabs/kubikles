import { useState, useEffect, useCallback, useMemo } from 'react';
import { ListReplicaSets } from '../../wailsjs/go/main/App';
import { useK8s } from '../context/K8sContext';
import { optimizeNamespaceQuery } from './useNamespaceOptimization';
import { useResourceWatcher } from './useResourceWatcher';
import { createNamespacedResourceEventHandler } from './useResourceEventHandler';

export function useReplicaSets(currentContext, namespaces, isVisible = true) {
    const [replicaSets, setReplicaSets] = useState([]);
    const [loading, setLoading] = useState(false);
    const { lastRefresh, namespaces: allNamespaces } = useK8s();

    // Calculate optimized namespaces for watching
    const optimizedNamespaces = useMemo(() => {
        const optimized = optimizeNamespaceQuery(namespaces, allNamespaces);
        if (optimized === null) return [];
        if (optimized === '') return ['']; // Watch all namespaces
        return optimized;
    }, [namespaces, allNamespaces]);

    // Fetch initial list
    useEffect(() => {
        if (!isVisible || !currentContext || namespaces === null || namespaces === undefined) return;

        const fetchReplicaSets = async () => {
            setLoading(true);
            try {
                const optimized = optimizeNamespaceQuery(namespaces, allNamespaces);

                if (optimized === null) {
                    setReplicaSets([]);
                } else if (optimized === '') {
                    const result = await ListReplicaSets('');
                    setReplicaSets(result || []);
                } else {
                    const allReplicaSets = await Promise.all(
                        optimized.map(ns => ListReplicaSets(ns).catch(err => {
                            console.error(`Failed to fetch replicasets from namespace ${ns}`, err);
                            return [];
                        }))
                    );
                    const merged = allReplicaSets.flat();
                    const unique = merged.filter((rs, index, self) =>
                        index === self.findIndex(r => r.metadata.uid === rs.metadata.uid)
                    );
                    setReplicaSets(unique);
                }
            } catch (err) {
                console.error("Failed to fetch replicasets:", err);
                setReplicaSets([]);
            } finally {
                setLoading(false);
            }
        };

        fetchReplicaSets();
    }, [currentContext, namespaces, lastRefresh, isVisible, allNamespaces]);

    // Create selected namespaces array for event filtering
    const selectedNamespacesList = useMemo(() => {
        if (!namespaces) return [];
        return Array.isArray(namespaces) ? namespaces : [namespaces];
    }, [namespaces]);

    // Subscribe to replicaset events
    const handleEvent = useCallback(
        createNamespacedResourceEventHandler(setReplicaSets, selectedNamespacesList),
        [selectedNamespacesList]
    );

    useResourceWatcher(
        "replicasets",
        optimizedNamespaces,
        handleEvent,
        currentContext && isVisible && optimizedNamespaces.length > 0
    );

    return { replicaSets, loading };
}
