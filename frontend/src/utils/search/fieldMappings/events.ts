/**
 * Event Field Mappings
 *
 * Event-specific fields for advanced search filtering.
 */

export const eventFields = {
    name: {
        extractor: (item) => item.metadata?.name || '',
        aliases: ['n']
    },

    namespace: {
        extractor: (item) => item.metadata?.namespace || '',
        aliases: ['ns']
    },

    type: {
        extractor: (item) => item.type || '',
        aliases: ['t', 'kind']
    },

    reason: {
        extractor: (item) => item.reason || '',
        aliases: ['r']
    },

    message: {
        extractor: (item) => item.message || '',
        aliases: ['msg', 'm']
    },

    object: {
        extractor: (item) => {
            const obj = item.involvedObject;
            if (!obj) return '';
            return `${obj.kind}/${obj.name}`;
        },
        aliases: ['involvedobject', 'involved', 'obj']
    },

    objectkind: {
        extractor: (item) => item.involvedObject?.kind || '',
        aliases: ['objkind', 'ok']
    },

    objectname: {
        extractor: (item) => item.involvedObject?.name || '',
        aliases: ['objname', 'on']
    },

    source: {
        extractor: (item) => {
            const src = item.source;
            if (!src) return '';
            return `${src.component || ''}/${src.host || ''}`;
        },
        aliases: ['src']
    },

    count: {
        extractor: (item) => String(item.count || 1),
        aliases: ['c']
    },

    uid: {
        extractor: (item) => item.metadata?.uid || '',
        aliases: []
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
    }
};
