import { useState, useEffect } from 'react';
import { ListDeployments, StartPodWatcher } from '../../wailsjs/go/main/App';
import { useK8s } from '../context/K8sContext';
import { optimizeNamespaceQuery } from './useNamespaceOptimization';

export const useDeployments = (currentContext, selectedNamespaces, isVisible) => {
    const [deployments, setDeployments] = useState([]);
    const [allPods, setAllPods] = useState([]);
    const [loading, setLoading] = useState(false);
    const [podsLoading, setPodsLoading] = useState(false);
    const [error, setError] = useState(null);
    const { namespaces: allNamespaces } = useK8s();

    useEffect(() => {
        if (!currentContext || selectedNamespaces === null || selectedNamespaces === undefined || !isVisible) return;

        const fetchDeployments = async () => {
            setLoading(true);
            setPodsLoading(true);
            try {
                const optimized = optimizeNamespaceQuery(selectedNamespaces, allNamespaces);

                if (optimized === null) {
                    // No namespaces selected - return empty
                    setDeployments([]);
                } else if (optimized === '') {
                    // Fetch from all namespaces in a single query (optimized)
                    const list = await ListDeployments('');
                    setDeployments(list || []);
                    StartPodWatcher('');
                } else {
                    // Fetch from each namespace and merge results
                    const allDeployments = await Promise.all(
                        optimized.map(ns => ListDeployments(ns).catch(err => {
                            console.error(`Failed to fetch deployments from namespace ${ns}`, err);
                            return [];
                        }))
                    );
                    // Flatten and deduplicate by UID
                    const merged = allDeployments.flat();
                    const unique = merged.filter((dep, index, self) =>
                        index === self.findIndex(d => d.metadata.uid === dep.metadata.uid)
                    );
                    setDeployments(unique);

                    // Start watchers for each namespace
                    optimized.forEach(ns => StartPodWatcher(ns));
                }
                setError(null);
            } catch (err) {
                console.error("Failed to fetch deployments", err);
                setError(err);
            } finally {
                setLoading(false);
                setPodsLoading(false);
            }
        };

        fetchDeployments();
    }, [currentContext, selectedNamespaces, isVisible, allNamespaces]);

    return { deployments, loading, podsLoading, error, setDeployments };
};
