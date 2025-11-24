import { useState, useEffect } from 'react';
import { ListJobs } from '../../wailsjs/go/main/App';
import { useK8s } from '../context/K8sContext';

export function useJobs(currentContext, namespace, isVisible = true) {
    const [jobs, setJobs] = useState([]);
    const [loading, setLoading] = useState(false);
    const { lastRefresh } = useK8s();

    useEffect(() => {
        if (!isVisible || !currentContext) return;

        const fetchJobs = async () => {
            setLoading(true);
            try {
                const result = await ListJobs(namespace);
                setJobs(result || []);
            } catch (err) {
                console.error("Failed to fetch jobs:", err);
                setJobs([]);
            } finally {
                setLoading(false);
            }
        };

        fetchJobs();
    }, [currentContext, namespace, lastRefresh, isVisible]);

    return { jobs, loading };
}
