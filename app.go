package main

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"kubikles/pkg/helm"
	"kubikles/pkg/k8s"
	"kubikles/pkg/terminal"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	schedulingv1 "k8s.io/api/scheduling/v1"
	storagev1 "k8s.io/api/storage/v1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

// App struct
type App struct {
	ctx                   context.Context
	k8sClient             *k8s.Client
	helmClient            *helm.Client
	terminalService       *terminal.Service
	watcherManager        *ResourceWatcherManager
	portForwardManager    *PortForwardManager
	ingressForwardManager *IngressForwardManager
	// Log streaming
	logStreams      map[string]context.CancelFunc
	logStreamsMutex sync.Mutex
	// Prometheus config cache (per context)
	prometheusConfigs     map[string]*k8s.PrometheusInfo
	prometheusConfigPath  string
	prometheusConfigMutex sync.RWMutex
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

// WatcherErrorEvent is emitted when a watcher encounters an error
type WatcherErrorEvent struct {
	ResourceType string `json:"resourceType"` // Resource type that failed
	Namespace    string `json:"namespace"`    // Namespace being watched
	Error        string `json:"error"`        // Error message
	Recoverable  bool   `json:"recoverable"`  // Whether the watcher will retry
}

// WatcherStatusEvent is emitted when watcher status changes
type WatcherStatusEvent struct {
	ResourceType string `json:"resourceType"` // Resource type
	Namespace    string `json:"namespace"`    // Namespace being watched
	Status       string `json:"status"`       // "connected", "reconnecting", "stopped"
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

	// Setup prometheus config path
	configDir, _ := os.UserConfigDir()
	appDir := filepath.Join(configDir, "kubikles")
	os.MkdirAll(appDir, 0755)

	return &App{
		k8sClient:            client,
		helmClient:           helm.NewClient(),
		terminalService:      terminal.NewService(),
		logStreams:           make(map[string]context.CancelFunc),
		prometheusConfigs:    make(map[string]*k8s.PrometheusInfo),
		prometheusConfigPath: filepath.Join(appDir, "prometheus_config.json"),
	}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.watcherManager = NewResourceWatcherManager(ctx, a)
	a.portForwardManager = NewPortForwardManager(a)
	a.ingressForwardManager = NewIngressForwardManager(a)
	a.loadPrometheusConfigs()
	if err := a.terminalService.Start(); err != nil {
		fmt.Printf("Failed to start terminal service: %v\n", err)
	}
}

// shutdown is called when the app is closing
func (a *App) shutdown(ctx context.Context) {
	a.LogDebug("App shutdown initiated")

	// Clean up ingress forwarding (removes hosts file entries)
	if a.ingressForwardManager != nil {
		a.ingressForwardManager.Cleanup()
	}

	// Save port forward running state and stop all forwards
	if a.portForwardManager != nil {
		a.portForwardManager.StopAllAndSaveState()
	}

	// Stop all watchers
	if a.watcherManager != nil {
		a.watcherManager.StopAll()
	}

	a.LogDebug("App shutdown complete")
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

	// Stop all existing watchers before switching context
	// This prevents stale events from the old context being processed
	if a.watcherManager != nil {
		a.LogDebug("SwitchContext: Stopping all watchers before context switch")
		a.watcherManager.StopAll()
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

func (a *App) GetNamespaceResourceCounts(namespace string) (*k8s.NamespaceResourceCounts, error) {
	a.LogDebug("GetNamespaceResourceCounts called: namespace=%s", namespace)
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetNamespaceResourceCounts(namespace)
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
// It includes reconnection logic with exponential backoff
func (a *App) watchResourceLoop(ctx context.Context, resourceType, namespace string) {
	defer func() {
		a.LogDebug("Resource watcher stopped: type=%s, namespace=%s", resourceType, namespace)
		runtime.EventsEmit(a.ctx, "watcher-status", WatcherStatusEvent{
			ResourceType: resourceType,
			Namespace:    namespace,
			Status:       "stopped",
		})
	}()

	if a.k8sClient == nil {
		a.LogDebug("watchResourceLoop: k8s client not initialized")
		return
	}

	// Reconnection parameters
	maxRetries := 5
	baseDelay := 1 * time.Second
	maxDelay := 30 * time.Second

	for retryCount := 0; ; retryCount++ {
		// Check if context is cancelled before starting/reconnecting
		select {
		case <-ctx.Done():
			return
		default:
		}

		watcher, err := a.k8sClient.WatchResource(ctx, resourceType, namespace)
		if err != nil {
			a.LogDebug("Failed to start resource watcher: type=%s, err=%v, retry=%d", resourceType, err, retryCount)

			// Emit error event
			runtime.EventsEmit(a.ctx, "watcher-error", WatcherErrorEvent{
				ResourceType: resourceType,
				Namespace:    namespace,
				Error:        err.Error(),
				Recoverable:  retryCount < maxRetries,
			})

			if retryCount >= maxRetries {
				a.LogDebug("Resource watcher max retries exceeded: type=%s", resourceType)
				return
			}

			// Exponential backoff
			delay := baseDelay * time.Duration(1<<uint(retryCount))
			if delay > maxDelay {
				delay = maxDelay
			}

			runtime.EventsEmit(a.ctx, "watcher-status", WatcherStatusEvent{
				ResourceType: resourceType,
				Namespace:    namespace,
				Status:       "reconnecting",
			})

			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
				continue
			}
		}

		// Successfully connected
		retryCount = 0 // Reset retry count on successful connection
		runtime.EventsEmit(a.ctx, "watcher-status", WatcherStatusEvent{
			ResourceType: resourceType,
			Namespace:    namespace,
			Status:       "connected",
		})

		// Process events from this watcher
		watcherDone := false
		for !watcherDone {
			select {
			case <-ctx.Done():
				watcher.Stop()
				return
			case event, ok := <-watcher.ResultChan():
				if !ok {
					a.LogDebug("Resource watcher channel closed: type=%s, will reconnect", resourceType)
					watcher.Stop()
					watcherDone = true
					break
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
}

// watchCRDLoop is the watch loop for Custom Resource Definitions using dynamic client
// It includes reconnection logic with exponential backoff
func (a *App) watchCRDLoop(ctx context.Context, group, version, resource, namespace string) {
	crdResourceType := fmt.Sprintf("crd:%s/%s/%s", group, version, resource)

	defer func() {
		a.LogDebug("CRD watcher stopped: gvr=%s/%s/%s, namespace=%s", group, version, resource, namespace)
		runtime.EventsEmit(a.ctx, "watcher-status", WatcherStatusEvent{
			ResourceType: crdResourceType,
			Namespace:    namespace,
			Status:       "stopped",
		})
	}()

	if a.k8sClient == nil {
		a.LogDebug("watchCRDLoop: k8s client not initialized")
		return
	}

	// Reconnection parameters
	maxRetries := 5
	baseDelay := 1 * time.Second
	maxDelay := 30 * time.Second

	for retryCount := 0; ; retryCount++ {
		// Check if context is cancelled before starting/reconnecting
		select {
		case <-ctx.Done():
			return
		default:
		}

		watcher, err := a.k8sClient.WatchCRD(ctx, group, version, resource, namespace)
		if err != nil {
			a.LogDebug("Failed to start CRD watcher: gvr=%s/%s/%s, err=%v, retry=%d", group, version, resource, err, retryCount)

			// Emit error event
			runtime.EventsEmit(a.ctx, "watcher-error", WatcherErrorEvent{
				ResourceType: crdResourceType,
				Namespace:    namespace,
				Error:        err.Error(),
				Recoverable:  retryCount < maxRetries,
			})

			if retryCount >= maxRetries {
				a.LogDebug("CRD watcher max retries exceeded: gvr=%s/%s/%s", group, version, resource)
				return
			}

			// Exponential backoff
			delay := baseDelay * time.Duration(1<<uint(retryCount))
			if delay > maxDelay {
				delay = maxDelay
			}

			runtime.EventsEmit(a.ctx, "watcher-status", WatcherStatusEvent{
				ResourceType: crdResourceType,
				Namespace:    namespace,
				Status:       "reconnecting",
			})

			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
				continue
			}
		}

		// Successfully connected
		retryCount = 0 // Reset retry count on successful connection
		runtime.EventsEmit(a.ctx, "watcher-status", WatcherStatusEvent{
			ResourceType: crdResourceType,
			Namespace:    namespace,
			Status:       "connected",
		})

		// Process events from this watcher
		watcherDone := false
		for !watcherDone {
			select {
			case <-ctx.Done():
				watcher.Stop()
				return
			case event, ok := <-watcher.ResultChan():
				if !ok {
					a.LogDebug("CRD watcher channel closed: gvr=%s/%s/%s, will reconnect", group, version, resource)
					watcher.Stop()
					watcherDone = true
					break
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

					runtime.EventsEmit(a.ctx, "resource-event", ResourceEvent{
						Type:         string(event.Type),
						ResourceType: crdResourceType,
						Namespace:    resourceNs,
						Resource:     resourceMap,
					})
				}
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

// YamlBackupEntry represents a single resource's YAML for backup
type YamlBackupEntry struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Yaml      string `json:"yaml"`
}

// SaveYamlBackup saves multiple resource YAMLs as a zip file with native dialog
func (a *App) SaveYamlBackup(entries []YamlBackupEntry, defaultFilename string) error {
	filePath, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: defaultFilename,
		Title:           "Save YAML Backup",
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
		// Create path: namespace_name.yaml
		var yamlPath string
		if entry.Namespace != "" {
			yamlPath = fmt.Sprintf("%s_%s.yaml", entry.Namespace, entry.Name)
		} else {
			yamlPath = fmt.Sprintf("%s.yaml", entry.Name)
		}
		writer, err := zipWriter.Create(yamlPath)
		if err != nil {
			return fmt.Errorf("failed to create zip entry %s: %w", yamlPath, err)
		}
		_, err = writer.Write([]byte(entry.Yaml))
		if err != nil {
			return fmt.Errorf("failed to write YAML for %s: %w", yamlPath, err)
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

func (a *App) ResizePVC(namespace, name, newSize string) error {
	currentContext := a.GetCurrentContext()
	a.LogDebug("ResizePVC called: context=%s, ns=%s, name=%s, newSize=%s", currentContext, namespace, name, newSize)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ResizePVC(currentContext, namespace, name, newSize)
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

func (a *App) GetStorageClass(name string) (*storagev1.StorageClass, error) {
	currentContext := a.GetCurrentContext()
	a.LogDebug("GetStorageClass called: context=%s, name=%s", currentContext, name)
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetStorageClass(currentContext, name)
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

// ExpandDependencyNode returns additional nodes when a summary node is expanded
func (a *App) ExpandDependencyNode(resourceType, namespace, name, summaryNodeID string, offset int) (*k8s.DependencyGraph, error) {
	currentContext := a.GetCurrentContext()
	a.LogDebug("ExpandDependencyNode called: context=%s, type=%s, ns=%s, name=%s, summaryID=%s, offset=%d",
		currentContext, resourceType, namespace, name, summaryNodeID, offset)
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ExpandDependencyNode(currentContext, resourceType, namespace, name, summaryNodeID, offset)
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

// GetCRDPrinterColumns returns the additional printer columns for a CRD
func (a *App) GetCRDPrinterColumns(crdName string) ([]k8s.PrinterColumn, error) {
	currentContext := a.GetCurrentContext()
	a.LogDebug("GetCRDPrinterColumns called: context=%s, crdName=%s", currentContext, crdName)
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetCRDPrinterColumns(currentContext, crdName)
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

// --- Port Forwarding APIs ---

// GetPortForwardConfigs returns all port forward configurations, optionally filtered by context
func (a *App) GetPortForwardConfigs(contextFilter string) []PortForwardConfig {
	a.LogDebug("GetPortForwardConfigs called: contextFilter=%s", contextFilter)
	if a.portForwardManager == nil {
		return []PortForwardConfig{}
	}
	return a.portForwardManager.GetConfigs(contextFilter)
}

// GetActivePortForwards returns all active port forwards
func (a *App) GetActivePortForwards() []ActivePortForward {
	a.LogDebug("GetActivePortForwards called")
	if a.portForwardManager == nil {
		return []ActivePortForward{}
	}
	return a.portForwardManager.GetActiveForwards()
}

// AddPortForwardConfig adds a new port forward configuration
func (a *App) AddPortForwardConfig(cfg PortForwardConfig) (*PortForwardConfig, error) {
	a.LogDebug("AddPortForwardConfig called: context=%s, ns=%s, type=%s, name=%s, ports=%d:%d",
		cfg.Context, cfg.Namespace, cfg.ResourceType, cfg.ResourceName, cfg.LocalPort, cfg.RemotePort)
	if a.portForwardManager == nil {
		return nil, fmt.Errorf("port forward manager not initialized")
	}
	return a.portForwardManager.AddConfig(cfg)
}

// UpdatePortForwardConfig updates an existing port forward configuration
func (a *App) UpdatePortForwardConfig(cfg PortForwardConfig) error {
	a.LogDebug("UpdatePortForwardConfig called: id=%s", cfg.ID)
	if a.portForwardManager == nil {
		return fmt.Errorf("port forward manager not initialized")
	}
	return a.portForwardManager.UpdateConfig(cfg)
}

// DeletePortForwardConfig deletes a port forward configuration
func (a *App) DeletePortForwardConfig(configID string) error {
	a.LogDebug("DeletePortForwardConfig called: id=%s", configID)
	if a.portForwardManager == nil {
		return fmt.Errorf("port forward manager not initialized")
	}
	return a.portForwardManager.DeleteConfig(configID)
}

// StartPortForward starts a port forward
func (a *App) StartPortForward(configID string) error {
	a.LogDebug("StartPortForward called: id=%s", configID)
	if a.portForwardManager == nil {
		return fmt.Errorf("port forward manager not initialized")
	}
	return a.portForwardManager.Start(configID)
}

// StopPortForward stops a port forward
func (a *App) StopPortForward(configID string) error {
	a.LogDebug("StopPortForward called: id=%s", configID)
	if a.portForwardManager == nil {
		return fmt.Errorf("port forward manager not initialized")
	}
	return a.portForwardManager.Stop(configID)
}

// StopAllPortForwards stops all active port forwards
func (a *App) StopAllPortForwards() {
	a.LogDebug("StopAllPortForwards called")
	if a.portForwardManager == nil {
		return
	}
	a.portForwardManager.StopAll()
}

// GetAvailablePort finds an available local port
func (a *App) GetAvailablePort(preferred int) int {
	a.LogDebug("GetAvailablePort called: preferred=%d", preferred)
	if a.portForwardManager == nil {
		return 0
	}
	return a.portForwardManager.GetAvailablePort(preferred)
}

// GetRandomAvailablePort gets a random available port avoiding well-known and configured ports
func (a *App) GetRandomAvailablePort() int {
	a.LogDebug("GetRandomAvailablePort called")
	if a.portForwardManager == nil {
		return 0
	}
	return a.portForwardManager.GetRandomAvailablePort()
}

// StartFavoritePortForwards starts all favorite port forwards for a context
func (a *App) StartFavoritePortForwards(contextName string) {
	a.LogDebug("StartFavoritePortForwards called: context=%s", contextName)
	if a.portForwardManager == nil {
		return
	}
	a.portForwardManager.StartFavorites(contextName)
}

// StartPortForwardsWithMode starts port forwards based on the specified mode
// mode can be: "all", "favorites", "none"
// Only starts forwards that were running when the app was closed
func (a *App) StartPortForwardsWithMode(contextName, mode string) {
	a.LogDebug("StartPortForwardsWithMode called: context=%s, mode=%s", contextName, mode)
	if a.portForwardManager == nil {
		return
	}
	a.portForwardManager.StartWithMode(contextName, mode)
}

// --- Ingress Forwarding APIs ---

// GetIngressForwardState returns the current ingress forward state
func (a *App) GetIngressForwardState() IngressForwardState {
	a.LogDebug("GetIngressForwardState called")
	if a.ingressForwardManager == nil {
		return IngressForwardState{Active: false, Status: "stopped"}
	}
	return a.ingressForwardManager.GetState()
}

// DetectIngressController finds the ingress controller in the cluster
func (a *App) DetectIngressController() (*IngressController, error) {
	a.LogDebug("DetectIngressController called")
	if a.ingressForwardManager == nil {
		return nil, fmt.Errorf("ingress forward manager not initialized")
	}
	return a.ingressForwardManager.DetectIngressController()
}

// CollectIngressHostnames collects all unique hostnames from ingresses
func (a *App) CollectIngressHostnames(namespaces []string) ([]string, error) {
	a.LogDebug("CollectIngressHostnames called: namespaces=%v", namespaces)
	if a.ingressForwardManager == nil {
		return nil, fmt.Errorf("ingress forward manager not initialized")
	}
	return a.ingressForwardManager.CollectIngressHostnames(namespaces)
}

// StartIngressForward starts ingress forwarding with the given controller
func (a *App) StartIngressForward(controller IngressController, namespaces []string) error {
	a.LogDebug("StartIngressForward called: controller=%s/%s, namespaces=%v",
		controller.Namespace, controller.Name, namespaces)
	if a.ingressForwardManager == nil {
		return fmt.Errorf("ingress forward manager not initialized")
	}
	return a.ingressForwardManager.Start(&controller, namespaces)
}

// StopIngressForward stops ingress forwarding and cleans up hosts file
func (a *App) StopIngressForward() error {
	a.LogDebug("StopIngressForward called")
	if a.ingressForwardManager == nil {
		return fmt.Errorf("ingress forward manager not initialized")
	}
	return a.ingressForwardManager.Stop()
}

// RefreshIngressHostnames re-collects hostnames and updates the hosts file
func (a *App) RefreshIngressHostnames(namespaces []string) error {
	a.LogDebug("RefreshIngressHostnames called: namespaces=%v", namespaces)
	if a.ingressForwardManager == nil {
		return fmt.Errorf("ingress forward manager not initialized")
	}
	return a.ingressForwardManager.RefreshHostnames(namespaces)
}

// GetManagedHosts returns the currently managed hosts file entries
func (a *App) GetManagedHosts() ([]string, error) {
	a.LogDebug("GetManagedHosts called")
	if a.ingressForwardManager == nil {
		return nil, fmt.Errorf("ingress forward manager not initialized")
	}
	entries, err := a.ingressForwardManager.GetManagedHosts()
	if err != nil {
		return nil, err
	}
	hostnames := make([]string, len(entries))
	for i, e := range entries {
		hostnames[i] = e.Hostname
	}
	return hostnames, nil
}

// GetPodPorts returns the container ports for a pod
func (a *App) GetPodPorts(namespace, podName string) ([]int32, error) {
	currentContext := a.GetCurrentContext()
	a.LogDebug("GetPodPorts called: context=%s, ns=%s, pod=%s", currentContext, namespace, podName)
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetPodContainerPorts(currentContext, namespace, podName)
}

// GetServicePorts returns the ports exposed by a service
func (a *App) GetServicePorts(namespace, serviceName string) ([]int32, error) {
	currentContext := a.GetCurrentContext()
	a.LogDebug("GetServicePorts called: context=%s, ns=%s, svc=%s", currentContext, namespace, serviceName)
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetServicePorts(currentContext, namespace, serviceName)
}

// =============================================================================
// Helm Release Management
// =============================================================================

// ListHelmReleases returns all Helm releases across the specified namespaces
func (a *App) ListHelmReleases(namespaces []string) ([]helm.Release, error) {
	currentContext := a.GetCurrentContext()
	a.LogDebug("ListHelmReleases called: context=%s, namespaces=%v", currentContext, namespaces)
	if a.helmClient == nil {
		return nil, fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.ListReleases(currentContext, namespaces)
}

// GetHelmRelease returns detailed information about a specific release
func (a *App) GetHelmRelease(namespace, name string) (*helm.ReleaseDetail, error) {
	currentContext := a.GetCurrentContext()
	a.LogDebug("GetHelmRelease called: context=%s, ns=%s, name=%s", currentContext, namespace, name)
	if a.helmClient == nil {
		return nil, fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.GetRelease(currentContext, namespace, name)
}

// GetHelmReleaseValues returns the user-supplied values for a release
func (a *App) GetHelmReleaseValues(namespace, name string) (map[string]interface{}, error) {
	currentContext := a.GetCurrentContext()
	a.LogDebug("GetHelmReleaseValues called: context=%s, ns=%s, name=%s", currentContext, namespace, name)
	if a.helmClient == nil {
		return nil, fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.GetReleaseValues(currentContext, namespace, name)
}

// GetHelmReleaseAllValues returns all computed values for a release
func (a *App) GetHelmReleaseAllValues(namespace, name string) (map[string]interface{}, error) {
	currentContext := a.GetCurrentContext()
	a.LogDebug("GetHelmReleaseAllValues called: context=%s, ns=%s, name=%s", currentContext, namespace, name)
	if a.helmClient == nil {
		return nil, fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.GetReleaseAllValues(currentContext, namespace, name)
}

// GetHelmReleaseHistory returns the revision history for a release
func (a *App) GetHelmReleaseHistory(namespace, name string) ([]helm.ReleaseHistory, error) {
	currentContext := a.GetCurrentContext()
	a.LogDebug("GetHelmReleaseHistory called: context=%s, ns=%s, name=%s", currentContext, namespace, name)
	if a.helmClient == nil {
		return nil, fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.GetReleaseHistory(currentContext, namespace, name)
}

// UninstallHelmRelease removes a Helm release
func (a *App) UninstallHelmRelease(namespace, name string) error {
	currentContext := a.GetCurrentContext()
	a.LogDebug("UninstallHelmRelease called: context=%s, ns=%s, name=%s", currentContext, namespace, name)
	if a.helmClient == nil {
		return fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.Uninstall(currentContext, namespace, name)
}

// RollbackHelmRelease rolls back a release to a specific revision
func (a *App) RollbackHelmRelease(namespace, name string, revision int) error {
	currentContext := a.GetCurrentContext()
	a.LogDebug("RollbackHelmRelease called: context=%s, ns=%s, name=%s, revision=%d", currentContext, namespace, name, revision)
	if a.helmClient == nil {
		return fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.Rollback(currentContext, namespace, name, revision)
}

// GetHelmReleaseResources returns the Kubernetes resources managed by a Helm release
func (a *App) GetHelmReleaseResources(namespace, name string) ([]helm.ResourceReference, error) {
	currentContext := a.GetCurrentContext()
	a.LogDebug("GetHelmReleaseResources called: context=%s, ns=%s, name=%s", currentContext, namespace, name)
	if a.helmClient == nil {
		return nil, fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.GetReleaseResources(currentContext, namespace, name)
}

// =============================================================================
// Helm Repository Management
// =============================================================================

// ListHelmRepositories returns all configured Helm repositories with priorities
func (a *App) ListHelmRepositories() ([]helm.Repository, error) {
	a.LogDebug("ListHelmRepositories called")
	if a.helmClient == nil {
		return nil, fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.ListRepositories()
}

// AddHelmRepository adds a new Helm repository
func (a *App) AddHelmRepository(name, url string, priority int) error {
	a.LogDebug("AddHelmRepository called: name=%s, url=%s, priority=%d", name, url, priority)
	if a.helmClient == nil {
		return fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.AddRepository(name, url, priority)
}

// RemoveHelmRepository removes a Helm repository
func (a *App) RemoveHelmRepository(name string) error {
	a.LogDebug("RemoveHelmRepository called: name=%s", name)
	if a.helmClient == nil {
		return fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.RemoveRepository(name)
}

// UpdateHelmRepository updates the index for a repository
func (a *App) UpdateHelmRepository(name string) error {
	a.LogDebug("UpdateHelmRepository called: name=%s", name)
	if a.helmClient == nil {
		return fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.UpdateRepository(name)
}

// UpdateAllHelmRepositories updates the index for all repositories
func (a *App) UpdateAllHelmRepositories() error {
	fmt.Println(">>> UpdateAllHelmRepositories called <<<")
	a.LogDebug("UpdateAllHelmRepositories called")
	if a.helmClient == nil {
		return fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.UpdateAllRepositories()
}

// SetHelmRepositoryPriority sets the priority for a repository
func (a *App) SetHelmRepositoryPriority(name string, priority int) error {
	a.LogDebug("SetHelmRepositoryPriority called: name=%s, priority=%d", name, priority)
	if a.helmClient == nil {
		return fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.SetRepositoryPriority(name, priority)
}

// SearchHelmChart searches for a chart across all repositories
func (a *App) SearchHelmChart(chartName string) ([]helm.ChartSource, error) {
	a.LogDebug("SearchHelmChart called: chartName=%s", chartName)
	if a.helmClient == nil {
		return nil, fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.SearchChart(chartName)
}

// GetHelmChartVersions returns available versions for a chart from a specific repo
func (a *App) GetHelmChartVersions(repoName, chartName string) ([]helm.ChartVersion, error) {
	a.LogDebug("GetHelmChartVersions called: repo=%s, chart=%s", repoName, chartName)
	if a.helmClient == nil {
		return nil, fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.GetChartVersions(repoName, chartName)
}

// UpgradeHelmRelease upgrades or reinstalls a release
func (a *App) UpgradeHelmRelease(namespace, name string, opts helm.UpgradeOptions) error {
	currentContext := a.GetCurrentContext()
	a.LogDebug("UpgradeHelmRelease called: context=%s, ns=%s, name=%s, repo=%s, chart=%s, version=%s",
		currentContext, namespace, name, opts.RepoName, opts.ChartName, opts.Version)
	if a.helmClient == nil {
		return fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.UpgradeRelease(currentContext, namespace, name, opts)
}

// ForceHelmReleaseStatus forces a release to a specific status (e.g., "deployed")
func (a *App) ForceHelmReleaseStatus(namespace, name, status string) error {
	currentContext := a.GetCurrentContext()
	a.LogDebug("ForceHelmReleaseStatus called: context=%s, ns=%s, name=%s, status=%s",
		currentContext, namespace, name, status)
	if a.helmClient == nil {
		return fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.ForceReleaseStatus(currentContext, namespace, name, status)
}

// ListOCIRegistries returns a list of OCI registries with authentication status
func (a *App) ListOCIRegistries() ([]helm.OCIRegistry, error) {
	a.LogDebug("ListOCIRegistries called")
	if a.helmClient == nil {
		return nil, fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.ListOCIRegistries()
}

// LoginOCIRegistry authenticates to an OCI registry with username/password
func (a *App) LoginOCIRegistry(registry, username, password string) error {
	a.LogDebug("LoginOCIRegistry called: registry=%s, username=%s", registry, username)
	if a.helmClient == nil {
		return fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.LoginOCIRegistry(registry, username, password)
}

// LoginACRWithAzureCLI logs into an Azure Container Registry using Azure CLI
func (a *App) LoginACRWithAzureCLI(registry string) error {
	a.LogDebug("LoginACRWithAzureCLI called: registry=%s", registry)
	if a.helmClient == nil {
		return fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.LoginACRWithAzureCLI(registry)
}

// LogoutOCIRegistry logs out from an OCI registry
func (a *App) LogoutOCIRegistry(registry string) error {
	a.LogDebug("LogoutOCIRegistry called: registry=%s", registry)
	if a.helmClient == nil {
		return fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.LogoutOCIRegistry(registry)
}

// SetOCIRegistryPriority sets the priority for an OCI registry
func (a *App) SetOCIRegistryPriority(registryURL string, priority int) error {
	a.LogDebug("SetOCIRegistryPriority called: registry=%s, priority=%d", registryURL, priority)
	if a.helmClient == nil {
		return fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.SetOCIRegistryPriority(registryURL, priority)
}

// RemoveOCIRegistry removes an OCI registry (logout and remove priority)
func (a *App) RemoveOCIRegistry(registry string) error {
	a.LogDebug("RemoveOCIRegistry called: registry=%s", registry)
	if a.helmClient == nil {
		return fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.RemoveOCIRegistry(registry)
}

// ListChartSources returns all available chart sources (HTTP repos + OCI registries)
func (a *App) ListChartSources() ([]helm.ChartSourceInfo, error) {
	a.LogDebug("ListChartSources called")
	if a.helmClient == nil {
		return nil, fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.ListChartSources()
}

// SearchChartInSource searches for a chart in a specific source
func (a *App) SearchChartInSource(sourceName, chartName string) (*helm.ChartSearchResult, error) {
	a.LogDebug("SearchChartInSource called: source=%s, chart=%s", sourceName, chartName)
	if a.helmClient == nil {
		return nil, fmt.Errorf("helm client not initialized")
	}
	return a.helmClient.SearchChartInSource(sourceName, chartName)
}

// ==================== RBAC / Access Control ====================

// ServiceAccount operations
func (a *App) ListServiceAccounts(namespace string) ([]v1.ServiceAccount, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListServiceAccounts(namespace)
}

func (a *App) GetServiceAccountYaml(namespace, name string) (string, error) {
	a.LogDebug("GetServiceAccountYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetServiceAccountYaml(namespace, name)
}

func (a *App) UpdateServiceAccountYaml(namespace, name, yamlContent string) error {
	a.LogDebug("UpdateServiceAccountYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateServiceAccountYaml(namespace, name, yamlContent)
}

func (a *App) DeleteServiceAccount(namespace, name string) error {
	currentContext := a.GetCurrentContext()
	a.LogDebug("DeleteServiceAccount called: context=%s, ns=%s, name=%s", currentContext, namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteServiceAccount(currentContext, namespace, name)
}

// Role operations (namespaced)
func (a *App) ListRoles(namespace string) ([]rbacv1.Role, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListRoles(namespace)
}

func (a *App) GetRoleYaml(namespace, name string) (string, error) {
	a.LogDebug("GetRoleYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetRoleYaml(namespace, name)
}

func (a *App) UpdateRoleYaml(namespace, name, yamlContent string) error {
	a.LogDebug("UpdateRoleYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateRoleYaml(namespace, name, yamlContent)
}

func (a *App) DeleteRole(namespace, name string) error {
	currentContext := a.GetCurrentContext()
	a.LogDebug("DeleteRole called: context=%s, ns=%s, name=%s", currentContext, namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteRole(currentContext, namespace, name)
}

// ClusterRole operations (cluster-scoped)
func (a *App) ListClusterRoles() ([]rbacv1.ClusterRole, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListClusterRoles()
}

func (a *App) GetClusterRoleYaml(name string) (string, error) {
	a.LogDebug("GetClusterRoleYaml called: name=%s", name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetClusterRoleYaml(name)
}

func (a *App) UpdateClusterRoleYaml(name, yamlContent string) error {
	a.LogDebug("UpdateClusterRoleYaml called: name=%s", name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateClusterRoleYaml(name, yamlContent)
}

func (a *App) DeleteClusterRole(name string) error {
	currentContext := a.GetCurrentContext()
	a.LogDebug("DeleteClusterRole called: context=%s, name=%s", currentContext, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteClusterRole(currentContext, name)
}

// RoleBinding operations (namespaced)
func (a *App) ListRoleBindings(namespace string) ([]rbacv1.RoleBinding, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListRoleBindings(namespace)
}

func (a *App) GetRoleBindingYaml(namespace, name string) (string, error) {
	a.LogDebug("GetRoleBindingYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetRoleBindingYaml(namespace, name)
}

func (a *App) UpdateRoleBindingYaml(namespace, name, yamlContent string) error {
	a.LogDebug("UpdateRoleBindingYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateRoleBindingYaml(namespace, name, yamlContent)
}

func (a *App) DeleteRoleBinding(namespace, name string) error {
	currentContext := a.GetCurrentContext()
	a.LogDebug("DeleteRoleBinding called: context=%s, ns=%s, name=%s", currentContext, namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteRoleBinding(currentContext, namespace, name)
}

// ClusterRoleBinding operations (cluster-scoped)
func (a *App) ListClusterRoleBindings() ([]rbacv1.ClusterRoleBinding, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListClusterRoleBindings()
}

func (a *App) GetClusterRoleBindingYaml(name string) (string, error) {
	a.LogDebug("GetClusterRoleBindingYaml called: name=%s", name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetClusterRoleBindingYaml(name)
}

func (a *App) UpdateClusterRoleBindingYaml(name, yamlContent string) error {
	a.LogDebug("UpdateClusterRoleBindingYaml called: name=%s", name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateClusterRoleBindingYaml(name, yamlContent)
}

func (a *App) DeleteClusterRoleBinding(name string) error {
	currentContext := a.GetCurrentContext()
	a.LogDebug("DeleteClusterRoleBinding called: context=%s, name=%s", currentContext, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteClusterRoleBinding(currentContext, name)
}

// NetworkPolicy operations (namespaced)
func (a *App) ListNetworkPolicies(namespace string) ([]networkingv1.NetworkPolicy, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListNetworkPolicies(namespace)
}

func (a *App) GetNetworkPolicyYaml(namespace, name string) (string, error) {
	a.LogDebug("GetNetworkPolicyYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetNetworkPolicyYaml(namespace, name)
}

func (a *App) UpdateNetworkPolicyYaml(namespace, name, yamlContent string) error {
	a.LogDebug("UpdateNetworkPolicyYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateNetworkPolicyYaml(namespace, name, yamlContent)
}

func (a *App) DeleteNetworkPolicy(namespace, name string) error {
	currentContext := a.GetCurrentContext()
	a.LogDebug("DeleteNetworkPolicy called: context=%s, ns=%s, name=%s", currentContext, namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteNetworkPolicy(currentContext, namespace, name)
}

// HorizontalPodAutoscaler operations (namespaced)
func (a *App) ListHPAs(namespace string) ([]autoscalingv2.HorizontalPodAutoscaler, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListHPAs(namespace)
}

func (a *App) GetHPAYaml(namespace, name string) (string, error) {
	a.LogDebug("GetHPAYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetHPAYaml(namespace, name)
}

func (a *App) UpdateHPAYaml(namespace, name, yamlContent string) error {
	a.LogDebug("UpdateHPAYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateHPAYaml(namespace, name, yamlContent)
}

func (a *App) DeleteHPA(namespace, name string) error {
	currentContext := a.GetCurrentContext()
	a.LogDebug("DeleteHPA called: context=%s, ns=%s, name=%s", currentContext, namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteHPA(currentContext, namespace, name)
}

// PodDisruptionBudget operations (namespaced)
func (a *App) ListPDBs(namespace string) ([]policyv1.PodDisruptionBudget, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListPDBs(namespace)
}

func (a *App) GetPDBYaml(namespace, name string) (string, error) {
	a.LogDebug("GetPDBYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetPDBYaml(namespace, name)
}

func (a *App) UpdatePDBYaml(namespace, name, yamlContent string) error {
	a.LogDebug("UpdatePDBYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdatePDBYaml(namespace, name, yamlContent)
}

func (a *App) DeletePDB(namespace, name string) error {
	currentContext := a.GetCurrentContext()
	a.LogDebug("DeletePDB called: context=%s, ns=%s, name=%s", currentContext, namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeletePDB(currentContext, namespace, name)
}

// ResourceQuota operations (namespaced)
func (a *App) ListResourceQuotas(namespace string) ([]v1.ResourceQuota, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListResourceQuotas(namespace)
}

func (a *App) GetResourceQuotaYaml(namespace, name string) (string, error) {
	a.LogDebug("GetResourceQuotaYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetResourceQuotaYaml(namespace, name)
}

func (a *App) UpdateResourceQuotaYaml(namespace, name, yamlContent string) error {
	a.LogDebug("UpdateResourceQuotaYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateResourceQuotaYaml(namespace, name, yamlContent)
}

func (a *App) DeleteResourceQuota(namespace, name string) error {
	currentContext := a.GetCurrentContext()
	a.LogDebug("DeleteResourceQuota called: context=%s, ns=%s, name=%s", currentContext, namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteResourceQuota(currentContext, namespace, name)
}

// LimitRange operations (namespaced)
func (a *App) ListLimitRanges(namespace string) ([]v1.LimitRange, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListLimitRanges(namespace)
}

func (a *App) GetLimitRangeYaml(namespace, name string) (string, error) {
	a.LogDebug("GetLimitRangeYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetLimitRangeYaml(namespace, name)
}

func (a *App) UpdateLimitRangeYaml(namespace, name, yamlContent string) error {
	a.LogDebug("UpdateLimitRangeYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateLimitRangeYaml(namespace, name, yamlContent)
}

func (a *App) DeleteLimitRange(namespace, name string) error {
	currentContext := a.GetCurrentContext()
	a.LogDebug("DeleteLimitRange called: context=%s, ns=%s, name=%s", currentContext, namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteLimitRange(currentContext, namespace, name)
}

// Endpoints operations (namespaced)
func (a *App) ListEndpoints(namespace string) ([]v1.Endpoints, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListEndpoints(namespace)
}

func (a *App) GetEndpointsYaml(namespace, name string) (string, error) {
	a.LogDebug("GetEndpointsYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetEndpointsYaml(namespace, name)
}

func (a *App) UpdateEndpointsYaml(namespace, name, yamlContent string) error {
	a.LogDebug("UpdateEndpointsYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateEndpointsYaml(namespace, name, yamlContent)
}

func (a *App) DeleteEndpoints(namespace, name string) error {
	currentContext := a.GetCurrentContext()
	a.LogDebug("DeleteEndpoints called: context=%s, ns=%s, name=%s", currentContext, namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteEndpoints(currentContext, namespace, name)
}

// EndpointSlice operations (namespaced, discovery.k8s.io/v1)
func (a *App) ListEndpointSlices(namespace string) ([]discoveryv1.EndpointSlice, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListEndpointSlices(namespace)
}

func (a *App) GetEndpointSliceYaml(namespace, name string) (string, error) {
	a.LogDebug("GetEndpointSliceYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetEndpointSliceYaml(namespace, name)
}

func (a *App) UpdateEndpointSliceYaml(namespace, name, yamlContent string) error {
	a.LogDebug("UpdateEndpointSliceYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateEndpointSliceYaml(namespace, name, yamlContent)
}

func (a *App) DeleteEndpointSlice(namespace, name string) error {
	currentContext := a.GetCurrentContext()
	a.LogDebug("DeleteEndpointSlice called: context=%s, ns=%s, name=%s", currentContext, namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteEndpointSlice(currentContext, namespace, name)
}

// ValidatingWebhookConfiguration operations (cluster-scoped)
func (a *App) ListValidatingWebhookConfigurations() ([]admissionregistrationv1.ValidatingWebhookConfiguration, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListValidatingWebhookConfigurations()
}

func (a *App) GetValidatingWebhookConfigurationYaml(name string) (string, error) {
	a.LogDebug("GetValidatingWebhookConfigurationYaml called: name=%s", name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetValidatingWebhookConfigurationYaml(name)
}

func (a *App) UpdateValidatingWebhookConfigurationYaml(name, yamlContent string) error {
	a.LogDebug("UpdateValidatingWebhookConfigurationYaml called: name=%s", name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateValidatingWebhookConfigurationYaml(name, yamlContent)
}

func (a *App) DeleteValidatingWebhookConfiguration(name string) error {
	currentContext := a.GetCurrentContext()
	a.LogDebug("DeleteValidatingWebhookConfiguration called: context=%s, name=%s", currentContext, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteValidatingWebhookConfiguration(currentContext, name)
}

// MutatingWebhookConfiguration operations (cluster-scoped)
func (a *App) ListMutatingWebhookConfigurations() ([]admissionregistrationv1.MutatingWebhookConfiguration, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListMutatingWebhookConfigurations()
}

func (a *App) GetMutatingWebhookConfigurationYaml(name string) (string, error) {
	a.LogDebug("GetMutatingWebhookConfigurationYaml called: name=%s", name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetMutatingWebhookConfigurationYaml(name)
}

func (a *App) UpdateMutatingWebhookConfigurationYaml(name, yamlContent string) error {
	a.LogDebug("UpdateMutatingWebhookConfigurationYaml called: name=%s", name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateMutatingWebhookConfigurationYaml(name, yamlContent)
}

func (a *App) DeleteMutatingWebhookConfiguration(name string) error {
	currentContext := a.GetCurrentContext()
	a.LogDebug("DeleteMutatingWebhookConfiguration called: context=%s, name=%s", currentContext, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteMutatingWebhookConfiguration(currentContext, name)
}

// PriorityClass operations (cluster-scoped)
func (a *App) ListPriorityClasses() ([]schedulingv1.PriorityClass, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListPriorityClasses()
}

func (a *App) GetPriorityClassYaml(name string) (string, error) {
	a.LogDebug("GetPriorityClassYaml called: name=%s", name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetPriorityClassYaml(name)
}

func (a *App) UpdatePriorityClassYaml(name, yamlContent string) error {
	a.LogDebug("UpdatePriorityClassYaml called: name=%s", name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdatePriorityClassYaml(name, yamlContent)
}

func (a *App) DeletePriorityClass(name string) error {
	currentContext := a.GetCurrentContext()
	a.LogDebug("DeletePriorityClass called: context=%s, name=%s", currentContext, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeletePriorityClass(currentContext, name)
}

// Lease operations (namespaced)
func (a *App) ListLeases(namespace string) ([]coordinationv1.Lease, error) {
	currentContext := a.GetCurrentContext()
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListLeases(currentContext, namespace)
}

func (a *App) GetLeaseYaml(namespace, name string) (string, error) {
	currentContext := a.GetCurrentContext()
	a.LogDebug("GetLeaseYaml called: context=%s, ns=%s, name=%s", currentContext, namespace, name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetLeaseYaml(currentContext, namespace, name)
}

func (a *App) UpdateLeaseYaml(namespace, name, yamlContent string) error {
	currentContext := a.GetCurrentContext()
	a.LogDebug("UpdateLeaseYaml called: context=%s, ns=%s, name=%s", currentContext, namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateLeaseYaml(currentContext, namespace, name, yamlContent)
}

func (a *App) DeleteLease(namespace, name string) error {
	currentContext := a.GetCurrentContext()
	a.LogDebug("DeleteLease called: context=%s, ns=%s, name=%s", currentContext, namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteLease(currentContext, namespace, name)
}

// CSIDriver operations (cluster-scoped)
func (a *App) ListCSIDrivers() ([]storagev1.CSIDriver, error) {
	currentContext := a.GetCurrentContext()
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListCSIDrivers(currentContext)
}

func (a *App) GetCSIDriverYaml(name string) (string, error) {
	currentContext := a.GetCurrentContext()
	a.LogDebug("GetCSIDriverYaml called: context=%s, name=%s", currentContext, name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetCSIDriverYaml(currentContext, name)
}

func (a *App) UpdateCSIDriverYaml(name, yamlContent string) error {
	currentContext := a.GetCurrentContext()
	a.LogDebug("UpdateCSIDriverYaml called: context=%s, name=%s", currentContext, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateCSIDriverYaml(currentContext, name, yamlContent)
}

func (a *App) DeleteCSIDriver(name string) error {
	currentContext := a.GetCurrentContext()
	a.LogDebug("DeleteCSIDriver called: context=%s, name=%s", currentContext, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteCSIDriver(currentContext, name)
}

// CSINode operations (cluster-scoped)
func (a *App) ListCSINodes() ([]storagev1.CSINode, error) {
	currentContext := a.GetCurrentContext()
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListCSINodes(currentContext)
}

func (a *App) GetCSINodeYaml(name string) (string, error) {
	currentContext := a.GetCurrentContext()
	a.LogDebug("GetCSINodeYaml called: context=%s, name=%s", currentContext, name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetCSINodeYaml(currentContext, name)
}

func (a *App) UpdateCSINodeYaml(name, yamlContent string) error {
	currentContext := a.GetCurrentContext()
	a.LogDebug("UpdateCSINodeYaml called: context=%s, name=%s", currentContext, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateCSINodeYaml(currentContext, name, yamlContent)
}

func (a *App) DeleteCSINode(name string) error {
	currentContext := a.GetCurrentContext()
	a.LogDebug("DeleteCSINode called: context=%s, name=%s", currentContext, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteCSINode(currentContext, name)
}

// ApplyYAML creates a resource from YAML content
func (a *App) ApplyYAML(yamlContent string) error {
	currentContext := a.GetCurrentContext()
	a.LogDebug("ApplyYAML called: context=%s", currentContext)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ApplyYAML(currentContext, yamlContent)
}

// ============================================================================
// Prometheus Integration
// ============================================================================

// loadPrometheusConfigs loads saved Prometheus configurations from disk
func (a *App) loadPrometheusConfigs() {
	a.prometheusConfigMutex.Lock()
	defer a.prometheusConfigMutex.Unlock()

	data, err := os.ReadFile(a.prometheusConfigPath)
	if err != nil {
		if !os.IsNotExist(err) {
			a.LogDebug("Prometheus: Failed to read config file: %v", err)
		}
		return
	}

	var configs map[string]*k8s.PrometheusInfo
	if err := json.Unmarshal(data, &configs); err != nil {
		a.LogDebug("Prometheus: Failed to parse config file: %v", err)
		return
	}

	a.prometheusConfigs = configs
	a.LogDebug("Prometheus: Loaded %d saved configurations", len(configs))
}

// savePrometheusConfigs saves Prometheus configurations to disk
func (a *App) savePrometheusConfigs() error {
	a.prometheusConfigMutex.RLock()
	defer a.prometheusConfigMutex.RUnlock()

	data, err := json.MarshalIndent(a.prometheusConfigs, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal prometheus configs: %w", err)
	}

	if err := os.WriteFile(a.prometheusConfigPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write prometheus config file: %w", err)
	}

	return nil
}

// GetCachedPrometheusConfig returns the cached Prometheus config for the current context
func (a *App) GetCachedPrometheusConfig() *k8s.PrometheusInfo {
	currentContext := a.GetCurrentContext()
	a.prometheusConfigMutex.RLock()
	defer a.prometheusConfigMutex.RUnlock()

	if config, ok := a.prometheusConfigs[currentContext]; ok {
		return config
	}
	return nil
}

// SavePrometheusConfig saves a Prometheus configuration for the current context
func (a *App) SavePrometheusConfig(namespace, service string, port int) error {
	currentContext := a.GetCurrentContext()
	a.LogDebug("SavePrometheusConfig called: context=%s, endpoint=%s/%s:%d", currentContext, namespace, service, port)

	config := &k8s.PrometheusInfo{
		Available:       true,
		Namespace:       namespace,
		Service:         service,
		Port:            port,
		DetectionMethod: "manual",
	}

	a.prometheusConfigMutex.Lock()
	a.prometheusConfigs[currentContext] = config
	a.prometheusConfigMutex.Unlock()

	if err := a.savePrometheusConfigs(); err != nil {
		a.LogDebug("Prometheus: Failed to save config: %v", err)
		return err
	}

	a.LogDebug("Prometheus: Saved config for context %s", currentContext)
	return nil
}

// ClearPrometheusConfig clears the cached Prometheus config for the current context
func (a *App) ClearPrometheusConfig() error {
	currentContext := a.GetCurrentContext()
	a.LogDebug("ClearPrometheusConfig called: context=%s", currentContext)

	a.prometheusConfigMutex.Lock()
	delete(a.prometheusConfigs, currentContext)
	a.prometheusConfigMutex.Unlock()

	return a.savePrometheusConfigs()
}

// DetectPrometheus auto-detects Prometheus installation in the cluster
// First checks for cached config, then falls back to auto-detection
func (a *App) DetectPrometheus() (*k8s.PrometheusInfo, error) {
	currentContext := a.GetCurrentContext()
	a.LogDebug("DetectPrometheus called: context=%s", currentContext)

	// Check cached config first
	if cached := a.GetCachedPrometheusConfig(); cached != nil {
		a.LogDebug("DetectPrometheus: Using cached config for context %s", currentContext)
		// Verify it's still reachable
		if a.k8sClient != nil {
			err := a.k8sClient.TestPrometheusEndpoint(currentContext, k8s.PrometheusEndpoint{
				Namespace: cached.Namespace,
				Service:   cached.Service,
				Port:      cached.Port,
			})
			if err == nil {
				return cached, nil
			}
			a.LogDebug("DetectPrometheus: Cached config no longer reachable, will re-detect")
		}
	}

	if a.k8sClient == nil {
		return &k8s.PrometheusInfo{Available: false}, fmt.Errorf("k8s client not initialized")
	}

	// Auto-detect
	info, err := a.k8sClient.DetectPrometheus(currentContext)
	if err != nil {
		return info, err
	}

	// Cache the detected config if successful
	if info != nil && info.Available {
		a.prometheusConfigMutex.Lock()
		a.prometheusConfigs[currentContext] = info
		a.prometheusConfigMutex.Unlock()
		a.savePrometheusConfigs()
	}

	return info, nil
}

// ListPrometheusInstalls returns all Prometheus installations found in the cluster
func (a *App) ListPrometheusInstalls() ([]k8s.PrometheusInstall, error) {
	currentContext := a.GetCurrentContext()
	a.LogDebug("ListPrometheusInstalls called: context=%s", currentContext)
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListPrometheusInstalls(currentContext)
}

// TestPrometheusEndpoint tests a custom Prometheus endpoint
func (a *App) TestPrometheusEndpoint(namespace, service string, port int) error {
	currentContext := a.GetCurrentContext()
	a.LogDebug("TestPrometheusEndpoint called: context=%s, endpoint=%s/%s:%d", currentContext, namespace, service, port)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.TestPrometheusEndpoint(currentContext, k8s.PrometheusEndpoint{
		Namespace: namespace,
		Service:   service,
		Port:      port,
	})
}

// GetPodMetricsHistory retrieves historical metrics for a pod from Prometheus
func (a *App) GetPodMetricsHistory(prometheusNamespace, prometheusService string, prometheusPort int, namespace, pod, container, duration string) (*k8s.PodMetricsHistory, error) {
	currentContext := a.GetCurrentContext()
	a.LogDebug("GetPodMetricsHistory called: context=%s, pod=%s/%s, duration=%s", currentContext, namespace, pod, duration)
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}

	// Parse duration
	var dur time.Duration
	switch duration {
	case "1h":
		dur = time.Hour
	case "6h":
		dur = 6 * time.Hour
	case "24h":
		dur = 24 * time.Hour
	case "7d":
		dur = 7 * 24 * time.Hour
	case "30d":
		dur = 30 * 24 * time.Hour
	case "all":
		dur = 90 * 24 * time.Hour // 90 days max for "all"
	default:
		dur = time.Hour
	}

	info := &k8s.PrometheusInfo{
		Available: true,
		Namespace: prometheusNamespace,
		Service:   prometheusService,
		Port:      prometheusPort,
	}

	// Target ~150 data points for readable charts (chart width ~320px, line width 2px)
	return a.k8sClient.GetPodMetricsHistoryWithResolution(currentContext, info, namespace, pod, container, dur, 150)
}

// GetControllerMetricsHistory retrieves historical metrics for a controller (deployment, statefulset, etc.)
func (a *App) GetControllerMetricsHistory(prometheusNamespace, prometheusService string, prometheusPort int, namespace, name, controllerType, duration string) (*k8s.ControllerMetricsHistory, error) {
	currentContext := a.GetCurrentContext()
	a.LogDebug("GetControllerMetricsHistory called: context=%s, controller=%s/%s, type=%s, duration=%s", currentContext, namespace, name, controllerType, duration)
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}

	// Parse duration
	var dur time.Duration
	switch duration {
	case "1h":
		dur = time.Hour
	case "6h":
		dur = 6 * time.Hour
	case "24h":
		dur = 24 * time.Hour
	case "7d":
		dur = 7 * 24 * time.Hour
	case "30d":
		dur = 30 * 24 * time.Hour
	case "all":
		dur = 90 * 24 * time.Hour
	default:
		dur = time.Hour
	}

	info := &k8s.PrometheusInfo{
		Available: true,
		Namespace: prometheusNamespace,
		Service:   prometheusService,
		Port:      prometheusPort,
	}

	return a.k8sClient.GetControllerMetricsHistory(currentContext, info, namespace, name, controllerType, dur, 150)
}

// GetNodeMetricsHistory retrieves historical metrics for a node
func (a *App) GetNodeMetricsHistory(prometheusNamespace, prometheusService string, prometheusPort int, nodeName, duration string) (*k8s.NodeMetricsHistory, error) {
	currentContext := a.GetCurrentContext()
	a.LogDebug("GetNodeMetricsHistory called: context=%s, node=%s, duration=%s", currentContext, nodeName, duration)
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}

	// Parse duration
	var dur time.Duration
	switch duration {
	case "1h":
		dur = time.Hour
	case "6h":
		dur = 6 * time.Hour
	case "24h":
		dur = 24 * time.Hour
	case "7d":
		dur = 7 * 24 * time.Hour
	case "30d":
		dur = 30 * 24 * time.Hour
	case "all":
		dur = 90 * 24 * time.Hour
	default:
		dur = time.Hour
	}

	info := &k8s.PrometheusInfo{
		Available: true,
		Namespace: prometheusNamespace,
		Service:   prometheusService,
		Port:      prometheusPort,
	}

	return a.k8sClient.GetNodeMetricsHistory(currentContext, info, nodeName, dur, 150)
}
