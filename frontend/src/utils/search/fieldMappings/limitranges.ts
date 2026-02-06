/**
 * LimitRange Field Mappings
 *
 * LimitRange-specific fields for advanced search filtering.
 */

import { commonFields } from './common';

export const limitRangeFields = {
    ...commonFields,

    limittype: {
        extractor: (item: any) => {
            const limits = item.spec?.limits || [];
            return limits.map((l: any) => l.type).join(' ');
        },
        aliases: ['type', 'types']
    },

    limitcount: {
        extractor: (item: any) => String(item.spec?.limits?.length || 0),
        aliases: ['count', 'limits']
    },

    hascontainer: {
        extractor: (item: any) => {
            const limits = item.spec?.limits || [];
            return limits.some((l: any) => l.type === 'Container') ? 'true' : 'false';
        },
        aliases: ['container']
    },

    haspod: {
        extractor: (item: any) => {
            const limits = item.spec?.limits || [];
            return limits.some((l: any) => l.type === 'Pod') ? 'true' : 'false';
        },
        aliases: ['pod']
    },

    haspvc: {
        extractor: (item: any) => {
            const limits = item.spec?.limits || [];
            return limits.some((l: any) => l.type === 'PersistentVolumeClaim') ? 'true' : 'false';
        },
        aliases: ['pvc']
    }
};
