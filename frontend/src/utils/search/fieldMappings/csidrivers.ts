/**
 * CSIDriver Field Mappings
 *
 * CSIDriver-specific fields for advanced search filtering.
 */

import { commonFields } from './common';

export const csiDriverFields = {
    ...commonFields,

    attachrequired: {
        extractor: (item: any) => (item.spec?.attachRequired ?? true) ? 'true' : 'false',
        aliases: ['attach', 'attachreq']
    },

    podinfoonmount: {
        extractor: (item: any) => (item.spec?.podInfoOnMount ?? false) ? 'true' : 'false',
        aliases: ['podinfo']
    },

    storagecapacity: {
        extractor: (item: any) => (item.spec?.storageCapacity ?? false) ? 'true' : 'false',
        aliases: ['capacity']
    },

    volumemodes: {
        extractor: (item: any) => (item.spec?.volumeLifecycleModes || []).join(','),
        aliases: ['volumelifecyclemodes', 'modes']
    },

    fsgrouppolicy: {
        extractor: (item: any) => item.spec?.fsGroupPolicy || '',
        aliases: ['fsgroup', 'fspolicy']
    }
};
