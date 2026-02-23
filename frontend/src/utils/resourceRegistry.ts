/**
 * Resource Registry
 *
 * Central registry for Kubernetes resource type definitions.
 * Maps resource types to their API operations and metadata.
 */

import {
    GetPodYaml, UpdatePodYaml, DeletePod,
    GetDeploymentYaml, UpdateDeploymentYaml, DeleteDeployment, RestartDeployment,
    GetStatefulSetYaml, UpdateStatefulSetYaml, DeleteStatefulSet, RestartStatefulSet,
    GetDaemonSetYaml, UpdateDaemonSetYaml, DeleteDaemonSet, RestartDaemonSet,
    GetReplicaSetYaml, UpdateReplicaSetYaml, DeleteReplicaSet,
    GetJobYaml, UpdateJobYaml, DeleteJob,
    GetCronJobYaml, UpdateCronJobYaml, DeleteCronJob,
    GetConfigMapYaml, UpdateConfigMapYaml, DeleteConfigMap,
    GetSecretYaml, UpdateSecretYaml, DeleteSecret,
    GetServiceYaml, UpdateServiceYaml, DeleteService,
    GetIngressYaml, UpdateIngressYaml, DeleteIngress,
    GetIngressClassYaml, UpdateIngressClassYaml, DeleteIngressClass,
    GetNodeYaml, UpdateNodeYaml,
    GetNamespaceYAML, UpdateNamespaceYAML, DeleteNamespace,
    GetEventYAML, UpdateEventYAML, DeleteEvent,
    GetPVCYaml, UpdatePVCYaml, DeletePVC,
    GetPVYaml, UpdatePVYaml, DeletePV,
    GetStorageClassYaml, UpdateStorageClassYaml, DeleteStorageClass,
    GetCRDYaml, UpdateCRDYaml, DeleteCRD,
    GetServiceAccountYaml, UpdateServiceAccountYaml,
    GetRoleYaml, UpdateRoleYaml,
    GetClusterRoleYaml, UpdateClusterRoleYaml,
    GetRoleBindingYaml, UpdateRoleBindingYaml,
    GetClusterRoleBindingYaml, UpdateClusterRoleBindingYaml,
    GetNetworkPolicyYaml, UpdateNetworkPolicyYaml,
    GetHPAYaml, UpdateHPAYaml,
    GetPDBYaml, UpdatePDBYaml,
    GetResourceQuotaYaml, UpdateResourceQuotaYaml,
    GetLimitRangeYaml, UpdateLimitRangeYaml,
    GetEndpointsYaml, UpdateEndpointsYaml,
    GetValidatingWebhookConfigurationYaml, UpdateValidatingWebhookConfigurationYaml,
    GetMutatingWebhookConfigurationYaml, UpdateMutatingWebhookConfigurationYaml,
    GetPriorityClassYaml, UpdatePriorityClassYaml,
    GetLeaseYaml, UpdateLeaseYaml,
    GetCSIDriverYaml, UpdateCSIDriverYaml,
    GetCSINodeYaml, UpdateCSINodeYaml,
} from 'wailsjs/go/main/App';

interface ResourceDefinition {
    kind: string;
    plural: string;
    namespaced: boolean;
    getYaml: (namespace: string, name: string) => Promise<string>;
    updateYaml: (namespace: string, name: string, content: string) => Promise<any>;
}

