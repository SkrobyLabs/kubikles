package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"kubikles/pkg/k8s"
)

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
			a.logDebug("Prometheus: Failed to read config file: %v", err)
		}
		return
	}

	var configs map[string]*k8s.PrometheusInfo
	if err := json.Unmarshal(data, &configs); err != nil {
		a.logDebug("Prometheus: Failed to parse config file: %v", err)
		return
	}

	a.prometheusConfigs = configs
	a.logDebug("Prometheus: Loaded %d saved configurations", len(configs))
}

// savePrometheusConfigs saves Prometheus configurations to disk
func (a *App) savePrometheusConfigs() error {
	a.prometheusConfigMutex.RLock()
	defer a.prometheusConfigMutex.RUnlock()

	data, err := json.MarshalIndent(a.prometheusConfigs, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal prometheus configs: %w", err)
	}

	if err := os.WriteFile(a.prometheusConfigPath, data, 0600); err != nil {
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
	a.logDebug("SavePrometheusConfig called: context=%s, endpoint=%s/%s:%d", currentContext, namespace, service, port)

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
		a.logDebug("Prometheus: Failed to save config: %v", err)
		return err
	}

	a.logDebug("Prometheus: Saved config for context %s", currentContext)
	return nil
}

// ClearPrometheusConfig clears the cached Prometheus config for the current context
func (a *App) ClearPrometheusConfig() error {
	currentContext := a.GetCurrentContext()
	a.logDebug("ClearPrometheusConfig called: context=%s", currentContext)

	a.prometheusConfigMutex.Lock()
	delete(a.prometheusConfigs, currentContext)
	a.prometheusConfigMutex.Unlock()

	return a.savePrometheusConfigs()
}

// DetectPrometheus auto-detects Prometheus installation in the cluster
// First checks for cached config, then falls back to auto-detection
func (a *App) DetectPrometheus() (*k8s.PrometheusInfo, error) {
	currentContext := a.GetCurrentContext()
	a.logDebug("DetectPrometheus called: context=%s", currentContext)

	// Check cached config first
	if cached := a.GetCachedPrometheusConfig(); cached != nil {
		a.logDebug("DetectPrometheus: Using cached config for context %s", currentContext)
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
			a.logDebug("DetectPrometheus: Cached config no longer reachable, will re-detect")
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
		_ = a.savePrometheusConfigs() // Best-effort persist
	}

	return info, nil
}

// ListPrometheusInstalls returns all Prometheus installations found in the cluster
func (a *App) ListPrometheusInstalls() ([]k8s.PrometheusInstall, error) {
	currentContext := a.GetCurrentContext()
	a.logDebug("ListPrometheusInstalls called: context=%s", currentContext)
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListPrometheusInstalls(currentContext)
}

