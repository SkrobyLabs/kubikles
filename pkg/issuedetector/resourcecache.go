package issuedetector

import (
	"context"
	"fmt"
	"sync"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"

	"kubikles/pkg/k8s"

	"golang.org/x/sync/errgroup"
)

// ResourceCache holds fetched Kubernetes resources for rule evaluation.
// It is populated once per scan and is request-scoped (no stale data).
type ResourceCache struct {
	client     *k8s.Client
	namespaces []string // empty = all namespaces

	mu   sync.RWMutex
	data map[string]interface{} // kind -> typed slice

	// Non-fatal errors encountered during fetching (e.g. RBAC)
	errors []string
}

// NewResourceCache creates a new empty resource cache.
func NewResourceCache(client *k8s.Client, namespaces []string) *ResourceCache {
	return &ResourceCache{
		client:     client,
		namespaces: namespaces,
		data:       make(map[string]interface{}),
	}
}

// Fetch populates the cache with the requested resource kinds in parallel.
func (rc *ResourceCache) Fetch(ctx context.Context, kinds []string) error {
	g, gctx := errgroup.WithContext(ctx)

	for _, kind := range kinds {
		kind := kind
		g.Go(func() error {
			if err := rc.fetchKind(gctx, kind); err != nil {
				// Non-fatal: collect error and continue
				rc.mu.Lock()
				rc.errors = append(rc.errors, fmt.Sprintf("failed to fetch %s: %v", kind, err))
				rc.mu.Unlock()
			}
			return nil // always nil so other fetches continue
		})
	}

	return g.Wait()
}

// Errors returns any non-fatal errors from fetching.
func (rc *ResourceCache) Errors() []string {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	return append([]string{}, rc.errors...)
}

// ResourceCount returns the number of resources fetched for a kind.
func (rc *ResourceCache) ResourceCount(kind string) int {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	switch v := rc.data[kind].(type) {
	case []v1.Pod:
		return len(v)
	case []v1.Service:
		return len(v)
	case []networkingv1.Ingress:
		return len(v)
	case []networkingv1.IngressClass:
		return len(v)
	case []v1.Endpoints:
		return len(v)
	case []v1.ConfigMap:
		return len(v)
	case []v1.Secret:
		return len(v)
	case []appsv1.Deployment:
		return len(v)
	case []appsv1.StatefulSet:
		return len(v)
	case []appsv1.DaemonSet:
		return len(v)
	case []v1.PersistentVolumeClaim:
		return len(v)
	case []v1.PersistentVolume:
		return len(v)
	case []v1.Node:
		return len(v)
	case []v1.ServiceAccount:
		return len(v)
	case []autoscalingv2.HorizontalPodAutoscaler:
		return len(v)
	case []batchv1.Job:
		return len(v)
	case []batchv1.CronJob:
		return len(v)
	case []rbacv1.Role:
		return len(v)
	case []rbacv1.ClusterRole:
		return len(v)
	case []rbacv1.RoleBinding:
		return len(v)
	case []rbacv1.ClusterRoleBinding:
		return len(v)
	default:
		return 0
	}
}

// ---- Typed getters (return empty slice, never nil) ----

func (rc *ResourceCache) Pods() []v1.Pod {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	if v, ok := rc.data["pods"].([]v1.Pod); ok {
		return v
	}
	return []v1.Pod{}
}

func (rc *ResourceCache) Services() []v1.Service {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	if v, ok := rc.data["services"].([]v1.Service); ok {
		return v
	}
	return []v1.Service{}
}

func (rc *ResourceCache) Ingresses() []networkingv1.Ingress {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	if v, ok := rc.data["ingresses"].([]networkingv1.Ingress); ok {
		return v
	}
	return []networkingv1.Ingress{}
}

func (rc *ResourceCache) IngressClasses() []networkingv1.IngressClass {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	if v, ok := rc.data["ingressclasses"].([]networkingv1.IngressClass); ok {
		return v
	}
	return []networkingv1.IngressClass{}
}

func (rc *ResourceCache) Endpoints() []v1.Endpoints {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	if v, ok := rc.data["endpoints"].([]v1.Endpoints); ok {
		return v
	}
	return []v1.Endpoints{}
}

