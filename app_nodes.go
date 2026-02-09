package main

import (
	"fmt"

	"kubikles/pkg/debug"
	"kubikles/pkg/k8s"

	v1 "k8s.io/api/core/v1"
)

// =============================================================================
// Nodes
// =============================================================================

func (a *App) ListNodes(requestId string) ([]v1.Node, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	if requestId != "" {
		ctx, seq := a.listRequestManager.StartRequest(requestId)
		defer a.listRequestManager.CompleteRequest(requestId, seq)

		result, err := a.k8sClient.ListNodesWithContext(ctx)
		if err == k8s.ErrRequestCancelled {
			return nil, nil
		}
		return result, err
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

// GetNodeMetricsFromPrometheus fetches node metrics from Prometheus (fallback when metrics-server unavailable)
func (a *App) GetNodeMetricsFromPrometheus(prometheusNamespace, prometheusService string, prometheusPort int) (*k8s.NodeMetricsResult, error) {
	currentContext := a.GetCurrentContext()
	debug.LogK8s("GetNodeMetricsFromPrometheus called", map[string]interface{}{"context": currentContext, "prometheusNs": prometheusNamespace, "prometheusService": prometheusService, "prometheusPort": prometheusPort})
	if a.k8sClient == nil {
		debug.LogK8s("GetNodeMetricsFromPrometheus: k8s client not initialized", nil)
		return &k8s.NodeMetricsResult{Available: false}, nil
	}

	info := &k8s.PrometheusInfo{
		Available: true,
		Namespace: prometheusNamespace,
		Service:   prometheusService,
		Port:      prometheusPort,
	}

	result, err := a.k8sClient.GetNodeMetricsFromPrometheus(currentContext, info)
	if err != nil {
		debug.LogK8s("GetNodeMetricsFromPrometheus error", map[string]interface{}{"error": err.Error()})
	} else {
		debug.LogK8s("GetNodeMetricsFromPrometheus result", map[string]interface{}{"available": result.Available, "metrics_count": len(result.Metrics), "error": result.Error})
	}
	return result, err
}

// GetPodMetricsFromPrometheus fetches pod metrics from Prometheus (fallback when metrics-server unavailable)
func (a *App) GetPodMetricsFromPrometheus(prometheusNamespace, prometheusService string, prometheusPort int) (*k8s.PodMetricsResult, error) {
	currentContext := a.GetCurrentContext()
	debug.LogK8s("GetPodMetricsFromPrometheus called", map[string]interface{}{"context": currentContext, "prometheusNs": prometheusNamespace, "prometheusService": prometheusService, "prometheusPort": prometheusPort})
	if a.k8sClient == nil {
		debug.LogK8s("GetPodMetricsFromPrometheus: k8s client not initialized", nil)
		return &k8s.PodMetricsResult{Available: false}, nil
	}

	info := &k8s.PrometheusInfo{
		Available: true,
		Namespace: prometheusNamespace,
		Service:   prometheusService,
		Port:      prometheusPort,
	}

	result, err := a.k8sClient.GetPodMetricsFromPrometheus(currentContext, info)
	if err != nil {
		debug.LogK8s("GetPodMetricsFromPrometheus error", map[string]interface{}{"error": err.Error()})
	} else {
		debug.LogK8s("GetPodMetricsFromPrometheus result", map[string]interface{}{"available": result.Available, "metrics_count": len(result.Metrics), "error": result.Error})
	}
	return result, err
}

func (a *App) GetNodeYaml(name string) (string, error) {
	debug.LogK8s("GetNodeYaml called", map[string]interface{}{"name": name})
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetNodeYaml(name)
}

func (a *App) UpdateNodeYaml(name, yamlContent string) error {
	debug.LogK8s("UpdateNodeYaml called", map[string]interface{}{"name": name})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateNodeYaml(name, yamlContent)
}

func (a *App) DeleteNode(name string) error {
	currentContext := a.GetCurrentContext()
	debug.LogK8s("DeleteNode called", map[string]interface{}{"context": currentContext, "name": name})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteNode(currentContext, name)
}

func (a *App) SetNodeSchedulable(name string, schedulable bool) error {
	currentContext := a.GetCurrentContext()
	debug.LogK8s("SetNodeSchedulable called", map[string]interface{}{"context": currentContext, "name": name, "schedulable": schedulable})
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

func (a *App) CreateNodeDebugPod(nodeName, image string) (*NodeDebugPodResult, error) {
	currentContext := a.GetCurrentContext()
	debug.LogK8s("CreateNodeDebugPod called", map[string]interface{}{"context": currentContext, "nodeName": nodeName, "image": image})
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	pod, err := a.k8sClient.CreateNodeDebugPod(currentContext, nodeName, image)
	if err != nil {
		return nil, err
	}
	return &NodeDebugPodResult{
		PodName:   pod.Name,
		Namespace: pod.Namespace,
	}, nil
}
