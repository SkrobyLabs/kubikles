import { useState, useEffect } from 'react';
import { ListCronJobs } from '../../wailsjs/go/main/App';
import { useK8s } from '../context/K8sContext';

export function useCronJobs(context, namespace, isVisible) {
    const [cronJobs, setCronJobs] = useState([]);
    const [loading, setLoading] = useState(true);
    const { lastRefresh } = useK8s();

    useEffect(() => {
        if (!isVisible || !context || namespace === null || namespace === undefined) {
            return;
        }

        let isCancelled = false;

        const fetchCronJobs = async () => {
            setLoading(true);
            try {
                const data = await ListCronJobs(namespace);
                if (!isCancelled) {
                    setCronJobs(data || []);
                }
            } catch (err) {
                if (!isCancelled) {
                    console.error('Error fetching cron jobs:', err);
                    setCronJobs([]);
                }
            } finally {
                if (!isCancelled) {
                    setLoading(false);
                }
            }
        };

        fetchCronJobs();

        return () => {
            isCancelled = true;
        };
    }, [context, namespace, isVisible, lastRefresh]);

    return { cronJobs, loading };
}
