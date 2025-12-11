import { useState, useEffect, useCallback, useMemo } from 'react';
import { ListConfigMaps } from '../../wailsjs/go/main/App';
import { useK8s } from '../context/K8sContext';
import { optimizeNamespaceQuery } from './useNamespaceOptimization';
import { useResourceWatcher } from './useResourceWatcher';
import { createNamespacedResourceEventHandler } from './useResourceEventHandler';

export const useConfigMaps = (currentContext, namespaces, isVisible) => {
    const [configMaps, setConfigMaps] = useState([]);
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

        const fetchConfigMaps = async () => {
            setLoading(true);
            try {
                const optimized = optimizeNamespaceQuery(namespaces, allNamespaces);

                if (optimized === null) {
                    setConfigMaps([]);
                } else if (optimized === '') {
                    const list = await ListConfigMaps('');
                    setConfigMaps(list || []);
                } else {
                    const allConfigMaps = await Promise.all(
                        optimized.map(ns => ListConfigMaps(ns).catch(err => {
                            console.error(`Failed to fetch configmaps from namespace ${ns}`, err);
                            return [];
                        }))
                    );
                    const merged = allConfigMaps.flat();
                    const unique = merged.filter((cm, index, self) =>
                        index === self.findIndex(c => c.metadata.uid === cm.metadata.uid)
                    );
                    setConfigMaps(unique);
                }
                setError(null);
            } catch (err) {
                console.error("Failed to fetch configmaps", err);
                setError(err);
            } finally {
                setLoading(false);
            }
        };

        fetchConfigMaps();
    }, [currentContext, namespaces, isVisible, allNamespaces, lastRefresh]);

    // Create selected namespaces array for event filtering
    const selectedNamespacesList = useMemo(() => {
        if (!namespaces) return [];
        return Array.isArray(namespaces) ? namespaces : [namespaces];
    }, [namespaces]);

    // Subscribe to configmap events
    const handleEvent = useCallback(
        createNamespacedResourceEventHandler(setConfigMaps, selectedNamespacesList),
        [selectedNamespacesList]
    );

    useResourceWatcher(
        "configmaps",
        optimizedNamespaces,
        handleEvent,
        currentContext && isVisible && optimizedNamespaces.length > 0
    );

    return { configMaps, loading, error };
};
