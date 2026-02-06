/**
 * Event Field Mappings
 *
 * Event-specific fields for advanced search filtering.
 */

export const eventFields = {
    name: {
        extractor: (item: any) => item.metadata?.name || '',
        aliases: ['n']
    },

    namespace: {
        extractor: (item: any) => item.metadata?.namespace || '',
        aliases: ['ns']
    },

    type: {
        extractor: (item: any) => item.type || '',
        aliases: ['t', 'kind']
    },

    reason: {
        extractor: (item: any) => item.reason || '',
        aliases: ['r']
    },

    message: {
        extractor: (item: any) => item.message || '',
        aliases: ['msg', 'm']
    },

    object: {
        extractor: (item: any) => {
            const obj = item.involvedObject;
            if (!obj) return '';
            return `${obj.kind}/${obj.name}`;
        },
        aliases: ['involvedobject', 'involved', 'obj']
    },

    objectkind: {
        extractor: (item: any) => item.involvedObject?.kind || '',
        aliases: ['objkind', 'ok']
    },

    objectname: {
        extractor: (item: any) => item.involvedObject?.name || '',
        aliases: ['objname', 'on']
    },

    source: {
        extractor: (item: any) => {
            const src = item.source;
            if (!src) return '';
            return `${src.component || ''}/${src.host || ''}`;
        },
        aliases: ['src']
    },

    count: {
        extractor: (item: any) => String(item.count || 1),
        aliases: ['c']
    },

    uid: {
        extractor: (item: any) => item.metadata?.uid || '',
        aliases: []
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
    }
};
