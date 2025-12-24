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
} from '../../wailsjs/go/main/App';

const registry = {
    pod: {
        kind: 'Pod',
        plural: 'pods',
        namespaced: true,
        getYaml: (namespace, name) => GetPodYaml(namespace, name),
        updateYaml: (namespace, name, content) => UpdatePodYaml(namespace, name, content),
    },
    deployment: {
        kind: 'Deployment',
        plural: 'deployments',
        namespaced: true,
        getYaml: (namespace, name) => GetDeploymentYaml(namespace, name),
        updateYaml: (namespace, name, content) => UpdateDeploymentYaml(namespace, name, content),
    },
    statefulset: {
        kind: 'StatefulSet',
        plural: 'statefulsets',
        namespaced: true,
        getYaml: (namespace, name) => GetStatefulSetYaml(namespace, name),
        updateYaml: (namespace, name, content) => UpdateStatefulSetYaml(namespace, name, content),
    },
    daemonset: {
        kind: 'DaemonSet',
        plural: 'daemonsets',
        namespaced: true,
        getYaml: (namespace, name) => GetDaemonSetYaml(namespace, name),
        updateYaml: (namespace, name, content) => UpdateDaemonSetYaml(namespace, name, content),
    },
    replicaset: {
        kind: 'ReplicaSet',
        plural: 'replicasets',
        namespaced: true,
        getYaml: (namespace, name) => GetReplicaSetYaml(namespace, name),
        updateYaml: (namespace, name, content) => UpdateReplicaSetYaml(namespace, name, content),
    },
    job: {
        kind: 'Job',
        plural: 'jobs',
        namespaced: true,
        getYaml: (namespace, name) => GetJobYaml(namespace, name),
        updateYaml: (namespace, name, content) => UpdateJobYaml(namespace, name, content),
    },
    cronjob: {
        kind: 'CronJob',
        plural: 'cronjobs',
        namespaced: true,
        getYaml: (namespace, name) => GetCronJobYaml(namespace, name),
        updateYaml: (namespace, name, content) => UpdateCronJobYaml(namespace, name, content),
    },
    configmap: {
        kind: 'ConfigMap',
        plural: 'configmaps',
        namespaced: true,
        getYaml: (namespace, name) => GetConfigMapYaml(namespace, name),
        updateYaml: (namespace, name, content) => UpdateConfigMapYaml(namespace, name, content),
    },
    secret: {
        kind: 'Secret',
        plural: 'secrets',
        namespaced: true,
        getYaml: (namespace, name) => GetSecretYaml(namespace, name),
        updateYaml: (namespace, name, content) => UpdateSecretYaml(namespace, name, content),
    },
    service: {
        kind: 'Service',
        plural: 'services',
        namespaced: true,
        getYaml: (namespace, name) => GetServiceYaml(namespace, name),
        updateYaml: (namespace, name, content) => UpdateServiceYaml(namespace, name, content),
    },
    ingress: {
        kind: 'Ingress',
        plural: 'ingresses',
        namespaced: true,
        getYaml: (namespace, name) => GetIngressYaml(namespace, name),
        updateYaml: (namespace, name, content) => UpdateIngressYaml(namespace, name, content),
    },
    ingressclass: {
        kind: 'IngressClass',
        plural: 'ingressclasses',
        namespaced: false,
        getYaml: (_, name) => GetIngressClassYaml(name),
        updateYaml: (_, name, content) => UpdateIngressClassYaml(name, content),
    },
    namespace: {
        kind: 'Namespace',
        plural: 'namespaces',
        namespaced: false,
        getYaml: (_, name) => GetNamespaceYAML(name),
        updateYaml: (_, name, content) => UpdateNamespaceYAML(name, content),
    },
    event: {
        kind: 'Event',
        plural: 'events',
        namespaced: true,
        getYaml: (namespace, name) => GetEventYAML(namespace, name),
        updateYaml: (namespace, name, content) => UpdateEventYAML(namespace, name, content),
    },
    pvc: {
        kind: 'PersistentVolumeClaim',
        plural: 'persistentvolumeclaims',
        namespaced: true,
        getYaml: (namespace, name) => GetPVCYaml(namespace, name),
        updateYaml: (namespace, name, content) => UpdatePVCYaml(namespace, name, content),
    },
    pv: {
        kind: 'PersistentVolume',
        plural: 'persistentvolumes',
        namespaced: false,
        getYaml: (_, name) => GetPVYaml(name),
        updateYaml: (_, name, content) => UpdatePVYaml(name, content),
    },
    storageclass: {
        kind: 'StorageClass',
        plural: 'storageclasses',
        namespaced: false,
        getYaml: (_, name) => GetStorageClassYaml(name),
        updateYaml: (_, name, content) => UpdateStorageClassYaml(name, content),
    },
    node: {
        kind: 'Node',
        plural: 'nodes',
        namespaced: false,
        getYaml: (_, name) => GetNodeYaml(name),
        updateYaml: (_, name, content) => UpdateNodeYaml(name, content),
    },
    crd: {
        kind: 'CustomResourceDefinition',
        plural: 'customresourcedefinitions',
        namespaced: false,
        getYaml: (_, name) => GetCRDYaml(name),
        updateYaml: (_, name, content) => UpdateCRDYaml(name, content),
    },
    serviceaccount: {
        kind: 'ServiceAccount',
        plural: 'serviceaccounts',
        namespaced: true,
        getYaml: (namespace, name) => GetServiceAccountYaml(namespace, name),
        updateYaml: (namespace, name, content) => UpdateServiceAccountYaml(namespace, name, content),
    },
    role: {
        kind: 'Role',
        plural: 'roles',
        namespaced: true,
        getYaml: (namespace, name) => GetRoleYaml(namespace, name),
        updateYaml: (namespace, name, content) => UpdateRoleYaml(namespace, name, content),
    },
    clusterrole: {
        kind: 'ClusterRole',
        plural: 'clusterroles',
        namespaced: false,
        getYaml: (_, name) => GetClusterRoleYaml(name),
        updateYaml: (_, name, content) => UpdateClusterRoleYaml(name, content),
    },
    rolebinding: {
        kind: 'RoleBinding',
        plural: 'rolebindings',
        namespaced: true,
        getYaml: (namespace, name) => GetRoleBindingYaml(namespace, name),
        updateYaml: (namespace, name, content) => UpdateRoleBindingYaml(namespace, name, content),
    },
    clusterrolebinding: {
        kind: 'ClusterRoleBinding',
        plural: 'clusterrolebindings',
        namespaced: false,
        getYaml: (_, name) => GetClusterRoleBindingYaml(name),
        updateYaml: (_, name, content) => UpdateClusterRoleBindingYaml(name, content),
    },
};

/**
 * Get resource definition by type (case-insensitive).
 * @param {string} resourceType - The resource type (e.g., 'deployment', 'Deployment', 'pod')
 * @returns {object|null} Resource definition or null if not found
 */
export function getResource(resourceType) {
    if (!resourceType) return null;
    return registry[resourceType.toLowerCase()] || null;
}

/**
 * Get resource definition by Kubernetes kind.
 * @param {string} kind - The Kubernetes kind (e.g., 'Deployment', 'Pod')
 * @returns {object|null} Resource definition or null if not found
 */
export function getResourceByKind(kind) {
    if (!kind) return null;
    const kindLower = kind.toLowerCase();
    return Object.values(registry).find(r => r.kind.toLowerCase() === kindLower) || null;
}

/**
 * Get all registered resource types.
 * @returns {string[]} Array of resource type keys
 */
export function getResourceTypes() {
    return Object.keys(registry);
}

/**
 * Check if a resource type is registered.
 * @param {string} resourceType - The resource type to check
 * @returns {boolean}
 */
export function isRegisteredResource(resourceType) {
    return resourceType && resourceType.toLowerCase() in registry;
}
