import { useState, useEffect } from 'react';
import { ListCRDs } from '../../wailsjs/go/main/App';
import { useK8s } from '../context';

export const useCRDs = (currentContext, isVisible) => {
    const [crds, setCRDs] = useState([]);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState(null);
    const { lastRefresh } = useK8s();

    useEffect(() => {
        if (!currentContext || !isVisible) return;

        const fetchCRDs = async () => {
            setLoading(true);
            try {
                const list = await ListCRDs();
                setCRDs(list || []);
                setError(null);
            } catch (err) {
                console.error("Failed to fetch CRDs", err);
                setError(err);
            } finally {
                setLoading(false);
            }
        };

        fetchCRDs();
    }, [currentContext, isVisible, lastRefresh]);

    return { crds, loading, error };
};