const registry: Record<string, ResourceDefinition> = {
    pod: {
        kind: 'Pod',
        plural: 'pods',
        namespaced: true,
        getYaml: (namespace: string, name: string) => GetPodYaml(namespace, name),
        updateYaml: (namespace: string, name: string, content: string) => UpdatePodYaml(namespace, name, content),
    },
    deployment: {
        kind: 'Deployment',
        plural: 'deployments',
        namespaced: true,
        getYaml: (namespace: string, name: string) => GetDeploymentYaml(namespace, name),
        updateYaml: (namespace: string, name: string, content: string) => UpdateDeploymentYaml(namespace, name, content),
    },
    statefulset: {
        kind: 'StatefulSet',
        plural: 'statefulsets',
        namespaced: true,
        getYaml: (namespace: string, name: string) => GetStatefulSetYaml(namespace, name),
        updateYaml: (namespace: string, name: string, content: string) => UpdateStatefulSetYaml(namespace, name, content),
    },
    daemonset: {
        kind: 'DaemonSet',
        plural: 'daemonsets',
        namespaced: true,
        getYaml: (namespace: string, name: string) => GetDaemonSetYaml(namespace, name),
        updateYaml: (namespace: string, name: string, content: string) => UpdateDaemonSetYaml(namespace, name, content),
    },
    replicaset: {
        kind: 'ReplicaSet',
        plural: 'replicasets',
        namespaced: true,
        getYaml: (namespace: string, name: string) => GetReplicaSetYaml(namespace, name),
        updateYaml: (namespace: string, name: string, content: string) => UpdateReplicaSetYaml(namespace, name, content),
    },
    job: {
        kind: 'Job',
        plural: 'jobs',
        namespaced: true,
        getYaml: (namespace: string, name: string) => GetJobYaml(namespace, name),
        updateYaml: (namespace: string, name: string, content: string) => UpdateJobYaml(namespace, name, content),
    },
    cronjob: {
        kind: 'CronJob',
        plural: 'cronjobs',
        namespaced: true,
        getYaml: (namespace: string, name: string) => GetCronJobYaml(namespace, name),
        updateYaml: (namespace: string, name: string, content: string) => UpdateCronJobYaml(namespace, name, content),
    },
    configmap: {
        kind: 'ConfigMap',
        plural: 'configmaps',
        namespaced: true,
        getYaml: (namespace: string, name: string) => GetConfigMapYaml(namespace, name),
        updateYaml: (namespace: string, name: string, content: string) => UpdateConfigMapYaml(namespace, name, content),
    },
    secret: {
        kind: 'Secret',
        plural: 'secrets',
        namespaced: true,
        getYaml: (namespace: string, name: string) => GetSecretYaml(namespace, name),
        updateYaml: (namespace: string, name: string, content: string) => UpdateSecretYaml(namespace, name, content),
    },
    service: {
        kind: 'Service',
        plural: 'services',
        namespaced: true,
        getYaml: (namespace: string, name: string) => GetServiceYaml(namespace, name),
        updateYaml: (namespace: string, name: string, content: string) => UpdateServiceYaml(namespace, name, content),
    },
    ingress: {
        kind: 'Ingress',
        plural: 'ingresses',
        namespaced: true,
        getYaml: (namespace: string, name: string) => GetIngressYaml(namespace, name),
        updateYaml: (namespace: string, name: string, content: string) => UpdateIngressYaml(namespace, name, content),
    },
    ingressclass: {
        kind: 'IngressClass',
        plural: 'ingressclasses',
        namespaced: false,
        getYaml: (_: string, name: string) => GetIngressClassYaml(name),
        updateYaml: (_: string, name: string, content: string) => UpdateIngressClassYaml(name, content),
    },
    namespace: {
        kind: 'Namespace',
        plural: 'namespaces',
        namespaced: false,
        getYaml: (_: string, name: string) => GetNamespaceYAML(name),
        updateYaml: (_: string, name: string, content: string) => UpdateNamespaceYAML(name, content),
    },
    event: {
        kind: 'Event',
        plural: 'events',
        namespaced: true,
        getYaml: (namespace: string, name: string) => GetEventYAML(namespace, name),
        updateYaml: (namespace: string, name: string, content: string) => UpdateEventYAML(namespace, name, content),
    },
    pvc: {
        kind: 'PersistentVolumeClaim',
        plural: 'persistentvolumeclaims',
        namespaced: true,
        getYaml: (namespace: string, name: string) => GetPVCYaml(namespace, name),
        updateYaml: (namespace: string, name: string, content: string) => UpdatePVCYaml(namespace, name, content),
    },
    pv: {
        kind: 'PersistentVolume',
        plural: 'persistentvolumes',
        namespaced: false,
        getYaml: (_: string, name: string) => GetPVYaml(name),
        updateYaml: (_: string, name: string, content: string) => UpdatePVYaml(name, content),
    },
    storageclass: {
        kind: 'StorageClass',
        plural: 'storageclasses',
        namespaced: false,
        getYaml: (_: string, name: string) => GetStorageClassYaml(name),
        updateYaml: (_: string, name: string, content: string) => UpdateStorageClassYaml(name, content),
    },
    node: {
        kind: 'Node',
        plural: 'nodes',
        namespaced: false,
        getYaml: (_: string, name: string) => GetNodeYaml(name),
        updateYaml: (_: string, name: string, content: string) => UpdateNodeYaml(name, content),
    },
    crd: {
        kind: 'CustomResourceDefinition',
        plural: 'customresourcedefinitions',
        namespaced: false,
        getYaml: (_: string, name: string) => GetCRDYaml(name),
        updateYaml: (_: string, name: string, content: string) => UpdateCRDYaml(name, content),
    },
    serviceaccount: {
        kind: 'ServiceAccount',
        plural: 'serviceaccounts',
        namespaced: true,
        getYaml: (namespace: string, name: string) => GetServiceAccountYaml(namespace, name),
        updateYaml: (namespace: string, name: string, content: string) => UpdateServiceAccountYaml(namespace, name, content),
    },
    role: {
        kind: 'Role',
        plural: 'roles',
        namespaced: true,
        getYaml: (namespace: string, name: string) => GetRoleYaml(namespace, name),
        updateYaml: (namespace: string, name: string, content: string) => UpdateRoleYaml(namespace, name, content),
    },
    clusterrole: {
        kind: 'ClusterRole',
        plural: 'clusterroles',
        namespaced: false,
        getYaml: (_: string, name: string) => GetClusterRoleYaml(name),
        updateYaml: (_: string, name: string, content: string) => UpdateClusterRoleYaml(name, content),
    },
    rolebinding: {
        kind: 'RoleBinding',
        plural: 'rolebindings',
        namespaced: true,
        getYaml: (namespace: string, name: string) => GetRoleBindingYaml(namespace, name),
        updateYaml: (namespace: string, name: string, content: string) => UpdateRoleBindingYaml(namespace, name, content),
    },
    clusterrolebinding: {
        kind: 'ClusterRoleBinding',
        plural: 'clusterrolebindings',
        namespaced: false,
        getYaml: (_: string, name: string) => GetClusterRoleBindingYaml(name),
        updateYaml: (_: string, name: string, content: string) => UpdateClusterRoleBindingYaml(name, content),
    },
    networkpolicy: {
        kind: 'NetworkPolicy',
        plural: 'networkpolicies',
        namespaced: true,
        getYaml: (namespace: string, name: string) => GetNetworkPolicyYaml(namespace, name),
        updateYaml: (namespace: string, name: string, content: string) => UpdateNetworkPolicyYaml(namespace, name, content),
    },
    hpa: {
        kind: 'HorizontalPodAutoscaler',
        plural: 'horizontalpodautoscalers',
        namespaced: true,
        getYaml: (namespace: string, name: string) => GetHPAYaml(namespace, name),
        updateYaml: (namespace: string, name: string, content: string) => UpdateHPAYaml(namespace, name, content),
    },
    pdb: {
        kind: 'PodDisruptionBudget',
        plural: 'poddisruptionbudgets',
        namespaced: true,
        getYaml: (namespace: string, name: string) => GetPDBYaml(namespace, name),
        updateYaml: (namespace: string, name: string, content: string) => UpdatePDBYaml(namespace, name, content),
    },
    resourcequota: {
        kind: 'ResourceQuota',
        plural: 'resourcequotas',
        namespaced: true,
        getYaml: (namespace: string, name: string) => GetResourceQuotaYaml(namespace, name),
        updateYaml: (namespace: string, name: string, content: string) => UpdateResourceQuotaYaml(namespace, name, content),
    },
    limitrange: {
        kind: 'LimitRange',
        plural: 'limitranges',
        namespaced: true,
        getYaml: (namespace: string, name: string) => GetLimitRangeYaml(namespace, name),
        updateYaml: (namespace: string, name: string, content: string) => UpdateLimitRangeYaml(namespace, name, content),
    },
    endpoints: {
        kind: 'Endpoints',
        plural: 'endpoints',
        namespaced: true,
        getYaml: (namespace: string, name: string) => GetEndpointsYaml(namespace, name),
        updateYaml: (namespace: string, name: string, content: string) => UpdateEndpointsYaml(namespace, name, content),
    },
    validatingwebhookconfiguration: {
        kind: 'ValidatingWebhookConfiguration',
        plural: 'validatingwebhookconfigurations',
        namespaced: false,
        getYaml: (_: string, name: string) => GetValidatingWebhookConfigurationYaml(name),
        updateYaml: (_: string, name: string, content: string) => UpdateValidatingWebhookConfigurationYaml(name, content),
    },
    mutatingwebhookconfiguration: {
        kind: 'MutatingWebhookConfiguration',
        plural: 'mutatingwebhookconfigurations',
        namespaced: false,
        getYaml: (_: string, name: string) => GetMutatingWebhookConfigurationYaml(name),
        updateYaml: (_: string, name: string, content: string) => UpdateMutatingWebhookConfigurationYaml(name, content),
    },
    priorityclass: {
        kind: 'PriorityClass',
        plural: 'priorityclasses',
        namespaced: false,
        getYaml: (_: string, name: string) => GetPriorityClassYaml(name),
        updateYaml: (_: string, name: string, content: string) => UpdatePriorityClassYaml(name, content),
    },
    lease: {
        kind: 'Lease',
        plural: 'leases',
        namespaced: true,
        getYaml: (namespace: string, name: string) => GetLeaseYaml(namespace, name),
        updateYaml: (namespace: string, name: string, content: string) => UpdateLeaseYaml(namespace, name, content),
    },
    csidriver: {
        kind: 'CSIDriver',
        plural: 'csidrivers',
        namespaced: false,
        getYaml: (_: string, name: string) => GetCSIDriverYaml(name),
        updateYaml: (_: string, name: string, content: string) => UpdateCSIDriverYaml(name, content),
    },
    csinode: {
        kind: 'CSINode',
        plural: 'csinodes',
        namespaced: false,
        getYaml: (_: string, name: string) => GetCSINodeYaml(name),
        updateYaml: (_: string, name: string, content: string) => UpdateCSINodeYaml(name, content),
    },
};

/**
 * Get resource definition by type (case-insensitive).
 * @param {string} resourceType - The resource type (e.g., 'deployment', 'Deployment', 'pod')
 * @returns {object|null} Resource definition or null if not found
 */
export function getResource(resourceType: string): ResourceDefinition | null {
    if (!resourceType) return null;
    return registry[resourceType.toLowerCase()] || null;
}

/**
 * Get resource definition by Kubernetes kind.
 * @param {string} kind - The Kubernetes kind (e.g., 'Deployment', 'Pod')
 * @returns {object|null} Resource definition or null if not found
 */
export function getResourceByKind(kind: string): ResourceDefinition | null {
    if (!kind) return null;
    const kindLower = kind.toLowerCase();
    return Object.values(registry).find((r: any) => r.kind.toLowerCase() === kindLower) || null;
}

