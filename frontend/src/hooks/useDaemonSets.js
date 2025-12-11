import { useState, useEffect, useCallback, useMemo } from 'react';
import { ListDaemonSets } from '../../wailsjs/go/main/App';
import { useK8s } from '../context/K8sContext';
import { optimizeNamespaceQuery } from './useNamespaceOptimization';
import { useResourceWatcher } from './useResourceWatcher';
import { createNamespacedResourceEventHandler } from './useResourceEventHandler';

export function useDaemonSets(currentContext, namespaces, isVisible = true) {
    const [daemonSets, setDaemonSets] = useState([]);
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

        const fetchDaemonSets = async () => {
            setLoading(true);
            try {
                const optimized = optimizeNamespaceQuery(namespaces, allNamespaces);

                if (optimized === null) {
                    setDaemonSets([]);
                } else if (optimized === '') {
                    const result = await ListDaemonSets('');
                    setDaemonSets(result || []);
                } else {
                    const allDaemonSets = await Promise.all(
                        optimized.map(ns => ListDaemonSets(ns).catch(err => {
                            console.error(`Failed to fetch daemonsets from namespace ${ns}`, err);
                            return [];
                        }))
                    );
                    const merged = allDaemonSets.flat();
                    const unique = merged.filter((ds, index, self) =>
                        index === self.findIndex(d => d.metadata.uid === ds.metadata.uid)
                    );
                    setDaemonSets(unique);
                }
            } catch (err) {
                console.error("Failed to fetch daemonsets:", err);
                setDaemonSets([]);
            } finally {
                setLoading(false);
            }
        };

        fetchDaemonSets();
    }, [currentContext, namespaces, lastRefresh, isVisible, allNamespaces]);

    // Create selected namespaces array for event filtering
    const selectedNamespacesList = useMemo(() => {
        if (!namespaces) return [];
        return Array.isArray(namespaces) ? namespaces : [namespaces];
    }, [namespaces]);

    // Subscribe to daemonset events
    const handleEvent = useCallback(
        createNamespacedResourceEventHandler(setDaemonSets, selectedNamespacesList),
        [selectedNamespacesList]
    );

    useResourceWatcher(
        "daemonsets",
        optimizedNamespaces,
        handleEvent,
        currentContext && isVisible && optimizedNamespaces.length > 0
    );

    return { daemonSets, loading };
}
