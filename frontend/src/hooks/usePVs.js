import { useState, useEffect } from 'react';
import { ListPVs } from '../../wailsjs/go/main/App';

export const usePVs = (currentContext, isVisible) => {
    const [pvs, setPVs] = useState([]);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState(null);

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
    }, [currentContext, isVisible]);

    return { pvs, loading, error };
};
