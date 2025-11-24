import { useState, useEffect, useRef } from 'react';
import { ListPods, StartPodWatcher } from '../../wailsjs/go/main/App';
import { useK8s } from '../context/K8sContext';
import { optimizeNamespaceQuery } from './useNamespaceOptimization';

export const usePods = (currentContext, selectedNamespaces, isVisible) => {
    const [pods, setPods] = useState([]);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState(null);
    const { namespaces: allNamespaces } = useK8s();

    useEffect(() => {
        if (!currentContext || selectedNamespaces === null || selectedNamespaces === undefined || !isVisible) return;

        const fetchPods = async () => {
            setLoading(true);
            try {
                const optimized = optimizeNamespaceQuery(selectedNamespaces, allNamespaces);

                if (optimized === null) {
                    // No namespaces selected - return empty
                    setPods([]);
                } else if (optimized === '') {
                    // Fetch from all namespaces in a single query (optimized)
                    const list = await ListPods('');
                    setPods(list || []);
                } else {
                    // Fetch from each namespace and merge results
                    const allPods = await Promise.all(
                        optimized.map(ns => ListPods(ns).catch(err => {
                            console.error(`Failed to fetch pods from namespace ${ns}`, err);
                            return [];
                        }))
                    );
                    // Flatten and deduplicate by UID
                    const merged = allPods.flat();
                    const unique = merged.filter((pod, index, self) =>
                        index === self.findIndex(p => p.metadata.uid === pod.metadata.uid)
                    );
                    setPods(unique);
                }
                setError(null);
            } catch (err) {
                console.error("Failed to fetch pods", err);
                setError(err);
            } finally {
                setLoading(false);
            }
        };

        fetchPods();

        // Start watchers - optimize the same way
        const optimizedWatch = optimizeNamespaceQuery(selectedNamespaces, allNamespaces);
        if (optimizedWatch === '') {
            StartPodWatcher('');
        } else if (optimizedWatch !== null) {
            optimizedWatch.forEach(ns => StartPodWatcher(ns));
        }

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
    }, [currentContext, selectedNamespaces, isVisible, allNamespaces]);

    return { pods, loading, error, setPods };
};
