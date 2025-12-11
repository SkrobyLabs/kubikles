import { useState, useEffect, useCallback, useMemo } from 'react';
import { ListIngresses } from '../../wailsjs/go/main/App';
import { useK8s } from '../context/K8sContext';
import { optimizeNamespaceQuery } from './useNamespaceOptimization';
import { useResourceWatcher } from './useResourceWatcher';
import { createNamespacedResourceEventHandler } from './useResourceEventHandler';

export const useIngresses = (currentContext, namespaces, isVisible) => {
    const [ingresses, setIngresses] = useState([]);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState(null);
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
        if (!currentContext || namespaces === null || namespaces === undefined || !isVisible) return;

        const fetchIngresses = async () => {
            setLoading(true);
            try {
                const optimized = optimizeNamespaceQuery(namespaces, allNamespaces);

                if (optimized === null) {
                    setIngresses([]);
                } else if (optimized === '') {
                    const list = await ListIngresses('');
                    setIngresses(list || []);
                } else {
                    const allIngresses = await Promise.all(
                        optimized.map(ns => ListIngresses(ns).catch(err => {
                            console.error(`Failed to fetch ingresses from namespace ${ns}`, err);
                            return [];
                        }))
                    );
                    const merged = allIngresses.flat();
                    const unique = merged.filter((ing, index, self) =>
                        index === self.findIndex(i => i.metadata.uid === ing.metadata.uid)
                    );
                    setIngresses(unique);
                }
                setError(null);
            } catch (err) {
                console.error("Failed to fetch ingresses", err);
                setError(err);
            } finally {
                setLoading(false);
            }
        };

        fetchIngresses();
    }, [currentContext, namespaces, isVisible, allNamespaces, lastRefresh]);

    // Create selected namespaces array for event filtering
    const selectedNamespacesList = useMemo(() => {
        if (!namespaces) return [];
        return Array.isArray(namespaces) ? namespaces : [namespaces];
    }, [namespaces]);

    // Subscribe to ingress events
    const handleEvent = useCallback(
        createNamespacedResourceEventHandler(setIngresses, selectedNamespacesList),
        [selectedNamespacesList]
    );

    useResourceWatcher(
        "ingresses",
        optimizedNamespaces,
        handleEvent,
        currentContext && isVisible && optimizedNamespaces.length > 0
    );

    return { ingresses, loading, error };
};
