package k8s

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	v1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	schedulingv1 "k8s.io/api/scheduling/v1"
	storagev1 "k8s.io/api/storage/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsclientset "k8s.io/metrics/pkg/client/clientset/versioned"
	"sigs.k8s.io/yaml"
)

// ErrRequestCancelled is returned when a request was canceled
var ErrRequestCancelled = errors.New("request canceled")

// isCancelledError checks if an error is a context cancellation or deadline exceeded
func isCancelledError(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

// ptr returns a pointer to the given value. Used for optional fields in K8s API structs.
func ptr[T any](v T) *T {
	return &v
}

type Client struct {
	clientset      kubernetes.Interface
	metricsClient  metricsclientset.Interface
	configLoading  clientcmd.ClientConfig
	currentContext string
	mu             sync.RWMutex
	apiTimeout     time.Duration
	warmupDone     chan struct{} // Closed when connection warmup completes

	// HTTP protocol settings
	// When true, forces HTTP/1.1 instead of HTTP/2. HTTP/1.1 opens multiple TCP
	// connections for parallel requests, avoiding HTTP/2 flow control bottlenecks.
	forceHTTP1 bool

	// Client pool for rotating connections. Each clientset has its own HTTP/2
	// connection, so rotating between them provides better parallelism.
	clientPool     []kubernetes.Interface
	clientPoolSize int
	clientPoolIdx  uint64 // atomic counter for round-robin
}

// DefaultAPITimeout is the default timeout for Kubernetes API calls
const DefaultAPITimeout = 60 * time.Second

// SetAPITimeout sets the timeout for Kubernetes API calls
func (c *Client) SetAPITimeout(timeout time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.apiTimeout = timeout
}

// SetForceHTTP1 enables or disables forcing HTTP/1.1 instead of HTTP/2.
// HTTP/1.1 opens multiple connections for parallel requests, avoiding
// HTTP/2 single-connection bottlenecks. Requires reconnect to take effect.
func (c *Client) SetForceHTTP1(enabled bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.forceHTTP1 = enabled
	log.Printf("[K8s Client] Force HTTP/1.1: %v", enabled)
}

// SetClientPoolSize sets the number of clientsets in the rotation pool.
// More clients = more parallel HTTP/2 connections. Set to 0 to disable pooling.
// Requires reconnect to take effect.
func (c *Client) SetClientPoolSize(size int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.clientPoolSize = size
	log.Printf("[K8s Client] Client pool size: %d", size)
}

// contextWithTimeout returns a context with the configured API timeout
func (c *Client) contextWithTimeout() (context.Context, context.CancelFunc) {
	c.mu.RLock()
	timeout := c.apiTimeout
	c.mu.RUnlock()
	if timeout == 0 {
		timeout = DefaultAPITimeout
	}
	return context.WithTimeout(context.Background(), timeout)
}

// NodeMetrics represents CPU/Memory usage for a node
type NodeMetrics struct {
	Name         string `json:"name"`
	CPUUsage     int64  `json:"cpuUsage"`     // millicores
	MemoryUsage  int64  `json:"memoryUsage"`  // bytes
	CPUCapacity  int64  `json:"cpuCapacity"`  // millicores
	MemCapacity  int64  `json:"memCapacity"`  // bytes
	CPURequested int64  `json:"cpuRequested"` // millicores (sum of pod requests)
	MemRequested int64  `json:"memRequested"` // bytes (sum of pod requests)
	CPUCommitted int64  `json:"cpuCommitted"` // millicores (sum of max(usage, request) per container)
	MemCommitted int64  `json:"memCommitted"` // bytes (sum of max(usage, request) per container)
	PodCount     int64  `json:"podCount"`     // number of running pods on node
	PodCapacity  int64  `json:"podCapacity"`  // max pods allowed on node
}

// NodeMetricsResult wraps the metrics response with availability status
type NodeMetricsResult struct {
	Available bool          `json:"available"`
	Metrics   []NodeMetrics `json:"metrics"`
	Error     string        `json:"error,omitempty"`
}

// PodMetrics represents CPU/Memory usage for a pod relative to its node
type PodMetrics struct {
	Namespace       string `json:"namespace"`
	Name            string `json:"name"`
	NodeName        string `json:"nodeName"`
	CPUUsage        int64  `json:"cpuUsage"`        // millicores
	MemoryUsage     int64  `json:"memoryUsage"`     // bytes
	CPURequested    int64  `json:"cpuRequested"`    // millicores
	MemRequested    int64  `json:"memRequested"`    // bytes
	CPUCommitted    int64  `json:"cpuCommitted"`    // millicores (max of usage, request)
	MemCommitted    int64  `json:"memCommitted"`    // bytes (max of usage, request)
	NodeCPUCapacity int64  `json:"nodeCpuCapacity"` // millicores
	NodeMemCapacity int64  `json:"nodeMemCapacity"` // bytes
}

// PodMetricsResult wraps the pod metrics response
type PodMetricsResult struct {
	Available bool         `json:"available"`
	Metrics   []PodMetrics `json:"metrics"`
	Error     string       `json:"error,omitempty"`
}

func NewClient() (*Client, error) {
	c := &Client{}
	if err := c.loadConfig(""); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Client) loadConfig(contextName string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	home := homedir.HomeDir()
	kubeconfigPath := filepath.Join(home, ".kube", "config")

	loadingRules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath}
	configOverrides := &clientcmd.ConfigOverrides{}

	if contextName != "" {
		configOverrides.CurrentContext = contextName
	}

	c.configLoading = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	config, err := c.configLoading.ClientConfig()
	if err != nil {
		return fmt.Errorf("failed to load client config: %w", err)
	}

	// Apply HTTP protocol settings
	if c.forceHTTP1 {
		// Force HTTP/1.1 by disabling HTTP/2 ALPN negotiation
		if config.TLSClientConfig.NextProtos == nil {
			config.TLSClientConfig.NextProtos = []string{"http/1.1"}
		}
		// Also need to disable HTTP/2 at the transport level
		config.WrapTransport = func(rt http.RoundTripper) http.RoundTripper {
			if t, ok := rt.(*http.Transport); ok {
				t.TLSNextProto = make(map[string]func(authority string, c *tls.Conn) http.RoundTripper)
				t.ForceAttemptHTTP2 = false
			}
			return rt
		}
		log.Printf("[K8s Client] Using HTTP/1.1 (HTTP/2 disabled)")
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create clientset: %w", err)
	}

	c.clientset = clientset
	c.metricsClient = nil // Reset metrics client so it's recreated for the new context

	// Create additional client connections for rotation (improves parallelism)
	c.clientPool = nil
	if c.clientPoolSize > 0 {
		c.clientPool = make([]kubernetes.Interface, c.clientPoolSize)
		for i := 0; i < c.clientPoolSize; i++ {
			// Create a fresh config for each pool member to ensure separate connections
			poolConfig, err := c.configLoading.ClientConfig()
			if err != nil {
				return fmt.Errorf("failed to load pool client config: %w", err)
			}
			// Apply same HTTP settings
			if c.forceHTTP1 {
				if poolConfig.TLSClientConfig.NextProtos == nil {
					poolConfig.TLSClientConfig.NextProtos = []string{"http/1.1"}
				}
				poolConfig.WrapTransport = func(rt http.RoundTripper) http.RoundTripper {
					if t, ok := rt.(*http.Transport); ok {
						t.TLSNextProto = make(map[string]func(authority string, c *tls.Conn) http.RoundTripper)
						t.ForceAttemptHTTP2 = false
					}
					return rt
				}
			}
			poolCs, err := kubernetes.NewForConfig(poolConfig)
			if err != nil {
				return fmt.Errorf("failed to create pool clientset %d: %w", i, err)
			}
			c.clientPool[i] = poolCs
		}
		log.Printf("[K8s Client] Created %d additional client connections (%d total)", c.clientPoolSize, c.clientPoolSize+1)
	}

	// Update current context
	rawConfig, err := c.configLoading.RawConfig()
	if err == nil {
		if contextName != "" {
			c.currentContext = contextName
		} else {
			c.currentContext = rawConfig.CurrentContext
		}
	}

	return nil
}

func (c *Client) SwitchContext(contextName string) error {
	if err := c.loadConfig(contextName); err != nil {
		return err
	}

	// Create a new warmup channel
	c.mu.Lock()
	c.warmupDone = make(chan struct{})
	warmupChan := c.warmupDone
	c.mu.Unlock()

	// Warm up the connection by making a lightweight API call in the background.
	// This triggers TLS handshake, auth token fetch, and connection pooling
	// so subsequent requests are fast.
	go func() {
		defer close(warmupChan)
		start := time.Now()
		cs, err := c.getClientset()
		if err != nil {
			log.Printf("[Warmup] Failed to get clientset: %v", err)
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		// RESTClient().Get() is a lightweight call that warms up the connection
		err = cs.CoreV1().RESTClient().Get().AbsPath("/api/v1").Do(ctx).Error()
		log.Printf("[Warmup] Connection warmup took %v, err=%v", time.Since(start), err)
	}()

	return nil
}

// WaitForWarmup waits for connection warmup to complete (max 15 seconds)
func (c *Client) WaitForWarmup() {
	c.mu.RLock()
	warmupChan := c.warmupDone
	c.mu.RUnlock()

	if warmupChan == nil {
		return
	}

	select {
	case <-warmupChan:
		// Warmup completed
	case <-time.After(15 * time.Second):
		// Timeout waiting for warmup
		log.Printf("[Warmup] Timeout waiting for warmup to complete")
	}
}

func (c *Client) GetCurrentContext() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.currentContext
}

// TestConnection performs a quick connectivity check to the cluster.
// Returns nil if the cluster is reachable, or an error describing the failure.
// Use a short timeout (e.g., 5 seconds) for fast feedback on unreachable clusters.
func (c *Client) TestConnection(ctx context.Context) error {
	cs, err := c.getClientset()
	if err != nil {
		return fmt.Errorf("failed to get clientset: %w", err)
	}

	// Use RESTClient with context for proper timeout control
	// Hits /api which is lightweight and respects the context deadline
	err = cs.CoreV1().RESTClient().Get().AbsPath("/api").Do(ctx).Error()
	if err != nil {
		return fmt.Errorf("cluster unreachable: %w", err)
	}

	return nil
}

func (c *Client) ListContexts() ([]string, error) {
	rawConfig, err := c.configLoading.RawConfig()
	if err != nil {
		return nil, err
	}
	contexts := make([]string, 0, len(rawConfig.Contexts))
	for name := range rawConfig.Contexts {
		contexts = append(contexts, name)
	}
	return contexts, nil
}

// --- Resources ---

func (c *Client) getClientset() (kubernetes.Interface, error) {
	c.mu.RLock()
	pool := c.clientPool
	mainCs := c.clientset
	c.mu.RUnlock()

	if mainCs == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}

	// If pool is configured, rotate among main client + pool clients
	if len(pool) > 0 {
		idx := atomic.AddUint64(&c.clientPoolIdx, 1)
		totalClients := uint64(len(pool) + 1) //nolint:gosec // pool size is small, safe conversion
		selected := idx % totalClients
		if selected == 0 {
			return mainCs, nil
		}
		return pool[selected-1], nil
	}

	return mainCs, nil
}

func (c *Client) ListPods(namespace string) ([]v1.Pod, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	pods, err := cs.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return pods.Items, nil
}

// ListPodsWithContext lists pods with cancellation support
func (c *Client) ListPodsWithContext(ctx context.Context, namespace string) ([]v1.Pod, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	pods, err := cs.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		if isCancelledError(err) {
			return nil, ErrRequestCancelled
		}
		return nil, err
	}
	return pods.Items, nil
}

// ListPodsForContext lists pods for a specific kubeconfig context
func (c *Client) ListPodsForContext(contextName, namespace string) ([]v1.Pod, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	pods, err := cs.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return pods.Items, nil
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
func (c *Client) WatchResource(ctx context.Context, resourceType, namespace, resourceVersion string) (watch.Interface, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}

	opts := metav1.ListOptions{
		TimeoutSeconds:      ptr(WatchTimeout),
		AllowWatchBookmarks: true,
		ResourceVersion:     resourceVersion,
	}

	switch resourceType {
	// Core API (v1)
	case "pods":
		return cs.CoreV1().Pods(namespace).Watch(ctx, opts)
	case "namespaces":
		return cs.CoreV1().Namespaces().Watch(ctx, opts)
	case "nodes":
		return cs.CoreV1().Nodes().Watch(ctx, opts)
	case "events":
		return cs.CoreV1().Events(namespace).Watch(ctx, opts)
	case "services":
		return cs.CoreV1().Services(namespace).Watch(ctx, opts)
	case "configmaps":
		return cs.CoreV1().ConfigMaps(namespace).Watch(ctx, opts)
	case "secrets":
		return cs.CoreV1().Secrets(namespace).Watch(ctx, opts)
	case "persistentvolumes":
		return cs.CoreV1().PersistentVolumes().Watch(ctx, opts)
	case "persistentvolumeclaims":
		return cs.CoreV1().PersistentVolumeClaims(namespace).Watch(ctx, opts)

	// Apps API (v1)
	case "deployments":
		return cs.AppsV1().Deployments(namespace).Watch(ctx, opts)
	case "statefulsets":
		return cs.AppsV1().StatefulSets(namespace).Watch(ctx, opts)
	case "daemonsets":
		return cs.AppsV1().DaemonSets(namespace).Watch(ctx, opts)
	case "replicasets":
		return cs.AppsV1().ReplicaSets(namespace).Watch(ctx, opts)

	// Batch API (v1)
	case "jobs":
		return cs.BatchV1().Jobs(namespace).Watch(ctx, opts)
	case "cronjobs":
		return cs.BatchV1().CronJobs(namespace).Watch(ctx, opts)

	// Networking API (v1)
	case "ingresses":
		return cs.NetworkingV1().Ingresses(namespace).Watch(ctx, opts)
	case "ingressclasses":
		return cs.NetworkingV1().IngressClasses().Watch(ctx, opts)
	case "networkpolicies":
		return cs.NetworkingV1().NetworkPolicies(namespace).Watch(ctx, opts)

	// Storage API (v1)
	case "storageclasses":
		return cs.StorageV1().StorageClasses().Watch(ctx, opts)
	case "csidrivers":
		return cs.StorageV1().CSIDrivers().Watch(ctx, opts)
	case "csinodes":
		return cs.StorageV1().CSINodes().Watch(ctx, opts)

	// Autoscaling API (v2)
	case "hpas":
		return cs.AutoscalingV2().HorizontalPodAutoscalers(namespace).Watch(ctx, opts)

	// Policy API (v1)
	case "pdbs":
		return cs.PolicyV1().PodDisruptionBudgets(namespace).Watch(ctx, opts)

	// Core API (v1) - additional resources
	case "resourcequotas":
		return cs.CoreV1().ResourceQuotas(namespace).Watch(ctx, opts)
	case "limitranges":
		return cs.CoreV1().LimitRanges(namespace).Watch(ctx, opts)
	case "endpoints":
		return cs.CoreV1().Endpoints(namespace).Watch(ctx, opts)

	// RBAC API (v1) - service accounts already in core
	case "serviceaccounts":
		return cs.CoreV1().ServiceAccounts(namespace).Watch(ctx, opts)
	case "roles":
		return cs.RbacV1().Roles(namespace).Watch(ctx, opts)
	case "clusterroles":
		return cs.RbacV1().ClusterRoles().Watch(ctx, opts)
	case "rolebindings":
		return cs.RbacV1().RoleBindings(namespace).Watch(ctx, opts)
	case "clusterrolebindings":
		return cs.RbacV1().ClusterRoleBindings().Watch(ctx, opts)

	// Admission Registration API (v1)
	case "validatingwebhookconfigurations":
		return cs.AdmissionregistrationV1().ValidatingWebhookConfigurations().Watch(ctx, opts)
	case "mutatingwebhookconfigurations":
		return cs.AdmissionregistrationV1().MutatingWebhookConfigurations().Watch(ctx, opts)

	// Scheduling API (v1)
	case "priorityclasses":
		return cs.SchedulingV1().PriorityClasses().Watch(ctx, opts)

	// Coordination API (v1)
	case "leases":
		return cs.CoordinationV1().Leases(namespace).Watch(ctx, opts)

	default:
		return nil, fmt.Errorf("unsupported resource type: %s", resourceType)
	}
}

// WatchCRD creates a watch for a custom resource using the dynamic client.
// resourceVersion: if non-empty, resumes watch from this version (avoids duplicate ADDED events)
func (c *Client) WatchCRD(ctx context.Context, group, version, resource, namespace, resourceVersion string) (watch.Interface, error) {
	dc, err := c.getDynamicClientForContext("")
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}

	gvr := schema.GroupVersionResource{
		Group:    group,
		Version:  version,
		Resource: resource,
	}

	opts := metav1.ListOptions{
		TimeoutSeconds:      ptr(WatchTimeout),
		AllowWatchBookmarks: true,
		ResourceVersion:     resourceVersion,
	}

	if namespace != "" {
		return dc.Resource(gvr).Namespace(namespace).Watch(ctx, opts)
	}
	return dc.Resource(gvr).Watch(ctx, opts)
}

// RuntimeObjectToMap converts a runtime.Object to a map[string]interface{}
// This is used for generic resource event handling
func RuntimeObjectToMap(obj interface{}) (map[string]interface{}, error) {
	// If it's already an unstructured object, get the map directly
	if u, ok := obj.(*unstructured.Unstructured); ok {
		return u.Object, nil
	}

	// For typed objects, we need to convert to JSON then to map
	data, err := yaml.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal object: %w", err)
	}

	var result map[string]interface{}
	if err := yaml.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal to map: %w", err)
	}

	return result, nil
}

func (c *Client) ListNodes() ([]v1.Node, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	nodes, err := cs.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return nodes.Items, nil
}

// ListNodesWithContext lists nodes with cancellation support
func (c *Client) ListNodesWithContext(ctx context.Context) ([]v1.Node, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	nodes, err := cs.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		if isCancelledError(err) {
			return nil, ErrRequestCancelled
		}
		return nil, err
	}
	return nodes.Items, nil
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

// GetNodeMetrics fetches CPU and Memory metrics for all nodes from the metrics-server
func (c *Client) GetNodeMetrics() (*NodeMetricsResult, error) {
	c.mu.Lock()
	// Lazy init metrics client
	if c.metricsClient == nil {
		config, err := c.configLoading.ClientConfig()
		if err != nil {
			c.mu.Unlock()
			return &NodeMetricsResult{Available: false, Error: err.Error()}, nil
		}
		mc, err := metricsclientset.NewForConfig(config)
		if err != nil {
			c.mu.Unlock()
			return &NodeMetricsResult{Available: false, Error: err.Error()}, nil
		}
		c.metricsClient = mc
	}
	c.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get clientset early so we can use it in goroutines
	cs, err := c.getClientset()
	if err != nil {
		return &NodeMetricsResult{Available: false, Error: err.Error()}, nil
	}

	// Fetch node metrics, nodes, pods, and pod metrics in parallel
	var nodeMetricsList *metricsv1beta1.NodeMetricsList
	var nodes *v1.NodeList
	var pods *v1.PodList
	var podMetricsList *metricsv1beta1.PodMetricsList
	var podMetricsErr error

	g, gctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		var err error
		nodeMetricsList, err = c.metricsClient.MetricsV1beta1().NodeMetricses().List(gctx, metav1.ListOptions{})
		return err
	})

	g.Go(func() error {
		var err error
		nodes, err = cs.CoreV1().Nodes().List(gctx, metav1.ListOptions{})
		return err
	})

	g.Go(func() error {
		var err error
		pods, err = cs.CoreV1().Pods("").List(gctx, metav1.ListOptions{})
		return err
	})

	g.Go(func() error {
		var err error
		podMetricsList, err = c.metricsClient.MetricsV1beta1().PodMetricses("").List(gctx, metav1.ListOptions{})
		// Pod metrics might fail, but we can still return node metrics without committed
		podMetricsErr = err
		return nil // Don't fail the group for pod metrics
	})

	if err := g.Wait(); err != nil {
		return &NodeMetricsResult{Available: false, Error: err.Error()}, nil
	}

	// Check if metrics-server returned any data
	// The API might exist but return empty results if metrics-server isn't functioning
	if nodeMetricsList == nil || len(nodeMetricsList.Items) == 0 {
		return &NodeMetricsResult{Available: false, Error: "metrics-server returned no data"}, nil
	}

	// Clear podMetricsList if it failed
	if podMetricsErr != nil {
		podMetricsList = nil
	}

	// Build capacity map using Allocatable (resources available to workloads after system reserved)
	capacityMap := make(map[string]struct{ cpu, memory, pods int64 })
	for _, node := range nodes.Items {
		cpu := node.Status.Allocatable.Cpu().MilliValue() // millicores
		mem := node.Status.Allocatable.Memory().Value()   // bytes
		podCap := node.Status.Allocatable.Pods().Value()  // max pods
		capacityMap[node.Name] = struct{ cpu, memory, pods int64 }{cpu, mem, podCap}
	}

	// Build container usage map: namespace/podName/containerName -> {cpu, memory}
	containerUsageMap := make(map[string]struct{ cpu, memory int64 })
	if podMetricsList != nil {
		for _, pm := range podMetricsList.Items {
			for _, cm := range pm.Containers {
				key := pm.Namespace + "/" + pm.Name + "/" + cm.Name
				containerUsageMap[key] = struct{ cpu, memory int64 }{
					cpu:    cm.Usage.Cpu().MilliValue(),
					memory: cm.Usage.Memory().Value(),
				}
			}
		}
	}

	// Sum resource requests and committed per node
	type nodeResources struct {
		requestedCPU, requestedMem int64
		committedCPU, committedMem int64
		podCount                   int64
	}
	resourcesMap := make(map[string]*nodeResources)

	for _, pod := range pods.Items {
		// Skip pods not scheduled or in terminal states
		if pod.Spec.NodeName == "" || pod.Status.Phase == v1.PodSucceeded || pod.Status.Phase == v1.PodFailed {
			continue
		}
		nodeName := pod.Spec.NodeName
		if resourcesMap[nodeName] == nil {
			resourcesMap[nodeName] = &nodeResources{}
		}
		res := resourcesMap[nodeName]
		res.podCount++

		for _, container := range pod.Spec.Containers {
			var reqCPU, reqMem int64
			if container.Resources.Requests != nil {
				reqCPU = container.Resources.Requests.Cpu().MilliValue()
				reqMem = container.Resources.Requests.Memory().Value()
			}
			res.requestedCPU += reqCPU
			res.requestedMem += reqMem

			// Calculate committed: max(usage, request)
			key := pod.Namespace + "/" + pod.Name + "/" + container.Name
			if usage, ok := containerUsageMap[key]; ok {
				if usage.cpu > reqCPU {
					res.committedCPU += usage.cpu
				} else {
					res.committedCPU += reqCPU
				}
				if usage.memory > reqMem {
					res.committedMem += usage.memory
				} else {
					res.committedMem += reqMem
				}
			} else {
				// No usage data, use request as committed
				res.committedCPU += reqCPU
				res.committedMem += reqMem
			}
		}
	}

	// Combine metrics with capacity, requests, and committed
	result := make([]NodeMetrics, 0, len(nodeMetricsList.Items))
	for _, nm := range nodeMetricsList.Items {
		cap := capacityMap[nm.Name]
		res := resourcesMap[nm.Name]
		var reqCPU, reqMem, comCPU, comMem, podCount int64
		if res != nil {
			reqCPU = res.requestedCPU
			reqMem = res.requestedMem
			comCPU = res.committedCPU
			comMem = res.committedMem
			podCount = res.podCount
		}

		// Get node-level usage from metrics-server
		cpuUsage := nm.Usage.Cpu().MilliValue()
		memUsage := nm.Usage.Memory().Value()

		// Ensure committed >= usage at node level
		// (container-level committed may be lower than node usage due to system processes)
		if cpuUsage > comCPU {
			comCPU = cpuUsage
		}
		if memUsage > comMem {
			comMem = memUsage
		}

		result = append(result, NodeMetrics{
			Name:         nm.Name,
			CPUUsage:     cpuUsage, // millicores
			MemoryUsage:  memUsage, // bytes
			CPUCapacity:  cap.cpu,
			MemCapacity:  cap.memory,
			CPURequested: reqCPU,
			MemRequested: reqMem,
			CPUCommitted: comCPU,
			MemCommitted: comMem,
			PodCount:     podCount,
			PodCapacity:  cap.pods,
		})
	}

	return &NodeMetricsResult{Available: true, Metrics: result}, nil
}

