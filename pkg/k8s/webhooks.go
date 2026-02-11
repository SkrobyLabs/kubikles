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
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	list, err := cs.AdmissionregistrationV1().ValidatingWebhookConfigurations().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) ListValidatingWebhookConfigurationsWithContext(ctx context.Context) ([]admissionregistrationv1.ValidatingWebhookConfiguration, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	list, err := cs.AdmissionregistrationV1().ValidatingWebhookConfigurations().List(ctx, metav1.ListOptions{})
	if err != nil {
		if isCancelledError(err) {
			return nil, ErrRequestCancelled
		}
		return nil, err
	}
	return list.Items, nil
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
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	list, err := cs.AdmissionregistrationV1().MutatingWebhookConfigurations().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) ListMutatingWebhookConfigurationsWithContext(ctx context.Context) ([]admissionregistrationv1.MutatingWebhookConfiguration, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	list, err := cs.AdmissionregistrationV1().MutatingWebhookConfigurations().List(ctx, metav1.ListOptions{})
	if err != nil {
		if isCancelledError(err) {
			return nil, ErrRequestCancelled
		}
		return nil, err
	}
	return list.Items, nil
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
