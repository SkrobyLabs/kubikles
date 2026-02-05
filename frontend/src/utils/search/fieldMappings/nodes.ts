/**
 * Node Field Mappings
 *
 * Node-specific fields for advanced search filtering.
 * Note: Nodes are cluster-scoped and don't have namespace.
 */

/**
 * Get node status (Ready/NotReady)
 */
function getNodeStatus(node) {
    const readyCondition = (node.status?.conditions || []).find(c => c.type === 'Ready');
    return readyCondition?.status === 'True' ? 'Ready' : 'NotReady';
}

/**
 * Get node roles from labels
 */
function getNodeRoles(node) {
    const labels = node.metadata?.labels || {};
    const roles = [];
    for (const [key, value] of Object.entries(labels)) {
        if (key.startsWith('node-role.kubernetes.io/')) {
            roles.push(key.replace('node-role.kubernetes.io/', ''));
        }
    }
    return roles.length > 0 ? roles.join(' ') : 'worker';
}

/**
 * Get specific condition status
 */
function getConditionStatus(node, conditionType) {
    const condition = (node.status?.conditions || []).find(c => c.type === conditionType);
    return condition?.status || '';
}

export const nodeFields = {
    name: {
        extractor: (item) => item.metadata?.name || '',
        aliases: ['n']
    },

    labels: {
        extractor: (item) => {
            const labels = item.metadata?.labels || {};
            return Object.entries(labels)
                .map(([k, v]) => `${k}=${v}`)
                .join(' ');
        },
        aliases: ['label', 'l']
    },

    annotations: {
        extractor: (item) => {
            const annotations = item.metadata?.annotations || {};
            return Object.entries(annotations)
                .map(([k, v]) => `${k}=${v}`)
                .join(' ');
        },
        aliases: ['annotation', 'a']
    },

    uid: {
        extractor: (item) => item.metadata?.uid || '',
        aliases: []
    },

    status: {
        extractor: (item) => getNodeStatus(item),
        aliases: ['state']
    },

    role: {
        extractor: (item) => getNodeRoles(item),
        aliases: ['roles']
    },

    version: {
        extractor: (item) => item.status?.nodeInfo?.kubeletVersion || '',
        aliases: ['kubeletversion', 'k8sversion']
    },

    os: {
        extractor: (item) => item.status?.nodeInfo?.osImage || '',
        aliases: ['osimage']
    },

    kernel: {
        extractor: (item) => item.status?.nodeInfo?.kernelVersion || '',
        aliases: ['kernelversion']
    },

    containerruntime: {
        extractor: (item) => item.status?.nodeInfo?.containerRuntimeVersion || '',
        aliases: ['runtime', 'cri']
    },

    arch: {
        extractor: (item) => item.status?.nodeInfo?.architecture || '',
        aliases: ['architecture']
    },

    podcidr: {
        extractor: (item) => item.spec?.podCIDR || '',
        aliases: ['cidr']
    },

    internalip: {
        extractor: (item) => {
            const addr = (item.status?.addresses || []).find(a => a.type === 'InternalIP');
            return addr?.address || '';
        },
        aliases: ['ip']
    },

    externalip: {
        extractor: (item) => {
            const addr = (item.status?.addresses || []).find(a => a.type === 'ExternalIP');
            return addr?.address || '';
        },
        aliases: ['extip']
    },

    hostname: {
        extractor: (item) => {
            const addr = (item.status?.addresses || []).find(a => a.type === 'Hostname');
            return addr?.address || '';
        },
        aliases: ['host']
    },

    memorypressure: {
        extractor: (item) => getConditionStatus(item, 'MemoryPressure'),
        aliases: ['memory']
    },

    diskpressure: {
        extractor: (item) => getConditionStatus(item, 'DiskPressure'),
        aliases: ['disk']
    },

    pidpressure: {
        extractor: (item) => getConditionStatus(item, 'PIDPressure'),
        aliases: ['pid']
    },

    unschedulable: {
        extractor: (item) => String(item.spec?.unschedulable || false),
        aliases: ['cordon', 'cordoned']
    },

    taints: {
        extractor: (item) => {
            const taints = item.spec?.taints || [];
            return taints.map(t => `${t.key}=${t.value}:${t.effect}`).join(' ');
        },
        aliases: ['taint']
    }
};
