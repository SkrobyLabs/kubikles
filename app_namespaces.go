package main

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	v1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"

	"kubikles/pkg/debug"
	"kubikles/pkg/k8s"
)

// =============================================================================
// Namespaces
// =============================================================================

func (a *App) ListNamespaces(requestId string) ([]v1.Namespace, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	if requestId != "" {
		ctx, seq := a.listRequestManager.StartRequest(requestId)
		defer a.listRequestManager.CompleteRequest(requestId, seq)

		result, err := a.k8sClient.ListNamespacesWithContext(ctx)
		if err == k8s.ErrRequestCancelled {
			return nil, nil
		}
		return result, err
	}
	return a.k8sClient.ListNamespaces()
}

// ListNamespacesForContext lists namespaces for a specific kubeconfig context
func (a *App) ListNamespacesForContext(contextName string) ([]v1.Namespace, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	// Empty context name means use current context
	if contextName == "" {
		return a.k8sClient.ListNamespaces()
	}
	return a.k8sClient.ListNamespacesForContext(contextName)
}

// ListPodsForContext lists pods for a specific kubeconfig context
func (a *App) ListPodsForContext(contextName, namespace string) ([]v1.Pod, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	if contextName == "" {
		return a.k8sClient.ListPods(namespace)
	}
	return a.k8sClient.ListPodsForContext(contextName, namespace)
}

// ListDeploymentsForContext lists deployments for a specific kubeconfig context
func (a *App) ListDeploymentsForContext(contextName, namespace string) ([]appsv1.Deployment, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	if contextName == "" {
		return a.k8sClient.ListDeployments(namespace)
	}
	return a.k8sClient.ListDeploymentsForContext(contextName, namespace)
}

// ResourceNameItem represents a simple resource reference with name and namespace
type ResourceNameItem struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
}

