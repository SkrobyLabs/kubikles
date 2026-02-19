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
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return c.ListIngressesWithContext(ctx, namespace)
}

func (c *Client) ListIngressesWithContext(ctx context.Context, namespace string, onProgress ...func(loaded, total int)) ([]networkingv1.Ingress, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	var progressFn func(loaded, total int)
	if len(onProgress) > 0 {
		progressFn = onProgress[0]
	}
	result, err := paginatedList(ctx, "ingresses", defaultPageSize, func(ctx context.Context, opts metav1.ListOptions) ([]networkingv1.Ingress, string, *int64, error) {
		list, err := cs.NetworkingV1().Ingresses(namespace).List(ctx, opts)
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

// ListIngressesForContext lists ingresses for a specific kubeconfig context
func (c *Client) ListIngressesForContext(contextName, namespace string) ([]networkingv1.Ingress, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	result, err := paginatedList(ctx, "ingresses", defaultPageSize, func(ctx context.Context, opts metav1.ListOptions) ([]networkingv1.Ingress, string, *int64, error) {
		list, err := cs.NetworkingV1().Ingresses(namespace).List(ctx, opts)
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
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return c.ListIngressClassesWithContext(ctx, contextName)
}

func (c *Client) ListIngressClassesWithContext(ctx context.Context, contextName string, onProgress ...func(loaded, total int)) ([]networkingv1.IngressClass, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	var progressFn func(loaded, total int)
	if len(onProgress) > 0 {
		progressFn = onProgress[0]
	}
	result, err := paginatedList(ctx, "ingressclasses", defaultPageSize, func(ctx context.Context, opts metav1.ListOptions) ([]networkingv1.IngressClass, string, *int64, error) {
		list, err := cs.NetworkingV1().IngressClasses().List(ctx, opts)
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
