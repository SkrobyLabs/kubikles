package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"kubikles/pkg/debug"
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
			debug.LogPerformance("Prometheus: Failed to read config file", map[string]interface{}{"error": err.Error()})
		}
		return
	}

	var configs map[string]*k8s.PrometheusInfo
	if err := json.Unmarshal(data, &configs); err != nil {
		debug.LogPerformance("Prometheus: Failed to parse config file", map[string]interface{}{"error": err.Error()})
		return
	}

	a.prometheusConfigs = configs
	debug.LogPerformance("Prometheus: Loaded saved configurations", map[string]interface{}{"count": len(configs)})
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
	debug.LogPerformance("SavePrometheusConfig called", map[string]interface{}{"context": currentContext, "namespace": namespace, "service": service, "port": port})

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
		debug.LogPerformance("Prometheus: Failed to save config", map[string]interface{}{"error": err.Error()})
		return err
	}

	debug.LogPerformance("Prometheus: Saved config", map[string]interface{}{"context": currentContext})
	return nil
}

// ClearPrometheusConfig clears the cached Prometheus config for the current context
func (a *App) ClearPrometheusConfig() error {
	currentContext := a.GetCurrentContext()
	debug.LogPerformance("ClearPrometheusConfig called", map[string]interface{}{"context": currentContext})

	a.prometheusConfigMutex.Lock()
	delete(a.prometheusConfigs, currentContext)
	a.prometheusConfigMutex.Unlock()

	return a.savePrometheusConfigs()
}

// DetectPrometheus auto-detects Prometheus installation in the cluster
// First checks for cached config, then falls back to auto-detection
func (a *App) DetectPrometheus() (*k8s.PrometheusInfo, error) {
	currentContext := a.GetCurrentContext()
	debug.LogPerformance("DetectPrometheus called", map[string]interface{}{"context": currentContext})

	// Check cached config first
	if cached := a.GetCachedPrometheusConfig(); cached != nil {
		debug.LogPerformance("DetectPrometheus: Using cached config", map[string]interface{}{"context": currentContext})
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
			debug.LogPerformance("DetectPrometheus: Cached config no longer reachable, will re-detect", nil)
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
	debug.LogPerformance("ListPrometheusInstalls called", map[string]interface{}{"context": currentContext})
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListPrometheusInstalls(currentContext)
}

// TestPrometheusEndpoint tests a custom Prometheus endpoint
func (a *App) TestPrometheusEndpoint(namespace, service string, port int) error {
	currentContext := a.GetCurrentContext()
	debug.LogPerformance("TestPrometheusEndpoint called", map[string]interface{}{"context": currentContext, "namespace": namespace, "service": service, "port": port})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.TestPrometheusEndpoint(currentContext, k8s.PrometheusEndpoint{
		Namespace: namespace,
		Service:   service,
		Port:      port,
	})
}

// parseMetricsDuration converts a duration string to time.Duration for metrics queries
func parseMetricsDuration(duration string) time.Duration {
	switch duration {
	case "1h":
		return time.Hour
	case "6h":
		return 6 * time.Hour
	case "24h":
		return 24 * time.Hour
	case "7d":
		return 7 * 24 * time.Hour
	case "30d":
		return 30 * 24 * time.Hour
	case "all":
		return 90 * 24 * time.Hour
	default:
		return time.Hour
	}
}

// GetPodMetricsHistory retrieves historical metrics for a pod from Prometheus
func (a *App) GetPodMetricsHistory(requestId, prometheusNamespace, prometheusService string, prometheusPort int, namespace, pod, container, duration string) (*k8s.PodMetricsHistory, error) {
	currentContext := a.GetCurrentContext()
	debug.LogPerformance("GetPodMetricsHistory called", map[string]interface{}{"context": currentContext, "namespace": namespace, "pod": pod, "duration": duration, "requestId": requestId})
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}

	dur := parseMetricsDuration(duration)
	end := time.Now()
	start := end.Add(-dur)

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
	return a.k8sClient.GetPodMetricsHistoryWithContext(ctx, currentContext, info, namespace, pod, container, start, end, 150)
}

