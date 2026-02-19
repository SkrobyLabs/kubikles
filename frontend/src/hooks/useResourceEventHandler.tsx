import { K8sResource } from '../types/k8s';

/**
 * Resource watch event from Wails backend
 */
interface ResourceEvent<T extends K8sResource = K8sResource> {
    type: 'ADDED' | 'MODIFIED' | 'DELETED';
    resource: T;
    namespace?: string;
}

/**
 * Resource event handler function type
 */
type ResourceEventHandler<T extends K8sResource = K8sResource> = (event: ResourceEvent<T>) => void;

/**
 * React setState function type for resource maps (UID -> resource).
 * Using Map<string, T> for O(1) lookups on watch events.
 */
type SetResourceMapState<T extends K8sResource = K8sResource> = React.Dispatch<React.SetStateAction<Map<string, T>>>;

/**
 * Creates a state updater function for resource watch events.
 * Uses Map<string, T> keyed by UID for O(1) event processing.
 *
 * @example
 * const handleEvent = useCallback(createResourceEventHandler(setDataMap), []);
 * useResourceWatcher("namespaces", "", handleEvent, isVisible);
 */
export const createResourceEventHandler = <T extends K8sResource = K8sResource>(
    setState: SetResourceMapState<T>
): ResourceEventHandler<T> => (event: ResourceEvent<T>): void => {
    const { type, resource } = event;

    setState(prev => {
        const uid = resource?.metadata?.uid;
        if (!uid) return prev;

        switch (type) {
            case 'ADDED':
                // Avoid duplicates - O(1) check
                if (prev.has(uid)) return prev;
                { const next = new Map(prev); next.set(uid, resource); return next; }

            case 'MODIFIED': {
                // Replace existing or add if not found (handles race condition)
                if (prev.has(uid)) {
                    const next = new Map(prev);
                    next.set(uid, resource);
                    return next;
                }
                // MODIFIED arrived before ADDED - treat as add, but NOT if the
                // resource is being deleted (has deletionTimestamp). A MODIFIED
                // for a non-existent resource with deletionTimestamp means the
                // DELETE was already processed and this is a stale event.
                if (resource?.metadata?.deletionTimestamp) {
                    return prev;
                }
                const next = new Map(prev);
                next.set(uid, resource);
                return next;
            }

            case 'DELETED':
                // O(1) check and removal
                if (!prev.has(uid)) return prev;
                { const next = new Map(prev); next.delete(uid); return next; }

            default:
                return prev;
        }
    });
};

/**
 * Creates a namespaced resource event handler that only processes events
 * for resources in the specified namespaces.
 * Uses Map<string, T> keyed by UID for O(1) event processing.
 *
 * @example
 * const handleEvent = useCallback(
 *   createNamespacedResourceEventHandler(setDataMap, selectedNamespaces),
 *   [selectedNamespaces]
 * );
 */
export const createNamespacedResourceEventHandler = <T extends K8sResource = K8sResource>(
    setState: SetResourceMapState<T>,
    selectedNamespaces: string[]
): ResourceEventHandler<T> => (event: ResourceEvent<T>): void => {
    const { type, resource, namespace: eventNamespace } = event;

    // Check if we should process this event based on namespace
    const shouldProcess = selectedNamespaces.includes('*') ||
        selectedNamespaces.length === 0 ||
        selectedNamespaces.includes(eventNamespace || '');

    if (!shouldProcess) return;

    setState(prev => {
        const uid = resource?.metadata?.uid;
        if (!uid) return prev;

        switch (type) {
            case 'ADDED':
                if (prev.has(uid)) return prev;
                { const next = new Map(prev); next.set(uid, resource); return next; }

            case 'MODIFIED': {
                if (prev.has(uid)) {
                    const next = new Map(prev);
                    next.set(uid, resource);
                    return next;
                }
                // MODIFIED arrived before ADDED - treat as add, but NOT if the
                // resource is being deleted (has deletionTimestamp). A MODIFIED
                // for a non-existent resource with deletionTimestamp means the
                // DELETE was already processed and this is a stale event.
                if (resource?.metadata?.deletionTimestamp) {
                    return prev;
                }
                const next = new Map(prev);
                next.set(uid, resource);
                return next;
            }

            case 'DELETED':
                if (!prev.has(uid)) return prev;
                { const next = new Map(prev); next.delete(uid); return next; }

            default:
                return prev;
        }
    });
};
