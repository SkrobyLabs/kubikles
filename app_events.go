package main

import (
	"fmt"

	v1 "k8s.io/api/core/v1"

	"kubikles/pkg/debug"
	"kubikles/pkg/k8s"
)

// =============================================================================
// Events
// =============================================================================

func (a *App) ListEvents(requestId, namespace string) ([]v1.Event, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	if requestId != "" {
		ctx, seq := a.listRequestManager.StartRequest(requestId)
		defer a.listRequestManager.CompleteRequest(requestId, seq)

		result, err := a.k8sClient.ListEventsWithContext(ctx, namespace)
		if err == k8s.ErrRequestCancelled {
			return nil, nil
		}
		return result, err
	}
	return a.k8sClient.ListEvents(namespace)
}

func (a *App) GetEventYAML(namespace, name string) (string, error) {
	debug.LogK8s("GetEventYAML called", map[string]interface{}{"namespace": namespace, "name": name})
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetEventYAML(namespace, name)
}

func (a *App) UpdateEventYAML(namespace, name string, yamlContent string) error {
	debug.LogK8s("UpdateEventYAML called", map[string]interface{}{"namespace": namespace, "name": name})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateEventYAML(namespace, name, yamlContent)
}

// GetMetricsEventMarkers returns lifecycle event markers for overlaying on metrics charts
func (a *App) GetMetricsEventMarkers(namespace, name, kind, duration string) ([]k8s.LifecycleMarker, error) {
	if a.k8sClient == nil {
		return []k8s.LifecycleMarker{}, nil
	}
	dur := parseMetricsDuration(duration)
	return a.k8sClient.GetLifecycleMarkers(namespace, name, kind, dur)
}

func (a *App) DeleteEvent(namespace, name string) error {
	contextName := a.GetCurrentContext()
	debug.LogK8s("DeleteEvent called", map[string]interface{}{"context": contextName, "namespace": namespace, "name": name})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteEvent(contextName, namespace, name)
}
