package k8s

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

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
