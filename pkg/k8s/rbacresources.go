package k8s

import (
	"context"
	"fmt"
	"log"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

func (c *Client) ListRoles(namespace string) ([]rbacv1.Role, error) {
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return c.ListRolesWithContext(ctx, namespace)
}

// ListRolesWithContext lists roles with cancellation support and pagination.
func (c *Client) ListRolesWithContext(ctx context.Context, namespace string, onProgress ...func(loaded, total int)) ([]rbacv1.Role, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	var progressFn func(loaded, total int)
	if len(onProgress) > 0 {
		progressFn = onProgress[0]
	}
	result, err := paginatedList(ctx, "roles", defaultPageSize, func(ctx context.Context, opts metav1.ListOptions) ([]rbacv1.Role, string, *int64, error) {
		list, err := cs.RbacV1().Roles(namespace).List(ctx, opts)
		if err != nil {
			return nil, "", nil, err
		}
		return list.Items, list.Continue, list.RemainingItemCount, nil
	}, progressFn)
	if err != nil {
		if isCancelledError(err) {
			return nil, ErrRequestCancelled
		}
		return nil, err
	}
	return result, nil
}

// ListRolesForContext lists roles for a specific kubeconfig context
func (c *Client) ListRolesForContext(contextName, namespace string) ([]rbacv1.Role, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	result, err := paginatedList(ctx, "roles", defaultPageSize, func(ctx context.Context, opts metav1.ListOptions) ([]rbacv1.Role, string, *int64, error) {
		list, err := cs.RbacV1().Roles(namespace).List(ctx, opts)
		if err != nil {
			return nil, "", nil, err
		}
		return list.Items, list.Continue, list.RemainingItemCount, nil
	}, nil)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Client) GetRoleYaml(namespace, name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	role, err := cs.RbacV1().Roles(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	role.ManagedFields = nil
	yamlBytes, err := yaml.Marshal(role)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateRoleYaml(namespace, name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	var role rbacv1.Role
	if err := yaml.Unmarshal([]byte(yamlContent), &role); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.RbacV1().Roles(namespace).Update(ctx, &role, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteRole(contextName, namespace, name string) error {
	log.Printf("Deleting role: context=%s, ns=%s, name=%s", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return cs.RbacV1().Roles(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

// ClusterRole operations (cluster-scoped)
func (c *Client) ListClusterRoles() ([]rbacv1.ClusterRole, error) {
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return c.ListClusterRolesWithContext(ctx)
}

// ListClusterRolesForContext lists cluster roles for a specific kubeconfig context
func (c *Client) ListClusterRolesForContext(contextName string) ([]rbacv1.ClusterRole, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	result, err := paginatedList(ctx, "clusterroles", defaultPageSize, func(ctx context.Context, opts metav1.ListOptions) ([]rbacv1.ClusterRole, string, *int64, error) {
		list, err := cs.RbacV1().ClusterRoles().List(ctx, opts)
		if err != nil {
			return nil, "", nil, err
		}
		return list.Items, list.Continue, list.RemainingItemCount, nil
	}, nil)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// ListClusterRolesWithContext lists cluster roles with cancellation support and pagination.
func (c *Client) ListClusterRolesWithContext(ctx context.Context, onProgress ...func(loaded, total int)) ([]rbacv1.ClusterRole, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	var progressFn func(loaded, total int)
	if len(onProgress) > 0 {
		progressFn = onProgress[0]
	}
	result, err := paginatedList(ctx, "clusterroles", defaultPageSize, func(ctx context.Context, opts metav1.ListOptions) ([]rbacv1.ClusterRole, string, *int64, error) {
		list, err := cs.RbacV1().ClusterRoles().List(ctx, opts)
		if err != nil {
			return nil, "", nil, err
		}
		return list.Items, list.Continue, list.RemainingItemCount, nil
	}, progressFn)
	if err != nil {
		if isCancelledError(err) {
			return nil, ErrRequestCancelled
		}
		return nil, err
	}
	return result, nil
}

func (c *Client) GetClusterRoleYaml(name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	role, err := cs.RbacV1().ClusterRoles().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	role.ManagedFields = nil
	yamlBytes, err := yaml.Marshal(role)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateClusterRoleYaml(name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	var role rbacv1.ClusterRole
	if err := yaml.Unmarshal([]byte(yamlContent), &role); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.RbacV1().ClusterRoles().Update(ctx, &role, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteClusterRole(contextName, name string) error {
	log.Printf("Deleting cluster role: context=%s, name=%s", contextName, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return cs.RbacV1().ClusterRoles().Delete(ctx, name, metav1.DeleteOptions{})
}

// RoleBinding operations (namespaced)
func (c *Client) ListRoleBindings(namespace string) ([]rbacv1.RoleBinding, error) {
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return c.ListRoleBindingsWithContext(ctx, namespace)
}

// ListRoleBindingsWithContext lists role bindings with cancellation support and pagination.
func (c *Client) ListRoleBindingsWithContext(ctx context.Context, namespace string, onProgress ...func(loaded, total int)) ([]rbacv1.RoleBinding, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	var progressFn func(loaded, total int)
	if len(onProgress) > 0 {
		progressFn = onProgress[0]
	}
	result, err := paginatedList(ctx, "rolebindings", defaultPageSize, func(ctx context.Context, opts metav1.ListOptions) ([]rbacv1.RoleBinding, string, *int64, error) {
		list, err := cs.RbacV1().RoleBindings(namespace).List(ctx, opts)
		if err != nil {
			return nil, "", nil, err
		}
		return list.Items, list.Continue, list.RemainingItemCount, nil
	}, progressFn)
	if err != nil {
		if isCancelledError(err) {
			return nil, ErrRequestCancelled
		}
		return nil, err
	}
	return result, nil
}

// ListRoleBindingsForContext lists role bindings for a specific kubeconfig context
func (c *Client) ListRoleBindingsForContext(contextName, namespace string) ([]rbacv1.RoleBinding, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	result, err := paginatedList(ctx, "rolebindings", defaultPageSize, func(ctx context.Context, opts metav1.ListOptions) ([]rbacv1.RoleBinding, string, *int64, error) {
		list, err := cs.RbacV1().RoleBindings(namespace).List(ctx, opts)
		if err != nil {
			return nil, "", nil, err
		}
		return list.Items, list.Continue, list.RemainingItemCount, nil
	}, nil)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Client) GetRoleBindingYaml(namespace, name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	binding, err := cs.RbacV1().RoleBindings(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	binding.ManagedFields = nil
	yamlBytes, err := yaml.Marshal(binding)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateRoleBindingYaml(namespace, name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	var binding rbacv1.RoleBinding
	if err := yaml.Unmarshal([]byte(yamlContent), &binding); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.RbacV1().RoleBindings(namespace).Update(ctx, &binding, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteRoleBinding(contextName, namespace, name string) error {
	log.Printf("Deleting role binding: context=%s, ns=%s, name=%s", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return cs.RbacV1().RoleBindings(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

// ClusterRoleBinding operations (cluster-scoped)
func (c *Client) ListClusterRoleBindings() ([]rbacv1.ClusterRoleBinding, error) {
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return c.ListClusterRoleBindingsWithContext(ctx)
}

// ListClusterRoleBindingsForContext lists cluster role bindings for a specific kubeconfig context
func (c *Client) ListClusterRoleBindingsForContext(contextName string) ([]rbacv1.ClusterRoleBinding, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	result, err := paginatedList(ctx, "clusterrolebindings", defaultPageSize, func(ctx context.Context, opts metav1.ListOptions) ([]rbacv1.ClusterRoleBinding, string, *int64, error) {
		list, err := cs.RbacV1().ClusterRoleBindings().List(ctx, opts)
		if err != nil {
			return nil, "", nil, err
		}
		return list.Items, list.Continue, list.RemainingItemCount, nil
	}, nil)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// ListClusterRoleBindingsWithContext lists cluster role bindings with cancellation support and pagination.
func (c *Client) ListClusterRoleBindingsWithContext(ctx context.Context, onProgress ...func(loaded, total int)) ([]rbacv1.ClusterRoleBinding, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	var progressFn func(loaded, total int)
	if len(onProgress) > 0 {
		progressFn = onProgress[0]
	}
	result, err := paginatedList(ctx, "clusterrolebindings", defaultPageSize, func(ctx context.Context, opts metav1.ListOptions) ([]rbacv1.ClusterRoleBinding, string, *int64, error) {
		list, err := cs.RbacV1().ClusterRoleBindings().List(ctx, opts)
		if err != nil {
			return nil, "", nil, err
		}
		return list.Items, list.Continue, list.RemainingItemCount, nil
	}, progressFn)
	if err != nil {
		if isCancelledError(err) {
			return nil, ErrRequestCancelled
		}
		return nil, err
	}
	return result, nil
}

func (c *Client) GetClusterRoleBindingYaml(name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	binding, err := cs.RbacV1().ClusterRoleBindings().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	binding.ManagedFields = nil
	yamlBytes, err := yaml.Marshal(binding)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateClusterRoleBindingYaml(name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	var binding rbacv1.ClusterRoleBinding
	if err := yaml.Unmarshal([]byte(yamlContent), &binding); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.RbacV1().ClusterRoleBindings().Update(ctx, &binding, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteClusterRoleBinding(contextName, name string) error {
	log.Printf("Deleting cluster role binding: context=%s, name=%s", contextName, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return cs.RbacV1().ClusterRoleBindings().Delete(ctx, name, metav1.DeleteOptions{})
}

// NetworkPolicy operations (namespaced)
