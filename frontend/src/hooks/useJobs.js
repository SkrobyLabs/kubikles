import { useState, useEffect } from 'react';
import { ListJobs } from '../../wailsjs/go/main/App';
import { useK8s } from '../context/K8sContext';
import { optimizeNamespaceQuery } from './useNamespaceOptimization';

export function useJobs(currentContext, namespaces, isVisible = true) {
    const [jobs, setJobs] = useState([]);
    const [loading, setLoading] = useState(false);
    const { lastRefresh, namespaces: allNamespaces } = useK8s();

    useEffect(() => {
        if (!isVisible || !currentContext || namespaces === null || namespaces === undefined) return;

        const fetchJobs = async () => {
            setLoading(true);
            try {
                const optimized = optimizeNamespaceQuery(namespaces, allNamespaces);

                if (optimized === null) {
                    // No namespaces selected - return empty
                    setJobs([]);
                } else if (optimized === '') {
                    // Fetch from all namespaces in a single query (optimized)
                    const result = await ListJobs('');
                    setJobs(result || []);
                } else {
                    // Fetch from each namespace and merge results
                    const allJobs = await Promise.all(
                        optimized.map(ns => ListJobs(ns).catch(err => {
                            console.error(`Failed to fetch jobs from namespace ${ns}`, err);
                            return [];
                        }))
                    );
                    // Flatten and deduplicate by UID
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

    return { jobs, loading };
}
