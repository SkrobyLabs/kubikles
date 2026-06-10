// Code split from prometheus.go; see that file for the Prometheus types and detection.
package k8s

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"kubikles/pkg/debug"
)

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