func (rc *ResourceCache) ConfigMaps() []v1.ConfigMap {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	if v, ok := rc.data["configmaps"].([]v1.ConfigMap); ok {
		return v
	}
	return []v1.ConfigMap{}
}

func (rc *ResourceCache) Secrets() []v1.Secret {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	if v, ok := rc.data["secrets"].([]v1.Secret); ok {
		return v
	}
	return []v1.Secret{}
}

func (rc *ResourceCache) Deployments() []appsv1.Deployment {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	if v, ok := rc.data["deployments"].([]appsv1.Deployment); ok {
		return v
	}
	return []appsv1.Deployment{}
}

func (rc *ResourceCache) StatefulSets() []appsv1.StatefulSet {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	if v, ok := rc.data["statefulsets"].([]appsv1.StatefulSet); ok {
		return v
	}
	return []appsv1.StatefulSet{}
}

func (rc *ResourceCache) DaemonSets() []appsv1.DaemonSet {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	if v, ok := rc.data["daemonsets"].([]appsv1.DaemonSet); ok {
		return v
	}
	return []appsv1.DaemonSet{}
}

func (rc *ResourceCache) PVCs() []v1.PersistentVolumeClaim {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	if v, ok := rc.data["pvcs"].([]v1.PersistentVolumeClaim); ok {
		return v
	}
	return []v1.PersistentVolumeClaim{}
}

func (rc *ResourceCache) PVs() []v1.PersistentVolume {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	if v, ok := rc.data["pvs"].([]v1.PersistentVolume); ok {
		return v
	}
	return []v1.PersistentVolume{}
}

func (rc *ResourceCache) Nodes() []v1.Node {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	if v, ok := rc.data["nodes"].([]v1.Node); ok {
		return v
	}
	return []v1.Node{}
}

func (rc *ResourceCache) ServiceAccounts() []v1.ServiceAccount {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	if v, ok := rc.data["serviceaccounts"].([]v1.ServiceAccount); ok {
		return v
	}
	return []v1.ServiceAccount{}
}

func (rc *ResourceCache) HPAs() []autoscalingv2.HorizontalPodAutoscaler {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	if v, ok := rc.data["hpas"].([]autoscalingv2.HorizontalPodAutoscaler); ok {
		return v
	}
	return []autoscalingv2.HorizontalPodAutoscaler{}
}

func (rc *ResourceCache) Jobs() []batchv1.Job {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	if v, ok := rc.data["jobs"].([]batchv1.Job); ok {
		return v
	}
	return []batchv1.Job{}
}

func (rc *ResourceCache) CronJobs() []batchv1.CronJob {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	if v, ok := rc.data["cronjobs"].([]batchv1.CronJob); ok {
		return v
	}
	return []batchv1.CronJob{}
}

func (rc *ResourceCache) Roles() []rbacv1.Role {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	if v, ok := rc.data["roles"].([]rbacv1.Role); ok {
		return v
	}
	return []rbacv1.Role{}
}

func (rc *ResourceCache) ClusterRoles() []rbacv1.ClusterRole {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	if v, ok := rc.data["clusterroles"].([]rbacv1.ClusterRole); ok {
		return v
	}
	return []rbacv1.ClusterRole{}
}

func (rc *ResourceCache) RoleBindings() []rbacv1.RoleBinding {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	if v, ok := rc.data["rolebindings"].([]rbacv1.RoleBinding); ok {
		return v
	}
	return []rbacv1.RoleBinding{}
}

func (rc *ResourceCache) ClusterRoleBindings() []rbacv1.ClusterRoleBinding {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	if v, ok := rc.data["clusterrolebindings"].([]rbacv1.ClusterRoleBinding); ok {
		return v
	}
	return []rbacv1.ClusterRoleBinding{}
}

// ---- Internal fetch dispatching ----

