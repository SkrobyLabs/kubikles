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
function getJobCondition(job) {
    const conditions = job.status?.conditions || [];
    if (conditions.length === 0) return '';
    return conditions[conditions.length - 1].type || '';
}

export const jobFields = {
    ...commonFields,

    completions: {
        extractor: (item) => String(item.spec?.completions || 1),
        aliases: []
    },

    parallelism: {
        extractor: (item) => String(item.spec?.parallelism || 1),
        aliases: []
    },

    succeeded: {
        extractor: (item) => String(item.status?.succeeded || 0),
        aliases: ['success']
    },

    failed: {
        extractor: (item) => String(item.status?.failed || 0),
        aliases: ['failures']
    },

    active: {
        extractor: (item) => String(item.status?.active || 0),
        aliases: ['running']
    },

    condition: {
        extractor: (item) => getJobCondition(item),
        aliases: ['status', 'state']
    },

    backofflimit: {
        extractor: (item) => String(item.spec?.backoffLimit || 6),
        aliases: ['backoff']
    },

    image: {
        extractor: (item) => {
            const containers = item.spec?.template?.spec?.containers || [];
            const initContainers = item.spec?.template?.spec?.initContainers || [];
            return [...containers, ...initContainers]
                .map(c => c.image)
                .join(' ');
        },
        aliases: ['images']
    },

    container: {
        extractor: (item) => {
            const containers = item.spec?.template?.spec?.containers || [];
            const initContainers = item.spec?.template?.spec?.initContainers || [];
            return [...containers, ...initContainers]
                .map(c => c.name)
                .join(' ');
        },
        aliases: ['containers']
    },

    serviceaccount: {
        extractor: (item) => item.spec?.template?.spec?.serviceAccountName || '',
        aliases: ['sa']
    },

    controlledby: {
        extractor: (item) => {
            const owners = item.metadata?.ownerReferences || [];
            const controller = owners.find(owner => owner.controller);
            return controller ? controller.kind : '';
        },
        aliases: ['controller', 'owner']
    },

    controllername: {
        extractor: (item) => {
            const owners = item.metadata?.ownerReferences || [];
            const controller = owners.find(owner => owner.controller);
            return controller ? controller.name : '';
        },
        aliases: ['ownername']
    }
};
