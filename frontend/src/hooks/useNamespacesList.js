import { useState, useEffect, useCallback } from 'react';
import { ListNamespaces } from '../../wailsjs/go/main/App';
import { useK8s } from '../context/K8sContext';
import { useResourceWatcher } from './useResourceWatcher';
import { createResourceEventHandler } from './useResourceEventHandler';

export const useNamespacesList = (currentContext, isVisible) => {
    const [namespaces, setNamespaces] = useState([]);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState(null);
    const { lastRefresh } = useK8s();

    // Fetch initial list
    useEffect(() => {
        if (!currentContext || !isVisible) return;

        const fetchNamespaces = async () => {
            setLoading(true);
            try {
                const list = await ListNamespaces();
                setNamespaces(list || []);
                setError(null);
            } catch (err) {
                console.error("Failed to fetch namespaces", err);
                setError(err);
            } finally {
                setLoading(false);
            }
        };

        fetchNamespaces();
    }, [currentContext, isVisible, lastRefresh]);

    // Subscribe to namespace events (cluster-scoped, so namespace = "")
    const handleEvent = useCallback(createResourceEventHandler(setNamespaces), []);
    useResourceWatcher("namespaces", "", handleEvent, currentContext && isVisible);

    return { namespaces, loading, error };
};
