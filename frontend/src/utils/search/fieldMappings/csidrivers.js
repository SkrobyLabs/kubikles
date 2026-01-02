/**
 * CSIDriver Field Mappings
 *
 * CSIDriver-specific fields for advanced search filtering.
 */

import { commonFields } from './common';

export const csiDriverFields = {
    ...commonFields,

    attachrequired: {
        extractor: (item) => (item.spec?.attachRequired ?? true) ? 'true' : 'false',
        aliases: ['attach', 'attachreq']
    },

    podinfoonmount: {
        extractor: (item) => (item.spec?.podInfoOnMount ?? false) ? 'true' : 'false',
        aliases: ['podinfo']
    },

    storagecapacity: {
        extractor: (item) => (item.spec?.storageCapacity ?? false) ? 'true' : 'false',
        aliases: ['capacity']
    },

    volumemodes: {
        extractor: (item) => (item.spec?.volumeLifecycleModes || []).join(','),
        aliases: ['volumelifecyclemodes', 'modes']
    },

    fsgrouppolicy: {
        extractor: (item) => item.spec?.fsGroupPolicy || '',
        aliases: ['fsgroup', 'fspolicy']
    }
};
