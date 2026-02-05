/**
 * Owner Navigation Utilities
 *
 * Helpers for resolving Kubernetes owner references to navigation view IDs.
 * Supports both built-in resources and Custom Resources (CRDs).
 */

// Map standard Kubernetes resource kinds to view names
export const kindToView: Record<string, string> = {
    'Pod': 'pods',
    'Deployment': 'deployments',
    'ReplicaSet': 'replicasets',
    'StatefulSet': 'statefulsets',
    'DaemonSet': 'daemonsets',
    'Job': 'jobs',
    'CronJob': 'cronjobs',
    'Service': 'services',
    'ConfigMap': 'configmaps',
    'Secret': 'secrets',
    'Ingress': 'ingresses',
    'PersistentVolumeClaim': 'pvc',
    'PersistentVolume': 'pv',
    'ServiceAccount': 'serviceaccounts',
    'Role': 'roles',
    'ClusterRole': 'clusterroles',
    'RoleBinding': 'rolebindings',
    'ClusterRoleBinding': 'clusterrolebindings',
    'NetworkPolicy': 'networkpolicies',
    'HorizontalPodAutoscaler': 'hpas',
    'PodDisruptionBudget': 'pdbs',
    'Namespace': 'namespaces',
    'Node': 'nodes',
    'StorageClass': 'storageclasses',
    'Endpoints': 'endpoints',
    'EndpointSlice': 'endpointslices',
    'Event': 'events',
    'LimitRange': 'limitranges',
    'ResourceQuota': 'resourcequotas',
    'PriorityClass': 'priorityclasses',
    'Lease': 'leases',
    'CSIDriver': 'csidrivers',
    'CSINode': 'csinodes',
    'IngressClass': 'ingressclasses',
    'ValidatingWebhookConfiguration': 'validatingwebhookconfigurations',
    'MutatingWebhookConfiguration': 'mutatingwebhookconfigurations',
};

interface OwnerReference {
    apiVersion?: string;
    kind: string;
    uid?: string;
}

interface CRD {
    spec?: {
        group?: string;
        names?: {
            kind?: string;
            plural?: string;
        };
        scope?: string;
    };
}

/**
 * Get the view ID for navigating to an owner/involved object.
 *
 * @param owner - Owner reference or involved object with apiVersion, kind, uid
 * @param crds - List of CRDs from K8sContext for custom resource lookup
 * @returns View ID for navigation, or null if not navigable
 *
 * @example
 * // For built-in resource
 * getOwnerViewId({ apiVersion: 'apps/v1', kind: 'Deployment', uid: '...' }, [])
 * // Returns: 'deployments'
 *
 * @example
 * // For custom resource
 * getOwnerViewId({ apiVersion: 'argoproj.io/v1alpha1', kind: 'Application', uid: '...' }, crds)
 * // Returns: 'cr:argoproj.io:v1alpha1:applications:Application:true'
 */
export function getOwnerViewId(owner: OwnerReference | null | undefined, crds: CRD[] = []): string | null {
    if (!owner || !owner.kind) return null;

    // Check built-in resources first
    const builtInView = kindToView[owner.kind];
    if (builtInView) {
        return builtInView;
    }

    // Try to find matching CRD for custom resources
    if (!owner.apiVersion || crds.length === 0) {
        return null;
    }

    // Parse apiVersion: "group/version" or just "version" for core API
    const parts = owner.apiVersion.split('/');
    const group = parts.length === 2 ? parts[0] : '';
    const version = parts.length === 2 ? parts[1] : parts[0];

    // Find matching CRD
    const crd = crds.find(c => {
        const crdGroup = c.spec?.group || '';
        const crdKind = c.spec?.names?.kind || '';
        return crdGroup === group && crdKind === owner.kind;
    });

    if (!crd) {
        return null;
    }

    // Build custom resource view ID: cr:{group}:{version}:{plural}:{kind}:{namespaced}
    const plural = crd.spec?.names?.plural || '';
    const namespaced = crd.spec?.scope === 'Namespaced';

    return `cr:${group}:${version}:${plural}:${owner.kind}:${namespaced}`;
}