// GetControllerMetricsHistory retrieves historical metrics for a controller (deployment, statefulset, etc.)
func (a *App) GetControllerMetricsHistory(requestId, prometheusNamespace, prometheusService string, prometheusPort int, namespace, name, controllerType, duration string) (*k8s.ControllerMetricsHistory, error) {
	currentContext := a.GetCurrentContext()
	debug.LogPerformance("GetControllerMetricsHistory called", map[string]interface{}{"context": currentContext, "namespace": namespace, "name": name, "controllerType": controllerType, "duration": duration, "requestId": requestId})
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}

	dur := parseMetricsDuration(duration)
	end := time.Now()
	start := end.Add(-dur)

	info := &k8s.PrometheusInfo{
		Available: true,
		Namespace: prometheusNamespace,
		Service:   prometheusService,
		Port:      prometheusPort,
	}

	// Start cancellable request with timeout
	ctx, seq := a.metricsRequestManager.StartRequest(requestId)
	defer a.metricsRequestManager.CompleteRequest(requestId, seq)

	return a.k8sClient.GetControllerMetricsHistoryWithContext(ctx, currentContext, info, namespace, name, controllerType, start, end, 150)
}

// GetNodeMetricsHistory retrieves historical metrics for a node
func (a *App) GetNodeMetricsHistory(requestId, prometheusNamespace, prometheusService string, prometheusPort int, nodeName, duration string) (*k8s.NodeMetricsHistory, error) {
	currentContext := a.GetCurrentContext()
	debug.LogPerformance("GetNodeMetricsHistory called", map[string]interface{}{"context": currentContext, "node": nodeName, "duration": duration, "requestId": requestId})
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}

	dur := parseMetricsDuration(duration)
	end := time.Now()
	start := end.Add(-dur)

	info := &k8s.PrometheusInfo{
		Available: true,
		Namespace: prometheusNamespace,
		Service:   prometheusService,
		Port:      prometheusPort,
	}

	// Start cancellable request with timeout
	ctx, seq := a.metricsRequestManager.StartRequest(requestId)
	defer a.metricsRequestManager.CompleteRequest(requestId, seq)

	return a.k8sClient.GetNodeMetricsHistoryWithContext(ctx, currentContext, info, nodeName, start, end, 150)
}

// GetNamespaceMetricsHistory retrieves historical metrics for a namespace
func (a *App) GetNamespaceMetricsHistory(requestId, prometheusNamespace, prometheusService string, prometheusPort int, namespace, duration string) (*k8s.NamespaceMetricsHistory, error) {
	currentContext := a.GetCurrentContext()
	debug.LogPerformance("GetNamespaceMetricsHistory called", map[string]interface{}{"context": currentContext, "namespace": namespace, "duration": duration, "requestId": requestId})
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}

	dur := parseMetricsDuration(duration)
	if duration == "all" {
		dur = 365 * 24 * time.Hour // Namespace uses 365d for "all"
	}
	end := time.Now()
	start := end.Add(-dur)

	info := &k8s.PrometheusInfo{
		Available: true,
		Namespace: prometheusNamespace,
		Service:   prometheusService,
		Port:      prometheusPort,
	}

	// Start cancellable request with timeout
	ctx, seq := a.metricsRequestManager.StartRequest(requestId)
	defer a.metricsRequestManager.CompleteRequest(requestId, seq)

	return a.k8sClient.GetNamespaceMetricsHistoryWithContext(ctx, currentContext, info, namespace, start, end, 150)
}

// GetPodMetricsHistoryRange retrieves historical pod metrics for an explicit time range (zoom)
func (a *App) GetPodMetricsHistoryRange(requestId, prometheusNamespace, prometheusService string, prometheusPort int, namespace, pod, container string, startMs, endMs int64) (*k8s.PodMetricsHistory, error) {
	currentContext := a.GetCurrentContext()
	debug.LogPerformance("GetPodMetricsHistoryRange called", map[string]interface{}{"context": currentContext, "namespace": namespace, "pod": pod, "startMs": startMs, "endMs": endMs, "requestId": requestId})
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}

	start := time.UnixMilli(startMs)
	end := time.UnixMilli(endMs)

	info := &k8s.PrometheusInfo{
		Available: true,
		Namespace: prometheusNamespace,
		Service:   prometheusService,
		Port:      prometheusPort,
	}

	ctx, seq := a.metricsRequestManager.StartRequest(requestId)
	defer a.metricsRequestManager.CompleteRequest(requestId, seq)

	return a.k8sClient.GetPodMetricsHistoryWithContext(ctx, currentContext, info, namespace, pod, container, start, end, 150)
}

