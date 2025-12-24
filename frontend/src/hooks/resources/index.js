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
    ListSecrets,
    ListPVCs,
    ListPVs,
    ListNodes,
    ListNamespaces,
    ListEvents,
    ListIngressClasses,
    ListStorageClasses,
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

/** Hook for Kubernetes Secrets */
export const useSecrets = createNamespacedResourceHook('secrets', ListSecrets, 'secrets');

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
