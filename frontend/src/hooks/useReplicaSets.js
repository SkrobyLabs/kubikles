import { useState, useEffect } from 'react';
import { ListReplicaSets } from '../../wailsjs/go/main/App';
import { useK8s } from '../context/K8sContext';

export function useReplicaSets(currentContext, namespace, isVisible = true) {
    const [replicaSets, setReplicaSets] = useState([]);
    const [loading, setLoading] = useState(false);
    const { lastRefresh } = useK8s();

    useEffect(() => {
        if (!isVisible || !currentContext) return;

        const fetchReplicaSets = async () => {
            setLoading(true);
            try {
                const result = await ListReplicaSets(namespace);
                setReplicaSets(result || []);
            } catch (err) {
                console.error("Failed to fetch replicasets:", err);
                setReplicaSets([]);
            } finally {
                setLoading(false);
            }
        };

        fetchReplicaSets();
    }, [currentContext, namespace, lastRefresh, isVisible]);

    return { replicaSets, loading };
}
