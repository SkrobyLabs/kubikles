import { useState, useEffect } from 'react';
import { ListNamespaces } from '../../wailsjs/go/main/App';

export const useNamespacesList = (currentContext, isVisible) => {
    const [namespaces, setNamespaces] = useState([]);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState(null);

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
    }, [currentContext, isVisible]);

    return { namespaces, loading, error };
};
