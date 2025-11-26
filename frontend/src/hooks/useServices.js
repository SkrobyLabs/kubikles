import { useState, useEffect } from 'react';
import { ListServices } from '../../wailsjs/go/main/App';
import { useK8s } from '../context/K8sContext';
import { optimizeNamespaceQuery } from './useNamespaceOptimization';

export const useServices = (currentContext, namespaces, isVisible) => {
    const [services, setServices] = useState([]);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState(null);
    const { namespaces: allNamespaces, lastRefresh } = useK8s();

    useEffect(() => {
        if (!currentContext || namespaces === null || namespaces === undefined || !isVisible) return;

        const fetchServices = async () => {
            setLoading(true);
            try {
                const optimized = optimizeNamespaceQuery(namespaces, allNamespaces);

                if (optimized === null) {
                    // No namespaces selected - return empty
                    setServices([]);
                } else if (optimized === '') {
                    // Fetch from all namespaces in a single query (optimized)
                    const list = await ListServices('');
                    setServices(list || []);
                } else {
                    // Fetch from each namespace and merge results
                    const allServices = await Promise.all(
                        optimized.map(ns => ListServices(ns).catch(err => {
                            console.error(`Failed to fetch services from namespace ${ns}`, err);
                            return [];
                        }))
                    );
                    // Flatten and deduplicate by UID
                    const merged = allServices.flat();
                    const unique = merged.filter((svc, index, self) =>
                        index === self.findIndex(s => s.metadata.uid === svc.metadata.uid)
                    );
                    setServices(unique);
                }
                setError(null);
            } catch (err) {
                console.error("Failed to fetch services", err);
                setError(err);
            } finally {
                setLoading(false);
            }
        };

        fetchServices();
    }, [currentContext, namespaces, isVisible, allNamespaces, lastRefresh]);

    return { services, loading, error };
};
