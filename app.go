package main

import (
	"archive/zip"
	"context"
	"fmt"
	"kubikles/pkg/k8s"
	"kubikles/pkg/terminal"
	"net/url"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	storagev1 "k8s.io/api/storage/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

// App struct
type App struct {
	ctx              context.Context
	k8sClient        *k8s.Client
	terminalService  *terminal.Service
	watcherManager   *ResourceWatcherManager
	// Log streaming
	logStreams      map[string]context.CancelFunc
	logStreamsMutex sync.Mutex
}

// WatcherCleanupDelay is the time to wait before stopping a watcher with no subscribers
const WatcherCleanupDelay = 5 * time.Second

// ResourceEvent is the generic event emitted for any resource type change
type ResourceEvent struct {
	Type         string                 `json:"type"`         // ADDED, MODIFIED, DELETED
	ResourceType string                 `json:"resourceType"` // pods, namespaces, deployments, etc.
	Namespace    string                 `json:"namespace"`    // Resource's namespace (empty for cluster-scoped)
	Resource     map[string]interface{} `json:"resource"`     // Unstructured resource data
}

// ResourceWatcher tracks a single watcher instance with reference counting
type ResourceWatcher struct {
	Key          string             // "resourceType:namespace" or "crd:group/version/resource:namespace"
	ResourceType string             // Resource type identifier
	Namespace    string             // Namespace being watched (empty for cluster-scoped)
	Group        string             // API group (for CRDs)
	Version      string             // API version (for CRDs)
	Resource     string             // Resource plural name (for CRDs)
	IsCRD        bool               // Whether this is a CRD watcher
	RefCount     int32              // Atomic counter for subscribers
	Cancel       context.CancelFunc // Cancel function for the watch loop
	CleanupTimer *time.Timer        // Delayed cleanup timer
}

// ResourceWatcherManager manages all active resource watchers with reference counting
type ResourceWatcherManager struct {
	ctx      context.Context
	app      *App
	watchers map[string]*ResourceWatcher
	mutex    sync.RWMutex
}

// NewResourceWatcherManager creates a new ResourceWatcherManager
func NewResourceWatcherManager(ctx context.Context, app *App) *ResourceWatcherManager {
	return &ResourceWatcherManager{
		ctx:      ctx,
		app:      app,
		watchers: make(map[string]*ResourceWatcher),
	}
}

// Subscribe subscribes to a resource watcher, returning the watcher key
// If a watcher already exists, it increments the refCount and cancels any pending cleanup
// If no watcher exists, it creates a new one and starts the watch loop
func (m *ResourceWatcherManager) Subscribe(resourceType, namespace string) string {
	key := fmt.Sprintf("%s:%s", resourceType, namespace)

	m.mutex.Lock()
	defer m.mutex.Unlock()

	if watcher, exists := m.watchers[key]; exists {
		// Cancel any pending cleanup
		if watcher.CleanupTimer != nil {
			watcher.CleanupTimer.Stop()
			watcher.CleanupTimer = nil
		}
		// Increment ref count
		atomic.AddInt32(&watcher.RefCount, 1)
		m.app.LogDebug("ResourceWatcher: Reusing existing watcher for %s (refCount=%d)", key, atomic.LoadInt32(&watcher.RefCount))
		return key
	}

	// Create new watcher
	ctx, cancel := context.WithCancel(context.Background())
	watcher := &ResourceWatcher{
		Key:          key,
		ResourceType: resourceType,
		Namespace:    namespace,
		IsCRD:        false,
		RefCount:     1,
		Cancel:       cancel,
	}
	m.watchers[key] = watcher

	m.app.LogDebug("ResourceWatcher: Starting new watcher for %s", key)

	// Start watch loop in goroutine
	go m.app.watchResourceLoop(ctx, resourceType, namespace)

	return key
}

// SubscribeCRD subscribes to a CRD watcher using GVR, returning the watcher key
func (m *ResourceWatcherManager) SubscribeCRD(group, version, resource, namespace string) string {
	key := fmt.Sprintf("crd:%s/%s/%s:%s", group, version, resource, namespace)

	m.mutex.Lock()
	defer m.mutex.Unlock()

	if watcher, exists := m.watchers[key]; exists {
		// Cancel any pending cleanup
		if watcher.CleanupTimer != nil {
			watcher.CleanupTimer.Stop()
			watcher.CleanupTimer = nil
		}
		// Increment ref count
		atomic.AddInt32(&watcher.RefCount, 1)
		m.app.LogDebug("ResourceWatcher: Reusing existing CRD watcher for %s (refCount=%d)", key, atomic.LoadInt32(&watcher.RefCount))
		return key
	}

	// Create new watcher
	ctx, cancel := context.WithCancel(context.Background())
	watcher := &ResourceWatcher{
		Key:          key,
		ResourceType: resource,
		Namespace:    namespace,
		Group:        group,
		Version:      version,
		Resource:     resource,
		IsCRD:        true,
		RefCount:     1,
		Cancel:       cancel,
	}
	m.watchers[key] = watcher

	m.app.LogDebug("ResourceWatcher: Starting new CRD watcher for %s", key)

	// Start watch loop in goroutine
	go m.app.watchCRDLoop(ctx, group, version, resource, namespace)

	return key
}

// Unsubscribe decrements the refCount for a watcher and schedules cleanup if refCount reaches 0
func (m *ResourceWatcherManager) Unsubscribe(watcherKey string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	watcher, exists := m.watchers[watcherKey]
	if !exists {
		m.app.LogDebug("ResourceWatcher: Unsubscribe called for non-existent watcher %s", watcherKey)
		return
	}

	newCount := atomic.AddInt32(&watcher.RefCount, -1)
	m.app.LogDebug("ResourceWatcher: Unsubscribe from %s (refCount=%d)", watcherKey, newCount)

	if newCount <= 0 {
		// Schedule cleanup after delay
		watcher.CleanupTimer = time.AfterFunc(WatcherCleanupDelay, func() {
			m.cleanup(watcherKey)
		})
		m.app.LogDebug("ResourceWatcher: Scheduled cleanup for %s in %v", watcherKey, WatcherCleanupDelay)
	}
}

// cleanup stops and removes a watcher (called after cleanup delay if refCount is still 0)
func (m *ResourceWatcherManager) cleanup(watcherKey string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	watcher, exists := m.watchers[watcherKey]
	if !exists {
		return
	}

	// Check if refCount is still 0 (someone might have subscribed during the delay)
	if atomic.LoadInt32(&watcher.RefCount) > 0 {
		m.app.LogDebug("ResourceWatcher: Cleanup cancelled for %s - new subscribers", watcherKey)
		return
	}

	// Stop the watcher
	if watcher.Cancel != nil {
		watcher.Cancel()
	}
	delete(m.watchers, watcherKey)
	m.app.LogDebug("ResourceWatcher: Cleaned up watcher %s", watcherKey)
}

// StopAll stops all active watchers immediately (called on context switch)
func (m *ResourceWatcherManager) StopAll() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.app.LogDebug("ResourceWatcher: Stopping all watchers (%d active)", len(m.watchers))

	for key, watcher := range m.watchers {
		// Cancel any pending cleanup timers
		if watcher.CleanupTimer != nil {
			watcher.CleanupTimer.Stop()
		}
		// Stop the watcher
		if watcher.Cancel != nil {
			watcher.Cancel()
		}
		m.app.LogDebug("ResourceWatcher: Stopped watcher %s", key)
	}

	// Clear all watchers
	m.watchers = make(map[string]*ResourceWatcher)
}

