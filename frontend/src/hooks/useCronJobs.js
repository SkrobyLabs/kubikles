import { useState, useEffect, useCallback, useMemo } from 'react';
import { ListCronJobs } from '../../wailsjs/go/main/App';
import { useK8s } from '../context/K8sContext';
import { optimizeNamespaceQuery } from './useNamespaceOptimization';
import { useResourceWatcher } from './useResourceWatcher';
import { createNamespacedResourceEventHandler } from './useResourceEventHandler';

export function useCronJobs(context, namespaces, isVisible) {
    const [cronJobs, setCronJobs] = useState([]);
    const [loading, setLoading] = useState(true);
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
        if (!isVisible || !context || namespaces === null || namespaces === undefined) {
            return;
        }

        let isCancelled = false;

        const fetchCronJobs = async () => {
            setLoading(true);
            try {
                const optimized = optimizeNamespaceQuery(namespaces, allNamespaces);

                if (optimized === null) {
                    if (!isCancelled) {
                        setCronJobs([]);
                    }
                } else if (optimized === '') {
                    const data = await ListCronJobs('');
                    if (!isCancelled) {
                        setCronJobs(data || []);
                    }
                } else {
                    const allCronJobs = await Promise.all(
                        optimized.map(ns => ListCronJobs(ns).catch(err => {
                            console.error(`Failed to fetch cronjobs from namespace ${ns}`, err);
                            return [];
                        }))
                    );
                    if (!isCancelled) {
                        const merged = allCronJobs.flat();
                        const unique = merged.filter((cj, index, self) =>
                            index === self.findIndex(c => c.metadata.uid === cj.metadata.uid)
                        );
                        setCronJobs(unique);
                    }
                }
            } catch (err) {
                if (!isCancelled) {
                    console.error('Error fetching cron jobs:', err);
                    setCronJobs([]);
                }
            } finally {
                if (!isCancelled) {
                    setLoading(false);
                }
            }
        };

        fetchCronJobs();

        return () => {
            isCancelled = true;
        };
    }, [context, namespaces, isVisible, lastRefresh, allNamespaces]);

    // Create selected namespaces array for event filtering
    const selectedNamespacesList = useMemo(() => {
        if (!namespaces) return [];
        return Array.isArray(namespaces) ? namespaces : [namespaces];
    }, [namespaces]);

    // Subscribe to cronjob events
    const handleEvent = useCallback(
        createNamespacedResourceEventHandler(setCronJobs, selectedNamespacesList),
        [selectedNamespacesList]
    );

    useResourceWatcher(
        "cronjobs",
        optimizedNamespaces,
        handleEvent,
        context && isVisible && optimizedNamespaces.length > 0
    );

    return { cronJobs, loading };
}
