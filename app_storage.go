package main

import (
	"fmt"

	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"

	"kubikles/pkg/debug"
	"kubikles/pkg/k8s"
)

// =============================================================================
// Storage
// =============================================================================

func (a *App) ListPVCs(requestId, namespace string) ([]v1.PersistentVolumeClaim, error) {
	currentContext := a.GetCurrentContext()
	debug.LogK8s("ListPVCs called", map[string]interface{}{"context": currentContext, "ns": namespace})
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	if requestId != "" {
		ctx, seq := a.listRequestManager.StartRequest(requestId)
		defer a.listRequestManager.CompleteRequest(requestId, seq)

		result, err := a.k8sClient.ListPVCsWithContext(ctx, currentContext, namespace, a.listProgressCallback("pvcs"))
		if err == k8s.ErrRequestCancelled {
			return nil, nil
		}
		return result, err
	}
	return a.k8sClient.ListPVCs(currentContext, namespace)
}

func (a *App) GetPVCYaml(namespace, name string) (string, error) {
	debug.LogK8s("GetPVCYaml called", map[string]interface{}{"ns": namespace, "name": name})
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetPVCYaml(namespace, name)
}

func (a *App) UpdatePVCYaml(namespace, name, yamlContent string) error {
	debug.LogK8s("UpdatePVCYaml called", map[string]interface{}{"ns": namespace, "name": name})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdatePVCYaml(namespace, name, yamlContent)
}

func (a *App) DeletePVC(namespace, name string) error {
	currentContext := a.GetCurrentContext()
	debug.LogK8s("DeletePVC called", map[string]interface{}{"context": currentContext, "ns": namespace, "name": name})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeletePVC(currentContext, namespace, name)
}

func (a *App) ResizePVC(namespace, name, newSize string) error {
	currentContext := a.GetCurrentContext()
	debug.LogK8s("ResizePVC called", map[string]interface{}{"context": currentContext, "ns": namespace, "name": name, "newSize": newSize})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ResizePVC(currentContext, namespace, name, newSize)
}

// PersistentVolume operations (cluster-scoped)
func (a *App) ListPVs(requestId string) ([]v1.PersistentVolume, error) {
	currentContext := a.GetCurrentContext()
	debug.LogK8s("ListPVs called", map[string]interface{}{"context": currentContext})
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	if requestId != "" {
		ctx, seq := a.listRequestManager.StartRequest(requestId)
		defer a.listRequestManager.CompleteRequest(requestId, seq)

		result, err := a.k8sClient.ListPVsWithContext(ctx, currentContext, a.listProgressCallback("pvs"))
		if err == k8s.ErrRequestCancelled {
			return nil, nil
		}
		return result, err
	}
	return a.k8sClient.ListPVs(currentContext)
}

func (a *App) GetPVYaml(name string) (string, error) {
	debug.LogK8s("GetPVYaml called", map[string]interface{}{"name": name})
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetPVYaml(name)
}

func (a *App) UpdatePVYaml(name, yamlContent string) error {
	debug.LogK8s("UpdatePVYaml called", map[string]interface{}{"name": name})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdatePVYaml(name, yamlContent)
}

func (a *App) DeletePV(name string) error {
	currentContext := a.GetCurrentContext()
	debug.LogK8s("DeletePV called", map[string]interface{}{"context": currentContext, "name": name})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeletePV(currentContext, name)
}

// StorageClass operations (cluster-scoped)
func (a *App) ListStorageClasses(requestId string) ([]storagev1.StorageClass, error) {
	currentContext := a.GetCurrentContext()
	debug.LogK8s("ListStorageClasses called", map[string]interface{}{"context": currentContext})
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	if requestId != "" {
		ctx, seq := a.listRequestManager.StartRequest(requestId)
		defer a.listRequestManager.CompleteRequest(requestId, seq)

		result, err := a.k8sClient.ListStorageClassesWithContext(ctx, currentContext, a.listProgressCallback("storageclasses"))
		if err == k8s.ErrRequestCancelled {
			return nil, nil
		}
		return result, err
	}
	return a.k8sClient.ListStorageClasses(currentContext)
}

func (a *App) GetStorageClass(name string) (*storagev1.StorageClass, error) {
	currentContext := a.GetCurrentContext()
	debug.LogK8s("GetStorageClass called", map[string]interface{}{"context": currentContext, "name": name})
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetStorageClass(currentContext, name)
}

func (a *App) GetStorageClassYaml(name string) (string, error) {
	debug.LogK8s("GetStorageClassYaml called", map[string]interface{}{"name": name})
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetStorageClassYaml(name)
}

func (a *App) UpdateStorageClassYaml(name, yamlContent string) error {
	debug.LogK8s("UpdateStorageClassYaml called", map[string]interface{}{"name": name})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateStorageClassYaml(name, yamlContent)
}

func (a *App) DeleteStorageClass(name string) error {
	currentContext := a.GetCurrentContext()
	debug.LogK8s("DeleteStorageClass called", map[string]interface{}{"context": currentContext, "name": name})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteStorageClass(currentContext, name)
}

// GetResourceDependencies returns the dependency graph for a given resource
func (a *App) GetResourceDependencies(resourceType, namespace, name string) (*k8s.DependencyGraph, error) {
	currentContext := a.GetCurrentContext()
	debug.LogK8s("GetResourceDependencies called", map[string]interface{}{"context": currentContext, "type": resourceType, "ns": namespace, "name": name})
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetResourceDependencies(currentContext, resourceType, namespace, name)
}

// ExpandDependencyNode returns additional nodes when a summary node is expanded
func (a *App) ExpandDependencyNode(resourceType, namespace, name, summaryNodeID string, offset int) (*k8s.DependencyGraph, error) {
	currentContext := a.GetCurrentContext()
	debug.LogK8s("ExpandDependencyNode called", map[string]interface{}{"context": currentContext, "type": resourceType, "ns": namespace, "name": name, "summaryID": summaryNodeID, "offset": offset})
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ExpandDependencyNode(currentContext, resourceType, namespace, name, summaryNodeID, offset)
}