// NewApp creates a new App application struct
func NewApp() *App {
	client, err := k8s.NewClient()
	if err != nil {
		fmt.Printf("Error initializing K8s client: %v\n", err)
	}
	return &App{
		k8sClient:       client,
		terminalService: terminal.NewService(),
		logStreams:      make(map[string]context.CancelFunc),
	}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.watcherManager = NewResourceWatcherManager(ctx, a)
	if err := a.terminalService.Start(); err != nil {
		fmt.Printf("Failed to start terminal service: %v\n", err)
	}
}

// Greet returns a greeting for the given name
func (a *App) Greet(name string) string {
	return fmt.Sprintf("Hello %s, It's show time!", name)
}

// TestEmit emits a test debug log event
func (a *App) TestEmit() {
	a.LogDebug("TestEmit called from frontend")
}

// --- K8s Methods Exposed to Frontend ---

func (a *App) ListContexts() ([]string, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListContexts()
}

func (a *App) GetCurrentContext() string {
	if a.k8sClient == nil {
		return ""
	}
	return a.k8sClient.GetCurrentContext()
}

func (a *App) SwitchContext(name string) error {
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.SwitchContext(name)
}

func (a *App) ListPods(namespace string) ([]v1.Pod, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListPods(namespace)
}

func (a *App) ListNodes() ([]v1.Node, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListNodes()
}

func (a *App) GetNodeMetrics() (*k8s.NodeMetricsResult, error) {
	if a.k8sClient == nil {
		return &k8s.NodeMetricsResult{Available: false}, nil
	}
	return a.k8sClient.GetNodeMetrics()
}

func (a *App) GetPodMetrics() (*k8s.PodMetricsResult, error) {
	if a.k8sClient == nil {
		return &k8s.PodMetricsResult{Available: false}, nil
	}
	return a.k8sClient.GetPodMetrics()
}

