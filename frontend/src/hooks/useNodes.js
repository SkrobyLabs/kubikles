import { useState, useEffect, useCallback } from 'react';
import { ListNodes } from '../../wailsjs/go/main/App';

export const useNodes = (currentContext, isVisible) => {
    const [nodes, setNodes] = useState([]);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState(null);

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
    }, [currentContext, isVisible]);

    useEffect(() => {
        fetchNodes();
    }, [fetchNodes]);

    const refetch = useCallback(() => {
        fetchNodes();
    }, [fetchNodes]);

    return { nodes, loading, error, refetch };
};
