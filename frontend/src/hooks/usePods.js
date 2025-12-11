import { useState, useEffect, useCallback, useMemo } from 'react';
import { ListPods } from '../../wailsjs/go/main/App';
import { useK8s } from '../context/K8sContext';
import { optimizeNamespaceQuery } from './useNamespaceOptimization';
import { useResourceWatcher } from './useResourceWatcher';
import { createNamespacedResourceEventHandler } from './useResourceEventHandler';

export const usePods = (currentContext, selectedNamespaces, isVisible) => {
    const [pods, setPods] = useState([]);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState(null);
    const { namespaces: allNamespaces, lastRefresh } = useK8s();

    // Calculate optimized namespaces for watching
    const optimizedNamespaces = useMemo(() => {
        const optimized = optimizeNamespaceQuery(selectedNamespaces, allNamespaces);
        if (optimized === null) return [];
        if (optimized === '') return ['']; // Watch all namespaces
        return optimized;
    }, [selectedNamespaces, allNamespaces]);

    // Fetch initial list
    useEffect(() => {
        if (!currentContext || selectedNamespaces === null || selectedNamespaces === undefined || !isVisible) return;

        const fetchPods = async () => {
            setLoading(true);
            try {
                const optimized = optimizeNamespaceQuery(selectedNamespaces, allNamespaces);

                if (optimized === null) {
                    setPods([]);
                } else if (optimized === '') {
                    const list = await ListPods('');
                    setPods(list || []);
                } else {
                    const allPods = await Promise.all(
                        optimized.map(ns => ListPods(ns).catch(err => {
                            console.error(`Failed to fetch pods from namespace ${ns}`, err);
                            return [];
                        }))
                    );
                    const merged = allPods.flat();
                    const unique = merged.filter((pod, index, self) =>
                        index === self.findIndex(p => p.metadata.uid === pod.metadata.uid)
                    );
                    setPods(unique);
                }
                setError(null);
            } catch (err) {
                console.error("Failed to fetch pods", err);
                setError(err);
            } finally {
                setLoading(false);
            }
        };

        fetchPods();
    }, [currentContext, selectedNamespaces, isVisible, allNamespaces, lastRefresh]);

    // Create selected namespaces array for event filtering
    const selectedNamespacesList = useMemo(() => {
        if (!selectedNamespaces) return [];
        return Array.isArray(selectedNamespaces) ? selectedNamespaces : [selectedNamespaces];
    }, [selectedNamespaces]);

    // Subscribe to pod events
    const handleEvent = useCallback(
        createNamespacedResourceEventHandler(setPods, selectedNamespacesList),
        [selectedNamespacesList]
    );

    useResourceWatcher(
        "pods",
        optimizedNamespaces,
        handleEvent,
        currentContext && isVisible && optimizedNamespaces.length > 0
    );

    return { pods, loading, error, setPods };
};
