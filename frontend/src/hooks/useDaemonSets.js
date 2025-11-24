import { useState, useEffect } from 'react';
import { ListDaemonSets } from '../../wailsjs/go/main/App';
import { useK8s } from '../context/K8sContext';

export function useDaemonSets(currentContext, namespace, isVisible = true) {
    const [daemonSets, setDaemonSets] = useState([]);
    const [loading, setLoading] = useState(false);
    const { lastRefresh } = useK8s();

    useEffect(() => {
        if (!isVisible || !currentContext) return;

        const fetchDaemonSets = async () => {
            setLoading(true);
            try {
                const result = await ListDaemonSets(namespace);
                setDaemonSets(result || []);
            } catch (err) {
                console.error("Failed to fetch daemonsets:", err);
                setDaemonSets([]);
            } finally {
                setLoading(false);
            }
        };

        fetchDaemonSets();
    }, [currentContext, namespace, lastRefresh, isVisible]);

    return { daemonSets, loading };
}
