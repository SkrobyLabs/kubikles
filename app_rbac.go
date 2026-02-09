package main

import (
	"fmt"

	v1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"

	"kubikles/pkg/debug"
	"kubikles/pkg/k8s"
)

// =============================================================================
// RBAC & Access Control
// =============================================================================

func (a *App) ListServiceAccounts(requestId, namespace string) ([]v1.ServiceAccount, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	if requestId != "" {
		ctx, seq := a.listRequestManager.StartRequest(requestId)
		defer a.listRequestManager.CompleteRequest(requestId, seq)

		result, err := a.k8sClient.ListServiceAccountsWithContext(ctx, namespace)
		if err == k8s.ErrRequestCancelled {
			return nil, nil
		}
		return result, err
	}
	return a.k8sClient.ListServiceAccounts(namespace)
}

func (a *App) GetServiceAccountYaml(namespace, name string) (string, error) {
	debug.LogK8s("GetServiceAccountYaml called", map[string]interface{}{"ns": namespace, "name": name})
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetServiceAccountYaml(namespace, name)
}

func (a *App) UpdateServiceAccountYaml(namespace, name, yamlContent string) error {
	debug.LogK8s("UpdateServiceAccountYaml called", map[string]interface{}{"ns": namespace, "name": name})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateServiceAccountYaml(namespace, name, yamlContent)
}

func (a *App) DeleteServiceAccount(namespace, name string) error {
	currentContext := a.GetCurrentContext()
	debug.LogK8s("DeleteServiceAccount called", map[string]interface{}{"context": currentContext, "ns": namespace, "name": name})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteServiceAccount(currentContext, namespace, name)
}

// Role operations (namespaced)
func (a *App) ListRoles(requestId, namespace string) ([]rbacv1.Role, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	if requestId != "" {
		ctx, seq := a.listRequestManager.StartRequest(requestId)
		defer a.listRequestManager.CompleteRequest(requestId, seq)

		result, err := a.k8sClient.ListRolesWithContext(ctx, namespace)
		if err == k8s.ErrRequestCancelled {
			return nil, nil
		}
		return result, err
	}
	return a.k8sClient.ListRoles(namespace)
}

func (a *App) GetRoleYaml(namespace, name string) (string, error) {
	debug.LogK8s("GetRoleYaml called", map[string]interface{}{"ns": namespace, "name": name})
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetRoleYaml(namespace, name)
}

func (a *App) UpdateRoleYaml(namespace, name, yamlContent string) error {
	debug.LogK8s("UpdateRoleYaml called", map[string]interface{}{"ns": namespace, "name": name})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateRoleYaml(namespace, name, yamlContent)
}

func (a *App) DeleteRole(namespace, name string) error {
	currentContext := a.GetCurrentContext()
	debug.LogK8s("DeleteRole called", map[string]interface{}{"context": currentContext, "ns": namespace, "name": name})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteRole(currentContext, namespace, name)
}

// ClusterRole operations (cluster-scoped)
func (a *App) ListClusterRoles(requestId string) ([]rbacv1.ClusterRole, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	if requestId != "" {
		ctx, seq := a.listRequestManager.StartRequest(requestId)
		defer a.listRequestManager.CompleteRequest(requestId, seq)

		result, err := a.k8sClient.ListClusterRolesWithContext(ctx)
		if err == k8s.ErrRequestCancelled {
			return nil, nil
		}
		return result, err
	}
	return a.k8sClient.ListClusterRoles()
}

func (a *App) GetClusterRoleYaml(name string) (string, error) {
	debug.LogK8s("GetClusterRoleYaml called", map[string]interface{}{"name": name})
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetClusterRoleYaml(name)
}

func (a *App) UpdateClusterRoleYaml(name, yamlContent string) error {
	debug.LogK8s("UpdateClusterRoleYaml called", map[string]interface{}{"name": name})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateClusterRoleYaml(name, yamlContent)
}

func (a *App) DeleteClusterRole(name string) error {
	currentContext := a.GetCurrentContext()
	debug.LogK8s("DeleteClusterRole called", map[string]interface{}{"context": currentContext, "name": name})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteClusterRole(currentContext, name)
}

// RoleBinding operations (namespaced)
func (a *App) ListRoleBindings(requestId, namespace string) ([]rbacv1.RoleBinding, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	if requestId != "" {
		ctx, seq := a.listRequestManager.StartRequest(requestId)
		defer a.listRequestManager.CompleteRequest(requestId, seq)

		result, err := a.k8sClient.ListRoleBindingsWithContext(ctx, namespace)
		if err == k8s.ErrRequestCancelled {
			return nil, nil
		}
		return result, err
	}
	return a.k8sClient.ListRoleBindings(namespace)
}

func (a *App) GetRoleBindingYaml(namespace, name string) (string, error) {
	debug.LogK8s("GetRoleBindingYaml called", map[string]interface{}{"ns": namespace, "name": name})
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetRoleBindingYaml(namespace, name)
}

func (a *App) UpdateRoleBindingYaml(namespace, name, yamlContent string) error {
	debug.LogK8s("UpdateRoleBindingYaml called", map[string]interface{}{"ns": namespace, "name": name})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateRoleBindingYaml(namespace, name, yamlContent)
}

func (a *App) DeleteRoleBinding(namespace, name string) error {
	currentContext := a.GetCurrentContext()
	debug.LogK8s("DeleteRoleBinding called", map[string]interface{}{"context": currentContext, "ns": namespace, "name": name})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteRoleBinding(currentContext, namespace, name)
}

// ClusterRoleBinding operations (cluster-scoped)
func (a *App) ListClusterRoleBindings(requestId string) ([]rbacv1.ClusterRoleBinding, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	if requestId != "" {
		ctx, seq := a.listRequestManager.StartRequest(requestId)
		defer a.listRequestManager.CompleteRequest(requestId, seq)

		result, err := a.k8sClient.ListClusterRoleBindingsWithContext(ctx)
		if err == k8s.ErrRequestCancelled {
			return nil, nil
		}
		return result, err
	}
	return a.k8sClient.ListClusterRoleBindings()
}

func (a *App) GetClusterRoleBindingYaml(name string) (string, error) {
	debug.LogK8s("GetClusterRoleBindingYaml called", map[string]interface{}{"name": name})
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetClusterRoleBindingYaml(name)
}

func (a *App) UpdateClusterRoleBindingYaml(name, yamlContent string) error {
	debug.LogK8s("UpdateClusterRoleBindingYaml called", map[string]interface{}{"name": name})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateClusterRoleBindingYaml(name, yamlContent)
}

func (a *App) DeleteClusterRoleBinding(name string) error {
	currentContext := a.GetCurrentContext()
	debug.LogK8s("DeleteClusterRoleBinding called", map[string]interface{}{"context": currentContext, "name": name})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteClusterRoleBinding(currentContext, name)
}
