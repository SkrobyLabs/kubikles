package k8s

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	storagev1 "k8s.io/api/storage/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	metricsclientset "k8s.io/metrics/pkg/client/clientset/versioned"
	"sigs.k8s.io/yaml"
)

type Client struct {
	clientset      *kubernetes.Clientset
	metricsClient  metricsclientset.Interface
	configLoading  clientcmd.ClientConfig
	currentContext string
	mu             sync.RWMutex
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

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create clientset: %w", err)
	}

	c.clientset = clientset

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
	return c.loadConfig(contextName)
}

func (c *Client) GetCurrentContext() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.currentContext
}

func (c *Client) ListContexts() ([]string, error) {
	rawConfig, err := c.configLoading.RawConfig()
	if err != nil {
		return nil, err
	}
	var contexts []string
	for name := range rawConfig.Contexts {
		contexts = append(contexts, name)
	}
	return contexts, nil
}

// --- Resources ---

func (c *Client) getClientset() (*kubernetes.Clientset, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.clientset == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return c.clientset, nil
}

func (c *Client) ListPods(namespace string) ([]v1.Pod, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	pods, err := cs.CoreV1().Pods(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return pods.Items, nil
}

func (c *Client) WatchPods(ctx context.Context, namespace string) (watch.Interface, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	return cs.CoreV1().Pods(namespace).Watch(ctx, metav1.ListOptions{})
}

// WatchResource creates a watch for the specified resource type
// Supported resource types: pods, namespaces, nodes, events, deployments, statefulsets,
// daemonsets, replicasets, services, ingresses, ingressclasses, networkpolicies, configmaps, secrets,
// jobs, cronjobs, persistentvolumes, persistentvolumeclaims, storageclasses, hpas, pdbs, resourcequotas, limitranges
func (c *Client) WatchResource(ctx context.Context, resourceType, namespace string) (watch.Interface, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}

	opts := metav1.ListOptions{}

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

	default:
		return nil, fmt.Errorf("unsupported resource type: %s", resourceType)
	}
}

// WatchCRD creates a watch for a custom resource using the dynamic client
func (c *Client) WatchCRD(ctx context.Context, group, version, resource, namespace string) (watch.Interface, error) {
	dc, err := c.getDynamicClientForContext("")
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}

	gvr := schema.GroupVersionResource{
		Group:    group,
		Version:  version,
		Resource: resource,
	}

	opts := metav1.ListOptions{}

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

func (c *Client) ListNodes() ([]v1.Node, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	nodes, err := cs.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return nodes.Items, nil
}

func (c *Client) GetNodeYaml(name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	node, err := cs.CoreV1().Nodes().Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	// Remove managed fields
	node.ManagedFields = nil

	yamlBytes, err := yaml.Marshal(node)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateNodeYaml(name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	var node v1.Node
	if err := yaml.Unmarshal([]byte(yamlContent), &node); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	if node.Name != name {
		return fmt.Errorf("node name in YAML (%s) does not match expected name (%s)", node.Name, name)
	}
	_, err = cs.CoreV1().Nodes().Update(context.TODO(), &node, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteNode(contextName, name string) error {
	fmt.Printf("Deleting node: context=%s, name=%s\n", contextName, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	return cs.CoreV1().Nodes().Delete(context.TODO(), name, metav1.DeleteOptions{})
}

func (c *Client) SetNodeSchedulable(contextName, name string, schedulable bool) error {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}

	// Patch spec.unschedulable - true means cordoned (unschedulable), false means uncordoned
	patchData := fmt.Sprintf(`{"spec":{"unschedulable":%t}}`, !schedulable)

	_, err = cs.CoreV1().Nodes().Patch(
		context.TODO(),
		name,
		types.MergePatchType,
		[]byte(patchData),
		metav1.PatchOptions{},
	)
	if err != nil {
		return fmt.Errorf("failed to patch node: %w", err)
	}
	return nil
}

func (c *Client) CreateNodeDebugPod(contextName, nodeName string) (*v1.Pod, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}

	privileged := true
	debugPod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("node-shell-%s-", nodeName),
			Namespace:    "default",
		},
		Spec: v1.PodSpec{
			NodeName:      nodeName,
			HostPID:       true,
			HostNetwork:   true,
			HostIPC:       true,
			RestartPolicy: v1.RestartPolicyNever,
			Containers: []v1.Container{
				{
					Name:  "shell",
					Image: "alpine:latest",
					Command: []string{
						"sleep", "infinity",
					},
					Stdin: true,
					TTY:   true,
					SecurityContext: &v1.SecurityContext{
						Privileged: &privileged,
					},
				},
			},
		},
	}

	return cs.CoreV1().Pods("default").Create(context.TODO(), debugPod, metav1.CreateOptions{})
}

// GetNodeMetrics fetches CPU and Memory metrics for all nodes from the metrics-server
func (c *Client) GetNodeMetrics() (*NodeMetricsResult, error) {
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

	// Fetch node metrics from metrics-server
	nodeMetricsList, err := c.metricsClient.MetricsV1beta1().NodeMetricses().List(ctx, metav1.ListOptions{})
	if err != nil {
		return &NodeMetricsResult{Available: false, Error: err.Error()}, nil
	}

	// Fetch node capacities
	cs, err := c.getClientset()
	if err != nil {
		return &NodeMetricsResult{Available: false, Error: err.Error()}, nil
	}

	nodes, err := cs.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return &NodeMetricsResult{Available: false, Error: err.Error()}, nil
	}

	// Build capacity map
	capacityMap := make(map[string]struct{ cpu, memory int64 })
	for _, node := range nodes.Items {
		cpu := node.Status.Capacity.Cpu().MilliValue() // millicores
		mem := node.Status.Capacity.Memory().Value()   // bytes
		capacityMap[node.Name] = struct{ cpu, memory int64 }{cpu, mem}
	}

	// Fetch all pods to calculate requested resources per node
	pods, err := cs.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return &NodeMetricsResult{Available: false, Error: err.Error()}, nil
	}

	// Fetch pod metrics for committed calculation
	podMetricsList, err := c.metricsClient.MetricsV1beta1().PodMetricses("").List(ctx, metav1.ListOptions{})
	if err != nil {
		// Pod metrics might fail, but we can still return node metrics without committed
		podMetricsList = nil
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
		var reqCPU, reqMem, comCPU, comMem int64
		if res != nil {
			reqCPU = res.requestedCPU
			reqMem = res.requestedMem
			comCPU = res.committedCPU
			comMem = res.committedMem
		}
		result = append(result, NodeMetrics{
			Name:         nm.Name,
			CPUUsage:     nm.Usage.Cpu().MilliValue(), // millicores
			MemoryUsage:  nm.Usage.Memory().Value(),   // bytes
			CPUCapacity:  cap.cpu,
			MemCapacity:  cap.memory,
			CPURequested: reqCPU,
			MemRequested: reqMem,
			CPUCommitted: comCPU,
			MemCommitted: comMem,
		})
	}

	return &NodeMetricsResult{Available: true, Metrics: result}, nil
}

// GetPodMetrics fetches CPU and Memory metrics for all pods, relative to node capacity
func (c *Client) GetPodMetrics() (*PodMetricsResult, error) {
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

	// Fetch pod metrics from metrics-server
	podMetricsList, err := c.metricsClient.MetricsV1beta1().PodMetricses("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return &PodMetricsResult{Available: false, Error: err.Error()}, nil
	}

	// Fetch all pods to get requests and node assignments
	cs, err := c.getClientset()
	if err != nil {
		return &PodMetricsResult{Available: false, Error: err.Error()}, nil
	}

	pods, err := cs.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return &PodMetricsResult{Available: false, Error: err.Error()}, nil
	}

	// Fetch node capacities
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

	// Build pod info map: namespace/name -> {nodeName, requests}
	type podInfo struct {
		nodeName   string
		cpuReq     int64
		memReq     int64
	}
	podInfoMap := make(map[string]podInfo)
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

