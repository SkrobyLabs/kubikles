import { useState, useEffect, useCallback, useMemo } from 'react';
import { ListEvents } from '../../wailsjs/go/main/App';
import { useK8s } from '../context/K8sContext';
import { useResourceWatcher } from './useResourceWatcher';
import { createNamespacedResourceEventHandler } from './useResourceEventHandler';

export const useEventsList = (currentContext, currentNamespace, isVisible) => {
    const [events, setEvents] = useState([]);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState(null);
    const { lastRefresh } = useK8s();

    // Calculate namespaces for watching
    const optimizedNamespaces = useMemo(() => {
        const namespaces = Array.isArray(currentNamespace) ? currentNamespace : [currentNamespace];
        if (namespaces.includes('*')) return ['']; // Watch all namespaces
        return namespaces.filter(ns => ns); // Filter out empty values
    }, [currentNamespace]);

    // Fetch initial list
    useEffect(() => {
        if (!currentContext || !isVisible) return;

        const fetchEvents = async () => {
            setLoading(true);
            try {
                const namespaces = Array.isArray(currentNamespace) ? currentNamespace : [currentNamespace];

                if (namespaces.includes('*')) {
                    const list = await ListEvents('');
                    setEvents(list || []);
                } else {
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

    // Create selected namespaces array for event filtering
    const selectedNamespacesList = useMemo(() => {
        const namespaces = Array.isArray(currentNamespace) ? currentNamespace : [currentNamespace];
        if (namespaces.includes('*')) return []; // Empty list means accept all
        return namespaces.filter(ns => ns);
    }, [currentNamespace]);

    // Subscribe to event events
    const handleEvent = useCallback(
        createNamespacedResourceEventHandler(setEvents, selectedNamespacesList),
        [selectedNamespacesList]
    );

    useResourceWatcher(
        "events",
        optimizedNamespaces,
        handleEvent,
        currentContext && isVisible && optimizedNamespaces.length > 0
    );

    return { events, loading, error };
};
