/**
 * PriorityClass Field Mappings
 *
 * PriorityClass-specific fields for advanced search filtering.
 */

import { commonFields } from './common';

export const priorityClassFields = {
    ...commonFields,

    value: {
        extractor: (item: any) => String(item.value || 0),
        aliases: ['priority', 'priorityvalue']
    },

    globaldefault: {
        extractor: (item: any) => item.globalDefault ? 'true' : 'false',
        aliases: ['default', 'isdefault']
    },

    preemption: {
        extractor: (item: any) => item.preemptionPolicy || 'PreemptLowerPriority',
        aliases: ['preemptionpolicy', 'preemptlowerpriority']
    },

    description: {
        extractor: (item: any) => item.description || '',
        aliases: ['desc']
    }
};
