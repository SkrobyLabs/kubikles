import { useState, useEffect } from 'react';
import { ListNodes } from '../../wailsjs/go/main/App';

export const useNodes = (currentContext, isVisible) => {
    const [nodes, setNodes] = useState([]);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState(null);

    useEffect(() => {
        if (!currentContext || !isVisible) return;

        const fetchNodes = async () => {
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
        };

        fetchNodes();
    }, [currentContext, isVisible]);

    return { nodes, loading, error };
};
