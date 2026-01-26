/**
 * Resource hooks factory - consolidated hooks for Kubernetes resources.
 *
 * This module replaces individual hook files (usePods.js, useDeployments.js, etc.)
 * with a factory pattern that eliminates code duplication.
 *
 * Usage:
 *   import { usePods, useDeployments } from '../../hooks/resources';
 *
 * Namespaced hooks signature:
 *   const { pods, loading, error, setPods } = usePods(currentContext, selectedNamespaces, isVisible);
 *
 * Cluster-scoped hooks signature:
 *   const { nodes, loading, error, refetch } = useNodes(currentContext, isVisible);
 */

import { createNamespacedResourceHook, createClusterScopedResourceHook } from '../useResource';

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
} from '../../../wailsjs/go/main/App';

// =============================================================================
// NAMESPACED RESOURCES
// These resources belong to a namespace and support multi-namespace queries.
// Signature: (currentContext, selectedNamespaces, isVisible)
// =============================================================================

/** Hook for Kubernetes Pods */
export const usePods = createNamespacedResourceHook('pods', ListPods, 'pods');

/** Hook for Kubernetes Deployments */
export const useDeployments = createNamespacedResourceHook('deployments', ListDeployments, 'deployments');

/** Hook for Kubernetes StatefulSets */
export const useStatefulSets = createNamespacedResourceHook('statefulsets', ListStatefulSets, 'statefulSets');

/** Hook for Kubernetes DaemonSets */
export const useDaemonSets = createNamespacedResourceHook('daemonsets', ListDaemonSets, 'daemonSets');

/** Hook for Kubernetes ReplicaSets */
export const useReplicaSets = createNamespacedResourceHook('replicasets', ListReplicaSets, 'replicaSets');

/** Hook for Kubernetes Jobs */
export const useJobs = createNamespacedResourceHook('jobs', ListJobs, 'jobs');

/** Hook for Kubernetes CronJobs */
export const useCronJobs = createNamespacedResourceHook('cronjobs', ListCronJobs, 'cronJobs');

/** Hook for Kubernetes Services */
export const useServices = createNamespacedResourceHook('services', ListServices, 'services');

/** Hook for Kubernetes Ingresses */
export const useIngresses = createNamespacedResourceHook('ingresses', ListIngresses, 'ingresses');

/** Hook for Kubernetes ConfigMaps */
export const useConfigMaps = createNamespacedResourceHook('configmaps', ListConfigMaps, 'configMaps');

/** Hook for Kubernetes Secrets - uses metadata-only fetch to avoid transferring secret data */
export const useSecrets = createNamespacedResourceHook('secrets', ListSecretsMetadata, 'secrets');

/** Hook for Kubernetes PersistentVolumeClaims */
export const usePVCs = createNamespacedResourceHook('persistentvolumeclaims', ListPVCs, 'pvcs');

/** Hook for Kubernetes Events */
export const useEventsList = createNamespacedResourceHook('events', ListEvents, 'events');

// =============================================================================
// CLUSTER-SCOPED RESOURCES
// These resources are not namespaced (cluster-wide).
// Signature: (currentContext, isVisible)
// =============================================================================

/** Hook for Kubernetes Nodes */
export const useNodes = createClusterScopedResourceHook('nodes', ListNodes, 'nodes');

/** Hook for Kubernetes Namespaces */
export const useNamespacesList = createClusterScopedResourceHook('namespaces', ListNamespaces, 'namespaces');

/** Hook for Kubernetes PersistentVolumes */
export const usePVs = createClusterScopedResourceHook('persistentvolumes', ListPVs, 'pvs');

/** Hook for Kubernetes IngressClasses */
export const useIngressClasses = createClusterScopedResourceHook('ingressclasses', ListIngressClasses, 'ingressClasses');

/** Hook for Kubernetes StorageClasses */
export const useStorageClasses = createClusterScopedResourceHook('storageclasses', ListStorageClasses, 'storageClasses');

// =============================================================================
// ACCESS CONTROL / RBAC RESOURCES
// =============================================================================

