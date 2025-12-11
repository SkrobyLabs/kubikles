import { useState, useEffect, useCallback, useMemo } from 'react';
import { ListSecrets } from '../../wailsjs/go/main/App';
import { useK8s } from '../context/K8sContext';
import { optimizeNamespaceQuery } from './useNamespaceOptimization';
import { useResourceWatcher } from './useResourceWatcher';
import { createNamespacedResourceEventHandler } from './useResourceEventHandler';

export const useSecrets = (currentContext, namespaces, isVisible) => {
    const [secrets, setSecrets] = useState([]);
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

        const fetchSecrets = async () => {
            setLoading(true);
            try {
                const optimized = optimizeNamespaceQuery(namespaces, allNamespaces);

                if (optimized === null) {
                    // No namespaces selected - return empty
                    setSecrets([]);
                } else if (optimized === '') {
                    // Fetch from all namespaces in a single query (optimized)
                    const list = await ListSecrets('');
                    setSecrets(list || []);
                } else {
                    // Fetch from each namespace and merge results
                    const allSecrets = await Promise.all(
                        optimized.map(ns => ListSecrets(ns).catch(err => {
                            console.error(`Failed to fetch secrets from namespace ${ns}`, err);
                            return [];
                        }))
                    );
                    // Flatten and deduplicate by UID
                    const merged = allSecrets.flat();
                    const unique = merged.filter((sec, index, self) =>
                        index === self.findIndex(s => s.metadata.uid === sec.metadata.uid)
                    );
                    setSecrets(unique);
                }
                setError(null);
            } catch (err) {
                console.error("Failed to fetch secrets", err);
                setError(err);
            } finally {
                setLoading(false);
            }
        };

        fetchSecrets();
    }, [currentContext, namespaces, isVisible, allNamespaces, lastRefresh]);

    // Create selected namespaces array for event filtering
    const selectedNamespacesList = useMemo(() => {
        if (!namespaces) return [];
        return Array.isArray(namespaces) ? namespaces : [namespaces];
    }, [namespaces]);

    // Subscribe to secret events
    const handleEvent = useCallback(
        createNamespacedResourceEventHandler(setSecrets, selectedNamespacesList),
        [selectedNamespacesList]
    );

    useResourceWatcher(
        "secrets",
        optimizedNamespaces,
        handleEvent,
        currentContext && isVisible && optimizedNamespaces.length > 0
    );

    return { secrets, loading, error };
};
