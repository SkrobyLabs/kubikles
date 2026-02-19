package k8s

import (
	"context"
	"fmt"
	"log"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/yaml"
)

func (c *Client) ListDeployments(namespace string) ([]appsv1.Deployment, error) {
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return c.ListDeploymentsWithContext(ctx, namespace)
}

// ListDeploymentsWithContext lists deployments with cancellation support and pagination.
func (c *Client) ListDeploymentsWithContext(ctx context.Context, namespace string, onProgress ...func(loaded, total int)) ([]appsv1.Deployment, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	var progressFn func(loaded, total int)
	if len(onProgress) > 0 {
		progressFn = onProgress[0]
	}
	result, err := paginatedList(ctx, "deployments", defaultPageSize, func(ctx context.Context, opts metav1.ListOptions) ([]appsv1.Deployment, string, *int64, error) {
		list, err := cs.AppsV1().Deployments(namespace).List(ctx, opts)
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

// ListDeploymentsForContext lists deployments for a specific kubeconfig context
func (c *Client) ListDeploymentsForContext(contextName, namespace string) ([]appsv1.Deployment, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	result, err := paginatedList(ctx, "deployments", defaultPageSize, func(ctx context.Context, opts metav1.ListOptions) ([]appsv1.Deployment, string, *int64, error) {
		list, err := cs.AppsV1().Deployments(namespace).List(ctx, opts)
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

func (c *Client) GetDeploymentYaml(namespace, name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	deployment, err := cs.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	// Remove managed fields
	deployment.ManagedFields = nil

	y, err := yaml.Marshal(deployment)
	if err != nil {
		return "", err
	}
	return string(y), nil
}

func (c *Client) UpdateDeploymentYaml(namespace, name, content string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()

	var deployment appsv1.Deployment
	if err := yaml.Unmarshal([]byte(content), &deployment); err != nil {
		return fmt.Errorf("failed to parse yaml: %w", err)
	}

	if deployment.Namespace != namespace || deployment.Name != name {
		return fmt.Errorf("namespace/name mismatch in yaml")
	}

	_, err = cs.AppsV1().Deployments(namespace).Update(ctx, &deployment, metav1.UpdateOptions{})
	return err
}

func (c *Client) ScaleDeployment(namespace, name string, replicas int32) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()

	deployment, err := cs.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get deployment: %w", err)
	}

	deployment.Spec.Replicas = &replicas
	_, err = cs.AppsV1().Deployments(namespace).Update(ctx, deployment, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteDeployment(contextName, namespace, name string) error {
	log.Printf("Deleting deployment: context=%s, ns=%s, name=%s", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return cs.AppsV1().Deployments(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

func (c *Client) RestartDeployment(contextName, namespace, name string) error {
	fmt.Printf("Restarting deployment: context=%s, ns=%s, name=%s\n", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()

	// Patch the deployment to trigger a rollout
	// We update the spec.template.metadata.annotations with a timestamp
	patch := fmt.Sprintf(`{"spec":{"template":{"metadata":{"annotations":{"kubectl.kubernetes.io/restartedAt":"%s"}}}}}`, metav1.Now().String())
	_, err = cs.AppsV1().Deployments(namespace).Patch(ctx, name, types.StrategicMergePatchType, []byte(patch), metav1.PatchOptions{})
	return err
}

// StatefulSet operations
