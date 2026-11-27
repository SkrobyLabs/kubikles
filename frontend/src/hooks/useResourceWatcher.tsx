import { useEffect, useRef, useMemo } from 'react';
import { SubscribeResourceWatcher, SubscribeResourceWatcherFrom, SubscribeCRDWatcher, SubscribeCRDWatcherFrom, UnsubscribeWatcher } from 'wailsjs/go/main/App';
import { EventsOn } from 'wailsjs/runtime/runtime';
import { useK8s } from '../context';

// Resource event structure
interface ResourceEvent {
    type: string;
    resourceType: string;
    namespace?: string;
    context?: string;
    resource?: any;
}

// Event handler callback type
type ResourceEventHandler = ((event: ResourceEvent) => void) & { batch?: (events: ResourceEvent[]) => void };

/**
 * Creates a stable key from namespaces array that doesn't change on array reordering.
 */
export const createNamespaceKey = (namespaces: string | string[] | null | undefined): string => {
    if (!namespaces) return '';
    const arr = Array.isArray(namespaces) ? namespaces : [namespaces];
    return arr.slice().sort().join(',');
};

/**
 * Controls when and how a hook asks the backend for watchers.
 *
 * Event listeners always attach while the hook is enabled, so events from an
 * already-running (shared) watcher are seen while a list is still in flight and
 * can be journalled. Backend subscriptions can be deferred until the caller has
 * a list to anchor them to: a watcher started from the list's resourceVersion
 * only streams what changed afterwards, instead of replaying every object.
 */
export interface WatcherSubscribeOptions {
    /** False keeps the listeners attached but defers the backend subscription. Defaults to true. */
    subscribe?: boolean;
    /**
     * resourceVersion of the list this caller just completed for the namespace.
     * Returning nothing (or rejecting) subscribes without an anchor, which makes
     * a new watcher replay current state; a watcher that is already running is
     * unaffected either way.
     */
    resourceVersionFor?: (namespace: string) => string | undefined | Promise<string | undefined>;
}

async function anchorFor(options: WatcherSubscribeOptions | undefined, namespace: string): Promise<string | undefined> {
    if (!options?.resourceVersionFor) return undefined;
    try {
        return (await options.resourceVersionFor(namespace)) || undefined;
    } catch {
        return undefined;
    }
}

/**
 * Attaches the single-event and batch listeners for events matching a
 * resource type. Cleanup removes only this hook's listeners.
 */
function useEventListeners(
    matchType: string,
    onEventRef: { current: ResourceEventHandler },
    enabled: boolean,
    currentContext: string | null | undefined,
): void {
    useEffect(() => {
        if (!enabled) return;
        let isMounted = true;
        const handleEvent = (event: ResourceEvent): void => {
            if (isMounted && (!event.context || event.context === currentContext) && event.resourceType === matchType) {
                onEventRef.current(event);
            }
        };
        // Batch listener (coalesced events from frame batching)
        const handleBatchEvents = (events: ResourceEvent[]): void => {
            if (!isMounted || !Array.isArray(events)) return;
            const matching = events.filter(event => (!event.context || event.context === currentContext) && event.resourceType === matchType);
            if (onEventRef.current.batch) onEventRef.current.batch(matching);
            else matching.forEach(event => onEventRef.current(event));
        };
        const cancelEvent = EventsOn("resource-event", handleEvent);
        const cancelBatch = EventsOn("resource-events-batch", handleBatchEvents);
        return () => {
            isMounted = false;
            cancelEvent();
            cancelBatch();
        };
    }, [matchType, enabled, currentContext, onEventRef]);
}

/**
 * Subscribes to a backend watcher per namespace and unsubscribes on cleanup.
 * Namespaces subscribe concurrently; each failure is reported on its own.
 */
