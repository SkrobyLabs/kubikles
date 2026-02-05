/**
 * Utility to check if we should optimize namespace queries.
 * If all namespaces are selected, returns '' to fetch from all in a single query.
 * If no namespaces are selected, returns null to indicate no query should be made.
 * Otherwise returns the array of namespaces to query individually.
 *
 * Special marker '*' means "all namespaces" - this will always fetch from all in one query.
 * This ensures that when new namespaces are added, they're automatically included.
 *
 * @param {string|string[]} selectedNamespaces - Selected namespace(s), may include '*' marker for all
 * @param {string[]} allAvailableNamespaces - All available namespaces
 * @returns {string|string[]|null} Either '' for all namespaces, array of specific namespaces, or null for none
 */
export const optimizeNamespaceQuery = (selectedNamespaces, allAvailableNamespaces) => {
    const namespacesToFetch = Array.isArray(selectedNamespaces) ? selectedNamespaces : [selectedNamespaces];
    const allAvailableNs = allAvailableNamespaces.filter(ns => ns !== '');

    // If empty array, no namespaces selected - return null to skip query
    if (namespacesToFetch.length === 0) {
        return null;
    }

    // Check for special "all" marker
    if (namespacesToFetch.includes('*')) {
        return '';
    }

    // If all namespaces are selected individually, fetch from all in one query
    const shouldFetchAll = allAvailableNs.length > 0 &&
                          namespacesToFetch.length === allAvailableNs.length &&
                          allAvailableNs.every(ns => namespacesToFetch.includes(ns));

    return shouldFetchAll ? '' : namespacesToFetch;
};
