package main

import (
	"fmt"

	v1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
)

// =============================================================================
// RBAC & Access Control
// =============================================================================

func (a *App) ListServiceAccounts(namespace string) ([]v1.ServiceAccount, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListServiceAccounts(namespace)
}

func (a *App) GetServiceAccountYaml(namespace, name string) (string, error) {
	a.logDebug("GetServiceAccountYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetServiceAccountYaml(namespace, name)
}

func (a *App) UpdateServiceAccountYaml(namespace, name, yamlContent string) error {
	a.logDebug("UpdateServiceAccountYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateServiceAccountYaml(namespace, name, yamlContent)
}

func (a *App) DeleteServiceAccount(namespace, name string) error {
	currentContext := a.GetCurrentContext()
	a.logDebug("DeleteServiceAccount called: context=%s, ns=%s, name=%s", currentContext, namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteServiceAccount(currentContext, namespace, name)
}

// Role operations (namespaced)
func (a *App) ListRoles(namespace string) ([]rbacv1.Role, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListRoles(namespace)
}

func (a *App) GetRoleYaml(namespace, name string) (string, error) {
	a.logDebug("GetRoleYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetRoleYaml(namespace, name)
}

func (a *App) UpdateRoleYaml(namespace, name, yamlContent string) error {
	a.logDebug("UpdateRoleYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateRoleYaml(namespace, name, yamlContent)
}

func (a *App) DeleteRole(namespace, name string) error {
	currentContext := a.GetCurrentContext()
	a.logDebug("DeleteRole called: context=%s, ns=%s, name=%s", currentContext, namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteRole(currentContext, namespace, name)
}

// ClusterRole operations (cluster-scoped)
func (a *App) ListClusterRoles() ([]rbacv1.ClusterRole, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListClusterRoles()
}

func (a *App) GetClusterRoleYaml(name string) (string, error) {
	a.logDebug("GetClusterRoleYaml called: name=%s", name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetClusterRoleYaml(name)
}

func (a *App) UpdateClusterRoleYaml(name, yamlContent string) error {
	a.logDebug("UpdateClusterRoleYaml called: name=%s", name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateClusterRoleYaml(name, yamlContent)
}

func (a *App) DeleteClusterRole(name string) error {
	currentContext := a.GetCurrentContext()
	a.logDebug("DeleteClusterRole called: context=%s, name=%s", currentContext, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteClusterRole(currentContext, name)
}

// RoleBinding operations (namespaced)
func (a *App) ListRoleBindings(namespace string) ([]rbacv1.RoleBinding, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListRoleBindings(namespace)
}

func (a *App) GetRoleBindingYaml(namespace, name string) (string, error) {
	a.logDebug("GetRoleBindingYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetRoleBindingYaml(namespace, name)
}

func (a *App) UpdateRoleBindingYaml(namespace, name, yamlContent string) error {
	a.logDebug("UpdateRoleBindingYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateRoleBindingYaml(namespace, name, yamlContent)
}

func (a *App) DeleteRoleBinding(namespace, name string) error {
	currentContext := a.GetCurrentContext()
	a.logDebug("DeleteRoleBinding called: context=%s, ns=%s, name=%s", currentContext, namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteRoleBinding(currentContext, namespace, name)
}

// ClusterRoleBinding operations (cluster-scoped)
func (a *App) ListClusterRoleBindings() ([]rbacv1.ClusterRoleBinding, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListClusterRoleBindings()
}

func (a *App) GetClusterRoleBindingYaml(name string) (string, error) {
	a.logDebug("GetClusterRoleBindingYaml called: name=%s", name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetClusterRoleBindingYaml(name)
}

func (a *App) UpdateClusterRoleBindingYaml(name, yamlContent string) error {
	a.logDebug("UpdateClusterRoleBindingYaml called: name=%s", name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateClusterRoleBindingYaml(name, yamlContent)
}

func (a *App) DeleteClusterRoleBinding(name string) error {
	currentContext := a.GetCurrentContext()
	a.logDebug("DeleteClusterRoleBinding called: context=%s, name=%s", currentContext, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteClusterRoleBinding(currentContext, name)
}
