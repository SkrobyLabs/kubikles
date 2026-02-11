package k8s

import (
	"context"
	"fmt"
	"log"

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

func (c *Client) ListIngresses(namespace string) ([]networkingv1.Ingress, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	ingresses, err := cs.NetworkingV1().Ingresses(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return ingresses.Items, nil
}

func (c *Client) ListIngressesWithContext(ctx context.Context, namespace string) ([]networkingv1.Ingress, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	ingresses, err := cs.NetworkingV1().Ingresses(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		if isCancelledError(err) {
			return nil, ErrRequestCancelled
		}
		return nil, err
	}
	return ingresses.Items, nil
}

// ListIngressesForContext lists ingresses for a specific kubeconfig context
func (c *Client) ListIngressesForContext(contextName, namespace string) ([]networkingv1.Ingress, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	ingresses, err := cs.NetworkingV1().Ingresses(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return ingresses.Items, nil
}

func (c *Client) GetIngressYaml(namespace, name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	ingress, err := cs.NetworkingV1().Ingresses(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	// Remove managed fields
	ingress.ManagedFields = nil

	yamlBytes, err := yaml.Marshal(ingress)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateIngressYaml(namespace, name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	var ingress networkingv1.Ingress
	if err := yaml.Unmarshal([]byte(yamlContent), &ingress); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.NetworkingV1().Ingresses(namespace).Update(ctx, &ingress, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteIngress(contextName, namespace, name string) error {
	log.Printf("Deleting ingress: context=%s, ns=%s, name=%s", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return cs.NetworkingV1().Ingresses(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

// IngressClass operations (cluster-scoped)
func (c *Client) ListIngressClasses(contextName string) ([]networkingv1.IngressClass, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	ingressClasses, err := cs.NetworkingV1().IngressClasses().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return ingressClasses.Items, nil
}

func (c *Client) ListIngressClassesWithContext(ctx context.Context, contextName string) ([]networkingv1.IngressClass, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ingressClasses, err := cs.NetworkingV1().IngressClasses().List(ctx, metav1.ListOptions{})
	if err != nil {
		if isCancelledError(err) {
			return nil, ErrRequestCancelled
		}
		return nil, err
	}
	return ingressClasses.Items, nil
}

func (c *Client) GetIngressClassYaml(name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	ingressClass, err := cs.NetworkingV1().IngressClasses().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	ingressClass.ManagedFields = nil

	yamlBytes, err := yaml.Marshal(ingressClass)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateIngressClassYaml(name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	var ingressClass networkingv1.IngressClass
	if err := yaml.Unmarshal([]byte(yamlContent), &ingressClass); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.NetworkingV1().IngressClasses().Update(ctx, &ingressClass, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteIngressClass(contextName, name string) error {
	log.Printf("Deleting ingressclass: context=%s, name=%s", contextName, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return cs.NetworkingV1().IngressClasses().Delete(ctx, name, metav1.DeleteOptions{})
}
