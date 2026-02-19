package main

import (
	"kubikles/pkg/debug"
	"kubikles/pkg/k8s"

	"fmt"

	appsv1 "k8s.io/api/apps/v1"
)

// =============================================================================
// DaemonSets
// =============================================================================

func (a *App) ListDaemonSets(requestId, namespace string) ([]appsv1.DaemonSet, error) {
	currentContext := a.GetCurrentContext()
	debug.LogK8s("ListDaemonSets called", map[string]interface{}{"context": currentContext, "ns": namespace})
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	if requestId != "" {
		ctx, seq := a.listRequestManager.StartRequest(requestId)
		defer a.listRequestManager.CompleteRequest(requestId, seq)

		result, err := a.k8sClient.ListDaemonSetsWithContext(ctx, currentContext, namespace, a.listProgressCallback("daemonsets"))
		if err == k8s.ErrRequestCancelled {
			return nil, nil
		}
		return result, err
	}
	return a.k8sClient.ListDaemonSets(currentContext, namespace)
}

func (a *App) GetDaemonSetYaml(namespace, name string) (string, error) {
	debug.LogK8s("GetDaemonSetYaml called", map[string]interface{}{"ns": namespace, "name": name})
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetDaemonSetYaml(namespace, name)
}

func (a *App) UpdateDaemonSetYaml(namespace, name, yamlContent string) error {
	debug.LogK8s("UpdateDaemonSetYaml called", map[string]interface{}{"ns": namespace, "name": name})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateDaemonSetYaml(namespace, name, yamlContent)
}

func (a *App) RestartDaemonSet(namespace, name string) error {
	currentContext := a.GetCurrentContext()
	debug.LogK8s("RestartDaemonSet called", map[string]interface{}{"context": currentContext, "ns": namespace, "name": name})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.RestartDaemonSet(currentContext, namespace, name)
}

func (a *App) DeleteDaemonSet(namespace, name string) error {
	currentContext := a.GetCurrentContext()
	debug.LogK8s("DeleteDaemonSet called", map[string]interface{}{"context": currentContext, "ns": namespace, "name": name})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteDaemonSet(currentContext, namespace, name)
}
