package k8s

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"kubikles/pkg/debug"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/client-go/util/homedir"
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

// defaultPageSize is the number of items per page for paginated list requests.
// 1000 is a good balance between minimizing round-trips and keeping responses manageable.
const defaultPageSize int64 = 1000

// paginatedList fetches all items using the Kubernetes API pagination mechanism.
// It accumulates results across pages and optionally reports progress.
//
// When onProgress is nil, it performs a single unbounded List call (no Limit set)
// which allows the API server to serve from the watch cache for maximum performance.
// When onProgress is provided, it uses pagination with Limit to enable progress tracking.
func paginatedList[T any](
	ctx context.Context,
	resourceType string,
	pageSize int64,
	fetchPage func(ctx context.Context, opts metav1.ListOptions) (items []T, continueToken string, remaining *int64, err error),
	onProgress func(loaded, total int),
) ([]T, error) {
	start := time.Now()

	// Fast path: no progress callback → single unbounded List (watch-cache friendly)
	if onProgress == nil {
		items, _, _, err := fetchPage(ctx, metav1.ListOptions{})
		if err != nil {
			debug.LogK8s("paginatedList fast-path error", map[string]interface{}{"resource": resourceType, "duration": time.Since(start).String(), "error": err.Error()})
			return nil, err
		}
		debug.LogK8s("paginatedList fast-path", map[string]interface{}{"resource": resourceType, "items": len(items), "duration": time.Since(start).String()})
		return items, nil
	}

	// Paginated path: use Limit to enable progress tracking
	var allItems []T
	opts := metav1.ListOptions{Limit: pageSize}
	pages := 0

	for {
		// Check context before each page
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		items, continueToken, remaining, err := fetchPage(ctx, opts)
		if err != nil {
			debug.LogK8s("paginatedList page error", map[string]interface{}{"resource": resourceType, "page": pages + 1, "duration": time.Since(start).String(), "error": err.Error()})
			return nil, err
		}
		pages++

		allItems = append(allItems, items...)

		// Report progress
		total := len(allItems)
		if remaining != nil {
			total = len(allItems) + int(*remaining)
		}
		onProgress(len(allItems), total)

		// No more pages
		if continueToken == "" {
			break
		}

		opts.Continue = continueToken
	}

	debug.LogK8s("paginatedList complete", map[string]interface{}{"resource": resourceType, "items": len(allItems), "pages": pages, "duration": time.Since(start).String()})
	return allItems, nil
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

	// Additional kubeconfig file paths beyond the default ~/.kube/config
	extraKubeconfigPaths []string
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

// SetExtraKubeconfigPaths sets additional kubeconfig file paths to merge contexts from.
func (c *Client) SetExtraKubeconfigPaths(paths []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.extraKubeconfigPaths = paths
}

// getLoadingRules builds ClientConfigLoadingRules that include all kubeconfig paths.
func (c *Client) getLoadingRules() *clientcmd.ClientConfigLoadingRules {
	home := homedir.HomeDir()
	primary := filepath.Join(home, ".kube", "config")

	c.mu.RLock()
	extra := c.extraKubeconfigPaths
	c.mu.RUnlock()

	if len(extra) == 0 {
		return &clientcmd.ClientConfigLoadingRules{ExplicitPath: primary}
	}

	// Use Precedence instead of ExplicitPath to merge multiple files
	allPaths := append([]string{primary}, extra...)
	return &clientcmd.ClientConfigLoadingRules{Precedence: allPaths}
}

// ContextDetail contains metadata about a kubeconfig context.
type ContextDetail struct {
	Name      string `json:"name"`
	Cluster   string `json:"cluster"`
	Server    string `json:"server"`
	AuthInfo  string `json:"authInfo"`
	Namespace string `json:"namespace"`
	IsActive  bool   `json:"isActive"`
}

// GetContextDetails returns detailed info for all kubeconfig contexts.
func (c *Client) GetContextDetails() ([]ContextDetail, error) {
	loader := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		c.getLoadingRules(), &clientcmd.ConfigOverrides{},
	)
	rawConfig, err := loader.RawConfig()
	if err != nil {
		return nil, err
	}

	currentCtx := c.GetCurrentContext()
	details := make([]ContextDetail, 0, len(rawConfig.Contexts))
	for name, ctx := range rawConfig.Contexts {
		d := ContextDetail{
			Name:      name,
			Cluster:   ctx.Cluster,
			AuthInfo:  ctx.AuthInfo,
			Namespace: ctx.Namespace,
			IsActive:  name == currentCtx,
		}
		if cluster, ok := rawConfig.Clusters[ctx.Cluster]; ok {
			d.Server = cluster.Server
		}
		details = append(details, d)
	}
	return details, nil
}

