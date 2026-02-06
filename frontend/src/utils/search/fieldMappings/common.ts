/**
 * Common Field Mappings
 *
 * Shared fields that apply to all Kubernetes resources.
 * Each field has an extractor function and optional aliases.
 */

export const commonFields = {
    name: {
        extractor: (item: any) => item.metadata?.name || '',
        aliases: ['n']
    },
    namespace: {
        extractor: (item: any) => item.metadata?.namespace || '',
        aliases: ['ns']
    },
    labels: {
        extractor: (item: any) => {
            const labels = item.metadata?.labels || {};
            return Object.entries(labels)
                .map(([k, v]) => `${k}=${v}`)
                .join(' ');
        },
        aliases: ['label', 'l']
    },
    annotations: {
        extractor: (item: any) => {
            const annotations = item.metadata?.annotations || {};
            return Object.entries(annotations)
                .map(([k, v]) => `${k}=${v}`)
                .join(' ');
        },
        aliases: ['annotation', 'a']
    },
    uid: {
        extractor: (item: any) => item.metadata?.uid || '',
        aliases: []
    }
};
