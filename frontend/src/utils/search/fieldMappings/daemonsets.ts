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
        extractor: (item: any) => String(item.status?.desiredNumberScheduled || 0),
        aliases: ['desirednumber']
    },

    current: {
        extractor: (item: any) => String(item.status?.currentNumberScheduled || 0),
        aliases: ['currentnumber']
    },

    ready: {
        extractor: (item: any) => String(item.status?.numberReady || 0),
        aliases: ['readynumber']
    },

    available: {
        extractor: (item: any) => String(item.status?.numberAvailable || 0),
        aliases: ['availablenumber']
    },

    updated: {
        extractor: (item: any) => String(item.status?.updatedNumberScheduled || 0),
        aliases: ['updatednumber']
    },

    misscheduled: {
        extractor: (item: any) => String(item.status?.numberMisscheduled || 0),
        aliases: []
    },

    updatestrategy: {
        extractor: (item: any) => item.spec?.updateStrategy?.type || 'RollingUpdate',
        aliases: ['strategy']
    }
};
