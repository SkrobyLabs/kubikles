package main

import (
	"fmt"

	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"

	"kubikles/pkg/k8s"
)

// =============================================================================
// Storage
// =============================================================================

func (a *App) ListPVCs(namespace string) ([]v1.PersistentVolumeClaim, error) {
	currentContext := a.GetCurrentContext()
	a.logDebug("ListPVCs called: context=%s, ns=%s", currentContext, namespace)
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListPVCs(currentContext, namespace)
}

func (a *App) GetPVCYaml(namespace, name string) (string, error) {
	a.logDebug("GetPVCYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetPVCYaml(namespace, name)
}

func (a *App) UpdatePVCYaml(namespace, name, yamlContent string) error {
	a.logDebug("UpdatePVCYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdatePVCYaml(namespace, name, yamlContent)
}

func (a *App) DeletePVC(namespace, name string) error {
	currentContext := a.GetCurrentContext()
	a.logDebug("DeletePVC called: context=%s, ns=%s, name=%s", currentContext, namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeletePVC(currentContext, namespace, name)
}

func (a *App) ResizePVC(namespace, name, newSize string) error {
	currentContext := a.GetCurrentContext()
	a.logDebug("ResizePVC called: context=%s, ns=%s, name=%s, newSize=%s", currentContext, namespace, name, newSize)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ResizePVC(currentContext, namespace, name, newSize)
}

// PersistentVolume operations (cluster-scoped)
func (a *App) ListPVs() ([]v1.PersistentVolume, error) {
	currentContext := a.GetCurrentContext()
	a.logDebug("ListPVs called: context=%s", currentContext)
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListPVs(currentContext)
}

func (a *App) GetPVYaml(name string) (string, error) {
	a.logDebug("GetPVYaml called: name=%s", name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetPVYaml(name)
}

func (a *App) UpdatePVYaml(name, yamlContent string) error {
	a.logDebug("UpdatePVYaml called: name=%s", name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdatePVYaml(name, yamlContent)
}

func (a *App) DeletePV(name string) error {
	currentContext := a.GetCurrentContext()
	a.logDebug("DeletePV called: context=%s, name=%s", currentContext, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeletePV(currentContext, name)
}

// StorageClass operations (cluster-scoped)
func (a *App) ListStorageClasses() ([]storagev1.StorageClass, error) {
	currentContext := a.GetCurrentContext()
	a.logDebug("ListStorageClasses called: context=%s", currentContext)
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListStorageClasses(currentContext)
}

func (a *App) GetStorageClass(name string) (*storagev1.StorageClass, error) {
	currentContext := a.GetCurrentContext()
	a.logDebug("GetStorageClass called: context=%s, name=%s", currentContext, name)
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetStorageClass(currentContext, name)
}

func (a *App) GetStorageClassYaml(name string) (string, error) {
	a.logDebug("GetStorageClassYaml called: name=%s", name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetStorageClassYaml(name)
}

func (a *App) UpdateStorageClassYaml(name, yamlContent string) error {
	a.logDebug("UpdateStorageClassYaml called: name=%s", name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateStorageClassYaml(name, yamlContent)
}

func (a *App) DeleteStorageClass(name string) error {
	currentContext := a.GetCurrentContext()
	a.logDebug("DeleteStorageClass called: context=%s, name=%s", currentContext, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteStorageClass(currentContext, name)
}

// GetResourceDependencies returns the dependency graph for a given resource
func (a *App) GetResourceDependencies(resourceType, namespace, name string) (*k8s.DependencyGraph, error) {
	currentContext := a.GetCurrentContext()
	a.logDebug("GetResourceDependencies called: context=%s, type=%s, ns=%s, name=%s", currentContext, resourceType, namespace, name)
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetResourceDependencies(currentContext, resourceType, namespace, name)
}

// ExpandDependencyNode returns additional nodes when a summary node is expanded
func (a *App) ExpandDependencyNode(resourceType, namespace, name, summaryNodeID string, offset int) (*k8s.DependencyGraph, error) {
	currentContext := a.GetCurrentContext()
	a.logDebug("ExpandDependencyNode called: context=%s, type=%s, ns=%s, name=%s, summaryID=%s, offset=%d",
		currentContext, resourceType, namespace, name, summaryNodeID, offset)
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ExpandDependencyNode(currentContext, resourceType, namespace, name, summaryNodeID, offset)
}
