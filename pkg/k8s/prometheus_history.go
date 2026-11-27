// Code split from prometheus.go; see that file for the Prometheus types and detection.
package k8s

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"sync"
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

	// Query Memory usage (working set bytes)
	memQuery := fmt.Sprintf(
		`sum by (container) (container_memory_working_set_bytes{namespace="%s", pod="%s"%s, container!="", container!="POD"})`,
		namespace, pod, containerFilter,
	)

	// Network series are best-effort and independent of CPU/memory; they run
	// alongside the two required queries. If a required query fails the call
	// returns an error, so cancel the network work instead of waiting for it.
	netCtx, cancelNet := context.WithCancel(ctx)
	defer cancelNet()
	batch := c.newRangeQueryBatch(netCtx, contextName, info, start, end, step)
	var cpuResult, memResult *PrometheusQueryResult
	var cpuErr, memErr error
	var required sync.WaitGroup
	required.Add(2)
	go func() {
		defer required.Done()
		cpuResult, cpuErr = c.QueryPrometheusRangeWithContext(ctx, contextName, info, cpuQuery, start, end, step)
		if cpuErr != nil {
			cancelNet()
		}
	}()
	go func() {
		defer required.Done()
		memResult, memErr = c.QueryPrometheusRangeWithContext(ctx, contextName, info, memQuery, start, end, step)
		if memErr != nil {
			cancelNet()
		}
	}()

	// --- Network Metrics (pod-level) ---
	// Receive bytes/s
	rxBytesQuery := fmt.Sprintf(
		`sum(rate(container_network_receive_bytes_total{namespace="%s", pod="%s"}[5m]))`,
		namespace, pod,
	)
	batch.points(&result.Network.ReceiveBytes, rxBytesQuery)

	// Transmit bytes/s
	txBytesQuery := fmt.Sprintf(
		`sum(rate(container_network_transmit_bytes_total{namespace="%s", pod="%s"}[5m]))`,
		namespace, pod,
	)
	batch.points(&result.Network.TransmitBytes, txBytesQuery)

	// Receive packets/s
	rxPacketsQuery := fmt.Sprintf(
		`sum(rate(container_network_receive_packets_total{namespace="%s", pod="%s"}[5m]))`,
		namespace, pod,
	)
	batch.points(&result.Network.ReceivePackets, rxPacketsQuery)

	// Transmit packets/s
	txPacketsQuery := fmt.Sprintf(
		`sum(rate(container_network_transmit_packets_total{namespace="%s", pod="%s"}[5m]))`,
		namespace, pod,
	)
	batch.points(&result.Network.TransmitPackets, txPacketsQuery)

	// Dropped packets (receive)
	rxDroppedQuery := fmt.Sprintf(
		`sum(rate(container_network_receive_packets_dropped_total{namespace="%s", pod="%s"}[5m]))`,
		namespace, pod,
	)
	batch.points(&result.Network.ReceiveDropped, rxDroppedQuery)

	// Dropped packets (transmit)
	txDroppedQuery := fmt.Sprintf(
		`sum(rate(container_network_transmit_packets_dropped_total{namespace="%s", pod="%s"}[5m]))`,
		namespace, pod,
	)
	batch.points(&result.Network.TransmitDropped, txDroppedQuery)

	required.Wait()
	if cpuErr != nil {
		batch.wait()
		return nil, fmt.Errorf("failed to query CPU metrics: %w", cpuErr)
	}
	if memErr != nil {
		batch.wait()
		return nil, fmt.Errorf("failed to query memory metrics: %w", memErr)
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

	batch.wait()

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
	batch := c.newRangeQueryBatch(ctx, contextName, info, start, end, step)

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
	batch.points(&result.CPU.Usage, cpuUsageQuery)

	// CPU Request (from kube-state-metrics, convert cores to millicores)
	cpuRequestQuery := fmt.Sprintf(
		`sum(kube_pod_container_resource_requests{namespace="%s", pod=~"%s", resource="cpu"}) * 1000`,
		namespace, podRegex,
	)
	batch.points(&result.CPU.Request, cpuRequestQuery)

	// CPU Limit
	cpuLimitQuery := fmt.Sprintf(
		`sum(kube_pod_container_resource_limits{namespace="%s", pod=~"%s", resource="cpu"}) * 1000`,
		namespace, podRegex,
	)
	batch.points(&result.CPU.Limit, cpuLimitQuery)

	// Node Allocatable CPU (for nodes where controller pods run)
	cpuAllocatableQuery := fmt.Sprintf(
		`sum(kube_node_status_allocatable{resource="cpu"} * on(node) group_left() (sum by(node) (kube_pod_info{namespace="%s", pod=~"%s"}) > 0)) * 1000`,
		namespace, podRegex,
	)
	batch.points(&result.CPU.NodeAllocatable, cpuAllocatableQuery)

	// Node Uncommitted CPU (allocatable - committed requests on those nodes)
	cpuUncommittedQuery := fmt.Sprintf(
		`(sum(kube_node_status_allocatable{resource="cpu"} * on(node) group_left() (sum by(node) (kube_pod_info{namespace="%s", pod=~"%s"}) > 0)) - sum(kube_pod_container_resource_requests{resource="cpu"} * on(pod, namespace) group_left(node) (kube_pod_info{node=~".+"} * on(node) group_left() (sum by(node) (kube_pod_info{namespace="%s", pod=~"%s"}) > 0)))) * 1000`,
		namespace, podRegex, namespace, podRegex,
	)
	batch.points(&result.CPU.NodeUncommitted, cpuUncommittedQuery)

	// --- Memory Metrics ---
	// Memory Usage (bytes)
	memUsageQuery := fmt.Sprintf(
		`sum(container_memory_working_set_bytes{namespace="%s", pod=~"%s", container!="", container!="POD"})`,
		namespace, podRegex,
	)
	batch.points(&result.Memory.Usage, memUsageQuery)

	// Memory Request
	memRequestQuery := fmt.Sprintf(
		`sum(kube_pod_container_resource_requests{namespace="%s", pod=~"%s", resource="memory"})`,
		namespace, podRegex,
	)
	batch.points(&result.Memory.Request, memRequestQuery)

	// Memory Limit
	memLimitQuery := fmt.Sprintf(
		`sum(kube_pod_container_resource_limits{namespace="%s", pod=~"%s", resource="memory"})`,
		namespace, podRegex,
	)
	batch.points(&result.Memory.Limit, memLimitQuery)

	// Node Allocatable Memory
	memAllocatableQuery := fmt.Sprintf(
		`sum(kube_node_status_allocatable{resource="memory"} * on(node) group_left() (sum by(node) (kube_pod_info{namespace="%s", pod=~"%s"}) > 0))`,
		namespace, podRegex,
	)
	batch.points(&result.Memory.NodeAllocatable, memAllocatableQuery)

	// Node Uncommitted Memory
	memUncommittedQuery := fmt.Sprintf(
		`sum(kube_node_status_allocatable{resource="memory"} * on(node) group_left() (sum by(node) (kube_pod_info{namespace="%s", pod=~"%s"}) > 0)) - sum(kube_pod_container_resource_requests{resource="memory"} * on(pod, namespace) group_left(node) (kube_pod_info{node=~".+"} * on(node) group_left() (sum by(node) (kube_pod_info{namespace="%s", pod=~"%s"}) > 0)))`,
		namespace, podRegex, namespace, podRegex,
	)
	batch.points(&result.Memory.NodeUncommitted, memUncommittedQuery)

	// --- Pod Count Metrics ---
	// Running pods
	podsRunningQuery := fmt.Sprintf(
		`count(kube_pod_status_phase{namespace="%s", pod=~"%s", phase="Running"})`,
		namespace, podRegex,
	)
	batch.points(&result.Pods.Running, podsRunningQuery)

	// Ready pods
	podsReadyQuery := fmt.Sprintf(
		`sum(kube_pod_status_ready{namespace="%s", pod=~"%s", condition="true"})`,
		namespace, podRegex,
	)
	batch.points(&result.Pods.Ready, podsReadyQuery)

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
		batch.points(&result.Pods.Desired, desiredQuery)
	}

	// --- Network Metrics ---
	// Receive bytes/s
	rxBytesQuery := fmt.Sprintf(
		`sum(rate(container_network_receive_bytes_total{namespace="%s", pod=~"%s"}[5m]))`,
		namespace, podRegex,
	)
	batch.points(&result.Network.ReceiveBytes, rxBytesQuery)

	// Transmit bytes/s
	txBytesQuery := fmt.Sprintf(
		`sum(rate(container_network_transmit_bytes_total{namespace="%s", pod=~"%s"}[5m]))`,
		namespace, podRegex,
	)
	batch.points(&result.Network.TransmitBytes, txBytesQuery)

	// Receive packets/s
	rxPacketsQuery := fmt.Sprintf(
		`sum(rate(container_network_receive_packets_total{namespace="%s", pod=~"%s"}[5m]))`,
		namespace, podRegex,
	)
	batch.points(&result.Network.ReceivePackets, rxPacketsQuery)

	// Transmit packets/s
	txPacketsQuery := fmt.Sprintf(
		`sum(rate(container_network_transmit_packets_total{namespace="%s", pod=~"%s"}[5m]))`,
		namespace, podRegex,
	)
	batch.points(&result.Network.TransmitPackets, txPacketsQuery)

	// Dropped packets (receive)
	rxDroppedQuery := fmt.Sprintf(
		`sum(rate(container_network_receive_packets_dropped_total{namespace="%s", pod=~"%s"}[5m]))`,
		namespace, podRegex,
	)
	batch.points(&result.Network.ReceiveDropped, rxDroppedQuery)

	// Dropped packets (transmit)
	txDroppedQuery := fmt.Sprintf(
		`sum(rate(container_network_transmit_packets_dropped_total{namespace="%s", pod=~"%s"}[5m]))`,
		namespace, podRegex,
	)
	batch.points(&result.Network.TransmitDropped, txDroppedQuery)

	// --- Restarts ---
	restartsQuery := fmt.Sprintf(
		`sum(kube_pod_container_status_restarts_total{namespace="%s", pod=~"%s"})`,
		namespace, podRegex,
	)
	batch.points(&result.Restarts, restartsQuery)

	batch.wait()
	return result, nil
}

