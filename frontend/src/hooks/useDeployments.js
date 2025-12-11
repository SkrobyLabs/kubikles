import { useState, useEffect, useCallback, useMemo } from 'react';
import { ListDeployments } from '../../wailsjs/go/main/App';
import { useK8s } from '../context/K8sContext';
import { optimizeNamespaceQuery } from './useNamespaceOptimization';
import { useResourceWatcher } from './useResourceWatcher';
import { createNamespacedResourceEventHandler } from './useResourceEventHandler';

export const useDeployments = (currentContext, selectedNamespaces, isVisible) => {
    const [deployments, setDeployments] = useState([]);
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

        const fetchDeployments = async () => {
            setLoading(true);
            try {
                const optimized = optimizeNamespaceQuery(selectedNamespaces, allNamespaces);

                if (optimized === null) {
                    setDeployments([]);
                } else if (optimized === '') {
                    const list = await ListDeployments('');
                    setDeployments(list || []);
                } else {
                    const allDeployments = await Promise.all(
                        optimized.map(ns => ListDeployments(ns).catch(err => {
                            console.error(`Failed to fetch deployments from namespace ${ns}`, err);
                            return [];
                        }))
                    );
                    const merged = allDeployments.flat();
                    const unique = merged.filter((dep, index, self) =>
                        index === self.findIndex(d => d.metadata.uid === dep.metadata.uid)
                    );
                    setDeployments(unique);
                }
                setError(null);
            } catch (err) {
                console.error("Failed to fetch deployments", err);
                setError(err);
            } finally {
                setLoading(false);
            }
        };

        fetchDeployments();
    }, [currentContext, selectedNamespaces, isVisible, allNamespaces, lastRefresh]);

    // Create selected namespaces array for event filtering
    const selectedNamespacesList = useMemo(() => {
        if (!selectedNamespaces) return [];
        return Array.isArray(selectedNamespaces) ? selectedNamespaces : [selectedNamespaces];
    }, [selectedNamespaces]);

    // Subscribe to deployment events
    const handleEvent = useCallback(
        createNamespacedResourceEventHandler(setDeployments, selectedNamespacesList),
        [selectedNamespacesList]
    );

    useResourceWatcher(
        "deployments",
        optimizedNamespaces,
        handleEvent,
        currentContext && isVisible && optimizedNamespaces.length > 0
    );

    return { deployments, loading, error, setDeployments };
};
