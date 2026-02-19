package main

import (
	"fmt"

	"kubikles/pkg/debug"
	"kubikles/pkg/k8s"

	v1 "k8s.io/api/core/v1"
)

// =============================================================================
// Services
// =============================================================================

func (a *App) ListServices(requestId, namespace string) ([]v1.Service, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	if requestId != "" {
		ctx, seq := a.listRequestManager.StartRequest(requestId)
		defer a.listRequestManager.CompleteRequest(requestId, seq)

		result, err := a.k8sClient.ListServicesWithContext(ctx, namespace, a.listProgressCallback("services"))
		if err == k8s.ErrRequestCancelled {
			return nil, nil
		}
		return result, err
	}
	return a.k8sClient.ListServices(namespace)
}

func (a *App) GetServiceYaml(namespace, name string) (string, error) {
	debug.LogK8s("GetServiceYaml called", map[string]interface{}{"ns": namespace, "name": name})
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetServiceYaml(namespace, name)
}

func (a *App) UpdateServiceYaml(namespace, name, yamlContent string) error {
	debug.LogK8s("UpdateServiceYaml called", map[string]interface{}{"ns": namespace, "name": name})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateServiceYaml(namespace, name, yamlContent)
}

func (a *App) DeleteService(namespace, name string) error {
	currentContext := a.GetCurrentContext()
	debug.LogK8s("DeleteService called", map[string]interface{}{"context": currentContext, "ns": namespace, "name": name})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteService(currentContext, namespace, name)
}