func (rc *ResourceCache) fetchKind(ctx context.Context, kind string) error {
	switch kind {
	case "pods":
		return rc.fetchNamespaced(ctx, kind, func(ctx context.Context, ns string) (interface{}, error) {
			return rc.client.ListPodsWithContext(ctx, ns)
		}, func(results []interface{}) interface{} {
			var all []v1.Pod
			for _, r := range results {
				all = append(all, r.([]v1.Pod)...)
			}
			return all
		})
	case "services":
		return rc.fetchNamespaced(ctx, kind, func(ctx context.Context, ns string) (interface{}, error) {
			return rc.client.ListServicesWithContext(ctx, ns)
		}, func(results []interface{}) interface{} {
			var all []v1.Service
			for _, r := range results {
				all = append(all, r.([]v1.Service)...)
			}
			return all
		})
	case "ingresses":
		return rc.fetchNamespaced(ctx, kind, func(ctx context.Context, ns string) (interface{}, error) {
			return rc.client.ListIngressesWithContext(ctx, ns)
		}, func(results []interface{}) interface{} {
			var all []networkingv1.Ingress
			for _, r := range results {
				all = append(all, r.([]networkingv1.Ingress)...)
			}
			return all
		})
	case "ingressclasses":
		return rc.fetchClusterScoped(ctx, kind, func(ctx context.Context) (interface{}, error) {
			return rc.client.ListIngressClassesWithContext(ctx, "")
		})
	case "endpoints":
		return rc.fetchNamespaced(ctx, kind, func(ctx context.Context, ns string) (interface{}, error) {
			return rc.client.ListEndpointsWithContext(ctx, ns)
		}, func(results []interface{}) interface{} {
			var all []v1.Endpoints
			for _, r := range results {
				all = append(all, r.([]v1.Endpoints)...)
			}
			return all
		})
	case "configmaps":
		return rc.fetchNamespaced(ctx, kind, func(ctx context.Context, ns string) (interface{}, error) {
			return rc.client.ListConfigMapsWithContext(ctx, ns)
		}, func(results []interface{}) interface{} {
			var all []v1.ConfigMap
			for _, r := range results {
				all = append(all, r.([]v1.ConfigMap)...)
			}
			return all
		})
	case "secrets":
		return rc.fetchNamespaced(ctx, kind, func(ctx context.Context, ns string) (interface{}, error) {
			return rc.client.ListSecretsWithContext(ctx, ns)
		}, func(results []interface{}) interface{} {
			var all []v1.Secret
			for _, r := range results {
				all = append(all, r.([]v1.Secret)...)
			}
			return all
		})
	case "deployments":
		return rc.fetchNamespaced(ctx, kind, func(ctx context.Context, ns string) (interface{}, error) {
			return rc.client.ListDeploymentsWithContext(ctx, ns)
		}, func(results []interface{}) interface{} {
			var all []appsv1.Deployment
			for _, r := range results {
				all = append(all, r.([]appsv1.Deployment)...)
			}
			return all
		})
	case "statefulsets":
		return rc.fetchNamespaced(ctx, kind, func(ctx context.Context, ns string) (interface{}, error) {
			return rc.client.ListStatefulSetsWithContext(ctx, "", ns)
		}, func(results []interface{}) interface{} {
			var all []appsv1.StatefulSet
			for _, r := range results {
				all = append(all, r.([]appsv1.StatefulSet)...)
			}
			return all
		})
	case "daemonsets":
		return rc.fetchNamespaced(ctx, kind, func(ctx context.Context, ns string) (interface{}, error) {
			return rc.client.ListDaemonSetsWithContext(ctx, "", ns)
		}, func(results []interface{}) interface{} {
			var all []appsv1.DaemonSet
			for _, r := range results {
				all = append(all, r.([]appsv1.DaemonSet)...)
			}
			return all
		})
	case "pvcs":
		return rc.fetchNamespaced(ctx, kind, func(ctx context.Context, ns string) (interface{}, error) {
			return rc.client.ListPVCsWithContext(ctx, "", ns)
		}, func(results []interface{}) interface{} {
			var all []v1.PersistentVolumeClaim
			for _, r := range results {
				all = append(all, r.([]v1.PersistentVolumeClaim)...)
			}
			return all
		})
	case "pvs":
		return rc.fetchClusterScoped(ctx, kind, func(ctx context.Context) (interface{}, error) {
			return rc.client.ListPVsWithContext(ctx, "")
		})
	case "nodes":
		return rc.fetchClusterScoped(ctx, kind, func(ctx context.Context) (interface{}, error) {
			return rc.client.ListNodesWithContext(ctx)
		})
	case "serviceaccounts":
		return rc.fetchNamespaced(ctx, kind, func(ctx context.Context, ns string) (interface{}, error) {
			return rc.client.ListServiceAccountsWithContext(ctx, ns)
		}, func(results []interface{}) interface{} {
			var all []v1.ServiceAccount
			for _, r := range results {
				all = append(all, r.([]v1.ServiceAccount)...)
			}
			return all
		})
	case "hpas":
		return rc.fetchNamespaced(ctx, kind, func(ctx context.Context, ns string) (interface{}, error) {
			return rc.client.ListHPAsWithContext(ctx, ns)
		}, func(results []interface{}) interface{} {
			var all []autoscalingv2.HorizontalPodAutoscaler
			for _, r := range results {
				all = append(all, r.([]autoscalingv2.HorizontalPodAutoscaler)...)
			}
			return all
		})
	case "jobs":
		return rc.fetchNamespaced(ctx, kind, func(ctx context.Context, ns string) (interface{}, error) {
			return rc.client.ListJobsWithContext(ctx, "", ns)
		}, func(results []interface{}) interface{} {
			var all []batchv1.Job
			for _, r := range results {
				all = append(all, r.([]batchv1.Job)...)
			}
			return all
		})
	case "cronjobs":
		return rc.fetchNamespaced(ctx, kind, func(ctx context.Context, ns string) (interface{}, error) {
			return rc.client.ListCronJobsWithContext(ctx, "", ns)
		}, func(results []interface{}) interface{} {
			var all []batchv1.CronJob
			for _, r := range results {
				all = append(all, r.([]batchv1.CronJob)...)
			}
			return all
		})
	case "roles":
		return rc.fetchNamespaced(ctx, kind, func(ctx context.Context, ns string) (interface{}, error) {
			return rc.client.ListRolesWithContext(ctx, ns)
		}, func(results []interface{}) interface{} {
			var all []rbacv1.Role
			for _, r := range results {
				all = append(all, r.([]rbacv1.Role)...)
			}
			return all
		})
	case "clusterroles":
		return rc.fetchClusterScoped(ctx, kind, func(ctx context.Context) (interface{}, error) {
			return rc.client.ListClusterRolesWithContext(ctx)
		})
	case "rolebindings":
		return rc.fetchNamespaced(ctx, kind, func(ctx context.Context, ns string) (interface{}, error) {
			return rc.client.ListRoleBindingsWithContext(ctx, ns)
		}, func(results []interface{}) interface{} {
			var all []rbacv1.RoleBinding
			for _, r := range results {
				all = append(all, r.([]rbacv1.RoleBinding)...)
			}
			return all
		})
	case "clusterrolebindings":
		return rc.fetchClusterScoped(ctx, kind, func(ctx context.Context) (interface{}, error) {
			return rc.client.ListClusterRoleBindingsWithContext(ctx)
		})
	default:
		return fmt.Errorf("unknown resource kind: %s", kind)
	}
}

// fetchNamespaced fetches a namespaced resource. If rc.namespaces is empty, passes "" to fetch all.
func (rc *ResourceCache) fetchNamespaced(
	ctx context.Context,
	kind string,
	fetch func(ctx context.Context, ns string) (interface{}, error),
	merge func(results []interface{}) interface{},
) error {
	if len(rc.namespaces) == 0 {
		result, err := fetch(ctx, "")
		if err != nil {
			return err
		}
		rc.mu.Lock()
		rc.data[kind] = result
		rc.mu.Unlock()
		return nil
	}

	var results []interface{}
	for _, ns := range rc.namespaces {
		result, err := fetch(ctx, ns)
		if err != nil {
			return err
		}
		results = append(results, result)
	}

	rc.mu.Lock()
	rc.data[kind] = merge(results)
	rc.mu.Unlock()
	return nil
}

// fetchClusterScoped fetches a cluster-scoped resource.
func (rc *ResourceCache) fetchClusterScoped(
	ctx context.Context,
	kind string,
	fetch func(ctx context.Context) (interface{}, error),
) error {
	result, err := fetch(ctx)
	if err != nil {
		return err
	}
	rc.mu.Lock()
	rc.data[kind] = result
	rc.mu.Unlock()
	return nil
}
