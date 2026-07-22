import { useState, useEffect, useCallback } from 'react';
import { ListCRDs } from 'wailsjs/go/main/App';
import { useK8s } from '../context';
import { K8sCustomResourceDefinition } from '../types/k8s';
import { useCompletionPolling } from './useCompletionPolling';

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
    const { lastRefresh, connectionMode } = useK8s();

    const fetchCRDs = useCallback(async (isCurrent: () => boolean = () => true): Promise<void> => {
        if (!currentContext || !isVisible) return;
        setLoading(true);
        try {
            const list = await ListCRDs();
            if (!isCurrent()) return;
            setCRDs(list || []);
            setError(null);
        } catch (err: any) {
            console.error("Failed to fetch CRDs", err);
            if (isCurrent()) setError(err as Error);
        } finally {
            if (isCurrent()) setLoading(false);
        }
    }, [currentContext, isVisible]);

    useEffect(() => {
        let current = true;
        fetchCRDs(() => current);
        return () => { current = false; };
    }, [fetchCRDs, lastRefresh]);
    useCompletionPolling(connectionMode === 'polling' && isVisible, fetchCRDs, [fetchCRDs]);

    return { crds, loading, error };
};
