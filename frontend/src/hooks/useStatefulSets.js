import { useState, useEffect } from 'react';
import { ListStatefulSets } from '../../wailsjs/go/main/App';

export const useStatefulSets = (contextName, namespace, isVisible) => {
    const [statefulSets, setStatefulSets] = useState([]);
    const [loading, setLoading] = useState(true);

    useEffect(() => {
        if (!isVisible || !namespace || !contextName) {
            setLoading(false);
            return;
        }

        const fetchStatefulSets = async () => {
            setLoading(true);
            try {
                const list = await ListStatefulSets(contextName, namespace);
                setStatefulSets(list || []);
            } catch (err) {
                console.error('Failed to fetch statefulsets:', err);
                setStatefulSets([]);
            } finally {
                setLoading(false);
            }
        };

        fetchStatefulSets();
    }, [contextName, namespace, isVisible]);

    return { statefulSets, loading };
};
