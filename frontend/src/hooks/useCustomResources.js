import { useState, useEffect } from 'react';
import { ListCustomResources } from '../../wailsjs/go/main/App';
import { useK8s } from '../context/K8sContext';
import { optimizeNamespaceQuery } from './useNamespaceOptimization';

/**
 * Hook to fetch custom resource instances for a given CRD.
 * @param {string} currentContext - The current k8s context
 * @param {string} group - The API group (e.g., 'traefik.io')
 * @param {string} version - The API version (e.g., 'v1alpha1')
 * @param {string} resource - The plural resource name (e.g., 'ingressroutes')
 * @param {string[]} selectedNamespaces - Selected namespaces array (for namespaced resources)
 * @param {boolean} isVisible - Whether the component is visible
 * @param {boolean} isNamespaced - Whether the resource is namespaced
 */
export const useCustomResources = (currentContext, group, version, resource, selectedNamespaces, isVisible, isNamespaced) => {
    const [resources, setResources] = useState([]);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState(null);
    const { namespaces: allNamespaces } = useK8s();

    useEffect(() => {
        if (!currentContext || !isVisible || !group || !version || !resource) return;

        const fetchResources = async () => {
            setLoading(true);
            try {
                if (!isNamespaced) {
                    // Cluster-scoped: fetch all
                    const list = await ListCustomResources(group, version, resource, '');
                    setResources(list || []);
                } else {
                    // Namespaced: use optimization logic
                    const optimized = optimizeNamespaceQuery(selectedNamespaces, allNamespaces);

                    if (optimized === null) {
                        // No namespaces selected - return empty
                        setResources([]);
                    } else if (optimized === '') {
                        // Fetch from all namespaces in a single query
                        const list = await ListCustomResources(group, version, resource, '');
                        setResources(list || []);
                    } else {
                        // Fetch from each namespace and merge results
                        const allResources = await Promise.all(
                            optimized.map(ns => ListCustomResources(group, version, resource, ns).catch(err => {
                                console.error(`Failed to fetch custom resources from namespace ${ns}`, err);
                                return [];
                            }))
                        );
                        // Flatten and deduplicate by UID
                        const merged = allResources.flat();
                        const unique = merged.filter((item, index, self) =>
                            index === self.findIndex(r => r.metadata?.uid === item.metadata?.uid)
                        );
                        setResources(unique);
                    }
                }
                setError(null);
            } catch (err) {
                console.error("Failed to fetch custom resources", err);
                setError(err);
            } finally {
                setLoading(false);
            }
        };

        fetchResources();
    }, [currentContext, group, version, resource, selectedNamespaces, isVisible, isNamespaced, allNamespaces]);

    return { resources, loading, error };
};
