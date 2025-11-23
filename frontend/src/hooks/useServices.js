import { useState, useEffect } from 'react';
import { ListServices } from '../../wailsjs/go/main/App';

export const useServices = (namespace, isVisible) => {
    const [services, setServices] = useState([]);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState(null);

    useEffect(() => {
        if (!namespace || !isVisible) return;

        const fetchServices = async () => {
            setLoading(true);
            try {
                const list = await ListServices(namespace);
                setServices(list || []);
                setError(null);
            } catch (err) {
                console.error("Failed to fetch services", err);
                setError(err);
            } finally {
                setLoading(false);
            }
        };

        fetchServices();
    }, [namespace, isVisible]);

    return { services, loading, error };
};
