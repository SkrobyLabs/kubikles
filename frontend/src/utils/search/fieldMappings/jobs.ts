/**
 * Job Field Mappings
 *
 * Job-specific fields for advanced search filtering.
 * Extends common fields with Job-specific extractors.
 */

import { commonFields } from './common';

/**
 * Get the last condition type of a job
 */
function getJobCondition(job: any) {
    const conditions = job.status?.conditions || [];
    if (conditions.length === 0) return '';
    return conditions[conditions.length - 1].type || '';
}

export const jobFields = {
    ...commonFields,

    completions: {
        extractor: (item: any) => String(item.spec?.completions || 1),
        aliases: []
    },

    parallelism: {
        extractor: (item: any) => String(item.spec?.parallelism || 1),
        aliases: []
    },

    succeeded: {
        extractor: (item: any) => String(item.status?.succeeded || 0),
        aliases: ['success']
    },

    failed: {
        extractor: (item: any) => String(item.status?.failed || 0),
        aliases: ['failures']
    },

    active: {
        extractor: (item: any) => String(item.status?.active || 0),
        aliases: ['running']
    },

    condition: {
        extractor: (item: any) => getJobCondition(item),
        aliases: ['status', 'state']
    },

    backofflimit: {
        extractor: (item: any) => String(item.spec?.backoffLimit || 6),
        aliases: ['backoff']
    },

    image: {
        extractor: (item: any) => {
            const containers = item.spec?.template?.spec?.containers || [];
            const initContainers = item.spec?.template?.spec?.initContainers || [];
            return [...containers, ...initContainers]
                .map((c: any) => c.image)
                .join(' ');
        },
        aliases: ['images']
    },

    container: {
        extractor: (item: any) => {
            const containers = item.spec?.template?.spec?.containers || [];
            const initContainers = item.spec?.template?.spec?.initContainers || [];
            return [...containers, ...initContainers]
                .map((c: any) => c.name)
                .join(' ');
        },
        aliases: ['containers']
    },

    serviceaccount: {
        extractor: (item: any) => item.spec?.template?.spec?.serviceAccountName || '',
        aliases: ['sa']
    },

    controlledby: {
        extractor: (item: any) => {
            const owners = item.metadata?.ownerReferences || [];
            const controller = owners.find((owner: any) => owner.controller);
            return controller ? controller.kind : '';
        },
        aliases: ['controller', 'owner']
    },

    controllername: {
        extractor: (item: any) => {
            const owners = item.metadata?.ownerReferences || [];
            const controller = owners.find((owner: any) => owner.controller);
            return controller ? controller.name : '';
        },
        aliases: ['ownername']
    }
};
