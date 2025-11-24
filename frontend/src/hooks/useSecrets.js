import { useState, useEffect } from 'react';
import { ListSecrets } from '../../wailsjs/go/main/App';

export const useSecrets = (currentContext, namespace, isVisible) => {
    const [secrets, setSecrets] = useState([]);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState(null);

    useEffect(() => {
        if (!currentContext || namespace === null || namespace === undefined || !isVisible) return;

        const fetchSecrets = async () => {
            setLoading(true);
            try {
                const list = await ListSecrets(namespace);
                setSecrets(list || []);
                setError(null);
            } catch (err) {
                console.error("Failed to fetch secrets", err);
                setError(err);
            } finally {
                setLoading(false);
            }
        };

        fetchSecrets();
    }, [currentContext, namespace, isVisible]);

    return { secrets, loading, error };
};
