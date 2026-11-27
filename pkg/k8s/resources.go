package k8s

import (
	"context"
	"sort"

	admissionv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	schedulingv1 "k8s.io/api/scheduling/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
)

// ResourceDef describes how one built-in resource type is listed and watched
// through a typed clientset. It is the single table behind WatchResource and
// the shared store, so supporting a new type is one entry in builtinResources.
//
// Namespace arguments are ignored for cluster-scoped types, exactly as the
// previous per-type switch did.
type ResourceDef struct {
	// GVR addresses the type on the wire (Table and metadata requests).
	GVR        schema.GroupVersionResource
	Namespaced bool
	// List performs one LIST call and returns the typed list object.
	List func(ctx context.Context, cs kubernetes.Interface, namespace string, opts metav1.ListOptions) (runtime.Object, error)
	// Watch opens one watch.
	Watch func(ctx context.Context, cs kubernetes.Interface, namespace string, opts metav1.ListOptions) (watch.Interface, error)
}

// LookupResource returns the definition for a resource type string such as
// "pods". Custom resources ("crd:...") are not built in and are not found.
func LookupResource(resourceType string) (ResourceDef, bool) {
	d, ok := builtinResources[resourceType]
	return d, ok
}

// ResourceTypes returns the supported built-in resource type strings, sorted.
func ResourceTypes() []string {
	out := make([]string, 0, len(builtinResources))
	for k := range builtinResources {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

type listWatcher[L runtime.Object] interface {
	List(ctx context.Context, opts metav1.ListOptions) (L, error)
	Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error)
}

func def[L runtime.Object](gvr schema.GroupVersionResource, namespaced bool, get func(cs kubernetes.Interface, ns string) listWatcher[L]) ResourceDef {
	return ResourceDef{
		GVR:        gvr,
		Namespaced: namespaced,
		List: func(ctx context.Context, cs kubernetes.Interface, ns string, opts metav1.ListOptions) (runtime.Object, error) {
			l, err := get(cs, ns).List(ctx, opts)
			if err != nil {
				return nil, err
			}
			return l, nil
		},
		Watch: func(ctx context.Context, cs kubernetes.Interface, ns string, opts metav1.ListOptions) (watch.Interface, error) {
			return get(cs, ns).Watch(ctx, opts)
		},
	}
}

func gvr(group, version, resource string) schema.GroupVersionResource {
	return schema.GroupVersionResource{Group: group, Version: version, Resource: resource}
}

var builtinResources = map[string]ResourceDef{
	// Core API (v1)
	"pods": def(gvr("", "v1", "pods"), true, func(cs kubernetes.Interface, ns string) listWatcher[*corev1.PodList] {
		return cs.CoreV1().Pods(ns)
	}),
	"namespaces": def(gvr("", "v1", "namespaces"), false, func(cs kubernetes.Interface, _ string) listWatcher[*corev1.NamespaceList] {
		return cs.CoreV1().Namespaces()
	}),
	"nodes": def(gvr("", "v1", "nodes"), false, func(cs kubernetes.Interface, _ string) listWatcher[*corev1.NodeList] {
		return cs.CoreV1().Nodes()
	}),
	"events": def(gvr("", "v1", "events"), true, func(cs kubernetes.Interface, ns string) listWatcher[*corev1.EventList] {
		return cs.CoreV1().Events(ns)
	}),
	"services": def(gvr("", "v1", "services"), true, func(cs kubernetes.Interface, ns string) listWatcher[*corev1.ServiceList] {
		return cs.CoreV1().Services(ns)
	}),
	"configmaps": def(gvr("", "v1", "configmaps"), true, func(cs kubernetes.Interface, ns string) listWatcher[*corev1.ConfigMapList] {
		return cs.CoreV1().ConfigMaps(ns)
	}),
	"secrets": def(gvr("", "v1", "secrets"), true, func(cs kubernetes.Interface, ns string) listWatcher[*corev1.SecretList] {
		return cs.CoreV1().Secrets(ns)
	}),
	"persistentvolumes": def(gvr("", "v1", "persistentvolumes"), false, func(cs kubernetes.Interface, _ string) listWatcher[*corev1.PersistentVolumeList] {
		return cs.CoreV1().PersistentVolumes()
	}),
	"persistentvolumeclaims": def(gvr("", "v1", "persistentvolumeclaims"), true, func(cs kubernetes.Interface, ns string) listWatcher[*corev1.PersistentVolumeClaimList] {
		return cs.CoreV1().PersistentVolumeClaims(ns)
	}),
	"resourcequotas": def(gvr("", "v1", "resourcequotas"), true, func(cs kubernetes.Interface, ns string) listWatcher[*corev1.ResourceQuotaList] {
		return cs.CoreV1().ResourceQuotas(ns)
	}),
	"limitranges": def(gvr("", "v1", "limitranges"), true, func(cs kubernetes.Interface, ns string) listWatcher[*corev1.LimitRangeList] {
		return cs.CoreV1().LimitRanges(ns)
	}),
	"endpoints": def(gvr("", "v1", "endpoints"), true, func(cs kubernetes.Interface, ns string) listWatcher[*corev1.EndpointsList] {
		return cs.CoreV1().Endpoints(ns)
	}),
	"serviceaccounts": def(gvr("", "v1", "serviceaccounts"), true, func(cs kubernetes.Interface, ns string) listWatcher[*corev1.ServiceAccountList] {
		return cs.CoreV1().ServiceAccounts(ns)
	}),

	// Apps API (v1)
	"deployments": def(gvr("apps", "v1", "deployments"), true, func(cs kubernetes.Interface, ns string) listWatcher[*appsv1.DeploymentList] {
		return cs.AppsV1().Deployments(ns)
	}),
	"statefulsets": def(gvr("apps", "v1", "statefulsets"), true, func(cs kubernetes.Interface, ns string) listWatcher[*appsv1.StatefulSetList] {
		return cs.AppsV1().StatefulSets(ns)
	}),
	"daemonsets": def(gvr("apps", "v1", "daemonsets"), true, func(cs kubernetes.Interface, ns string) listWatcher[*appsv1.DaemonSetList] {
		return cs.AppsV1().DaemonSets(ns)
	}),
	"replicasets": def(gvr("apps", "v1", "replicasets"), true, func(cs kubernetes.Interface, ns string) listWatcher[*appsv1.ReplicaSetList] {
		return cs.AppsV1().ReplicaSets(ns)
	}),

	// Batch API (v1)
	"jobs": def(gvr("batch", "v1", "jobs"), true, func(cs kubernetes.Interface, ns string) listWatcher[*batchv1.JobList] {
		return cs.BatchV1().Jobs(ns)
	}),
	"cronjobs": def(gvr("batch", "v1", "cronjobs"), true, func(cs kubernetes.Interface, ns string) listWatcher[*batchv1.CronJobList] {
		return cs.BatchV1().CronJobs(ns)
	}),

	// Networking API (v1)
	"ingresses": def(gvr("networking.k8s.io", "v1", "ingresses"), true, func(cs kubernetes.Interface, ns string) listWatcher[*networkingv1.IngressList] {
		return cs.NetworkingV1().Ingresses(ns)
	}),
	"ingressclasses": def(gvr("networking.k8s.io", "v1", "ingressclasses"), false, func(cs kubernetes.Interface, _ string) listWatcher[*networkingv1.IngressClassList] {
		return cs.NetworkingV1().IngressClasses()
	}),
	"networkpolicies": def(gvr("networking.k8s.io", "v1", "networkpolicies"), true, func(cs kubernetes.Interface, ns string) listWatcher[*networkingv1.NetworkPolicyList] {
		return cs.NetworkingV1().NetworkPolicies(ns)
	}),

	// Discovery API (v1)
	"endpointslices": def(gvr("discovery.k8s.io", "v1", "endpointslices"), true, func(cs kubernetes.Interface, ns string) listWatcher[*discoveryv1.EndpointSliceList] {
		return cs.DiscoveryV1().EndpointSlices(ns)
	}),

	// Storage API (v1)
	"storageclasses": def(gvr("storage.k8s.io", "v1", "storageclasses"), false, func(cs kubernetes.Interface, _ string) listWatcher[*storagev1.StorageClassList] {
		return cs.StorageV1().StorageClasses()
	}),
	"csidrivers": def(gvr("storage.k8s.io", "v1", "csidrivers"), false, func(cs kubernetes.Interface, _ string) listWatcher[*storagev1.CSIDriverList] {
		return cs.StorageV1().CSIDrivers()
	}),
	"csinodes": def(gvr("storage.k8s.io", "v1", "csinodes"), false, func(cs kubernetes.Interface, _ string) listWatcher[*storagev1.CSINodeList] {
		return cs.StorageV1().CSINodes()
	}),

	// Autoscaling API (v2)
	"hpas": def(gvr("autoscaling", "v2", "horizontalpodautoscalers"), true, func(cs kubernetes.Interface, ns string) listWatcher[*autoscalingv2.HorizontalPodAutoscalerList] {
		return cs.AutoscalingV2().HorizontalPodAutoscalers(ns)
	}),

	// Policy API (v1)
	"pdbs": def(gvr("policy", "v1", "poddisruptionbudgets"), true, func(cs kubernetes.Interface, ns string) listWatcher[*policyv1.PodDisruptionBudgetList] {
		return cs.PolicyV1().PodDisruptionBudgets(ns)
	}),

	// RBAC API (v1)
	"roles": def(gvr("rbac.authorization.k8s.io", "v1", "roles"), true, func(cs kubernetes.Interface, ns string) listWatcher[*rbacv1.RoleList] {
		return cs.RbacV1().Roles(ns)
	}),
	"clusterroles": def(gvr("rbac.authorization.k8s.io", "v1", "clusterroles"), false, func(cs kubernetes.Interface, _ string) listWatcher[*rbacv1.ClusterRoleList] {
		return cs.RbacV1().ClusterRoles()
	}),
	"rolebindings": def(gvr("rbac.authorization.k8s.io", "v1", "rolebindings"), true, func(cs kubernetes.Interface, ns string) listWatcher[*rbacv1.RoleBindingList] {
		return cs.RbacV1().RoleBindings(ns)
	}),
	"clusterrolebindings": def(gvr("rbac.authorization.k8s.io", "v1", "clusterrolebindings"), false, func(cs kubernetes.Interface, _ string) listWatcher[*rbacv1.ClusterRoleBindingList] {
		return cs.RbacV1().ClusterRoleBindings()
	}),

	// Admission Registration API (v1)
	"validatingwebhookconfigurations": def(gvr("admissionregistration.k8s.io", "v1", "validatingwebhookconfigurations"), false, func(cs kubernetes.Interface, _ string) listWatcher[*admissionv1.ValidatingWebhookConfigurationList] {
		return cs.AdmissionregistrationV1().ValidatingWebhookConfigurations()
	}),
	"mutatingwebhookconfigurations": def(gvr("admissionregistration.k8s.io", "v1", "mutatingwebhookconfigurations"), false, func(cs kubernetes.Interface, _ string) listWatcher[*admissionv1.MutatingWebhookConfigurationList] {
		return cs.AdmissionregistrationV1().MutatingWebhookConfigurations()
	}),

	// Scheduling API (v1)
	"priorityclasses": def(gvr("scheduling.k8s.io", "v1", "priorityclasses"), false, func(cs kubernetes.Interface, _ string) listWatcher[*schedulingv1.PriorityClassList] {
		return cs.SchedulingV1().PriorityClasses()
	}),

	// Coordination API (v1)
	"leases": def(gvr("coordination.k8s.io", "v1", "leases"), true, func(cs kubernetes.Interface, ns string) listWatcher[*coordinationv1.LeaseList] {
		return cs.CoordinationV1().Leases(ns)
	}),
}
