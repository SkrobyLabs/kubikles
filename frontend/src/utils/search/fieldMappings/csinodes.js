/**
 * CSINode Field Mappings
 *
 * CSINode-specific fields for advanced search filtering.
 */

import { commonFields } from './common';

export const csiNodeFields = {
    ...commonFields,

    drivercount: {
        extractor: (item) => String((item.spec?.drivers || []).length),
        aliases: ['drivers', 'numDrivers']
    },

    drivernames: {
        extractor: (item) => (item.spec?.drivers || []).map(d => d.name).join(','),
        aliases: ['driver']
    },

    nodeids: {
        extractor: (item) => (item.spec?.drivers || []).map(d => d.nodeID).join(','),
        aliases: ['nodeid']
    }
};
