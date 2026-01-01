/**
 * LimitRange Field Mappings
 *
 * LimitRange-specific fields for advanced search filtering.
 */

import { commonFields } from './common';

export const limitRangeFields = {
    ...commonFields,

    limittype: {
        extractor: (item) => {
            const limits = item.spec?.limits || [];
            return limits.map(l => l.type).join(' ');
        },
        aliases: ['type', 'types']
    },

    limitcount: {
        extractor: (item) => String(item.spec?.limits?.length || 0),
        aliases: ['count', 'limits']
    },

    hascontainer: {
        extractor: (item) => {
            const limits = item.spec?.limits || [];
            return limits.some(l => l.type === 'Container') ? 'true' : 'false';
        },
        aliases: ['container']
    },

    haspod: {
        extractor: (item) => {
            const limits = item.spec?.limits || [];
            return limits.some(l => l.type === 'Pod') ? 'true' : 'false';
        },
        aliases: ['pod']
    },

    haspvc: {
        extractor: (item) => {
            const limits = item.spec?.limits || [];
            return limits.some(l => l.type === 'PersistentVolumeClaim') ? 'true' : 'false';
        },
        aliases: ['pvc']
    }
};
