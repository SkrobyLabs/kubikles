/**
 * Secret Field Mappings
 *
 * Secret-specific fields for advanced search filtering.
 * Extends common fields with Secret-specific extractors.
 */

import { commonFields } from './common';

export const secretFields = {
    ...commonFields,

    type: {
        extractor: (item) => item.type || 'Opaque',
        aliases: ['secrettype']
    },

    keys: {
        extractor: (item) => Object.keys(item.data || {}).join(' '),
        aliases: ['key', 'datakeys']
    },

    keycount: {
        extractor: (item) => String(Object.keys(item.data || {}).length),
        aliases: ['count']
    }
};
