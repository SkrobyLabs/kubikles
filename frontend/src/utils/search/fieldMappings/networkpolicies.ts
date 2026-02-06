/**
 * NetworkPolicy Field Mappings
 *
 * NetworkPolicy-specific fields for advanced search filtering.
 */

import { commonFields } from './common';

export const networkPolicyFields = {
    ...commonFields,

    podselector: {
        extractor: (item: any) => {
            const selector = item.spec?.podSelector?.matchLabels || {};
            if (Object.keys(selector).length === 0) return 'all';
            return Object.entries(selector)
                .map(([k, v]) => `${k}=${v}`)
                .join(' ');
        },
        aliases: ['selector', 'pods']
    },

    policytype: {
        extractor: (item: any) => (item.spec?.policyTypes || ['Ingress']).join(' '),
        aliases: ['policytypes', 'type', 'types']
    },

    ingress: {
        extractor: (item: any) => {
            const rules = item.spec?.ingress || [];
            return rules.length > 0 ? 'true' : 'false';
        },
        aliases: ['hasingress']
    },

    egress: {
        extractor: (item: any) => {
            const rules = item.spec?.egress || [];
            return rules.length > 0 ? 'true' : 'false';
        },
        aliases: ['hasegress']
    },

    ingresscount: {
        extractor: (item: any) => String(item.spec?.ingress?.length || 0),
        aliases: ['ingressrules']
    },

    egresscount: {
        extractor: (item: any) => String(item.spec?.egress?.length || 0),
        aliases: ['egressrules']
    }
};
