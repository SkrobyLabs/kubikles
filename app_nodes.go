package main

import (
	"fmt"

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
	a.logDebug("GetNodeMetricsFromPrometheus called: context=%s, prometheus=%s/%s:%d", currentContext, prometheusNamespace, prometheusService, prometheusPort)
	if a.k8sClient == nil {
		a.logDebug("GetNodeMetricsFromPrometheus: k8s client not initialized")
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
		a.logDebug("GetNodeMetricsFromPrometheus error: %v", err)
	} else {
		a.logDebug("GetNodeMetricsFromPrometheus result: available=%v, metrics_count=%d, error=%s", result.Available, len(result.Metrics), result.Error)
	}
	return result, err
}

// GetPodMetricsFromPrometheus fetches pod metrics from Prometheus (fallback when metrics-server unavailable)
func (a *App) GetPodMetricsFromPrometheus(prometheusNamespace, prometheusService string, prometheusPort int) (*k8s.PodMetricsResult, error) {
	currentContext := a.GetCurrentContext()
	a.logDebug("GetPodMetricsFromPrometheus called: context=%s, prometheus=%s/%s:%d", currentContext, prometheusNamespace, prometheusService, prometheusPort)
	if a.k8sClient == nil {
		a.logDebug("GetPodMetricsFromPrometheus: k8s client not initialized")
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
		a.logDebug("GetPodMetricsFromPrometheus error: %v", err)
	} else {
		a.logDebug("GetPodMetricsFromPrometheus result: available=%v, metrics_count=%d, error=%s", result.Available, len(result.Metrics), result.Error)
	}
	return result, err
}

func (a *App) GetNodeYaml(name string) (string, error) {
	a.logDebug("GetNodeYaml called: name=%s", name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetNodeYaml(name)
}

func (a *App) UpdateNodeYaml(name, yamlContent string) error {
	a.logDebug("UpdateNodeYaml called: name=%s", name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateNodeYaml(name, yamlContent)
}

func (a *App) DeleteNode(name string) error {
	currentContext := a.GetCurrentContext()
	a.logDebug("DeleteNode called: context=%s, name=%s", currentContext, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteNode(currentContext, name)
}

func (a *App) SetNodeSchedulable(name string, schedulable bool) error {
	currentContext := a.GetCurrentContext()
	a.logDebug("SetNodeSchedulable called: context=%s, name=%s, schedulable=%v", currentContext, name, schedulable)
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
	a.logDebug("CreateNodeDebugPod called: context=%s, nodeName=%s, image=%s", currentContext, nodeName, image)
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
