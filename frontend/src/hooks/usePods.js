import { useState, useEffect, useRef } from 'react';
import { ListPods, StartPodWatcher } from '../../wailsjs/go/main/App';

export const usePods = (currentContext, namespace, isVisible) => {
    const [pods, setPods] = useState([]);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState(null);

    useEffect(() => {
        if (!currentContext || namespace === null || namespace === undefined || !isVisible) return;

        const fetchPods = async () => {
            setLoading(true);
            try {
                const list = await ListPods(namespace);
                setPods(list || []);
                setError(null);
            } catch (err) {
                console.error("Failed to fetch pods", err);
                setError(err);
            } finally {
                setLoading(false);
            }
        };

        fetchPods();
        StartPodWatcher(namespace);

        // Event Listener
        const handlePodEvent = (event) => {
            const { type, pod } = event;
            // console.log(`Pod Event: ${type} - ${pod.metadata.name}`);

            setPods(prevData => {
                if (type === 'ADDED') {
                    if (prevData.find(p => p.metadata.uid === pod.metadata.uid)) return prevData;
                    return [...prevData, pod];
                } else if (type === 'MODIFIED') {
                    return prevData.map(p => p.metadata.uid === pod.metadata.uid ? pod : p);
                } else if (type === 'DELETED') {
                    return prevData.filter(p => p.metadata.uid !== pod.metadata.uid);
                }
                return prevData;
            });
        };

        if (window.runtime) {
            window.runtime.EventsOn("pod-event", handlePodEvent);
        }

        return () => {
            if (window.runtime) {
                window.runtime.EventsOff("pod-event");
            }
        };
    }, [currentContext, namespace, isVisible]);

    return { pods, loading, error, setPods };
};
