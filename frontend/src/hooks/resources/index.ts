/**
 * Resource hooks factory - consolidated hooks for Kubernetes resources.
 *
 * This module replaces individual hook files (usePods.js, useDeployments.js, etc.)
 * with a factory pattern that eliminates code duplication.
 *
 * Usage:
 *   import { usePods, useDeployments } from '~/hooks/resources';
 *
 * Namespaced hooks signature:
 *   const { pods, loading, error, setPods } = usePods(currentContext, selectedNamespaces, isVisible) as any;
 *
 * Cluster-scoped hooks signature:
 *   const { nodes, loading, error, refetch } = useNodes(currentContext, isVisible) as any;
 */

import { createNamespacedResourceHook, createClusterScopedResourceHook } from '../useResource';
import { viewCapableNamespaced, viewCapableCluster } from '../useRemoteOrLegacy';

// Import all list functions from Wails
import {
    ListPods,
    ListDeployments,
    ListStatefulSets,
    ListDaemonSets,
    ListReplicaSets,
    ListJobs,
    ListCronJobs,
    ListServices,
    ListIngresses,
    ListConfigMaps,
    ListSecretsMetadata,
    ListPVCs,
    ListPVs,
    ListNodes,
    ListNamespaces,
    ListEvents,
    ListIngressClasses,
    ListStorageClasses,
    ListServiceAccounts,
    ListRoles,
    ListClusterRoles,
    ListRoleBindings,
    ListClusterRoleBindings,
    ListNetworkPolicies,
    ListHPAs,
    ListPDBs,
    ListResourceQuotas,
    ListLimitRanges,
    ListEndpoints,
    ListEndpointSlices,
    ListValidatingWebhookConfigurations,
    ListMutatingWebhookConfigurations,
    ListPriorityClasses,
    ListLeases,
    ListCSIDrivers,
    ListCSINodes,
} from 'wailsjs/go/main/App';

// =============================================================================
// NAMESPACED RESOURCES
// These resources belong to a namespace and support multi-namespace queries.
// Signature: (currentContext, selectedNamespaces, isVisible)
// =============================================================================

/** Hook for Kubernetes Pods */
const usePodsLegacyHook = createNamespacedResourceHook('pods', ListPods, 'pods');
export const usePods = viewCapableNamespaced('pods', 'pods', usePodsLegacyHook);

/** The legacy pod hook on its own: pod notifications diff the whole pod list even in view mode. */
export const usePodsLegacy = usePodsLegacyHook;

/** Hook for Kubernetes Deployments */
export const useDeployments = viewCapableNamespaced('deployments', 'deployments', createNamespacedResourceHook('deployments', ListDeployments, 'deployments'));

/** Hook for Kubernetes StatefulSets */
export const useStatefulSets = viewCapableNamespaced('statefulsets', 'statefulSets', createNamespacedResourceHook('statefulsets', ListStatefulSets, 'statefulSets'));

/** Hook for Kubernetes DaemonSets */
export const useDaemonSets = viewCapableNamespaced('daemonsets', 'daemonSets', createNamespacedResourceHook('daemonsets', ListDaemonSets, 'daemonSets'));

/** Hook for Kubernetes ReplicaSets */
export const useReplicaSets = viewCapableNamespaced('replicasets', 'replicaSets', createNamespacedResourceHook('replicasets', ListReplicaSets, 'replicaSets'));

/** Hook for Kubernetes Jobs */
export const useJobs = viewCapableNamespaced('jobs', 'jobs', createNamespacedResourceHook('jobs', ListJobs, 'jobs'));

/** Hook for Kubernetes CronJobs */
export const useCronJobs = viewCapableNamespaced('cronjobs', 'cronJobs', createNamespacedResourceHook('cronjobs', ListCronJobs, 'cronJobs'));

/** Hook for Kubernetes Services */
export const useServices = viewCapableNamespaced('services', 'services', createNamespacedResourceHook('services', ListServices, 'services'));

