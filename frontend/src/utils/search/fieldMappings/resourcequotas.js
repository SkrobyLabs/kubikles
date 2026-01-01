/**
 * ResourceQuota Field Mappings
 *
 * ResourceQuota-specific fields for advanced search filtering.
 */

import { commonFields } from './common';

export const resourceQuotaFields = {
    ...commonFields,

    hard: {
        extractor: (item) => {
            const hard = item.spec?.hard || {};
            return Object.entries(hard)
                .map(([k, v]) => `${k}=${v}`)
                .join(' ');
        },
        aliases: ['limits', 'resources']
    },

    used: {
        extractor: (item) => {
            const used = item.status?.used || {};
            return Object.entries(used)
                .map(([k, v]) => `${k}=${v}`)
                .join(' ');
        },
        aliases: ['usage']
    },

    scopes: {
        extractor: (item) => (item.spec?.scopes || []).join(' '),
        aliases: ['scope']
    },

    resourcecount: {
        extractor: (item) => String(Object.keys(item.spec?.hard || {}).length),
        aliases: ['count']
    }
};
