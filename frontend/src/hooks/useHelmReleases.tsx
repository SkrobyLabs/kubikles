import { useState, useEffect, useCallback, useMemo, useRef } from 'react';
import {
    GetHelmRelease,
    GetHelmReleaseValues,
    GetHelmReleaseAllValues,
    GetHelmReleaseHistory,
    UninstallHelmRelease,
    RollbackHelmRelease
} from 'wailsjs/go/main/App';
import { useK8s } from '../context';
import { K8sHelmRelease } from '../types/k8s';
import { useSecretReadSource } from '~/features/config/secrets/secretReadSource';
import { normalizeSecretNamespaces } from '~/features/config/secrets/secretListOperations';
import { optimizeNamespaceQuery } from './useNamespaceOptimization';
import Logger from '~/utils/Logger';

const HELM_RELEASE_SECRET_TYPE = 'helm.sh/release.v1';

const releaseNamespaces = (selectedNamespaces: string[], allNamespaces: string[]) => {
    const optimized = optimizeNamespaceQuery(selectedNamespaces, allNamespaces);
    if (optimized === null) return [];
    if (optimized === '') return [''];
    return normalizeSecretNamespaces(optimized, allNamespaces);
};

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

/** Helm releases are projected in the active backend and invalidated by the shared Secret watcher. */
export const useHelmReleases = (
    currentContext: string | null,
    selectedNamespaces: string[],
    isVisible: boolean
): UseHelmReleasesResult => {
    const [releases, setReleases] = useState<K8sHelmRelease[]>([]);
    const [loading, setLoading] = useState<boolean>(false);
    const [error, setError] = useState<Error | null>(null);
    const { namespaces: allNamespaces, lastRefresh, reconcileToken } = useK8s();
    const secretSource = useSecretReadSource();
    const namespaces = useMemo(
        () => releaseNamespaces(selectedNamespaces, allNamespaces),
        [JSON.stringify(selectedNamespaces), JSON.stringify(allNamespaces)],
    );
    const namespaceKey = JSON.stringify(namespaces);
    const expectedWatchReady = `${secretSource.sourceKey}:${currentContext ?? ''}:${namespaceKey}`;
    const [helmEventRevision, setHelmEventRevision] = useState(0);
    const [manualRevision, setManualRevision] = useState(0);
    const [watchReady, setWatchReady] = useState('');
    const listRun = useRef(0);
    const requestSequence = useRef(0);

    useEffect(() => {
        let active = true;
        let watchersSettled = false;
        let debounce: ReturnType<typeof setTimeout> | null = null;
        const subscriptions: string[] = [];
        const scheduleReconcile = () => {
            if (debounce) clearTimeout(debounce);
            debounce = setTimeout(() => {
                debounce = null;
                if (active) setHelmEventRevision(value => value + 1);
            }, 150);
        };

        setWatchReady('');
        if (!currentContext || !isVisible || namespaces.length === 0) {
            setReleases([]);
            setLoading(false);
            setError(null);
            return () => { active = false; };
        }
        setLoading(true);

        const disposeResource = secretSource.onResource(event => {
            if (watchersSettled && event?.resource?.type === HELM_RELEASE_SECRET_TYPE) scheduleReconcile();
        });
        const disposeStatus = secretSource.onStatus(event => {
            if (watchersSettled && event?.status === 'connected') scheduleReconcile();
        });
        void Promise.allSettled(namespaces.map(async namespace => {
            const watcherSpecId = await secretSource.subscribe(namespace, false);
            if (!watcherSpecId) return;
            if (active) subscriptions.push(watcherSpecId);
            else await secretSource.unsubscribe(watcherSpecId);
        })).then(results => {
            if (!active) return;
            watchersSettled = true;
            const failures = results.filter(result => result.status === 'rejected').length;
            if (failures > 0) Logger.warn('Some Helm release watchers could not be started', { failures, namespaces: namespaces.length }, 'helm');
            setWatchReady(expectedWatchReady);
        });

        return () => {
            active = false;
            if (debounce) clearTimeout(debounce);
            disposeResource();
            disposeStatus();
            for (const watcherSpecId of subscriptions) void secretSource.unsubscribe(watcherSpecId);
        };
    }, [secretSource, currentContext, namespaceKey, isVisible, expectedWatchReady]);

    useEffect(() => {
        const run = ++listRun.current;
        let active = true;
        const current = () => active && listRun.current === run;

        if (!currentContext || !isVisible) {
            setReleases([]);
            setLoading(false);
            setError(null);
            return () => { active = false; };
        }
        if (watchReady !== expectedWatchReady || namespaces.length === 0) {
            return () => { active = false; };
        }

        setLoading(true);
        setError(null);
        const requestIds = namespaces.map(() => `helm-releases-${++requestSequence.current}`);
        void Promise.allSettled(namespaces.map((namespace, index) =>
            secretSource.listHelmReleaseMetadata(requestIds[index], namespace)))
            .then(results => {
                if (!current()) return;
                const failures = results.filter(result => result.status === 'rejected');
                const listed = results.flatMap(result => result.status === 'fulfilled' && Array.isArray(result.value) ? result.value : []);
                listed.sort((left, right) => {
                    const updated = new Date(right.updated).getTime() - new Date(left.updated).getTime();
                    return updated || left.namespace.localeCompare(right.namespace) || left.name.localeCompare(right.name);
                });
                setReleases(listed);
                if (failures.length > 0) {
                    Logger.warn('Some Helm release metadata lists failed', { failures: failures.length, namespaces: namespaces.length }, 'helm');
                    if (listed.length === 0) {
                        const reason = failures[0].status === 'rejected' ? failures[0].reason : 'Helm release metadata is unavailable';
                        setError(reason instanceof Error ? reason : new Error(String(reason)));
                    }
                }
            })
            .finally(() => {
                if (current()) setLoading(false);
            });
        return () => {
            active = false;
            for (const requestId of requestIds) void secretSource.cancelList(requestId);
        };
    }, [secretSource, currentContext, namespaceKey, isVisible, watchReady, expectedWatchReady, helmEventRevision, manualRevision, lastRefresh, reconcileToken]);

    const fetchReleases = useCallback(async (): Promise<void> => {
        setManualRevision(value => value + 1);
    }, []);

    // Get release details
    const getRelease = useCallback(async (namespace: string, name: string): Promise<K8sHelmRelease> => {
        try {
            return await GetHelmRelease(namespace, name);
        } catch (err: any) {
            console.error(`Failed to get Helm release ${namespace}/${name}`, err);
            throw err;
        }
    }, []);

    // Get release values (user-supplied)
    const getValues = useCallback(async (namespace: string, name: string): Promise<string> => {
        try {
            return await GetHelmReleaseValues(namespace, name);
        } catch (err: any) {
            console.error(`Failed to get values for ${namespace}/${name}`, err);
            throw err;
        }
    }, []);

    // Get all computed values
    const getAllValues = useCallback(async (namespace: string, name: string): Promise<string> => {
        try {
            return await GetHelmReleaseAllValues(namespace, name);
        } catch (err: any) {
            console.error(`Failed to get all values for ${namespace}/${name}`, err);
            throw err;
        }
    }, []);

    // Get release history
    const getHistory = useCallback(async (namespace: string, name: string): Promise<HelmReleaseHistory[]> => {
        try {
            return await GetHelmReleaseHistory(namespace, name);
        } catch (err: any) {
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
        } catch (err: any) {
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
        } catch (err: any) {
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