// TestPrometheusEndpoint tests a custom Prometheus endpoint
func (a *App) TestPrometheusEndpoint(namespace, service string, port int) error {
	currentContext := a.GetCurrentContext()
	a.logDebug("TestPrometheusEndpoint called: context=%s, endpoint=%s/%s:%d", currentContext, namespace, service, port)
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
func (a *App) GetPodMetricsHistory(requestId, prometheusNamespace, prometheusService string, prometheusPort int, namespace, pod, container, duration string) (*k8s.PodMetricsHistory, error) {
	currentContext := a.GetCurrentContext()
	a.logDebug("GetPodMetricsHistory called: context=%s, pod=%s/%s, duration=%s, requestId=%s", currentContext, namespace, pod, duration, requestId)
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

	// Start cancellable request with timeout
	ctx, seq := a.metricsRequestManager.StartRequest(requestId)
	defer a.metricsRequestManager.CompleteRequest(requestId, seq)

	// Target ~150 data points for readable charts (chart width ~320px, line width 2px)
	return a.k8sClient.GetPodMetricsHistoryWithContext(ctx, currentContext, info, namespace, pod, container, dur, 150)
}

// GetControllerMetricsHistory retrieves historical metrics for a controller (deployment, statefulset, etc.)
func (a *App) GetControllerMetricsHistory(requestId, prometheusNamespace, prometheusService string, prometheusPort int, namespace, name, controllerType, duration string) (*k8s.ControllerMetricsHistory, error) {
	currentContext := a.GetCurrentContext()
	a.logDebug("GetControllerMetricsHistory called: context=%s, controller=%s/%s, type=%s, duration=%s, requestId=%s", currentContext, namespace, name, controllerType, duration, requestId)
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

	// Start cancellable request with timeout
	ctx, seq := a.metricsRequestManager.StartRequest(requestId)
	defer a.metricsRequestManager.CompleteRequest(requestId, seq)

	return a.k8sClient.GetControllerMetricsHistoryWithContext(ctx, currentContext, info, namespace, name, controllerType, dur, 150)
}

// GetNodeMetricsHistory retrieves historical metrics for a node
func (a *App) GetNodeMetricsHistory(requestId, prometheusNamespace, prometheusService string, prometheusPort int, nodeName, duration string) (*k8s.NodeMetricsHistory, error) {
	currentContext := a.GetCurrentContext()
	a.logDebug("GetNodeMetricsHistory called: context=%s, node=%s, duration=%s, requestId=%s", currentContext, nodeName, duration, requestId)
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

	// Start cancellable request with timeout
	ctx, seq := a.metricsRequestManager.StartRequest(requestId)
	defer a.metricsRequestManager.CompleteRequest(requestId, seq)

	return a.k8sClient.GetNodeMetricsHistoryWithContext(ctx, currentContext, info, nodeName, dur, 150)
}

// GetNamespaceMetricsHistory retrieves historical metrics for a namespace
func (a *App) GetNamespaceMetricsHistory(requestId, prometheusNamespace, prometheusService string, prometheusPort int, namespace, duration string) (*k8s.NamespaceMetricsHistory, error) {
	currentContext := a.GetCurrentContext()
	a.logDebug("GetNamespaceMetricsHistory called: context=%s, namespace=%s, duration=%s, requestId=%s", currentContext, namespace, duration, requestId)
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
		dur = 365 * 24 * time.Hour
	default:
		dur = time.Hour
	}

	info := &k8s.PrometheusInfo{
		Available: true,
		Namespace: prometheusNamespace,
		Service:   prometheusService,
		Port:      prometheusPort,
	}

	// Start cancellable request with timeout
	ctx, seq := a.metricsRequestManager.StartRequest(requestId)
	defer a.metricsRequestManager.CompleteRequest(requestId, seq)

	return a.k8sClient.GetNamespaceMetricsHistoryWithContext(ctx, currentContext, info, namespace, dur, 150)
}

// CancelMetricsRequest cancels an in-flight metrics request
func (a *App) CancelMetricsRequest(requestId string) bool {
	return a.metricsRequestManager.CancelRequest(requestId)
}

// GetMetricsRequestStats returns statistics about metrics requests
func (a *App) GetMetricsRequestStats() MetricsRequestStats {
	return a.metricsRequestManager.GetStats()
}

// CancelListRequest cancels an in-flight list request
func (a *App) CancelListRequest(requestId string) bool {
	return a.listRequestManager.CancelRequest(requestId)
}

// GetListRequestStats returns statistics about list requests
func (a *App) GetListRequestStats() ListRequestStats {
	return a.listRequestManager.GetStats()
}

// SetRequestCancellationEnabled enables or disables actual HTTP request cancellation.
// Due to a Go HTTP/2 bug (golang/go#34944), canceling requests can cause O(N²) performance
// collapse and connection pool issues. When disabled, requests complete in background
// but stale results are ignored via sequence tracking.
func (a *App) SetRequestCancellationEnabled(enabled bool) {
	a.listRequestManager.SetCancellationEnabled(enabled)
	a.logDebug("Request cancellation enabled: %v", enabled)
}

// IsRequestCancellationEnabled returns whether HTTP request cancellation is enabled.
func (a *App) IsRequestCancellationEnabled() bool {
	return a.listRequestManager.IsCancellationEnabled()
}
