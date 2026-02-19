package k8s

import (
	"context"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsclientset "k8s.io/metrics/pkg/client/clientset/versioned"
)

func (c *Client) GetNodeMetrics() (*NodeMetricsResult, error) {
	if IsDebugClusterContext(c.GetCurrentContext()) {
		return &NodeMetricsResult{Available: false, Error: "metrics not available on debug cluster"}, nil
	}

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
	if IsDebugClusterContext(c.GetCurrentContext()) {
		return &PodMetricsResult{Available: false, Error: "metrics not available on debug cluster"}, nil
	}

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
