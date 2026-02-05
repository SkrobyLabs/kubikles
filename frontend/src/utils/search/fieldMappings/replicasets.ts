/**
 * ReplicaSet Field Mappings
 *
 * ReplicaSet-specific fields for advanced search filtering.
 * Extends workload common fields with ReplicaSet-specific extractors.
 */

import { workloadCommonFields } from './workloads';

export const replicaSetFields = {
    ...workloadCommonFields,

    replicas: {
        extractor: (item) => String(item.spec?.replicas || 0),
        aliases: ['desired']
    },

    current: {
        extractor: (item) => String(item.status?.replicas || 0),
        aliases: ['currentreplicas']
    },

    ready: {
        extractor: (item) => String(item.status?.readyReplicas || 0),
        aliases: ['readyreplicas']
    },

    available: {
        extractor: (item) => String(item.status?.availableReplicas || 0),
        aliases: ['availablereplicas']
    },

    fulllabeled: {
        extractor: (item) => String(item.status?.fullyLabeledReplicas || 0),
        aliases: ['labeled']
    }
};
