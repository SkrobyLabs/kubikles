import { useState, useEffect, useCallback, useMemo } from 'react';
import { ListServices } from '../../wailsjs/go/main/App';
import { useK8s } from '../context/K8sContext';
import { optimizeNamespaceQuery } from './useNamespaceOptimization';
import { useResourceWatcher } from './useResourceWatcher';
import { createNamespacedResourceEventHandler } from './useResourceEventHandler';

export const useServices = (currentContext, namespaces, isVisible) => {
    const [services, setServices] = useState([]);
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

        const fetchServices = async () => {
            setLoading(true);
            try {
                const optimized = optimizeNamespaceQuery(namespaces, allNamespaces);

                if (optimized === null) {
                    setServices([]);
                } else if (optimized === '') {
                    const list = await ListServices('');
                    setServices(list || []);
                } else {
                    const allServices = await Promise.all(
                        optimized.map(ns => ListServices(ns).catch(err => {
                            console.error(`Failed to fetch services from namespace ${ns}`, err);
                            return [];
                        }))
                    );
                    const merged = allServices.flat();
                    const unique = merged.filter((svc, index, self) =>
                        index === self.findIndex(s => s.metadata.uid === svc.metadata.uid)
                    );
                    setServices(unique);
                }
                setError(null);
            } catch (err) {
                console.error("Failed to fetch services", err);
                setError(err);
            } finally {
                setLoading(false);
            }
        };

        fetchServices();
    }, [currentContext, namespaces, isVisible, allNamespaces, lastRefresh]);

    // Create selected namespaces array for event filtering
    const selectedNamespacesList = useMemo(() => {
        if (!namespaces) return [];
        return Array.isArray(namespaces) ? namespaces : [namespaces];
    }, [namespaces]);

    // Subscribe to service events
    const handleEvent = useCallback(
        createNamespacedResourceEventHandler(setServices, selectedNamespacesList),
        [selectedNamespacesList]
    );

    useResourceWatcher(
        "services",
        optimizedNamespaces,
        handleEvent,
        currentContext && isVisible && optimizedNamespaces.length > 0
    );

    return { services, loading, error };
};
