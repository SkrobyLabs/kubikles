import { useState, useEffect } from 'react';
import { ListIngresses } from '../../wailsjs/go/main/App';
import { useK8s } from '../context/K8sContext';
import { optimizeNamespaceQuery } from './useNamespaceOptimization';

export const useIngresses = (currentContext, namespaces, isVisible) => {
    const [ingresses, setIngresses] = useState([]);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState(null);
    const { namespaces: allNamespaces, lastRefresh } = useK8s();

    useEffect(() => {
        if (!currentContext || namespaces === null || namespaces === undefined || !isVisible) return;

        const fetchIngresses = async () => {
            setLoading(true);
            try {
                const optimized = optimizeNamespaceQuery(namespaces, allNamespaces);

                if (optimized === null) {
                    // No namespaces selected - return empty
                    setIngresses([]);
                } else if (optimized === '') {
                    // Fetch from all namespaces in a single query (optimized)
                    const list = await ListIngresses('');
                    setIngresses(list || []);
                } else {
                    // Fetch from each namespace and merge results
                    const allIngresses = await Promise.all(
                        optimized.map(ns => ListIngresses(ns).catch(err => {
                            console.error(`Failed to fetch ingresses from namespace ${ns}`, err);
                            return [];
                        }))
                    );
                    // Flatten and deduplicate by UID
                    const merged = allIngresses.flat();
                    const unique = merged.filter((ing, index, self) =>
                        index === self.findIndex(i => i.metadata.uid === ing.metadata.uid)
                    );
                    setIngresses(unique);
                }
                setError(null);
            } catch (err) {
                console.error("Failed to fetch ingresses", err);
                setError(err);
            } finally {
                setLoading(false);
            }
        };

        fetchIngresses();
    }, [currentContext, namespaces, isVisible, allNamespaces, lastRefresh]);

    return { ingresses, loading, error };
};
