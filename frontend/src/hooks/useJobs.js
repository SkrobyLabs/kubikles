import { useState, useEffect, useCallback, useMemo } from 'react';
import { ListJobs } from '../../wailsjs/go/main/App';
import { useK8s } from '../context/K8sContext';
import { optimizeNamespaceQuery } from './useNamespaceOptimization';
import { useResourceWatcher } from './useResourceWatcher';
import { createNamespacedResourceEventHandler } from './useResourceEventHandler';

export function useJobs(currentContext, namespaces, isVisible = true) {
    const [jobs, setJobs] = useState([]);
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

        const fetchJobs = async () => {
            setLoading(true);
            try {
                const optimized = optimizeNamespaceQuery(namespaces, allNamespaces);

                if (optimized === null) {
                    setJobs([]);
                } else if (optimized === '') {
                    const result = await ListJobs('');
                    setJobs(result || []);
                } else {
                    const allJobs = await Promise.all(
                        optimized.map(ns => ListJobs(ns).catch(err => {
                            console.error(`Failed to fetch jobs from namespace ${ns}`, err);
                            return [];
                        }))
                    );
                    const merged = allJobs.flat();
                    const unique = merged.filter((job, index, self) =>
                        index === self.findIndex(j => j.metadata.uid === job.metadata.uid)
                    );
                    setJobs(unique);
                }
            } catch (err) {
                console.error("Failed to fetch jobs:", err);
                setJobs([]);
            } finally {
                setLoading(false);
            }
        };

        fetchJobs();
    }, [currentContext, namespaces, lastRefresh, isVisible, allNamespaces]);

    // Create selected namespaces array for event filtering
    const selectedNamespacesList = useMemo(() => {
        if (!namespaces) return [];
        return Array.isArray(namespaces) ? namespaces : [namespaces];
    }, [namespaces]);

    // Subscribe to job events
    const handleEvent = useCallback(
        createNamespacedResourceEventHandler(setJobs, selectedNamespacesList),
        [selectedNamespacesList]
    );

    useResourceWatcher(
        "jobs",
        optimizedNamespaces,
        handleEvent,
        currentContext && isVisible && optimizedNamespaces.length > 0
    );

    return { jobs, loading };
}
