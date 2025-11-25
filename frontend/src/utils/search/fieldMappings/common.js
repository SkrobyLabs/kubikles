/**
 * Common Field Mappings
 *
 * Shared fields that apply to all Kubernetes resources.
 * Each field has an extractor function and optional aliases.
 */

export const commonFields = {
    name: {
        extractor: (item) => item.metadata?.name || '',
        aliases: ['n']
    },
    namespace: {
        extractor: (item) => item.metadata?.namespace || '',
        aliases: ['ns']
    },
    labels: {
        extractor: (item) => {
            const labels = item.metadata?.labels || {};
            return Object.entries(labels)
                .map(([k, v]) => `${k}=${v}`)
                .join(' ');
        },
        aliases: ['label', 'l']
    },
    annotations: {
        extractor: (item) => {
            const annotations = item.metadata?.annotations || {};
            return Object.entries(annotations)
                .map(([k, v]) => `${k}=${v}`)
                .join(' ');
        },
        aliases: ['annotation', 'a']
    },
    uid: {
        extractor: (item) => item.metadata?.uid || '',
        aliases: []
    }
};
