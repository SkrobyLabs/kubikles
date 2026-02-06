/**
 * CronJob Field Mappings
 *
 * CronJob-specific fields for advanced search filtering.
 * Extends common fields with CronJob-specific extractors.
 */

import { commonFields } from './common';

export const cronJobFields = {
    ...commonFields,

    schedule: {
        extractor: (item: any) => item.spec?.schedule || '',
        aliases: ['cron']
    },

    suspend: {
        extractor: (item: any) => String(item.spec?.suspend || false),
        aliases: ['suspended', 'paused']
    },

    concurrencypolicy: {
        extractor: (item: any) => item.spec?.concurrencyPolicy || 'Allow',
        aliases: ['concurrency']
    },

    lastscheduled: {
        extractor: (item: any) => item.status?.lastScheduleTime || '',
        aliases: ['lastrun', 'lastschedule']
    },

    successfuljobs: {
        extractor: (item: any) => String(item.spec?.successfulJobsHistoryLimit ?? 3),
        aliases: ['successhistory']
    },

    failedjobs: {
        extractor: (item: any) => String(item.spec?.failedJobsHistoryLimit ?? 1),
        aliases: ['failhistory']
    },

    activejobs: {
        extractor: (item: any) => String((item.status?.active || []).length),
        aliases: ['active', 'running']
    },

    image: {
        extractor: (item: any) => {
            const containers = item.spec?.jobTemplate?.spec?.template?.spec?.containers || [];
            const initContainers = item.spec?.jobTemplate?.spec?.template?.spec?.initContainers || [];
            return [...containers, ...initContainers]
                .map((c: any) => c.image)
                .join(' ');
        },
        aliases: ['images']
    },

    container: {
        extractor: (item: any) => {
            const containers = item.spec?.jobTemplate?.spec?.template?.spec?.containers || [];
            const initContainers = item.spec?.jobTemplate?.spec?.template?.spec?.initContainers || [];
            return [...containers, ...initContainers]
                .map((c: any) => c.name)
                .join(' ');
        },
        aliases: ['containers']
    },

    serviceaccount: {
        extractor: (item: any) => item.spec?.jobTemplate?.spec?.template?.spec?.serviceAccountName || '',
        aliases: ['sa']
    }
};
