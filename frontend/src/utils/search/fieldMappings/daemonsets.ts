/**
 * DaemonSet Field Mappings
 *
 * DaemonSet-specific fields for advanced search filtering.
 * Extends workload common fields with DaemonSet-specific extractors.
 */

import { workloadCommonFields } from './workloads';

export const daemonSetFields = {
    ...workloadCommonFields,

    desired: {
        extractor: (item) => String(item.status?.desiredNumberScheduled || 0),
        aliases: ['desirednumber']
    },

    current: {
        extractor: (item) => String(item.status?.currentNumberScheduled || 0),
        aliases: ['currentnumber']
    },

    ready: {
        extractor: (item) => String(item.status?.numberReady || 0),
        aliases: ['readynumber']
    },

    available: {
        extractor: (item) => String(item.status?.numberAvailable || 0),
        aliases: ['availablenumber']
    },

    updated: {
        extractor: (item) => String(item.status?.updatedNumberScheduled || 0),
        aliases: ['updatednumber']
    },

    misscheduled: {
        extractor: (item) => String(item.status?.numberMisscheduled || 0),
        aliases: []
    },

    updatestrategy: {
        extractor: (item) => item.spec?.updateStrategy?.type || 'RollingUpdate',
        aliases: ['strategy']
    }
};
