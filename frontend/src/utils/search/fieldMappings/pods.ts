/**
 * Pod Field Mappings
 *
 * Pod-specific fields for advanced search filtering.
 * Extends common fields with Pod-specific extractors.
 */

import { commonFields } from './common';

/**
 * Get Pod status (replicates logic from k8s-helpers.js)
 */
function getPodStatusForSearch(pod) {
    if (pod.metadata?.deletionTimestamp) return 'Terminating';

    // Check for init container failures
    if (pod.status?.initContainerStatuses) {
        for (const status of pod.status.initContainerStatuses) {
            if (status.state?.terminated && status.state.terminated.exitCode !== 0) {
                return 'Init:Error';
            }
            if (status.state?.waiting && status.state.waiting.reason === 'CrashLoopBackOff') {
                return 'Init:CrashLoopBackOff';
            }
            if (status.state?.running === undefined && status.state?.terminated === undefined) {
                return 'Init:Running';
            }
        }
    }

    // Check container statuses for more specific status
    const containerStatuses = pod.status?.containerStatuses || [];
    for (const status of containerStatuses) {
        if (status.state?.waiting) {
            const reason = status.state.waiting.reason;
            if (reason === 'CrashLoopBackOff' || reason === 'ErrImagePull' || reason === 'ImagePullBackOff') {
                return reason;
            }
        }
    }

    return pod.status?.phase || 'Unknown';
}

export const podFields = {
    ...commonFields,

    nodename: {
        extractor: (pod) => pod.spec?.nodeName || '',
        aliases: ['node']
    },

    status: {
        extractor: (pod) => getPodStatusForSearch(pod),
        aliases: ['phase', 's']
    },

    ip: {
        extractor: (pod) => pod.status?.podIP || '',
        aliases: ['podip']
    },

    hostip: {
        extractor: (pod) => pod.status?.hostIP || '',
        aliases: []
    },

    restarts: {
        extractor: (pod) => {
            const count = (pod.status?.containerStatuses || [])
                .reduce((acc, curr) => acc + (curr.restartCount || 0), 0);
            return String(count);
        },
        aliases: ['restart']
    },

    controlledby: {
        extractor: (pod) => {
            const owners = pod.metadata?.ownerReferences || [];
            const controller = owners.find(owner => owner.controller);
            return controller ? controller.kind : '';
        },
        aliases: ['controller', 'owner']
    },

    controllername: {
        extractor: (pod) => {
            const owners = pod.metadata?.ownerReferences || [];
            const controller = owners.find(owner => owner.controller);
            return controller ? controller.name : '';
        },
        aliases: ['ownername']
    },

    container: {
        extractor: (pod) => {
            const containers = pod.spec?.containers || [];
            const initContainers = pod.spec?.initContainers || [];
            return [...containers, ...initContainers]
                .map(c => c.name)
                .join(' ');
        },
        aliases: ['containers']
    },

    image: {
        extractor: (pod) => {
            const containers = pod.spec?.containers || [];
            const initContainers = pod.spec?.initContainers || [];
            return [...containers, ...initContainers]
                .map(c => c.image)
                .join(' ');
        },
        aliases: ['images']
    },

    serviceaccount: {
        extractor: (pod) => pod.spec?.serviceAccountName || pod.spec?.serviceAccount || '',
        aliases: ['sa']
    },

    qos: {
        extractor: (pod) => pod.status?.qosClass || '',
        aliases: ['qosclass']
    }
};
