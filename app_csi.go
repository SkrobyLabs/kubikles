package main

import (
	"fmt"

	storagev1 "k8s.io/api/storage/v1"
)

// =============================================================================
// CSI
// =============================================================================

func (a *App) ListCSIDrivers() ([]storagev1.CSIDriver, error) {
	currentContext := a.GetCurrentContext()
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListCSIDrivers(currentContext)
}

func (a *App) GetCSIDriverYaml(name string) (string, error) {
	currentContext := a.GetCurrentContext()
	a.logDebug("GetCSIDriverYaml called: context=%s, name=%s", currentContext, name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetCSIDriverYaml(currentContext, name)
}

func (a *App) UpdateCSIDriverYaml(name, yamlContent string) error {
	currentContext := a.GetCurrentContext()
	a.logDebug("UpdateCSIDriverYaml called: context=%s, name=%s", currentContext, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateCSIDriverYaml(currentContext, name, yamlContent)
}

func (a *App) DeleteCSIDriver(name string) error {
	currentContext := a.GetCurrentContext()
	a.logDebug("DeleteCSIDriver called: context=%s, name=%s", currentContext, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteCSIDriver(currentContext, name)
}

// CSINode operations (cluster-scoped)
func (a *App) ListCSINodes() ([]storagev1.CSINode, error) {
	currentContext := a.GetCurrentContext()
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListCSINodes(currentContext)
}

func (a *App) GetCSINodeYaml(name string) (string, error) {
	currentContext := a.GetCurrentContext()
	a.logDebug("GetCSINodeYaml called: context=%s, name=%s", currentContext, name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetCSINodeYaml(currentContext, name)
}

func (a *App) UpdateCSINodeYaml(name, yamlContent string) error {
	currentContext := a.GetCurrentContext()
	a.logDebug("UpdateCSINodeYaml called: context=%s, name=%s", currentContext, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateCSINodeYaml(currentContext, name, yamlContent)
}

func (a *App) DeleteCSINode(name string) error {
	currentContext := a.GetCurrentContext()
	a.logDebug("DeleteCSINode called: context=%s, name=%s", currentContext, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteCSINode(currentContext, name)
}

// ApplyYAML creates a resource from YAML content
func (a *App) ApplyYAML(yamlContent string) error {
	currentContext := a.GetCurrentContext()
	a.logDebug("ApplyYAML called: context=%s", currentContext)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ApplyYAML(currentContext, yamlContent)
}
