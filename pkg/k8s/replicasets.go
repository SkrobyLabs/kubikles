package k8s

import (
	"context"
	"fmt"
	"log"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

func (c *Client) ListReplicaSets(contextName, namespace string) ([]appsv1.ReplicaSet, error) {
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return c.ListReplicaSetsWithContext(ctx, contextName, namespace)
}

// ListReplicaSetsWithContext lists replicasets with cancellation support and pagination.
func (c *Client) ListReplicaSetsWithContext(ctx context.Context, contextName, namespace string, onProgress ...func(loaded, total int)) ([]appsv1.ReplicaSet, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	var progressFn func(loaded, total int)
	if len(onProgress) > 0 {
		progressFn = onProgress[0]
	}
	result, err := paginatedList(ctx, "replicasets", defaultPageSize, func(ctx context.Context, opts metav1.ListOptions) ([]appsv1.ReplicaSet, string, *int64, error) {
		list, err := cs.AppsV1().ReplicaSets(namespace).List(ctx, opts)
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

func (c *Client) GetReplicaSetYaml(namespace, name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	replicaset, err := cs.AppsV1().ReplicaSets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	// Remove managed fields
	replicaset.ManagedFields = nil

	yamlBytes, err := yaml.Marshal(replicaset)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateReplicaSetYaml(namespace, name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	var replicaset appsv1.ReplicaSet
	if err := yaml.Unmarshal([]byte(yamlContent), &replicaset); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.AppsV1().ReplicaSets(namespace).Update(ctx, &replicaset, metav1.UpdateOptions{})
	return err
}

func (c *Client) ScaleReplicaSet(namespace, name string, replicas int32) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()

	replicaSet, err := cs.AppsV1().ReplicaSets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get replicaset: %w", err)
	}

	replicaSet.Spec.Replicas = &replicas
	_, err = cs.AppsV1().ReplicaSets(namespace).Update(ctx, replicaSet, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteReplicaSet(contextName, namespace, name string) error {
	log.Printf("Deleting replicaset: context=%s, ns=%s, name=%s", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return cs.AppsV1().ReplicaSets(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

// Job operations