// GetNamespaceMetricsHistoryWithContext retrieves historical metrics for a namespace with cancellation support
func (c *Client) GetNamespaceMetricsHistoryWithContext(ctx context.Context, contextName string, info *PrometheusInfo, namespace string, start, end time.Time, maxDataPoints int) (*NamespaceMetricsHistory, error) {
	step := calculateMetricsStep(start, end, maxDataPoints)
	batch := c.newRangeQueryBatch(ctx, contextName, info, start, end, step)

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
	batch.points(&result.CPU, cpuUsageQuery)

	// --- Memory Metrics ---
	// Memory Usage (bytes) - sum of all containers in the namespace
	memUsageQuery := fmt.Sprintf(
		`sum(container_memory_working_set_bytes{namespace="%s", container!="", container!="POD"})`,
		namespace,
	)
	batch.points(&result.Memory, memUsageQuery)

	// --- Pod Count ---
	podCountQuery := fmt.Sprintf(
		`count(kube_pod_info{namespace="%s"})`,
		namespace,
	)
	batch.points(&result.PodCount, podCountQuery)

	// --- Network Metrics ---
	// Receive bytes/s
	rxBytesQuery := fmt.Sprintf(
		`sum(rate(container_network_receive_bytes_total{namespace="%s"}[5m]))`,
		namespace,
	)
	batch.points(&result.Network.ReceiveBytes, rxBytesQuery)

	// Transmit bytes/s
	txBytesQuery := fmt.Sprintf(
		`sum(rate(container_network_transmit_bytes_total{namespace="%s"}[5m]))`,
		namespace,
	)
	batch.points(&result.Network.TransmitBytes, txBytesQuery)

	// Receive packets/s
	rxPacketsQuery := fmt.Sprintf(
		`sum(rate(container_network_receive_packets_total{namespace="%s"}[5m]))`,
		namespace,
	)
	batch.points(&result.Network.ReceivePackets, rxPacketsQuery)

	// Transmit packets/s
	txPacketsQuery := fmt.Sprintf(
		`sum(rate(container_network_transmit_packets_total{namespace="%s"}[5m]))`,
		namespace,
	)
	batch.points(&result.Network.TransmitPackets, txPacketsQuery)

	// Dropped packets (receive)
	rxDroppedQuery := fmt.Sprintf(
		`sum(rate(container_network_receive_packets_dropped_total{namespace="%s"}[5m]))`,
		namespace,
	)
	batch.points(&result.Network.ReceiveDropped, rxDroppedQuery)

	// Dropped packets (transmit)
	txDroppedQuery := fmt.Sprintf(
		`sum(rate(container_network_transmit_packets_dropped_total{namespace="%s"}[5m]))`,
		namespace,
	)
	batch.points(&result.Network.TransmitDropped, txDroppedQuery)

	batch.wait()
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

func nodeMemoryBucketQuery(aggregation, expression string, step time.Duration) string {
	return fmt.Sprintf(`%s_over_time((%s)[%ds:])`, aggregation, expression, int(step.Seconds()))
}

func nodeMemoryContributorQuery(nodeName string, step time.Duration) string {
	expression := fmt.Sprintf(`sum by(namespace, pod) (container_memory_working_set_bytes{node="%s", container!="", container!="POD"})`, nodeName)
	return fmt.Sprintf(`topk(5, %s)`, nodeMemoryBucketQuery("max", expression, step))
}

// nodeMemoryAllocatableHeadroomQuery reports the allocatable memory remaining
// after the working sets of scheduled workload containers. This is distinct
// from kubelet eviction headroom, which requires an observable root cgroup.
func nodeMemoryAllocatableHeadroomQuery(nodeName string, step time.Duration) string {
	expression := fmt.Sprintf(`clamp_min(kube_node_status_allocatable{node="%s", resource="memory"} - on(node) sum by(node) (container_memory_working_set_bytes{node="%s", container!="", container!="POD"}), 0)`, nodeName, nodeName)
	return nodeMemoryBucketQuery("min", expression, step)
}

func parseNodeMemoryContributors(series []struct {
	Metric map[string]string `json:"metric"`
	Value  []interface{}     `json:"value"`
	Values [][]interface{}   `json:"values"`
}) []NodeMemoryContributor {
	contributors := make([]NodeMemoryContributor, 0, len(series))
	for _, result := range series {
		namespace, pod := result.Metric["namespace"], result.Metric["pod"]
		if namespace == "" || pod == "" {
			continue
		}
		points := make([]MetricsDataPoint, 0, len(result.Values))
		for _, value := range result.Values {
			if len(value) < 2 {
				continue
			}
			ts, ok := value[0].(float64)
			if !ok {
				continue
			}
			valueString, ok := value[1].(string)
			if !ok {
				continue
			}
			metricValue, err := strconv.ParseFloat(valueString, 64)
			if err != nil {
				continue
			}
			points = append(points, MetricsDataPoint{Timestamp: int64(ts * 1000), Value: metricValue})
		}
		if len(points) > 0 {
			contributors = append(contributors, NodeMemoryContributor{Namespace: namespace, Pod: pod, WorkingSet: points})
		}
	}
	sort.Slice(contributors, func(i, j int) bool {
		peak := func(points []MetricsDataPoint) float64 {
			value := points[0].Value
			for _, point := range points[1:] {
				if point.Value > value {
					value = point.Value
				}
			}
			return value
		}
		left, right := peak(contributors[i].WorkingSet), peak(contributors[j].WorkingSet)
		if left != right {
			return left > right
		}
		if contributors[i].Namespace != contributors[j].Namespace {
			return contributors[i].Namespace < contributors[j].Namespace
		}
		return contributors[i].Pod < contributors[j].Pod
	})
	if len(contributors) > 5 {
		contributors = contributors[:5]
	}
	return contributors
}

func (c *Client) queryNodeMemoryContributorsWithContext(ctx context.Context, contextName string, info *PrometheusInfo, query string, start, end time.Time, step time.Duration) []NodeMemoryContributor {
	result, err := c.QueryPrometheusRangeWithContext(ctx, contextName, info, query, start, end, step)
	if err != nil {
		if !isCancelledError(err) {
			debug.LogK8s("Prometheus node memory contributor query failed (silent)", map[string]interface{}{"error": err.Error(), "query": query})
		}
		return []NodeMemoryContributor{}
	}
	return parseNodeMemoryContributors(result.Data.Result)
}

// NodeMetricsHistory holds historical metrics for a node
type NodeMetricsHistory struct {
	NodeName     string               `json:"nodeName"`
	CPU          *NodeResourceMetrics `json:"cpu"`
	Memory       *NodeResourceMetrics `json:"memory"`
	Pods         *NodePodMetrics      `json:"pods"`
	Network      *NetworkMetrics      `json:"network"`
	RangeStartMs int64                `json:"rangeStartMs"`
	RangeEndMs   int64                `json:"rangeEndMs"`
	StepMs       int64                `json:"stepMs"`
}

// NodeMemoryDiagnosticsHistory holds optional, memory-specific diagnostic
// histories. It is intentionally separate from the node overview response so
// opening Metrics never requests diagnostic series.
type NodeMemoryDiagnosticsHistory struct {
	NodeName           string                  `json:"nodeName"`
	Memory             *NodeResourceMetrics    `json:"memory"`
	MemoryContributors []NodeMemoryContributor `json:"memoryContributors"`
	RangeStartMs       int64                   `json:"rangeStartMs"`
	RangeEndMs         int64                   `json:"rangeEndMs"`
	StepMs             int64                   `json:"stepMs"`
}

// NodeResourceMetrics holds CPU or memory metrics for a node
type NodeResourceMetrics struct {
	Usage            []MetricsDataPoint `json:"usage"`
	Allocatable      []MetricsDataPoint `json:"allocatable"`
	Reserved         []MetricsDataPoint `json:"reserved"`
	Committed        []MetricsDataPoint `json:"committed"`
	Available        []MetricsDataPoint `json:"available,omitempty"`        // Memory only: Linux MemAvailable
	KubeletAvailable []MetricsDataPoint `json:"kubeletAvailable,omitempty"` // Memory only: allocatable capacity minus workload working sets
	WorkingSet       []MetricsDataPoint `json:"workingSet,omitempty"`       // Memory only: root-cgroup working set
	PageCache        []MetricsDataPoint `json:"pageCache,omitempty"`        // Memory only: cached pages and buffers
	Slab             []MetricsDataPoint `json:"slab,omitempty"`             // Memory only: kernel slab allocations
	SharedMemory     []MetricsDataPoint `json:"sharedMemory,omitempty"`     // Memory only: shmem, including tmpfs
	Capacity         []MetricsDataPoint `json:"capacity,omitempty"`         // Memory only: node capacity
	PodWorkingSet    []MetricsDataPoint `json:"podWorkingSet,omitempty"`    // Memory only: summed pod working sets
	Unexplained      []MetricsDataPoint `json:"unexplained,omitempty"`      // Memory only: root working set minus pods
	MemoryPressure   []MetricsDataPoint `json:"memoryPressure,omitempty"`   // Memory only: kube-state-metrics condition
}

// NodeMemoryContributor is a bounded pod working-set history for a node.
type NodeMemoryContributor struct {
	Namespace  string             `json:"namespace"`
	Pod        string             `json:"pod"`
	WorkingSet []MetricsDataPoint `json:"workingSet"`
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
	batch := c.newRangeQueryBatch(ctx, contextName, info, start, end, step)

	result := &NodeMetricsHistory{
		NodeName:     nodeName,
		CPU:          &NodeResourceMetrics{},
		Memory:       &NodeResourceMetrics{},
		Pods:         &NodePodMetrics{},
		Network:      &NetworkMetrics{},
		RangeStartMs: start.UnixMilli(),
		RangeEndMs:   end.UnixMilli(),
		StepMs:       step.Milliseconds(),
	}

	// node_uname_info's nodename is the OS hostname, which can differ in case from the
	// (always lowercase) Kubernetes node name — e.g. AKS VMSS instance "vmss00000A".
	// Match it case-insensitively; PromQL regex matchers are fully anchored.
	unameNodeMatcher := "(?i)" + regexp.QuoteMeta(nodeName)

	// --- CPU Metrics ---
	// CPU Usage (node-level when node-exporter is available, fallback to all pods on this node, in cores)
	cpuUsageQuery := fmt.Sprintf(
		`sum((1 - rate(node_cpu_seconds_total{mode="idle"}[5m])) * on(instance) group_left(nodename) node_uname_info{nodename=~"%s"}) or sum(rate(container_cpu_usage_seconds_total{node="%s", container!="", container!="POD"}[5m]))`,
		unameNodeMatcher, nodeName,
	)
	batch.points(&result.CPU.Usage, cpuUsageQuery)

	// CPU Allocatable (node capacity for pods, in cores)
	cpuAllocatableQuery := fmt.Sprintf(
		`kube_node_status_allocatable{node="%s", resource="cpu"}`,
		nodeName,
	)
	batch.points(&result.CPU.Allocatable, cpuAllocatableQuery)

	// CPU Reserved = sum of requests for pods on this node
	cpuReservedQuery := fmt.Sprintf(
		`sum(kube_pod_container_resource_requests{resource="cpu"} * on(namespace, pod) group_left() (kube_pod_info{node="%s"} > bool 0))`,
		nodeName,
	)
	batch.points(&result.CPU.Reserved, cpuReservedQuery)

	// CPU Committed = sum of max(usage, request) per container
	// First get requests for pods on this node, then join with usage and compute max
	cpuCommittedQuery := fmt.Sprintf(
		`sum(max by(namespace, pod, container) (label_replace(rate(container_cpu_usage_seconds_total{node="%s", container!="", container!="POD"}[5m]), "src", "usage", "", "") or label_replace(kube_pod_container_resource_requests{resource="cpu"} * on(namespace, pod) group_left() (kube_pod_info{node="%s"} > bool 0), "src", "request", "", "")))`,
		nodeName, nodeName,
	)
	batch.points(&result.CPU.Committed, cpuCommittedQuery)

	// --- Memory Metrics ---
	// Memory Usage (node-level when node-exporter is available, fallback to all pods on this node)
	memUsageQuery := fmt.Sprintf(
		`sum((node_memory_MemTotal_bytes - node_memory_MemAvailable_bytes) * on(instance) group_left(nodename) node_uname_info{nodename=~"%s"}) or sum(container_memory_working_set_bytes{node="%s", container!="", container!="POD"})`,
		unameNodeMatcher, nodeName,
	)
	batch.points(&result.Memory.Usage, memUsageQuery)

	// Memory Allocatable
	memAllocatableQuery := fmt.Sprintf(
		`kube_node_status_allocatable{node="%s", resource="memory"}`,
		nodeName,
	)
	batch.points(&result.Memory.Allocatable, memAllocatableQuery)

	// Memory Reserved = sum of requests for pods on this node
	memReservedQuery := fmt.Sprintf(
		`sum(kube_pod_container_resource_requests{resource="memory"} * on(namespace, pod) group_left() (kube_pod_info{node="%s"} > bool 0))`,
		nodeName,
	)
	batch.points(&result.Memory.Reserved, memReservedQuery)

	// Memory Committed = sum of max(usage, request) per container
	// First get requests for pods on this node, then join with usage and compute max
	memCommittedQuery := fmt.Sprintf(
		`sum(max by(namespace, pod, container) (label_replace(container_memory_working_set_bytes{node="%s", container!="", container!="POD"}, "src", "usage", "", "") or label_replace(kube_pod_container_resource_requests{resource="memory"} * on(namespace, pod) group_left() (kube_pod_info{node="%s"} > bool 0), "src", "request", "", "")))`,
		nodeName, nodeName,
	)
	batch.points(&result.Memory.Committed, memCommittedQuery)

	// --- Pod Count Metrics ---
	// Running pods on node
	podRunningQuery := fmt.Sprintf(
		`count(kube_pod_info{node="%s"})`,
		nodeName,
	)
	batch.points(&result.Pods.Running, podRunningQuery)

	// Pod capacity
	podCapacityQuery := fmt.Sprintf(
		`kube_node_status_capacity{node="%s", resource="pods"}`,
		nodeName,
	)
	batch.points(&result.Pods.Capacity, podCapacityQuery)

	// --- Network Metrics ---
	// Network receive bytes/s - join with node_uname_info to match by node name
	// Use positive device filter for physical interfaces (eth*, en*) which works across Linux distributions
	// Fallback to node/kubernetes_node labels for clusters where those are available
	rxBytesQuery := fmt.Sprintf(
		`sum(rate(node_network_receive_bytes_total{device=~"eth.*|en.*"}[5m]) * on(instance) group_left(nodename) node_uname_info{nodename=~"%s"}) or sum(rate(node_network_receive_bytes_total{node="%s", device=~"eth.*|en.*"}[5m])) or sum(rate(node_network_receive_bytes_total{kubernetes_node="%s", device=~"eth.*|en.*"}[5m]))`,
		unameNodeMatcher, nodeName, nodeName,
	)
	batch.points(&result.Network.ReceiveBytes, rxBytesQuery)

	// Network transmit bytes/s
	txBytesQuery := fmt.Sprintf(
		`sum(rate(node_network_transmit_bytes_total{device=~"eth.*|en.*"}[5m]) * on(instance) group_left(nodename) node_uname_info{nodename=~"%s"}) or sum(rate(node_network_transmit_bytes_total{node="%s", device=~"eth.*|en.*"}[5m])) or sum(rate(node_network_transmit_bytes_total{kubernetes_node="%s", device=~"eth.*|en.*"}[5m]))`,
		unameNodeMatcher, nodeName, nodeName,
	)
	batch.points(&result.Network.TransmitBytes, txBytesQuery)

	// Network receive packets/s
	rxPacketsQuery := fmt.Sprintf(
		`sum(rate(node_network_receive_packets_total{device=~"eth.*|en.*"}[5m]) * on(instance) group_left(nodename) node_uname_info{nodename=~"%s"}) or sum(rate(node_network_receive_packets_total{node="%s", device=~"eth.*|en.*"}[5m])) or sum(rate(node_network_receive_packets_total{kubernetes_node="%s", device=~"eth.*|en.*"}[5m]))`,
		unameNodeMatcher, nodeName, nodeName,
	)
	batch.points(&result.Network.ReceivePackets, rxPacketsQuery)

	// Network transmit packets/s
	txPacketsQuery := fmt.Sprintf(
		`sum(rate(node_network_transmit_packets_total{device=~"eth.*|en.*"}[5m]) * on(instance) group_left(nodename) node_uname_info{nodename=~"%s"}) or sum(rate(node_network_transmit_packets_total{node="%s", device=~"eth.*|en.*"}[5m])) or sum(rate(node_network_transmit_packets_total{kubernetes_node="%s", device=~"eth.*|en.*"}[5m]))`,
		unameNodeMatcher, nodeName, nodeName,
	)
	batch.points(&result.Network.TransmitPackets, txPacketsQuery)

	// Network receive dropped packets/s
	rxDroppedQuery := fmt.Sprintf(
		`sum(rate(node_network_receive_drop_total{device=~"eth.*|en.*"}[5m]) * on(instance) group_left(nodename) node_uname_info{nodename=~"%s"}) or sum(rate(node_network_receive_drop_total{node="%s", device=~"eth.*|en.*"}[5m])) or sum(rate(node_network_receive_drop_total{kubernetes_node="%s", device=~"eth.*|en.*"}[5m]))`,
		unameNodeMatcher, nodeName, nodeName,
	)
	batch.points(&result.Network.ReceiveDropped, rxDroppedQuery)

	// Network transmit dropped packets/s
	txDroppedQuery := fmt.Sprintf(
		`sum(rate(node_network_transmit_drop_total{device=~"eth.*|en.*"}[5m]) * on(instance) group_left(nodename) node_uname_info{nodename=~"%s"}) or sum(rate(node_network_transmit_drop_total{node="%s", device=~"eth.*|en.*"}[5m])) or sum(rate(node_network_transmit_drop_total{kubernetes_node="%s", device=~"eth.*|en.*"}[5m]))`,
		unameNodeMatcher, nodeName, nodeName,
	)
	batch.points(&result.Network.TransmitDropped, txDroppedQuery)

	batch.wait()
	return result, nil
}

// GetNodeMemoryDiagnosticsHistoryWithContext retrieves optional memory
// diagnostics separately from the overview history. These queries can be
// absent on a cluster, so every source remains independently optional.
func (c *Client) GetNodeMemoryDiagnosticsHistoryWithContext(ctx context.Context, contextName string, info *PrometheusInfo, nodeName string, start, end time.Time, maxDataPoints int) (*NodeMemoryDiagnosticsHistory, error) {
	step := calculateMetricsStep(start, end, maxDataPoints)
	batch := c.newRangeQueryBatch(ctx, contextName, info, start, end, step)
	result := &NodeMemoryDiagnosticsHistory{
		NodeName:           nodeName,
		Memory:             &NodeResourceMetrics{},
		MemoryContributors: []NodeMemoryContributor{},
		RangeStartMs:       start.UnixMilli(),
		RangeEndMs:         end.UnixMilli(),
		StepMs:             step.Milliseconds(),
	}

	// node_uname_info's hostname can differ in case from the Kubernetes node
	// name. PromQL regex matchers are anchored, so use a quoted matcher here.
	unameNodeMatcher := "(?i)" + regexp.QuoteMeta(nodeName)
	memAvailableQuery := nodeMemoryBucketQuery("min", fmt.Sprintf(
		`sum(node_memory_MemAvailable_bytes * on(instance) group_left(nodename) node_uname_info{nodename=~"%s"})`,
		unameNodeMatcher,
	), step)
	batch.points(&result.Memory.Available, memAvailableQuery)

	rootWorkingSetQuery := nodeMemoryBucketQuery("max", fmt.Sprintf(
		`max(container_memory_working_set_bytes{node="%s", id="/"}) or max(container_memory_working_set_bytes{id="/"} * on(instance) group_left(node) kubelet_node_name{node="%s"})`,
		nodeName, nodeName,
	), step)
	batch.points(&result.Memory.WorkingSet, rootWorkingSetQuery)
	batch.points(&result.Memory.KubeletAvailable, nodeMemoryAllocatableHeadroomQuery(nodeName, step))

	pageCacheQuery := nodeMemoryBucketQuery("max", fmt.Sprintf(
		`sum((node_memory_Cached_bytes + node_memory_Buffers_bytes) * on(instance) group_left(nodename) node_uname_info{nodename=~"%s"})`,
		unameNodeMatcher,
	), step)
	batch.points(&result.Memory.PageCache, pageCacheQuery)
	slabQuery := nodeMemoryBucketQuery("max", fmt.Sprintf(
		`sum(node_memory_Slab_bytes * on(instance) group_left(nodename) node_uname_info{nodename=~"%s"})`,
		unameNodeMatcher,
	), step)
	batch.points(&result.Memory.Slab, slabQuery)
	sharedMemoryQuery := nodeMemoryBucketQuery("max", fmt.Sprintf(
		`sum(node_memory_Shmem_bytes * on(instance) group_left(nodename) node_uname_info{nodename=~"%s"})`,
		unameNodeMatcher,
	), step)
	batch.points(&result.Memory.SharedMemory, sharedMemoryQuery)

	capacityQuery := nodeMemoryBucketQuery("max", fmt.Sprintf(`kube_node_status_capacity{node="%s", resource="memory"}`, nodeName), step)
	batch.points(&result.Memory.Capacity, capacityQuery)
	podWorkingSetExpr := fmt.Sprintf(`sum(container_memory_working_set_bytes{node="%s", container!="", container!="POD"})`, nodeName)
	batch.points(&result.Memory.PodWorkingSet, nodeMemoryBucketQuery("max", podWorkingSetExpr, step))
	unexplainedExpr := fmt.Sprintf(`clamp_min((max(container_memory_working_set_bytes{node="%s", id="/"}) or max(container_memory_working_set_bytes{id="/"} * on(instance) group_left(node) kubelet_node_name{node="%s"})) - (%s), 0)`, nodeName, nodeName, podWorkingSetExpr)
	batch.points(&result.Memory.Unexplained, nodeMemoryBucketQuery("max", unexplainedExpr, step))
	pressureQuery := nodeMemoryBucketQuery("max", fmt.Sprintf(`max(kube_node_status_condition{node="%s", condition="MemoryPressure", status="true"})`, nodeName), step)
	batch.points(&result.Memory.MemoryPressure, pressureQuery)
	batch.contributors(&result.MemoryContributors, nodeMemoryContributorQuery(nodeName, step))

	// Allocation diagnostics retain the same scheduling series as the overview
	// chart, but only load when the Diagnostics tab is selected.
	memAllocatableQuery := fmt.Sprintf(`kube_node_status_allocatable{node="%s", resource="memory"}`, nodeName)
	batch.points(&result.Memory.Allocatable, memAllocatableQuery)
	memReservedQuery := fmt.Sprintf(`sum(kube_pod_container_resource_requests{resource="memory"} * on(namespace, pod) group_left() (kube_pod_info{node="%s"} > bool 0))`, nodeName)
	batch.points(&result.Memory.Reserved, memReservedQuery)

	batch.wait()
	return result, nil
}

// getClientsetForContext gets a clientset for a specific context