func (c *Client) ListNamespaces() ([]v1.Namespace, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	namespaces, err := cs.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return namespaces.Items, nil
}

// NamespaceResourceCounts holds the count of various resource types in a namespace
type NamespaceResourceCounts struct {
	Pods         int `json:"pods"`
	Deployments  int `json:"deployments"`
	StatefulSets int `json:"statefulsets"`
	DaemonSets   int `json:"daemonsets"`
	ReplicaSets  int `json:"replicasets"`
	Jobs         int `json:"jobs"`
	CronJobs     int `json:"cronjobs"`
	Services     int `json:"services"`
	Ingresses    int `json:"ingresses"`
	ConfigMaps   int `json:"configmaps"`
	Secrets      int `json:"secrets"`
	PVCs         int `json:"pvcs"`
}

// GetNamespaceResourceCounts returns counts of various resource types in a namespace
func (c *Client) GetNamespaceResourceCounts(namespace string) (*NamespaceResourceCounts, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}

	ctx := context.TODO()
	counts := &NamespaceResourceCounts{}

	// Use goroutines for parallel counting
	var wg sync.WaitGroup
	var mu sync.Mutex
	errChan := make(chan error, 12)

	// Pods
	wg.Add(1)
	go func() {
		defer wg.Done()
		list, err := cs.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			errChan <- err
			return
		}
		mu.Lock()
		counts.Pods = len(list.Items)
		mu.Unlock()
	}()

	// Deployments
	wg.Add(1)
	go func() {
		defer wg.Done()
		list, err := cs.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			errChan <- err
			return
		}
		mu.Lock()
		counts.Deployments = len(list.Items)
		mu.Unlock()
	}()

	// StatefulSets
	wg.Add(1)
	go func() {
		defer wg.Done()
		list, err := cs.AppsV1().StatefulSets(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			errChan <- err
			return
		}
		mu.Lock()
		counts.StatefulSets = len(list.Items)
		mu.Unlock()
	}()

	// DaemonSets
	wg.Add(1)
	go func() {
		defer wg.Done()
		list, err := cs.AppsV1().DaemonSets(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			errChan <- err
			return
		}
		mu.Lock()
		counts.DaemonSets = len(list.Items)
		mu.Unlock()
	}()

	// ReplicaSets
	wg.Add(1)
	go func() {
		defer wg.Done()
		list, err := cs.AppsV1().ReplicaSets(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			errChan <- err
			return
		}
		mu.Lock()
		counts.ReplicaSets = len(list.Items)
		mu.Unlock()
	}()

	// Jobs
	wg.Add(1)
	go func() {
		defer wg.Done()
		list, err := cs.BatchV1().Jobs(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			errChan <- err
			return
		}
		mu.Lock()
		counts.Jobs = len(list.Items)
		mu.Unlock()
	}()

	// CronJobs
	wg.Add(1)
	go func() {
		defer wg.Done()
		list, err := cs.BatchV1().CronJobs(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			errChan <- err
			return
		}
		mu.Lock()
		counts.CronJobs = len(list.Items)
		mu.Unlock()
	}()

	// Services
	wg.Add(1)
	go func() {
		defer wg.Done()
		list, err := cs.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			errChan <- err
			return
		}
		mu.Lock()
		counts.Services = len(list.Items)
		mu.Unlock()
	}()

	// Ingresses
	wg.Add(1)
	go func() {
		defer wg.Done()
		list, err := cs.NetworkingV1().Ingresses(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			errChan <- err
			return
		}
		mu.Lock()
		counts.Ingresses = len(list.Items)
		mu.Unlock()
	}()

	// ConfigMaps
	wg.Add(1)
	go func() {
		defer wg.Done()
		list, err := cs.CoreV1().ConfigMaps(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			errChan <- err
			return
		}
		mu.Lock()
		counts.ConfigMaps = len(list.Items)
		mu.Unlock()
	}()

	// Secrets
	wg.Add(1)
	go func() {
		defer wg.Done()
		list, err := cs.CoreV1().Secrets(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			errChan <- err
			return
		}
		mu.Lock()
		counts.Secrets = len(list.Items)
		mu.Unlock()
	}()

	// PVCs
	wg.Add(1)
	go func() {
		defer wg.Done()
		list, err := cs.CoreV1().PersistentVolumeClaims(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			errChan <- err
			return
		}
		mu.Lock()
		counts.PVCs = len(list.Items)
		mu.Unlock()
	}()

	wg.Wait()
	close(errChan)

	// Check for errors (return first error if any)
	for err := range errChan {
		if err != nil {
			return nil, err
		}
	}

	return counts, nil
}

func (c *Client) DeleteNamespace(name string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	return cs.CoreV1().Namespaces().Delete(context.TODO(), name, metav1.DeleteOptions{})
}

func (c *Client) GetNamespaceYAML(name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}

	ns, err := cs.CoreV1().Namespaces().Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	// Clean up fields that shouldn't be in the YAML
	ns.ManagedFields = nil

	yamlBytes, err := yaml.Marshal(ns)
	if err != nil {
		return "", fmt.Errorf("failed to marshal namespace to YAML: %w", err)
	}

	return string(yamlBytes), nil
}

func (c *Client) UpdateNamespaceYAML(name string, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}

	// Parse the YAML to a Namespace object
	var ns v1.Namespace
	if err := yaml.Unmarshal([]byte(yamlContent), &ns); err != nil {
		return fmt.Errorf("failed to parse yaml: %w", err)
	}

	// Ensure the name matches
	if ns.Name != name {
		return fmt.Errorf("namespace name in YAML (%s) does not match expected name (%s)", ns.Name, name)
	}

	_, err = cs.CoreV1().Namespaces().Update(context.TODO(), &ns, metav1.UpdateOptions{})
	return err
}

