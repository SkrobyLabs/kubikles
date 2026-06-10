// Code split from client.go; see that file for the Client type and lifecycle.
package k8s

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
)

func (c *Client) WatchResource(ctx context.Context, resourceType, namespace, resourceVersion string) (watch.Interface, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}

	opts := metav1.ListOptions{
		TimeoutSeconds:      ptr(WatchTimeout),
		AllowWatchBookmarks: true,
		ResourceVersion:     resourceVersion,
	}

	switch resourceType {
	// Core API (v1)
	case "pods":
		return cs.CoreV1().Pods(namespace).Watch(ctx, opts)
	case "namespaces":
		return cs.CoreV1().Namespaces().Watch(ctx, opts)
	case "nodes":
		return cs.CoreV1().Nodes().Watch(ctx, opts)
	case "events":
		return cs.CoreV1().Events(namespace).Watch(ctx, opts)
	case "services":
		return cs.CoreV1().Services(namespace).Watch(ctx, opts)
	case "configmaps":
		return cs.CoreV1().ConfigMaps(namespace).Watch(ctx, opts)
	case "secrets":
		return cs.CoreV1().Secrets(namespace).Watch(ctx, opts)
	case "persistentvolumes":
		return cs.CoreV1().PersistentVolumes().Watch(ctx, opts)
	case "persistentvolumeclaims":
		return cs.CoreV1().PersistentVolumeClaims(namespace).Watch(ctx, opts)

	// Apps API (v1)
	case "deployments":
		return cs.AppsV1().Deployments(namespace).Watch(ctx, opts)
	case "statefulsets":
		return cs.AppsV1().StatefulSets(namespace).Watch(ctx, opts)
	case "daemonsets":
		return cs.AppsV1().DaemonSets(namespace).Watch(ctx, opts)
	case "replicasets":
		return cs.AppsV1().ReplicaSets(namespace).Watch(ctx, opts)

	// Batch API (v1)
	case "jobs":
		return cs.BatchV1().Jobs(namespace).Watch(ctx, opts)
	case "cronjobs":
		return cs.BatchV1().CronJobs(namespace).Watch(ctx, opts)

	// Networking API (v1)
	case "ingresses":
		return cs.NetworkingV1().Ingresses(namespace).Watch(ctx, opts)
	case "ingressclasses":
		return cs.NetworkingV1().IngressClasses().Watch(ctx, opts)
	case "networkpolicies":
		return cs.NetworkingV1().NetworkPolicies(namespace).Watch(ctx, opts)

	// Storage API (v1)
	case "storageclasses":
		return cs.StorageV1().StorageClasses().Watch(ctx, opts)
	case "csidrivers":
		return cs.StorageV1().CSIDrivers().Watch(ctx, opts)
	case "csinodes":
		return cs.StorageV1().CSINodes().Watch(ctx, opts)

	// Autoscaling API (v2)
	case "hpas":
		return cs.AutoscalingV2().HorizontalPodAutoscalers(namespace).Watch(ctx, opts)

	// Policy API (v1)
	case "pdbs":
		return cs.PolicyV1().PodDisruptionBudgets(namespace).Watch(ctx, opts)

	// Core API (v1) - additional resources
	case "resourcequotas":
		return cs.CoreV1().ResourceQuotas(namespace).Watch(ctx, opts)
	case "limitranges":
		return cs.CoreV1().LimitRanges(namespace).Watch(ctx, opts)
	case "endpoints":
		return cs.CoreV1().Endpoints(namespace).Watch(ctx, opts)

	// RBAC API (v1) - service accounts already in core
	case "serviceaccounts":
		return cs.CoreV1().ServiceAccounts(namespace).Watch(ctx, opts)
	case "roles":
		return cs.RbacV1().Roles(namespace).Watch(ctx, opts)
	case "clusterroles":
		return cs.RbacV1().ClusterRoles().Watch(ctx, opts)
	case "rolebindings":
		return cs.RbacV1().RoleBindings(namespace).Watch(ctx, opts)
	case "clusterrolebindings":
		return cs.RbacV1().ClusterRoleBindings().Watch(ctx, opts)

	// Admission Registration API (v1)
	case "validatingwebhookconfigurations":
		return cs.AdmissionregistrationV1().ValidatingWebhookConfigurations().Watch(ctx, opts)
	case "mutatingwebhookconfigurations":
		return cs.AdmissionregistrationV1().MutatingWebhookConfigurations().Watch(ctx, opts)

	// Scheduling API (v1)
	case "priorityclasses":
		return cs.SchedulingV1().PriorityClasses().Watch(ctx, opts)

	// Coordination API (v1)
	case "leases":
		return cs.CoordinationV1().Leases(namespace).Watch(ctx, opts)

	default:
		return nil, fmt.Errorf("unsupported resource type: %s", resourceType)
	}
}

// WatchCRD creates a watch for a custom resource using the dynamic client.
// resourceVersion: if non-empty, resumes watch from this version (avoids duplicate ADDED events)
func (c *Client) WatchCRD(ctx context.Context, group, version, resource, namespace, resourceVersion string) (watch.Interface, error) {
	dc, err := c.getDynamicClientForContext("")
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}

	gvr := schema.GroupVersionResource{
		Group:    group,
		Version:  version,
		Resource: resource,
	}

	opts := metav1.ListOptions{
		TimeoutSeconds:      ptr(WatchTimeout),
		AllowWatchBookmarks: true,
		ResourceVersion:     resourceVersion,
	}

	if namespace != "" {
		return dc.Resource(gvr).Namespace(namespace).Watch(ctx, opts)
	}
	return dc.Resource(gvr).Watch(ctx, opts)
}
