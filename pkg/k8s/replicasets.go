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
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	replicasets, err := cs.AppsV1().ReplicaSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return replicasets.Items, nil
}

// ListReplicaSetsWithContext lists replicasets with cancellation support
func (c *Client) ListReplicaSetsWithContext(ctx context.Context, contextName, namespace string) ([]appsv1.ReplicaSet, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	replicasets, err := cs.AppsV1().ReplicaSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		if isCancelledError(err) {
			return nil, ErrRequestCancelled
		}
		return nil, err
	}
	return replicasets.Items, nil
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
