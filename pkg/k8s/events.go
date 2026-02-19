package k8s

import (
	"context"
	"fmt"
	"log"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

func (c *Client) ListEvents(namespace string) ([]v1.Event, error) {
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return c.ListEventsWithContext(ctx, namespace)
}

func (c *Client) ListEventsWithContext(ctx context.Context, namespace string, onProgress ...func(loaded, total int)) ([]v1.Event, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	var progressFn func(loaded, total int)
	if len(onProgress) > 0 {
		progressFn = onProgress[0]
	}
	result, err := paginatedList(ctx, "events", defaultPageSize, func(ctx context.Context, opts metav1.ListOptions) ([]v1.Event, string, *int64, error) {
		list, err := cs.CoreV1().Events(namespace).List(ctx, opts)
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

func (c *Client) GetEventYAML(namespace, name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()

	event, err := cs.CoreV1().Events(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	event.ManagedFields = nil

	yamlBytes, err := yaml.Marshal(event)
	if err != nil {
		return "", fmt.Errorf("failed to marshal event to YAML: %w", err)
	}

	return string(yamlBytes), nil
}

func (c *Client) UpdateEventYAML(namespace, name string, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()

	var event v1.Event
	if err := yaml.Unmarshal([]byte(yamlContent), &event); err != nil {
		return fmt.Errorf("failed to parse yaml: %w", err)
	}

	if event.Namespace != namespace || event.Name != name {
		return fmt.Errorf("namespace/name mismatch in yaml")
	}

	_, err = cs.CoreV1().Events(namespace).Update(ctx, &event, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteEvent(contextName, namespace, name string) error {
	log.Printf("Deleting event: context=%s, ns=%s, name=%s", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return cs.CoreV1().Events(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}
