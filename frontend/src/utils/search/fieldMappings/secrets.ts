/**
 * Secret Field Mappings
 *
 * Secret-specific fields for advanced search filtering.
 * Extends common fields with Secret-specific extractors.
 *
 * Note: List view uses metadata-only fetch for performance.
 * Key names are not available - only keycount from table API.
 */

import { commonFields } from './common';

export const secretFields = {
    ...commonFields,

    type: {
        extractor: (item: any) => item.type || 'Opaque',
        aliases: ['secrettype']
    },

    // Note: 'keys' field removed - not available in metadata-only list view
    // Key count is available from the Table API response

    keycount: {
        extractor: (item: any) => String(item.dataKeys ?? 0),
        aliases: ['count', 'datakeys']
    }
};
