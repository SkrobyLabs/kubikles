import { useState, useEffect } from 'react';
import { ListConfigMaps } from '../../wailsjs/go/main/App';

export const useConfigMaps = (namespace, isVisible) => {
    const [configMaps, setConfigMaps] = useState([]);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState(null);

    useEffect(() => {
        if (!namespace || !isVisible) return;

        const fetchConfigMaps = async () => {
            setLoading(true);
            try {
                const list = await ListConfigMaps(namespace);
                setConfigMaps(list || []);
                setError(null);
            } catch (err) {
                console.error("Failed to fetch configmaps", err);
                setError(err);
            } finally {
                setLoading(false);
            }
        };

        fetchConfigMaps();
    }, [namespace, isVisible]);

    return { configMaps, loading, error };
};
