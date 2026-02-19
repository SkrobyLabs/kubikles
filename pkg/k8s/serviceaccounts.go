package k8s

import (
	"context"
	"fmt"
	"log"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

func (c *Client) ListServiceAccounts(namespace string) ([]v1.ServiceAccount, error) {
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return c.ListServiceAccountsWithContext(ctx, namespace)
}

// ListServiceAccountsWithContext lists service accounts with cancellation support and pagination.
func (c *Client) ListServiceAccountsWithContext(ctx context.Context, namespace string, onProgress ...func(loaded, total int)) ([]v1.ServiceAccount, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	var progressFn func(loaded, total int)
	if len(onProgress) > 0 {
		progressFn = onProgress[0]
	}
	result, err := paginatedList(ctx, "serviceaccounts", defaultPageSize, func(ctx context.Context, opts metav1.ListOptions) ([]v1.ServiceAccount, string, *int64, error) {
		list, err := cs.CoreV1().ServiceAccounts(namespace).List(ctx, opts)
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

// ListServiceAccountsForContext lists service accounts for a specific kubeconfig context
func (c *Client) ListServiceAccountsForContext(contextName, namespace string) ([]v1.ServiceAccount, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	result, err := paginatedList(ctx, "serviceaccounts", defaultPageSize, func(ctx context.Context, opts metav1.ListOptions) ([]v1.ServiceAccount, string, *int64, error) {
		list, err := cs.CoreV1().ServiceAccounts(namespace).List(ctx, opts)
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

func (c *Client) GetServiceAccountYaml(namespace, name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	sa, err := cs.CoreV1().ServiceAccounts(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	sa.ManagedFields = nil
	yamlBytes, err := yaml.Marshal(sa)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateServiceAccountYaml(namespace, name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	var sa v1.ServiceAccount
	if err := yaml.Unmarshal([]byte(yamlContent), &sa); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.CoreV1().ServiceAccounts(namespace).Update(ctx, &sa, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteServiceAccount(contextName, namespace, name string) error {
	log.Printf("Deleting service account: context=%s, ns=%s, name=%s", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return cs.CoreV1().ServiceAccounts(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

// Role operations (namespaced)
