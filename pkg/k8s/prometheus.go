package k8s

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"kubikles/pkg/debug"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

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

// extractPrometheusError tries to parse a Prometheus API error response body
// and return a clean error message. Returns empty string if parsing fails.
func extractPrometheusError(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var promResp struct {
		Status    string `json:"status"`
		ErrorType string `json:"errorType"`
		Error     string `json:"error"`
	}
	if err := json.Unmarshal(body, &promResp); err != nil {
		// Not JSON — return truncated raw body
		s := string(body)
		if len(s) > 200 {
			s = s[:200]
		}
		return s
	}
	if promResp.Error != "" {
		return promResp.Error
	}
	return ""
}

// queryPrometheusRaw makes a raw query to Prometheus via K8s API proxy
func (c *Client) queryPrometheusRaw(contextName string, info *PrometheusInfo, path string, params map[string]string) ([]byte, error) {
	return c.queryPrometheusRawWithContext(context.Background(), contextName, info, path, params)
}

// queryPrometheusRawWithContext makes a raw query to Prometheus with cancellation support.
// Uses POST with form-encoded body for parameterized queries to avoid URL-length limits
// that can occur with complex PromQL queries through the K8s API proxy.
// Uses Do() instead of DoRaw() to preserve response body on error for diagnostics.
func (c *Client) queryPrometheusRawWithContext(ctx context.Context, contextName string, info *PrometheusInfo, path string, params map[string]string) ([]byte, error) {
	cs, err := c.getClientsetForContext(contextName)
	if err != nil {
		return nil, err
	}

	svcName := fmt.Sprintf("%s:%d", info.Service, info.Port)

	// Use POST with form-encoded body when params are present (query_range, query).
	// GET URL params can hit K8s API proxy URL-length limits with complex PromQL.
	if len(params) > 0 {
		formData := url.Values{}
		for k, v := range params {
			formData.Set(k, v)
		}

		req := cs.CoreV1().RESTClient().Post().
			Namespace(info.Namespace).
			Resource("services").
			Name(svcName).
			SubResource("proxy").
			Suffix(path).
			SetHeader("Content-Type", "application/x-www-form-urlencoded").
			Body([]byte(formData.Encode()))

		// Use Do() instead of DoRaw() — Do() preserves the response body even on
		// non-2xx status, letting us capture the actual Prometheus error message.
		rawResult := req.Do(ctx)
		var statusCode int
		rawResult.StatusCode(&statusCode)
		body, err := rawResult.Raw()
		if err != nil {
			promErr := extractPrometheusError(body)
			debug.LogK8s("Prometheus query failed", map[string]interface{}{
				"error":      err.Error(),
				"httpStatus": statusCode,
				"promError":  promErr,
				"path":       path,
				"service":    fmt.Sprintf("%s/%s", info.Namespace, svcName),
				"query":      params["query"],
				"start":      params["start"],
				"end":        params["end"],
				"step":       params["step"],
			})
			if promErr != "" {
				return nil, fmt.Errorf("prometheus error: %s", promErr)
			}
			return nil, fmt.Errorf("prometheus query failed: %w", err)
		}
		return body, nil
	}

	// GET for simple requests without params (e.g. connection tests)
	req := cs.CoreV1().RESTClient().Get().
		Namespace(info.Namespace).
		Resource("services").
		Name(svcName).
		SubResource("proxy").
		Suffix(path)

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
		if !isCancelledError(err) {
			debug.LogK8s("Prometheus range query failed (silent)", map[string]interface{}{
				"error":   err.Error(),
				"query":   query,
				"start":   start.Format(time.RFC3339),
				"end":     end.Format(time.RFC3339),
				"step":    step.String(),
				"service": fmt.Sprintf("%s/%s:%d", info.Namespace, info.Service, info.Port),
			})
		}
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
