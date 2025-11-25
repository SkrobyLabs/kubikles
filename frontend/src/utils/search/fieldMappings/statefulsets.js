/**
 * StatefulSet Field Mappings
 *
 * StatefulSet-specific fields for advanced search filtering.
 * Extends workload common fields with StatefulSet-specific extractors.
 */

import { workloadCommonFields } from './workloads';

export const statefulSetFields = {
    ...workloadCommonFields,

    replicas: {
        extractor: (item) => String(item.spec?.replicas || 0),
        aliases: ['desired']
    },

    ready: {
        extractor: (item) => String(item.status?.readyReplicas || 0),
        aliases: ['readyreplicas']
    },

    current: {
        extractor: (item) => String(item.status?.currentReplicas || 0),
        aliases: ['currentreplicas']
    },

    updated: {
        extractor: (item) => String(item.status?.updatedReplicas || 0),
        aliases: ['updatedreplicas']
    },

    servicename: {
        extractor: (item) => item.spec?.serviceName || '',
        aliases: ['service', 'headless']
    },

    podmanagementpolicy: {
        extractor: (item) => item.spec?.podManagementPolicy || 'OrderedReady',
        aliases: ['podpolicy']
    },

    updatestrategy: {
        extractor: (item) => item.spec?.updateStrategy?.type || 'RollingUpdate',
        aliases: ['strategy']
    }
};
