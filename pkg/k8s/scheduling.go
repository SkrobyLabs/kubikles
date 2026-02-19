package k8s

import (
	"context"
	"fmt"
	"log"

	coordinationv1 "k8s.io/api/coordination/v1"
	schedulingv1 "k8s.io/api/scheduling/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

func (c *Client) ListPriorityClasses() ([]schedulingv1.PriorityClass, error) {
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return c.ListPriorityClassesWithContext(ctx)
}

// ListPriorityClassesWithContext lists priority classes with cancellation support and pagination.
func (c *Client) ListPriorityClassesWithContext(ctx context.Context, onProgress ...func(loaded, total int)) ([]schedulingv1.PriorityClass, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	var progressFn func(loaded, total int)
	if len(onProgress) > 0 {
		progressFn = onProgress[0]
	}
	result, err := paginatedList(ctx, "priorityclasses", defaultPageSize, func(ctx context.Context, opts metav1.ListOptions) ([]schedulingv1.PriorityClass, string, *int64, error) {
		list, err := cs.SchedulingV1().PriorityClasses().List(ctx, opts)
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

func (c *Client) GetPriorityClassYaml(name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	pc, err := cs.SchedulingV1().PriorityClasses().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	pc.ManagedFields = nil
	yamlBytes, err := yaml.Marshal(pc)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdatePriorityClassYaml(name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	var pc schedulingv1.PriorityClass
	if err := yaml.Unmarshal([]byte(yamlContent), &pc); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.SchedulingV1().PriorityClasses().Update(ctx, &pc, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeletePriorityClass(contextName, name string) error {
	log.Printf("Deleting priority class: context=%s, name=%s", contextName, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return cs.SchedulingV1().PriorityClasses().Delete(ctx, name, metav1.DeleteOptions{})
}

// ============================================================================
// Leases (coordination.k8s.io/v1) - Namespaced
// ============================================================================

func (c *Client) ListLeases(contextName, namespace string) ([]coordinationv1.Lease, error) {
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return c.ListLeasesWithContext(ctx, contextName, namespace)
}

// ListLeasesWithContext lists leases with cancellation support and pagination.
func (c *Client) ListLeasesWithContext(ctx context.Context, contextName, namespace string, onProgress ...func(loaded, total int)) ([]coordinationv1.Lease, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	var progressFn func(loaded, total int)
	if len(onProgress) > 0 {
		progressFn = onProgress[0]
	}
	result, err := paginatedList(ctx, "leases", defaultPageSize, func(ctx context.Context, opts metav1.ListOptions) ([]coordinationv1.Lease, string, *int64, error) {
		list, err := cs.CoordinationV1().Leases(namespace).List(ctx, opts)
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

func (c *Client) GetLeaseYaml(contextName, namespace, name string) (string, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return "", fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	lease, err := cs.CoordinationV1().Leases(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	lease.ManagedFields = nil
	yamlBytes, err := yaml.Marshal(lease)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateLeaseYaml(contextName, namespace, name, yamlContent string) error {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	var lease coordinationv1.Lease
	if err := yaml.Unmarshal([]byte(yamlContent), &lease); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.CoordinationV1().Leases(namespace).Update(ctx, &lease, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteLease(contextName, namespace, name string) error {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return cs.CoordinationV1().Leases(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

// ============================================================================
// CSIDrivers (storage.k8s.io/v1) - Cluster-scoped
// ============================================================================