/** Hook for Kubernetes Ingresses */
export const useIngresses = viewCapableNamespaced('ingresses', 'ingresses', createNamespacedResourceHook('ingresses', ListIngresses, 'ingresses'));

/** Hook for Kubernetes ConfigMaps */
export const useConfigMaps = viewCapableNamespaced('configmaps', 'configMaps', createNamespacedResourceHook('configmaps', ListConfigMaps, 'configMaps'));

/** Hook for Kubernetes Secrets - uses metadata-only fetch to avoid transferring secret data */
export const useSecrets = viewCapableNamespaced('secrets', 'secrets', createNamespacedResourceHook('secrets', ListSecretsMetadata, 'secrets'));

/** Hook for Kubernetes PersistentVolumeClaims */
export const usePVCs = viewCapableNamespaced('persistentvolumeclaims', 'pvcs', createNamespacedResourceHook('persistentvolumeclaims', ListPVCs, 'pvcs'));

/** Hook for Kubernetes Events */
export const useEventsList = viewCapableNamespaced('events', 'events', createNamespacedResourceHook('events', ListEvents, 'events'));

// =============================================================================
// CLUSTER-SCOPED RESOURCES
// These resources are not namespaced (cluster-wide).
// Signature: (currentContext, isVisible)
// =============================================================================

/** Hook for Kubernetes Nodes */
const useNodesLegacyHook = createClusterScopedResourceHook('nodes', ListNodes, 'nodes');
export const useNodes = viewCapableCluster('nodes', 'nodes', useNodesLegacyHook);

/** The legacy node hook on its own: the topology diagram needs the whole node list even in view mode. */
export const useNodesLegacy = useNodesLegacyHook;

/** Hook for Kubernetes Namespaces */
export const useNamespacesList = viewCapableCluster('namespaces', 'namespaces', createClusterScopedResourceHook('namespaces', ListNamespaces, 'namespaces'));

/** Hook for Kubernetes PersistentVolumes */
export const usePVs = viewCapableCluster('persistentvolumes', 'pvs', createClusterScopedResourceHook('persistentvolumes', ListPVs, 'pvs'));

/** Hook for Kubernetes IngressClasses */
export const useIngressClasses = viewCapableCluster('ingressclasses', 'ingressClasses', createClusterScopedResourceHook('ingressclasses', ListIngressClasses, 'ingressClasses'));

/** Hook for Kubernetes StorageClasses */
export const useStorageClasses = viewCapableCluster('storageclasses', 'storageClasses', createClusterScopedResourceHook('storageclasses', ListStorageClasses, 'storageClasses'));

// =============================================================================
// ACCESS CONTROL / RBAC RESOURCES
// =============================================================================

/** Hook for Kubernetes ServiceAccounts (namespaced) */
export const useServiceAccounts = viewCapableNamespaced('serviceaccounts', 'serviceAccounts', createNamespacedResourceHook('serviceaccounts', ListServiceAccounts, 'serviceAccounts'));

/** Hook for Kubernetes Roles (namespaced) */
export const useRoles = viewCapableNamespaced('roles', 'roles', createNamespacedResourceHook('roles', ListRoles, 'roles'));

/** Hook for Kubernetes RoleBindings (namespaced) */
export const useRoleBindings = viewCapableNamespaced('rolebindings', 'roleBindings', createNamespacedResourceHook('rolebindings', ListRoleBindings, 'roleBindings'));

/** Hook for Kubernetes ClusterRoles (cluster-scoped) */
export const useClusterRoles = viewCapableCluster('clusterroles', 'clusterRoles', createClusterScopedResourceHook('clusterroles', ListClusterRoles, 'clusterRoles'));

/** Hook for Kubernetes ClusterRoleBindings (cluster-scoped) */
export const useClusterRoleBindings = viewCapableCluster('clusterrolebindings', 'clusterRoleBindings', createClusterScopedResourceHook('clusterrolebindings', ListClusterRoleBindings, 'clusterRoleBindings'));

