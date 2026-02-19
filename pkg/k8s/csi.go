package k8s

import (
	"context"
	"fmt"

	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

func (c *Client) ListCSIDrivers(contextName string) ([]storagev1.CSIDriver, error) {
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return c.ListCSIDriversWithContext(ctx, contextName)
}

// ListCSIDriversWithContext lists CSI drivers with cancellation support and pagination.
func (c *Client) ListCSIDriversWithContext(ctx context.Context, contextName string, onProgress ...func(loaded, total int)) ([]storagev1.CSIDriver, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	var progressFn func(loaded, total int)
	if len(onProgress) > 0 {
		progressFn = onProgress[0]
	}
	result, err := paginatedList(ctx, "csidrivers", defaultPageSize, func(ctx context.Context, opts metav1.ListOptions) ([]storagev1.CSIDriver, string, *int64, error) {
		list, err := cs.StorageV1().CSIDrivers().List(ctx, opts)
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

func (c *Client) GetCSIDriverYaml(contextName, name string) (string, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return "", fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	driver, err := cs.StorageV1().CSIDrivers().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	driver.ManagedFields = nil
	yamlBytes, err := yaml.Marshal(driver)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateCSIDriverYaml(contextName, name, yamlContent string) error {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	var driver storagev1.CSIDriver
	if err := yaml.Unmarshal([]byte(yamlContent), &driver); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.StorageV1().CSIDrivers().Update(ctx, &driver, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteCSIDriver(contextName, name string) error {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return cs.StorageV1().CSIDrivers().Delete(ctx, name, metav1.DeleteOptions{})
}

// ============================================================================
// CSINodes (storage.k8s.io/v1) - Cluster-scoped
// ============================================================================

func (c *Client) ListCSINodes(contextName string) ([]storagev1.CSINode, error) {
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return c.ListCSINodesWithContext(ctx, contextName)
}

// ListCSINodesWithContext lists CSI nodes with cancellation support and pagination.
func (c *Client) ListCSINodesWithContext(ctx context.Context, contextName string, onProgress ...func(loaded, total int)) ([]storagev1.CSINode, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	var progressFn func(loaded, total int)
	if len(onProgress) > 0 {
		progressFn = onProgress[0]
	}
	result, err := paginatedList(ctx, "csinodes", defaultPageSize, func(ctx context.Context, opts metav1.ListOptions) ([]storagev1.CSINode, string, *int64, error) {
		list, err := cs.StorageV1().CSINodes().List(ctx, opts)
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

func (c *Client) GetCSINodeYaml(contextName, name string) (string, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return "", fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	node, err := cs.StorageV1().CSINodes().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	node.ManagedFields = nil
	yamlBytes, err := yaml.Marshal(node)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateCSINodeYaml(contextName, name, yamlContent string) error {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	var node storagev1.CSINode
	if err := yaml.Unmarshal([]byte(yamlContent), &node); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.StorageV1().CSINodes().Update(ctx, &node, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteCSINode(contextName, name string) error {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return cs.StorageV1().CSINodes().Delete(ctx, name, metav1.DeleteOptions{})
}

// ============================================================================
// Generic Resource Creation from YAML
// ============================================================================

// kindToResource maps Kubernetes kinds to their plural resource names
