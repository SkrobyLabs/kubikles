import { useState, useEffect, useCallback, useMemo } from 'react';
import { ListPVCs } from '../../wailsjs/go/main/App';
import { useK8s } from '../context/K8sContext';
import { optimizeNamespaceQuery } from './useNamespaceOptimization';
import { useResourceWatcher } from './useResourceWatcher';
import { createNamespacedResourceEventHandler } from './useResourceEventHandler';

export const usePVCs = (currentContext, namespaces, isVisible) => {
    const [pvcs, setPVCs] = useState([]);
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

        const fetchPVCs = async () => {
            setLoading(true);
            try {
                const optimized = optimizeNamespaceQuery(namespaces, allNamespaces);

                if (optimized === null) {
                    setPVCs([]);
                } else if (optimized === '') {
                    const list = await ListPVCs('');
                    setPVCs(list || []);
                } else {
                    const allPVCs = await Promise.all(
                        optimized.map(ns => ListPVCs(ns).catch(err => {
                            console.error(`Failed to fetch PVCs from namespace ${ns}`, err);
                            return [];
                        }))
                    );
                    const merged = allPVCs.flat();
                    const unique = merged.filter((pvc, index, self) =>
                        index === self.findIndex(p => p.metadata.uid === pvc.metadata.uid)
                    );
                    setPVCs(unique);
                }
                setError(null);
            } catch (err) {
                console.error("Failed to fetch PVCs", err);
                setError(err);
            } finally {
                setLoading(false);
            }
        };

        fetchPVCs();
    }, [currentContext, namespaces, isVisible, allNamespaces, lastRefresh]);

    // Create selected namespaces array for event filtering
    const selectedNamespacesList = useMemo(() => {
        if (!namespaces) return [];
        return Array.isArray(namespaces) ? namespaces : [namespaces];
    }, [namespaces]);

    // Subscribe to PVC events
    const handleEvent = useCallback(
        createNamespacedResourceEventHandler(setPVCs, selectedNamespacesList),
        [selectedNamespacesList]
    );

    useResourceWatcher(
        "persistentvolumeclaims",
        optimizedNamespaces,
        handleEvent,
        currentContext && isVisible && optimizedNamespaces.length > 0
    );

    return { pvcs, loading, error };
};
