import { useState, useEffect } from 'react';
import { ListCronJobs } from '../../wailsjs/go/main/App';
import { useK8s } from '../context/K8sContext';
import { optimizeNamespaceQuery } from './useNamespaceOptimization';

export function useCronJobs(context, namespaces, isVisible) {
    const [cronJobs, setCronJobs] = useState([]);
    const [loading, setLoading] = useState(true);
    const { lastRefresh, namespaces: allNamespaces } = useK8s();

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
                    // No namespaces selected - return empty
                    if (!isCancelled) {
                        setCronJobs([]);
                    }
                } else if (optimized === '') {
                    // Fetch from all namespaces in a single query (optimized)
                    const data = await ListCronJobs('');
                    if (!isCancelled) {
                        setCronJobs(data || []);
                    }
                } else {
                    // Fetch from each namespace and merge results
                    const allCronJobs = await Promise.all(
                        optimized.map(ns => ListCronJobs(ns).catch(err => {
                            console.error(`Failed to fetch cronjobs from namespace ${ns}`, err);
                            return [];
                        }))
                    );
                    if (!isCancelled) {
                        // Flatten and deduplicate by UID
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

    return { cronJobs, loading };
}
