import { readResourceSnapshot, writeResourceSnapshot, deleteResourceSnapshot } from '../utils/resourceSnapshots';
import { useState, useEffect, useCallback, useMemo, useRef } from 'react';
import { useK8s } from '../context';
import { useResourceWatcher } from './useResourceWatcher';
import { createResourceEventHandler, applyResourceEvents } from './useResourceEventHandler';
import { CancelListRequest } from 'wailsjs/go/main/App';
import { EventsOn } from 'wailsjs/runtime/runtime';
import { runWithPollingBudget } from '../utils/pollingBudget';
import { chooseNamespaceScope, namespaceSelection, observeNamespaceList, listNamespaceScope, isNamespaceForbidden, type NamespaceQueryObservation } from '../utils/namespaceQuery';

// K8s resource with metadata
interface K8sResource {
    metadata?: {
        uid?: string;
        [key: string]: any;
    };
    [key: string]: any;
}

// List function types
type NamespacedListFn<T extends K8sResource> = (requestId: string, namespace: string) => Promise<T[]>;
type ClusterScopedListFn<T extends K8sResource> = (requestId: string) => Promise<T[]>;

// Loading progress from paginated backend list
export interface LoadingProgress {
    loaded: number;
    total: number;
}

export interface ResourceLoadState {
    phase: 'loading' | 'refreshing' | 'complete' | 'incomplete';
    error: Error | null;
    progress: LoadingProgress | null;
    retry: () => void;
}

interface NamespacedResourceHookReturn<T extends K8sResource> {
    loading: boolean;
    error: Error | null;
    loadingProgress: LoadingProgress | null;
    loadState: ResourceLoadState;
    refetch: () => Promise<void>;
    [key: string]: T[] | boolean | Error | null | LoadingProgress | ResourceLoadState | (() => void);
}

// Global counter for unique request IDs - Date.now() can return same value within same millisecond
let requestCounter = 0;

/**
 * Creates a namespace key from selected namespaces for use in request IDs.
 * Sorts namespaces for stable keys.
 */
export function createNamespaceKey(selectedNamespaces: string | string[] | null | undefined): string {
    if (Array.isArray(selectedNamespaces)) {
        return [...selectedNamespaces].sort().join(',') || 'all';
    }
    return selectedNamespaces || 'all';
}

/**
 * Creates a unique request ID for namespaced resource list requests.
 * Uses incrementing counter to guarantee uniqueness even within same millisecond.
 */
export function createNamespacedRequestId(resourceType: string, selectedNamespaces: string | string[] | null | undefined): string {
    const nsKey = createNamespaceKey(selectedNamespaces);
    return `list-${resourceType}-${nsKey}-${++requestCounter}`;
}

/**
 * Creates a unique request ID for cluster-scoped resource list requests.
 * Uses incrementing counter to guarantee uniqueness even within same millisecond.
 */
export function createClusterScopedRequestId(resourceType: string): string {
    return `list-${resourceType}-cluster-${++requestCounter}`;
}

/**
 * Converts an array of K8s resources to a UID-keyed Map.
 */
function arrayToMap<T extends K8sResource>(items: T[]): Map<string, T> {
    const map = new Map<string, T>();
    for (const item of items) {
        const uid = item.metadata?.uid;
        if (uid) map.set(uid, item);
    }
    return map;
}

/**
 * Factory function to create a hook for namespaced Kubernetes resources.
 * Handles namespace optimization, multi-namespace fetching, and real-time updates.
 * Internally uses Map<string, T> for O(1) event processing, exposes T[] for consumers.
 */
