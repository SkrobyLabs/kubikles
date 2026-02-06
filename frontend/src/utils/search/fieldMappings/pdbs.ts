/**
 * PodDisruptionBudget Field Mappings
 *
 * PDB-specific fields for advanced search filtering.
 */

import { commonFields } from './common';

export const pdbFields = {
    ...commonFields,

    minavailable: {
        extractor: (item: any) => String(item.spec?.minAvailable ?? ''),
        aliases: ['min']
    },

    maxunavailable: {
        extractor: (item: any) => String(item.spec?.maxUnavailable ?? ''),
        aliases: ['max']
    },

    selector: {
        extractor: (item: any) => {
            const selector = item.spec?.selector?.matchLabels || {};
            return Object.entries(selector)
                .map(([k, v]) => `${k}=${v}`)
                .join(' ');
        },
        aliases: ['podselector']
    },

    currenthealthy: {
        extractor: (item: any) => String(item.status?.currentHealthy ?? ''),
        aliases: ['healthy', 'current']
    },

    desiredhealthy: {
        extractor: (item: any) => String(item.status?.desiredHealthy ?? ''),
        aliases: ['desired']
    },

    disruptionsallowed: {
        extractor: (item: any) => String(item.status?.disruptionsAllowed ?? ''),
        aliases: ['allowed', 'disruptions']
    },

    expectedpods: {
        extractor: (item: any) => String(item.status?.expectedPods ?? ''),
        aliases: ['expected', 'pods']
    }
};
