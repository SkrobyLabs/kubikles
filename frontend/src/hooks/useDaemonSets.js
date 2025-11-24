import { useState, useEffect } from 'react';
import { ListDaemonSets } from '../../wailsjs/go/main/App';
import { useK8s } from '../context/K8sContext';
import { optimizeNamespaceQuery } from './useNamespaceOptimization';

export function useDaemonSets(currentContext, namespaces, isVisible = true) {
    const [daemonSets, setDaemonSets] = useState([]);
    const [loading, setLoading] = useState(false);
    const { lastRefresh, namespaces: allNamespaces } = useK8s();

    useEffect(() => {
        if (!isVisible || !currentContext || namespaces === null || namespaces === undefined) return;

        const fetchDaemonSets = async () => {
            setLoading(true);
            try {
                const optimized = optimizeNamespaceQuery(namespaces, allNamespaces);

                if (optimized === null) {
                    // No namespaces selected - return empty
                    setDaemonSets([]);
                } else if (optimized === '') {
                    // Fetch from all namespaces in a single query (optimized)
                    const result = await ListDaemonSets('');
                    setDaemonSets(result || []);
                } else {
                    // Fetch from each namespace and merge results
                    const allDaemonSets = await Promise.all(
                        optimized.map(ns => ListDaemonSets(ns).catch(err => {
                            console.error(`Failed to fetch daemonsets from namespace ${ns}`, err);
                            return [];
                        }))
                    );
                    // Flatten and deduplicate by UID
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

    return { daemonSets, loading };
}