func (a *App) GetNodeYaml(name string) (string, error) {
	a.LogDebug("GetNodeYaml called: name=%s", name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetNodeYaml(name)
}

func (a *App) UpdateNodeYaml(name, yamlContent string) error {
	a.LogDebug("UpdateNodeYaml called: name=%s", name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateNodeYaml(name, yamlContent)
}

func (a *App) DeleteNode(name string) error {
	currentContext := a.GetCurrentContext()
	a.LogDebug("DeleteNode called: context=%s, name=%s", currentContext, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteNode(currentContext, name)
}

func (a *App) SetNodeSchedulable(name string, schedulable bool) error {
	currentContext := a.GetCurrentContext()
	a.LogDebug("SetNodeSchedulable called: context=%s, name=%s, schedulable=%v", currentContext, name, schedulable)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.SetNodeSchedulable(currentContext, name, schedulable)
}

// NodeDebugPodResult contains the result of creating a debug pod for node shell access
type NodeDebugPodResult struct {
	PodName   string `json:"podName"`
	Namespace string `json:"namespace"`
}

func (a *App) CreateNodeDebugPod(nodeName string) (*NodeDebugPodResult, error) {
	currentContext := a.GetCurrentContext()
	a.LogDebug("CreateNodeDebugPod called: context=%s, nodeName=%s", currentContext, nodeName)
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	pod, err := a.k8sClient.CreateNodeDebugPod(currentContext, nodeName)
	if err != nil {
		return nil, err
	}
	return &NodeDebugPodResult{
		PodName:   pod.Name,
		Namespace: pod.Namespace,
	}, nil
}

func (a *App) ListNamespaces() ([]v1.Namespace, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListNamespaces()
}

func (a *App) DeleteNamespace(name string) error {
	a.LogDebug("DeleteNamespace called: name=%s", name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteNamespace(name)
}

func (a *App) GetNamespaceYAML(name string) (string, error) {
	a.LogDebug("GetNamespaceYAML called: name=%s", name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetNamespaceYAML(name)
}

func (a *App) UpdateNamespaceYAML(name string, yamlContent string) error {
	a.LogDebug("UpdateNamespaceYAML called: name=%s", name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateNamespaceYAML(name, yamlContent)
}

func (a *App) ListEvents(namespace string) ([]v1.Event, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListEvents(namespace)
}

func (a *App) GetEventYAML(namespace, name string) (string, error) {
	a.LogDebug("GetEventYAML called: namespace=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetEventYAML(namespace, name)
}

func (a *App) UpdateEventYAML(namespace, name string, yamlContent string) error {
	a.LogDebug("UpdateEventYAML called: namespace=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateEventYAML(namespace, name, yamlContent)
}

func (a *App) DeleteEvent(namespace, name string) error {
	a.LogDebug("DeleteEvent called: namespace=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteEvent(namespace, name)
}

func (a *App) ListServices(namespace string) ([]v1.Service, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListServices(namespace)
}

func (a *App) GetServiceYaml(namespace, name string) (string, error) {
	a.LogDebug("GetServiceYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetServiceYaml(namespace, name)
}

func (a *App) UpdateServiceYaml(namespace, name, yamlContent string) error {
	a.LogDebug("UpdateServiceYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateServiceYaml(namespace, name, yamlContent)
}

func (a *App) DeleteService(namespace, name string) error {
	currentContext := a.GetCurrentContext()
	a.LogDebug("DeleteService called: context=%s, ns=%s, name=%s", currentContext, namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteService(currentContext, namespace, name)
}

// Ingress operations
func (a *App) ListIngresses(namespace string) ([]networkingv1.Ingress, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListIngresses(namespace)
}

func (a *App) GetIngressYaml(namespace, name string) (string, error) {
	a.LogDebug("GetIngressYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetIngressYaml(namespace, name)
}

func (a *App) UpdateIngressYaml(namespace, name, yamlContent string) error {
	a.LogDebug("UpdateIngressYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateIngressYaml(namespace, name, yamlContent)
}

func (a *App) DeleteIngress(namespace, name string) error {
	currentContext := a.GetCurrentContext()
	a.LogDebug("DeleteIngress called: context=%s, ns=%s, name=%s", currentContext, namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteIngress(currentContext, namespace, name)
}

// IngressClass operations (cluster-scoped)
func (a *App) ListIngressClasses() ([]networkingv1.IngressClass, error) {
	currentContext := a.GetCurrentContext()
	a.LogDebug("ListIngressClasses called: context=%s", currentContext)
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListIngressClasses(currentContext)
}

func (a *App) GetIngressClassYaml(name string) (string, error) {
	a.LogDebug("GetIngressClassYaml called: name=%s", name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetIngressClassYaml(name)
}

func (a *App) UpdateIngressClassYaml(name, yamlContent string) error {
	a.LogDebug("UpdateIngressClassYaml called: name=%s", name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateIngressClassYaml(name, yamlContent)
}

func (a *App) DeleteIngressClass(name string) error {
	currentContext := a.GetCurrentContext()
	a.LogDebug("DeleteIngressClass called: context=%s, name=%s", currentContext, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteIngressClass(currentContext, name)
}

func (a *App) ListConfigMaps(namespace string) ([]v1.ConfigMap, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListConfigMaps(namespace)
}

func (a *App) ListSecrets(namespace string) ([]v1.Secret, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListSecrets(namespace)
}

// ConfigMap YAML operations
func (a *App) GetConfigMapYaml(namespace, name string) (string, error) {
	a.LogDebug("GetConfigMapYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetConfigMapYaml(namespace, name)
}

func (a *App) UpdateConfigMapYaml(namespace, name, yamlContent string) error {
	a.LogDebug("UpdateConfigMapYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateConfigMapYaml(namespace, name, yamlContent)
}

func (a *App) DeleteConfigMap(namespace, name string) error {
	a.LogDebug("DeleteConfigMap called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteConfigMap(namespace, name)
}

// Secret YAML operations
func (a *App) GetSecretYaml(namespace, name string) (string, error) {
	a.LogDebug("GetSecretYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetSecretYaml(namespace, name)
}

func (a *App) UpdateSecretYaml(namespace, name, yamlContent string) error {
	a.LogDebug("UpdateSecretYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateSecretYaml(namespace, name, yamlContent)
}

func (a *App) DeleteSecret(namespace, name string) error {
	a.LogDebug("DeleteSecret called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteSecret(namespace, name)
}

func (a *App) GetSecretData(namespace, name string) (map[string]string, error) {
	a.LogDebug("GetSecretData called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetSecretData(namespace, name)
}

func (a *App) UpdateSecretData(namespace, name string, data map[string]string) error {
	a.LogDebug("UpdateSecretData called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateSecretData(namespace, name, data)
}

func (a *App) ListDeployments(namespace string) ([]appsv1.Deployment, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListDeployments(namespace)
}

func (a *App) GetPodLogs(namespace, podName, containerName string, timestamps bool, previous bool, sinceTime string) (string, error) {
	currentContext := a.GetCurrentContext()
	a.LogDebug("GetPodLogs called: context=%s, ns=%s, pod=%s, container=%s, timestamps=%v, previous=%v, sinceTime=%s", currentContext, namespace, podName, containerName, timestamps, previous, sinceTime)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetPodLogs(namespace, podName, containerName, timestamps, previous, sinceTime)
}

func (a *App) GetAllPodLogs(namespace, podName, containerName string, timestamps bool, previous bool) (string, error) {
	currentContext := a.GetCurrentContext()
	a.LogDebug("GetAllPodLogs called: context=%s, ns=%s, pod=%s, container=%s, timestamps=%v, previous=%v", currentContext, namespace, podName, containerName, timestamps, previous)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetAllPodLogs(namespace, podName, containerName, timestamps, previous)
}

func (a *App) GetPodLogsFromStart(namespace, podName, containerName string, timestamps bool, previous bool) (string, error) {
	currentContext := a.GetCurrentContext()
	a.LogDebug("GetPodLogsFromStart called: context=%s, ns=%s, pod=%s, container=%s, timestamps=%v, previous=%v", currentContext, namespace, podName, containerName, timestamps, previous)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetPodLogsFromStart(namespace, podName, containerName, timestamps, previous, 200)
}

// LogChunkResult represents the result of a chunked log fetch
type LogChunkResult struct {
	Logs    string `json:"logs"`
	HasMore bool   `json:"hasMore"`
}

func (a *App) GetPodLogsBefore(namespace, podName, containerName string, timestamps bool, previous bool, beforeTime string, limit int) (*LogChunkResult, error) {
	currentContext := a.GetCurrentContext()
	a.LogDebug("GetPodLogsBefore called: context=%s, ns=%s, pod=%s, container=%s, beforeTime=%s, limit=%d", currentContext, namespace, podName, containerName, beforeTime, limit)
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	logs, hasMore, err := a.k8sClient.GetPodLogsBefore(namespace, podName, containerName, timestamps, previous, beforeTime, limit)
	if err != nil {
		return nil, err
	}
	return &LogChunkResult{Logs: logs, HasMore: hasMore}, nil
}

func (a *App) GetPodLogsAfter(namespace, podName, containerName string, timestamps bool, previous bool, afterTime string, limit int) (*LogChunkResult, error) {
	currentContext := a.GetCurrentContext()
	a.LogDebug("GetPodLogsAfter called: context=%s, ns=%s, pod=%s, container=%s, afterTime=%s, limit=%d", currentContext, namespace, podName, containerName, afterTime, limit)
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	logs, hasMore, err := a.k8sClient.GetPodLogsAfter(namespace, podName, containerName, timestamps, previous, afterTime, limit)
	if err != nil {
		return nil, err
	}
	return &LogChunkResult{Logs: logs, HasMore: hasMore}, nil
}

// LogStreamEvent is emitted for each log line during streaming
type LogStreamEvent struct {
	StreamID string `json:"streamId"`
	Line     string `json:"line"`
	Error    string `json:"error,omitempty"`
	Done     bool   `json:"done,omitempty"`
}

// StartLogStream starts streaming logs from a pod container.
// Returns a stream ID that can be used to stop the stream.
// Log lines are emitted via "log-stream" events.
func (a *App) StartLogStream(namespace, podName, containerName string, timestamps bool) (string, error) {
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}

	// Generate a unique stream ID
	streamID := fmt.Sprintf("%s/%s/%s-%d", namespace, podName, containerName, time.Now().UnixNano())
	a.LogDebug("StartLogStream: streamID=%s", streamID)

	// Create a cancellable context for this stream
	ctx, cancel := context.WithCancel(context.Background())

	// Store the cancel function
	a.logStreamsMutex.Lock()
	a.logStreams[streamID] = cancel
	a.logStreamsMutex.Unlock()

	// Start streaming in a goroutine
	go func() {
		defer func() {
			// Clean up when done
			a.logStreamsMutex.Lock()
			delete(a.logStreams, streamID)
			a.logStreamsMutex.Unlock()

			// Emit done event
			runtime.EventsEmit(a.ctx, "log-stream", LogStreamEvent{
				StreamID: streamID,
				Done:     true,
			})
		}()

		err := a.k8sClient.StreamPodLogs(ctx, namespace, podName, containerName, timestamps, 200, func(line string) {
			runtime.EventsEmit(a.ctx, "log-stream", LogStreamEvent{
				StreamID: streamID,
				Line:     line,
			})
		})

		if err != nil && err != context.Canceled {
			a.LogDebug("Log stream error: %v", err)
			runtime.EventsEmit(a.ctx, "log-stream", LogStreamEvent{
				StreamID: streamID,
				Error:    err.Error(),
			})
		}
	}()

	return streamID, nil
}

// StopLogStream stops an active log stream
func (a *App) StopLogStream(streamID string) {
	a.LogDebug("StopLogStream: streamID=%s", streamID)
	a.logStreamsMutex.Lock()
	defer a.logStreamsMutex.Unlock()

	if cancel, ok := a.logStreams[streamID]; ok {
		cancel()
		delete(a.logStreams, streamID)
	}
}

// LogDebug sends a debug message to the frontend
func (a *App) LogDebug(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Println("DEBUG:", msg)
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "debug-log", msg)
	}
}

func (a *App) DeletePod(contextName, namespace, name string) error {
	a.LogDebug("DeletePod called: context=%s, ns=%s, name=%s", contextName, namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	err := a.k8sClient.DeletePod(contextName, namespace, name)
	if err != nil {
		a.LogDebug("DeletePod error: %v", err)
	} else {
		a.LogDebug("DeletePod success")
	}
	return err
}

func (a *App) ForceDeletePod(contextName, namespace, name string) error {
	a.LogDebug("ForceDeletePod called: context=%s, ns=%s, name=%s", contextName, namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	err := a.k8sClient.ForceDeletePod(contextName, namespace, name)
	if err != nil {
		a.LogDebug("ForceDeletePod error: %v", err)
	} else {
		a.LogDebug("ForceDeletePod success")
	}
	return err
}

func (a *App) GetPodYaml(namespace, name string) (string, error) {
	a.LogDebug("GetPodYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetPodYaml(namespace, name)
}

func (a *App) UpdatePodYaml(namespace, name, yamlContent string) error {
	a.LogDebug("UpdatePodYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdatePodYaml(namespace, name, yamlContent)
}

func (a *App) OpenTerminal(contextName, namespace, podName, containerName string) (string, error) {
	a.LogDebug("OpenTerminal called: context=%s, ns=%s, pod=%s, container=%s", contextName, namespace, podName, containerName)
	if a.terminalService == nil {
		return "", fmt.Errorf("terminal service not initialized")
	}

	// Generate a unique ID for the terminal session (unused for now, but good for future)
	// terminalID := fmt.Sprintf("%s-%s-%s", namespace, podName, containerName)
	url := fmt.Sprintf("ws://localhost:%d/terminal?context=%s&namespace=%s&pod=%s&container=%s",
		a.terminalService.Port, contextName, namespace, podName, containerName)

	return url, nil
}

func (a *App) OpenTerminalWithCommand(contextName, namespace, podName, containerName, command string) (string, error) {
	a.LogDebug("OpenTerminalWithCommand called: context=%s, ns=%s, pod=%s, container=%s, cmd=%s", contextName, namespace, podName, containerName, command)
	if a.terminalService == nil {
		return "", fmt.Errorf("terminal service not initialized")
	}

	wsURL := fmt.Sprintf("ws://localhost:%d/terminal?context=%s&namespace=%s&pod=%s&container=%s&command=%s",
		a.terminalService.Port, contextName, namespace, podName, containerName, url.QueryEscape(command))

	return wsURL, nil
}

// --- Generic Resource Watcher (exposed to frontend) ---

// SubscribeResourceWatcher subscribes to a resource watcher, returning the watcher key
func (a *App) SubscribeResourceWatcher(resourceType, namespace string) string {
	if a.watcherManager == nil {
		a.LogDebug("SubscribeResourceWatcher: watcher manager not initialized")
		return ""
	}
	return a.watcherManager.Subscribe(resourceType, namespace)
}

// SubscribeCRDWatcher subscribes to a CRD watcher using GVR, returning the watcher key
func (a *App) SubscribeCRDWatcher(group, version, resource, namespace string) string {
	if a.watcherManager == nil {
		a.LogDebug("SubscribeCRDWatcher: watcher manager not initialized")
		return ""
	}
	return a.watcherManager.SubscribeCRD(group, version, resource, namespace)
}

// UnsubscribeWatcher unsubscribes from a watcher by key
func (a *App) UnsubscribeWatcher(watcherKey string) {
	if a.watcherManager == nil {
		a.LogDebug("UnsubscribeWatcher: watcher manager not initialized")
		return
	}
	a.watcherManager.Unsubscribe(watcherKey)
}

// StopAllWatchers stops all active watchers (called on context switch)
func (a *App) StopAllWatchers() {
	if a.watcherManager == nil {
		a.LogDebug("StopAllWatchers: watcher manager not initialized")
		return
	}
	a.watcherManager.StopAll()
}

// watchResourceLoop is the generic watch loop for standard Kubernetes resources
func (a *App) watchResourceLoop(ctx context.Context, resourceType, namespace string) {
	defer func() {
		a.LogDebug("Resource watcher stopped: type=%s, namespace=%s", resourceType, namespace)
	}()

	if a.k8sClient == nil {
		a.LogDebug("watchResourceLoop: k8s client not initialized")
		return
	}

	watcher, err := a.k8sClient.WatchResource(ctx, resourceType, namespace)
	if err != nil {
		a.LogDebug("Failed to start resource watcher: type=%s, err=%v", resourceType, err)
		return
	}
	defer watcher.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-watcher.ResultChan():
			if !ok {
				a.LogDebug("Resource watcher channel closed: type=%s", resourceType)
				return
			}

			// Only emit ADDED, MODIFIED, DELETED events
			if event.Type == "ADDED" || event.Type == "MODIFIED" || event.Type == "DELETED" {
				// Convert to unstructured map
				resourceMap, err := k8s.RuntimeObjectToMap(event.Object)
				if err != nil {
					a.LogDebug("Failed to convert resource to map: %v", err)
					continue
				}

				// Extract namespace from resource metadata
				resourceNs := ""
				if metadata, ok := resourceMap["metadata"].(map[string]interface{}); ok {
					if ns, ok := metadata["namespace"].(string); ok {
						resourceNs = ns
					}
				}

				runtime.EventsEmit(a.ctx, "resource-event", ResourceEvent{
					Type:         string(event.Type),
					ResourceType: resourceType,
					Namespace:    resourceNs,
					Resource:     resourceMap,
				})
			}
		}
	}
}

// watchCRDLoop is the watch loop for Custom Resource Definitions using dynamic client
func (a *App) watchCRDLoop(ctx context.Context, group, version, resource, namespace string) {
	defer func() {
		a.LogDebug("CRD watcher stopped: gvr=%s/%s/%s, namespace=%s", group, version, resource, namespace)
	}()

	if a.k8sClient == nil {
		a.LogDebug("watchCRDLoop: k8s client not initialized")
		return
	}

	watcher, err := a.k8sClient.WatchCRD(ctx, group, version, resource, namespace)
	if err != nil {
		a.LogDebug("Failed to start CRD watcher: gvr=%s/%s/%s, err=%v", group, version, resource, err)
		return
	}
	defer watcher.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-watcher.ResultChan():
			if !ok {
				a.LogDebug("CRD watcher channel closed: gvr=%s/%s/%s", group, version, resource)
				return
			}

			// Only emit ADDED, MODIFIED, DELETED events
			if event.Type == "ADDED" || event.Type == "MODIFIED" || event.Type == "DELETED" {
				// Convert to unstructured map
				resourceMap, err := k8s.RuntimeObjectToMap(event.Object)
				if err != nil {
					a.LogDebug("Failed to convert CRD to map: %v", err)
					continue
				}

				// Extract namespace from resource metadata
				resourceNs := ""
				if metadata, ok := resourceMap["metadata"].(map[string]interface{}); ok {
					if ns, ok := metadata["namespace"].(string); ok {
						resourceNs = ns
					}
				}

				// Use the resource plural name as resourceType for CRDs
				runtime.EventsEmit(a.ctx, "resource-event", ResourceEvent{
					Type:         string(event.Type),
					ResourceType: fmt.Sprintf("crd:%s/%s/%s", group, version, resource),
					Namespace:    resourceNs,
					Resource:     resourceMap,
				})
			}
		}
	}
}

func (a *App) GetDeploymentYaml(namespace, name string) (string, error) {
	a.LogDebug("GetDeploymentYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetDeploymentYaml(namespace, name)
}

func (a *App) UpdateDeploymentYaml(namespace, name, yamlContent string) error {
	a.LogDebug("UpdateDeploymentYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateDeploymentYaml(namespace, name, yamlContent)
}

func (a *App) DeleteDeployment(contextName, namespace, name string) error {
	a.LogDebug("DeleteDeployment called: context=%s, ns=%s, name=%s", contextName, namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	err := a.k8sClient.DeleteDeployment(contextName, namespace, name)
	if err != nil {
		a.LogDebug("DeleteDeployment error: %v", err)
	} else {
		a.LogDebug("DeleteDeployment success")
	}
	return err
}

func (a *App) RestartDeployment(contextName, namespace, name string) error {
	a.LogDebug("RestartDeployment called: context=%s, ns=%s, name=%s", contextName, namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	err := a.k8sClient.RestartDeployment(contextName, namespace, name)
	if err != nil {
		a.LogDebug("RestartDeployment error: %v", err)
	} else {
		a.LogDebug("RestartDeployment success")
	}
	return err
}

// StatefulSet operations
func (a *App) ListStatefulSets(contextName, namespace string) ([]appsv1.StatefulSet, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListStatefulSets(contextName, namespace)
}

func (a *App) GetStatefulSetYaml(namespace, name string) (string, error) {
	a.LogDebug("GetStatefulSetYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetStatefulSetYaml(namespace, name)
}

func (a *App) UpdateStatefulSetYaml(namespace, name, yamlContent string) error {
	a.LogDebug("UpdateStatefulSetYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateStatefulSetYaml(namespace, name, yamlContent)
}

// DaemonSet wrappers
func (a *App) ListDaemonSets(namespace string) ([]appsv1.DaemonSet, error) {
	currentContext := a.GetCurrentContext()
	a.LogDebug("ListDaemonSets called: context=%s, ns=%s", currentContext, namespace)
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListDaemonSets(currentContext, namespace)
}

func (a *App) GetDaemonSetYaml(namespace, name string) (string, error) {
	a.LogDebug("GetDaemonSetYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetDaemonSetYaml(namespace, name)
}

func (a *App) UpdateDaemonSetYaml(namespace, name, yamlContent string) error {
	a.LogDebug("UpdateDaemonSetYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateDaemonSetYaml(namespace, name, yamlContent)
}

func (a *App) RestartDaemonSet(namespace, name string) error {
	currentContext := a.GetCurrentContext()
	a.LogDebug("RestartDaemonSet called: context=%s, ns=%s, name=%s", currentContext, namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.RestartDaemonSet(currentContext, namespace, name)
}

func (a *App) DeleteDaemonSet(namespace, name string) error {
	currentContext := a.GetCurrentContext()
	a.LogDebug("DeleteDaemonSet called: context=%s, ns=%s, name=%s", currentContext, namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteDaemonSet(currentContext, namespace, name)
}

// ReplicaSet wrappers
func (a *App) ListReplicaSets(namespace string) ([]appsv1.ReplicaSet, error) {
	currentContext := a.GetCurrentContext()
	a.LogDebug("ListReplicaSets called: context=%s, ns=%s", currentContext, namespace)
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListReplicaSets(currentContext, namespace)
}

func (a *App) GetReplicaSetYaml(namespace, name string) (string, error) {
	a.LogDebug("GetReplicaSetYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetReplicaSetYaml(namespace, name)
}

func (a *App) UpdateReplicaSetYaml(namespace, name, yamlContent string) error {
	a.LogDebug("UpdateReplicaSetYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateReplicaSetYaml(namespace, name, yamlContent)
}

func (a *App) DeleteReplicaSet(namespace, name string) error {
	currentContext := a.GetCurrentContext()
	a.LogDebug("DeleteReplicaSet called: context=%s, ns=%s, name=%s", currentContext, namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteReplicaSet(currentContext, namespace, name)
}

func (a *App) RestartStatefulSet(contextName, namespace, name string) error {
	a.LogDebug("RestartStatefulSet called: context=%s, ns=%s, name=%s", contextName, namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	err := a.k8sClient.RestartStatefulSet(contextName, namespace, name)
	if err != nil {
		a.LogDebug("RestartStatefulSet error: %v", err)
	} else {
		a.LogDebug("RestartStatefulSet success")
	}
	return err
}

func (a *App) DeleteStatefulSet(contextName, namespace, name string) error {
	a.LogDebug("DeleteStatefulSet called: context=%s, ns=%s, name=%s", contextName, namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	err := a.k8sClient.DeleteStatefulSet(contextName, namespace, name)
	if err != nil {
		a.LogDebug("DeleteStatefulSet error: %v", err)
	} else {
		a.LogDebug("DeleteStatefulSet success")
	}
	return err
}

func (a *App) SaveLogFile(content string) error {
	filePath, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: "kubikles-debug-logs.txt",
		Title:           "Save Debug Logs",
		Filters: []runtime.FileFilter{
			{
				DisplayName: "Text Files (*.txt)",
				Pattern:     "*.txt",
			},
		},
	})

	if err != nil {
		return err
	}

	if filePath == "" {
		return nil // User cancelled
	}

	return os.WriteFile(filePath, []byte(content), 0644)
}

func (a *App) SavePodLogs(content string, defaultFilename string) error {
	filePath, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: defaultFilename,
		Title:           "Save Pod Logs",
		Filters: []runtime.FileFilter{
			{
				DisplayName: "Log Files (*.log)",
				Pattern:     "*.log",
			},
			{
				DisplayName: "Text Files (*.txt)",
				Pattern:     "*.txt",
			},
		},
	})

	if err != nil {
		return err
	}

	if filePath == "" {
		return nil // User cancelled
	}

	return os.WriteFile(filePath, []byte(content), 0644)
}

// PodLogEntry represents a single container's logs for the bundle
type PodLogEntry struct {
	PodName       string `json:"podName"`
	ContainerName string `json:"containerName"`
	Logs          string `json:"logs"`
}

// SaveLogsBundle saves multiple pod logs as a zip file
func (a *App) SaveLogsBundle(entries []PodLogEntry, defaultFilename string) error {
	filePath, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: defaultFilename,
		Title:           "Save Logs Bundle",
		Filters: []runtime.FileFilter{
			{
				DisplayName: "Zip Files (*.zip)",
				Pattern:     "*.zip",
			},
		},
	})

	if err != nil {
		return err
	}

	if filePath == "" {
		return nil // User cancelled
	}

	// Create the zip file
	zipFile, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create zip file: %w", err)
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	for _, entry := range entries {
		// Create path: podName/containerName.log
		logPath := fmt.Sprintf("%s/%s.log", entry.PodName, entry.ContainerName)
		writer, err := zipWriter.Create(logPath)
		if err != nil {
			return fmt.Errorf("failed to create zip entry %s: %w", logPath, err)
		}
		_, err = writer.Write([]byte(entry.Logs))
		if err != nil {
			return fmt.Errorf("failed to write logs for %s: %w", logPath, err)
		}
	}

	return nil
}

// Job operations
func (a *App) ListJobs(namespace string) ([]batchv1.Job, error) {
	currentContext := a.GetCurrentContext()
	a.LogDebug("ListJobs called: context=%s, ns=%s", currentContext, namespace)
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListJobs(currentContext, namespace)
}

func (a *App) GetJobYaml(namespace, name string) (string, error) {
	a.LogDebug("GetJobYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetJobYaml(namespace, name)
}

func (a *App) UpdateJobYaml(namespace, name, yamlContent string) error {
	a.LogDebug("UpdateJobYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateJobYaml(namespace, name, yamlContent)
}

func (a *App) DeleteJob(namespace, name string) error {
	currentContext := a.GetCurrentContext()
	a.LogDebug("DeleteJob called: context=%s, ns=%s, name=%s", currentContext, namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteJob(currentContext, namespace, name)
}

// CronJob operations
func (a *App) ListCronJobs(namespace string) ([]batchv1.CronJob, error) {
	currentContext := a.GetCurrentContext()
	a.LogDebug("ListCronJobs called: context=%s, ns=%s", currentContext, namespace)
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListCronJobs(currentContext, namespace)
}

func (a *App) GetCronJobYaml(namespace, name string) (string, error) {
	a.LogDebug("GetCronJobYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetCronJobYaml(namespace, name)
}

func (a *App) UpdateCronJobYaml(namespace, name, yamlContent string) error {
	a.LogDebug("UpdateCronJobYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateCronJobYaml(namespace, name, yamlContent)
}

func (a *App) DeleteCronJob(namespace, name string) error {
	currentContext := a.GetCurrentContext()
	a.LogDebug("DeleteCronJob called: context=%s, ns=%s, name=%s", currentContext, namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteCronJob(currentContext, namespace, name)
}

func (a *App) TriggerCronJob(namespace, name string) error {
	currentContext := a.GetCurrentContext()
	a.LogDebug("TriggerCronJob called: context=%s, ns=%s, name=%s", currentContext, namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.TriggerCronJob(currentContext, namespace, name)
}

func (a *App) SuspendCronJob(namespace, name string, suspend bool) error {
	currentContext := a.GetCurrentContext()
	a.LogDebug("SuspendCronJob called: context=%s, ns=%s, name=%s, suspend=%v", currentContext, namespace, name, suspend)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.SuspendCronJob(currentContext, namespace, name, suspend)
}

// PersistentVolumeClaim operations
func (a *App) ListPVCs(namespace string) ([]v1.PersistentVolumeClaim, error) {
	currentContext := a.GetCurrentContext()
	a.LogDebug("ListPVCs called: context=%s, ns=%s", currentContext, namespace)
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListPVCs(currentContext, namespace)
}

func (a *App) GetPVCYaml(namespace, name string) (string, error) {
	a.LogDebug("GetPVCYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetPVCYaml(namespace, name)
}

func (a *App) UpdatePVCYaml(namespace, name, yamlContent string) error {
	a.LogDebug("UpdatePVCYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdatePVCYaml(namespace, name, yamlContent)
}

func (a *App) DeletePVC(namespace, name string) error {
	currentContext := a.GetCurrentContext()
	a.LogDebug("DeletePVC called: context=%s, ns=%s, name=%s", currentContext, namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeletePVC(currentContext, namespace, name)
}

// PersistentVolume operations (cluster-scoped)
func (a *App) ListPVs() ([]v1.PersistentVolume, error) {
	currentContext := a.GetCurrentContext()
	a.LogDebug("ListPVs called: context=%s", currentContext)
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListPVs(currentContext)
}

func (a *App) GetPVYaml(name string) (string, error) {
	a.LogDebug("GetPVYaml called: name=%s", name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetPVYaml(name)
}

func (a *App) UpdatePVYaml(name, yamlContent string) error {
	a.LogDebug("UpdatePVYaml called: name=%s", name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdatePVYaml(name, yamlContent)
}

func (a *App) DeletePV(name string) error {
	currentContext := a.GetCurrentContext()
	a.LogDebug("DeletePV called: context=%s, name=%s", currentContext, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeletePV(currentContext, name)
}

// StorageClass operations (cluster-scoped)
func (a *App) ListStorageClasses() ([]storagev1.StorageClass, error) {
	currentContext := a.GetCurrentContext()
	a.LogDebug("ListStorageClasses called: context=%s", currentContext)
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListStorageClasses(currentContext)
}

func (a *App) GetStorageClassYaml(name string) (string, error) {
	a.LogDebug("GetStorageClassYaml called: name=%s", name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetStorageClassYaml(name)
}

func (a *App) UpdateStorageClassYaml(name, yamlContent string) error {
	a.LogDebug("UpdateStorageClassYaml called: name=%s", name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateStorageClassYaml(name, yamlContent)
}

func (a *App) DeleteStorageClass(name string) error {
	currentContext := a.GetCurrentContext()
	a.LogDebug("DeleteStorageClass called: context=%s, name=%s", currentContext, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteStorageClass(currentContext, name)
}

// GetResourceDependencies returns the dependency graph for a given resource
func (a *App) GetResourceDependencies(resourceType, namespace, name string) (*k8s.DependencyGraph, error) {
	currentContext := a.GetCurrentContext()
	a.LogDebug("GetResourceDependencies called: context=%s, type=%s, ns=%s, name=%s", currentContext, resourceType, namespace, name)
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetResourceDependencies(currentContext, resourceType, namespace, name)
}

// CustomResourceDefinition operations (cluster-scoped)
func (a *App) ListCRDs() ([]apiextensionsv1.CustomResourceDefinition, error) {
	currentContext := a.GetCurrentContext()
	a.LogDebug("ListCRDs called: context=%s", currentContext)
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListCRDs(currentContext)
}

func (a *App) GetCRDYaml(name string) (string, error) {
	currentContext := a.GetCurrentContext()
	a.LogDebug("GetCRDYaml called: name=%s", name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetCRDYaml(currentContext, name)
}

func (a *App) UpdateCRDYaml(name, yamlContent string) error {
	currentContext := a.GetCurrentContext()
	a.LogDebug("UpdateCRDYaml called: name=%s", name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateCRDYaml(currentContext, name, yamlContent)
}

func (a *App) DeleteCRD(name string) error {
	currentContext := a.GetCurrentContext()
	a.LogDebug("DeleteCRD called: context=%s, name=%s", currentContext, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteCRD(currentContext, name)
}

// Custom Resource instance operations (dynamic)
func (a *App) ListCustomResources(group, version, resource, namespace string) ([]map[string]interface{}, error) {
	currentContext := a.GetCurrentContext()
	a.LogDebug("ListCustomResources called: context=%s, gvr=%s/%s/%s, ns=%s", currentContext, group, version, resource, namespace)
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListCustomResources(currentContext, group, version, resource, namespace)
}

func (a *App) GetCustomResourceYaml(group, version, resource, namespace, name string) (string, error) {
	currentContext := a.GetCurrentContext()
	a.LogDebug("GetCustomResourceYaml called: gvr=%s/%s/%s, ns=%s, name=%s", group, version, resource, namespace, name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetCustomResourceYaml(currentContext, group, version, resource, namespace, name)
}

func (a *App) UpdateCustomResourceYaml(group, version, resource, namespace, name, yamlContent string) error {
	currentContext := a.GetCurrentContext()
	a.LogDebug("UpdateCustomResourceYaml called: gvr=%s/%s/%s, ns=%s, name=%s", group, version, resource, namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateCustomResourceYaml(currentContext, group, version, resource, namespace, name, yamlContent)
}

func (a *App) DeleteCustomResource(group, version, resource, namespace, name string) error {
	currentContext := a.GetCurrentContext()
	a.LogDebug("DeleteCustomResource called: context=%s, gvr=%s/%s/%s, ns=%s, name=%s", currentContext, group, version, resource, namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteCustomResource(currentContext, group, version, resource, namespace, name)
}
