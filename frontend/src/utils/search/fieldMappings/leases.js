/**
 * Lease Field Mappings
 *
 * Lease-specific fields for advanced search filtering.
 */

import { commonFields } from './common';

export const leaseFields = {
    ...commonFields,

    holder: {
        extractor: (item) => item.spec?.holderIdentity || '',
        aliases: ['holderidentity', 'leader', 'owner']
    },

    duration: {
        extractor: (item) => String(item.spec?.leaseDurationSeconds || ''),
        aliases: ['leaseduration', 'leasedurationseconds']
    },

    transitions: {
        extractor: (item) => String(item.spec?.leaseTransitions || 0),
        aliases: ['leasetransitions', 'changes']
    }
};
