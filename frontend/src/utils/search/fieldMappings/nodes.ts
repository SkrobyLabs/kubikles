/**
 * Node Field Mappings
 *
 * Node-specific fields for advanced search filtering.
 * Note: Nodes are cluster-scoped and don't have namespace.
 */

/**
 * Get node status (Ready/NotReady)
 */
function getNodeStatus(node: any) {
    const readyCondition = (node.status?.conditions || []).find((c: any) => c.type === 'Ready');
    return readyCondition?.status === 'True' ? 'Ready' : 'NotReady';
}

/**
 * Get node roles from labels
 */
function getNodeRoles(node: any) {
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
function getConditionStatus(node: any, conditionType: string) {
    const condition = (node.status?.conditions || []).find((c: any) => c.type === conditionType);
    return condition?.status || '';
}

export const nodeFields = {
    name: {
        extractor: (item: any) => item.metadata?.name || '',
        aliases: ['n']
    },

    labels: {
        extractor: (item: any) => {
            const labels = item.metadata?.labels || {};
            return Object.entries(labels)
                .map(([k, v]) => `${k}=${v}`)
                .join(' ');
        },
        aliases: ['label', 'l']
    },

    annotations: {
        extractor: (item: any) => {
            const annotations = item.metadata?.annotations || {};
            return Object.entries(annotations)
                .map(([k, v]) => `${k}=${v}`)
                .join(' ');
        },
        aliases: ['annotation', 'a']
    },

    uid: {
        extractor: (item: any) => item.metadata?.uid || '',
        aliases: []
    },

    status: {
        extractor: (item: any) => getNodeStatus(item),
        aliases: ['state']
    },

    role: {
        extractor: (item: any) => getNodeRoles(item),
        aliases: ['roles']
    },

    version: {
        extractor: (item: any) => item.status?.nodeInfo?.kubeletVersion || '',
        aliases: ['kubeletversion', 'k8sversion']
    },

    os: {
        extractor: (item: any) => item.status?.nodeInfo?.osImage || '',
        aliases: ['osimage']
    },

    kernel: {
        extractor: (item: any) => item.status?.nodeInfo?.kernelVersion || '',
        aliases: ['kernelversion']
    },

    containerruntime: {
        extractor: (item: any) => item.status?.nodeInfo?.containerRuntimeVersion || '',
        aliases: ['runtime', 'cri']
    },

    arch: {
        extractor: (item: any) => item.status?.nodeInfo?.architecture || '',
        aliases: ['architecture']
    },

    podcidr: {
        extractor: (item: any) => item.spec?.podCIDR || '',
        aliases: ['cidr']
    },

    internalip: {
        extractor: (item: any) => {
            const addr = (item.status?.addresses || []).find((a: any) => a.type === 'InternalIP');
            return addr?.address || '';
        },
        aliases: ['ip']
    },

    externalip: {
        extractor: (item: any) => {
            const addr = (item.status?.addresses || []).find((a: any) => a.type === 'ExternalIP');
            return addr?.address || '';
        },
        aliases: ['extip']
    },

    hostname: {
        extractor: (item: any) => {
            const addr = (item.status?.addresses || []).find((a: any) => a.type === 'Hostname');
            return addr?.address || '';
        },
        aliases: ['host']
    },

    memorypressure: {
        extractor: (item: any) => getConditionStatus(item, 'MemoryPressure'),
        aliases: ['memory']
    },

    diskpressure: {
        extractor: (item: any) => getConditionStatus(item, 'DiskPressure'),
        aliases: ['disk']
    },

    pidpressure: {
        extractor: (item: any) => getConditionStatus(item, 'PIDPressure'),
        aliases: ['pid']
    },

    unschedulable: {
        extractor: (item: any) => String(item.spec?.unschedulable || false),
        aliases: ['cordon', 'cordoned']
    },

    taints: {
        extractor: (item: any) => {
            const taints = item.spec?.taints || [];
            return taints.map((t: any) => `${t.key}=${t.value}:${t.effect}`).join(' ');
        },
        aliases: ['taint']
    }
};
