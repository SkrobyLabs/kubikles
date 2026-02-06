/**
 * CSINode Field Mappings
 *
 * CSINode-specific fields for advanced search filtering.
 */

import { commonFields } from './common';

export const csiNodeFields = {
    ...commonFields,

    drivercount: {
        extractor: (item: any) => String((item.spec?.drivers || []).length),
        aliases: ['drivers', 'numDrivers']
    },

    drivernames: {
        extractor: (item: any) => (item.spec?.drivers || []).map((d: any) => d.name).join(','),
        aliases: ['driver']
    },

    nodeids: {
        extractor: (item: any) => (item.spec?.drivers || []).map((d: any) => d.nodeID).join(','),
        aliases: ['nodeid']
    }
};
