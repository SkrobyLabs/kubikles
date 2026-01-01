/**
 * Creates a state updater function for resource watch events.
 * Handles ADDED, MODIFIED, and DELETED events using the resource's metadata.uid.
 *
 * @param {Function} setState - React setState function for the resource list
 * @returns {Function} Event handler that can be passed to useResourceWatcher
 *
 * @example
 * const handleEvent = useCallback(createResourceEventHandler(setNamespaces), []);
 * useResourceWatcher("namespaces", "", handleEvent, isVisible);
 */
export const createResourceEventHandler = (setState) => (event) => {
    const { type, resource } = event;

    setState(prev => {
        const uid = resource?.metadata?.uid;
        if (!uid) return prev;

        switch (type) {
            case 'ADDED':
                // Avoid duplicates - check if resource already exists
                if (prev.find(r => r.metadata?.uid === uid)) {
                    return prev;
                }
                return [...prev, resource];

            case 'MODIFIED': {
                // Replace the existing resource, or add if not found (handles race condition)
                const exists = prev.some(r => r.metadata?.uid === uid);
                if (exists) {
                    return prev.map(r => r.metadata?.uid === uid ? resource : r);
                }
                // MODIFIED arrived before ADDED - treat as add
                return [...prev, resource];
            }

            case 'DELETED':
                // Remove the resource from the list
                return prev.filter(r => r.metadata?.uid !== uid);

            default:
                return prev;
        }
    });
};

/**
 * Creates a namespaced resource event handler that only processes events
 * for resources in the specified namespaces.
 *
 * @param {Function} setState - React setState function for the resource list
 * @param {string[]} selectedNamespaces - Array of namespaces to include, or ['*'] for all
 * @returns {Function} Event handler that filters by namespace
 *
 * @example
 * const handleEvent = useCallback(
 *   createNamespacedResourceEventHandler(setDeployments, selectedNamespaces),
 *   [selectedNamespaces]
 * );
 */
export const createNamespacedResourceEventHandler = (setState, selectedNamespaces) => (event) => {
    const { type, resource, namespace: eventNamespace } = event;

    // Check if we should process this event based on namespace
    const shouldProcess = selectedNamespaces.includes('*') ||
        selectedNamespaces.length === 0 ||
        selectedNamespaces.includes(eventNamespace);

    if (!shouldProcess) return;

    setState(prev => {
        const uid = resource?.metadata?.uid;
        if (!uid) return prev;

        switch (type) {
            case 'ADDED':
                if (prev.find(r => r.metadata?.uid === uid)) {
                    return prev;
                }
                return [...prev, resource];

            case 'MODIFIED': {
                // Replace the existing resource, or add if not found (handles race condition)
                const exists = prev.some(r => r.metadata?.uid === uid);
                if (exists) {
                    return prev.map(r => r.metadata?.uid === uid ? resource : r);
                }
                // MODIFIED arrived before ADDED - treat as add
                return [...prev, resource];
            }

            case 'DELETED':
                return prev.filter(r => r.metadata?.uid !== uid);

            default:
                return prev;
        }
    });
};
