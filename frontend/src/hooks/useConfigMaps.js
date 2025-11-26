import { useState, useEffect } from 'react';
import { ListConfigMaps } from '../../wailsjs/go/main/App';
import { useK8s } from '../context/K8sContext';
import { optimizeNamespaceQuery } from './useNamespaceOptimization';

export const useConfigMaps = (currentContext, namespaces, isVisible) => {
    const [configMaps, setConfigMaps] = useState([]);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState(null);
    const { namespaces: allNamespaces, lastRefresh } = useK8s();

    useEffect(() => {
        if (!currentContext || namespaces === null || namespaces === undefined || !isVisible) return;

        const fetchConfigMaps = async () => {
            setLoading(true);
            try {
                const optimized = optimizeNamespaceQuery(namespaces, allNamespaces);

                if (optimized === null) {
                    // No namespaces selected - return empty
                    setConfigMaps([]);
                } else if (optimized === '') {
                    // Fetch from all namespaces in a single query (optimized)
                    const list = await ListConfigMaps('');
                    setConfigMaps(list || []);
                } else {
                    // Fetch from each namespace and merge results
                    const allConfigMaps = await Promise.all(
                        optimized.map(ns => ListConfigMaps(ns).catch(err => {
                            console.error(`Failed to fetch configmaps from namespace ${ns}`, err);
                            return [];
                        }))
                    );
                    // Flatten and deduplicate by UID
                    const merged = allConfigMaps.flat();
                    const unique = merged.filter((cm, index, self) =>
                        index === self.findIndex(c => c.metadata.uid === cm.metadata.uid)
                    );
                    setConfigMaps(unique);
                }
                setError(null);
            } catch (err) {
                console.error("Failed to fetch configmaps", err);
                setError(err);
            } finally {
                setLoading(false);
            }
        };

        fetchConfigMaps();
    }, [currentContext, namespaces, isVisible, allNamespaces, lastRefresh]);

    return { configMaps, loading, error };
};
