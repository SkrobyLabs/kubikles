import { useState, useEffect } from 'react';
import { ListStatefulSets } from '../../wailsjs/go/main/App';
import { useK8s } from '../context/K8sContext';
import { optimizeNamespaceQuery } from './useNamespaceOptimization';

export const useStatefulSets = (contextName, namespaces, isVisible) => {
    const [statefulSets, setStatefulSets] = useState([]);
    const [loading, setLoading] = useState(true);
    const { namespaces: allNamespaces } = useK8s();

    useEffect(() => {
        if (!isVisible || namespaces === null || namespaces === undefined || !contextName) {
            setLoading(false);
            return;
        }

        const fetchStatefulSets = async () => {
            setLoading(true);
            try {
                const optimized = optimizeNamespaceQuery(namespaces, allNamespaces);

                if (optimized === null) {
                    // No namespaces selected - return empty
                    setStatefulSets([]);
                } else if (optimized === '') {
                    // Fetch from all namespaces in a single query (optimized)
                    const list = await ListStatefulSets(contextName, '');
                    setStatefulSets(list || []);
                } else {
                    // Fetch from each namespace and merge results
                    const allStatefulSets = await Promise.all(
                        optimized.map(ns => ListStatefulSets(contextName, ns).catch(err => {
                            console.error(`Failed to fetch statefulsets from namespace ${ns}`, err);
                            return [];
                        }))
                    );
                    // Flatten and deduplicate by UID
                    const merged = allStatefulSets.flat();
                    const unique = merged.filter((sts, index, self) =>
                        index === self.findIndex(s => s.metadata.uid === sts.metadata.uid)
                    );
                    setStatefulSets(unique);
                }
            } catch (err) {
                console.error('Failed to fetch statefulsets:', err);
                setStatefulSets([]);
            } finally {
                setLoading(false);
            }
        };

        fetchStatefulSets();
    }, [contextName, namespaces, isVisible, allNamespaces]);

    return { statefulSets, loading };
};
