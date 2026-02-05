/**
 * Workloads Common Field Mappings
 *
 * Shared fields for workload resources (Deployments, StatefulSets, DaemonSets, ReplicaSets).
 * Extends common fields with workload-specific extractors.
 */

import { commonFields } from './common';

/**
 * Get owner reference info
 */
function getOwnerInfo(item) {
    const owners = item.metadata?.ownerReferences || [];
    const controller = owners.find(owner => owner.controller);
    return controller ? { kind: controller.kind, name: controller.name } : null;
}

export const workloadCommonFields = {
    ...commonFields,

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
        extractor: (item) => item.spec?.template?.spec?.serviceAccountName || item.spec?.template?.spec?.serviceAccount || '',
        aliases: ['sa']
    },

    selector: {
        extractor: (item) => {
            const matchLabels = item.spec?.selector?.matchLabels || {};
            return Object.entries(matchLabels)
                .map(([k, v]) => `${k}=${v}`)
                .join(' ');
        },
        aliases: ['sel']
    },

    controlledby: {
        extractor: (item) => {
            const owner = getOwnerInfo(item);
            return owner ? owner.kind : '';
        },
        aliases: ['controller', 'owner']
    },

    controllername: {
        extractor: (item) => {
            const owner = getOwnerInfo(item);
            return owner ? owner.name : '';
        },
        aliases: ['ownername']
    }
};