// DeleteContext removes a context from the kubeconfig file.
func (c *Client) DeleteContext(name string) error {
	if name == c.GetCurrentContext() {
		return fmt.Errorf("cannot delete the active context %q; switch to another context first", name)
	}

	// Find and modify the kubeconfig file containing this context
	return c.modifyKubeconfigContext(name, func(config *clientcmdapi.Config) error {
		if _, ok := config.Contexts[name]; !ok {
			return fmt.Errorf("context %q not found", name)
		}
		delete(config.Contexts, name)
		if config.CurrentContext == name {
			config.CurrentContext = ""
		}
		return nil
	})
}

// RenameContext renames a context in the kubeconfig file.
func (c *Client) RenameContext(oldName, newName string) error {
	if newName == "" {
		return fmt.Errorf("new context name cannot be empty")
	}
	if oldName == newName {
		return nil
	}

	err := c.modifyKubeconfigContext(oldName, func(config *clientcmdapi.Config) error {
		if _, ok := config.Contexts[oldName]; !ok {
			return fmt.Errorf("context %q not found", oldName)
		}
		if _, exists := config.Contexts[newName]; exists {
			return fmt.Errorf("context %q already exists", newName)
		}
		config.Contexts[newName] = config.Contexts[oldName]
		delete(config.Contexts, oldName)
		if config.CurrentContext == oldName {
			config.CurrentContext = newName
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Update internal state if renaming the active context
	if oldName == c.GetCurrentContext() {
		c.mu.Lock()
		c.currentContext = newName
		c.mu.Unlock()
	}
	return nil
}

// modifyKubeconfigContext finds the kubeconfig file containing the named context,
// applies the mutation, and writes the file back.
func (c *Client) modifyKubeconfigContext(contextName string, mutate func(*clientcmdapi.Config) error) error {
	home := homedir.HomeDir()
	primary := filepath.Join(home, ".kube", "config")

	c.mu.RLock()
	extra := c.extraKubeconfigPaths
	c.mu.RUnlock()

	allPaths := append([]string{primary}, extra...)

	for _, path := range allPaths {
		rules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: path}
		loader := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, &clientcmd.ConfigOverrides{})
		rawConfig, err := loader.RawConfig()
		if err != nil {
			continue
		}
		if _, ok := rawConfig.Contexts[contextName]; !ok {
			continue
		}
		// Found the file — apply mutation
		if err := mutate(&rawConfig); err != nil {
			return err
		}
		return clientcmd.WriteToFile(rawConfig, path)
	}

	return fmt.Errorf("context %q not found in any kubeconfig file", contextName)
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
	if IsDebugClusterContext(contextName) {
		if err := c.switchToDebugCluster(); err != nil {
			return err
		}
		// Warmup is instant for debug cluster
		c.mu.Lock()
		ch := make(chan struct{})
		close(ch)
		c.warmupDone = ch
		c.mu.Unlock()
		return nil
	}

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
	if IsDebugClusterContext(c.GetCurrentContext()) {
		return nil // debug cluster is always "connected"
	}

	cs, err := c.getClientset()
	if err != nil {
		return fmt.Errorf("failed to get clientset: %w", err)
	}

	// Use RESTClient with context for proper timeout control
	// Hits /api which is lightweight and respects the context deadline
	err = cs.CoreV1().RESTClient().Get().AbsPath("/api").Do(ctx).Error()
	if err != nil {
		enriched := c.enrichExecError(err)
		return fmt.Errorf("cluster unreachable: %w", enriched)
	}

	return nil
}

// enrichExecError checks if an error is from an exec-based credential plugin
// (e.g. "executable aws failed with exit code 254") and tries to capture the
// actual stderr from the plugin command for a more useful error message.
// client-go pipes exec plugin stderr directly to os.Stderr, so it never appears
// in the Go error. We re-run the command briefly to capture the real error.
func (c *Client) enrichExecError(origErr error) error {
	errStr := origErr.Error()
	if !strings.Contains(errStr, "failed with exit code") || !strings.Contains(errStr, "exec:") {
		return origErr
	}

	// Look up the exec config from kubeconfig
	c.mu.RLock()
	configLoading := c.configLoading
	currentCtx := c.currentContext
	c.mu.RUnlock()

	if configLoading == nil {
		return origErr
	}

	rawConfig, err := configLoading.RawConfig()
	if err != nil {
		return origErr
	}

	// Use c.currentContext (the overridden context) rather than rawConfig.CurrentContext
	// which only reflects the kubeconfig file's default context.
	if currentCtx == "" {
		currentCtx = rawConfig.CurrentContext
	}
	ctxObj, ok := rawConfig.Contexts[currentCtx]
	if !ok || ctxObj == nil {
		return origErr
	}

	authInfo, ok := rawConfig.AuthInfos[ctxObj.AuthInfo]
	if !ok || authInfo == nil || authInfo.Exec == nil {
		return origErr
	}

	execCfg := authInfo.Exec

	// Run the exec command with captured stderr to get the real error message.
	// Inherit the process environment so PATH, AWS_PROFILE, etc. are available,
	// then overlay any env vars from the kubeconfig exec config.
	cmdCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, execCfg.Command, execCfg.Args...) //nolint:gosec // args come from user's kubeconfig
	if len(execCfg.Env) > 0 {
		cmd.Env = os.Environ()
		for _, env := range execCfg.Env {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", env.Name, env.Value))
		}
	}

	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	_ = cmd.Run() // We expect this to fail — we want the stderr

	stderrStr := strings.TrimSpace(stderr.String())
	if stderrStr == "" {
		return origErr
	}

	debug.LogK8s("enrichExecError: captured exec plugin stderr", map[string]interface{}{
		"command": execCfg.Command,
		"stderr":  stderrStr,
	})

	return fmt.Errorf("%w\n\n%s stderr:\n%s", origErr, execCfg.Command, stderrStr)
}

func (c *Client) ListContexts() ([]string, error) {
	// Use a fresh loader each call to pick up externally added/removed contexts.
	// The shared c.configLoading may cache internal state after ClientConfig() is called.
	loader := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		c.getLoadingRules(), &clientcmd.ConfigOverrides{},
	)
	rawConfig, err := loader.RawConfig()
	if err != nil {
		return nil, err
	}
	contexts := make([]string, 0, len(rawConfig.Contexts)+1)
	for name := range rawConfig.Contexts {
		contexts = append(contexts, name)
	}
	if DebugClusterContextName != "" {
		contexts = append(contexts, DebugClusterContextName)
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

func (c *Client) getClientForContext(contextName string) (kubernetes.Interface, error) {
	if IsDebugClusterContext(contextName) {
		return GetDebugClusterClientset()
	}

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

func (c *Client) GetRestConfigForContext(contextName string) (*rest.Config, error) {
	if IsDebugClusterContext(contextName) {
		return nil, fmt.Errorf("no REST config for debug cluster (exec/port-forward/logs are not supported)")
	}

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

func (c *Client) getClientsetForContext(contextName string) (kubernetes.Interface, error) {
	if IsDebugClusterContext(contextName) {
		return GetDebugClusterClientset()
	}

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
