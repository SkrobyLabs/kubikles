import { useState, useEffect } from 'react';
import { ListEvents } from '../../wailsjs/go/main/App';
import { useK8s } from '../context/K8sContext';

export const useEventsList = (currentContext, currentNamespace, isVisible) => {
    const [events, setEvents] = useState([]);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState(null);
    const { lastRefresh } = useK8s();

    useEffect(() => {
        if (!currentContext || !isVisible) return;

        const fetchEvents = async () => {
            setLoading(true);
            try {
                // Handle multiple namespaces
                const namespaces = Array.isArray(currentNamespace) ? currentNamespace : [currentNamespace];

                if (namespaces.includes('*')) {
                    // Fetch from all namespaces
                    const list = await ListEvents('');
                    setEvents(list || []);
                } else {
                    // Fetch from selected namespaces
                    const allEvents = [];
                    for (const ns of namespaces) {
                        const list = await ListEvents(ns);
                        if (list) allEvents.push(...list);
                    }
                    setEvents(allEvents);
                }
                setError(null);
            } catch (err) {
                console.error("Failed to fetch events", err);
                setError(err);
            } finally {
                setLoading(false);
            }
        };

        fetchEvents();
    }, [currentContext, currentNamespace, isVisible, lastRefresh]);

    return { events, loading, error };
};
