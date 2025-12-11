import { useState, useEffect, useCallback, useMemo } from 'react';
import { ListStatefulSets } from '../../wailsjs/go/main/App';
import { useK8s } from '../context/K8sContext';
import { optimizeNamespaceQuery } from './useNamespaceOptimization';
import { useResourceWatcher } from './useResourceWatcher';
import { createNamespacedResourceEventHandler } from './useResourceEventHandler';

export const useStatefulSets = (contextName, namespaces, isVisible) => {
    const [statefulSets, setStatefulSets] = useState([]);
    const [loading, setLoading] = useState(true);
    const { namespaces: allNamespaces, lastRefresh } = useK8s();

    // Calculate optimized namespaces for watching
    const optimizedNamespaces = useMemo(() => {
        const optimized = optimizeNamespaceQuery(namespaces, allNamespaces);
        if (optimized === null) return [];
        if (optimized === '') return ['']; // Watch all namespaces
        return optimized;
    }, [namespaces, allNamespaces]);

    // Fetch initial list
    useEffect(() => {
        if (!isVisible || namespaces === null || namespaces === undefined || !contextName) {
            setLoading(false);
            return;
        }

        const fetchStatefulSets = async () => {
            setLoading(true);
            try {
                const optimized = optimizeNamespaceQuery(namespaces, allNamespaces);

                if (optimized === null) {
                    setStatefulSets([]);
                } else if (optimized === '') {
                    const list = await ListStatefulSets(contextName, '');
                    setStatefulSets(list || []);
                } else {
                    const allStatefulSets = await Promise.all(
                        optimized.map(ns => ListStatefulSets(contextName, ns).catch(err => {
                            console.error(`Failed to fetch statefulsets from namespace ${ns}`, err);
                            return [];
                        }))
                    );
                    const merged = allStatefulSets.flat();
                    const unique = merged.filter((sts, index, self) =>
                        index === self.findIndex(s => s.metadata.uid === sts.metadata.uid)
                    );
                    setStatefulSets(unique);
                }
            } catch (err) {
                console.error('Failed to fetch statefulsets:', err);
                setStatefulSets([]);
            } finally {
                setLoading(false);
            }
        };

        fetchStatefulSets();
    }, [contextName, namespaces, isVisible, allNamespaces, lastRefresh]);

    // Create selected namespaces array for event filtering
    const selectedNamespacesList = useMemo(() => {
        if (!namespaces) return [];
        return Array.isArray(namespaces) ? namespaces : [namespaces];
    }, [namespaces]);

    // Subscribe to statefulset events
    const handleEvent = useCallback(
        createNamespacedResourceEventHandler(setStatefulSets, selectedNamespacesList),
        [selectedNamespacesList]
    );

    useResourceWatcher(
        "statefulsets",
        optimizedNamespaces,
        handleEvent,
        contextName && isVisible && optimizedNamespaces.length > 0
    );

    return { statefulSets, loading };
};
