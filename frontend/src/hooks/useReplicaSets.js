import { useState, useEffect } from 'react';
import { ListReplicaSets } from '../../wailsjs/go/main/App';
import { useK8s } from '../context/K8sContext';
import { optimizeNamespaceQuery } from './useNamespaceOptimization';

export function useReplicaSets(currentContext, namespaces, isVisible = true) {
    const [replicaSets, setReplicaSets] = useState([]);
    const [loading, setLoading] = useState(false);
    const { lastRefresh, namespaces: allNamespaces } = useK8s();

    useEffect(() => {
        if (!isVisible || !currentContext || namespaces === null || namespaces === undefined) return;

        const fetchReplicaSets = async () => {
            setLoading(true);
            try {
                const optimized = optimizeNamespaceQuery(namespaces, allNamespaces);

                if (optimized === null) {
                    // No namespaces selected - return empty
                    setReplicaSets([]);
                } else if (optimized === '') {
                    // Fetch from all namespaces in a single query (optimized)
                    const result = await ListReplicaSets('');
                    setReplicaSets(result || []);
                } else {
                    // Fetch from each namespace and merge results
                    const allReplicaSets = await Promise.all(
                        optimized.map(ns => ListReplicaSets(ns).catch(err => {
                            console.error(`Failed to fetch replicasets from namespace ${ns}`, err);
                            return [];
                        }))
                    );
                    // Flatten and deduplicate by UID
                    const merged = allReplicaSets.flat();
                    const unique = merged.filter((rs, index, self) =>
                        index === self.findIndex(r => r.metadata.uid === rs.metadata.uid)
                    );
                    setReplicaSets(unique);
                }
            } catch (err) {
                console.error("Failed to fetch replicasets:", err);
                setReplicaSets([]);
            } finally {
                setLoading(false);
            }
        };

        fetchReplicaSets();
    }, [currentContext, namespaces, lastRefresh, isVisible, allNamespaces]);

    return { replicaSets, loading };
}
