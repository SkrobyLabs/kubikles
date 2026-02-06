/**
 * Deployment Field Mappings
 *
 * Deployment-specific fields for advanced search filtering.
 * Extends workload common fields with Deployment-specific extractors.
 */

import { workloadCommonFields } from './workloads';

export const deploymentFields = {
    ...workloadCommonFields,

    replicas: {
        extractor: (item: any) => String(item.spec?.replicas || 0),
        aliases: ['desired']
    },

    ready: {
        extractor: (item: any) => String(item.status?.readyReplicas || 0),
        aliases: ['readyreplicas']
    },

    available: {
        extractor: (item: any) => String(item.status?.availableReplicas || 0),
        aliases: ['availablereplicas']
    },

    updated: {
        extractor: (item: any) => String(item.status?.updatedReplicas || 0),
        aliases: ['updatedreplicas']
    },

    strategy: {
        extractor: (item: any) => item.spec?.strategy?.type || '',
        aliases: ['updatestrategy']
    },

    paused: {
        extractor: (item: any) => String(item.spec?.paused || false),
        aliases: []
    }
};