// ListResourceNamesForContext lists resource names for a specific resource type and context.
// This is a generic function that handles all resource types for cross-context resource selection.
// Returns just the names to avoid complex type handling for 18+ different resource types.
func (a *App) ListResourceNamesForContext(contextName, resourceType, namespace string) ([]ResourceNameItem, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}

	var items []ResourceNameItem

	switch resourceType {
	case "deployment":
		var resources []appsv1.Deployment
		var err error
		if contextName == "" {
			resources, err = a.k8sClient.ListDeployments(namespace)
		} else {
			resources, err = a.k8sClient.ListDeploymentsForContext(contextName, namespace)
		}
		if err != nil {
			return nil, err
		}
		for _, r := range resources {
			items = append(items, ResourceNameItem{Name: r.Name, Namespace: r.Namespace})
		}

	case "statefulset":
		// ListStatefulSets already takes contextName as first param
		resources, err := a.k8sClient.ListStatefulSets(contextName, namespace)
		if err != nil {
			return nil, err
		}
		for _, r := range resources {
			items = append(items, ResourceNameItem{Name: r.Name, Namespace: r.Namespace})
		}

	case "daemonset":
		// ListDaemonSets already takes contextName as first param
		resources, err := a.k8sClient.ListDaemonSets(contextName, namespace)
		if err != nil {
			return nil, err
		}
		for _, r := range resources {
			items = append(items, ResourceNameItem{Name: r.Name, Namespace: r.Namespace})
		}

	case "pod":
		var resources []v1.Pod
		var err error
		if contextName == "" {
			resources, err = a.k8sClient.ListPods(namespace)
		} else {
			resources, err = a.k8sClient.ListPodsForContext(contextName, namespace)
		}
		if err != nil {
			return nil, err
		}
		for _, r := range resources {
			items = append(items, ResourceNameItem{Name: r.Name, Namespace: r.Namespace})
		}

	case "service":
		var resources []v1.Service
		var err error
		if contextName == "" {
			resources, err = a.k8sClient.ListServices(namespace)
		} else {
			resources, err = a.k8sClient.ListServicesForContext(contextName, namespace)
		}
		if err != nil {
			return nil, err
		}
		for _, r := range resources {
			items = append(items, ResourceNameItem{Name: r.Name, Namespace: r.Namespace})
		}

	case "configmap":
		var resources []v1.ConfigMap
		var err error
		if contextName == "" {
			resources, err = a.k8sClient.ListConfigMaps(namespace)
		} else {
			resources, err = a.k8sClient.ListConfigMapsForContext(contextName, namespace)
		}
		if err != nil {
			return nil, err
		}
		for _, r := range resources {
			items = append(items, ResourceNameItem{Name: r.Name, Namespace: r.Namespace})
		}

	case "secret":
		var resources []v1.Secret
		var err error
		if contextName == "" {
			resources, err = a.k8sClient.ListSecrets(namespace)
		} else {
			resources, err = a.k8sClient.ListSecretsForContext(contextName, namespace)
		}
		if err != nil {
			return nil, err
		}
		for _, r := range resources {
			items = append(items, ResourceNameItem{Name: r.Name, Namespace: r.Namespace})
		}

	case "ingress":
		var resources []networkingv1.Ingress
		var err error
		if contextName == "" {
			resources, err = a.k8sClient.ListIngresses(namespace)
		} else {
			resources, err = a.k8sClient.ListIngressesForContext(contextName, namespace)
		}
		if err != nil {
			return nil, err
		}
		for _, r := range resources {
			items = append(items, ResourceNameItem{Name: r.Name, Namespace: r.Namespace})
		}

	case "job":
		// ListJobs already takes contextName as first param
		resources, err := a.k8sClient.ListJobs(contextName, namespace)
		if err != nil {
			return nil, err
		}
		for _, r := range resources {
			items = append(items, ResourceNameItem{Name: r.Name, Namespace: r.Namespace})
		}

	case "cronjob":
		// ListCronJobs already takes contextName as first param
		resources, err := a.k8sClient.ListCronJobs(contextName, namespace)
		if err != nil {
			return nil, err
		}
		for _, r := range resources {
			items = append(items, ResourceNameItem{Name: r.Name, Namespace: r.Namespace})
		}

	case "pvc":
		// ListPVCs already takes contextName as first param
		resources, err := a.k8sClient.ListPVCs(contextName, namespace)
		if err != nil {
			return nil, err
		}
		for _, r := range resources {
			items = append(items, ResourceNameItem{Name: r.Name, Namespace: r.Namespace})
		}

	case "serviceaccount":
		var resources []v1.ServiceAccount
		var err error
		if contextName == "" {
			resources, err = a.k8sClient.ListServiceAccounts(namespace)
		} else {
			resources, err = a.k8sClient.ListServiceAccountsForContext(contextName, namespace)
		}
		if err != nil {
			return nil, err
		}
		for _, r := range resources {
			items = append(items, ResourceNameItem{Name: r.Name, Namespace: r.Namespace})
		}

	case "role":
		var resources []rbacv1.Role
		var err error
		if contextName == "" {
			resources, err = a.k8sClient.ListRoles(namespace)
		} else {
			resources, err = a.k8sClient.ListRolesForContext(contextName, namespace)
		}
		if err != nil {
			return nil, err
		}
		for _, r := range resources {
			items = append(items, ResourceNameItem{Name: r.Name, Namespace: r.Namespace})
		}

	case "rolebinding":
		var resources []rbacv1.RoleBinding
		var err error
		if contextName == "" {
			resources, err = a.k8sClient.ListRoleBindings(namespace)
		} else {
			resources, err = a.k8sClient.ListRoleBindingsForContext(contextName, namespace)
		}
		if err != nil {
			return nil, err
		}
		for _, r := range resources {
			items = append(items, ResourceNameItem{Name: r.Name, Namespace: r.Namespace})
		}

	case "clusterrole":
		var resources []rbacv1.ClusterRole
		var err error
		if contextName == "" {
			resources, err = a.k8sClient.ListClusterRoles()
		} else {
			resources, err = a.k8sClient.ListClusterRolesForContext(contextName)
		}
		if err != nil {
			return nil, err
		}
		for _, r := range resources {
			items = append(items, ResourceNameItem{Name: r.Name})
		}

	case "clusterrolebinding":
		var resources []rbacv1.ClusterRoleBinding
		var err error
		if contextName == "" {
			resources, err = a.k8sClient.ListClusterRoleBindings()
		} else {
			resources, err = a.k8sClient.ListClusterRoleBindingsForContext(contextName)
		}
		if err != nil {
			return nil, err
		}
		for _, r := range resources {
			items = append(items, ResourceNameItem{Name: r.Name})
		}

	case "networkpolicy":
		var resources []networkingv1.NetworkPolicy
		var err error
		if contextName == "" {
			resources, err = a.k8sClient.ListNetworkPolicies(namespace)
		} else {
			resources, err = a.k8sClient.ListNetworkPoliciesForContext(contextName, namespace)
		}
		if err != nil {
			return nil, err
		}
		for _, r := range resources {
			items = append(items, ResourceNameItem{Name: r.Name, Namespace: r.Namespace})
		}

	case "hpa":
		var resources []autoscalingv2.HorizontalPodAutoscaler
		var err error
		if contextName == "" {
			resources, err = a.k8sClient.ListHPAs(namespace)
		} else {
			resources, err = a.k8sClient.ListHPAsForContext(contextName, namespace)
		}
		if err != nil {
			return nil, err
		}
		for _, r := range resources {
			items = append(items, ResourceNameItem{Name: r.Name, Namespace: r.Namespace})
		}

	default:
		return nil, fmt.Errorf("unsupported resource type: %s", resourceType)
	}

	return items, nil
}

func (a *App) GetNamespaceResourceCounts(namespace string) (*k8s.NamespaceResourceCounts, error) {
	debug.LogK8s("GetNamespaceResourceCounts called", map[string]interface{}{"namespace": namespace})
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetNamespaceResourceCounts(namespace)
}

func (a *App) DeleteNamespace(name string) error {
	contextName := a.GetCurrentContext()
	debug.LogK8s("DeleteNamespace called", map[string]interface{}{"context": contextName, "name": name})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteNamespace(contextName, name)
}

func (a *App) GetNamespaceYAML(name string) (string, error) {
	debug.LogK8s("GetNamespaceYAML called", map[string]interface{}{"name": name})
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetNamespaceYAML(name)
}

func (a *App) UpdateNamespaceYAML(name string, yamlContent string) error {
	debug.LogK8s("UpdateNamespaceYAML called", map[string]interface{}{"name": name})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateNamespaceYAML(name, yamlContent)
}