// GetControllerMetricsHistoryRange retrieves historical controller metrics for an explicit time range (zoom)
func (a *App) GetControllerMetricsHistoryRange(requestId, prometheusNamespace, prometheusService string, prometheusPort int, namespace, name, controllerType string, startMs, endMs int64) (*k8s.ControllerMetricsHistory, error) {
	currentContext := a.GetCurrentContext()
	debug.LogPerformance("GetControllerMetricsHistoryRange called", map[string]interface{}{"context": currentContext, "namespace": namespace, "name": name, "startMs": startMs, "endMs": endMs, "requestId": requestId})
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}

	start := time.UnixMilli(startMs)
	end := time.UnixMilli(endMs)

	info := &k8s.PrometheusInfo{
		Available: true,
		Namespace: prometheusNamespace,
		Service:   prometheusService,
		Port:      prometheusPort,
	}

	ctx, seq := a.metricsRequestManager.StartRequest(requestId)
	defer a.metricsRequestManager.CompleteRequest(requestId, seq)

	return a.k8sClient.GetControllerMetricsHistoryWithContext(ctx, currentContext, info, namespace, name, controllerType, start, end, 150)
}

// GetNodeMetricsHistoryRange retrieves historical node metrics for an explicit time range (zoom)
func (a *App) GetNodeMetricsHistoryRange(requestId, prometheusNamespace, prometheusService string, prometheusPort int, nodeName string, startMs, endMs int64) (*k8s.NodeMetricsHistory, error) {
	currentContext := a.GetCurrentContext()
	debug.LogPerformance("GetNodeMetricsHistoryRange called", map[string]interface{}{"context": currentContext, "node": nodeName, "startMs": startMs, "endMs": endMs, "requestId": requestId})
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}

	start := time.UnixMilli(startMs)
	end := time.UnixMilli(endMs)

	info := &k8s.PrometheusInfo{
		Available: true,
		Namespace: prometheusNamespace,
		Service:   prometheusService,
		Port:      prometheusPort,
	}

	ctx, seq := a.metricsRequestManager.StartRequest(requestId)
	defer a.metricsRequestManager.CompleteRequest(requestId, seq)

	return a.k8sClient.GetNodeMetricsHistoryWithContext(ctx, currentContext, info, nodeName, start, end, 150)
}

// GetNamespaceMetricsHistoryRange retrieves historical namespace metrics for an explicit time range (zoom)
func (a *App) GetNamespaceMetricsHistoryRange(requestId, prometheusNamespace, prometheusService string, prometheusPort int, namespace string, startMs, endMs int64) (*k8s.NamespaceMetricsHistory, error) {
	currentContext := a.GetCurrentContext()
	debug.LogPerformance("GetNamespaceMetricsHistoryRange called", map[string]interface{}{"context": currentContext, "namespace": namespace, "startMs": startMs, "endMs": endMs, "requestId": requestId})
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}

	start := time.UnixMilli(startMs)
	end := time.UnixMilli(endMs)

	info := &k8s.PrometheusInfo{
		Available: true,
		Namespace: prometheusNamespace,
		Service:   prometheusService,
		Port:      prometheusPort,
	}

	ctx, seq := a.metricsRequestManager.StartRequest(requestId)
	defer a.metricsRequestManager.CompleteRequest(requestId, seq)

	return a.k8sClient.GetNamespaceMetricsHistoryWithContext(ctx, currentContext, info, namespace, start, end, 150)
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
	debug.LogPerformance("Request cancellation enabled", map[string]interface{}{"enabled": enabled})
}

// IsRequestCancellationEnabled returns whether HTTP request cancellation is enabled.
func (a *App) IsRequestCancellationEnabled() bool {
	return a.listRequestManager.IsCancellationEnabled()
}
