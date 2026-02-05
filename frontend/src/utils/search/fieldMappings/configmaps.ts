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
        extractor: (item) => Object.keys(item.data || {}).join(' '),
        aliases: ['key', 'datakeys']
    },

    keycount: {
        extractor: (item) => String(Object.keys(item.data || {}).length),
        aliases: ['count']
    },

    binarykeys: {
        extractor: (item) => Object.keys(item.binaryData || {}).join(' '),
        aliases: ['binarykey', 'binary']
    }
};
