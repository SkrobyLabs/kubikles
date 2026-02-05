/**
 * Namespace Field Mappings
 *
 * Namespace-specific fields for advanced search filtering.
 * Note: Namespaces are cluster-scoped and don't have a namespace field.
 */

export const namespaceFields = {
    name: {
        extractor: (item) => item.metadata?.name || '',
        aliases: ['n']
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
    },

    status: {
        extractor: (item) => item.status?.phase || 'Unknown',
        aliases: ['phase', 'state']
    },

    finalizers: {
        extractor: (item) => (item.spec?.finalizers || []).join(' '),
        aliases: ['finalizer']
    }
};
