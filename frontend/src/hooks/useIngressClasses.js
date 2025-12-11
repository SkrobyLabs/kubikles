import { useState, useEffect, useCallback } from 'react';
import { ListIngressClasses } from '../../wailsjs/go/main/App';
import { useK8s } from '../context/K8sContext';
import { useResourceWatcher } from './useResourceWatcher';
import { createResourceEventHandler } from './useResourceEventHandler';

export const useIngressClasses = (currentContext, isVisible) => {
    const [ingressClasses, setIngressClasses] = useState([]);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState(null);
    const { lastRefresh } = useK8s();

    // Fetch initial list
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

    // Subscribe to ingressclass events (cluster-scoped, so namespace = "")
    const handleEvent = useCallback(createResourceEventHandler(setIngressClasses), []);
    useResourceWatcher("ingressclasses", "", handleEvent, currentContext && isVisible);

    return { ingressClasses, loading, error };
};