func (c *Client) ListEvents(namespace string) ([]v1.Event, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	events, err := cs.CoreV1().Events(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return events.Items, nil
}

func (c *Client) GetEventYAML(namespace, name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}

	event, err := cs.CoreV1().Events(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	event.ManagedFields = nil

	yamlBytes, err := yaml.Marshal(event)
	if err != nil {
		return "", fmt.Errorf("failed to marshal event to YAML: %w", err)
	}

	return string(yamlBytes), nil
}

func (c *Client) UpdateEventYAML(namespace, name string, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}

	var event v1.Event
	if err := yaml.Unmarshal([]byte(yamlContent), &event); err != nil {
		return fmt.Errorf("failed to parse yaml: %w", err)
	}

	if event.Namespace != namespace || event.Name != name {
		return fmt.Errorf("namespace/name mismatch in yaml")
	}

	_, err = cs.CoreV1().Events(namespace).Update(context.TODO(), &event, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteEvent(namespace, name string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	return cs.CoreV1().Events(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
}

func (c *Client) ListServices(namespace string) ([]v1.Service, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	services, err := cs.CoreV1().Services(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return services.Items, nil
}

func (c *Client) GetServiceYaml(namespace, name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	service, err := cs.CoreV1().Services(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	// Remove managed fields
	service.ManagedFields = nil

	yamlBytes, err := yaml.Marshal(service)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateServiceYaml(namespace, name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	var service v1.Service
	if err := yaml.Unmarshal([]byte(yamlContent), &service); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.CoreV1().Services(namespace).Update(context.TODO(), &service, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteService(contextName, namespace, name string) error {
	fmt.Printf("Deleting service: context=%s, ns=%s, name=%s\n", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	return cs.CoreV1().Services(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
}

// Ingress operations
func (c *Client) ListIngresses(namespace string) ([]networkingv1.Ingress, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	ingresses, err := cs.NetworkingV1().Ingresses(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return ingresses.Items, nil
}

func (c *Client) GetIngressYaml(namespace, name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	ingress, err := cs.NetworkingV1().Ingresses(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	// Remove managed fields
	ingress.ManagedFields = nil

	yamlBytes, err := yaml.Marshal(ingress)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateIngressYaml(namespace, name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	var ingress networkingv1.Ingress
	if err := yaml.Unmarshal([]byte(yamlContent), &ingress); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.NetworkingV1().Ingresses(namespace).Update(context.TODO(), &ingress, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteIngress(contextName, namespace, name string) error {
	fmt.Printf("Deleting ingress: context=%s, ns=%s, name=%s\n", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	return cs.NetworkingV1().Ingresses(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
}

// IngressClass operations (cluster-scoped)
func (c *Client) ListIngressClasses(contextName string) ([]networkingv1.IngressClass, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ingressClasses, err := cs.NetworkingV1().IngressClasses().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return ingressClasses.Items, nil
}

func (c *Client) GetIngressClassYaml(name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	ingressClass, err := cs.NetworkingV1().IngressClasses().Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	ingressClass.ManagedFields = nil

	yamlBytes, err := yaml.Marshal(ingressClass)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateIngressClassYaml(name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	var ingressClass networkingv1.IngressClass
	if err := yaml.Unmarshal([]byte(yamlContent), &ingressClass); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.NetworkingV1().IngressClasses().Update(context.TODO(), &ingressClass, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteIngressClass(contextName, name string) error {
	fmt.Printf("Deleting ingressclass: context=%s, name=%s\n", contextName, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	return cs.NetworkingV1().IngressClasses().Delete(context.TODO(), name, metav1.DeleteOptions{})
}

func (c *Client) ListConfigMaps(namespace string) ([]v1.ConfigMap, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	cms, err := cs.CoreV1().ConfigMaps(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return cms.Items, nil
}

func (c *Client) ListSecrets(namespace string) ([]v1.Secret, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	secrets, err := cs.CoreV1().Secrets(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	// Sanitize secrets? For now, we return them as is, UI should handle masking.
	return secrets.Items, nil
}

// ConfigMap YAML operations
func (c *Client) GetConfigMapYaml(namespace, name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	configMap, err := cs.CoreV1().ConfigMaps(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	// Remove managed fields
	configMap.ManagedFields = nil

	yamlBytes, err := yaml.Marshal(configMap)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateConfigMapYaml(namespace, name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	var configMap v1.ConfigMap
	if err := yaml.Unmarshal([]byte(yamlContent), &configMap); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.CoreV1().ConfigMaps(namespace).Update(context.TODO(), &configMap, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteConfigMap(namespace, name string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	return cs.CoreV1().ConfigMaps(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
}

// Secret YAML operations
func (c *Client) GetSecretYaml(namespace, name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	secret, err := cs.CoreV1().Secrets(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	// Remove managed fields
	secret.ManagedFields = nil

	yamlBytes, err := yaml.Marshal(secret)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateSecretYaml(namespace, name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	var secret v1.Secret
	if err := yaml.Unmarshal([]byte(yamlContent), &secret); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.CoreV1().Secrets(namespace).Update(context.TODO(), &secret, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteSecret(namespace, name string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	return cs.CoreV1().Secrets(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
}

// GetSecretData returns the secret's data as a map of key -> base64-encoded value
func (c *Client) GetSecretData(namespace, name string) (map[string]string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	secret, err := cs.CoreV1().Secrets(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	result := make(map[string]string)
	for k, v := range secret.Data {
		result[k] = string(v)
	}
	return result, nil
}

// UpdateSecretData updates the secret's data from a map of key -> value (values are raw strings, will be stored as bytes)
func (c *Client) UpdateSecretData(namespace, name string, data map[string]string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	secret, err := cs.CoreV1().Secrets(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	secret.Data = make(map[string][]byte)
	for k, v := range data {
		secret.Data[k] = []byte(v)
	}
	_, err = cs.CoreV1().Secrets(namespace).Update(context.TODO(), secret, metav1.UpdateOptions{})
	return err
}

func (c *Client) ListDeployments(namespace string) ([]appsv1.Deployment, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	deployments, err := cs.AppsV1().Deployments(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return deployments.Items, nil
}

func (c *Client) GetPodLogs(namespace, podName, containerName string, timestamps bool, previous bool, sinceTime string) (string, error) {
	// When sinceTime is set, we need to get logs starting from that time (first N lines after sinceTime)
	// Kubernetes TailLines gives last N lines, so we fetch all and truncate to first 200
	if sinceTime != "" {
		allLogs, err := c.getPodLogsWithOptions(namespace, podName, containerName, nil, timestamps, previous, sinceTime)
		if err != nil {
			return "", err
		}
		lines := strings.Split(allLogs, "\n")
		if len(lines) <= 200 {
			return allLogs, nil
		}
		return strings.Join(lines[:200], "\n"), nil
	}
	// Default: get last 200 lines
	return c.getPodLogsWithOptions(namespace, podName, containerName, func(i int64) *int64 { return &i }(200), timestamps, previous, sinceTime)
}

func (c *Client) GetAllPodLogs(namespace, podName, containerName string, timestamps bool, previous bool) (string, error) {
	return c.getPodLogsWithOptions(namespace, podName, containerName, nil, timestamps, previous, "")
}

// GetPodLogsFromStart fetches all logs and returns the first N lines (default 200)
func (c *Client) GetPodLogsFromStart(namespace, podName, containerName string, timestamps bool, previous bool, lineLimit int) (string, error) {
	allLogs, err := c.getPodLogsWithOptions(namespace, podName, containerName, nil, timestamps, previous, "")
	if err != nil {
		return "", err
	}
	if lineLimit <= 0 {
		lineLimit = 200
	}
	lines := strings.Split(allLogs, "\n")
	if len(lines) <= lineLimit {
		return allLogs, nil
	}
	return strings.Join(lines[:lineLimit], "\n"), nil
}

// GetPodLogsBefore fetches logs before a given timestamp.
// Returns up to lineLimit lines that occur before the specified timestamp.
// The beforeTime should be in RFC3339 format (e.g., 2024-11-26T14:30:00Z).
// Returns the logs and a boolean indicating if there are more logs before these.
func (c *Client) GetPodLogsBefore(namespace, podName, containerName string, timestamps bool, previous bool, beforeTime string, lineLimit int) (string, bool, error) {
	allLogs, err := c.getPodLogsWithOptions(namespace, podName, containerName, nil, true, previous, "") // Always fetch with timestamps to find position
	if err != nil {
		return "", false, err
	}
	if lineLimit <= 0 {
		lineLimit = 200
	}

	lines := strings.Split(allLogs, "\n")

	// Normalize beforeTime - extract just the comparable portion (first 30 chars if available)
	compareLen := 30
	if len(beforeTime) < compareLen {
		compareLen = len(beforeTime)
	}
	beforeTimePrefix := beforeTime[:compareLen]

	// Find the line index where timestamp >= beforeTime (strict: we want lines BEFORE this)
	cutoffIndex := -1
	for i, line := range lines {
		if len(line) >= 30 { // Timestamp is at least 30 chars: 2024-11-26T14:30:00.123456789Z
			lineTime := line[:30]
			// Use >= to find the first line at or after beforeTime
			// We exclude this line and all after it
			if lineTime >= beforeTimePrefix {
				cutoffIndex = i
				break
			}
		}
	}

	var resultLines []string
	hasMoreBefore := false

	if cutoffIndex == -1 {
		// beforeTime is after all logs, return last lineLimit lines
		if len(lines) > lineLimit {
			resultLines = lines[len(lines)-lineLimit:]
			hasMoreBefore = true
		} else {
			resultLines = lines
		}
	} else if cutoffIndex == 0 {
		// beforeTime is before all logs, nothing to return
		return "", false, nil
	} else {
		// Return lineLimit lines before cutoffIndex
		startIndex := cutoffIndex - lineLimit
		if startIndex < 0 {
			startIndex = 0
		} else {
			hasMoreBefore = true
		}
		resultLines = lines[startIndex:cutoffIndex]
	}

	// If caller doesn't want timestamps, strip them
	if !timestamps {
		for i, line := range resultLines {
			if len(line) > 31 {
				resultLines[i] = line[31:] // Skip timestamp and space
			}
		}
	}

	return strings.Join(resultLines, "\n"), hasMoreBefore, nil
}

// GetPodLogsAfter fetches logs after a given timestamp.
// Returns up to lineLimit lines that occur after the specified timestamp.
// The afterTime should be in RFC3339 format.
// Returns the logs and a boolean indicating if there are more logs after these.
func (c *Client) GetPodLogsAfter(namespace, podName, containerName string, timestamps bool, previous bool, afterTime string, lineLimit int) (string, bool, error) {
	// Always fetch with timestamps so we can properly compare
	allLogs, err := c.getPodLogsWithOptions(namespace, podName, containerName, nil, true, previous, afterTime)
	if err != nil {
		return "", false, err
	}
	if lineLimit <= 0 {
		lineLimit = 200
	}

	lines := strings.Split(allLogs, "\n")

	// Normalize afterTime for comparison
	compareLen := 30
	if len(afterTime) < compareLen {
		compareLen = len(afterTime)
	}
	afterTimePrefix := afterTime[:compareLen]

	// Skip lines that are at or before our afterTime marker (we already have these)
	startIdx := 0
	for i, line := range lines {
		if len(line) >= 30 {
			lineTime := line[:30]
			// Skip this line if its timestamp is <= afterTime (we already have it)
			if lineTime <= afterTimePrefix {
				startIdx = i + 1
				continue
			}
		}
		break
	}

	if startIdx >= len(lines) {
		return "", false, nil
	}

	lines = lines[startIdx:]

	hasMoreAfter := len(lines) > lineLimit
	if hasMoreAfter {
		lines = lines[:lineLimit]
	}

	// Strip timestamps if caller doesn't want them
	if !timestamps {
		for i, line := range lines {
			if len(line) > 31 {
				lines[i] = line[31:] // Skip timestamp and space
			}
		}
	}

	return strings.Join(lines, "\n"), hasMoreAfter, nil
}

func (c *Client) getPodLogsWithOptions(namespace, podName, containerName string, tailLines *int64, timestamps bool, previous bool, sinceTime string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}

	opts := &v1.PodLogOptions{
		TailLines:  tailLines,
		Timestamps: timestamps,
		Previous:   previous,
	}
	if containerName != "" {
		opts.Container = containerName
	}
	if sinceTime != "" {
		t, err := time.Parse(time.RFC3339, sinceTime)
		if err == nil {
			mt := metav1.NewTime(t)
			opts.SinceTime = &mt
		}
	}

	req := cs.CoreV1().Pods(namespace).GetLogs(podName, opts)

	podLogs, err := req.Stream(context.TODO())
	if err != nil {
		return "", err
	}
	defer podLogs.Close()

	buf := new(strings.Builder)
	_, err = io.Copy(buf, podLogs)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

// StreamPodLogs streams logs from a pod container and calls the callback for each line.
// It continues until the context is cancelled or an error occurs.
// The callback receives each log line as it arrives.
func (c *Client) StreamPodLogs(ctx context.Context, namespace, podName, containerName string, timestamps bool, tailLines int64, onLine func(line string)) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}

	opts := &v1.PodLogOptions{
		Follow:     true,
		Timestamps: timestamps,
	}
	if tailLines > 0 {
		opts.TailLines = &tailLines
	}
	if containerName != "" {
		opts.Container = containerName
	}

	req := cs.CoreV1().Pods(namespace).GetLogs(podName, opts)
	stream, err := req.Stream(ctx)
	if err != nil {
		return err
	}
	defer stream.Close()

	scanner := bufio.NewScanner(stream)
	// Increase buffer size for long log lines
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			onLine(scanner.Text())
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}
	return nil
}

func (c *Client) getClientForContext(contextName string) (*kubernetes.Clientset, error) {
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

func (c *Client) DeletePod(contextName, namespace, name string) error {
	fmt.Printf("Deleting pod: context=%s, ns=%s, name=%s\n", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	return cs.CoreV1().Pods(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
}

func (c *Client) ForceDeletePod(contextName, namespace, name string) error {
	fmt.Printf("Force deleting pod: context=%s, ns=%s, name=%s\n", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	gracePeriod := int64(0)
	return cs.CoreV1().Pods(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{
		GracePeriodSeconds: &gracePeriod,
	})
}

func (c *Client) GetPodYaml(namespace, name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	pod, err := cs.CoreV1().Pods(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	// Remove managed fields to make it cleaner for editing
	pod.ManagedFields = nil

	y, err := yaml.Marshal(pod)
	if err != nil {
		return "", err
	}
	return string(y), nil
}

func (c *Client) UpdatePodYaml(namespace, name, content string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}

	// Parse the YAML to a Pod object
	var pod v1.Pod
	if err := yaml.Unmarshal([]byte(content), &pod); err != nil {
		return fmt.Errorf("failed to parse yaml: %w", err)
	}

	// Ensure namespace and name match
	if pod.Namespace != namespace || pod.Name != name {
		return fmt.Errorf("namespace/name mismatch in yaml")
	}

	_, err = cs.CoreV1().Pods(namespace).Update(context.TODO(), &pod, metav1.UpdateOptions{})
	return err
}

func (c *Client) GetDeploymentYaml(namespace, name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	deployment, err := cs.AppsV1().Deployments(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	// Remove managed fields
	deployment.ManagedFields = nil

	y, err := yaml.Marshal(deployment)
	if err != nil {
		return "", err
	}
	return string(y), nil
}

func (c *Client) UpdateDeploymentYaml(namespace, name, content string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}

	var deployment appsv1.Deployment
	if err := yaml.Unmarshal([]byte(content), &deployment); err != nil {
		return fmt.Errorf("failed to parse yaml: %w", err)
	}

	if deployment.Namespace != namespace || deployment.Name != name {
		return fmt.Errorf("namespace/name mismatch in yaml")
	}

	_, err = cs.AppsV1().Deployments(namespace).Update(context.TODO(), &deployment, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteDeployment(contextName, namespace, name string) error {
	fmt.Printf("Deleting deployment: context=%s, ns=%s, name=%s\n", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	return cs.AppsV1().Deployments(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
}

func (c *Client) RestartDeployment(contextName, namespace, name string) error {
	fmt.Printf("Restarting deployment: context=%s, ns=%s, name=%s\n", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}

	// Patch the deployment to trigger a rollout
	// We update the spec.template.metadata.annotations with a timestamp
	patch := fmt.Sprintf(`{"spec":{"template":{"metadata":{"annotations":{"kubectl.kubernetes.io/restartedAt":"%s"}}}}}`, metav1.Now().String())
	_, err = cs.AppsV1().Deployments(namespace).Patch(context.TODO(), name, types.StrategicMergePatchType, []byte(patch), metav1.PatchOptions{})
	return err
}

// StatefulSet operations
func (c *Client) ListStatefulSets(contextName, namespace string) ([]appsv1.StatefulSet, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	statefulsets, err := cs.AppsV1().StatefulSets(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return statefulsets.Items, nil
}

func (c *Client) GetStatefulSetYaml(namespace, name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	statefulset, err := cs.AppsV1().StatefulSets(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	// Remove managed fields
	statefulset.ManagedFields = nil

	yamlBytes, err := yaml.Marshal(statefulset)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateStatefulSetYaml(namespace, name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	var statefulset appsv1.StatefulSet
	if err := yaml.Unmarshal([]byte(yamlContent), &statefulset); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.AppsV1().StatefulSets(namespace).Update(context.TODO(), &statefulset, metav1.UpdateOptions{})
	return err
}

func (c *Client) RestartStatefulSet(contextName, namespace, name string) error {
	fmt.Printf("Restarting statefulset: context=%s, ns=%s, name=%s\n", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}

	// Patch the statefulset to trigger a rollout
	patch := fmt.Sprintf(`{"spec":{"template":{"metadata":{"annotations":{"kubectl.kubernetes.io/restartedAt":"%s"}}}}}`, metav1.Now().String())
	_, err = cs.AppsV1().StatefulSets(namespace).Patch(context.TODO(), name, types.StrategicMergePatchType, []byte(patch), metav1.PatchOptions{})
	return err
}

func (c *Client) DeleteStatefulSet(contextName, namespace, name string) error {
	fmt.Printf("Deleting statefulset: context=%s, ns=%s, name=%s\n", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	return cs.AppsV1().StatefulSets(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
}

// DaemonSet operations
func (c *Client) ListDaemonSets(contextName, namespace string) ([]appsv1.DaemonSet, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	daemonsets, err := cs.AppsV1().DaemonSets(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return daemonsets.Items, nil
}

func (c *Client) GetDaemonSetYaml(namespace, name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	daemonset, err := cs.AppsV1().DaemonSets(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	// Remove managed fields
	daemonset.ManagedFields = nil

	yamlBytes, err := yaml.Marshal(daemonset)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateDaemonSetYaml(namespace, name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	var daemonset appsv1.DaemonSet
	if err := yaml.Unmarshal([]byte(yamlContent), &daemonset); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.AppsV1().DaemonSets(namespace).Update(context.TODO(), &daemonset, metav1.UpdateOptions{})
	return err
}

func (c *Client) RestartDaemonSet(contextName, namespace, name string) error {
	fmt.Printf("Restarting daemonset: context=%s, ns=%s, name=%s\n", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}

	// Patch the daemonset to trigger a rollout
	patch := fmt.Sprintf(`{"spec":{"template":{"metadata":{"annotations":{"kubectl.kubernetes.io/restartedAt":"%s"}}}}}`, metav1.Now().String())
	_, err = cs.AppsV1().DaemonSets(namespace).Patch(context.TODO(), name, types.StrategicMergePatchType, []byte(patch), metav1.PatchOptions{})
	return err
}

func (c *Client) DeleteDaemonSet(contextName, namespace, name string) error {
	fmt.Printf("Deleting daemonset: context=%s, ns=%s, name=%s\n", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	return cs.AppsV1().DaemonSets(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
}

// ReplicaSet operations
func (c *Client) ListReplicaSets(contextName, namespace string) ([]appsv1.ReplicaSet, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	replicasets, err := cs.AppsV1().ReplicaSets(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return replicasets.Items, nil
}

func (c *Client) GetReplicaSetYaml(namespace, name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	replicaset, err := cs.AppsV1().ReplicaSets(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	// Remove managed fields
	replicaset.ManagedFields = nil

	yamlBytes, err := yaml.Marshal(replicaset)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateReplicaSetYaml(namespace, name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	var replicaset appsv1.ReplicaSet
	if err := yaml.Unmarshal([]byte(yamlContent), &replicaset); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.AppsV1().ReplicaSets(namespace).Update(context.TODO(), &replicaset, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteReplicaSet(contextName, namespace, name string) error {
	fmt.Printf("Deleting replicaset: context=%s, ns=%s, name=%s\n", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	return cs.AppsV1().ReplicaSets(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
}

// Job operations
func (c *Client) ListJobs(contextName, namespace string) ([]batchv1.Job, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	jobs, err := cs.BatchV1().Jobs(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return jobs.Items, nil
}

func (c *Client) GetJobYaml(namespace, name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	job, err := cs.BatchV1().Jobs(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	yamlBytes, err := yaml.Marshal(job)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateJobYaml(namespace, name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	var job batchv1.Job
	if err := yaml.Unmarshal([]byte(yamlContent), &job); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.BatchV1().Jobs(namespace).Update(context.TODO(), &job, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteJob(contextName, namespace, name string) error {
	fmt.Printf("Deleting job: context=%s, ns=%s, name=%s\n", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	return cs.BatchV1().Jobs(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
}

// CronJob operations
func (c *Client) ListCronJobs(contextName, namespace string) ([]batchv1.CronJob, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	cronJobs, err := cs.BatchV1().CronJobs(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return cronJobs.Items, nil
}

func (c *Client) GetCronJobYaml(namespace, name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	cronJob, err := cs.BatchV1().CronJobs(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	yamlBytes, err := yaml.Marshal(cronJob)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateCronJobYaml(namespace, name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	var cronJob batchv1.CronJob
	if err := yaml.Unmarshal([]byte(yamlContent), &cronJob); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.BatchV1().CronJobs(namespace).Update(context.TODO(), &cronJob, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteCronJob(contextName, namespace, name string) error {
	fmt.Printf("Deleting cronjob: context=%s, ns=%s, name=%s\n", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	return cs.BatchV1().CronJobs(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
}

func (c *Client) TriggerCronJob(contextName, namespace, cronJobName string) error {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}

	// Get the CronJob to use as template
	cronJob, err := cs.BatchV1().CronJobs(namespace).Get(context.TODO(), cronJobName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get cronjob: %w", err)
	}

	// Create a Job from the CronJob spec
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: cronJobName + "-manual-",
			Namespace:    namespace,
			Annotations: map[string]string{
				"cronjob.kubernetes.io/instantiate": "manual",
			},
		},
		Spec: cronJob.Spec.JobTemplate.Spec,
	}

	_, err = cs.BatchV1().Jobs(namespace).Create(context.TODO(), job, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create job: %w", err)
	}

	return nil
}

func (c *Client) SuspendCronJob(contextName, namespace, name string, suspend bool) error {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}

	// Use JSON patch to update only the suspend field
	patchData := fmt.Sprintf(`{"spec":{"suspend":%t}}`, suspend)

	result, err := cs.BatchV1().CronJobs(namespace).Patch(
		context.TODO(),
		name,
		types.MergePatchType,
		[]byte(patchData),
		metav1.PatchOptions{},
	)
	if err != nil {
		return fmt.Errorf("failed to patch cronjob: %w", err)
	}

	_ = result
	return nil
}

// PersistentVolumeClaim operations
func (c *Client) ListPVCs(contextName, namespace string) ([]v1.PersistentVolumeClaim, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	pvcs, err := cs.CoreV1().PersistentVolumeClaims(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return pvcs.Items, nil
}

func (c *Client) GetPVCYaml(namespace, name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	pvc, err := cs.CoreV1().PersistentVolumeClaims(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	pvc.ManagedFields = nil

	yamlBytes, err := yaml.Marshal(pvc)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdatePVCYaml(namespace, name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	var pvc v1.PersistentVolumeClaim
	if err := yaml.Unmarshal([]byte(yamlContent), &pvc); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.CoreV1().PersistentVolumeClaims(namespace).Update(context.TODO(), &pvc, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeletePVC(contextName, namespace, name string) error {
	fmt.Printf("Deleting PVC: context=%s, ns=%s, name=%s\n", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	return cs.CoreV1().PersistentVolumeClaims(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
}

func (c *Client) ResizePVC(contextName, namespace, name, newSize string) error {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}

	// Get current PVC
	pvc, err := cs.CoreV1().PersistentVolumeClaims(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get PVC: %w", err)
	}

	// Parse and validate new size
	newQuantity, err := resource.ParseQuantity(newSize)
	if err != nil {
		return fmt.Errorf("invalid size format: %w", err)
	}

	// Check that new size is larger than current
	currentSize := pvc.Spec.Resources.Requests[v1.ResourceStorage]
	if newQuantity.Cmp(currentSize) <= 0 {
		return fmt.Errorf("new size must be larger than current size (%s)", currentSize.String())
	}

	// Update the storage request
	pvc.Spec.Resources.Requests[v1.ResourceStorage] = newQuantity

	_, err = cs.CoreV1().PersistentVolumeClaims(namespace).Update(context.TODO(), pvc, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to resize PVC: %w", err)
	}

	return nil
}

// PersistentVolume operations (cluster-scoped)
func (c *Client) ListPVs(contextName string) ([]v1.PersistentVolume, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	pvs, err := cs.CoreV1().PersistentVolumes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return pvs.Items, nil
}

func (c *Client) GetPVYaml(name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	pv, err := cs.CoreV1().PersistentVolumes().Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	pv.ManagedFields = nil

	yamlBytes, err := yaml.Marshal(pv)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdatePVYaml(name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	var pv v1.PersistentVolume
	if err := yaml.Unmarshal([]byte(yamlContent), &pv); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.CoreV1().PersistentVolumes().Update(context.TODO(), &pv, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeletePV(contextName, name string) error {
	fmt.Printf("Deleting PV: context=%s, name=%s\n", contextName, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	return cs.CoreV1().PersistentVolumes().Delete(context.TODO(), name, metav1.DeleteOptions{})
}

// StorageClass operations (cluster-scoped)
func (c *Client) GetStorageClass(contextName, name string) (*storagev1.StorageClass, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	sc, err := cs.StorageV1().StorageClasses().Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return sc, nil
}

func (c *Client) ListStorageClasses(contextName string) ([]storagev1.StorageClass, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	scs, err := cs.StorageV1().StorageClasses().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return scs.Items, nil
}

func (c *Client) GetStorageClassYaml(name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	sc, err := cs.StorageV1().StorageClasses().Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	sc.ManagedFields = nil

	yamlBytes, err := yaml.Marshal(sc)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateStorageClassYaml(name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	var sc storagev1.StorageClass
	if err := yaml.Unmarshal([]byte(yamlContent), &sc); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.StorageV1().StorageClasses().Update(context.TODO(), &sc, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteStorageClass(contextName, name string) error {
	fmt.Printf("Deleting StorageClass: context=%s, name=%s\n", contextName, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	return cs.StorageV1().StorageClasses().Delete(context.TODO(), name, metav1.DeleteOptions{})
}

// getApiExtensionsClientForContext returns an apiextensions clientset for a given context
func (c *Client) getApiExtensionsClientForContext(contextName string) (*apiextensionsclientset.Clientset, error) {
	home := homedir.HomeDir()
	kubeconfigPath := filepath.Join(home, ".kube", "config")
	loadingRules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath}

	configOverrides := &clientcmd.ConfigOverrides{}
	if contextName != "" {
		configOverrides.CurrentContext = contextName
	}

	configLoader := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
	config, err := configLoader.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load client config for context %s: %w", contextName, err)
	}

	return apiextensionsclientset.NewForConfig(config)
}

// CustomResourceDefinition operations (cluster-scoped)
func (c *Client) ListCRDs(contextName string) ([]apiextensionsv1.CustomResourceDefinition, error) {
	cs, err := c.getApiExtensionsClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get apiextensions client for context %s: %w", contextName, err)
	}
	crds, err := cs.ApiextensionsV1().CustomResourceDefinitions().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return crds.Items, nil
}

func (c *Client) GetCRDYaml(contextName, name string) (string, error) {
	cs, err := c.getApiExtensionsClientForContext(contextName)
	if err != nil {
		return "", fmt.Errorf("failed to get apiextensions client: %w", err)
	}
	crd, err := cs.ApiextensionsV1().CustomResourceDefinitions().Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	crd.ManagedFields = nil

	yamlBytes, err := yaml.Marshal(crd)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateCRDYaml(contextName, name, yamlContent string) error {
	cs, err := c.getApiExtensionsClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get apiextensions client: %w", err)
	}
	var crd apiextensionsv1.CustomResourceDefinition
	if err := yaml.Unmarshal([]byte(yamlContent), &crd); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.ApiextensionsV1().CustomResourceDefinitions().Update(context.TODO(), &crd, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteCRD(contextName, name string) error {
	fmt.Printf("Deleting CRD: context=%s, name=%s\n", contextName, name)
	cs, err := c.getApiExtensionsClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get apiextensions client for context %s: %w", contextName, err)
	}
	return cs.ApiextensionsV1().CustomResourceDefinitions().Delete(context.TODO(), name, metav1.DeleteOptions{})
}

// PrinterColumn represents an additional printer column from a CRD
type PrinterColumn struct {
	Name        string `json:"name"`
	Type        string `json:"type"`        // string, integer, number, boolean, date
	JSONPath    string `json:"jsonPath"`    // JSONPath expression to extract value
	Description string `json:"description"` // Optional description
	Priority    int32  `json:"priority"`    // 0 = always show, higher = hide in narrow views
}

// GetCRDPrinterColumns returns the additional printer columns for a CRD
func (c *Client) GetCRDPrinterColumns(contextName, crdName string) ([]PrinterColumn, error) {
	cs, err := c.getApiExtensionsClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get apiextensions client: %w", err)
	}

	crd, err := cs.ApiextensionsV1().CustomResourceDefinitions().Get(context.TODO(), crdName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get CRD %s: %w", crdName, err)
	}

	var columns []PrinterColumn

	// Find the served version's printer columns
	for _, version := range crd.Spec.Versions {
		if version.Served {
			for _, col := range version.AdditionalPrinterColumns {
				columns = append(columns, PrinterColumn{
					Name:        col.Name,
					Type:        col.Type,
					JSONPath:    col.JSONPath,
					Description: col.Description,
					Priority:    col.Priority,
				})
			}
			break // Use the first served version
		}
	}

	return columns, nil
}

// getDynamicClientForContext returns a dynamic client for a given context
func (c *Client) getDynamicClientForContext(contextName string) (dynamic.Interface, error) {
	home := homedir.HomeDir()
	kubeconfigPath := filepath.Join(home, ".kube", "config")
	loadingRules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath}

	configOverrides := &clientcmd.ConfigOverrides{}
	if contextName != "" {
		configOverrides.CurrentContext = contextName
	}

	configLoader := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
	config, err := configLoader.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load client config for context %s: %w", contextName, err)
	}

	return dynamic.NewForConfig(config)
}

// CustomResourceInfo represents metadata about a custom resource instance
type CustomResourceInfo struct {
	Name              string `json:"name"`
	Namespace         string `json:"namespace,omitempty"`
	CreationTimestamp string `json:"creationTimestamp"`
	UID               string `json:"uid"`
}

// ListCustomResources lists instances of a custom resource
func (c *Client) ListCustomResources(contextName, group, version, resource, namespace string) ([]map[string]interface{}, error) {
	dc, err := c.getDynamicClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get dynamic client for context %s: %w", contextName, err)
	}

	gvr := schema.GroupVersionResource{
		Group:    group,
		Version:  version,
		Resource: resource,
	}

	var list *unstructured.UnstructuredList
	if namespace != "" {
		list, err = dc.Resource(gvr).Namespace(namespace).List(context.TODO(), metav1.ListOptions{})
	} else {
		list, err = dc.Resource(gvr).List(context.TODO(), metav1.ListOptions{})
	}
	if err != nil {
		return nil, err
	}

	// Convert to slice of maps for easier JSON serialization
	result := make([]map[string]interface{}, len(list.Items))
	for i, item := range list.Items {
		result[i] = item.Object
	}
	return result, nil
}

// GetCustomResourceYaml gets a custom resource instance as YAML
func (c *Client) GetCustomResourceYaml(contextName, group, version, resource, namespace, name string) (string, error) {
	dc, err := c.getDynamicClientForContext(contextName)
	if err != nil {
		return "", fmt.Errorf("failed to get dynamic client: %w", err)
	}

	gvr := schema.GroupVersionResource{
		Group:    group,
		Version:  version,
		Resource: resource,
	}

	var obj *unstructured.Unstructured
	if namespace != "" {
		obj, err = dc.Resource(gvr).Namespace(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	} else {
		obj, err = dc.Resource(gvr).Get(context.TODO(), name, metav1.GetOptions{})
	}
	if err != nil {
		return "", err
	}

	// Remove managed fields
	unstructured.RemoveNestedField(obj.Object, "metadata", "managedFields")

	yamlBytes, err := yaml.Marshal(obj.Object)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

// UpdateCustomResourceYaml updates a custom resource instance from YAML
func (c *Client) UpdateCustomResourceYaml(contextName, group, version, resource, namespace, name, yamlContent string) error {
	dc, err := c.getDynamicClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get dynamic client: %w", err)
	}

	gvr := schema.GroupVersionResource{
		Group:    group,
		Version:  version,
		Resource: resource,
	}

	var obj map[string]interface{}
	if err := yaml.Unmarshal([]byte(yamlContent), &obj); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}

	unstructuredObj := &unstructured.Unstructured{Object: obj}

	if namespace != "" {
		_, err = dc.Resource(gvr).Namespace(namespace).Update(context.TODO(), unstructuredObj, metav1.UpdateOptions{})
	} else {
		_, err = dc.Resource(gvr).Update(context.TODO(), unstructuredObj, metav1.UpdateOptions{})
	}
	return err
}

// DeleteCustomResource deletes a custom resource instance
func (c *Client) DeleteCustomResource(contextName, group, version, resource, namespace, name string) error {
	fmt.Printf("Deleting custom resource: context=%s, gvr=%s/%s/%s, ns=%s, name=%s\n", contextName, group, version, resource, namespace, name)
	dc, err := c.getDynamicClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get dynamic client for context %s: %w", contextName, err)
	}

	gvr := schema.GroupVersionResource{
		Group:    group,
		Version:  version,
		Resource: resource,
	}

	if namespace != "" {
		return dc.Resource(gvr).Namespace(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
	}
	return dc.Resource(gvr).Delete(context.TODO(), name, metav1.DeleteOptions{})
}

// --- Port Forwarding Support ---

// GetRestConfigForContext returns the REST config for a specific context
func (c *Client) GetRestConfigForContext(contextName string) (*rest.Config, error) {
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

// GetServiceBackingPods finds running pods that back a service
func (c *Client) GetServiceBackingPods(contextName, namespace, serviceName string) ([]string, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get clientset for context %s: %w", contextName, err)
	}

	// Get the service
	svc, err := cs.CoreV1().Services(namespace).Get(context.TODO(), serviceName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get service %s: %w", serviceName, err)
	}

	// Build label selector from service selector
	if len(svc.Spec.Selector) == 0 {
		return nil, fmt.Errorf("service %s has no selector", serviceName)
	}

	var selectorParts []string
	for k, v := range svc.Spec.Selector {
		selectorParts = append(selectorParts, fmt.Sprintf("%s=%s", k, v))
	}
	selector := strings.Join(selectorParts, ",")

	// Find pods matching selector
	pods, err := cs.CoreV1().Pods(namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: selector,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	// Return names of running pods
	var result []string
	for _, pod := range pods.Items {
		if pod.Status.Phase == v1.PodRunning {
			result = append(result, pod.Name)
		}
	}

	return result, nil
}

// GetPodContainerPorts returns the container ports for a pod
func (c *Client) GetPodContainerPorts(contextName, namespace, podName string) ([]int32, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get clientset for context %s: %w", contextName, err)
	}

	pod, err := cs.CoreV1().Pods(namespace).Get(context.TODO(), podName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get pod %s: %w", podName, err)
	}

	var ports []int32
	for _, container := range pod.Spec.Containers {
		for _, port := range container.Ports {
			ports = append(ports, port.ContainerPort)
		}
	}

	return ports, nil
}

// GetServicePorts returns the ports exposed by a service
func (c *Client) GetServicePorts(contextName, namespace, serviceName string) ([]int32, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get clientset for context %s: %w", contextName, err)
	}

	svc, err := cs.CoreV1().Services(namespace).Get(context.TODO(), serviceName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get service %s: %w", serviceName, err)
	}

	var ports []int32
	for _, port := range svc.Spec.Ports {
		ports = append(ports, port.Port)
	}

	return ports, nil
}

// ==================== RBAC / Access Control ====================

// ServiceAccount operations
func (c *Client) ListServiceAccounts(namespace string) ([]v1.ServiceAccount, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	list, err := cs.CoreV1().ServiceAccounts(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) GetServiceAccountYaml(namespace, name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	sa, err := cs.CoreV1().ServiceAccounts(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	sa.ManagedFields = nil
	yamlBytes, err := yaml.Marshal(sa)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateServiceAccountYaml(namespace, name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	var sa v1.ServiceAccount
	if err := yaml.Unmarshal([]byte(yamlContent), &sa); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.CoreV1().ServiceAccounts(namespace).Update(context.TODO(), &sa, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteServiceAccount(contextName, namespace, name string) error {
	fmt.Printf("Deleting service account: context=%s, ns=%s, name=%s\n", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	return cs.CoreV1().ServiceAccounts(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
}

// Role operations (namespaced)
func (c *Client) ListRoles(namespace string) ([]rbacv1.Role, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	list, err := cs.RbacV1().Roles(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) GetRoleYaml(namespace, name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	role, err := cs.RbacV1().Roles(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	role.ManagedFields = nil
	yamlBytes, err := yaml.Marshal(role)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateRoleYaml(namespace, name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	var role rbacv1.Role
	if err := yaml.Unmarshal([]byte(yamlContent), &role); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.RbacV1().Roles(namespace).Update(context.TODO(), &role, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteRole(contextName, namespace, name string) error {
	fmt.Printf("Deleting role: context=%s, ns=%s, name=%s\n", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	return cs.RbacV1().Roles(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
}

// ClusterRole operations (cluster-scoped)
func (c *Client) ListClusterRoles() ([]rbacv1.ClusterRole, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	list, err := cs.RbacV1().ClusterRoles().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) GetClusterRoleYaml(name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	role, err := cs.RbacV1().ClusterRoles().Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	role.ManagedFields = nil
	yamlBytes, err := yaml.Marshal(role)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateClusterRoleYaml(name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	var role rbacv1.ClusterRole
	if err := yaml.Unmarshal([]byte(yamlContent), &role); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.RbacV1().ClusterRoles().Update(context.TODO(), &role, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteClusterRole(contextName, name string) error {
	fmt.Printf("Deleting cluster role: context=%s, name=%s\n", contextName, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	return cs.RbacV1().ClusterRoles().Delete(context.TODO(), name, metav1.DeleteOptions{})
}

// RoleBinding operations (namespaced)
func (c *Client) ListRoleBindings(namespace string) ([]rbacv1.RoleBinding, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	list, err := cs.RbacV1().RoleBindings(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) GetRoleBindingYaml(namespace, name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	binding, err := cs.RbacV1().RoleBindings(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	binding.ManagedFields = nil
	yamlBytes, err := yaml.Marshal(binding)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateRoleBindingYaml(namespace, name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	var binding rbacv1.RoleBinding
	if err := yaml.Unmarshal([]byte(yamlContent), &binding); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.RbacV1().RoleBindings(namespace).Update(context.TODO(), &binding, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteRoleBinding(contextName, namespace, name string) error {
	fmt.Printf("Deleting role binding: context=%s, ns=%s, name=%s\n", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	return cs.RbacV1().RoleBindings(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
}

// ClusterRoleBinding operations (cluster-scoped)
func (c *Client) ListClusterRoleBindings() ([]rbacv1.ClusterRoleBinding, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	list, err := cs.RbacV1().ClusterRoleBindings().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) GetClusterRoleBindingYaml(name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	binding, err := cs.RbacV1().ClusterRoleBindings().Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	binding.ManagedFields = nil
	yamlBytes, err := yaml.Marshal(binding)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateClusterRoleBindingYaml(name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	var binding rbacv1.ClusterRoleBinding
	if err := yaml.Unmarshal([]byte(yamlContent), &binding); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.RbacV1().ClusterRoleBindings().Update(context.TODO(), &binding, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteClusterRoleBinding(contextName, name string) error {
	fmt.Printf("Deleting cluster role binding: context=%s, name=%s\n", contextName, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	return cs.RbacV1().ClusterRoleBindings().Delete(context.TODO(), name, metav1.DeleteOptions{})
}

// NetworkPolicy operations (namespaced)
func (c *Client) ListNetworkPolicies(namespace string) ([]networkingv1.NetworkPolicy, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	list, err := cs.NetworkingV1().NetworkPolicies(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) GetNetworkPolicyYaml(namespace, name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	policy, err := cs.NetworkingV1().NetworkPolicies(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	policy.ManagedFields = nil
	yamlBytes, err := yaml.Marshal(policy)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateNetworkPolicyYaml(namespace, name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	var policy networkingv1.NetworkPolicy
	if err := yaml.Unmarshal([]byte(yamlContent), &policy); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.NetworkingV1().NetworkPolicies(namespace).Update(context.TODO(), &policy, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteNetworkPolicy(contextName, namespace, name string) error {
	fmt.Printf("Deleting network policy: context=%s, ns=%s, name=%s\n", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	return cs.NetworkingV1().NetworkPolicies(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
}

// HorizontalPodAutoscaler operations (namespaced)
func (c *Client) ListHPAs(namespace string) ([]autoscalingv2.HorizontalPodAutoscaler, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	list, err := cs.AutoscalingV2().HorizontalPodAutoscalers(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) GetHPAYaml(namespace, name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	hpa, err := cs.AutoscalingV2().HorizontalPodAutoscalers(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	hpa.ManagedFields = nil
	yamlBytes, err := yaml.Marshal(hpa)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateHPAYaml(namespace, name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	var hpa autoscalingv2.HorizontalPodAutoscaler
	if err := yaml.Unmarshal([]byte(yamlContent), &hpa); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.AutoscalingV2().HorizontalPodAutoscalers(namespace).Update(context.TODO(), &hpa, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteHPA(contextName, namespace, name string) error {
	fmt.Printf("Deleting HPA: context=%s, ns=%s, name=%s\n", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	return cs.AutoscalingV2().HorizontalPodAutoscalers(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
}

// PodDisruptionBudget operations (namespaced)
func (c *Client) ListPDBs(namespace string) ([]policyv1.PodDisruptionBudget, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	list, err := cs.PolicyV1().PodDisruptionBudgets(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) GetPDBYaml(namespace, name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	pdb, err := cs.PolicyV1().PodDisruptionBudgets(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	pdb.ManagedFields = nil
	yamlBytes, err := yaml.Marshal(pdb)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdatePDBYaml(namespace, name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	var pdb policyv1.PodDisruptionBudget
	if err := yaml.Unmarshal([]byte(yamlContent), &pdb); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.PolicyV1().PodDisruptionBudgets(namespace).Update(context.TODO(), &pdb, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeletePDB(contextName, namespace, name string) error {
	fmt.Printf("Deleting PDB: context=%s, ns=%s, name=%s\n", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	return cs.PolicyV1().PodDisruptionBudgets(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
}

// ResourceQuota operations (namespaced)
func (c *Client) ListResourceQuotas(namespace string) ([]v1.ResourceQuota, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	list, err := cs.CoreV1().ResourceQuotas(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) GetResourceQuotaYaml(namespace, name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	quota, err := cs.CoreV1().ResourceQuotas(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	quota.ManagedFields = nil
	yamlBytes, err := yaml.Marshal(quota)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateResourceQuotaYaml(namespace, name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	var quota v1.ResourceQuota
	if err := yaml.Unmarshal([]byte(yamlContent), &quota); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.CoreV1().ResourceQuotas(namespace).Update(context.TODO(), &quota, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteResourceQuota(contextName, namespace, name string) error {
	fmt.Printf("Deleting resource quota: context=%s, ns=%s, name=%s\n", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	return cs.CoreV1().ResourceQuotas(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
}

// LimitRange operations (namespaced)
func (c *Client) ListLimitRanges(namespace string) ([]v1.LimitRange, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	list, err := cs.CoreV1().LimitRanges(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) GetLimitRangeYaml(namespace, name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	lr, err := cs.CoreV1().LimitRanges(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	lr.ManagedFields = nil
	yamlBytes, err := yaml.Marshal(lr)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateLimitRangeYaml(namespace, name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	var lr v1.LimitRange
	if err := yaml.Unmarshal([]byte(yamlContent), &lr); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.CoreV1().LimitRanges(namespace).Update(context.TODO(), &lr, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteLimitRange(contextName, namespace, name string) error {
	fmt.Printf("Deleting limit range: context=%s, ns=%s, name=%s\n", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	return cs.CoreV1().LimitRanges(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
}
