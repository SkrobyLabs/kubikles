import { useState, useEffect, useCallback, useMemo } from 'react';
import {
    ListHelmReleases,
    GetHelmRelease,
    GetHelmReleaseValues,
    GetHelmReleaseAllValues,
    GetHelmReleaseHistory,
    UninstallHelmRelease,
    RollbackHelmRelease
} from '../../wailsjs/go/main/App';
import { useK8s } from '../context';
import { optimizeNamespaceQuery } from './useNamespaceOptimization';
import { K8sHelmRelease } from '../types/k8s';

interface HelmReleaseHistory {
    revision: number;
    updated: string;
    status: string;
    chart: string;
    appVersion: string;
    description: string;
}

interface UseHelmReleasesResult {
    releases: K8sHelmRelease[];
    loading: boolean;
    error: Error | null;
    refresh: () => Promise<void>;
    getRelease: (namespace: string, name: string) => Promise<K8sHelmRelease>;
    getValues: (namespace: string, name: string) => Promise<string>;
    getAllValues: (namespace: string, name: string) => Promise<string>;
    getHistory: (namespace: string, name: string) => Promise<HelmReleaseHistory[]>;
    uninstall: (namespace: string, name: string) => Promise<void>;
    rollback: (namespace: string, name: string, revision: number) => Promise<void>;
}

/**
 * Hook for managing Helm releases.
 * Unlike K8s resources, Helm releases don't support real-time watching,
 * so we fetch on demand and when namespaces/context change.
 */
export const useHelmReleases = (
    currentContext: string | null,
    selectedNamespaces: string[],
    isVisible: boolean
): UseHelmReleasesResult => {
    const [releases, setReleases] = useState<K8sHelmRelease[]>([]);
    const [loading, setLoading] = useState<boolean>(false);
    const [error, setError] = useState<Error | null>(null);
    const { namespaces: allNamespaces, lastRefresh } = useK8s();

    // Fetch releases
    const fetchReleases = useCallback(async (): Promise<void> => {
        if (!currentContext || !isVisible) return;

        setLoading(true);
        setError(null);
        try {
            const optimized = optimizeNamespaceQuery(selectedNamespaces, allNamespaces);

            let namespacesToQuery: string[] = [];
            if (optimized === null) {
                setReleases([]);
                return;
            } else if (optimized === '') {
                // All namespaces - pass empty array to backend
                namespacesToQuery = [];
            } else {
                namespacesToQuery = optimized;
            }

            const list = await ListHelmReleases(namespacesToQuery);
            setReleases(list || []);
        } catch (err) {
            console.error("Failed to fetch Helm releases", err);
            setError(err as Error);
            setReleases([]);
        } finally {
            setLoading(false);
        }
    }, [currentContext, selectedNamespaces, allNamespaces, isVisible]);

    // Fetch on mount and when dependencies change
    useEffect(() => {
        fetchReleases();
    }, [currentContext, selectedNamespaces, isVisible, allNamespaces, lastRefresh, fetchReleases]);

    // Get release details
    const getRelease = useCallback(async (namespace: string, name: string): Promise<K8sHelmRelease> => {
        try {
            return await GetHelmRelease(namespace, name);
        } catch (err) {
            console.error(`Failed to get Helm release ${namespace}/${name}`, err);
            throw err;
        }
    }, []);

    // Get release values (user-supplied)
    const getValues = useCallback(async (namespace: string, name: string): Promise<string> => {
        try {
            return await GetHelmReleaseValues(namespace, name);
        } catch (err) {
            console.error(`Failed to get values for ${namespace}/${name}`, err);
            throw err;
        }
    }, []);

    // Get all computed values
    const getAllValues = useCallback(async (namespace: string, name: string): Promise<string> => {
        try {
            return await GetHelmReleaseAllValues(namespace, name);
        } catch (err) {
            console.error(`Failed to get all values for ${namespace}/${name}`, err);
            throw err;
        }
    }, []);

    // Get release history
    const getHistory = useCallback(async (namespace: string, name: string): Promise<HelmReleaseHistory[]> => {
        try {
            return await GetHelmReleaseHistory(namespace, name);
        } catch (err) {
            console.error(`Failed to get history for ${namespace}/${name}`, err);
            throw err;
        }
    }, []);

    // Uninstall release
    const uninstall = useCallback(async (namespace: string, name: string): Promise<void> => {
        try {
            await UninstallHelmRelease(namespace, name);
            // Refresh list after uninstall
            await fetchReleases();
        } catch (err) {
            console.error(`Failed to uninstall ${namespace}/${name}`, err);
            throw err;
        }
    }, [fetchReleases]);

    // Rollback release
    const rollback = useCallback(async (namespace: string, name: string, revision: number): Promise<void> => {
        try {
            await RollbackHelmRelease(namespace, name, revision);
            // Refresh list after rollback
            await fetchReleases();
        } catch (err) {
            console.error(`Failed to rollback ${namespace}/${name} to revision ${revision}`, err);
            throw err;
        }
    }, [fetchReleases]);

    return {
        releases,
        loading,
        error,
        refresh: fetchReleases,
        getRelease,
        getValues,
        getAllValues,
        getHistory,
        uninstall,
        rollback
    };
};