// GetNodeMetricsFromPrometheus fetches node metrics from Prometheus as a fallback when metrics-server is unavailable
func (c *Client) GetNodeMetricsFromPrometheus(contextName string, info *PrometheusInfo) (*NodeMetricsResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cs, err := c.getClientsetForContext(contextName)
	if err != nil {
		return &NodeMetricsResult{Available: false, Error: err.Error()}, nil
	}

	// Get nodes for capacity info
	nodes, err := cs.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return &NodeMetricsResult{Available: false, Error: err.Error()}, nil
	}

	// Build capacity map from node specs
	type nodeCapacity struct {
		cpu    int64
		memory int64
		pods   int64
	}
	capacityMap := make(map[string]nodeCapacity)
	for _, node := range nodes.Items {
		capacityMap[node.Name] = nodeCapacity{
			cpu:    node.Status.Allocatable.Cpu().MilliValue(),
			memory: node.Status.Allocatable.Memory().Value(),
			pods:   node.Status.Allocatable.Pods().Value(),
		}
	}

	// Query Prometheus for current metrics - run all queries in parallel
	// Try node_exporter metrics first (node-level, matches metrics-server), fall back to container metrics
	var nodeExporterCpuResult, nodeExporterMemResult *PrometheusQueryResult
	var containerCpuResult, containerMemResult *PrometheusQueryResult
	var cpuRequestsResult, memRequestsResult, podCountResult *PrometheusQueryResult

	g, gctx := errgroup.WithContext(ctx)

	// Node-exporter CPU Usage (preferred - matches metrics-server)
	g.Go(func() error {
		nodeExporterCpuResult, _ = c.QueryPrometheusWithContext(gctx, contextName, info,
			`sum by (nodename) ((1 - rate(node_cpu_seconds_total{mode="idle"}[5m])) * on(instance) group_left(nodename) node_uname_info)`)
		return nil
	})

	// Node-exporter Memory Usage (preferred - matches metrics-server)
	g.Go(func() error {
		nodeExporterMemResult, _ = c.QueryPrometheusWithContext(gctx, contextName, info,
			`sum by (nodename) ((node_memory_MemTotal_bytes - node_memory_MemAvailable_bytes) * on(instance) group_left(nodename) node_uname_info)`)
		return nil
	})

	// Container CPU Usage (fallback)
	g.Go(func() error {
		containerCpuResult, _ = c.QueryPrometheusWithContext(gctx, contextName, info,
			`sum by (node) (rate(container_cpu_usage_seconds_total{container!="", container!="POD"}[5m]))`)
		return nil
	})

	// Container Memory Usage (fallback)
	g.Go(func() error {
		containerMemResult, _ = c.QueryPrometheusWithContext(gctx, contextName, info,
			`sum by (node) (container_memory_working_set_bytes{container!="", container!="POD"})`)
		return nil
	})

	// CPU Requests (optional)
	g.Go(func() error {
		cpuRequestsResult, _ = c.QueryPrometheusWithContext(gctx, contextName, info,
			`sum by (node) (kube_pod_container_resource_requests{resource="cpu", unit="core"})`)
		return nil
	})

	// Memory Requests (optional)
	g.Go(func() error {
		memRequestsResult, _ = c.QueryPrometheusWithContext(gctx, contextName, info,
			`sum by (node) (kube_pod_container_resource_requests{resource="memory", unit="byte"})`)
		return nil
	})

	// Pod count (optional) - exclude terminal states (Succeeded/Failed) to match K8s metrics behavior
	g.Go(func() error {
		podCountResult, _ = c.QueryPrometheusWithContext(gctx, contextName, info,
			`count by (node) (kube_pod_info{node!=""} unless on(namespace, pod) kube_pod_status_phase{phase=~"Succeeded|Failed"} == 1)`)
		return nil
	})

	_ = g.Wait() // All goroutines return nil, wait just for synchronization

	// Helper to extract values from Prometheus result, checking for both "node" and "nodename" labels
	extractValues := func(result *PrometheusQueryResult, labelKey string) map[string]float64 {
		values := make(map[string]float64)
		if result == nil || result.Data.Result == nil {
			return values
		}
		for _, r := range result.Data.Result {
			node, ok := r.Metric[labelKey]
			if !ok {
				continue
			}
			if len(r.Value) >= 2 {
				if valStr, ok := r.Value[1].(string); ok {
					if val, err := strconv.ParseFloat(valStr, 64); err == nil {
						values[node] = val
					}
				}
			}
		}
		return values
	}

	// Prefer node_exporter metrics (node-level), fall back to container metrics
	cpuUsage := extractValues(nodeExporterCpuResult, "nodename")
	if len(cpuUsage) == 0 {
		cpuUsage = extractValues(containerCpuResult, "node")
	}
	memUsage := extractValues(nodeExporterMemResult, "nodename")
	if len(memUsage) == 0 {
		memUsage = extractValues(containerMemResult, "node")
	}

	// Check if we have any usage data
	if len(cpuUsage) == 0 && len(memUsage) == 0 {
		return &NodeMetricsResult{Available: false, Error: "prometheus returned no usage metrics"}, nil
	}

	cpuRequests := extractValues(cpuRequestsResult, "node")
	memRequests := extractValues(memRequestsResult, "node")
	podCounts := extractValues(podCountResult, "node")

	// Build result
	result := make([]NodeMetrics, 0, len(nodes.Items))
	for _, node := range nodes.Items {
		name := node.Name
		cap := capacityMap[name]

		cpuUsageMilli := int64(cpuUsage[name] * 1000) // cores to millicores
		memUsageBytes := int64(memUsage[name])
		cpuReqMilli := int64(cpuRequests[name] * 1000) // cores to millicores
		memReqBytes := int64(memRequests[name])
		pods := int64(podCounts[name])

		// Committed = max(usage, requested)
		cpuCommitted := cpuUsageMilli
		if cpuReqMilli > cpuCommitted {
			cpuCommitted = cpuReqMilli
		}
		memCommitted := memUsageBytes
		if memReqBytes > memCommitted {
			memCommitted = memReqBytes
		}

		result = append(result, NodeMetrics{
			Name:         name,
			CPUUsage:     cpuUsageMilli,
			MemoryUsage:  memUsageBytes,
			CPUCapacity:  cap.cpu,
			MemCapacity:  cap.memory,
			CPURequested: cpuReqMilli,
			MemRequested: memReqBytes,
			CPUCommitted: cpuCommitted,
			MemCommitted: memCommitted,
			PodCount:     pods,
			PodCapacity:  cap.pods,
		})
	}

	return &NodeMetricsResult{Available: true, Metrics: result}, nil
}

// GetPodMetricsFromPrometheus fetches pod metrics from Prometheus as a fallback when metrics-server is unavailable
func (c *Client) GetPodMetricsFromPrometheus(contextName string, info *PrometheusInfo) (*PodMetricsResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cs, err := c.getClientsetForContext(contextName)
	if err != nil {
		return &PodMetricsResult{Available: false, Error: err.Error()}, nil
	}

	// Get nodes for capacity info
	nodes, err := cs.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return &PodMetricsResult{Available: false, Error: err.Error()}, nil
	}

	// Build node capacity map
	nodeCapacityMap := make(map[string]struct{ cpu, memory int64 })
	for _, node := range nodes.Items {
		nodeCapacityMap[node.Name] = struct{ cpu, memory int64 }{
			cpu:    node.Status.Capacity.Cpu().MilliValue(),
			memory: node.Status.Capacity.Memory().Value(),
		}
	}

	// Query Prometheus for pod-level metrics in parallel
	var cpuUsageResult, memUsageResult *PrometheusQueryResult
	var cpuRequestsResult, memRequestsResult *PrometheusQueryResult
	var podInfoResult *PrometheusQueryResult

	g, gctx := errgroup.WithContext(ctx)

	// Pod CPU usage (rate over 5m, excluding POD sandbox containers)
	g.Go(func() error {
		cpuUsageResult, _ = c.QueryPrometheusWithContext(gctx, contextName, info,
			`sum by (namespace, pod) (rate(container_cpu_usage_seconds_total{container!="", container!="POD"}[5m]))`)
		return nil
	})

	// Pod Memory usage (working set, excluding POD sandbox containers)
	g.Go(func() error {
		memUsageResult, _ = c.QueryPrometheusWithContext(gctx, contextName, info,
			`sum by (namespace, pod) (container_memory_working_set_bytes{container!="", container!="POD"})`)
		return nil
	})

	// Pod CPU requests
	g.Go(func() error {
		cpuRequestsResult, _ = c.QueryPrometheusWithContext(gctx, contextName, info,
			`sum by (namespace, pod) (kube_pod_container_resource_requests{resource="cpu", unit="core"})`)
		return nil
	})

	// Pod Memory requests
	g.Go(func() error {
		memRequestsResult, _ = c.QueryPrometheusWithContext(gctx, contextName, info,
			`sum by (namespace, pod) (kube_pod_container_resource_requests{resource="memory", unit="byte"})`)
		return nil
	})

	// Pod-to-node mapping
	g.Go(func() error {
		podInfoResult, _ = c.QueryPrometheusWithContext(gctx, contextName, info,
			`kube_pod_info{node!=""}`)
		return nil
	})

	_ = g.Wait() // All goroutines return nil, wait just for synchronization

	// Helper to extract pod-keyed values from Prometheus result (namespace + pod labels)
	extractPodValues := func(result *PrometheusQueryResult) map[string]float64 {
		values := make(map[string]float64)
		if result == nil || result.Data.Result == nil {
			return values
		}
		for _, r := range result.Data.Result {
			ns, nsOk := r.Metric["namespace"]
			pod, podOk := r.Metric["pod"]
			if !nsOk || !podOk {
				continue
			}
			if len(r.Value) >= 2 {
				if valStr, ok := r.Value[1].(string); ok {
					if val, err := strconv.ParseFloat(valStr, 64); err == nil {
						values[ns+"/"+pod] = val
					}
				}
			}
		}
		return values
	}

	cpuUsage := extractPodValues(cpuUsageResult)
	memUsage := extractPodValues(memUsageResult)

	// Check if we have any usage data
	if len(cpuUsage) == 0 && len(memUsage) == 0 {
		return &PodMetricsResult{Available: false, Error: "prometheus returned no pod usage metrics"}, nil
	}

	cpuRequests := extractPodValues(cpuRequestsResult)
	memRequests := extractPodValues(memRequestsResult)

	// Build pod-to-node mapping from kube_pod_info
	podNodeMap := make(map[string]string) // "namespace/pod" -> node
	if podInfoResult != nil && podInfoResult.Data.Result != nil {
		for _, r := range podInfoResult.Data.Result {
			ns := r.Metric["namespace"]
			pod := r.Metric["pod"]
			node := r.Metric["node"]
			if ns != "" && pod != "" && node != "" {
				podNodeMap[ns+"/"+pod] = node
			}
		}
	}

	// Collect all pod keys from usage data
	podKeys := make(map[string]struct{})
	for k := range cpuUsage {
		podKeys[k] = struct{}{}
	}
	for k := range memUsage {
		podKeys[k] = struct{}{}
	}

	// Build result
	result := make([]PodMetrics, 0, len(podKeys))
	for key := range podKeys {
		parts := strings.SplitN(key, "/", 2)
		if len(parts) != 2 {
			continue
		}
		ns, name := parts[0], parts[1]

		nodeName := podNodeMap[key]
		if nodeName == "" {
			continue // Skip pods without a node mapping
		}

		cpuUsageMilli := int64(cpuUsage[key] * 1000) // cores to millicores
		memUsageBytes := int64(memUsage[key])
		cpuReqMilli := int64(cpuRequests[key] * 1000) // cores to millicores
		memReqBytes := int64(memRequests[key])

		// Committed = max(usage, requested)
		cpuCommitted := cpuUsageMilli
		if cpuReqMilli > cpuCommitted {
			cpuCommitted = cpuReqMilli
		}
		memCommitted := memUsageBytes
		if memReqBytes > memCommitted {
			memCommitted = memReqBytes
		}

		nodeCap := nodeCapacityMap[nodeName]

		result = append(result, PodMetrics{
			Namespace:       ns,
			Name:            name,
			NodeName:        nodeName,
			CPUUsage:        cpuUsageMilli,
			MemoryUsage:     memUsageBytes,
			CPURequested:    cpuReqMilli,
			MemRequested:    memReqBytes,
			CPUCommitted:    cpuCommitted,
			MemCommitted:    memCommitted,
			NodeCPUCapacity: nodeCap.cpu,
			NodeMemCapacity: nodeCap.memory,
		})
	}

	return &PodMetricsResult{Available: true, Metrics: result}, nil
}

// GetPodMetrics fetches CPU and Memory metrics for all pods, relative to node capacity
func (c *Client) GetPodMetrics() (*PodMetricsResult, error) {
	c.mu.Lock()
	// Lazy init metrics client
	if c.metricsClient == nil {
		config, err := c.configLoading.ClientConfig()
		if err != nil {
			c.mu.Unlock()
			return &PodMetricsResult{Available: false, Error: err.Error()}, nil
		}
		mc, err := metricsclientset.NewForConfig(config)
		if err != nil {
			c.mu.Unlock()
			return &PodMetricsResult{Available: false, Error: err.Error()}, nil
		}
		c.metricsClient = mc
	}
	c.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get clientset early so we can use it in goroutines
	cs, err := c.getClientset()
	if err != nil {
		return &PodMetricsResult{Available: false, Error: err.Error()}, nil
	}

	// Fetch pod metrics, pods, and nodes in parallel
	var podMetricsList *metricsv1beta1.PodMetricsList
	var pods *v1.PodList
	var nodes *v1.NodeList

	g, gctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		var err error
		podMetricsList, err = c.metricsClient.MetricsV1beta1().PodMetricses("").List(gctx, metav1.ListOptions{})
		return err
	})

	g.Go(func() error {
		var err error
		pods, err = cs.CoreV1().Pods("").List(gctx, metav1.ListOptions{})
		return err
	})

	g.Go(func() error {
		var err error
		nodes, err = cs.CoreV1().Nodes().List(gctx, metav1.ListOptions{})
		return err
	})

	if err := g.Wait(); err != nil {
		return &PodMetricsResult{Available: false, Error: err.Error()}, nil
	}

	// Check if metrics-server returned any data
	// The API might exist but return empty results if metrics-server isn't functioning
	if podMetricsList == nil || len(podMetricsList.Items) == 0 {
		return &PodMetricsResult{Available: false, Error: "metrics-server returned no data"}, nil
	}

	// Build node capacity map
	nodeCapacityMap := make(map[string]struct{ cpu, memory int64 })
	for _, node := range nodes.Items {
		nodeCapacityMap[node.Name] = struct{ cpu, memory int64 }{
			cpu:    node.Status.Capacity.Cpu().MilliValue(),
			memory: node.Status.Capacity.Memory().Value(),
		}
	}

	// Build pod info map: namespace/name -> {nodeName, requests}
	type podInfo struct {
		nodeName string
		cpuReq   int64
		memReq   int64
	}
	podInfoMap := make(map[string]podInfo, len(pods.Items))
	for _, pod := range pods.Items {
		key := pod.Namespace + "/" + pod.Name
		var cpuReq, memReq int64
		for _, container := range pod.Spec.Containers {
			if container.Resources.Requests != nil {
				cpuReq += container.Resources.Requests.Cpu().MilliValue()
				memReq += container.Resources.Requests.Memory().Value()
			}
		}
		podInfoMap[key] = podInfo{
			nodeName: pod.Spec.NodeName,
			cpuReq:   cpuReq,
			memReq:   memReq,
		}
	}

	// Build result from pod metrics
	result := make([]PodMetrics, 0, len(podMetricsList.Items))
	for _, pm := range podMetricsList.Items {
		key := pm.Namespace + "/" + pm.Name
		info, ok := podInfoMap[key]
		if !ok || info.nodeName == "" {
			continue // Skip pods not found or not scheduled
		}

		// Sum container usage
		var cpuUsage, memUsage int64
		for _, cm := range pm.Containers {
			cpuUsage += cm.Usage.Cpu().MilliValue()
			memUsage += cm.Usage.Memory().Value()
		}

		// Calculate committed: max(usage, request)
		cpuCommitted := cpuUsage
		if info.cpuReq > cpuCommitted {
			cpuCommitted = info.cpuReq
		}
		memCommitted := memUsage
		if info.memReq > memCommitted {
			memCommitted = info.memReq
		}

		nodeCap := nodeCapacityMap[info.nodeName]

		result = append(result, PodMetrics{
			Namespace:       pm.Namespace,
			Name:            pm.Name,
			NodeName:        info.nodeName,
			CPUUsage:        cpuUsage,
			MemoryUsage:     memUsage,
			CPURequested:    info.cpuReq,
			MemRequested:    info.memReq,
			CPUCommitted:    cpuCommitted,
			MemCommitted:    memCommitted,
			NodeCPUCapacity: nodeCap.cpu,
			NodeMemCapacity: nodeCap.memory,
		})
	}

	return &PodMetricsResult{Available: true, Metrics: result}, nil
}

func (c *Client) ListNamespaces() ([]v1.Namespace, error) {
	start := time.Now()
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	log.Printf("[ListNamespaces] getClientset took %v", time.Since(start))
	apiStart := time.Now()
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	namespaces, err := cs.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	log.Printf("[ListNamespaces] API call took %v, returned %d items", time.Since(apiStart), len(namespaces.Items))
	if err != nil {
		return nil, err
	}
	return namespaces.Items, nil
}

// ListNamespacesWithContext lists namespaces with cancellation support
func (c *Client) ListNamespacesWithContext(ctx context.Context) ([]v1.Namespace, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	namespaces, err := cs.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		if isCancelledError(err) {
			return nil, ErrRequestCancelled
		}
		return nil, err
	}
	return namespaces.Items, nil
}

// ListNamespacesForContext lists namespaces for a specific kubeconfig context
func (c *Client) ListNamespacesForContext(contextName string) ([]v1.Namespace, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	namespaces, err := cs.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return namespaces.Items, nil
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

func (c *Client) ListEvents(namespace string) ([]v1.Event, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	events, err := cs.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return events.Items, nil
}

func (c *Client) ListEventsWithContext(ctx context.Context, namespace string) ([]v1.Event, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	events, err := cs.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		if isCancelledError(err) {
			return nil, ErrRequestCancelled
		}
		return nil, err
	}
	return events.Items, nil
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

func (c *Client) ListServices(namespace string) ([]v1.Service, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	services, err := cs.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return services.Items, nil
}

// ListServicesWithContext lists services with cancellation support
func (c *Client) ListServicesWithContext(ctx context.Context, namespace string) ([]v1.Service, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	services, err := cs.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		if isCancelledError(err) {
			return nil, ErrRequestCancelled
		}
		return nil, err
	}
	return services.Items, nil
}

// ListServicesForContext lists services for a specific kubeconfig context
func (c *Client) ListServicesForContext(contextName, namespace string) ([]v1.Service, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	services, err := cs.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return services.Items, nil
}

