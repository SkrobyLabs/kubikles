package k8s

import (
	"context"
	"fmt"
	"log"
	"time"

	v1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/yaml"
)

func (c *Client) ListPods(namespace string) ([]v1.Pod, error) {
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return c.ListPodsWithContext(ctx, namespace)
}

// ListPodsWithContext lists pods with cancellation support and pagination.
func (c *Client) ListPodsWithContext(ctx context.Context, namespace string, onProgress ...func(loaded, total int)) ([]v1.Pod, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	var progressFn func(loaded, total int)
	if len(onProgress) > 0 {
		progressFn = onProgress[0]
	}
	result, err := paginatedList(ctx, "pods", defaultPageSize, func(ctx context.Context, opts metav1.ListOptions) ([]v1.Pod, string, *int64, error) {
		list, err := cs.CoreV1().Pods(namespace).List(ctx, opts)
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

// ListPodsForContext lists pods for a specific kubeconfig context
func (c *Client) ListPodsForContext(contextName, namespace string) ([]v1.Pod, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	result, err := paginatedList(ctx, "pods", defaultPageSize, func(ctx context.Context, opts metav1.ListOptions) ([]v1.Pod, string, *int64, error) {
		list, err := cs.CoreV1().Pods(namespace).List(ctx, opts)
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

// ListPodsForNode lists all pods scheduled on a specific node using field selector.
// This is much faster than listing all pods when you only need one node's pods.
func (c *Client) ListPodsForNode(nodeName string) ([]v1.Pod, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	list, err := cs.CoreV1().Pods("").List(ctx, metav1.ListOptions{
		FieldSelector: "spec.nodeName=" + nodeName,
	})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) WatchPods(ctx context.Context, namespace string) (watch.Interface, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	return cs.CoreV1().Pods(namespace).Watch(ctx, metav1.ListOptions{})
}

// WatchTimeout is the timeout for watch connections in seconds.
// Set to 5 minutes to work with most proxy/load balancer timeouts (typically 60s-5min).
// The watch will automatically reconnect when this expires.
const WatchTimeout int64 = 300 // 5 minutes

// WatchResource creates a watch for the specified resource type.
// resourceVersion: if non-empty, resumes watch from this version (avoids duplicate ADDED events)
// Supported resource types: pods, namespaces, nodes, events, deployments, statefulsets,
// daemonsets, replicasets, services, ingresses, ingressclasses, networkpolicies, configmaps, secrets,
// jobs, cronjobs, persistentvolumes, persistentvolumeclaims, storageclasses, hpas, pdbs, resourcequotas, limitranges

func (c *Client) DeletePod(contextName, namespace, name string) error {
	log.Printf("Deleting pod: context=%s, ns=%s, name=%s", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return cs.CoreV1().Pods(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

func (c *Client) ForceDeletePod(contextName, namespace, name string) error {
	log.Printf("Force deleting pod: context=%s, ns=%s, name=%s", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	gracePeriod := int64(0)
	return cs.CoreV1().Pods(namespace).Delete(ctx, name, metav1.DeleteOptions{
		GracePeriodSeconds: &gracePeriod,
	})
}

// CreatePod creates a pod in the given namespace. contextName may be empty to use the current context.
func (c *Client) CreatePod(contextName, namespace string, pod *v1.Pod) (*v1.Pod, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return cs.CoreV1().Pods(namespace).Create(ctx, pod, metav1.CreateOptions{})
}

// WaitForPodRunning polls until the pod reaches Running phase with all containers ready,
// or until the timeout is exceeded.
func (c *Client) WaitForPodRunning(contextName, namespace, name string, timeout time.Duration) error {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		pod, err := cs.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
		cancel()
		if err == nil {
			switch pod.Status.Phase {
			case v1.PodRunning:
				allReady := true
				for _, cs := range pod.Status.ContainerStatuses {
					if !cs.Ready {
						allReady = false
						break
					}
				}
				if allReady {
					return nil
				}
			case v1.PodFailed, v1.PodSucceeded:
				return fmt.Errorf("pod %s/%s ended prematurely with phase %s", namespace, name, pod.Status.Phase)
			}
		}
		time.Sleep(3 * time.Second)
	}
	return fmt.Errorf("timed out waiting for pod %s/%s to become ready", namespace, name)
}

// IsPodRunning checks whether a pod exists and is not in a terminal phase (Succeeded/Failed).
func (c *Client) IsPodRunning(contextName, namespace, name string) bool {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return false
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()

	pod, err := cs.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return false
	}
	return pod.Status.Phase != v1.PodSucceeded && pod.Status.Phase != v1.PodFailed
}

// resolveControllerChain walks the ownership chain to find the top-level controller.
// For example, ReplicaSet→Deployment or Job→CronJob.
func resolveControllerChain(cs kubernetes.Interface, ctx context.Context, namespace, kind, name string) (string, string) {
	switch kind {
	case "ReplicaSet":
		rs, err := cs.AppsV1().ReplicaSets(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			log.Printf("[K8s Client] resolveControllerChain: failed to look up ReplicaSet %s/%s: %v", namespace, name, err)
			return kind, name
		}
		for _, ref := range rs.OwnerReferences {
			if ref.Controller != nil && *ref.Controller && ref.Kind == "Deployment" {
				return "Deployment", ref.Name
			}
		}
	case "Job":
		job, err := cs.BatchV1().Jobs(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			log.Printf("[K8s Client] resolveControllerChain: failed to look up Job %s/%s: %v", namespace, name, err)
			return kind, name
		}
		for _, ref := range job.OwnerReferences {
			if ref.Controller != nil && *ref.Controller && ref.Kind == "CronJob" {
				return "CronJob", ref.Name
			}
		}
	}
	return kind, name
}

// TopLevelOwner represents the resolved top-level controller for a resource.
type TopLevelOwner struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

// ResolveTopLevelOwner resolves the top-level controller for a given owner reference.
func (c *Client) ResolveTopLevelOwner(contextName, namespace, kind, name string) (*TopLevelOwner, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()

	resolvedKind, resolvedName := resolveControllerChain(cs, ctx, namespace, kind, name)
	return &TopLevelOwner{Kind: resolvedKind, Name: resolvedName}, nil
}

// PodEvictionInfo describes a pod's eviction category based on its ownership chain.
type PodEvictionInfo struct {
	Category  string `json:"category"`  // "reschedulable", "killable", "daemon"
	OwnerKind string `json:"ownerKind"` // top-level controller kind
	OwnerName string `json:"ownerName"` // top-level controller name
	PodName   string `json:"podName"`
	Namespace string `json:"namespace"`
}

// GetPodEvictionInfo resolves the ownership chain of a pod and returns its eviction category.
func (c *Client) GetPodEvictionInfo(contextName, namespace, name string) (*PodEvictionInfo, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()

	pod, err := cs.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get pod %s/%s: %w", namespace, name, err)
	}

	info := &PodEvictionInfo{
		PodName:   name,
		Namespace: namespace,
	}

	// Find the controller owner reference
	var controller *metav1.OwnerReference
	for i := range pod.OwnerReferences {
		if pod.OwnerReferences[i].Controller != nil && *pod.OwnerReferences[i].Controller {
			controller = &pod.OwnerReferences[i]
			break
		}
	}

	if controller == nil {
		// Standalone pod
		info.Category = "killable"
		return info, nil
	}

	switch controller.Kind {
	case "DaemonSet":
		info.Category = "daemon"
		info.OwnerKind = "DaemonSet"
		info.OwnerName = controller.Name

	case "Node":
		// Mirror pod
		info.Category = "daemon"
		info.OwnerKind = "Node"
		info.OwnerName = controller.Name

	case "Job":
		info.Category = "killable"
		info.OwnerKind, info.OwnerName = resolveControllerChain(cs, ctx, namespace, controller.Kind, controller.Name)

	case "ReplicaSet":
		info.Category = "reschedulable"
		info.OwnerKind, info.OwnerName = resolveControllerChain(cs, ctx, namespace, controller.Kind, controller.Name)

	case "StatefulSet":
		info.Category = "reschedulable"
		info.OwnerKind = "StatefulSet"
		info.OwnerName = controller.Name

	default:
		info.Category = "killable"
		info.OwnerKind = controller.Kind
		info.OwnerName = controller.Name
	}

	return info, nil
}

// EvictPod evicts a pod using the Kubernetes Eviction API, which respects PDBs.
func (c *Client) EvictPod(contextName, namespace, name string) error {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()

	eviction := &policyv1.Eviction{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
	}
	return cs.CoreV1().Pods(namespace).EvictV1(ctx, eviction)
}

// GetPod fetches a single pod by namespace and name.
func (c *Client) GetPod(namespace, name string) (*v1.Pod, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return cs.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
}

func (c *Client) GetPodYaml(namespace, name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	pod, err := cs.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	// Remove managed fields to make it cleaner for editing
	pod.ManagedFields = nil

	y, err := yaml.Marshal(pod)
	if err != nil {
		return "", err
	}
	return string(y), nil
}

func (c *Client) UpdatePodYaml(namespace, name, content string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()

	// Parse the YAML to a Pod object
	var pod v1.Pod
	if err := yaml.Unmarshal([]byte(content), &pod); err != nil {
		return fmt.Errorf("failed to parse yaml: %w", err)
	}

	// Ensure namespace and name match
	if pod.Namespace != namespace || pod.Name != name {
		return fmt.Errorf("namespace/name mismatch in yaml")
	}

	_, err = cs.CoreV1().Pods(namespace).Update(ctx, &pod, metav1.UpdateOptions{})
	return err
}
