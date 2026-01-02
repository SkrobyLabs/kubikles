/**
 * PriorityClass Field Mappings
 *
 * PriorityClass-specific fields for advanced search filtering.
 */

import { commonFields } from './common';

export const priorityClassFields = {
    ...commonFields,

    value: {
        extractor: (item) => String(item.value || 0),
        aliases: ['priority', 'priorityvalue']
    },

    globaldefault: {
        extractor: (item) => item.globalDefault ? 'true' : 'false',
        aliases: ['default', 'isdefault']
    },

    preemption: {
        extractor: (item) => item.preemptionPolicy || 'PreemptLowerPriority',
        aliases: ['preemptionpolicy', 'preemptlowerpriority']
    },

    description: {
        extractor: (item) => item.description || '',
        aliases: ['desc']
    }
};