export function createNamespacedResourceHook<T extends K8sResource>(
    resourceType: string,
    listFn: NamespacedListFn<T>,
    stateName: string,
    useWatcher = useResourceWatcher,
) {
    // Query measurements are shared across mounts; navigation snapshots use a bounded cache.
    const observations = new Map<string, NamespaceQueryObservation>();
    const scopedLatency = new Map<string, number>();
    return function useNamespacedResource(
        currentContext: string | null,
        selectedNamespaces: string | string[] | null | undefined,
        isVisible: boolean
    ): NamespacedResourceHookReturn<T> {
        const dataKey = JSON.stringify([currentContext, resourceType]);
        const [loading, setLoading] = useState(Boolean(currentContext && isVisible));
        const [refreshVersion, setRefreshVersion] = useState(0);
        const refetch = useCallback(async () => { setRefreshVersion(value => value + 1); }, []);
        const [error, setError] = useState<Error | null>(null);
        const [loadingProgress, setLoadingProgress] = useState<LoadingProgress | null>(null);
        const { namespaces: allNamespaces, lastRefresh, checkConnectionError, reconcileToken, connectionMode, registerPolling } = useK8s();
        const [broadDenied, setBroadDenied] = useState<string | null>(null);
        const loadedScope = useRef<{ context: string | null; key: string } | null>(null);
        const eventJournal = useRef<any[] | null>(null);
        const selectionKey = JSON.stringify(namespaceSelection(selectedNamespaces));
        const selected: string[] = useMemo(() => JSON.parse(selectionKey), [selectionKey]);
        const availableKey = JSON.stringify([...new Set(allNamespaces.filter(Boolean))].sort());
        // Explicit refresh retries permissions; polling does not repeatedly probe a denied scope.
        const permissionKey = JSON.stringify([dataKey, lastRefresh, refreshVersion]);
        const scope = useMemo(() => chooseNamespaceScope(selected, JSON.parse(availableKey),
            observations.get(dataKey), {
                broadDenied: broadDenied === permissionKey,
                reuseBroad: loadedScope.current?.context === dataKey && loadedScope.current?.key === '[""]',
                scopedDurationMs: scopedLatency.get(dataKey),
            }), [selectionKey, availableKey, dataKey, broadDenied, permissionKey]);
        const scopeKey = JSON.stringify(scope);
        const cacheKey = JSON.stringify([dataKey, scopeKey]);
        const [snapshot, setSnapshot] = useState(() => ({ context: dataKey, scopeKey,
            map: readResourceSnapshot<T>(cacheKey) || new Map<string, T>() }));
        const [refreshing, setRefreshing] = useState(snapshot.map.size > 0);
        const broad = scope.length === 1 && scope[0] === '';
        const selectedSet = useMemo(() => new Set(selected), [selectionKey]);
        const selectionRef = useRef(selectedSet);
        selectionRef.current = selectedSet;
        const data = useMemo(() => {
            if (snapshot.context !== dataKey || snapshot.scopeKey !== scopeKey || !selected.length) return [];
            return Array.from(snapshot.map.values()).filter(item => selectedSet.has('*') || selectedSet.has(item.metadata?.namespace));
        }, [snapshot, dataKey, scopeKey, selectedSet]);

        // Selection changes that retain the same fetch scope only update the local filter.
        useEffect(() => {
            if (!currentContext || !isVisible) return;
            let cancelled = false;
            let running = false;
            const activeRequests = new Set<string>();
            const namespaces: string[] = JSON.parse(scopeKey);
            let partial = new Map<string, T>();
            let keepSnapshot = false;
            const progressByRequest = new Map<string, { loaded: number; total: number | null }>();
            const cancelPages = EventsOn('list-page', (event: any) => {
                if (cancelled || !activeRequests.has(event?.requestId) ||
                    (event.context && event.context !== currentContext) || !Array.isArray(event.items)) return;
                for (const item of event.items) {
                    const uid = item.metadata?.uid;
                    if (uid) partial.set(uid, item);
                }
                progressByRequest.set(event.requestId, { loaded: event.loaded, total: event.total });
                const progress = [...progressByRequest.values()];
                setLoadingProgress({ loaded: progress.reduce((sum, item) => sum + item.loaded, 0),
                    total: progress.length === namespaces.length && progress.every(item => item.total != null)
                        ? progress.reduce((sum, item) => sum + item.total!, 0) : 0 });
                if (!keepSnapshot) setSnapshot({ context: dataKey, scopeKey,
                    map: applyResourceEvents(new Map(partial), eventJournal.current || []) });
            });
            const fetchData = async (): Promise<void> => {
                if (cancelled || running) return;
                running = true;
                const journal: any[] = [];
                eventJournal.current = journal;
                partial = new Map();
                progressByRequest.clear();
                setLoadingProgress(null);
                const cached = readResourceSnapshot<T>(cacheKey);
                keepSnapshot = Boolean(cached?.size || (loadedScope.current?.context === dataKey && loadedScope.current?.key === scopeKey));
                setRefreshing(keepSnapshot);
                if (cached) setSnapshot(previous => previous.context === dataKey && previous.scopeKey === scopeKey
                    ? previous : { context: dataKey, scopeKey, map: cached });
                setLoading(true);
                setError(null);
                try {
                    const started = performance.now();
                    const items = await listNamespaceScope(namespaces, async namespace => {
                        // Only first-load requests need page payloads; refreshes retain a usable snapshot.
                        const requestId = `${keepSnapshot ? '' : 'page-'}${createNamespacedRequestId(resourceType, namespace)}`;
                        const request = async () => {
                            if (cancelled) return [];
                            activeRequests.add(requestId);
                            const requestStarted = performance.now();
                            try {
                                const result = await listFn(requestId, namespace);
                                if (namespace && !cancelled) {
                                    const duration = performance.now() - requestStarted;
                                    const previous = scopedLatency.get(dataKey);
                                    scopedLatency.set(dataKey, previous === undefined ? duration : previous * 0.75 + duration * 0.25);
                                    if (scopedLatency.size > 64) scopedLatency.delete(scopedLatency.keys().next().value!);
                                }
                                return result || [];
                            } finally {
                                activeRequests.delete(requestId);
                            }
                        };
                        return connectionMode === 'polling' ? runWithPollingBudget(request) : request();
                    }, () => !cancelled);
                    if (cancelled) return;
                    if (broad) {
                        observations.set(dataKey, observeNamespaceList(items, performance.now() - started));
                        if (observations.size > 64) observations.delete(observations.keys().next().value!);
                    }
                    let map = arrayToMap(items);
                    // Preserve updates/deletions received while the list was in flight.
                    map = applyResourceEvents(map, journal);
                    setSnapshot({ context: dataKey, scopeKey, map });
                    loadedScope.current = { context: dataKey, key: scopeKey };
                    writeResourceSnapshot(cacheKey, map);
                    setError(null);
                } catch (err) {
                    if (cancelled) return;
                    deleteResourceSnapshot(cacheKey);
                    if (broad && !selectionRef.current.has('*') && isNamespaceForbidden(err)) {
                        setBroadDenied(permissionKey);
                    } else {
                        setError(err instanceof Error ? err : new Error(String(err)));
                        checkConnectionError(err);
                    }
                } finally {
                    if (eventJournal.current === journal) eventJournal.current = null;
                    running = false;
                    if (!cancelled) setLoading(false);
                }
            };
            void fetchData();
            const unregister = connectionMode === 'polling' ? registerPolling(fetchData) : undefined;
            return () => {
                cancelled = true;
                cancelPages();
                activeRequests.forEach(id => { void CancelListRequest(id).catch(() => {}); });
                unregister?.();
            };
        }, [currentContext, scopeKey, isVisible, lastRefresh, refreshVersion, reconcileToken, connectionMode, registerPolling, checkConnectionError, resourceType, listFn]);

        const handleEvents = useCallback((events: any[]) => {
            const matching = events.filter(event => {
                if (event.context && event.context !== currentContext) return false;
                const namespace = event.resource?.metadata?.namespace ?? event.namespace;
                return (broad || scope.includes(namespace)) && event.resource?.metadata?.uid;
            });
            eventJournal.current?.push(...matching);
            setSnapshot(previous => {
                if (previous.context !== dataKey || previous.scopeKey !== scopeKey) return previous;
                const map = applyResourceEvents(previous.map, matching);
                return map === previous.map ? previous : { ...previous, map };
            });
        }, [dataKey, scopeKey]);
        const handleEvent = useMemo(() => Object.assign((event: any) => handleEvents([event]), { batch: handleEvents }), [handleEvents]);

        const handleWatchError = useCallback((err: unknown, namespace: string) => {
            if (broad && namespace === '' && !selectedSet.has('*') && isNamespaceForbidden(err)) {
                setBroadDenied(permissionKey);
            } else {
                setError(err instanceof Error ? err : new Error(String(err)));
            }
        }, [broad, selectedSet, permissionKey]);
        useEffect(() => {
            if (!isVisible || !currentContext || connectionMode !== 'streaming') return;
            return EventsOn('watcher-error', (event: any) => {
                if (event?.resourceType !== resourceType || (event.context && event.context !== currentContext)) return;
                if (!scope.includes(event.namespace || '')) return;
                if (isNamespaceForbidden(event.error)) handleWatchError(event.error, event.namespace || '');
            });
        }, [currentContext, scopeKey, handleWatchError, isVisible, connectionMode]);
        useWatcher(resourceType, scope, handleEvent, Boolean(currentContext && isVisible && scope.length), handleWatchError);

        // A scope/context change must never briefly render a successful empty
        // state before the new effect has had a chance to start its request.
        const awaitingScope = Boolean(currentContext && isVisible && scope.length && !error &&
            (loadedScope.current?.context !== dataKey || loadedScope.current?.key !== scopeKey));
        const visibleLoading = loading || awaitingScope;
        const loadState: ResourceLoadState = {
            phase: error ? 'incomplete' : visibleLoading ? (refreshing ? 'refreshing' : 'loading') : 'complete',
            error, progress: loadingProgress, retry: refetch,
        };
        return { loading: visibleLoading, error, loadingProgress, loadState, refetch, [stateName]: data };
    };
}

/**
 * Factory function to create a hook for cluster-scoped Kubernetes resources.
 * These resources don't have namespaces (e.g., nodes, PVs, StorageClasses).
 * Internally uses Map<string, T> for O(1) event processing, exposes T[] for consumers.
 */
export function createClusterScopedResourceHook<T extends K8sResource>(
    resourceType: string,
    listFn: ClusterScopedListFn<T>,
    stateName: string
) {
    const useResource = createNamespacedResourceHook<T>(resourceType, id => listFn(id), stateName);
    return (currentContext: string | null, isVisible: boolean) => useResource(currentContext, ['*'], isVisible);
}

