import { useState, useEffect } from 'react';
import { ListDeployments, StartPodWatcher } from '../../wailsjs/go/main/App';

export const useDeployments = (currentContext, namespace, isVisible) => {
    const [deployments, setDeployments] = useState([]);
    const [allPods, setAllPods] = useState([]);
    const [loading, setLoading] = useState(false);
    const [podsLoading, setPodsLoading] = useState(false);
    const [error, setError] = useState(null);

    useEffect(() => {
        if (!currentContext || !namespace || !isVisible) return;

        const fetchDeployments = async () => {
            setLoading(true);
            setPodsLoading(true);
            try {
                const list = await ListDeployments(namespace);
                setDeployments(list || []);
                setError(null);

                // Start watcher for pod updates
                StartPodWatcher(namespace);
            } catch (err) {
                console.error("Failed to fetch deployments", err);
                setError(err);
            } finally {
                setLoading(false);
                // We don't set podsLoading to false here because we rely on the watcher/initial pod list?
                // Actually, in the original code, we fetched pods separately.
                // We should probably fetch pods here too if we want to show them immediately.
                // But wait, usePods handles fetching pods.
                // Maybe we should use usePods inside the DeploymentList component?
                // Yes, that would be cleaner.
                // But for now, let's replicate the existing logic where we need allPods for the deployment list.
                // Wait, if we use usePods hook in the component, we get the pods.
                // So useDeployments should just return deployments.
                // And the component can use usePods to get the pods to map to deployments.
                // That separates concerns nicely.
                setPodsLoading(false);
            }
        };

        fetchDeployments();
    }, [currentContext, namespace, isVisible]);

    return { deployments, loading, podsLoading, error, setDeployments };
};
