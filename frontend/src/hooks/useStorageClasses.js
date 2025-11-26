import { useState, useEffect } from 'react';
import { ListStorageClasses } from '../../wailsjs/go/main/App';
import { useK8s } from '../context/K8sContext';

export const useStorageClasses = (currentContext, isVisible) => {
    const [storageClasses, setStorageClasses] = useState([]);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState(null);
    const { lastRefresh } = useK8s();

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

    return { storageClasses, loading, error };
};
