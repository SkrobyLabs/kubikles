/**
 * Service Field Mappings
 *
 * Service-specific fields for advanced search filtering.
 * Extends common fields with Service-specific extractors.
 */

import { commonFields } from './common';

export const serviceFields = {
    ...commonFields,

    type: {
        extractor: (item) => item.spec?.type || 'ClusterIP',
        aliases: ['servicetype']
    },

    clusterip: {
        extractor: (item) => item.spec?.clusterIP || '',
        aliases: ['ip', 'cluster-ip']
    },

    externalip: {
        extractor: (item) => (item.spec?.externalIPs || []).join(' '),
        aliases: ['external-ip', 'extip']
    },

    loadbalancerip: {
        extractor: (item) => item.status?.loadBalancer?.ingress?.map(i => i.ip || i.hostname).join(' ') || '',
        aliases: ['lbip', 'loadbalancer']
    },

    port: {
        extractor: (item) => {
            const ports = item.spec?.ports || [];
            return ports.map(p => String(p.port)).join(' ');
        },
        aliases: ['ports']
    },

    targetport: {
        extractor: (item) => {
            const ports = item.spec?.ports || [];
            return ports.map(p => String(p.targetPort)).join(' ');
        },
        aliases: ['targetports']
    },

    nodeport: {
        extractor: (item) => {
            const ports = item.spec?.ports || [];
            return ports.filter(p => p.nodePort).map(p => String(p.nodePort)).join(' ');
        },
        aliases: ['nodeports']
    },

    protocol: {
        extractor: (item) => {
            const ports = item.spec?.ports || [];
            return ports.map(p => p.protocol).join(' ');
        },
        aliases: ['protocols']
    },

    selector: {
        extractor: (item) => {
            const selector = item.spec?.selector || {};
            return Object.entries(selector)
                .map(([k, v]) => `${k}=${v}`)
                .join(' ');
        },
        aliases: ['sel']
    },

    sessionaffinity: {
        extractor: (item) => item.spec?.sessionAffinity || 'None',
        aliases: ['affinity', 'session']
    },

    externalname: {
        extractor: (item) => item.spec?.externalName || '',
        aliases: ['extname']
    },

    headless: {
        extractor: (item) => item.spec?.clusterIP === 'None' ? 'true' : 'false',
        aliases: []
    }
};