/** Hook for Kubernetes ServiceAccounts (namespaced) */
export const useServiceAccounts = createNamespacedResourceHook('serviceaccounts', ListServiceAccounts, 'serviceAccounts');

/** Hook for Kubernetes Roles (namespaced) */
export const useRoles = createNamespacedResourceHook('roles', ListRoles, 'roles');

/** Hook for Kubernetes RoleBindings (namespaced) */
export const useRoleBindings = createNamespacedResourceHook('rolebindings', ListRoleBindings, 'roleBindings');

/** Hook for Kubernetes ClusterRoles (cluster-scoped) */
export const useClusterRoles = createClusterScopedResourceHook('clusterroles', ListClusterRoles, 'clusterRoles');

/** Hook for Kubernetes ClusterRoleBindings (cluster-scoped) */
export const useClusterRoleBindings = createClusterScopedResourceHook('clusterrolebindings', ListClusterRoleBindings, 'clusterRoleBindings');

// =============================================================================
// NETWORK RESOURCES
// =============================================================================

/** Hook for Kubernetes NetworkPolicies (namespaced) */
export const useNetworkPolicies = createNamespacedResourceHook('networkpolicies', ListNetworkPolicies, 'networkPolicies');

/** Hook for Kubernetes Endpoints (namespaced) */
export const useEndpoints = createNamespacedResourceHook('endpoints', ListEndpoints, 'endpoints');

/** Hook for Kubernetes EndpointSlices (namespaced) */
export const useEndpointSlices = createNamespacedResourceHook('endpointslices', ListEndpointSlices, 'endpointSlices');

// =============================================================================
// CONFIG & POLICY RESOURCES
// =============================================================================

/** Hook for Kubernetes HorizontalPodAutoscalers (namespaced) */
export const useHPAs = createNamespacedResourceHook('hpas', ListHPAs, 'hpas');

/** Hook for Kubernetes PodDisruptionBudgets (namespaced) */
export const usePDBs = createNamespacedResourceHook('pdbs', ListPDBs, 'pdbs');

/** Hook for Kubernetes ResourceQuotas (namespaced) */
export const useResourceQuotas = createNamespacedResourceHook('resourcequotas', ListResourceQuotas, 'resourceQuotas');

/** Hook for Kubernetes LimitRanges (namespaced) */
export const useLimitRanges = createNamespacedResourceHook('limitranges', ListLimitRanges, 'limitRanges');

// =============================================================================
// ADMISSION CONTROL RESOURCES (cluster-scoped)
// =============================================================================

/** Hook for Kubernetes ValidatingWebhookConfigurations */
export const useValidatingWebhookConfigurations = createClusterScopedResourceHook('validatingwebhookconfigurations', ListValidatingWebhookConfigurations, 'validatingWebhookConfigurations');

/** Hook for Kubernetes MutatingWebhookConfigurations */
export const useMutatingWebhookConfigurations = createClusterScopedResourceHook('mutatingwebhookconfigurations', ListMutatingWebhookConfigurations, 'mutatingWebhookConfigurations');

// =============================================================================
// SCHEDULING RESOURCES (cluster-scoped)
// =============================================================================

/** Hook for Kubernetes PriorityClasses */
export const usePriorityClasses = createClusterScopedResourceHook('priorityclasses', ListPriorityClasses, 'priorityClasses');

// =============================================================================
// COORDINATION RESOURCES
// =============================================================================

/** Hook for Kubernetes Leases (namespaced) - leader election debugging */
export const useLeases = createNamespacedResourceHook('leases', ListLeases, 'leases');

// =============================================================================
// CSI / STORAGE RESOURCES (cluster-scoped)
// =============================================================================

/** Hook for Kubernetes CSIDrivers */
export const useCSIDrivers = createClusterScopedResourceHook('csidrivers', ListCSIDrivers, 'csiDrivers');

/** Hook for Kubernetes CSINodes */
export const useCSINodes = createClusterScopedResourceHook('csinodes', ListCSINodes, 'csiNodes');
