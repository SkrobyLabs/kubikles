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
    GetNamespaceYAML, UpdateNamespaceYAML, DeleteNamespace,
    GetEventYAML, UpdateEventYAML, DeleteEvent,
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