// =============================================================================
// NETWORK RESOURCES
// =============================================================================

/** Hook for Kubernetes NetworkPolicies (namespaced) */
export const useNetworkPolicies = viewCapableNamespaced('networkpolicies', 'networkPolicies', createNamespacedResourceHook('networkpolicies', ListNetworkPolicies, 'networkPolicies'));

/** Hook for Kubernetes Endpoints (namespaced) */
export const useEndpoints = viewCapableNamespaced('endpoints', 'endpoints', createNamespacedResourceHook('endpoints', ListEndpoints, 'endpoints'));

/** Hook for Kubernetes EndpointSlices (namespaced) */
export const useEndpointSlices = viewCapableNamespaced('endpointslices', 'endpointSlices', createNamespacedResourceHook('endpointslices', ListEndpointSlices, 'endpointSlices'));

// =============================================================================
// CONFIG & POLICY RESOURCES
// =============================================================================

/** Hook for Kubernetes HorizontalPodAutoscalers (namespaced) */
export const useHPAs = viewCapableNamespaced('hpas', 'hpas', createNamespacedResourceHook('hpas', ListHPAs, 'hpas'));

/** Hook for Kubernetes PodDisruptionBudgets (namespaced) */
export const usePDBs = viewCapableNamespaced('pdbs', 'pdbs', createNamespacedResourceHook('pdbs', ListPDBs, 'pdbs'));

/** Hook for Kubernetes ResourceQuotas (namespaced) */
export const useResourceQuotas = viewCapableNamespaced('resourcequotas', 'resourceQuotas', createNamespacedResourceHook('resourcequotas', ListResourceQuotas, 'resourceQuotas'));

/** Hook for Kubernetes LimitRanges (namespaced) */
export const useLimitRanges = viewCapableNamespaced('limitranges', 'limitRanges', createNamespacedResourceHook('limitranges', ListLimitRanges, 'limitRanges'));

// =============================================================================
// ADMISSION CONTROL RESOURCES (cluster-scoped)
// =============================================================================

/** Hook for Kubernetes ValidatingWebhookConfigurations */
export const useValidatingWebhookConfigurations = viewCapableCluster('validatingwebhookconfigurations', 'validatingWebhookConfigurations', createClusterScopedResourceHook('validatingwebhookconfigurations', ListValidatingWebhookConfigurations, 'validatingWebhookConfigurations'));

/** Hook for Kubernetes MutatingWebhookConfigurations */
export const useMutatingWebhookConfigurations = viewCapableCluster('mutatingwebhookconfigurations', 'mutatingWebhookConfigurations', createClusterScopedResourceHook('mutatingwebhookconfigurations', ListMutatingWebhookConfigurations, 'mutatingWebhookConfigurations'));

// =============================================================================
// SCHEDULING RESOURCES (cluster-scoped)
// =============================================================================

/** Hook for Kubernetes PriorityClasses */
export const usePriorityClasses = viewCapableCluster('priorityclasses', 'priorityClasses', createClusterScopedResourceHook('priorityclasses', ListPriorityClasses, 'priorityClasses'));

// =============================================================================
// COORDINATION RESOURCES
// =============================================================================

/** Hook for Kubernetes Leases (namespaced) - leader election debugging */
export const useLeases = viewCapableNamespaced('leases', 'leases', createNamespacedResourceHook('leases', ListLeases, 'leases'));

// =============================================================================
// CSI / STORAGE RESOURCES (cluster-scoped)
// =============================================================================

/** Hook for Kubernetes CSIDrivers */
export const useCSIDrivers = viewCapableCluster('csidrivers', 'csiDrivers', createClusterScopedResourceHook('csidrivers', ListCSIDrivers, 'csiDrivers'));

/** Hook for Kubernetes CSINodes */
export const useCSINodes = viewCapableCluster('csinodes', 'csiNodes', createClusterScopedResourceHook('csinodes', ListCSINodes, 'csiNodes'));
