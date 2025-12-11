import { useState, useEffect, useCallback } from 'react';
import { ListStorageClasses } from '../../wailsjs/go/main/App';
import { useK8s } from '../context/K8sContext';
import { useResourceWatcher } from './useResourceWatcher';
import { createResourceEventHandler } from './useResourceEventHandler';

export const useStorageClasses = (currentContext, isVisible) => {
    const [storageClasses, setStorageClasses] = useState([]);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState(null);
    const { lastRefresh } = useK8s();

    // Fetch initial list
    useEffect(() => {
        if (!currentContext || !isVisible) return;

        const fetchStorageClasses = async () => {
            setLoading(true);
            try {
                const list = await ListStorageClasses();
                setStorageClasses(list || []);
                setError(null);
            } catch (err) {
                console.error("Failed to fetch StorageClasses", err);
                setError(err);
            } finally {
                setLoading(false);
            }
        };

        fetchStorageClasses();
    }, [currentContext, isVisible, lastRefresh]);

    // Subscribe to StorageClass events (cluster-scoped, so namespace = "")
    const handleEvent = useCallback(createResourceEventHandler(setStorageClasses), []);
    useResourceWatcher("storageclasses", "", handleEvent, currentContext && isVisible);

    return { storageClasses, loading, error };
};
