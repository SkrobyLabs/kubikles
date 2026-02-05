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
        extractor: (item) => item.spec?.schedule || '',
        aliases: ['cron']
    },

    suspend: {
        extractor: (item) => String(item.spec?.suspend || false),
        aliases: ['suspended', 'paused']
    },

    concurrencypolicy: {
        extractor: (item) => item.spec?.concurrencyPolicy || 'Allow',
        aliases: ['concurrency']
    },

    lastscheduled: {
        extractor: (item) => item.status?.lastScheduleTime || '',
        aliases: ['lastrun', 'lastschedule']
    },

    successfuljobs: {
        extractor: (item) => String(item.spec?.successfulJobsHistoryLimit ?? 3),
        aliases: ['successhistory']
    },

    failedjobs: {
        extractor: (item) => String(item.spec?.failedJobsHistoryLimit ?? 1),
        aliases: ['failhistory']
    },

    activejobs: {
        extractor: (item) => String((item.status?.active || []).length),
        aliases: ['active', 'running']
    },

    image: {
        extractor: (item) => {
            const containers = item.spec?.jobTemplate?.spec?.template?.spec?.containers || [];
            const initContainers = item.spec?.jobTemplate?.spec?.template?.spec?.initContainers || [];
            return [...containers, ...initContainers]
                .map(c => c.image)
                .join(' ');
        },
        aliases: ['images']
    },

    container: {
        extractor: (item) => {
            const containers = item.spec?.jobTemplate?.spec?.template?.spec?.containers || [];
            const initContainers = item.spec?.jobTemplate?.spec?.template?.spec?.initContainers || [];
            return [...containers, ...initContainers]
                .map(c => c.name)
                .join(' ');
        },
        aliases: ['containers']
    },

    serviceaccount: {
        extractor: (item) => item.spec?.jobTemplate?.spec?.template?.spec?.serviceAccountName || '',
        aliases: ['sa']
    }
};
