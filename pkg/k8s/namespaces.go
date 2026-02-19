package k8s

import (
	"context"
	"fmt"
	"log"
	"sync"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

func (c *Client) ListNamespaces() ([]v1.Namespace, error) {
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return c.ListNamespacesWithContext(ctx)
}

// ListNamespacesWithContext lists namespaces with cancellation support and pagination.
func (c *Client) ListNamespacesWithContext(ctx context.Context, onProgress ...func(loaded, total int)) ([]v1.Namespace, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	var progressFn func(loaded, total int)
	if len(onProgress) > 0 {
		progressFn = onProgress[0]
	}
	result, err := paginatedList(ctx, "namespaces", defaultPageSize, func(ctx context.Context, opts metav1.ListOptions) ([]v1.Namespace, string, *int64, error) {
		list, err := cs.CoreV1().Namespaces().List(ctx, opts)
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

// ListNamespacesForContext lists namespaces for a specific kubeconfig context
func (c *Client) ListNamespacesForContext(contextName string) ([]v1.Namespace, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	result, err := paginatedList(ctx, "namespaces", defaultPageSize, func(ctx context.Context, opts metav1.ListOptions) ([]v1.Namespace, string, *int64, error) {
		list, err := cs.CoreV1().Namespaces().List(ctx, opts)
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

// NamespaceResourceCounts holds the count of various resource types in a namespace
type NamespaceResourceCounts struct {
	Pods         int `json:"pods"`
	Deployments  int `json:"deployments"`
	StatefulSets int `json:"statefulsets"`
	DaemonSets   int `json:"daemonsets"`
	ReplicaSets  int `json:"replicasets"`
	Jobs         int `json:"jobs"`
	CronJobs     int `json:"cronjobs"`
	Services     int `json:"services"`
	Ingresses    int `json:"ingresses"`
	ConfigMaps   int `json:"configmaps"`
	Secrets      int `json:"secrets"`
	PVCs         int `json:"pvcs"`
}

// GetNamespaceResourceCounts returns counts of various resource types in a namespace
func (c *Client) GetNamespaceResourceCounts(namespace string) (*NamespaceResourceCounts, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}

	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	counts := &NamespaceResourceCounts{}

	// Use goroutines for parallel counting
	var wg sync.WaitGroup
	var mu sync.Mutex
	errChan := make(chan error, 12)

	// Pods
	wg.Add(1)
	go func() {
		defer wg.Done()
		list, err := cs.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			errChan <- err
			return
		}
		mu.Lock()
		counts.Pods = len(list.Items)
		mu.Unlock()
	}()

	// Deployments
	wg.Add(1)
	go func() {
		defer wg.Done()
		list, err := cs.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			errChan <- err
			return
		}
		mu.Lock()
		counts.Deployments = len(list.Items)
		mu.Unlock()
	}()

	// StatefulSets
	wg.Add(1)
	go func() {
		defer wg.Done()
		list, err := cs.AppsV1().StatefulSets(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			errChan <- err
			return
		}
		mu.Lock()
		counts.StatefulSets = len(list.Items)
		mu.Unlock()
	}()

	// DaemonSets
	wg.Add(1)
	go func() {
		defer wg.Done()
		list, err := cs.AppsV1().DaemonSets(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			errChan <- err
			return
		}
		mu.Lock()
		counts.DaemonSets = len(list.Items)
		mu.Unlock()
	}()

	// ReplicaSets
	wg.Add(1)
	go func() {
		defer wg.Done()
		list, err := cs.AppsV1().ReplicaSets(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			errChan <- err
			return
		}
		mu.Lock()
		counts.ReplicaSets = len(list.Items)
		mu.Unlock()
	}()

	// Jobs
	wg.Add(1)
	go func() {
		defer wg.Done()
		list, err := cs.BatchV1().Jobs(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			errChan <- err
			return
		}
		mu.Lock()
		counts.Jobs = len(list.Items)
		mu.Unlock()
	}()

	// CronJobs
	wg.Add(1)
	go func() {
		defer wg.Done()
		list, err := cs.BatchV1().CronJobs(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			errChan <- err
			return
		}
		mu.Lock()
		counts.CronJobs = len(list.Items)
		mu.Unlock()
	}()

	// Services
	wg.Add(1)
	go func() {
		defer wg.Done()
		list, err := cs.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			errChan <- err
			return
		}
		mu.Lock()
		counts.Services = len(list.Items)
		mu.Unlock()
	}()

	// Ingresses
	wg.Add(1)
	go func() {
		defer wg.Done()
		list, err := cs.NetworkingV1().Ingresses(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			errChan <- err
			return
		}
		mu.Lock()
		counts.Ingresses = len(list.Items)
		mu.Unlock()
	}()

	// ConfigMaps
	wg.Add(1)
	go func() {
		defer wg.Done()
		list, err := cs.CoreV1().ConfigMaps(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			errChan <- err
			return
		}
		mu.Lock()
		counts.ConfigMaps = len(list.Items)
		mu.Unlock()
	}()

	// Secrets
	wg.Add(1)
	go func() {
		defer wg.Done()
		list, err := cs.CoreV1().Secrets(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			errChan <- err
			return
		}
		mu.Lock()
		counts.Secrets = len(list.Items)
		mu.Unlock()
	}()

	// PVCs
	wg.Add(1)
	go func() {
		defer wg.Done()
		list, err := cs.CoreV1().PersistentVolumeClaims(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			errChan <- err
			return
		}
		mu.Lock()
		counts.PVCs = len(list.Items)
		mu.Unlock()
	}()

	wg.Wait()
	close(errChan)

	// Check for errors (return first error if any)
	for err := range errChan {
		if err != nil {
			return nil, err
		}
	}

	return counts, nil
}

func (c *Client) DeleteNamespace(contextName, name string) error {
	log.Printf("Deleting namespace: context=%s, name=%s", contextName, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return cs.CoreV1().Namespaces().Delete(ctx, name, metav1.DeleteOptions{})
}

func (c *Client) GetNamespaceYAML(name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()

	ns, err := cs.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	// Clean up fields that shouldn't be in the YAML
	ns.ManagedFields = nil

	yamlBytes, err := yaml.Marshal(ns)
	if err != nil {
		return "", fmt.Errorf("failed to marshal namespace to YAML: %w", err)
	}

	return string(yamlBytes), nil
}

func (c *Client) UpdateNamespaceYAML(name string, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()

	// Parse the YAML to a Namespace object
	var ns v1.Namespace
	if err := yaml.Unmarshal([]byte(yamlContent), &ns); err != nil {
		return fmt.Errorf("failed to parse yaml: %w", err)
	}

	// Ensure the name matches
	if ns.Name != name {
		return fmt.Errorf("namespace name in YAML (%s) does not match expected name (%s)", ns.Name, name)
	}

	_, err = cs.CoreV1().Namespaces().Update(ctx, &ns, metav1.UpdateOptions{})
	return err
}
