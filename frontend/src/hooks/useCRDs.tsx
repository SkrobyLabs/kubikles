import { useState, useEffect } from 'react';
import { ListCRDs } from 'wailsjs/go/main/App';
import { useK8s } from '../context';
import { K8sCustomResourceDefinition } from '../types/k8s';

interface UseCRDsResult {
    crds: K8sCustomResourceDefinition[];
    loading: boolean;
    error: Error | null;
}

export const useCRDs = (
    currentContext: string | null,
    isVisible: boolean
): UseCRDsResult => {
    const [crds, setCRDs] = useState<K8sCustomResourceDefinition[]>([]);
    const [loading, setLoading] = useState<boolean>(false);
    const [error, setError] = useState<Error | null>(null);
    const { lastRefresh } = useK8s();

    useEffect(() => {
        if (!currentContext || !isVisible) return;

        const fetchCRDs = async (): Promise<void> => {
            setLoading(true);
            try {
                const list = await ListCRDs();
                setCRDs(list || []);
                setError(null);
            } catch (err) {
                console.error("Failed to fetch CRDs", err);
                setError(err as Error);
            } finally {
                setLoading(false);
            }
        };

        fetchCRDs();
    }, [currentContext, isVisible, lastRefresh]);

    return { crds, loading, error };
};
