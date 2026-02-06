/**
 * ConfigMap Field Mappings
 *
 * ConfigMap-specific fields for advanced search filtering.
 * Extends common fields with ConfigMap-specific extractors.
 */

import { commonFields } from './common';

export const configMapFields = {
    ...commonFields,

    keys: {
        extractor: (item: any) => Object.keys(item.data || {}).join(' '),
        aliases: ['key', 'datakeys']
    },

    keycount: {
        extractor: (item: any) => String(Object.keys(item.data || {}).length),
        aliases: ['count']
    },

    binarykeys: {
        extractor: (item: any) => Object.keys(item.binaryData || {}).join(' '),
        aliases: ['binarykey', 'binary']
    }
};
