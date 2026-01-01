/**
 * NetworkPolicy Field Mappings
 *
 * NetworkPolicy-specific fields for advanced search filtering.
 */

import { commonFields } from './common';

export const networkPolicyFields = {
    ...commonFields,

    podselector: {
        extractor: (item) => {
            const selector = item.spec?.podSelector?.matchLabels || {};
            if (Object.keys(selector).length === 0) return 'all';
            return Object.entries(selector)
                .map(([k, v]) => `${k}=${v}`)
                .join(' ');
        },
        aliases: ['selector', 'pods']
    },

    policytype: {
        extractor: (item) => (item.spec?.policyTypes || ['Ingress']).join(' '),
        aliases: ['policytypes', 'type', 'types']
    },

    ingress: {
        extractor: (item) => {
            const rules = item.spec?.ingress || [];
            return rules.length > 0 ? 'true' : 'false';
        },
        aliases: ['hasingress']
    },

    egress: {
        extractor: (item) => {
            const rules = item.spec?.egress || [];
            return rules.length > 0 ? 'true' : 'false';
        },
        aliases: ['hasegress']
    },

    ingresscount: {
        extractor: (item) => String(item.spec?.ingress?.length || 0),
        aliases: ['ingressrules']
    },

    egresscount: {
        extractor: (item) => String(item.spec?.egress?.length || 0),
        aliases: ['egressrules']
    }
};
