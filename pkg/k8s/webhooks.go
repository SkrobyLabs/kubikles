package k8s

import (
	"context"
	"fmt"
	"log"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

func (c *Client) ListValidatingWebhookConfigurations() ([]admissionregistrationv1.ValidatingWebhookConfiguration, error) {
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return c.ListValidatingWebhookConfigurationsWithContext(ctx)
}

// ListValidatingWebhookConfigurationsWithContext lists validating webhook configurations with cancellation support and pagination.
func (c *Client) ListValidatingWebhookConfigurationsWithContext(ctx context.Context, onProgress ...func(loaded, total int)) ([]admissionregistrationv1.ValidatingWebhookConfiguration, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	var progressFn func(loaded, total int)
	if len(onProgress) > 0 {
		progressFn = onProgress[0]
	}
	result, err := paginatedList(ctx, "validatingwebhooks", defaultPageSize, func(ctx context.Context, opts metav1.ListOptions) ([]admissionregistrationv1.ValidatingWebhookConfiguration, string, *int64, error) {
		list, err := cs.AdmissionregistrationV1().ValidatingWebhookConfigurations().List(ctx, opts)
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

func (c *Client) GetValidatingWebhookConfigurationYaml(name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	wh, err := cs.AdmissionregistrationV1().ValidatingWebhookConfigurations().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	wh.ManagedFields = nil
	yamlBytes, err := yaml.Marshal(wh)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateValidatingWebhookConfigurationYaml(name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	var wh admissionregistrationv1.ValidatingWebhookConfiguration
	if err := yaml.Unmarshal([]byte(yamlContent), &wh); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.AdmissionregistrationV1().ValidatingWebhookConfigurations().Update(ctx, &wh, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteValidatingWebhookConfiguration(contextName, name string) error {
	log.Printf("Deleting validating webhook configuration: context=%s, name=%s", contextName, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return cs.AdmissionregistrationV1().ValidatingWebhookConfigurations().Delete(ctx, name, metav1.DeleteOptions{})
}

// MutatingWebhookConfiguration operations (cluster-scoped)
func (c *Client) ListMutatingWebhookConfigurations() ([]admissionregistrationv1.MutatingWebhookConfiguration, error) {
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return c.ListMutatingWebhookConfigurationsWithContext(ctx)
}

// ListMutatingWebhookConfigurationsWithContext lists mutating webhook configurations with cancellation support and pagination.
func (c *Client) ListMutatingWebhookConfigurationsWithContext(ctx context.Context, onProgress ...func(loaded, total int)) ([]admissionregistrationv1.MutatingWebhookConfiguration, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	var progressFn func(loaded, total int)
	if len(onProgress) > 0 {
		progressFn = onProgress[0]
	}
	result, err := paginatedList(ctx, "mutatingwebhooks", defaultPageSize, func(ctx context.Context, opts metav1.ListOptions) ([]admissionregistrationv1.MutatingWebhookConfiguration, string, *int64, error) {
		list, err := cs.AdmissionregistrationV1().MutatingWebhookConfigurations().List(ctx, opts)
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

func (c *Client) GetMutatingWebhookConfigurationYaml(name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	wh, err := cs.AdmissionregistrationV1().MutatingWebhookConfigurations().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	wh.ManagedFields = nil
	yamlBytes, err := yaml.Marshal(wh)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateMutatingWebhookConfigurationYaml(name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	var wh admissionregistrationv1.MutatingWebhookConfiguration
	if err := yaml.Unmarshal([]byte(yamlContent), &wh); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.AdmissionregistrationV1().MutatingWebhookConfigurations().Update(ctx, &wh, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteMutatingWebhookConfiguration(contextName, name string) error {
	log.Printf("Deleting mutating webhook configuration: context=%s, name=%s", contextName, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return cs.AdmissionregistrationV1().MutatingWebhookConfigurations().Delete(ctx, name, metav1.DeleteOptions{})
}

// PriorityClass operations (cluster-scoped)
