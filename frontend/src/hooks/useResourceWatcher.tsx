import { useEffect, useRef, useMemo } from 'react';
import { SubscribeResourceWatcher, SubscribeCRDWatcher, UnsubscribeWatcher } from 'wailsjs/go/main/App';
import { EventsOn, EventsOff } from 'wailsjs/runtime/runtime';

// Resource event structure
interface ResourceEvent {
    type: string;
    resourceType: string;
    namespace?: string;
    resource?: any;
}

// Event handler callback type
type ResourceEventHandler = (event: ResourceEvent) => void;

/**
 * Creates a stable key from namespaces array that doesn't change on array reordering.
 */
export const createNamespaceKey = (namespaces: string | string[] | null | undefined): string => {
    if (!namespaces) return '';
    const arr = Array.isArray(namespaces) ? namespaces : [namespaces];
    return arr.slice().sort().join(',');
};

/**
 * Generic hook for subscribing to resource watch events with reference counting.
 * The backend manages watchers with a 5-second delayed cleanup, so quick navigation
 * between views won't cause reconnection churn.
 */
export const useResourceWatcher = (
    resourceType: string,
    namespaces: string | string[],
    onEvent: ResourceEventHandler,
    enabled: boolean = true
): void => {
    const onEventRef = useRef<ResourceEventHandler>(onEvent);

    // Create stable namespace key to avoid unnecessary effect re-runs on array reorder
    const namespaceKey = useMemo(() => createNamespaceKey(namespaces), [namespaces]);

    // Keep callback ref updated without causing effect reruns
    useEffect(() => {
        onEventRef.current = onEvent;
    }, [onEvent]);

    useEffect(() => {
        if (!enabled) return;

        const namespacesToWatch = Array.isArray(namespaces) ? namespaces : [namespaces];

        // Track keys for this specific effect instance
        const subscribedKeys: string[] = [];
        let isMounted = true;

        // Subscribe to watchers for each namespace
        const subscribe = async (): Promise<void> => {
            for (const ns of namespacesToWatch) {
                if (!isMounted) break; // Stop if unmounted
                try {
                    const key = await SubscribeResourceWatcher(resourceType, ns || '');
                    if (key && isMounted) {
                        subscribedKeys.push(key);
                    } else if (key && !isMounted) {
                        // Subscribed but already unmounted - unsubscribe immediately
                        UnsubscribeWatcher(key).catch(() => {});
                    }
                } catch (err: any) {
                    console.error(`Failed to subscribe to ${resourceType} watcher:`, err);
                }
            }
        };

        subscribe();

        // Event listener (filters by resourceType, checks mount state to prevent updates after cleanup)
        const handleEvent = (event: ResourceEvent): void => {
            if (isMounted && event.resourceType === resourceType) {
                onEventRef.current(event);
            }
        };

        // Batch event listener (for coalesced events from 60fps frame batching)
        const handleBatchEvents = (events: ResourceEvent[]): void => {
            if (!isMounted || !Array.isArray(events)) return;
            for (const event of events) {
                if (event.resourceType === resourceType) {
                    onEventRef.current(event);
                }
            }
        };

        EventsOn("resource-event", handleEvent);
        EventsOn("resource-events-batch", handleBatchEvents);

        // Cleanup: unsubscribe all watchers
        return () => {
            isMounted = false;
            EventsOff("resource-event", handleEvent);
            EventsOff("resource-events-batch", handleBatchEvents);

            // Unsubscribe all keys that were subscribed during this effect
            subscribedKeys.forEach((key: any) => {
                UnsubscribeWatcher(key).catch((err: any) => {
                    console.error(`Failed to unsubscribe watcher ${key}:`, err);
                });
            });
        };
    }, [resourceType, namespaceKey, enabled]);
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
    enabled: boolean = true
): void => {
    const onEventRef = useRef<ResourceEventHandler>(onEvent);

    // Create stable namespace key to avoid unnecessary effect re-runs on array reorder
    const namespaceKey = useMemo(() => createNamespaceKey(namespaces), [namespaces]);

    // Keep callback ref updated
    useEffect(() => {
        onEventRef.current = onEvent;
    }, [onEvent]);

    // Generate the expected resourceType for CRD events
    const crdResourceType = `crd:${group}/${version}/${resource}`;

    useEffect(() => {
        if (!enabled) return;

        const namespacesToWatch = Array.isArray(namespaces) ? namespaces : [namespaces];

        // Track keys for this specific effect instance
        const subscribedKeys: string[] = [];
        let isMounted = true;

        // Subscribe to CRD watchers for each namespace
        const subscribe = async (): Promise<void> => {
            for (const ns of namespacesToWatch) {
                if (!isMounted) break;
                try {
                    const key = await SubscribeCRDWatcher(group, version, resource, ns || '');
                    if (key && isMounted) {
                        subscribedKeys.push(key);
                    } else if (key && !isMounted) {
                        UnsubscribeWatcher(key).catch(() => {});
                    }
                } catch (err: any) {
                    console.error(`Failed to subscribe to CRD watcher ${group}/${version}/${resource}:`, err);
                }
            }
        };

        subscribe();

        // Event listener (filters by CRD resourceType, checks mount state to prevent updates after cleanup)
        const handleEvent = (event: ResourceEvent): void => {
            if (isMounted && event.resourceType === crdResourceType) {
                onEventRef.current(event);
            }
        };

        // Batch event listener (for coalesced events from 60fps frame batching)
        const handleBatchEvents = (events: ResourceEvent[]): void => {
            if (!isMounted || !Array.isArray(events)) return;
            for (const event of events) {
                if (event.resourceType === crdResourceType) {
                    onEventRef.current(event);
                }
            }
        };

        EventsOn("resource-event", handleEvent);
        EventsOn("resource-events-batch", handleBatchEvents);

        // Cleanup
        return () => {
            isMounted = false;
            EventsOff("resource-event", handleEvent);
            EventsOff("resource-events-batch", handleBatchEvents);

            subscribedKeys.forEach((key: any) => {
                UnsubscribeWatcher(key).catch((err: any) => {
                    console.error(`Failed to unsubscribe CRD watcher ${key}:`, err);
                });
            });
        };
    }, [group, version, resource, crdResourceType, namespaceKey, enabled]);
};
