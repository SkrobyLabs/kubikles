import { useState, useEffect } from 'react';
import { ListIngressClasses } from '../../wailsjs/go/main/App';
import { useK8s } from '../context/K8sContext';

export const useIngressClasses = (currentContext, isVisible) => {
    const [ingressClasses, setIngressClasses] = useState([]);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState(null);
    const { lastRefresh } = useK8s();

    useEffect(() => {
        if (!currentContext || !isVisible) return;

        const fetchIngressClasses = async () => {
            setLoading(true);
            try {
                const list = await ListIngressClasses();
                setIngressClasses(list || []);
                setError(null);
            } catch (err) {
                console.error("Failed to fetch ingress classes", err);
                setError(err);
            } finally {
                setLoading(false);
            }
        };

        fetchIngressClasses();
    }, [currentContext, isVisible, lastRefresh]);

    return { ingressClasses, loading, error };
};
