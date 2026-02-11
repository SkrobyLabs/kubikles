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
 * React setState function type for resource lists
 */
type SetResourceState<T extends K8sResource = K8sResource> = React.Dispatch<React.SetStateAction<T[]>>;

/**
 * Creates a state updater function for resource watch events.
 * Handles ADDED, MODIFIED, and DELETED events using the resource's metadata.uid.
 *
 * @example
 * const handleEvent = useCallback(createResourceEventHandler(setNamespaces), []);
 * useResourceWatcher("namespaces", "", handleEvent, isVisible);
 */
export const createResourceEventHandler = <T extends K8sResource = K8sResource>(
    setState: SetResourceState<T>
): ResourceEventHandler<T> => (event: ResourceEvent<T>): void => {
    const { type, resource } = event;

    setState(prev => {
        const uid = resource?.metadata?.uid;
        if (!uid) return prev;

        switch (type) {
            case 'ADDED':
                // Avoid duplicates - check if resource already exists
                if (prev.find((r: any) => r.metadata?.uid === uid)) {
                    return prev;
                }
                return [...prev, resource];

            case 'MODIFIED': {
                // Replace the existing resource, or add if not found (handles race condition)
                const exists = prev.some((r: any) => r.metadata?.uid === uid);
                if (exists) {
                    return prev.map((r: any) => r.metadata?.uid === uid ? resource : r);
                }
                // MODIFIED arrived before ADDED - treat as add, but NOT if the
                // resource is being deleted (has deletionTimestamp). A MODIFIED
                // for a non-existent resource with deletionTimestamp means the
                // DELETE was already processed and this is a stale event.
                if (resource?.metadata?.deletionTimestamp) {
                    return prev;
                }
                return [...prev, resource];
            }

            case 'DELETED':
                // Remove the resource from the list
                return prev.filter((r: any) => r.metadata?.uid !== uid);

            default:
                return prev;
        }
    });
};

/**
 * Creates a namespaced resource event handler that only processes events
 * for resources in the specified namespaces.
 *
 * @example
 * const handleEvent = useCallback(
 *   createNamespacedResourceEventHandler(setDeployments, selectedNamespaces),
 *   [selectedNamespaces]
 * );
 */
export const createNamespacedResourceEventHandler = <T extends K8sResource = K8sResource>(
    setState: SetResourceState<T>,
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
                if (prev.find((r: any) => r.metadata?.uid === uid)) {
                    return prev;
                }
                return [...prev, resource];

            case 'MODIFIED': {
                // Replace the existing resource, or add if not found (handles race condition)
                const exists = prev.some((r: any) => r.metadata?.uid === uid);
                if (exists) {
                    return prev.map((r: any) => r.metadata?.uid === uid ? resource : r);
                }
                // MODIFIED arrived before ADDED - treat as add, but NOT if the
                // resource is being deleted (has deletionTimestamp). A MODIFIED
                // for a non-existent resource with deletionTimestamp means the
                // DELETE was already processed and this is a stale event.
                if (resource?.metadata?.deletionTimestamp) {
                    return prev;
                }
                return [...prev, resource];
            }

            case 'DELETED':
                return prev.filter((r: any) => r.metadata?.uid !== uid);

            default:
                return prev;
        }
    });
};
