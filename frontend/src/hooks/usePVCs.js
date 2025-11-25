import { useState, useEffect } from 'react';
import { ListPVCs } from '../../wailsjs/go/main/App';
import { useK8s } from '../context/K8sContext';
import { optimizeNamespaceQuery } from './useNamespaceOptimization';

export const usePVCs = (currentContext, namespaces, isVisible) => {
    const [pvcs, setPVCs] = useState([]);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState(null);
    const { namespaces: allNamespaces } = useK8s();

    useEffect(() => {
        if (!currentContext || namespaces === null || namespaces === undefined || !isVisible) return;

        const fetchPVCs = async () => {
            setLoading(true);
            try {
                const optimized = optimizeNamespaceQuery(namespaces, allNamespaces);

                if (optimized === null) {
                    setPVCs([]);
                } else if (optimized === '') {
                    const list = await ListPVCs('');
                    setPVCs(list || []);
                } else {
                    const allPVCs = await Promise.all(
                        optimized.map(ns => ListPVCs(ns).catch(err => {
                            console.error(`Failed to fetch PVCs from namespace ${ns}`, err);
                            return [];
                        }))
                    );
                    const merged = allPVCs.flat();
                    const unique = merged.filter((pvc, index, self) =>
                        index === self.findIndex(p => p.metadata.uid === pvc.metadata.uid)
                    );
                    setPVCs(unique);
                }
                setError(null);
            } catch (err) {
                console.error("Failed to fetch PVCs", err);
                setError(err);
            } finally {
                setLoading(false);
            }
        };

        fetchPVCs();
    }, [currentContext, namespaces, isVisible, allNamespaces]);

    return { pvcs, loading, error };
};
