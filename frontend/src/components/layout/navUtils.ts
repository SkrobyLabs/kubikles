// Map resource kind (lowercase) to sidebar view name.
// sync: pkg/tools/tools.go:NormalizeKind (reverse mapping: view→kind)
export const kindToViewName = {
    pod: 'pods', deployment: 'deployments', statefulset: 'statefulsets',
    daemonset: 'daemonsets', replicaset: 'replicasets', job: 'jobs',
    cronjob: 'cronjobs', configmap: 'configmaps', secret: 'secrets',
    service: 'services', ingress: 'ingresses', node: 'nodes',
    namespace: 'namespaces', event: 'events',
    persistentvolumeclaim: 'pvcs', pvc: 'pvcs',
    persistentvolume: 'pvs', pv: 'pvs',
    storageclass: 'storageclasses', serviceaccount: 'serviceaccounts',
    role: 'roles', clusterrole: 'clusterroles',
    rolebinding: 'rolebindings', clusterrolebinding: 'clusterrolebindings',
    horizontalpodautoscaler: 'hpas', hpa: 'hpas',
    poddisruptionbudget: 'pdbs', pdb: 'pdbs',
    networkpolicy: 'networkpolicies',
    endpoint: 'endpoints', endpointslice: 'endpointslices',
    ingressclass: 'ingressclasses', priorityclass: 'priorityclasses',
    resourcequota: 'resourcequotas', limitrange: 'limitranges',
    lease: 'leases', csidriver: 'csidrivers', csinode: 'csinodes',
};

// Parse a cr:GROUP:VERSION:RESOURCE:KIND string into its components
export function parseCrKind(kind: string) {
    if (!kind || !kind.startsWith('cr:')) return null;
    const segs = kind.split(':');
    if (segs.length < 4) return null; // need at least cr:GROUP:VERSION:RESOURCE
    const [, group, version, resource, displayKind] = segs;
    return { group, version, resource, kind: displayKind || resource };
}

// Regex for extracting nav links from message text
export const navLinkRegex = /\[([^\]]+)\]\((nav:\/\/[^)]+)\)/g;