func (c *Client) GetServiceYaml(namespace, name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	service, err := cs.CoreV1().Services(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	// Remove managed fields
	service.ManagedFields = nil

	yamlBytes, err := yaml.Marshal(service)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateServiceYaml(namespace, name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	var service v1.Service
	if err := yaml.Unmarshal([]byte(yamlContent), &service); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.CoreV1().Services(namespace).Update(ctx, &service, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteService(contextName, namespace, name string) error {
	log.Printf("Deleting service: context=%s, ns=%s, name=%s", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return cs.CoreV1().Services(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

// Ingress operations
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

func (c *Client) ListConfigMaps(namespace string) ([]v1.ConfigMap, error) {
	start := time.Now()
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	log.Printf("[ListConfigMaps] getClientset took %v", time.Since(start))
	apiStart := time.Now()
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	cms, err := cs.CoreV1().ConfigMaps(namespace).List(ctx, metav1.ListOptions{})
	log.Printf("[ListConfigMaps] API call took %v, returned %d items", time.Since(apiStart), len(cms.Items))
	if err != nil {
		return nil, err
	}
	return cms.Items, nil
}

// ListConfigMapsWithContext lists configmaps with cancellation support
func (c *Client) ListConfigMapsWithContext(ctx context.Context, namespace string) ([]v1.ConfigMap, error) {
	start := time.Now()
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	getClientsetTime := time.Since(start)

	apiStart := time.Now()
	cms, err := cs.CoreV1().ConfigMaps(namespace).List(ctx, metav1.ListOptions{})
	apiTime := time.Since(apiStart)

	// Check context state
	ctxErr := ctx.Err()
	deadline, hasDeadline := ctx.Deadline()
	deadlineInfo := "no deadline"
	if hasDeadline {
		deadlineInfo = fmt.Sprintf("deadline in %v", time.Until(deadline))
	}

	log.Printf("[ListConfigMapsWithContext] getClientset=%v, API=%v, total=%v, ns=%q, items=%d, err=%v, ctxErr=%v, %s",
		getClientsetTime, apiTime, time.Since(start), namespace, len(cms.Items), err, ctxErr, deadlineInfo)
	if err != nil {
		if isCancelledError(err) {
			return nil, ErrRequestCancelled
		}
		return nil, err
	}
	return cms.Items, nil
}

// ListConfigMapsForContext lists configmaps for a specific kubeconfig context
func (c *Client) ListConfigMapsForContext(contextName, namespace string) ([]v1.ConfigMap, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	cms, err := cs.CoreV1().ConfigMaps(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return cms.Items, nil
}

func (c *Client) ListSecrets(namespace string) ([]v1.Secret, error) {
	start := time.Now()
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	log.Printf("[ListSecrets] getClientset took %v", time.Since(start))
	apiStart := time.Now()
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	secrets, err := cs.CoreV1().Secrets(namespace).List(ctx, metav1.ListOptions{})
	log.Printf("[ListSecrets] API call took %v, returned %d items", time.Since(apiStart), len(secrets.Items))
	if err != nil {
		return nil, err
	}
	// Sanitize secrets? For now, we return them as is, UI should handle masking.
	return secrets.Items, nil
}

// ListSecretsWithContext lists secrets with cancellation support
func (c *Client) ListSecretsWithContext(ctx context.Context, namespace string) ([]v1.Secret, error) {
	start := time.Now()
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	secrets, err := cs.CoreV1().Secrets(namespace).List(ctx, metav1.ListOptions{})
	log.Printf("[ListSecretsWithContext] API call took %v, ns=%q, items=%d, err=%v", time.Since(start), namespace, len(secrets.Items), err)
	if err != nil {
		if isCancelledError(err) {
			return nil, ErrRequestCancelled
		}
		return nil, err
	}
	return secrets.Items, nil
}

// ListSecretsForContext lists secrets for a specific kubeconfig context
func (c *Client) ListSecretsForContext(contextName, namespace string) ([]v1.Secret, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	secrets, err := cs.CoreV1().Secrets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return secrets.Items, nil
}

// SecretListItem is a lightweight representation of a Secret for list views.
// It contains only the fields needed for display, avoiding transfer of actual secret data.
type SecretListItem struct {
	Metadata SecretMetadata `json:"metadata"`
	Type     string         `json:"type"`
	DataKeys int            `json:"dataKeys"` // Number of data keys, not the actual data
}

// SecretMetadata contains only the metadata fields needed for list display
type SecretMetadata struct {
	Name              string            `json:"name"`
	Namespace         string            `json:"namespace"`
	UID               string            `json:"uid"`
	CreationTimestamp metav1.Time       `json:"creationTimestamp"`
	Labels            map[string]string `json:"labels,omitempty"`
	Annotations       map[string]string `json:"annotations,omitempty"`
}

// ListSecretsMetadataWithContext lists secrets using metadata-only fetch for list views.
// This avoids transferring the actual secret data, significantly reducing response size.
func (c *Client) ListSecretsMetadataWithContext(ctx context.Context, namespace string) ([]SecretListItem, error) {
	start := time.Now()
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}

	// Build the request path
	var path string
	if namespace == "" {
		path = "/api/v1/secrets"
	} else {
		path = fmt.Sprintf("/api/v1/namespaces/%s/secrets", namespace)
	}

	// Use Table format with metadata-only objects
	// This returns column data (name, type, data count, age) plus minimal object metadata
	// without the actual secret data
	result := cs.CoreV1().RESTClient().Get().
		AbsPath(path).
		SetHeader("Accept", "application/json;as=Table;g=meta.k8s.io;v=v1").
		Do(ctx)

	if err := result.Error(); err != nil {
		log.Printf("[ListSecretsMetadata] API call failed after %v, ns=%q, err=%v", time.Since(start), namespace, err)
		if isCancelledError(err) {
			return nil, ErrRequestCancelled
		}
		return nil, err
	}

	// Parse the Table response
	body, err := result.Raw()
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var table metav1.Table
	if err := json.Unmarshal(body, &table); err != nil {
		return nil, fmt.Errorf("failed to parse table response: %w", err)
	}

	// Find column indices
	nameIdx, typeIdx, dataIdx := -1, -1, -1
	for i, col := range table.ColumnDefinitions {
		switch col.Name {
		case "Name":
			nameIdx = i
		case "Type":
			typeIdx = i
		case "Data":
			dataIdx = i
		}
	}

	// Convert rows to SecretListItem
	items := make([]SecretListItem, 0, len(table.Rows))
	for _, row := range table.Rows {
		item := SecretListItem{}

		// Extract cells
		if nameIdx >= 0 && nameIdx < len(row.Cells) {
			if name, ok := row.Cells[nameIdx].(string); ok {
				item.Metadata.Name = name
			}
		}
		if typeIdx >= 0 && typeIdx < len(row.Cells) {
			if t, ok := row.Cells[typeIdx].(string); ok {
				item.Type = t
			}
		}
		if dataIdx >= 0 && dataIdx < len(row.Cells) {
			// Data column contains count as number
			switch v := row.Cells[dataIdx].(type) {
			case float64:
				item.DataKeys = int(v)
			case int64:
				item.DataKeys = int(v)
			case int:
				item.DataKeys = v
			}
		}

		// Extract metadata from the embedded object
		if row.Object.Raw != nil {
			var partialMeta struct {
				Metadata struct {
					Name              string            `json:"name"`
					Namespace         string            `json:"namespace"`
					UID               string            `json:"uid"`
					CreationTimestamp metav1.Time       `json:"creationTimestamp"`
					Labels            map[string]string `json:"labels,omitempty"`
					Annotations       map[string]string `json:"annotations,omitempty"`
				} `json:"metadata"`
			}
			if err := json.Unmarshal(row.Object.Raw, &partialMeta); err == nil {
				item.Metadata.Name = partialMeta.Metadata.Name
				item.Metadata.Namespace = partialMeta.Metadata.Namespace
				item.Metadata.UID = partialMeta.Metadata.UID
				item.Metadata.CreationTimestamp = partialMeta.Metadata.CreationTimestamp
				item.Metadata.Labels = partialMeta.Metadata.Labels
				item.Metadata.Annotations = partialMeta.Metadata.Annotations
			}
		}

		items = append(items, item)
	}

	log.Printf("[ListSecretsMetadata] API call took %v, ns=%q, items=%d", time.Since(start), namespace, len(items))
	return items, nil
}

// ConfigMap YAML operations
func (c *Client) GetConfigMapYaml(namespace, name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	configMap, err := cs.CoreV1().ConfigMaps(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	// Remove managed fields
	configMap.ManagedFields = nil

	yamlBytes, err := yaml.Marshal(configMap)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateConfigMapYaml(namespace, name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	var configMap v1.ConfigMap
	if err := yaml.Unmarshal([]byte(yamlContent), &configMap); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.CoreV1().ConfigMaps(namespace).Update(ctx, &configMap, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteConfigMap(contextName, namespace, name string) error {
	log.Printf("Deleting configmap: context=%s, ns=%s, name=%s", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return cs.CoreV1().ConfigMaps(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

func (c *Client) GetConfigMapData(namespace, name string) (map[string]string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	cm, err := cs.CoreV1().ConfigMaps(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	result := make(map[string]string)
	for k, v := range cm.Data {
		result[k] = v
	}
	return result, nil
}

// UpdateConfigMapData updates the configmap's data from a map of key -> value
func (c *Client) UpdateConfigMapData(namespace, name string, data map[string]string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	cm, err := cs.CoreV1().ConfigMaps(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	cm.Data = data
	_, err = cs.CoreV1().ConfigMaps(namespace).Update(ctx, cm, metav1.UpdateOptions{})
	return err
}

// Secret YAML operations
func (c *Client) GetSecretYaml(namespace, name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	secret, err := cs.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	// Remove managed fields
	secret.ManagedFields = nil

	yamlBytes, err := yaml.Marshal(secret)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateSecretYaml(namespace, name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	var secret v1.Secret
	if err := yaml.Unmarshal([]byte(yamlContent), &secret); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.CoreV1().Secrets(namespace).Update(ctx, &secret, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteSecret(contextName, namespace, name string) error {
	log.Printf("Deleting secret: context=%s, ns=%s, name=%s", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return cs.CoreV1().Secrets(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

// GetSecretData returns the secret's data as a map of key -> base64-encoded value
func (c *Client) GetSecretData(namespace, name string) (map[string]string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	secret, err := cs.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	result := make(map[string]string)
	for k, v := range secret.Data {
		result[k] = string(v)
	}
	return result, nil
}

// UpdateSecretData updates the secret's data from a map of key -> value (values are raw strings, will be stored as bytes)
func (c *Client) UpdateSecretData(namespace, name string, data map[string]string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	secret, err := cs.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	secret.Data = make(map[string][]byte)
	for k, v := range data {
		secret.Data[k] = []byte(v)
	}
	_, err = cs.CoreV1().Secrets(namespace).Update(ctx, secret, metav1.UpdateOptions{})
	return err
}

func (c *Client) ListDeployments(namespace string) ([]appsv1.Deployment, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	deployments, err := cs.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return deployments.Items, nil
}

// ListDeploymentsWithContext lists deployments with cancellation support
func (c *Client) ListDeploymentsWithContext(ctx context.Context, namespace string) ([]appsv1.Deployment, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	deployments, err := cs.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		if isCancelledError(err) {
			return nil, ErrRequestCancelled
		}
		return nil, err
	}
	return deployments.Items, nil
}

// ListDeploymentsForContext lists deployments for a specific kubeconfig context
func (c *Client) ListDeploymentsForContext(contextName, namespace string) ([]appsv1.Deployment, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	deployments, err := cs.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return deployments.Items, nil
}

func (c *Client) GetPodLogs(namespace, podName, containerName string, timestamps bool, previous bool, sinceTime string) (string, error) {
	// When sinceTime is set, we need to get logs starting from that time (first N lines after sinceTime)
	// Kubernetes TailLines gives last N lines, so we fetch all and truncate to first 200
	if sinceTime != "" {
		allLogs, err := c.getPodLogsWithOptions(namespace, podName, containerName, nil, timestamps, previous, sinceTime)
		if err != nil {
			return "", err
		}
		lines := strings.Split(allLogs, "\n")
		if len(lines) <= 200 {
			return allLogs, nil
		}
		return strings.Join(lines[:200], "\n"), nil
	}
	// Default: get last 200 lines
	return c.getPodLogsWithOptions(namespace, podName, containerName, func(i int64) *int64 { return &i }(200), timestamps, previous, sinceTime)
}

func (c *Client) GetAllPodLogs(namespace, podName, containerName string, timestamps bool, previous bool) (string, error) {
	return c.getPodLogsWithOptions(namespace, podName, containerName, nil, timestamps, previous, "")
}

// GetPodLogsFromStart fetches all logs and returns the first N lines (default 200)
func (c *Client) GetPodLogsFromStart(namespace, podName, containerName string, timestamps bool, previous bool, lineLimit int) (string, error) {
	allLogs, err := c.getPodLogsWithOptions(namespace, podName, containerName, nil, timestamps, previous, "")
	if err != nil {
		return "", err
	}
	if lineLimit <= 0 {
		lineLimit = 200
	}
	lines := strings.Split(allLogs, "\n")
	if len(lines) <= lineLimit {
		return allLogs, nil
	}
	return strings.Join(lines[:lineLimit], "\n"), nil
}

// GetPodLogsBefore fetches logs before a given timestamp.
// Returns up to lineLimit lines that occur before the specified timestamp.
// The beforeTime should be in RFC3339 format (e.g., 2024-11-26T14:30:00Z).
// Returns the logs and a boolean indicating if there are more logs before these.
func (c *Client) GetPodLogsBefore(namespace, podName, containerName string, timestamps bool, previous bool, beforeTime string, lineLimit int) (string, bool, error) {
	allLogs, err := c.getPodLogsWithOptions(namespace, podName, containerName, nil, true, previous, "") // Always fetch with timestamps to find position
	if err != nil {
		return "", false, err
	}
	if lineLimit <= 0 {
		lineLimit = 200
	}

	lines := strings.Split(allLogs, "\n")

	// Normalize beforeTime - extract just the comparable portion (first 30 chars if available)
	compareLen := 30
	if len(beforeTime) < compareLen {
		compareLen = len(beforeTime)
	}
	beforeTimePrefix := beforeTime[:compareLen]

	// Find the line index where timestamp >= beforeTime (strict: we want lines BEFORE this)
	cutoffIndex := -1
	for i, line := range lines {
		if len(line) >= 30 { // Timestamp is at least 30 chars: 2024-11-26T14:30:00.123456789Z
			lineTime := line[:30]
			// Use >= to find the first line at or after beforeTime
			// We exclude this line and all after it
			if lineTime >= beforeTimePrefix {
				cutoffIndex = i
				break
			}
		}
	}

	var resultLines []string
	hasMoreBefore := false

	if cutoffIndex == -1 {
		// beforeTime is after all logs, return last lineLimit lines
		if len(lines) > lineLimit {
			resultLines = lines[len(lines)-lineLimit:]
			hasMoreBefore = true
		} else {
			resultLines = lines
		}
	} else if cutoffIndex == 0 {
		// beforeTime is before all logs, nothing to return
		return "", false, nil
	} else {
		// Return lineLimit lines before cutoffIndex
		startIndex := cutoffIndex - lineLimit
		if startIndex < 0 {
			startIndex = 0
		} else {
			hasMoreBefore = true
		}
		resultLines = lines[startIndex:cutoffIndex]
	}

	// If caller doesn't want timestamps, strip them
	if !timestamps {
		for i, line := range resultLines {
			if len(line) > 31 {
				resultLines[i] = line[31:] // Skip timestamp and space
			}
		}
	}

	return strings.Join(resultLines, "\n"), hasMoreBefore, nil
}

// GetPodLogsAfter fetches logs after a given timestamp.
// Returns up to lineLimit lines that occur after the specified timestamp.
// The afterTime should be in RFC3339 format.
// Returns the logs and a boolean indicating if there are more logs after these.
func (c *Client) GetPodLogsAfter(namespace, podName, containerName string, timestamps bool, previous bool, afterTime string, lineLimit int) (string, bool, error) {
	// Always fetch with timestamps so we can properly compare
	allLogs, err := c.getPodLogsWithOptions(namespace, podName, containerName, nil, true, previous, afterTime)
	if err != nil {
		return "", false, err
	}
	if lineLimit <= 0 {
		lineLimit = 200
	}

	lines := strings.Split(allLogs, "\n")

	// Normalize afterTime for comparison
	compareLen := 30
	if len(afterTime) < compareLen {
		compareLen = len(afterTime)
	}
	afterTimePrefix := afterTime[:compareLen]

	// Skip lines that are at or before our afterTime marker (we already have these)
	startIdx := 0
	for i, line := range lines {
		if len(line) >= 30 {
			lineTime := line[:30]
			// Skip this line if its timestamp is <= afterTime (we already have it)
			if lineTime <= afterTimePrefix {
				startIdx = i + 1
				continue
			}
		}
		break
	}

	if startIdx >= len(lines) {
		return "", false, nil
	}

	lines = lines[startIdx:]

	hasMoreAfter := len(lines) > lineLimit
	if hasMoreAfter {
		lines = lines[:lineLimit]
	}

	// Strip timestamps if caller doesn't want them
	if !timestamps {
		for i, line := range lines {
			if len(line) > 31 {
				lines[i] = line[31:] // Skip timestamp and space
			}
		}
	}

	return strings.Join(lines, "\n"), hasMoreAfter, nil
}

func (c *Client) getPodLogsWithOptions(namespace, podName, containerName string, tailLines *int64, timestamps bool, previous bool, sinceTime string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()

	opts := &v1.PodLogOptions{
		TailLines:  tailLines,
		Timestamps: timestamps,
		Previous:   previous,
	}
	if containerName != "" {
		opts.Container = containerName
	}
	if sinceTime != "" {
		t, err := time.Parse(time.RFC3339, sinceTime)
		if err == nil {
			mt := metav1.NewTime(t)
			opts.SinceTime = &mt
		}
	}

	req := cs.CoreV1().Pods(namespace).GetLogs(podName, opts)

	podLogs, err := req.Stream(ctx)
	if err != nil {
		return "", err
	}
	defer podLogs.Close()

	buf := new(strings.Builder)
	_, err = io.Copy(buf, podLogs)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

// StreamPodLogs streams logs from a pod container and calls the callback for each line.
// It continues until the context is canceled or an error occurs.
// The callback receives each log line as it arrives.
func (c *Client) StreamPodLogs(ctx context.Context, namespace, podName, containerName string, timestamps bool, tailLines int64, onLine func(line string)) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}

	opts := &v1.PodLogOptions{
		Follow:     true,
		Timestamps: timestamps,
	}
	if tailLines > 0 {
		opts.TailLines = &tailLines
	}
	if containerName != "" {
		opts.Container = containerName
	}

	req := cs.CoreV1().Pods(namespace).GetLogs(podName, opts)
	stream, err := req.Stream(ctx)
	if err != nil {
		return err
	}
	defer stream.Close()

	scanner := bufio.NewScanner(stream)
	// Increase buffer size for long log lines
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			onLine(scanner.Text())
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}
	return nil
}

// timestampedLogLine represents a log line with parsed timestamp for sorting
type timestampedLogLine struct {
	timestamp string // RFC3339 format timestamp
	content   string // Full line content including timestamp and container prefix
}

// GetAllContainersLogs fetches logs from all containers in a pod, merges them by timestamp,
// and prefixes each line with [containerName]. Returns the last 200 lines by default.
func (c *Client) GetAllContainersLogs(namespace, podName string, containerNames []string, timestamps bool, previous bool, sinceTime string) (string, error) {
	if len(containerNames) == 0 {
		return "", nil
	}

	// Fetch logs from all containers concurrently
	type containerLogs struct {
		containerName string
		logs          string
		err           error
	}

	results := make(chan containerLogs, len(containerNames))
	var wg sync.WaitGroup

	for _, containerName := range containerNames {
		wg.Add(1)
		go func(cn string) {
			defer wg.Done()
			// Always fetch with timestamps so we can sort
			logs, err := c.getPodLogsWithOptions(namespace, podName, cn, nil, true, previous, sinceTime)
			results <- containerLogs{containerName: cn, logs: logs, err: err}
		}(containerName)
	}

	wg.Wait()
	close(results)

	// Collect all log lines with timestamps
	var allLines []timestampedLogLine
	for result := range results {
		if result.err != nil {
			// Add error as a log line
			allLines = append(allLines, timestampedLogLine{
				timestamp: time.Now().Format(time.RFC3339Nano),
				content:   fmt.Sprintf("[%s] Error fetching logs: %v", result.containerName, result.err),
			})
			continue
		}

		lines := strings.Split(result.logs, "\n")
		for _, line := range lines {
			if line == "" {
				continue
			}
			// Parse timestamp from line (first 30 chars)
			var ts, content string
			if len(line) >= 31 && line[30] == ' ' {
				ts = line[:30]
				content = line[31:]
			} else {
				ts = ""
				content = line
			}

			// Build the merged line with container prefix
			var mergedLine string
			if timestamps && ts != "" {
				mergedLine = fmt.Sprintf("%s [%s] %s", ts, result.containerName, content)
			} else if ts != "" {
				mergedLine = fmt.Sprintf("%s [%s] %s", ts, result.containerName, content)
			} else {
				mergedLine = fmt.Sprintf("[%s] %s", result.containerName, content)
			}

			allLines = append(allLines, timestampedLogLine{
				timestamp: ts,
				content:   mergedLine,
			})
		}
	}

	// Sort by timestamp
	sort.SliceStable(allLines, func(i, j int) bool {
		return allLines[i].timestamp < allLines[j].timestamp
	})

	// Build result, taking last 200 lines if sinceTime is empty
	var resultLines []string
	for _, line := range allLines {
		if timestamps {
			resultLines = append(resultLines, line.content)
		} else {
			// Strip the timestamp prefix if caller doesn't want timestamps
			if len(line.content) > 31 && line.content[30] == ' ' {
				resultLines = append(resultLines, line.content[31:])
			} else {
				resultLines = append(resultLines, line.content)
			}
		}
	}

	// If sinceTime is set, return first 200 lines after that time
	// Otherwise return last 200 lines
	if sinceTime != "" && len(resultLines) > 200 {
		resultLines = resultLines[:200]
	} else if sinceTime == "" && len(resultLines) > 200 {
		resultLines = resultLines[len(resultLines)-200:]
	}

	return strings.Join(resultLines, "\n"), nil
}

// GetAllContainersLogsAll fetches all logs from all containers, merged by timestamp
func (c *Client) GetAllContainersLogsAll(namespace, podName string, containerNames []string, timestamps bool, previous bool) (string, error) {
	if len(containerNames) == 0 {
		return "", nil
	}

	// Fetch logs from all containers concurrently
	type containerLogs struct {
		containerName string
		logs          string
		err           error
	}

	results := make(chan containerLogs, len(containerNames))
	var wg sync.WaitGroup

	for _, containerName := range containerNames {
		wg.Add(1)
		go func(cn string) {
			defer wg.Done()
			logs, err := c.getPodLogsWithOptions(namespace, podName, cn, nil, true, previous, "")
			results <- containerLogs{containerName: cn, logs: logs, err: err}
		}(containerName)
	}

	wg.Wait()
	close(results)

	// Collect and merge all log lines
	var allLines []timestampedLogLine
	for result := range results {
		if result.err != nil {
			allLines = append(allLines, timestampedLogLine{
				timestamp: time.Now().Format(time.RFC3339Nano),
				content:   fmt.Sprintf("[%s] Error fetching logs: %v", result.containerName, result.err),
			})
			continue
		}

		lines := strings.Split(result.logs, "\n")
		for _, line := range lines {
			if line == "" {
				continue
			}
			var ts, content string
			if len(line) >= 31 && line[30] == ' ' {
				ts = line[:30]
				content = line[31:]
			} else {
				ts = ""
				content = line
			}

			var mergedLine string
			if ts != "" {
				mergedLine = fmt.Sprintf("%s [%s] %s", ts, result.containerName, content)
			} else {
				mergedLine = fmt.Sprintf("[%s] %s", result.containerName, content)
			}

			allLines = append(allLines, timestampedLogLine{
				timestamp: ts,
				content:   mergedLine,
			})
		}
	}

	sort.SliceStable(allLines, func(i, j int) bool {
		return allLines[i].timestamp < allLines[j].timestamp
	})

	var resultLines []string
	for _, line := range allLines {
		if timestamps {
			resultLines = append(resultLines, line.content)
		} else {
			if len(line.content) > 31 && line.content[30] == ' ' {
				resultLines = append(resultLines, line.content[31:])
			} else {
				resultLines = append(resultLines, line.content)
			}
		}
	}

	return strings.Join(resultLines, "\n"), nil
}

// GetAllContainersLogsFromStart fetches the first N lines from all containers, merged by timestamp
func (c *Client) GetAllContainersLogsFromStart(namespace, podName string, containerNames []string, timestamps bool, previous bool, lineLimit int) (string, error) {
	allLogs, err := c.GetAllContainersLogsAll(namespace, podName, containerNames, timestamps, previous)
	if err != nil {
		return "", err
	}
	if lineLimit <= 0 {
		lineLimit = 200
	}
	lines := strings.Split(allLogs, "\n")
	if len(lines) <= lineLimit {
		return allLogs, nil
	}
	return strings.Join(lines[:lineLimit], "\n"), nil
}

// GetAllContainersLogsBefore fetches logs before a given timestamp from all containers
func (c *Client) GetAllContainersLogsBefore(namespace, podName string, containerNames []string, timestamps bool, previous bool, beforeTime string, lineLimit int) (string, bool, error) {
	// Fetch all logs with timestamps to properly merge and find position
	allLogs, err := c.GetAllContainersLogsAll(namespace, podName, containerNames, true, previous)
	if err != nil {
		return "", false, err
	}
	if lineLimit <= 0 {
		lineLimit = 200
	}

	lines := strings.Split(allLogs, "\n")

	// Normalize beforeTime
	compareLen := 30
	if len(beforeTime) < compareLen {
		compareLen = len(beforeTime)
	}
	beforeTimePrefix := beforeTime[:compareLen]

	// Find cutoff index
	cutoffIndex := -1
	for i, line := range lines {
		if len(line) >= 30 {
			lineTime := line[:30]
			if lineTime >= beforeTimePrefix {
				cutoffIndex = i
				break
			}
		}
	}

	var resultLines []string
	hasMoreBefore := false

	if cutoffIndex == -1 {
		if len(lines) > lineLimit {
			resultLines = lines[len(lines)-lineLimit:]
			hasMoreBefore = true
		} else {
			resultLines = lines
		}
	} else if cutoffIndex == 0 {
		return "", false, nil
	} else {
		startIndex := cutoffIndex - lineLimit
		if startIndex < 0 {
			startIndex = 0
		} else {
			hasMoreBefore = true
		}
		resultLines = lines[startIndex:cutoffIndex]
	}

	// Strip timestamps if caller doesn't want them
	if !timestamps {
		for i, line := range resultLines {
			if len(line) > 31 {
				resultLines[i] = line[31:]
			}
		}
	}

	return strings.Join(resultLines, "\n"), hasMoreBefore, nil
}

// GetAllContainersLogsAfter fetches logs after a given timestamp from all containers
func (c *Client) GetAllContainersLogsAfter(namespace, podName string, containerNames []string, timestamps bool, previous bool, afterTime string, lineLimit int) (string, bool, error) {
	// Fetch all logs with timestamps
	allLogs, err := c.GetAllContainersLogsAll(namespace, podName, containerNames, true, previous)
	if err != nil {
		return "", false, err
	}
	if lineLimit <= 0 {
		lineLimit = 200
	}

	lines := strings.Split(allLogs, "\n")

	// Normalize afterTime
	compareLen := 30
	if len(afterTime) < compareLen {
		compareLen = len(afterTime)
	}
	afterTimePrefix := afterTime[:compareLen]

	// Skip lines at or before afterTime
	startIdx := 0
	for i, line := range lines {
		if len(line) >= 30 {
			lineTime := line[:30]
			if lineTime <= afterTimePrefix {
				startIdx = i + 1
				continue
			}
		}
		break
	}

	if startIdx >= len(lines) {
		return "", false, nil
	}

	lines = lines[startIdx:]

	hasMoreAfter := len(lines) > lineLimit
	if hasMoreAfter {
		lines = lines[:lineLimit]
	}

	// Strip timestamps if caller doesn't want them
	if !timestamps {
		for i, line := range lines {
			if len(line) > 31 {
				lines[i] = line[31:]
			}
		}
	}

	return strings.Join(lines, "\n"), hasMoreAfter, nil
}

// StreamAllContainersLogs streams logs from all containers, merging them in real-time by timestamp.
// Each line is prefixed with [containerName].
func (c *Client) StreamAllContainersLogs(ctx context.Context, namespace, podName string, containerNames []string, timestamps bool, tailLines int64, onLine func(line string)) error {
	if len(containerNames) == 0 {
		return nil
	}

	// For real-time streaming, we need to collect lines from all containers
	// and emit them in timestamp order. We use a priority queue approach.
	type streamLine struct {
		timestamp     string
		containerName string
		content       string
		fullLine      string
	}

	lineChan := make(chan streamLine, 1000)
	var wg sync.WaitGroup
	errChan := make(chan error, len(containerNames))

	// Start a goroutine for each container
	for _, containerName := range containerNames {
		wg.Add(1)
		go func(cn string) {
			defer wg.Done()
			err := c.StreamPodLogs(ctx, namespace, podName, cn, true, tailLines, func(line string) {
				var ts, content string
				if len(line) >= 31 && line[30] == ' ' {
					ts = line[:30]
					content = line[31:]
				} else {
					ts = ""
					content = line
				}

				var fullLine string
				if timestamps && ts != "" {
					fullLine = fmt.Sprintf("%s [%s] %s", ts, cn, content)
				} else if ts != "" {
					fullLine = fmt.Sprintf("%s [%s] %s", ts, cn, content)
				} else {
					fullLine = fmt.Sprintf("[%s] %s", cn, content)
				}

				select {
				case lineChan <- streamLine{timestamp: ts, containerName: cn, content: content, fullLine: fullLine}:
				case <-ctx.Done():
					return
				}
			})
			if err != nil && err != context.Canceled {
				errChan <- err
			}
		}(containerName)
	}

	// Close channels when all goroutines complete
	go func() {
		wg.Wait()
		close(lineChan)
		close(errChan)
	}()

	// Buffer for sorting incoming lines within a small time window
	var buffer []streamLine
	flushTicker := time.NewTicker(50 * time.Millisecond)
	defer flushTicker.Stop()

	flushBuffer := func() {
		if len(buffer) == 0 {
			return
		}
		// Sort buffer by timestamp
		sort.SliceStable(buffer, func(i, j int) bool {
			return buffer[i].timestamp < buffer[j].timestamp
		})
		for _, line := range buffer {
			if timestamps {
				onLine(line.fullLine)
			} else {
				// Strip timestamp from output
				if len(line.fullLine) > 31 && line.fullLine[30] == ' ' {
					onLine(line.fullLine[31:])
				} else {
					onLine(line.fullLine)
				}
			}
		}
		buffer = buffer[:0]
	}

	for {
		select {
		case <-ctx.Done():
			flushBuffer()
			return ctx.Err()
		case line, ok := <-lineChan:
			if !ok {
				flushBuffer()
				// Check for errors
				for err := range errChan {
					if err != nil {
						return err
					}
				}
				return nil
			}
			buffer = append(buffer, line)
		case <-flushTicker.C:
			flushBuffer()
		}
	}
}

// PodContainerPair represents a pod and its containers for multi-pod log fetching
type PodContainerPair struct {
	PodName        string
	ContainerNames []string // If empty or single, just use [podName] prefix; if multiple, use [podName/containerName]
}

// GetAllPodsLogs fetches logs from multiple pods, merges them by timestamp.
// When allContainers is true, prefixes with [podName/containerName], otherwise [podName].
func (c *Client) GetAllPodsLogs(namespace string, pods []PodContainerPair, allContainers bool, timestamps bool, previous bool, sinceTime string) (string, error) {
	if len(pods) == 0 {
		return "", nil
	}

	type podContainerLogs struct {
		podName       string
		containerName string
		logs          string
		err           error
	}

	// Count total fetches needed
	totalFetches := 0
	for _, p := range pods {
		if allContainers && len(p.ContainerNames) > 0 {
			totalFetches += len(p.ContainerNames)
		} else {
			totalFetches++
		}
	}

	results := make(chan podContainerLogs, totalFetches)
	var wg sync.WaitGroup

	for _, p := range pods {
		if allContainers && len(p.ContainerNames) > 1 {
			// Fetch from all containers
			for _, cn := range p.ContainerNames {
				wg.Add(1)
				go func(podName, containerName string) {
					defer wg.Done()
					logs, err := c.getPodLogsWithOptions(namespace, podName, containerName, nil, true, previous, sinceTime)
					results <- podContainerLogs{podName: podName, containerName: containerName, logs: logs, err: err}
				}(p.PodName, cn)
			}
		} else {
			// Fetch from single/first container
			containerName := ""
			if len(p.ContainerNames) > 0 {
				containerName = p.ContainerNames[0]
			}
			wg.Add(1)
			go func(podName, cn string) {
				defer wg.Done()
				logs, err := c.getPodLogsWithOptions(namespace, podName, cn, nil, true, previous, sinceTime)
				results <- podContainerLogs{podName: podName, containerName: cn, logs: logs, err: err}
			}(p.PodName, containerName)
		}
	}

	wg.Wait()
	close(results)

	var allLines []timestampedLogLine
	for result := range results {
		prefix := result.podName
		if allContainers && result.containerName != "" {
			prefix = result.podName + "/" + result.containerName
		}

		if result.err != nil {
			allLines = append(allLines, timestampedLogLine{
				timestamp: time.Now().Format(time.RFC3339Nano),
				content:   fmt.Sprintf("[%s] Error fetching logs: %v", prefix, result.err),
			})
			continue
		}

		lines := strings.Split(result.logs, "\n")
		for _, line := range lines {
			if line == "" {
				continue
			}
			var ts, content string
			if len(line) >= 31 && line[30] == ' ' {
				ts = line[:30]
				content = line[31:]
			} else {
				ts = ""
				content = line
			}

			var mergedLine string
			if ts != "" {
				mergedLine = fmt.Sprintf("%s [%s] %s", ts, prefix, content)
			} else {
				mergedLine = fmt.Sprintf("[%s] %s", prefix, content)
			}

			allLines = append(allLines, timestampedLogLine{
				timestamp: ts,
				content:   mergedLine,
			})
		}
	}

	sort.SliceStable(allLines, func(i, j int) bool {
		return allLines[i].timestamp < allLines[j].timestamp
	})

	var resultLines []string
	for _, line := range allLines {
		if timestamps {
			resultLines = append(resultLines, line.content)
		} else {
			if len(line.content) > 31 && line.content[30] == ' ' {
				resultLines = append(resultLines, line.content[31:])
			} else {
				resultLines = append(resultLines, line.content)
			}
		}
	}

	if sinceTime != "" && len(resultLines) > 200 {
		resultLines = resultLines[:200]
	} else if sinceTime == "" && len(resultLines) > 200 {
		resultLines = resultLines[len(resultLines)-200:]
	}

	return strings.Join(resultLines, "\n"), nil
}

// GetAllPodsLogsAll fetches all logs from multiple pods, merged by timestamp (no truncation)
func (c *Client) GetAllPodsLogsAll(namespace string, pods []PodContainerPair, allContainers bool, timestamps bool, previous bool) (string, error) {
	if len(pods) == 0 {
		return "", nil
	}

	type podContainerLogs struct {
		podName       string
		containerName string
		logs          string
		err           error
	}

	totalFetches := 0
	for _, p := range pods {
		if allContainers && len(p.ContainerNames) > 0 {
			totalFetches += len(p.ContainerNames)
		} else {
			totalFetches++
		}
	}

	results := make(chan podContainerLogs, totalFetches)
	var wg sync.WaitGroup

	for _, p := range pods {
		if allContainers && len(p.ContainerNames) > 1 {
			for _, cn := range p.ContainerNames {
				wg.Add(1)
				go func(podName, containerName string) {
					defer wg.Done()
					logs, err := c.getPodLogsWithOptions(namespace, podName, containerName, nil, true, previous, "")
					results <- podContainerLogs{podName: podName, containerName: containerName, logs: logs, err: err}
				}(p.PodName, cn)
			}
		} else {
			containerName := ""
			if len(p.ContainerNames) > 0 {
				containerName = p.ContainerNames[0]
			}
			wg.Add(1)
			go func(podName, cn string) {
				defer wg.Done()
				logs, err := c.getPodLogsWithOptions(namespace, podName, cn, nil, true, previous, "")
				results <- podContainerLogs{podName: podName, containerName: cn, logs: logs, err: err}
			}(p.PodName, containerName)
		}
	}

	wg.Wait()
	close(results)

	var allLines []timestampedLogLine
	for result := range results {
		prefix := result.podName
		if allContainers && result.containerName != "" {
			prefix = result.podName + "/" + result.containerName
		}

		if result.err != nil {
			allLines = append(allLines, timestampedLogLine{
				timestamp: time.Now().Format(time.RFC3339Nano),
				content:   fmt.Sprintf("[%s] Error fetching logs: %v", prefix, result.err),
			})
			continue
		}

		lines := strings.Split(result.logs, "\n")
		for _, line := range lines {
			if line == "" {
				continue
			}
			var ts, content string
			if len(line) >= 31 && line[30] == ' ' {
				ts = line[:30]
				content = line[31:]
			} else {
				ts = ""
				content = line
			}

			var mergedLine string
			if ts != "" {
				mergedLine = fmt.Sprintf("%s [%s] %s", ts, prefix, content)
			} else {
				mergedLine = fmt.Sprintf("[%s] %s", prefix, content)
			}

			allLines = append(allLines, timestampedLogLine{
				timestamp: ts,
				content:   mergedLine,
			})
		}
	}

	sort.SliceStable(allLines, func(i, j int) bool {
		return allLines[i].timestamp < allLines[j].timestamp
	})

	var resultLines []string
	for _, line := range allLines {
		if timestamps {
			resultLines = append(resultLines, line.content)
		} else {
			if len(line.content) > 31 && line.content[30] == ' ' {
				resultLines = append(resultLines, line.content[31:])
			} else {
				resultLines = append(resultLines, line.content)
			}
		}
	}

	return strings.Join(resultLines, "\n"), nil
}

// GetAllPodsLogsFromStart fetches the first N lines from multiple pods, merged by timestamp
func (c *Client) GetAllPodsLogsFromStart(namespace string, pods []PodContainerPair, allContainers bool, timestamps bool, previous bool, lineLimit int) (string, error) {
	allLogs, err := c.GetAllPodsLogsAll(namespace, pods, allContainers, timestamps, previous)
	if err != nil {
		return "", err
	}
	if lineLimit <= 0 {
		lineLimit = 200
	}
	lines := strings.Split(allLogs, "\n")
	if len(lines) <= lineLimit {
		return allLogs, nil
	}
	return strings.Join(lines[:lineLimit], "\n"), nil
}

// GetAllPodsLogsBefore fetches logs before a given timestamp from multiple pods
func (c *Client) GetAllPodsLogsBefore(namespace string, pods []PodContainerPair, allContainers bool, timestamps bool, previous bool, beforeTime string, lineLimit int) (string, bool, error) {
	allLogs, err := c.GetAllPodsLogsAll(namespace, pods, allContainers, true, previous)
	if err != nil {
		return "", false, err
	}
	if lineLimit <= 0 {
		lineLimit = 200
	}

	lines := strings.Split(allLogs, "\n")

	compareLen := 30
	if len(beforeTime) < compareLen {
		compareLen = len(beforeTime)
	}
	beforeTimePrefix := beforeTime[:compareLen]

	cutoffIndex := -1
	for i, line := range lines {
		if len(line) >= 30 {
			lineTime := line[:30]
			if lineTime >= beforeTimePrefix {
				cutoffIndex = i
				break
			}
		}
	}

	var resultLines []string
	hasMoreBefore := false

	if cutoffIndex == -1 {
		if len(lines) > lineLimit {
			resultLines = lines[len(lines)-lineLimit:]
			hasMoreBefore = true
		} else {
			resultLines = lines
		}
	} else if cutoffIndex == 0 {
		return "", false, nil
	} else {
		startIndex := cutoffIndex - lineLimit
		if startIndex < 0 {
			startIndex = 0
		} else {
			hasMoreBefore = true
		}
		resultLines = lines[startIndex:cutoffIndex]
	}

	if !timestamps {
		for i, line := range resultLines {
			if len(line) > 31 {
				resultLines[i] = line[31:]
			}
		}
	}

	return strings.Join(resultLines, "\n"), hasMoreBefore, nil
}

// GetAllPodsLogsAfter fetches logs after a given timestamp from multiple pods
func (c *Client) GetAllPodsLogsAfter(namespace string, pods []PodContainerPair, allContainers bool, timestamps bool, previous bool, afterTime string, lineLimit int) (string, bool, error) {
	allLogs, err := c.GetAllPodsLogsAll(namespace, pods, allContainers, true, previous)
	if err != nil {
		return "", false, err
	}
	if lineLimit <= 0 {
		lineLimit = 200
	}

	lines := strings.Split(allLogs, "\n")

	compareLen := 30
	if len(afterTime) < compareLen {
		compareLen = len(afterTime)
	}
	afterTimePrefix := afterTime[:compareLen]

	startIdx := 0
	for i, line := range lines {
		if len(line) >= 30 {
			lineTime := line[:30]
			if lineTime <= afterTimePrefix {
				startIdx = i + 1
				continue
			}
		}
		break
	}

	if startIdx >= len(lines) {
		return "", false, nil
	}

	lines = lines[startIdx:]

	hasMoreAfter := len(lines) > lineLimit
	if hasMoreAfter {
		lines = lines[:lineLimit]
	}

	if !timestamps {
		for i, line := range lines {
			if len(line) > 31 {
				lines[i] = line[31:]
			}
		}
	}

	return strings.Join(lines, "\n"), hasMoreAfter, nil
}

// StreamAllPodsLogs streams logs from multiple pods, merging them in real-time by timestamp.
func (c *Client) StreamAllPodsLogs(ctx context.Context, namespace string, pods []PodContainerPair, allContainers bool, timestamps bool, tailLines int64, onLine func(line string)) error {
	if len(pods) == 0 {
		return nil
	}

	type streamLine struct {
		timestamp string
		fullLine  string
	}

	lineChan := make(chan streamLine, 1000)
	var wg sync.WaitGroup
	errChan := make(chan error, len(pods)*10)

	for _, p := range pods {
		if allContainers && len(p.ContainerNames) > 1 {
			for _, cn := range p.ContainerNames {
				wg.Add(1)
				go func(podName, containerName string) {
					defer wg.Done()
					prefix := podName + "/" + containerName
					err := c.StreamPodLogs(ctx, namespace, podName, containerName, true, tailLines, func(line string) {
						var ts, content string
						if len(line) >= 31 && line[30] == ' ' {
							ts = line[:30]
							content = line[31:]
						} else {
							ts = ""
							content = line
						}

						var fullLine string
						if timestamps && ts != "" {
							fullLine = fmt.Sprintf("%s [%s] %s", ts, prefix, content)
						} else if ts != "" {
							fullLine = fmt.Sprintf("%s [%s] %s", ts, prefix, content)
						} else {
							fullLine = fmt.Sprintf("[%s] %s", prefix, content)
						}

						select {
						case lineChan <- streamLine{timestamp: ts, fullLine: fullLine}:
						case <-ctx.Done():
							return
						}
					})
					if err != nil && err != context.Canceled {
						errChan <- err
					}
				}(p.PodName, cn)
			}
		} else {
			containerName := ""
			if len(p.ContainerNames) > 0 {
				containerName = p.ContainerNames[0]
			}
			wg.Add(1)
			go func(podName, cn string) {
				defer wg.Done()
				prefix := podName
				err := c.StreamPodLogs(ctx, namespace, podName, cn, true, tailLines, func(line string) {
					var ts, content string
					if len(line) >= 31 && line[30] == ' ' {
						ts = line[:30]
						content = line[31:]
					} else {
						ts = ""
						content = line
					}

					var fullLine string
					if timestamps && ts != "" {
						fullLine = fmt.Sprintf("%s [%s] %s", ts, prefix, content)
					} else if ts != "" {
						fullLine = fmt.Sprintf("%s [%s] %s", ts, prefix, content)
					} else {
						fullLine = fmt.Sprintf("[%s] %s", prefix, content)
					}

					select {
					case lineChan <- streamLine{timestamp: ts, fullLine: fullLine}:
					case <-ctx.Done():
						return
					}
				})
				if err != nil && err != context.Canceled {
					errChan <- err
				}
			}(p.PodName, containerName)
		}
	}

	go func() {
		wg.Wait()
		close(lineChan)
		close(errChan)
	}()

	var buffer []streamLine
	flushTicker := time.NewTicker(50 * time.Millisecond)
	defer flushTicker.Stop()

	flushBuffer := func() {
		if len(buffer) == 0 {
			return
		}
		sort.SliceStable(buffer, func(i, j int) bool {
			return buffer[i].timestamp < buffer[j].timestamp
		})
		for _, line := range buffer {
			if timestamps {
				onLine(line.fullLine)
			} else {
				if len(line.fullLine) > 31 && line.fullLine[30] == ' ' {
					onLine(line.fullLine[31:])
				} else {
					onLine(line.fullLine)
				}
			}
		}
		buffer = buffer[:0]
	}

	for {
		select {
		case <-ctx.Done():
			flushBuffer()
			return ctx.Err()
		case line, ok := <-lineChan:
			if !ok {
				flushBuffer()
				for err := range errChan {
					if err != nil {
						return err
					}
				}
				return nil
			}
			buffer = append(buffer, line)
		case <-flushTicker.C:
			flushBuffer()
		}
	}
}

func (c *Client) getClientForContext(contextName string) (kubernetes.Interface, error) {
	c.mu.RLock()
	if contextName == "" || contextName == c.currentContext {
		defer c.mu.RUnlock()
		if c.clientset == nil {
			return nil, fmt.Errorf("k8s client not initialized")
		}
		return c.clientset, nil
	}
	c.mu.RUnlock()

	// Create a temporary config for the requested context
	// We don't want to lock here as loadConfig does, but we are creating a new config
	// entirely separate from the struct's state.
	// Actually, we can reuse the loading logic but we need to be careful not to modify c.configLoading
	// if it's shared.
	// Simplest way: create a new loader.

	home := homedir.HomeDir()
	kubeconfigPath := filepath.Join(home, ".kube", "config")
	loadingRules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath}
	configOverrides := &clientcmd.ConfigOverrides{CurrentContext: contextName}

	configLoader := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
	config, err := configLoader.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load client config for context %s: %w", contextName, err)
	}

	return kubernetes.NewForConfig(config)
}

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
func (c *Client) ListStatefulSets(contextName, namespace string) ([]appsv1.StatefulSet, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	statefulsets, err := cs.AppsV1().StatefulSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return statefulsets.Items, nil
}

// ListStatefulSetsWithContext lists statefulsets with cancellation support
func (c *Client) ListStatefulSetsWithContext(ctx context.Context, contextName, namespace string) ([]appsv1.StatefulSet, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	statefulsets, err := cs.AppsV1().StatefulSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		if isCancelledError(err) {
			return nil, ErrRequestCancelled
		}
		return nil, err
	}
	return statefulsets.Items, nil
}

func (c *Client) GetStatefulSetYaml(namespace, name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	statefulset, err := cs.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	// Remove managed fields
	statefulset.ManagedFields = nil

	yamlBytes, err := yaml.Marshal(statefulset)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateStatefulSetYaml(namespace, name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	var statefulset appsv1.StatefulSet
	if err := yaml.Unmarshal([]byte(yamlContent), &statefulset); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.AppsV1().StatefulSets(namespace).Update(ctx, &statefulset, metav1.UpdateOptions{})
	return err
}

func (c *Client) RestartStatefulSet(contextName, namespace, name string) error {
	fmt.Printf("Restarting statefulset: context=%s, ns=%s, name=%s\n", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()

	// Patch the statefulset to trigger a rollout
	patch := fmt.Sprintf(`{"spec":{"template":{"metadata":{"annotations":{"kubectl.kubernetes.io/restartedAt":"%s"}}}}}`, metav1.Now().String())
	_, err = cs.AppsV1().StatefulSets(namespace).Patch(ctx, name, types.StrategicMergePatchType, []byte(patch), metav1.PatchOptions{})
	return err
}

func (c *Client) ScaleStatefulSet(namespace, name string, replicas int32) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()

	statefulSet, err := cs.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get statefulset: %w", err)
	}

	statefulSet.Spec.Replicas = &replicas
	_, err = cs.AppsV1().StatefulSets(namespace).Update(ctx, statefulSet, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteStatefulSet(contextName, namespace, name string) error {
	log.Printf("Deleting statefulset: context=%s, ns=%s, name=%s", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return cs.AppsV1().StatefulSets(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

// DaemonSet operations
func (c *Client) ListDaemonSets(contextName, namespace string) ([]appsv1.DaemonSet, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	daemonsets, err := cs.AppsV1().DaemonSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return daemonsets.Items, nil
}

// ListDaemonSetsWithContext lists daemonsets with cancellation support
func (c *Client) ListDaemonSetsWithContext(ctx context.Context, contextName, namespace string) ([]appsv1.DaemonSet, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	daemonsets, err := cs.AppsV1().DaemonSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		if isCancelledError(err) {
			return nil, ErrRequestCancelled
		}
		return nil, err
	}
	return daemonsets.Items, nil
}

func (c *Client) GetDaemonSetYaml(namespace, name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	daemonset, err := cs.AppsV1().DaemonSets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	// Remove managed fields
	daemonset.ManagedFields = nil

	yamlBytes, err := yaml.Marshal(daemonset)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateDaemonSetYaml(namespace, name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	var daemonset appsv1.DaemonSet
	if err := yaml.Unmarshal([]byte(yamlContent), &daemonset); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.AppsV1().DaemonSets(namespace).Update(ctx, &daemonset, metav1.UpdateOptions{})
	return err
}

func (c *Client) RestartDaemonSet(contextName, namespace, name string) error {
	fmt.Printf("Restarting daemonset: context=%s, ns=%s, name=%s\n", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()

	// Patch the daemonset to trigger a rollout
	patch := fmt.Sprintf(`{"spec":{"template":{"metadata":{"annotations":{"kubectl.kubernetes.io/restartedAt":"%s"}}}}}`, metav1.Now().String())
	_, err = cs.AppsV1().DaemonSets(namespace).Patch(ctx, name, types.StrategicMergePatchType, []byte(patch), metav1.PatchOptions{})
	return err
}

func (c *Client) DeleteDaemonSet(contextName, namespace, name string) error {
	log.Printf("Deleting daemonset: context=%s, ns=%s, name=%s", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return cs.AppsV1().DaemonSets(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

// ReplicaSet operations
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
func (c *Client) ListJobs(contextName, namespace string) ([]batchv1.Job, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	jobs, err := cs.BatchV1().Jobs(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return jobs.Items, nil
}

// ListJobsWithContext lists jobs with cancellation support
func (c *Client) ListJobsWithContext(ctx context.Context, contextName, namespace string) ([]batchv1.Job, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	jobs, err := cs.BatchV1().Jobs(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		if isCancelledError(err) {
			return nil, ErrRequestCancelled
		}
		return nil, err
	}
	return jobs.Items, nil
}

func (c *Client) GetJobYaml(namespace, name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	job, err := cs.BatchV1().Jobs(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	yamlBytes, err := yaml.Marshal(job)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateJobYaml(namespace, name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	var job batchv1.Job
	if err := yaml.Unmarshal([]byte(yamlContent), &job); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.BatchV1().Jobs(namespace).Update(ctx, &job, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteJob(contextName, namespace, name string) error {
	log.Printf("Deleting job: context=%s, ns=%s, name=%s", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return cs.BatchV1().Jobs(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

// CronJob operations
func (c *Client) ListCronJobs(contextName, namespace string) ([]batchv1.CronJob, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	cronJobs, err := cs.BatchV1().CronJobs(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return cronJobs.Items, nil
}

// ListCronJobsWithContext lists cronjobs with cancellation support
func (c *Client) ListCronJobsWithContext(ctx context.Context, contextName, namespace string) ([]batchv1.CronJob, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	cronJobs, err := cs.BatchV1().CronJobs(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		if isCancelledError(err) {
			return nil, ErrRequestCancelled
		}
		return nil, err
	}
	return cronJobs.Items, nil
}

func (c *Client) GetCronJobYaml(namespace, name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	cronJob, err := cs.BatchV1().CronJobs(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	yamlBytes, err := yaml.Marshal(cronJob)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateCronJobYaml(namespace, name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	var cronJob batchv1.CronJob
	if err := yaml.Unmarshal([]byte(yamlContent), &cronJob); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.BatchV1().CronJobs(namespace).Update(ctx, &cronJob, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteCronJob(contextName, namespace, name string) error {
	log.Printf("Deleting cronjob: context=%s, ns=%s, name=%s", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return cs.BatchV1().CronJobs(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

func (c *Client) TriggerCronJob(contextName, namespace, cronJobName string) error {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()

	// Get the CronJob to use as template
	cronJob, err := cs.BatchV1().CronJobs(namespace).Get(ctx, cronJobName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get cronjob: %w", err)
	}

	// Create a Job from the CronJob spec
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: cronJobName + "-manual-",
			Namespace:    namespace,
			Annotations: map[string]string{
				"cronjob.kubernetes.io/instantiate": "manual",
			},
		},
		Spec: cronJob.Spec.JobTemplate.Spec,
	}

	_, err = cs.BatchV1().Jobs(namespace).Create(ctx, job, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create job: %w", err)
	}

	return nil
}

func (c *Client) SuspendCronJob(contextName, namespace, name string, suspend bool) error {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()

	// Use JSON patch to update only the suspend field
	patchData := fmt.Sprintf(`{"spec":{"suspend":%t}}`, suspend)

	result, err := cs.BatchV1().CronJobs(namespace).Patch(
		ctx,
		name,
		types.MergePatchType,
		[]byte(patchData),
		metav1.PatchOptions{},
	)
	if err != nil {
		return fmt.Errorf("failed to patch cronjob: %w", err)
	}

	_ = result
	return nil
}

// PersistentVolumeClaim operations
func (c *Client) ListPVCs(contextName, namespace string) ([]v1.PersistentVolumeClaim, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	pvcs, err := cs.CoreV1().PersistentVolumeClaims(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return pvcs.Items, nil
}

func (c *Client) ListPVCsWithContext(ctx context.Context, contextName, namespace string) ([]v1.PersistentVolumeClaim, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	pvcs, err := cs.CoreV1().PersistentVolumeClaims(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		if isCancelledError(err) {
			return nil, ErrRequestCancelled
		}
		return nil, err
	}
	return pvcs.Items, nil
}

func (c *Client) GetPVCYaml(namespace, name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	pvc, err := cs.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	pvc.ManagedFields = nil

	yamlBytes, err := yaml.Marshal(pvc)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdatePVCYaml(namespace, name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	var pvc v1.PersistentVolumeClaim
	if err := yaml.Unmarshal([]byte(yamlContent), &pvc); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.CoreV1().PersistentVolumeClaims(namespace).Update(ctx, &pvc, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeletePVC(contextName, namespace, name string) error {
	log.Printf("Deleting PVC: context=%s, ns=%s, name=%s", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return cs.CoreV1().PersistentVolumeClaims(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

func (c *Client) ResizePVC(contextName, namespace, name, newSize string) error {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()

	// Get current PVC
	pvc, err := cs.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get PVC: %w", err)
	}

	// Parse and validate new size
	newQuantity, err := resource.ParseQuantity(newSize)
	if err != nil {
		return fmt.Errorf("invalid size format: %w", err)
	}

	// Check that new size is larger than current
	currentSize := pvc.Spec.Resources.Requests[v1.ResourceStorage]
	if newQuantity.Cmp(currentSize) <= 0 {
		return fmt.Errorf("new size must be larger than current size (%s)", currentSize.String())
	}

	// Update the storage request
	pvc.Spec.Resources.Requests[v1.ResourceStorage] = newQuantity

	_, err = cs.CoreV1().PersistentVolumeClaims(namespace).Update(ctx, pvc, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to resize PVC: %w", err)
	}

	return nil
}

// PersistentVolume operations (cluster-scoped)
func (c *Client) ListPVs(contextName string) ([]v1.PersistentVolume, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	pvs, err := cs.CoreV1().PersistentVolumes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return pvs.Items, nil
}

func (c *Client) ListPVsWithContext(ctx context.Context, contextName string) ([]v1.PersistentVolume, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	pvs, err := cs.CoreV1().PersistentVolumes().List(ctx, metav1.ListOptions{})
	if err != nil {
		if isCancelledError(err) {
			return nil, ErrRequestCancelled
		}
		return nil, err
	}
	return pvs.Items, nil
}

func (c *Client) GetPVYaml(name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	pv, err := cs.CoreV1().PersistentVolumes().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	pv.ManagedFields = nil

	yamlBytes, err := yaml.Marshal(pv)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdatePVYaml(name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	var pv v1.PersistentVolume
	if err := yaml.Unmarshal([]byte(yamlContent), &pv); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.CoreV1().PersistentVolumes().Update(ctx, &pv, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeletePV(contextName, name string) error {
	log.Printf("Deleting PV: context=%s, name=%s", contextName, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return cs.CoreV1().PersistentVolumes().Delete(ctx, name, metav1.DeleteOptions{})
}

// StorageClass operations (cluster-scoped)
func (c *Client) GetStorageClass(contextName, name string) (*storagev1.StorageClass, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	sc, err := cs.StorageV1().StorageClasses().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return sc, nil
}

func (c *Client) ListStorageClasses(contextName string) ([]storagev1.StorageClass, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	scs, err := cs.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return scs.Items, nil
}

func (c *Client) ListStorageClassesWithContext(ctx context.Context, contextName string) ([]storagev1.StorageClass, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	scs, err := cs.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})
	if err != nil {
		if isCancelledError(err) {
			return nil, ErrRequestCancelled
		}
		return nil, err
	}
	return scs.Items, nil
}

func (c *Client) GetStorageClassYaml(name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	sc, err := cs.StorageV1().StorageClasses().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	sc.ManagedFields = nil

	yamlBytes, err := yaml.Marshal(sc)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateStorageClassYaml(name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	var sc storagev1.StorageClass
	if err := yaml.Unmarshal([]byte(yamlContent), &sc); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.StorageV1().StorageClasses().Update(ctx, &sc, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteStorageClass(contextName, name string) error {
	log.Printf("Deleting StorageClass: context=%s, name=%s", contextName, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return cs.StorageV1().StorageClasses().Delete(ctx, name, metav1.DeleteOptions{})
}

// getApiExtensionsClientForContext returns an apiextensions clientset for a given context
func (c *Client) getApiExtensionsClientForContext(contextName string) (*apiextensionsclientset.Clientset, error) {
	home := homedir.HomeDir()
	kubeconfigPath := filepath.Join(home, ".kube", "config")
	loadingRules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath}

	configOverrides := &clientcmd.ConfigOverrides{}
	if contextName != "" {
		configOverrides.CurrentContext = contextName
	}

	configLoader := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
	config, err := configLoader.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load client config for context %s: %w", contextName, err)
	}

	return apiextensionsclientset.NewForConfig(config)
}

// CustomResourceDefinition operations (cluster-scoped)
func (c *Client) ListCRDs(contextName string) ([]apiextensionsv1.CustomResourceDefinition, error) {
	cs, err := c.getApiExtensionsClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get apiextensions client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	crds, err := cs.ApiextensionsV1().CustomResourceDefinitions().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return crds.Items, nil
}

func (c *Client) GetCRDYaml(contextName, name string) (string, error) {
	cs, err := c.getApiExtensionsClientForContext(contextName)
	if err != nil {
		return "", fmt.Errorf("failed to get apiextensions client: %w", err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	crd, err := cs.ApiextensionsV1().CustomResourceDefinitions().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	crd.ManagedFields = nil

	yamlBytes, err := yaml.Marshal(crd)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateCRDYaml(contextName, name, yamlContent string) error {
	cs, err := c.getApiExtensionsClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get apiextensions client: %w", err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	var crd apiextensionsv1.CustomResourceDefinition
	if err := yaml.Unmarshal([]byte(yamlContent), &crd); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.ApiextensionsV1().CustomResourceDefinitions().Update(ctx, &crd, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteCRD(contextName, name string) error {
	log.Printf("Deleting CRD: context=%s, name=%s", contextName, name)
	cs, err := c.getApiExtensionsClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get apiextensions client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return cs.ApiextensionsV1().CustomResourceDefinitions().Delete(ctx, name, metav1.DeleteOptions{})
}

// PrinterColumn represents an additional printer column from a CRD
type PrinterColumn struct {
	Name        string `json:"name"`
	Type        string `json:"type"`        // string, integer, number, boolean, date
	JSONPath    string `json:"jsonPath"`    // JSONPath expression to extract value
	Description string `json:"description"` // Optional description
	Priority    int32  `json:"priority"`    // 0 = always show, higher = hide in narrow views
}

// GetCRDPrinterColumns returns the additional printer columns for a CRD
func (c *Client) GetCRDPrinterColumns(contextName, crdName string) ([]PrinterColumn, error) {
	cs, err := c.getApiExtensionsClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get apiextensions client: %w", err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()

	crd, err := cs.ApiextensionsV1().CustomResourceDefinitions().Get(ctx, crdName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get CRD %s: %w", crdName, err)
	}

	var columns []PrinterColumn

	// Find the served version's printer columns
	for _, version := range crd.Spec.Versions {
		if version.Served {
			for _, col := range version.AdditionalPrinterColumns {
				columns = append(columns, PrinterColumn{
					Name:        col.Name,
					Type:        col.Type,
					JSONPath:    col.JSONPath,
					Description: col.Description,
					Priority:    col.Priority,
				})
			}
			break // Use the first served version
		}
	}

	return columns, nil
}

// getDynamicClientForContext returns a dynamic client for a given context
func (c *Client) getDynamicClientForContext(contextName string) (dynamic.Interface, error) {
	// If context is empty, use the client's current context (not kubeconfig's default)
	if contextName == "" {
		c.mu.RLock()
		contextName = c.currentContext
		c.mu.RUnlock()
	}

	home := homedir.HomeDir()
	kubeconfigPath := filepath.Join(home, ".kube", "config")
	loadingRules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath}

	configOverrides := &clientcmd.ConfigOverrides{}
	if contextName != "" {
		configOverrides.CurrentContext = contextName
	}

	configLoader := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
	config, err := configLoader.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load client config for context %s: %w", contextName, err)
	}

	return dynamic.NewForConfig(config)
}

// CustomResourceInfo represents metadata about a custom resource instance
type CustomResourceInfo struct {
	Name              string `json:"name"`
	Namespace         string `json:"namespace,omitempty"`
	CreationTimestamp string `json:"creationTimestamp"`
	UID               string `json:"uid"`
}

// ListCustomResources lists instances of a custom resource
func (c *Client) ListCustomResources(contextName, group, version, resource, namespace string) ([]map[string]interface{}, error) {
	dc, err := c.getDynamicClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get dynamic client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()

	gvr := schema.GroupVersionResource{
		Group:    group,
		Version:  version,
		Resource: resource,
	}

	var list *unstructured.UnstructuredList
	if namespace != "" {
		list, err = dc.Resource(gvr).Namespace(namespace).List(ctx, metav1.ListOptions{})
	} else {
		list, err = dc.Resource(gvr).List(ctx, metav1.ListOptions{})
	}
	if err != nil {
		return nil, err
	}

	// Convert to slice of maps for easier JSON serialization
	result := make([]map[string]interface{}, len(list.Items))
	for i, item := range list.Items {
		result[i] = item.Object
	}
	return result, nil
}

// GetCustomResourceYaml gets a custom resource instance as YAML
func (c *Client) GetCustomResourceYaml(contextName, group, version, resource, namespace, name string) (string, error) {
	dc, err := c.getDynamicClientForContext(contextName)
	if err != nil {
		return "", fmt.Errorf("failed to get dynamic client: %w", err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()

	gvr := schema.GroupVersionResource{
		Group:    group,
		Version:  version,
		Resource: resource,
	}

	var obj *unstructured.Unstructured
	if namespace != "" {
		obj, err = dc.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	} else {
		obj, err = dc.Resource(gvr).Get(ctx, name, metav1.GetOptions{})
	}
	if err != nil {
		return "", err
	}

	// Remove managed fields
	unstructured.RemoveNestedField(obj.Object, "metadata", "managedFields")

	yamlBytes, err := yaml.Marshal(obj.Object)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

// UpdateCustomResourceYaml updates a custom resource instance from YAML
func (c *Client) UpdateCustomResourceYaml(contextName, group, version, resource, namespace, name, yamlContent string) error {
	dc, err := c.getDynamicClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get dynamic client: %w", err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()

	gvr := schema.GroupVersionResource{
		Group:    group,
		Version:  version,
		Resource: resource,
	}

	var obj map[string]interface{}
	if err := yaml.Unmarshal([]byte(yamlContent), &obj); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}

	unstructuredObj := &unstructured.Unstructured{Object: obj}

	if namespace != "" {
		_, err = dc.Resource(gvr).Namespace(namespace).Update(ctx, unstructuredObj, metav1.UpdateOptions{})
	} else {
		_, err = dc.Resource(gvr).Update(ctx, unstructuredObj, metav1.UpdateOptions{})
	}
	return err
}

// DeleteCustomResource deletes a custom resource instance
func (c *Client) DeleteCustomResource(contextName, group, version, resource, namespace, name string) error {
	log.Printf("Deleting custom resource: context=%s, gvr=%s/%s/%s, ns=%s, name=%s", contextName, group, version, resource, namespace, name)
	dc, err := c.getDynamicClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get dynamic client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()

	gvr := schema.GroupVersionResource{
		Group:    group,
		Version:  version,
		Resource: resource,
	}

	if namespace != "" {
		return dc.Resource(gvr).Namespace(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	}
	return dc.Resource(gvr).Delete(ctx, name, metav1.DeleteOptions{})
}

// --- Port Forwarding Support ---

// GetRestConfigForContext returns the REST config for a specific context
func (c *Client) GetRestConfigForContext(contextName string) (*rest.Config, error) {
	home := homedir.HomeDir()
	kubeconfigPath := filepath.Join(home, ".kube", "config")
	loadingRules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath}

	configOverrides := &clientcmd.ConfigOverrides{}
	if contextName != "" {
		configOverrides.CurrentContext = contextName
	}

	configLoader := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
	return configLoader.ClientConfig()
}

// GetServiceBackingPods finds running pods that back a service
func (c *Client) GetServiceBackingPods(contextName, namespace, serviceName string) ([]string, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get clientset for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()

	// Get the service
	svc, err := cs.CoreV1().Services(namespace).Get(ctx, serviceName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get service %s: %w", serviceName, err)
	}

	// Build label selector from service selector
	if len(svc.Spec.Selector) == 0 {
		return nil, fmt.Errorf("service %s has no selector", serviceName)
	}

	selectorParts := make([]string, 0, len(svc.Spec.Selector))
	for k, v := range svc.Spec.Selector {
		selectorParts = append(selectorParts, k+"="+v)
	}
	selector := strings.Join(selectorParts, ",")

	// Find pods matching selector
	pods, err := cs.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: selector,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	// Return names of running pods
	result := make([]string, 0, len(pods.Items))
	for _, pod := range pods.Items {
		if pod.Status.Phase == v1.PodRunning {
			result = append(result, pod.Name)
		}
	}

	return result, nil
}

// GetPodContainerPorts returns the container ports for a pod
func (c *Client) GetPodContainerPorts(contextName, namespace, podName string) ([]int32, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get clientset for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()

	pod, err := cs.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get pod %s: %w", podName, err)
	}

	// Calculate total ports for pre-allocation
	totalPorts := 0
	for _, container := range pod.Spec.Containers {
		totalPorts += len(container.Ports)
	}
	ports := make([]int32, 0, totalPorts)
	for _, container := range pod.Spec.Containers {
		for _, port := range container.Ports {
			ports = append(ports, port.ContainerPort)
		}
	}

	return ports, nil
}

// GetServicePorts returns the ports exposed by a service
func (c *Client) GetServicePorts(contextName, namespace, serviceName string) ([]int32, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get clientset for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()

	svc, err := cs.CoreV1().Services(namespace).Get(ctx, serviceName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get service %s: %w", serviceName, err)
	}

	ports := make([]int32, 0, len(svc.Spec.Ports))
	for _, port := range svc.Spec.Ports {
		ports = append(ports, port.Port)
	}

	return ports, nil
}

// ==================== RBAC / Access Control ====================

// ServiceAccount operations
func (c *Client) ListServiceAccounts(namespace string) ([]v1.ServiceAccount, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	list, err := cs.CoreV1().ServiceAccounts(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) ListServiceAccountsWithContext(ctx context.Context, namespace string) ([]v1.ServiceAccount, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	list, err := cs.CoreV1().ServiceAccounts(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		if isCancelledError(err) {
			return nil, ErrRequestCancelled
		}
		return nil, err
	}
	return list.Items, nil
}

// ListServiceAccountsForContext lists service accounts for a specific kubeconfig context
func (c *Client) ListServiceAccountsForContext(contextName, namespace string) ([]v1.ServiceAccount, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	list, err := cs.CoreV1().ServiceAccounts(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) GetServiceAccountYaml(namespace, name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	sa, err := cs.CoreV1().ServiceAccounts(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	sa.ManagedFields = nil
	yamlBytes, err := yaml.Marshal(sa)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateServiceAccountYaml(namespace, name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	var sa v1.ServiceAccount
	if err := yaml.Unmarshal([]byte(yamlContent), &sa); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.CoreV1().ServiceAccounts(namespace).Update(ctx, &sa, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteServiceAccount(contextName, namespace, name string) error {
	log.Printf("Deleting service account: context=%s, ns=%s, name=%s", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return cs.CoreV1().ServiceAccounts(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

// Role operations (namespaced)
func (c *Client) ListRoles(namespace string) ([]rbacv1.Role, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	list, err := cs.RbacV1().Roles(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) ListRolesWithContext(ctx context.Context, namespace string) ([]rbacv1.Role, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	list, err := cs.RbacV1().Roles(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		if isCancelledError(err) {
			return nil, ErrRequestCancelled
		}
		return nil, err
	}
	return list.Items, nil
}

// ListRolesForContext lists roles for a specific kubeconfig context
func (c *Client) ListRolesForContext(contextName, namespace string) ([]rbacv1.Role, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	list, err := cs.RbacV1().Roles(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) GetRoleYaml(namespace, name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	role, err := cs.RbacV1().Roles(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	role.ManagedFields = nil
	yamlBytes, err := yaml.Marshal(role)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateRoleYaml(namespace, name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	var role rbacv1.Role
	if err := yaml.Unmarshal([]byte(yamlContent), &role); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.RbacV1().Roles(namespace).Update(ctx, &role, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteRole(contextName, namespace, name string) error {
	log.Printf("Deleting role: context=%s, ns=%s, name=%s", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return cs.RbacV1().Roles(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

// ClusterRole operations (cluster-scoped)
func (c *Client) ListClusterRoles() ([]rbacv1.ClusterRole, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	list, err := cs.RbacV1().ClusterRoles().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

// ListClusterRolesForContext lists cluster roles for a specific kubeconfig context
func (c *Client) ListClusterRolesForContext(contextName string) ([]rbacv1.ClusterRole, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	list, err := cs.RbacV1().ClusterRoles().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) ListClusterRolesWithContext(ctx context.Context) ([]rbacv1.ClusterRole, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	list, err := cs.RbacV1().ClusterRoles().List(ctx, metav1.ListOptions{})
	if err != nil {
		if isCancelledError(err) {
			return nil, ErrRequestCancelled
		}
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) GetClusterRoleYaml(name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	role, err := cs.RbacV1().ClusterRoles().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	role.ManagedFields = nil
	yamlBytes, err := yaml.Marshal(role)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateClusterRoleYaml(name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	var role rbacv1.ClusterRole
	if err := yaml.Unmarshal([]byte(yamlContent), &role); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.RbacV1().ClusterRoles().Update(ctx, &role, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteClusterRole(contextName, name string) error {
	log.Printf("Deleting cluster role: context=%s, name=%s", contextName, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return cs.RbacV1().ClusterRoles().Delete(ctx, name, metav1.DeleteOptions{})
}

// RoleBinding operations (namespaced)
func (c *Client) ListRoleBindings(namespace string) ([]rbacv1.RoleBinding, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	list, err := cs.RbacV1().RoleBindings(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) ListRoleBindingsWithContext(ctx context.Context, namespace string) ([]rbacv1.RoleBinding, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	list, err := cs.RbacV1().RoleBindings(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		if isCancelledError(err) {
			return nil, ErrRequestCancelled
		}
		return nil, err
	}
	return list.Items, nil
}

// ListRoleBindingsForContext lists role bindings for a specific kubeconfig context
func (c *Client) ListRoleBindingsForContext(contextName, namespace string) ([]rbacv1.RoleBinding, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	list, err := cs.RbacV1().RoleBindings(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) GetRoleBindingYaml(namespace, name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	binding, err := cs.RbacV1().RoleBindings(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	binding.ManagedFields = nil
	yamlBytes, err := yaml.Marshal(binding)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateRoleBindingYaml(namespace, name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	var binding rbacv1.RoleBinding
	if err := yaml.Unmarshal([]byte(yamlContent), &binding); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.RbacV1().RoleBindings(namespace).Update(ctx, &binding, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteRoleBinding(contextName, namespace, name string) error {
	log.Printf("Deleting role binding: context=%s, ns=%s, name=%s", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return cs.RbacV1().RoleBindings(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

// ClusterRoleBinding operations (cluster-scoped)
func (c *Client) ListClusterRoleBindings() ([]rbacv1.ClusterRoleBinding, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	list, err := cs.RbacV1().ClusterRoleBindings().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

// ListClusterRoleBindingsForContext lists cluster role bindings for a specific kubeconfig context
func (c *Client) ListClusterRoleBindingsForContext(contextName string) ([]rbacv1.ClusterRoleBinding, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	list, err := cs.RbacV1().ClusterRoleBindings().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) ListClusterRoleBindingsWithContext(ctx context.Context) ([]rbacv1.ClusterRoleBinding, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	list, err := cs.RbacV1().ClusterRoleBindings().List(ctx, metav1.ListOptions{})
	if err != nil {
		if isCancelledError(err) {
			return nil, ErrRequestCancelled
		}
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) GetClusterRoleBindingYaml(name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	binding, err := cs.RbacV1().ClusterRoleBindings().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	binding.ManagedFields = nil
	yamlBytes, err := yaml.Marshal(binding)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateClusterRoleBindingYaml(name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	var binding rbacv1.ClusterRoleBinding
	if err := yaml.Unmarshal([]byte(yamlContent), &binding); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.RbacV1().ClusterRoleBindings().Update(ctx, &binding, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteClusterRoleBinding(contextName, name string) error {
	log.Printf("Deleting cluster role binding: context=%s, name=%s", contextName, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return cs.RbacV1().ClusterRoleBindings().Delete(ctx, name, metav1.DeleteOptions{})
}

// NetworkPolicy operations (namespaced)
func (c *Client) ListNetworkPolicies(namespace string) ([]networkingv1.NetworkPolicy, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	list, err := cs.NetworkingV1().NetworkPolicies(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

// ListNetworkPoliciesForContext lists network policies for a specific kubeconfig context
func (c *Client) ListNetworkPoliciesForContext(contextName, namespace string) ([]networkingv1.NetworkPolicy, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	list, err := cs.NetworkingV1().NetworkPolicies(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) ListNetworkPoliciesWithContext(ctx context.Context, namespace string) ([]networkingv1.NetworkPolicy, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	list, err := cs.NetworkingV1().NetworkPolicies(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		if isCancelledError(err) {
			return nil, ErrRequestCancelled
		}
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) GetNetworkPolicyYaml(namespace, name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	policy, err := cs.NetworkingV1().NetworkPolicies(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	policy.ManagedFields = nil
	yamlBytes, err := yaml.Marshal(policy)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateNetworkPolicyYaml(namespace, name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	var policy networkingv1.NetworkPolicy
	if err := yaml.Unmarshal([]byte(yamlContent), &policy); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.NetworkingV1().NetworkPolicies(namespace).Update(ctx, &policy, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteNetworkPolicy(contextName, namespace, name string) error {
	log.Printf("Deleting network policy: context=%s, ns=%s, name=%s", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return cs.NetworkingV1().NetworkPolicies(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

// HorizontalPodAutoscaler operations (namespaced)
func (c *Client) ListHPAs(namespace string) ([]autoscalingv2.HorizontalPodAutoscaler, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	list, err := cs.AutoscalingV2().HorizontalPodAutoscalers(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) ListHPAsWithContext(ctx context.Context, namespace string) ([]autoscalingv2.HorizontalPodAutoscaler, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	list, err := cs.AutoscalingV2().HorizontalPodAutoscalers(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		if isCancelledError(err) {
			return nil, ErrRequestCancelled
		}
		return nil, err
	}
	return list.Items, nil
}

// ListHPAsForContext lists HPAs for a specific kubeconfig context
func (c *Client) ListHPAsForContext(contextName, namespace string) ([]autoscalingv2.HorizontalPodAutoscaler, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	list, err := cs.AutoscalingV2().HorizontalPodAutoscalers(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) GetHPAYaml(namespace, name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	hpa, err := cs.AutoscalingV2().HorizontalPodAutoscalers(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	hpa.ManagedFields = nil
	yamlBytes, err := yaml.Marshal(hpa)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateHPAYaml(namespace, name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	var hpa autoscalingv2.HorizontalPodAutoscaler
	if err := yaml.Unmarshal([]byte(yamlContent), &hpa); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.AutoscalingV2().HorizontalPodAutoscalers(namespace).Update(ctx, &hpa, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteHPA(contextName, namespace, name string) error {
	log.Printf("Deleting HPA: context=%s, ns=%s, name=%s", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return cs.AutoscalingV2().HorizontalPodAutoscalers(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

// PodDisruptionBudget operations (namespaced)
func (c *Client) ListPDBs(namespace string) ([]policyv1.PodDisruptionBudget, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	list, err := cs.PolicyV1().PodDisruptionBudgets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) ListPDBsWithContext(ctx context.Context, namespace string) ([]policyv1.PodDisruptionBudget, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	list, err := cs.PolicyV1().PodDisruptionBudgets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		if isCancelledError(err) {
			return nil, ErrRequestCancelled
		}
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) GetPDBYaml(namespace, name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	pdb, err := cs.PolicyV1().PodDisruptionBudgets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	pdb.ManagedFields = nil
	yamlBytes, err := yaml.Marshal(pdb)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdatePDBYaml(namespace, name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	var pdb policyv1.PodDisruptionBudget
	if err := yaml.Unmarshal([]byte(yamlContent), &pdb); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.PolicyV1().PodDisruptionBudgets(namespace).Update(ctx, &pdb, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeletePDB(contextName, namespace, name string) error {
	log.Printf("Deleting PDB: context=%s, ns=%s, name=%s", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return cs.PolicyV1().PodDisruptionBudgets(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

// ResourceQuota operations (namespaced)
func (c *Client) ListResourceQuotas(namespace string) ([]v1.ResourceQuota, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	list, err := cs.CoreV1().ResourceQuotas(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) ListResourceQuotasWithContext(ctx context.Context, namespace string) ([]v1.ResourceQuota, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	list, err := cs.CoreV1().ResourceQuotas(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		if isCancelledError(err) {
			return nil, ErrRequestCancelled
		}
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) GetResourceQuotaYaml(namespace, name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	quota, err := cs.CoreV1().ResourceQuotas(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	quota.ManagedFields = nil
	yamlBytes, err := yaml.Marshal(quota)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateResourceQuotaYaml(namespace, name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	var quota v1.ResourceQuota
	if err := yaml.Unmarshal([]byte(yamlContent), &quota); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.CoreV1().ResourceQuotas(namespace).Update(ctx, &quota, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteResourceQuota(contextName, namespace, name string) error {
	log.Printf("Deleting resource quota: context=%s, ns=%s, name=%s", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return cs.CoreV1().ResourceQuotas(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

// LimitRange operations (namespaced)
func (c *Client) ListLimitRanges(namespace string) ([]v1.LimitRange, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	list, err := cs.CoreV1().LimitRanges(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) ListLimitRangesWithContext(ctx context.Context, namespace string) ([]v1.LimitRange, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	list, err := cs.CoreV1().LimitRanges(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		if isCancelledError(err) {
			return nil, ErrRequestCancelled
		}
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) GetLimitRangeYaml(namespace, name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	lr, err := cs.CoreV1().LimitRanges(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	lr.ManagedFields = nil
	yamlBytes, err := yaml.Marshal(lr)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateLimitRangeYaml(namespace, name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	var lr v1.LimitRange
	if err := yaml.Unmarshal([]byte(yamlContent), &lr); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.CoreV1().LimitRanges(namespace).Update(ctx, &lr, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteLimitRange(contextName, namespace, name string) error {
	log.Printf("Deleting limit range: context=%s, ns=%s, name=%s", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return cs.CoreV1().LimitRanges(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

// Endpoints operations (namespaced)
func (c *Client) ListEndpoints(namespace string) ([]v1.Endpoints, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	list, err := cs.CoreV1().Endpoints(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) ListEndpointsWithContext(ctx context.Context, namespace string) ([]v1.Endpoints, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	list, err := cs.CoreV1().Endpoints(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		if isCancelledError(err) {
			return nil, ErrRequestCancelled
		}
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) GetEndpointsYaml(namespace, name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	ep, err := cs.CoreV1().Endpoints(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	ep.ManagedFields = nil
	yamlBytes, err := yaml.Marshal(ep)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateEndpointsYaml(namespace, name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	var ep v1.Endpoints
	if err := yaml.Unmarshal([]byte(yamlContent), &ep); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.CoreV1().Endpoints(namespace).Update(ctx, &ep, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteEndpoints(contextName, namespace, name string) error {
	log.Printf("Deleting endpoints: context=%s, ns=%s, name=%s", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return cs.CoreV1().Endpoints(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

// EndpointSlice operations (namespaced, discovery.k8s.io/v1)
func (c *Client) ListEndpointSlices(namespace string) ([]discoveryv1.EndpointSlice, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	list, err := cs.DiscoveryV1().EndpointSlices(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) ListEndpointSlicesWithContext(ctx context.Context, namespace string) ([]discoveryv1.EndpointSlice, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	list, err := cs.DiscoveryV1().EndpointSlices(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		if isCancelledError(err) {
			return nil, ErrRequestCancelled
		}
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) GetEndpointSliceYaml(namespace, name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	eps, err := cs.DiscoveryV1().EndpointSlices(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	eps.ManagedFields = nil
	yamlBytes, err := yaml.Marshal(eps)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateEndpointSliceYaml(namespace, name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	var eps discoveryv1.EndpointSlice
	if err := yaml.Unmarshal([]byte(yamlContent), &eps); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.DiscoveryV1().EndpointSlices(namespace).Update(ctx, &eps, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteEndpointSlice(contextName, namespace, name string) error {
	log.Printf("Deleting endpointslice: context=%s, ns=%s, name=%s", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return cs.DiscoveryV1().EndpointSlices(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

// ValidatingWebhookConfiguration operations (cluster-scoped)
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
func (c *Client) ListPriorityClasses() ([]schedulingv1.PriorityClass, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	list, err := cs.SchedulingV1().PriorityClasses().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) ListPriorityClassesWithContext(ctx context.Context) ([]schedulingv1.PriorityClass, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	list, err := cs.SchedulingV1().PriorityClasses().List(ctx, metav1.ListOptions{})
	if err != nil {
		if isCancelledError(err) {
			return nil, ErrRequestCancelled
		}
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) GetPriorityClassYaml(name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	pc, err := cs.SchedulingV1().PriorityClasses().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	pc.ManagedFields = nil
	yamlBytes, err := yaml.Marshal(pc)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdatePriorityClassYaml(name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	var pc schedulingv1.PriorityClass
	if err := yaml.Unmarshal([]byte(yamlContent), &pc); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.SchedulingV1().PriorityClasses().Update(ctx, &pc, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeletePriorityClass(contextName, name string) error {
	log.Printf("Deleting priority class: context=%s, name=%s", contextName, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return cs.SchedulingV1().PriorityClasses().Delete(ctx, name, metav1.DeleteOptions{})
}

// ============================================================================
// Leases (coordination.k8s.io/v1) - Namespaced
// ============================================================================

func (c *Client) ListLeases(contextName, namespace string) ([]coordinationv1.Lease, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	list, err := cs.CoordinationV1().Leases(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) ListLeasesWithContext(ctx context.Context, contextName, namespace string) ([]coordinationv1.Lease, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	list, err := cs.CoordinationV1().Leases(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		if isCancelledError(err) {
			return nil, ErrRequestCancelled
		}
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) GetLeaseYaml(contextName, namespace, name string) (string, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return "", fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	lease, err := cs.CoordinationV1().Leases(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	lease.ManagedFields = nil
	yamlBytes, err := yaml.Marshal(lease)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateLeaseYaml(contextName, namespace, name, yamlContent string) error {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	var lease coordinationv1.Lease
	if err := yaml.Unmarshal([]byte(yamlContent), &lease); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.CoordinationV1().Leases(namespace).Update(ctx, &lease, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteLease(contextName, namespace, name string) error {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return cs.CoordinationV1().Leases(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

// ============================================================================
// CSIDrivers (storage.k8s.io/v1) - Cluster-scoped
// ============================================================================

func (c *Client) ListCSIDrivers(contextName string) ([]storagev1.CSIDriver, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	list, err := cs.StorageV1().CSIDrivers().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) ListCSIDriversWithContext(ctx context.Context, contextName string) ([]storagev1.CSIDriver, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	list, err := cs.StorageV1().CSIDrivers().List(ctx, metav1.ListOptions{})
	if err != nil {
		if isCancelledError(err) {
			return nil, ErrRequestCancelled
		}
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) GetCSIDriverYaml(contextName, name string) (string, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return "", fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	driver, err := cs.StorageV1().CSIDrivers().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	driver.ManagedFields = nil
	yamlBytes, err := yaml.Marshal(driver)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateCSIDriverYaml(contextName, name, yamlContent string) error {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	var driver storagev1.CSIDriver
	if err := yaml.Unmarshal([]byte(yamlContent), &driver); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.StorageV1().CSIDrivers().Update(ctx, &driver, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteCSIDriver(contextName, name string) error {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return cs.StorageV1().CSIDrivers().Delete(ctx, name, metav1.DeleteOptions{})
}

// ============================================================================
// CSINodes (storage.k8s.io/v1) - Cluster-scoped
// ============================================================================

func (c *Client) ListCSINodes(contextName string) ([]storagev1.CSINode, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	list, err := cs.StorageV1().CSINodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) ListCSINodesWithContext(ctx context.Context, contextName string) ([]storagev1.CSINode, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	list, err := cs.StorageV1().CSINodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		if isCancelledError(err) {
			return nil, ErrRequestCancelled
		}
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) GetCSINodeYaml(contextName, name string) (string, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return "", fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	node, err := cs.StorageV1().CSINodes().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	node.ManagedFields = nil
	yamlBytes, err := yaml.Marshal(node)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateCSINodeYaml(contextName, name, yamlContent string) error {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	var node storagev1.CSINode
	if err := yaml.Unmarshal([]byte(yamlContent), &node); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.StorageV1().CSINodes().Update(ctx, &node, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteCSINode(contextName, name string) error {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return cs.StorageV1().CSINodes().Delete(ctx, name, metav1.DeleteOptions{})
}

// ============================================================================
// Generic Resource Creation from YAML
// ============================================================================

// kindToResource maps Kubernetes kinds to their plural resource names
var kindToResource = map[string]string{
	"Pod":                            "pods",
	"Deployment":                     "deployments",
	"StatefulSet":                    "statefulsets",
	"DaemonSet":                      "daemonsets",
	"ReplicaSet":                     "replicasets",
	"Job":                            "jobs",
	"CronJob":                        "cronjobs",
	"Service":                        "services",
	"Ingress":                        "ingresses",
	"ConfigMap":                      "configmaps",
	"Secret":                         "secrets",
	"PersistentVolumeClaim":          "persistentvolumeclaims",
	"PersistentVolume":               "persistentvolumes",
	"StorageClass":                   "storageclasses",
	"ServiceAccount":                 "serviceaccounts",
	"Role":                           "roles",
	"ClusterRole":                    "clusterroles",
	"RoleBinding":                    "rolebindings",
	"ClusterRoleBinding":             "clusterrolebindings",
	"NetworkPolicy":                  "networkpolicies",
	"Namespace":                      "namespaces",
	"Node":                           "nodes",
	"Endpoints":                      "endpoints",
	"EndpointSlice":                  "endpointslices",
	"HorizontalPodAutoscaler":        "horizontalpodautoscalers",
	"PodDisruptionBudget":            "poddisruptionbudgets",
	"ResourceQuota":                  "resourcequotas",
	"LimitRange":                     "limitranges",
	"ValidatingWebhookConfiguration": "validatingwebhookconfigurations",
	"MutatingWebhookConfiguration":   "mutatingwebhookconfigurations",
	"PriorityClass":                  "priorityclasses",
	"Lease":                          "leases",
	"CSIDriver":                      "csidrivers",
	"CSINode":                        "csinodes",
	"IngressClass":                   "ingressclasses",
}

// ApplyYAML creates a resource from YAML content using the dynamic client
func (c *Client) ApplyYAML(contextName, yamlContent string) error {
	dc, err := c.getDynamicClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get dynamic client: %w", err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()

	// Parse YAML into unstructured object
	var obj map[string]interface{}
	if err := yaml.Unmarshal([]byte(yamlContent), &obj); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}

	unstructuredObj := &unstructured.Unstructured{Object: obj}

	// Extract apiVersion and kind
	apiVersion := unstructuredObj.GetAPIVersion()
	kind := unstructuredObj.GetKind()
	namespace := unstructuredObj.GetNamespace()

	if apiVersion == "" || kind == "" {
		return fmt.Errorf("YAML must contain apiVersion and kind")
	}

	// Parse apiVersion into group and version
	var group, version string
	if g, v, found := strings.Cut(apiVersion, "/"); found {
		group = g
		version = v
	} else {
		group = ""
		version = apiVersion
	}

	// Get resource name (plural form)
	resource, ok := kindToResource[kind]
	if !ok {
		// Fallback: lowercase the kind and add 's'
		resource = strings.ToLower(kind) + "s"
	}

	gvr := schema.GroupVersionResource{
		Group:    group,
		Version:  version,
		Resource: resource,
	}

	// Create the resource
	if namespace != "" {
		_, err = dc.Resource(gvr).Namespace(namespace).Create(ctx, unstructuredObj, metav1.CreateOptions{})
	} else {
		_, err = dc.Resource(gvr).Create(ctx, unstructuredObj, metav1.CreateOptions{})
	}

	if err != nil {
		return fmt.Errorf("failed to create %s: %w", kind, err)
	}

	return nil
}

// ============================================================================
// Prometheus Integration
// ============================================================================

// PrometheusInfo contains detected Prometheus endpoint information
type PrometheusInfo struct {
	Available       bool   `json:"available"`
	Namespace       string `json:"namespace"`
	Service         string `json:"service"`
	Port            int    `json:"port"`
	DetectionMethod string `json:"detectionMethod,omitempty"` // "crd", "service", "manual"
	CRDName         string `json:"crdName,omitempty"`         // Name of the Prometheus CR if detected via CRD
}

// PrometheusInstall represents a discovered Prometheus installation
type PrometheusInstall struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`    // CR name or service name
	Service   string `json:"service"` // The service to connect to
	Port      int    `json:"port"`
	Type      string `json:"type"`      // "operator" (CRD-based) or "standalone"
	Reachable bool   `json:"reachable"` // Whether we can connect to it
}

// PrometheusQueryResult represents the result of a Prometheus query
type PrometheusQueryResult struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric map[string]string `json:"metric"`
			Value  []interface{}     `json:"value"`  // [timestamp, value]
			Values [][]interface{}   `json:"values"` // for range queries
		} `json:"result"`
	} `json:"data"`
	Error     string `json:"error,omitempty"`
	ErrorType string `json:"errorType,omitempty"`
}

// MetricsDataPoint represents a single data point in time series
type MetricsDataPoint struct {
	Timestamp int64   `json:"timestamp"`
	Value     float64 `json:"value"`
}

// LifecycleMarker represents a K8s lifecycle event to overlay on metrics charts
type LifecycleMarker struct {
	Timestamp int64  `json:"timestamp"` // Unix ms (matches MetricsDataPoint)
	Reason    string `json:"reason"`    // "OOMKilling", "BackOff", etc.
	Severity  string `json:"severity"`  // "error" | "warning" | "info"
	Message   string `json:"message"`
	Kind      string `json:"kind"` // "Pod", "Deployment", etc.
}

// ContainerMetricsHistory holds historical metrics for a container
type ContainerMetricsHistory struct {
	Container string             `json:"container"`
	CPU       []MetricsDataPoint `json:"cpu"`    // millicores
	Memory    []MetricsDataPoint `json:"memory"` // bytes
}

// PodMetricsHistory holds historical metrics for a pod
type PodMetricsHistory struct {
	Namespace  string                    `json:"namespace"`
	Pod        string                    `json:"pod"`
	Containers []ContainerMetricsHistory `json:"containers"`
	Network    *NetworkMetrics           `json:"network"`
}

// DetectPrometheus tries to find a Prometheus installation in the cluster
func (c *Client) DetectPrometheus(contextName string) (*PrometheusInfo, error) {
	cs, err := c.getClientsetForContext(contextName)
	if err != nil {
		return &PrometheusInfo{Available: false}, err
	}

	ctx, cancel := c.contextWithTimeout()
	defer cancel()

	// First, try to detect via Prometheus Operator CRDs (most reliable)
	if info := c.detectPrometheusViaCRD(contextName); info != nil {
		return info, nil
	}

	// Fallback: Known Prometheus service patterns to check
	patterns := []struct {
		namespace string
		service   string
		port      int
	}{
		{"monitoring", "prometheus-operated", 9090},
		{"monitoring", "prometheus-server", 80},
		{"monitoring", "prometheus", 9090},
		{"monitoring", "kube-prometheus-stack-prometheus", 9090},
		{"prometheus", "prometheus-operated", 9090},
		{"prometheus", "prometheus-server", 80},
		{"prometheus", "prometheus", 9090},
		{"observability", "prometheus", 9090},
		{"default", "prometheus", 9090},
	}

	for _, p := range patterns {
		svc, err := cs.CoreV1().Services(p.namespace).Get(ctx, p.service, metav1.GetOptions{})
		if err == nil && svc != nil {
			// Verify it's actually reachable by checking if we can hit /api/v1/status/config
			info := &PrometheusInfo{
				Available:       true,
				Namespace:       p.namespace,
				Service:         p.service,
				Port:            p.port,
				DetectionMethod: "service",
			}
			// Test connection
			if c.testPrometheusConnection(contextName, info) {
				return info, nil
			}
		}
	}

	// Try finding by label selector across all namespaces
	allNsList, err := cs.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, ns := range allNsList.Items {
			svcs, err := cs.CoreV1().Services(ns.Name).List(ctx, metav1.ListOptions{
				LabelSelector: "app.kubernetes.io/name=prometheus",
			})
			if err == nil && len(svcs.Items) > 0 {
				svc := svcs.Items[0]
				port := 9090
				for _, p := range svc.Spec.Ports {
					if p.Name == "http" || p.Name == "web" || p.Name == "http-web" {
						port = int(p.Port)
						break
					}
				}
				info := &PrometheusInfo{
					Available:       true,
					Namespace:       ns.Name,
					Service:         svc.Name,
					Port:            port,
					DetectionMethod: "service",
				}
				if c.testPrometheusConnection(contextName, info) {
					return info, nil
				}
			}
		}
	}

	return &PrometheusInfo{Available: false}, nil
}

// detectPrometheusViaCRD checks for Prometheus Operator CRDs and lists Prometheus CRs
func (c *Client) detectPrometheusViaCRD(contextName string) *PrometheusInfo {
	dc, err := c.getDynamicClientForContext(contextName)
	if err != nil {
		fmt.Printf("[detectPrometheusViaCRD] Failed to get dynamic client: %v\n", err)
		return nil
	}

	cs, err := c.getClientsetForContext(contextName)
	if err != nil {
		fmt.Printf("[detectPrometheusViaCRD] Failed to get clientset: %v\n", err)
		return nil
	}

	ctx, cancel := c.contextWithTimeout()
	defer cancel()

	// Check if prometheuses.monitoring.coreos.com CRD exists
	gvr := schema.GroupVersionResource{
		Group:    "monitoring.coreos.com",
		Version:  "v1",
		Resource: "prometheuses",
	}

	// List all Prometheus CRs across all namespaces
	list, err := dc.Resource(gvr).List(ctx, metav1.ListOptions{})
	if err != nil {
		fmt.Printf("[detectPrometheusViaCRD] Failed to list Prometheus CRs: %v\n", err)
		return nil
	}

	fmt.Printf("[detectPrometheusViaCRD] Found %d Prometheus CRs\n", len(list.Items))

	// Collect all candidates first, then test connections
	var candidates []*PrometheusInfo

	// For each Prometheus CR, find the corresponding service
	for _, item := range list.Items {
		namespace := item.GetNamespace()
		name := item.GetName()
		fmt.Printf("[detectPrometheusViaCRD] Processing CR: %s/%s\n", namespace, name)

		// Prometheus Operator creates a service named "<prometheus-name>-operated"
		// or sometimes just "prometheus-operated"
		serviceNames := []string{
			name + "-operated",
			"prometheus-operated",
			name,
		}

		for _, svcName := range serviceNames {
			svc, err := cs.CoreV1().Services(namespace).Get(ctx, svcName, metav1.GetOptions{})
			if err != nil {
				continue
			}
			if svc == nil {
				continue
			}

			fmt.Printf("[detectPrometheusViaCRD] Found service: %s/%s\n", namespace, svcName)

			// Find the right port - check multiple port name patterns
			port := 9090
			for _, p := range svc.Spec.Ports {
				portName := strings.ToLower(p.Name)
				if portName == "http-web" || portName == "web" || portName == "http" || portName == "prometheus" {
					port = int(p.Port)
					fmt.Printf("[detectPrometheusViaCRD] Using port %d (name: %s)\n", port, p.Name)
					break
				}
			}
			// If no named port found, use the first port
			if port == 9090 && len(svc.Spec.Ports) > 0 {
				port = int(svc.Spec.Ports[0].Port)
				fmt.Printf("[detectPrometheusViaCRD] Using first port: %d\n", port)
			}

			info := &PrometheusInfo{
				Available:       true,
				Namespace:       namespace,
				Service:         svcName,
				Port:            port,
				DetectionMethod: "crd",
				CRDName:         name,
			}
			candidates = append(candidates, info)
			break // Found a service for this CR, move to next CR
		}
	}

	// Test connections for all candidates
	for _, info := range candidates {
		fmt.Printf("[detectPrometheusViaCRD] Testing connection to %s/%s:%d\n", info.Namespace, info.Service, info.Port)
		if c.testPrometheusConnection(contextName, info) {
			fmt.Printf("[detectPrometheusViaCRD] Connection successful!\n")
			return info
		}
		fmt.Printf("[detectPrometheusViaCRD] Connection failed\n")
	}

	// If we found candidates but none were reachable, return the first one anyway
	// so the user at least knows we found something
	if len(candidates) > 0 {
		fmt.Printf("[detectPrometheusViaCRD] Returning first candidate (not reachable but found)\n")
		candidates[0].Available = false // Mark as not reachable
		return candidates[0]
	}

	return nil
}

// ListPrometheusInstalls returns all Prometheus installations found in the cluster
// This is a fast version that doesn't test connections (use TestPrometheusEndpoint for that)
func (c *Client) ListPrometheusInstalls(contextName string) ([]PrometheusInstall, error) {
	var installs []PrometheusInstall

	dc, err := c.getDynamicClientForContext(contextName)
	if err != nil {
		return nil, err
	}

	cs, err := c.getClientsetForContext(contextName)
	if err != nil {
		return nil, err
	}

	// Use a timeout context to prevent hanging
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 1. Check for Prometheus Operator CRs (preferred method)
	gvr := schema.GroupVersionResource{
		Group:    "monitoring.coreos.com",
		Version:  "v1",
		Resource: "prometheuses",
	}

	list, err := dc.Resource(gvr).List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, item := range list.Items {
			namespace := item.GetNamespace()
			name := item.GetName()

			// Find the service
			serviceNames := []string{name + "-operated", "prometheus-operated", name}
			for _, svcName := range serviceNames {
				svc, err := cs.CoreV1().Services(namespace).Get(ctx, svcName, metav1.GetOptions{})
				if err == nil && svc != nil {
					port := 9090
					for _, p := range svc.Spec.Ports {
						portName := strings.ToLower(p.Name)
						if portName == "http-web" || portName == "web" || portName == "http" || portName == "prometheus" {
							port = int(p.Port)
							break
						}
					}
					if port == 9090 && len(svc.Spec.Ports) > 0 {
						port = int(svc.Spec.Ports[0].Port)
					}

					installs = append(installs, PrometheusInstall{
						Namespace: namespace,
						Name:      name,
						Service:   svcName,
						Port:      port,
						Type:      "operator",
						Reachable: true, // Assume reachable if service exists, user can test manually
					})
					break
				}
			}
		}
	}

	// 2. If we found CRD-based installs, skip the slow namespace scan
	if len(installs) > 0 {
		return installs, nil
	}

	// 3. Fallback: Check known namespaces for standalone Prometheus (faster than all namespaces)
	knownNamespaces := []string{"monitoring", "prometheus", "observability", "kube-system", "default"}
	for _, ns := range knownNamespaces {
		svcs, err := cs.CoreV1().Services(ns).List(ctx, metav1.ListOptions{
			LabelSelector: "app.kubernetes.io/name=prometheus",
		})
		if err != nil {
			continue
		}
		for _, svc := range svcs.Items {
			port := 9090
			for _, p := range svc.Spec.Ports {
				portName := strings.ToLower(p.Name)
				if portName == "http" || portName == "web" || portName == "http-web" || portName == "prometheus" {
					port = int(p.Port)
					break
				}
			}
			if port == 9090 && len(svc.Spec.Ports) > 0 {
				port = int(svc.Spec.Ports[0].Port)
			}

			installs = append(installs, PrometheusInstall{
				Namespace: ns,
				Name:      svc.Name,
				Service:   svc.Name,
				Port:      port,
				Type:      "standalone",
				Reachable: true, // Assume reachable, user can test manually
			})
		}
	}

	return installs, nil
}

// testPrometheusConnection tests if Prometheus is reachable via API proxy
func (c *Client) testPrometheusConnection(contextName string, info *PrometheusInfo) bool {
	_, err := c.queryPrometheusRaw(contextName, info, "api/v1/status/config", nil)
	if err != nil {
		fmt.Printf("[testPrometheusConnection] Failed for %s/%s:%d - %v\n", info.Namespace, info.Service, info.Port, err)
	}
	return err == nil
}

// queryPrometheusRaw makes a raw query to Prometheus via K8s API proxy
func (c *Client) queryPrometheusRaw(contextName string, info *PrometheusInfo, path string, params map[string]string) ([]byte, error) {
	return c.queryPrometheusRawWithContext(context.Background(), contextName, info, path, params)
}

// queryPrometheusRawWithContext makes a raw query to Prometheus with cancellation support
func (c *Client) queryPrometheusRawWithContext(ctx context.Context, contextName string, info *PrometheusInfo, path string, params map[string]string) ([]byte, error) {
	cs, err := c.getClientsetForContext(contextName)
	if err != nil {
		return nil, err
	}

	// Build request via K8s API proxy
	req := cs.CoreV1().RESTClient().Get().
		Namespace(info.Namespace).
		Resource("services").
		Name(fmt.Sprintf("%s:%d", info.Service, info.Port)).
		SubResource("proxy").
		Suffix(path)

	for k, v := range params {
		req = req.Param(k, v)
	}

	result, err := req.DoRaw(ctx)
	if err != nil {
		return nil, fmt.Errorf("prometheus query failed: %w", err)
	}

	return result, nil
}

// QueryPrometheus executes an instant query against Prometheus
func (c *Client) QueryPrometheus(contextName string, info *PrometheusInfo, query string) (*PrometheusQueryResult, error) {
	return c.QueryPrometheusWithContext(context.Background(), contextName, info, query)
}

// QueryPrometheusWithContext executes an instant query against Prometheus with cancellation support
func (c *Client) QueryPrometheusWithContext(ctx context.Context, contextName string, info *PrometheusInfo, query string) (*PrometheusQueryResult, error) {
	params := map[string]string{
		"query": query,
	}

	data, err := c.queryPrometheusRawWithContext(ctx, contextName, info, "api/v1/query", params)
	if err != nil {
		return nil, err
	}

	var result PrometheusQueryResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse prometheus response: %w", err)
	}

	if result.Status != "success" {
		return nil, fmt.Errorf("prometheus query error: %s - %s", result.ErrorType, result.Error)
	}

	return &result, nil
}

// QueryPrometheusRangeWithContext executes a range query against Prometheus with cancellation support
func (c *Client) QueryPrometheusRangeWithContext(ctx context.Context, contextName string, info *PrometheusInfo, query string, start, end time.Time, step time.Duration) (*PrometheusQueryResult, error) {
	params := map[string]string{
		"query": query,
		"start": fmt.Sprintf("%d", start.Unix()),
		"end":   fmt.Sprintf("%d", end.Unix()),
		"step":  fmt.Sprintf("%d", int(step.Seconds())),
	}

	data, err := c.queryPrometheusRawWithContext(ctx, contextName, info, "api/v1/query_range", params)
	if err != nil {
		return nil, err
	}

	var result PrometheusQueryResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse prometheus response: %w", err)
	}

	if result.Status != "success" {
		return nil, fmt.Errorf("prometheus query error: %s - %s", result.ErrorType, result.Error)
	}

	return &result, nil
}

// calculateMetricsStep computes a rounded Prometheus step interval for the given time range and target data points.
func calculateMetricsStep(start, end time.Time, maxDataPoints int) time.Duration {
	duration := end.Sub(start)
	step := duration / time.Duration(maxDataPoints)
	if step < 15*time.Second {
		step = 15 * time.Second
	}
	switch {
	case step < 30*time.Second:
		step = 15 * time.Second
	case step < time.Minute:
		step = 30 * time.Second
	case step < 5*time.Minute:
		step = time.Minute
	case step < 15*time.Minute:
		step = 5 * time.Minute
	case step < 30*time.Minute:
		step = 15 * time.Minute
	case step < time.Hour:
		step = 30 * time.Minute
	default:
		step = time.Hour
	}
	return step
}

// GetPodMetricsHistoryWithContext retrieves historical metrics with cancellation support
func (c *Client) GetPodMetricsHistoryWithContext(ctx context.Context, contextName string, info *PrometheusInfo, namespace, pod, container string, start, end time.Time, maxDataPoints int) (*PodMetricsHistory, error) {
	step := calculateMetricsStep(start, end, maxDataPoints)

	result := &PodMetricsHistory{
		Namespace:  namespace,
		Pod:        pod,
		Containers: []ContainerMetricsHistory{},
		Network:    &NetworkMetrics{},
	}

	// Build container filter
	containerFilter := ""
	if container != "" && container != "all" {
		containerFilter = fmt.Sprintf(`, container="%s"`, container)
	}

	// Query CPU usage (rate of cpu_usage_seconds_total)
	cpuQuery := fmt.Sprintf(
		`sum by (container) (rate(container_cpu_usage_seconds_total{namespace="%s", pod="%s"%s, container!="", container!="POD"}[5m])) * 1000`,
		namespace, pod, containerFilter,
	)

	cpuResult, err := c.QueryPrometheusRangeWithContext(ctx, contextName, info, cpuQuery, start, end, step)
	if err != nil {
		return nil, fmt.Errorf("failed to query CPU metrics: %w", err)
	}

	// Query Memory usage (working set bytes)
	memQuery := fmt.Sprintf(
		`sum by (container) (container_memory_working_set_bytes{namespace="%s", pod="%s"%s, container!="", container!="POD"})`,
		namespace, pod, containerFilter,
	)

	memResult, err := c.QueryPrometheusRangeWithContext(ctx, contextName, info, memQuery, start, end, step)
	if err != nil {
		return nil, fmt.Errorf("failed to query memory metrics: %w", err)
	}

	// Collect containers from results
	containers := make(map[string]*ContainerMetricsHistory)

	// Process CPU results
	for _, series := range cpuResult.Data.Result {
		containerName := series.Metric["container"]
		if containerName == "" {
			continue
		}

		container, exists := containers[containerName]
		if !exists {
			container = &ContainerMetricsHistory{
				Container: containerName,
				CPU:       make([]MetricsDataPoint, 0, len(series.Values)),
				Memory:    make([]MetricsDataPoint, 0, len(series.Values)),
			}
			containers[containerName] = container
		}

		for _, point := range series.Values {
			if len(point) >= 2 {
				ts, _ := point[0].(float64)
				valStr, _ := point[1].(string)
				val, _ := strconv.ParseFloat(valStr, 64)
				container.CPU = append(container.CPU, MetricsDataPoint{
					Timestamp: int64(ts) * 1000, // Convert to milliseconds
					Value:     val,
				})
			}
		}
	}

	// Process Memory results
	for _, series := range memResult.Data.Result {
		containerName := series.Metric["container"]
		if containerName == "" {
			continue
		}

		container, exists := containers[containerName]
		if !exists {
			container = &ContainerMetricsHistory{
				Container: containerName,
				CPU:       make([]MetricsDataPoint, 0, len(series.Values)),
				Memory:    make([]MetricsDataPoint, 0, len(series.Values)),
			}
			containers[containerName] = container
		}

		for _, point := range series.Values {
			if len(point) >= 2 {
				ts, _ := point[0].(float64)
				valStr, _ := point[1].(string)
				val, _ := strconv.ParseFloat(valStr, 64)
				container.Memory = append(container.Memory, MetricsDataPoint{
					Timestamp: int64(ts) * 1000, // Convert to milliseconds
					Value:     val,
				})
			}
		}
	}

	// Convert map to slice
	for _, ch := range containers {
		result.Containers = append(result.Containers, *ch)
	}

	// --- Network Metrics (pod-level) ---
	// Receive bytes/s
	rxBytesQuery := fmt.Sprintf(
		`sum(rate(container_network_receive_bytes_total{namespace="%s", pod="%s"}[5m]))`,
		namespace, pod,
	)
	result.Network.ReceiveBytes = c.queryRangeToDataPointsWithContext(ctx, contextName, info, rxBytesQuery, start, end, step)

	// Transmit bytes/s
	txBytesQuery := fmt.Sprintf(
		`sum(rate(container_network_transmit_bytes_total{namespace="%s", pod="%s"}[5m]))`,
		namespace, pod,
	)
	result.Network.TransmitBytes = c.queryRangeToDataPointsWithContext(ctx, contextName, info, txBytesQuery, start, end, step)

	// Receive packets/s
	rxPacketsQuery := fmt.Sprintf(
		`sum(rate(container_network_receive_packets_total{namespace="%s", pod="%s"}[5m]))`,
		namespace, pod,
	)
	result.Network.ReceivePackets = c.queryRangeToDataPointsWithContext(ctx, contextName, info, rxPacketsQuery, start, end, step)

	// Transmit packets/s
	txPacketsQuery := fmt.Sprintf(
		`sum(rate(container_network_transmit_packets_total{namespace="%s", pod="%s"}[5m]))`,
		namespace, pod,
	)
	result.Network.TransmitPackets = c.queryRangeToDataPointsWithContext(ctx, contextName, info, txPacketsQuery, start, end, step)

	// Dropped packets (receive)
	rxDroppedQuery := fmt.Sprintf(
		`sum(rate(container_network_receive_packets_dropped_total{namespace="%s", pod="%s"}[5m]))`,
		namespace, pod,
	)
	result.Network.ReceiveDropped = c.queryRangeToDataPointsWithContext(ctx, contextName, info, rxDroppedQuery, start, end, step)

	// Dropped packets (transmit)
	txDroppedQuery := fmt.Sprintf(
		`sum(rate(container_network_transmit_packets_dropped_total{namespace="%s", pod="%s"}[5m]))`,
		namespace, pod,
	)
	result.Network.TransmitDropped = c.queryRangeToDataPointsWithContext(ctx, contextName, info, txDroppedQuery, start, end, step)

	return result, nil
}

// PrometheusEndpoint allows manual configuration of Prometheus endpoint
type PrometheusEndpoint struct {
	Namespace string `json:"namespace"`
	Service   string `json:"service"`
	Port      int    `json:"port"`
}

// TestPrometheusEndpoint tests if a custom Prometheus endpoint works
func (c *Client) TestPrometheusEndpoint(contextName string, endpoint PrometheusEndpoint) error {
	info := &PrometheusInfo{
		Available: true,
		Namespace: endpoint.Namespace,
		Service:   endpoint.Service,
		Port:      endpoint.Port,
	}

	if !c.testPrometheusConnection(contextName, info) {
		return fmt.Errorf("could not connect to Prometheus at %s/%s:%d", endpoint.Namespace, endpoint.Service, endpoint.Port)
	}

	return nil
}

// NamespaceMetricsHistory holds historical metrics for a namespace
type NamespaceMetricsHistory struct {
	Namespace string             `json:"namespace"`
	CPU       []MetricsDataPoint `json:"cpu"`    // millicores
	Memory    []MetricsDataPoint `json:"memory"` // bytes
	Network   *NetworkMetrics    `json:"network"`
	PodCount  []MetricsDataPoint `json:"podCount"`
}

// ControllerMetricsHistory holds historical metrics for a controller (deployment, statefulset, etc.)
type ControllerMetricsHistory struct {
	Namespace      string             `json:"namespace"`
	Name           string             `json:"name"`
	ControllerType string             `json:"controllerType"`
	CPU            *ResourceMetrics   `json:"cpu"`
	Memory         *ResourceMetrics   `json:"memory"`
	Pods           *PodCountMetrics   `json:"pods"`
	Network        *NetworkMetrics    `json:"network"`
	Restarts       []MetricsDataPoint `json:"restarts"`
}

// ResourceMetrics holds CPU or memory metrics with node context
type ResourceMetrics struct {
	Usage           []MetricsDataPoint `json:"usage"`
	Request         []MetricsDataPoint `json:"request"`
	Limit           []MetricsDataPoint `json:"limit"`
	NodeAllocatable []MetricsDataPoint `json:"nodeAllocatable"`
	NodeUncommitted []MetricsDataPoint `json:"nodeUncommitted"`
}

// PodCountMetrics holds pod count metrics
type PodCountMetrics struct {
	Running []MetricsDataPoint `json:"running"`
	Desired []MetricsDataPoint `json:"desired"`
	Ready   []MetricsDataPoint `json:"ready"`
}

// NetworkMetrics holds network I/O metrics
type NetworkMetrics struct {
	ReceiveBytes    []MetricsDataPoint `json:"receiveBytes"`
	TransmitBytes   []MetricsDataPoint `json:"transmitBytes"`
	ReceivePackets  []MetricsDataPoint `json:"receivePackets"`
	TransmitPackets []MetricsDataPoint `json:"transmitPackets"`
	ReceiveDropped  []MetricsDataPoint `json:"receiveDropped"`
	TransmitDropped []MetricsDataPoint `json:"transmitDropped"`
}

// GetControllerMetricsHistory retrieves historical metrics for a controller
// GetControllerMetricsHistoryWithContext retrieves historical metrics with cancellation support
func (c *Client) GetControllerMetricsHistoryWithContext(ctx context.Context, contextName string, info *PrometheusInfo, namespace, name, controllerType string, start, end time.Time, maxDataPoints int) (*ControllerMetricsHistory, error) {
	step := calculateMetricsStep(start, end, maxDataPoints)

	result := &ControllerMetricsHistory{
		Namespace:      namespace,
		Name:           name,
		ControllerType: controllerType,
		CPU:            &ResourceMetrics{},
		Memory:         &ResourceMetrics{},
		Pods:           &PodCountMetrics{},
		Network:        &NetworkMetrics{},
		Restarts:       []MetricsDataPoint{},
	}

	// Build pod selector regex based on controller type
	// Deployments create ReplicaSets which create pods: deployment-name-<rs-hash>-<pod-hash>
	// StatefulSets create pods directly: statefulset-name-<ordinal>
	// DaemonSets create pods: daemonset-name-<hash>
	var podRegex string
	switch controllerType {
	case "deployment":
		podRegex = fmt.Sprintf("%s-[a-z0-9]+-[a-z0-9]+", name)
	case "statefulset":
		podRegex = fmt.Sprintf("%s-[0-9]+", name)
	case "daemonset", "replicaset":
		podRegex = fmt.Sprintf("%s-[a-z0-9]+", name)
	default:
		podRegex = fmt.Sprintf("%s-.*", name)
	}

	// --- CPU Metrics ---
	// CPU Usage (millicores)
	cpuUsageQuery := fmt.Sprintf(
		`sum(rate(container_cpu_usage_seconds_total{namespace="%s", pod=~"%s", container!="", container!="POD"}[5m])) * 1000`,
		namespace, podRegex,
	)
	result.CPU.Usage = c.queryRangeToDataPointsWithContext(ctx, contextName, info, cpuUsageQuery, start, end, step)

	// CPU Request (from kube-state-metrics, convert cores to millicores)
	cpuRequestQuery := fmt.Sprintf(
		`sum(kube_pod_container_resource_requests{namespace="%s", pod=~"%s", resource="cpu"}) * 1000`,
		namespace, podRegex,
	)
	result.CPU.Request = c.queryRangeToDataPointsWithContext(ctx, contextName, info, cpuRequestQuery, start, end, step)

	// CPU Limit
	cpuLimitQuery := fmt.Sprintf(
		`sum(kube_pod_container_resource_limits{namespace="%s", pod=~"%s", resource="cpu"}) * 1000`,
		namespace, podRegex,
	)
	result.CPU.Limit = c.queryRangeToDataPointsWithContext(ctx, contextName, info, cpuLimitQuery, start, end, step)

	// Node Allocatable CPU (for nodes where controller pods run)
	cpuAllocatableQuery := fmt.Sprintf(
		`sum(kube_node_status_allocatable{resource="cpu"} * on(node) group_left() (sum by(node) (kube_pod_info{namespace="%s", pod=~"%s"}) > 0)) * 1000`,
		namespace, podRegex,
	)
	result.CPU.NodeAllocatable = c.queryRangeToDataPointsWithContext(ctx, contextName, info, cpuAllocatableQuery, start, end, step)

	// Node Uncommitted CPU (allocatable - committed requests on those nodes)
	cpuUncommittedQuery := fmt.Sprintf(
		`(sum(kube_node_status_allocatable{resource="cpu"} * on(node) group_left() (sum by(node) (kube_pod_info{namespace="%s", pod=~"%s"}) > 0)) - sum(kube_pod_container_resource_requests{resource="cpu"} * on(pod, namespace) group_left(node) (kube_pod_info{node=~".+"} * on(node) group_left() (sum by(node) (kube_pod_info{namespace="%s", pod=~"%s"}) > 0)))) * 1000`,
		namespace, podRegex, namespace, podRegex,
	)
	result.CPU.NodeUncommitted = c.queryRangeToDataPointsWithContext(ctx, contextName, info, cpuUncommittedQuery, start, end, step)

	// --- Memory Metrics ---
	// Memory Usage (bytes)
	memUsageQuery := fmt.Sprintf(
		`sum(container_memory_working_set_bytes{namespace="%s", pod=~"%s", container!="", container!="POD"})`,
		namespace, podRegex,
	)
	result.Memory.Usage = c.queryRangeToDataPointsWithContext(ctx, contextName, info, memUsageQuery, start, end, step)

	// Memory Request
	memRequestQuery := fmt.Sprintf(
		`sum(kube_pod_container_resource_requests{namespace="%s", pod=~"%s", resource="memory"})`,
		namespace, podRegex,
	)
	result.Memory.Request = c.queryRangeToDataPointsWithContext(ctx, contextName, info, memRequestQuery, start, end, step)

	// Memory Limit
	memLimitQuery := fmt.Sprintf(
		`sum(kube_pod_container_resource_limits{namespace="%s", pod=~"%s", resource="memory"})`,
		namespace, podRegex,
	)
	result.Memory.Limit = c.queryRangeToDataPointsWithContext(ctx, contextName, info, memLimitQuery, start, end, step)

	// Node Allocatable Memory
	memAllocatableQuery := fmt.Sprintf(
		`sum(kube_node_status_allocatable{resource="memory"} * on(node) group_left() (sum by(node) (kube_pod_info{namespace="%s", pod=~"%s"}) > 0))`,
		namespace, podRegex,
	)
	result.Memory.NodeAllocatable = c.queryRangeToDataPointsWithContext(ctx, contextName, info, memAllocatableQuery, start, end, step)

	// Node Uncommitted Memory
	memUncommittedQuery := fmt.Sprintf(
		`sum(kube_node_status_allocatable{resource="memory"} * on(node) group_left() (sum by(node) (kube_pod_info{namespace="%s", pod=~"%s"}) > 0)) - sum(kube_pod_container_resource_requests{resource="memory"} * on(pod, namespace) group_left(node) (kube_pod_info{node=~".+"} * on(node) group_left() (sum by(node) (kube_pod_info{namespace="%s", pod=~"%s"}) > 0)))`,
		namespace, podRegex, namespace, podRegex,
	)
	result.Memory.NodeUncommitted = c.queryRangeToDataPointsWithContext(ctx, contextName, info, memUncommittedQuery, start, end, step)

	// --- Pod Count Metrics ---
	// Running pods
	podsRunningQuery := fmt.Sprintf(
		`count(kube_pod_status_phase{namespace="%s", pod=~"%s", phase="Running"})`,
		namespace, podRegex,
	)
	result.Pods.Running = c.queryRangeToDataPointsWithContext(ctx, contextName, info, podsRunningQuery, start, end, step)

	// Ready pods
	podsReadyQuery := fmt.Sprintf(
		`sum(kube_pod_status_ready{namespace="%s", pod=~"%s", condition="true"})`,
		namespace, podRegex,
	)
	result.Pods.Ready = c.queryRangeToDataPointsWithContext(ctx, contextName, info, podsReadyQuery, start, end, step)

	// Desired replicas (depends on controller type)
	var desiredQuery string
	switch controllerType {
	case "deployment":
		desiredQuery = fmt.Sprintf(`kube_deployment_spec_replicas{namespace="%s", deployment="%s"}`, namespace, name)
	case "statefulset":
		desiredQuery = fmt.Sprintf(`kube_statefulset_replicas{namespace="%s", statefulset="%s"}`, namespace, name)
	case "replicaset":
		desiredQuery = fmt.Sprintf(`kube_replicaset_spec_replicas{namespace="%s", replicaset="%s"}`, namespace, name)
	case "daemonset":
		desiredQuery = fmt.Sprintf(`kube_daemonset_status_desired_number_scheduled{namespace="%s", daemonset="%s"}`, namespace, name)
	}
	if desiredQuery != "" {
		result.Pods.Desired = c.queryRangeToDataPointsWithContext(ctx, contextName, info, desiredQuery, start, end, step)
	}

	// --- Network Metrics ---
	// Receive bytes/s
	rxBytesQuery := fmt.Sprintf(
		`sum(rate(container_network_receive_bytes_total{namespace="%s", pod=~"%s"}[5m]))`,
		namespace, podRegex,
	)
	result.Network.ReceiveBytes = c.queryRangeToDataPointsWithContext(ctx, contextName, info, rxBytesQuery, start, end, step)

	// Transmit bytes/s
	txBytesQuery := fmt.Sprintf(
		`sum(rate(container_network_transmit_bytes_total{namespace="%s", pod=~"%s"}[5m]))`,
		namespace, podRegex,
	)
	result.Network.TransmitBytes = c.queryRangeToDataPointsWithContext(ctx, contextName, info, txBytesQuery, start, end, step)

	// Receive packets/s
	rxPacketsQuery := fmt.Sprintf(
		`sum(rate(container_network_receive_packets_total{namespace="%s", pod=~"%s"}[5m]))`,
		namespace, podRegex,
	)
	result.Network.ReceivePackets = c.queryRangeToDataPointsWithContext(ctx, contextName, info, rxPacketsQuery, start, end, step)

	// Transmit packets/s
	txPacketsQuery := fmt.Sprintf(
		`sum(rate(container_network_transmit_packets_total{namespace="%s", pod=~"%s"}[5m]))`,
		namespace, podRegex,
	)
	result.Network.TransmitPackets = c.queryRangeToDataPointsWithContext(ctx, contextName, info, txPacketsQuery, start, end, step)

	// Dropped packets (receive)
	rxDroppedQuery := fmt.Sprintf(
		`sum(rate(container_network_receive_packets_dropped_total{namespace="%s", pod=~"%s"}[5m]))`,
		namespace, podRegex,
	)
	result.Network.ReceiveDropped = c.queryRangeToDataPointsWithContext(ctx, contextName, info, rxDroppedQuery, start, end, step)

	// Dropped packets (transmit)
	txDroppedQuery := fmt.Sprintf(
		`sum(rate(container_network_transmit_packets_dropped_total{namespace="%s", pod=~"%s"}[5m]))`,
		namespace, podRegex,
	)
	result.Network.TransmitDropped = c.queryRangeToDataPointsWithContext(ctx, contextName, info, txDroppedQuery, start, end, step)

	// --- Restarts ---
	restartsQuery := fmt.Sprintf(
		`sum(kube_pod_container_status_restarts_total{namespace="%s", pod=~"%s"})`,
		namespace, podRegex,
	)
	result.Restarts = c.queryRangeToDataPointsWithContext(ctx, contextName, info, restartsQuery, start, end, step)

	return result, nil
}

// GetNamespaceMetricsHistoryWithContext retrieves historical metrics for a namespace with cancellation support
func (c *Client) GetNamespaceMetricsHistoryWithContext(ctx context.Context, contextName string, info *PrometheusInfo, namespace string, start, end time.Time, maxDataPoints int) (*NamespaceMetricsHistory, error) {
	step := calculateMetricsStep(start, end, maxDataPoints)

	result := &NamespaceMetricsHistory{
		Namespace: namespace,
		Network:   &NetworkMetrics{},
	}

	// --- CPU Metrics ---
	// CPU Usage (millicores) - sum of all containers in the namespace
	cpuUsageQuery := fmt.Sprintf(
		`sum(rate(container_cpu_usage_seconds_total{namespace="%s", container!="", container!="POD"}[5m])) * 1000`,
		namespace,
	)
	result.CPU = c.queryRangeToDataPointsWithContext(ctx, contextName, info, cpuUsageQuery, start, end, step)

	// --- Memory Metrics ---
	// Memory Usage (bytes) - sum of all containers in the namespace
	memUsageQuery := fmt.Sprintf(
		`sum(container_memory_working_set_bytes{namespace="%s", container!="", container!="POD"})`,
		namespace,
	)
	result.Memory = c.queryRangeToDataPointsWithContext(ctx, contextName, info, memUsageQuery, start, end, step)

	// --- Pod Count ---
	podCountQuery := fmt.Sprintf(
		`count(kube_pod_info{namespace="%s"})`,
		namespace,
	)
	result.PodCount = c.queryRangeToDataPointsWithContext(ctx, contextName, info, podCountQuery, start, end, step)

	// --- Network Metrics ---
	// Receive bytes/s
	rxBytesQuery := fmt.Sprintf(
		`sum(rate(container_network_receive_bytes_total{namespace="%s"}[5m]))`,
		namespace,
	)
	result.Network.ReceiveBytes = c.queryRangeToDataPointsWithContext(ctx, contextName, info, rxBytesQuery, start, end, step)

	// Transmit bytes/s
	txBytesQuery := fmt.Sprintf(
		`sum(rate(container_network_transmit_bytes_total{namespace="%s"}[5m]))`,
		namespace,
	)
	result.Network.TransmitBytes = c.queryRangeToDataPointsWithContext(ctx, contextName, info, txBytesQuery, start, end, step)

	// Receive packets/s
	rxPacketsQuery := fmt.Sprintf(
		`sum(rate(container_network_receive_packets_total{namespace="%s"}[5m]))`,
		namespace,
	)
	result.Network.ReceivePackets = c.queryRangeToDataPointsWithContext(ctx, contextName, info, rxPacketsQuery, start, end, step)

	// Transmit packets/s
	txPacketsQuery := fmt.Sprintf(
		`sum(rate(container_network_transmit_packets_total{namespace="%s"}[5m]))`,
		namespace,
	)
	result.Network.TransmitPackets = c.queryRangeToDataPointsWithContext(ctx, contextName, info, txPacketsQuery, start, end, step)

	// Dropped packets (receive)
	rxDroppedQuery := fmt.Sprintf(
		`sum(rate(container_network_receive_packets_dropped_total{namespace="%s"}[5m]))`,
		namespace,
	)
	result.Network.ReceiveDropped = c.queryRangeToDataPointsWithContext(ctx, contextName, info, rxDroppedQuery, start, end, step)

	// Dropped packets (transmit)
	txDroppedQuery := fmt.Sprintf(
		`sum(rate(container_network_transmit_packets_dropped_total{namespace="%s"}[5m]))`,
		namespace,
	)
	result.Network.TransmitDropped = c.queryRangeToDataPointsWithContext(ctx, contextName, info, txDroppedQuery, start, end, step)

	return result, nil
}

// queryRangeToDataPoints is a helper that runs a range query and converts to data points
// queryRangeToDataPointsWithContext is a helper that runs a range query with cancellation support
func (c *Client) queryRangeToDataPointsWithContext(ctx context.Context, contextName string, info *PrometheusInfo, query string, start, end time.Time, step time.Duration) []MetricsDataPoint {
	result, err := c.QueryPrometheusRangeWithContext(ctx, contextName, info, query, start, end, step)
	if err != nil {
		return []MetricsDataPoint{}
	}

	if len(result.Data.Result) == 0 {
		return []MetricsDataPoint{}
	}
	values := result.Data.Result[0].Values
	points := make([]MetricsDataPoint, 0, len(values))
	for _, point := range values {
		if len(point) >= 2 {
			ts, _ := point[0].(float64)
			valStr, _ := point[1].(string)
			val, _ := strconv.ParseFloat(valStr, 64)
			points = append(points, MetricsDataPoint{
				Timestamp: int64(ts) * 1000, // Convert to milliseconds
				Value:     val,
			})
		}
	}
	return points
}

// NodeMetricsHistory holds historical metrics for a node
type NodeMetricsHistory struct {
	NodeName string               `json:"nodeName"`
	CPU      *NodeResourceMetrics `json:"cpu"`
	Memory   *NodeResourceMetrics `json:"memory"`
	Pods     *NodePodMetrics      `json:"pods"`
	Network  *NetworkMetrics      `json:"network"`
}

// NodeResourceMetrics holds CPU or memory metrics for a node
type NodeResourceMetrics struct {
	Usage       []MetricsDataPoint `json:"usage"`
	Allocatable []MetricsDataPoint `json:"allocatable"`
	Reserved    []MetricsDataPoint `json:"reserved"`
	Committed   []MetricsDataPoint `json:"committed"`
}

// NodePodMetrics holds pod count metrics for a node
type NodePodMetrics struct {
	Running  []MetricsDataPoint `json:"running"`
	Capacity []MetricsDataPoint `json:"capacity"`
}

// GetNodeMetricsHistory retrieves historical metrics for a specific node
// GetNodeMetricsHistoryWithContext retrieves historical metrics with cancellation support
func (c *Client) GetNodeMetricsHistoryWithContext(ctx context.Context, contextName string, info *PrometheusInfo, nodeName string, start, end time.Time, maxDataPoints int) (*NodeMetricsHistory, error) {
	step := calculateMetricsStep(start, end, maxDataPoints)

	result := &NodeMetricsHistory{
		NodeName: nodeName,
		CPU:      &NodeResourceMetrics{},
		Memory:   &NodeResourceMetrics{},
		Pods:     &NodePodMetrics{},
		Network:  &NetworkMetrics{},
	}

	// --- CPU Metrics ---
	// CPU Usage (all pods on this node, in cores)
	cpuUsageQuery := fmt.Sprintf(
		`sum(rate(container_cpu_usage_seconds_total{node="%s", container!="", container!="POD"}[5m]))`,
		nodeName,
	)
	result.CPU.Usage = c.queryRangeToDataPointsWithContext(ctx, contextName, info, cpuUsageQuery, start, end, step)

	// CPU Allocatable (node capacity for pods, in cores)
	cpuAllocatableQuery := fmt.Sprintf(
		`kube_node_status_allocatable{node="%s", resource="cpu"}`,
		nodeName,
	)
	result.CPU.Allocatable = c.queryRangeToDataPointsWithContext(ctx, contextName, info, cpuAllocatableQuery, start, end, step)

	// CPU Reserved = sum of requests for pods on this node
	cpuReservedQuery := fmt.Sprintf(
		`sum(kube_pod_container_resource_requests{resource="cpu"} * on(namespace, pod) group_left() (kube_pod_info{node="%s"} > bool 0))`,
		nodeName,
	)
	result.CPU.Reserved = c.queryRangeToDataPointsWithContext(ctx, contextName, info, cpuReservedQuery, start, end, step)

	// CPU Committed = sum of max(usage, request) per container
	// First get requests for pods on this node, then join with usage and compute max
	cpuCommittedQuery := fmt.Sprintf(
		`sum(max by(namespace, pod, container) (label_replace(rate(container_cpu_usage_seconds_total{node="%s", container!="", container!="POD"}[5m]), "src", "usage", "", "") or label_replace(kube_pod_container_resource_requests{resource="cpu"} * on(namespace, pod) group_left() (kube_pod_info{node="%s"} > bool 0), "src", "request", "", "")))`,
		nodeName, nodeName,
	)
	result.CPU.Committed = c.queryRangeToDataPointsWithContext(ctx, contextName, info, cpuCommittedQuery, start, end, step)

	// --- Memory Metrics ---
	// Memory Usage (all pods on this node)
	memUsageQuery := fmt.Sprintf(
		`sum(container_memory_working_set_bytes{node="%s", container!="", container!="POD"})`,
		nodeName,
	)
	result.Memory.Usage = c.queryRangeToDataPointsWithContext(ctx, contextName, info, memUsageQuery, start, end, step)

	// Memory Allocatable
	memAllocatableQuery := fmt.Sprintf(
		`kube_node_status_allocatable{node="%s", resource="memory"}`,
		nodeName,
	)
	result.Memory.Allocatable = c.queryRangeToDataPointsWithContext(ctx, contextName, info, memAllocatableQuery, start, end, step)

	// Memory Reserved = sum of requests for pods on this node
	memReservedQuery := fmt.Sprintf(
		`sum(kube_pod_container_resource_requests{resource="memory"} * on(namespace, pod) group_left() (kube_pod_info{node="%s"} > bool 0))`,
		nodeName,
	)
	result.Memory.Reserved = c.queryRangeToDataPointsWithContext(ctx, contextName, info, memReservedQuery, start, end, step)

	// Memory Committed = sum of max(usage, request) per container
	// First get requests for pods on this node, then join with usage and compute max
	memCommittedQuery := fmt.Sprintf(
		`sum(max by(namespace, pod, container) (label_replace(container_memory_working_set_bytes{node="%s", container!="", container!="POD"}, "src", "usage", "", "") or label_replace(kube_pod_container_resource_requests{resource="memory"} * on(namespace, pod) group_left() (kube_pod_info{node="%s"} > bool 0), "src", "request", "", "")))`,
		nodeName, nodeName,
	)
	result.Memory.Committed = c.queryRangeToDataPointsWithContext(ctx, contextName, info, memCommittedQuery, start, end, step)

	// --- Pod Count Metrics ---
	// Running pods on node
	podRunningQuery := fmt.Sprintf(
		`count(kube_pod_info{node="%s"})`,
		nodeName,
	)
	result.Pods.Running = c.queryRangeToDataPointsWithContext(ctx, contextName, info, podRunningQuery, start, end, step)

	// Pod capacity
	podCapacityQuery := fmt.Sprintf(
		`kube_node_status_capacity{node="%s", resource="pods"}`,
		nodeName,
	)
	result.Pods.Capacity = c.queryRangeToDataPointsWithContext(ctx, contextName, info, podCapacityQuery, start, end, step)

	// --- Network Metrics ---
	// Network receive bytes/s - join with node_uname_info to match by node name
	// Use positive device filter for physical interfaces (eth*, en*) which works across Linux distributions
	// Fallback to node/kubernetes_node labels for clusters where those are available
	rxBytesQuery := fmt.Sprintf(
		`sum(rate(node_network_receive_bytes_total{device=~"eth.*|en.*"}[5m]) * on(instance) group_left(nodename) node_uname_info{nodename="%s"}) or sum(rate(node_network_receive_bytes_total{node="%s", device=~"eth.*|en.*"}[5m])) or sum(rate(node_network_receive_bytes_total{kubernetes_node="%s", device=~"eth.*|en.*"}[5m]))`,
		nodeName, nodeName, nodeName,
	)
	result.Network.ReceiveBytes = c.queryRangeToDataPointsWithContext(ctx, contextName, info, rxBytesQuery, start, end, step)

	// Network transmit bytes/s
	txBytesQuery := fmt.Sprintf(
		`sum(rate(node_network_transmit_bytes_total{device=~"eth.*|en.*"}[5m]) * on(instance) group_left(nodename) node_uname_info{nodename="%s"}) or sum(rate(node_network_transmit_bytes_total{node="%s", device=~"eth.*|en.*"}[5m])) or sum(rate(node_network_transmit_bytes_total{kubernetes_node="%s", device=~"eth.*|en.*"}[5m]))`,
		nodeName, nodeName, nodeName,
	)
	result.Network.TransmitBytes = c.queryRangeToDataPointsWithContext(ctx, contextName, info, txBytesQuery, start, end, step)

	// Network receive packets/s
	rxPacketsQuery := fmt.Sprintf(
		`sum(rate(node_network_receive_packets_total{device=~"eth.*|en.*"}[5m]) * on(instance) group_left(nodename) node_uname_info{nodename="%s"}) or sum(rate(node_network_receive_packets_total{node="%s", device=~"eth.*|en.*"}[5m])) or sum(rate(node_network_receive_packets_total{kubernetes_node="%s", device=~"eth.*|en.*"}[5m]))`,
		nodeName, nodeName, nodeName,
	)
	result.Network.ReceivePackets = c.queryRangeToDataPointsWithContext(ctx, contextName, info, rxPacketsQuery, start, end, step)

	// Network transmit packets/s
	txPacketsQuery := fmt.Sprintf(
		`sum(rate(node_network_transmit_packets_total{device=~"eth.*|en.*"}[5m]) * on(instance) group_left(nodename) node_uname_info{nodename="%s"}) or sum(rate(node_network_transmit_packets_total{node="%s", device=~"eth.*|en.*"}[5m])) or sum(rate(node_network_transmit_packets_total{kubernetes_node="%s", device=~"eth.*|en.*"}[5m]))`,
		nodeName, nodeName, nodeName,
	)
	result.Network.TransmitPackets = c.queryRangeToDataPointsWithContext(ctx, contextName, info, txPacketsQuery, start, end, step)

	// Network receive dropped packets/s
	rxDroppedQuery := fmt.Sprintf(
		`sum(rate(node_network_receive_drop_total{device=~"eth.*|en.*"}[5m]) * on(instance) group_left(nodename) node_uname_info{nodename="%s"}) or sum(rate(node_network_receive_drop_total{node="%s", device=~"eth.*|en.*"}[5m])) or sum(rate(node_network_receive_drop_total{kubernetes_node="%s", device=~"eth.*|en.*"}[5m]))`,
		nodeName, nodeName, nodeName,
	)
	result.Network.ReceiveDropped = c.queryRangeToDataPointsWithContext(ctx, contextName, info, rxDroppedQuery, start, end, step)

	// Network transmit dropped packets/s
	txDroppedQuery := fmt.Sprintf(
		`sum(rate(node_network_transmit_drop_total{device=~"eth.*|en.*"}[5m]) * on(instance) group_left(nodename) node_uname_info{nodename="%s"}) or sum(rate(node_network_transmit_drop_total{node="%s", device=~"eth.*|en.*"}[5m])) or sum(rate(node_network_transmit_drop_total{kubernetes_node="%s", device=~"eth.*|en.*"}[5m]))`,
		nodeName, nodeName, nodeName,
	)
	result.Network.TransmitDropped = c.queryRangeToDataPointsWithContext(ctx, contextName, info, txDroppedQuery, start, end, step)

	return result, nil
}

// getClientsetForContext gets a clientset for a specific context
func (c *Client) getClientsetForContext(contextName string) (kubernetes.Interface, error) {
	c.mu.RLock()
	currentCtx := c.currentContext
	cs := c.clientset
	c.mu.RUnlock()

	if contextName == "" || contextName == currentCtx {
		if cs == nil {
			return nil, fmt.Errorf("k8s client not initialized")
		}
		return cs, nil
	}

	// Need to create a new client for different context
	home := homedir.HomeDir()
	kubeconfigPath := filepath.Join(home, ".kube", "config")

	loadingRules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath}
	configOverrides := &clientcmd.ConfigOverrides{CurrentContext: contextName}

	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
	config, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load config for context %s: %w", contextName, err)
	}

	return kubernetes.NewForConfig(config)
}
