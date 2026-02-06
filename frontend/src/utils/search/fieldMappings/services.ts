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
        extractor: (item: any) => item.spec?.type || 'ClusterIP',
        aliases: ['servicetype']
    },

    clusterip: {
        extractor: (item: any) => item.spec?.clusterIP || '',
        aliases: ['ip', 'cluster-ip']
    },

    externalip: {
        extractor: (item: any) => (item.spec?.externalIPs || []).join(' '),
        aliases: ['external-ip', 'extip']
    },

    loadbalancerip: {
        extractor: (item: any) => item.status?.loadBalancer?.ingress?.map((i: any) => i.ip || i.hostname).join(' ') || '',
        aliases: ['lbip', 'loadbalancer']
    },

    port: {
        extractor: (item: any) => {
            const ports = item.spec?.ports || [];
            return ports.map((p: any) => String(p.port)).join(' ');
        },
        aliases: ['ports']
    },

    targetport: {
        extractor: (item: any) => {
            const ports = item.spec?.ports || [];
            return ports.map((p: any) => String(p.targetPort)).join(' ');
        },
        aliases: ['targetports']
    },

    nodeport: {
        extractor: (item: any) => {
            const ports = item.spec?.ports || [];
            return ports.filter((p: any) => p.nodePort).map((p: any) => String(p.nodePort)).join(' ');
        },
        aliases: ['nodeports']
    },

    protocol: {
        extractor: (item: any) => {
            const ports = item.spec?.ports || [];
            return ports.map((p: any) => p.protocol).join(' ');
        },
        aliases: ['protocols']
    },

    selector: {
        extractor: (item: any) => {
            const selector = item.spec?.selector || {};
            return Object.entries(selector)
                .map(([k, v]) => `${k}=${v}`)
                .join(' ');
        },
        aliases: ['sel']
    },

    sessionaffinity: {
        extractor: (item: any) => item.spec?.sessionAffinity || 'None',
        aliases: ['affinity', 'session']
    },

    externalname: {
        extractor: (item: any) => item.spec?.externalName || '',
        aliases: ['extname']
    },

    headless: {
        extractor: (item: any) => item.spec?.clusterIP === 'None' ? 'true' : 'false',
        aliases: []
    }
};
