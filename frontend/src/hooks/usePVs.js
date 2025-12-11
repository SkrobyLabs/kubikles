import { useState, useEffect, useCallback } from 'react';
import { ListPVs } from '../../wailsjs/go/main/App';
import { useK8s } from '../context/K8sContext';
import { useResourceWatcher } from './useResourceWatcher';
import { createResourceEventHandler } from './useResourceEventHandler';

export const usePVs = (currentContext, isVisible) => {
    const [pvs, setPVs] = useState([]);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState(null);
    const { lastRefresh } = useK8s();

    // Fetch initial list
    useEffect(() => {
        if (!currentContext || !isVisible) return;

        const fetchPVs = async () => {
            setLoading(true);
            try {
                const list = await ListPVs();
                setPVs(list || []);
                setError(null);
            } catch (err) {
                console.error("Failed to fetch PVs", err);
                setError(err);
            } finally {
                setLoading(false);
            }
        };

        fetchPVs();
    }, [currentContext, isVisible, lastRefresh]);

    // Subscribe to PV events (cluster-scoped, so namespace = "")
    const handleEvent = useCallback(createResourceEventHandler(setPVs), []);
    useResourceWatcher("persistentvolumes", "", handleEvent, currentContext && isVisible);

    return { pvs, loading, error };
};
