import { useEffect, useRef, useMemo } from 'react';
import { SubscribeResourceWatcher, SubscribeCRDWatcher, UnsubscribeWatcher } from '../../wailsjs/go/main/App';

/**
 * Creates a stable key from namespaces array that doesn't change on array reordering.
 * @param {string|string[]} namespaces
 * @returns {string}
 */
export const createNamespaceKey = (namespaces) => {
    if (!namespaces) return '';
    const arr = Array.isArray(namespaces) ? namespaces : [namespaces];
    return arr.slice().sort().join(',');
};

/**
 * Generic hook for subscribing to resource watch events with reference counting.
 * The backend manages watchers with a 5-second delayed cleanup, so quick navigation
 * between views won't cause reconnection churn.
 *
 * @param {string} resourceType - The resource type (e.g., "namespaces", "deployments")
 * @param {string|string[]} namespaces - Namespace(s) to watch (empty string for cluster-scoped)
 * @param {Function} onEvent - Callback for handling events. Receives: { type, resourceType, namespace, resource }
 * @param {boolean} enabled - Whether watching is enabled
 */
export const useResourceWatcher = (resourceType, namespaces, onEvent, enabled = true) => {
    const onEventRef = useRef(onEvent);

    // Create stable namespace key to avoid unnecessary effect re-runs on array reorder
    const namespaceKey = useMemo(() => createNamespaceKey(namespaces), [namespaces]);

    // Keep callback ref updated without causing effect reruns
    useEffect(() => {
        onEventRef.current = onEvent;
    }, [onEvent]);

    useEffect(() => {
        if (!enabled || !window.runtime) return;

        const namespacesToWatch = Array.isArray(namespaces) ? namespaces : [namespaces];

        // Track keys for this specific effect instance
        let subscribedKeys = [];
        let isMounted = true;

        // Subscribe to watchers for each namespace
        const subscribe = async () => {
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
                } catch (err) {
                    console.error(`Failed to subscribe to ${resourceType} watcher:`, err);
                }
            }
        };

        subscribe();

        // Event listener (filters by resourceType, checks mount state to prevent updates after cleanup)
        const handleEvent = (event) => {
            if (isMounted && event.resourceType === resourceType) {
                onEventRef.current(event);
            }
        };

        // Batch event listener (for coalesced events from 60fps frame batching)
        const handleBatchEvents = (events) => {
            if (!isMounted || !Array.isArray(events)) return;
            for (const event of events) {
                if (event.resourceType === resourceType) {
                    onEventRef.current(event);
                }
            }
        };

        window.runtime.EventsOn("resource-event", handleEvent);
        window.runtime.EventsOn("resource-events-batch", handleBatchEvents);

        // Cleanup: unsubscribe all watchers
        return () => {
            isMounted = false;
            window.runtime.EventsOff("resource-event", handleEvent);
            window.runtime.EventsOff("resource-events-batch", handleBatchEvents);

            // Unsubscribe all keys that were subscribed during this effect
            subscribedKeys.forEach(key => {
                UnsubscribeWatcher(key).catch(err => {
                    console.error(`Failed to unsubscribe watcher ${key}:`, err);
                });
            });
        };
    }, [resourceType, namespaceKey, enabled]);
};

/**
 * Hook for subscribing to CRD watch events using Group/Version/Resource.
 *
 * @param {string} group - API group (e.g., "traefik.io")
 * @param {string} version - API version (e.g., "v1alpha1")
 * @param {string} resource - Resource plural name (e.g., "ingressroutes")
 * @param {string|string[]} namespaces - Namespace(s) to watch (empty string for cluster-scoped)
 * @param {Function} onEvent - Callback for handling events
 * @param {boolean} enabled - Whether watching is enabled
 */
export const useCRDWatcher = (group, version, resource, namespaces, onEvent, enabled = true) => {
    const onEventRef = useRef(onEvent);

    // Create stable namespace key to avoid unnecessary effect re-runs on array reorder
    const namespaceKey = useMemo(() => createNamespaceKey(namespaces), [namespaces]);

    // Keep callback ref updated
    useEffect(() => {
        onEventRef.current = onEvent;
    }, [onEvent]);

    // Generate the expected resourceType for CRD events
    const crdResourceType = `crd:${group}/${version}/${resource}`;

    useEffect(() => {
        if (!enabled || !window.runtime) return;

        const namespacesToWatch = Array.isArray(namespaces) ? namespaces : [namespaces];

        // Track keys for this specific effect instance
        let subscribedKeys = [];
        let isMounted = true;

        // Subscribe to CRD watchers for each namespace
        const subscribe = async () => {
            for (const ns of namespacesToWatch) {
                if (!isMounted) break;
                try {
                    const key = await SubscribeCRDWatcher(group, version, resource, ns || '');
                    if (key && isMounted) {
                        subscribedKeys.push(key);
                    } else if (key && !isMounted) {
                        UnsubscribeWatcher(key).catch(() => {});
                    }
                } catch (err) {
                    console.error(`Failed to subscribe to CRD watcher ${group}/${version}/${resource}:`, err);
                }
            }
        };

        subscribe();

        // Event listener (filters by CRD resourceType, checks mount state to prevent updates after cleanup)
        const handleEvent = (event) => {
            if (isMounted && event.resourceType === crdResourceType) {
                onEventRef.current(event);
            }
        };

        // Batch event listener (for coalesced events from 60fps frame batching)
        const handleBatchEvents = (events) => {
            if (!isMounted || !Array.isArray(events)) return;
            for (const event of events) {
                if (event.resourceType === crdResourceType) {
                    onEventRef.current(event);
                }
            }
        };

        window.runtime.EventsOn("resource-event", handleEvent);
        window.runtime.EventsOn("resource-events-batch", handleBatchEvents);

        // Cleanup
        return () => {
            isMounted = false;
            window.runtime.EventsOff("resource-event", handleEvent);
            window.runtime.EventsOff("resource-events-batch", handleBatchEvents);

            subscribedKeys.forEach(key => {
                UnsubscribeWatcher(key).catch(err => {
                    console.error(`Failed to unsubscribe CRD watcher ${key}:`, err);
                });
            });
        };
    }, [group, version, resource, crdResourceType, namespaceKey, enabled]);
};
