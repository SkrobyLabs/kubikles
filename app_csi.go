package main

import (
	"fmt"

	storagev1 "k8s.io/api/storage/v1"

	"kubikles/pkg/debug"
	"kubikles/pkg/k8s"
)

// =============================================================================
// CSI
// =============================================================================

func (a *App) ListCSIDrivers(requestId string) ([]storagev1.CSIDriver, error) {
	currentContext := a.GetCurrentContext()
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	if requestId != "" {
		ctx, seq := a.listRequestManager.StartRequest(requestId)
		defer a.listRequestManager.CompleteRequest(requestId, seq)

		result, err := a.k8sClient.ListCSIDriversWithContext(ctx, currentContext)
		if err == k8s.ErrRequestCancelled {
			return nil, nil
		}
		return result, err
	}
	return a.k8sClient.ListCSIDrivers(currentContext)
}

func (a *App) GetCSIDriverYaml(name string) (string, error) {
	currentContext := a.GetCurrentContext()
	debug.LogK8s("GetCSIDriverYaml called", map[string]interface{}{"context": currentContext, "name": name})
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetCSIDriverYaml(currentContext, name)
}

func (a *App) UpdateCSIDriverYaml(name, yamlContent string) error {
	currentContext := a.GetCurrentContext()
	debug.LogK8s("UpdateCSIDriverYaml called", map[string]interface{}{"context": currentContext, "name": name})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateCSIDriverYaml(currentContext, name, yamlContent)
}

func (a *App) DeleteCSIDriver(name string) error {
	currentContext := a.GetCurrentContext()
	debug.LogK8s("DeleteCSIDriver called", map[string]interface{}{"context": currentContext, "name": name})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteCSIDriver(currentContext, name)
}

// CSINode operations (cluster-scoped)
func (a *App) ListCSINodes(requestId string) ([]storagev1.CSINode, error) {
	currentContext := a.GetCurrentContext()
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	if requestId != "" {
		ctx, seq := a.listRequestManager.StartRequest(requestId)
		defer a.listRequestManager.CompleteRequest(requestId, seq)

		result, err := a.k8sClient.ListCSINodesWithContext(ctx, currentContext)
		if err == k8s.ErrRequestCancelled {
			return nil, nil
		}
		return result, err
	}
	return a.k8sClient.ListCSINodes(currentContext)
}

func (a *App) GetCSINodeYaml(name string) (string, error) {
	currentContext := a.GetCurrentContext()
	debug.LogK8s("GetCSINodeYaml called", map[string]interface{}{"context": currentContext, "name": name})
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetCSINodeYaml(currentContext, name)
}

func (a *App) UpdateCSINodeYaml(name, yamlContent string) error {
	currentContext := a.GetCurrentContext()
	debug.LogK8s("UpdateCSINodeYaml called", map[string]interface{}{"context": currentContext, "name": name})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateCSINodeYaml(currentContext, name, yamlContent)
}

func (a *App) DeleteCSINode(name string) error {
	currentContext := a.GetCurrentContext()
	debug.LogK8s("DeleteCSINode called", map[string]interface{}{"context": currentContext, "name": name})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteCSINode(currentContext, name)
}

// ApplyYAML creates a resource from YAML content
func (a *App) ApplyYAML(yamlContent string) error {
	currentContext := a.GetCurrentContext()
	debug.LogK8s("ApplyYAML called", map[string]interface{}{"context": currentContext})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ApplyYAML(currentContext, yamlContent)
}