function useWatcherSubscriptions(
    describe: string,
    namespaceKey: string,
    namespaces: string | string[],
    enabled: boolean,
    onErrorRef: { current: ((error: unknown, namespace: string) => void) | undefined },
    subscribeOne: (namespace: string) => Promise<string>,
    deps: unknown[],
): void {
    useEffect(() => {
        if (!enabled) return;
        const namespacesToWatch = Array.isArray(namespaces) ? namespaces : [namespaces];
        const subscribedKeys: string[] = [];
        let isMounted = true;

        const subscribeNamespace = async (ns: string): Promise<void> => {
            try {
                const key = await subscribeOne(ns || '');
                if (key && isMounted) {
                    subscribedKeys.push(key);
                } else if (key && !isMounted) {
                    // Subscribed but already unmounted - unsubscribe immediately
                    UnsubscribeWatcher(key).catch(() => {});
                }
            } catch (err: any) {
                if (isMounted) onErrorRef.current?.(err, ns || '');
                console.error(`Failed to subscribe to ${describe} watcher:`, err);
            }
        };
        void Promise.all(namespacesToWatch.map(subscribeNamespace));

        return () => {
            isMounted = false;
            subscribedKeys.forEach((key: any) => {
                UnsubscribeWatcher(key).catch((err: any) => {
                    console.error(`Failed to unsubscribe ${describe} watcher ${key}:`, err);
                });
            });
        };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [...deps, namespaceKey, enabled]);
}

/**
 * Generic hook for subscribing to resource watch events with reference counting.
 * The backend manages watchers with a 5-second delayed cleanup, so quick navigation
 * between views won't cause reconnection churn.
 */
export const useResourceWatcher = (
    resourceType: string,
    namespaces: string | string[],
    onEvent: ResourceEventHandler,
    enabled: boolean = true,
    onError?: (error: unknown, namespace: string) => void,
    options?: WatcherSubscribeOptions,
): void => {
    const { connectionMode, currentContext } = useK8s();
    const onEventRef = useRef<ResourceEventHandler>(onEvent);
    const onErrorRef = useRef(onError);
    onErrorRef.current = onError;
    const optionsRef = useRef(options);
    optionsRef.current = options;

    // Create stable namespace key to avoid unnecessary effect re-runs on array reorder
    const namespaceKey = useMemo(() => createNamespaceKey(namespaces), [namespaces]);

    // Keep callback ref updated without causing effect reruns
    useEffect(() => {
        onEventRef.current = onEvent;
    }, [onEvent]);

    const streaming = enabled && connectionMode === 'streaming';
    useEventListeners(resourceType, onEventRef, streaming, currentContext);
    useWatcherSubscriptions(
        resourceType, namespaceKey, namespaces, streaming && options?.subscribe !== false, onErrorRef,
        async (ns) => {
            const resourceVersion = await anchorFor(optionsRef.current, ns);
            return resourceVersion
                ? SubscribeResourceWatcherFrom(resourceType, ns, resourceVersion)
                : SubscribeResourceWatcher(resourceType, ns);
        },
        [resourceType, currentContext],
    );
};

/**
 * Hook for subscribing to CRD watch events using Group/Version/Resource.
 */
export const useCRDWatcher = (
    group: string,
    version: string,
    resource: string,
    namespaces: string | string[],
    onEvent: ResourceEventHandler,
    enabled: boolean = true,
    onError?: (error: unknown, namespace: string) => void,
    options?: WatcherSubscribeOptions,
): void => {
    const { connectionMode, currentContext } = useK8s();
    const onEventRef = useRef<ResourceEventHandler>(onEvent);
    const onErrorRef = useRef(onError);
    onErrorRef.current = onError;
    const optionsRef = useRef(options);
    optionsRef.current = options;

    const namespaceKey = useMemo(() => createNamespaceKey(namespaces), [namespaces]);

    useEffect(() => {
        onEventRef.current = onEvent;
    }, [onEvent]);

    // Generate the expected resourceType for CRD events
    const crdResourceType = `crd:${group}/${version}/${resource}`;

    const streaming = enabled && connectionMode === 'streaming';
    useEventListeners(crdResourceType, onEventRef, streaming, currentContext);
    useWatcherSubscriptions(
        `CRD ${group}/${version}/${resource}`, namespaceKey, namespaces, streaming && options?.subscribe !== false, onErrorRef,
        async (ns) => {
            const resourceVersion = await anchorFor(optionsRef.current, ns);
            return resourceVersion
                ? SubscribeCRDWatcherFrom(group, version, resource, ns, resourceVersion)
                : SubscribeCRDWatcher(group, version, resource, ns);
        },
        [group, version, resource, currentContext],
    );
};
