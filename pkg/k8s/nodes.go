package k8s

import (
	"context"
	"fmt"
	"log"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/yaml"
)

func (c *Client) ListNodes() ([]v1.Node, error) {
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return c.ListNodesWithContext(ctx)
}

// ListNodesWithContext lists nodes with cancellation support and pagination.
func (c *Client) ListNodesWithContext(ctx context.Context, onProgress ...func(loaded, total int)) ([]v1.Node, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	var progressFn func(loaded, total int)
	if len(onProgress) > 0 {
		progressFn = onProgress[0]
	}
	result, err := paginatedList(ctx, "nodes", defaultPageSize, func(ctx context.Context, opts metav1.ListOptions) ([]v1.Node, string, *int64, error) {
		list, err := cs.CoreV1().Nodes().List(ctx, opts)
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

func (c *Client) GetNodeYaml(name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	node, err := cs.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	// Remove managed fields
	node.ManagedFields = nil

	yamlBytes, err := yaml.Marshal(node)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateNodeYaml(name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	var node v1.Node
	if err := yaml.Unmarshal([]byte(yamlContent), &node); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	if node.Name != name {
		return fmt.Errorf("node name in YAML (%s) does not match expected name (%s)", node.Name, name)
	}
	_, err = cs.CoreV1().Nodes().Update(ctx, &node, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteNode(contextName, name string) error {
	log.Printf("Deleting node: context=%s, name=%s", contextName, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return cs.CoreV1().Nodes().Delete(ctx, name, metav1.DeleteOptions{})
}

func (c *Client) SetNodeSchedulable(contextName, name string, schedulable bool) error {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()

	// Patch spec.unschedulable - true means cordoned (unschedulable), false means uncordoned
	patchData := fmt.Sprintf(`{"spec":{"unschedulable":%t}}`, !schedulable)

	_, err = cs.CoreV1().Nodes().Patch(
		ctx,
		name,
		types.MergePatchType,
		[]byte(patchData),
		metav1.PatchOptions{},
	)
	if err != nil {
		return fmt.Errorf("failed to patch node: %w", err)
	}
	return nil
}

func (c *Client) CreateNodeDebugPod(contextName, nodeName, image string) (*v1.Pod, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()

	// Default to alpine:latest if no image specified
	if image == "" {
		image = "alpine:latest"
	}

	privileged := true
	debugPod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("node-shell-%s-", nodeName),
			Namespace:    "default",
		},
		Spec: v1.PodSpec{
			NodeName:      nodeName,
			HostPID:       true,
			HostNetwork:   true,
			HostIPC:       true,
			RestartPolicy: v1.RestartPolicyNever,
			Containers: []v1.Container{
				{
					Name:  "shell",
					Image: image,
					Command: []string{
						"sleep", "infinity",
					},
					Stdin: true,
					TTY:   true,
					SecurityContext: &v1.SecurityContext{
						Privileged: &privileged,
					},
				},
			},
		},
	}

	return cs.CoreV1().Pods("default").Create(ctx, debugPod, metav1.CreateOptions{})
}
