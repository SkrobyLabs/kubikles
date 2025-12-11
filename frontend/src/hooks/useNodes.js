import { useState, useEffect, useCallback } from 'react';
import { ListNodes } from '../../wailsjs/go/main/App';
import { useK8s } from '../context/K8sContext';
import { useResourceWatcher } from './useResourceWatcher';
import { createResourceEventHandler } from './useResourceEventHandler';

export const useNodes = (currentContext, isVisible) => {
    const [nodes, setNodes] = useState([]);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState(null);
    const { lastRefresh } = useK8s();

    const fetchNodes = useCallback(async () => {
        if (!currentContext || !isVisible) return;

        setLoading(true);
        try {
            const list = await ListNodes();
            setNodes(list || []);
            setError(null);
        } catch (err) {
            console.error("Failed to fetch nodes", err);
            setError(err);
        } finally {
            setLoading(false);
        }
    }, [currentContext, isVisible, lastRefresh]);

    useEffect(() => {
        fetchNodes();
    }, [fetchNodes]);

    const refetch = useCallback(() => {
        fetchNodes();
    }, [fetchNodes]);

    // Subscribe to node events (cluster-scoped, so namespace = "")
    const handleEvent = useCallback(createResourceEventHandler(setNodes), []);
    useResourceWatcher("nodes", "", handleEvent, currentContext && isVisible);

    return { nodes, loading, error, refetch };
};
