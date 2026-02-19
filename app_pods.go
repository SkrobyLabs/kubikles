package main

import (
	"fmt"

	v1 "k8s.io/api/core/v1"

	pkgdebug "kubikles/pkg/debug"
	"kubikles/pkg/k8s"
)

// =============================================================================
// Pods
// =============================================================================

func (a *App) ListPods(requestId, namespace string) ([]v1.Pod, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	if requestId != "" {
		ctx, seq := a.listRequestManager.StartRequest(requestId)
		defer a.listRequestManager.CompleteRequest(requestId, seq)

		result, err := a.k8sClient.ListPodsWithContext(ctx, namespace, a.listProgressCallback("pods"))
		if err == k8s.ErrRequestCancelled {
			return nil, nil // Return empty for canceled requests
		}
		return result, err
	}
	return a.k8sClient.ListPods(namespace)
}

func (a *App) ListPodsForNode(nodeName string) ([]v1.Pod, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListPodsForNode(nodeName)
}

func (a *App) GetPodEvictionInfo(namespace, name string) (*k8s.PodEvictionInfo, error) {
	contextName := a.GetCurrentContext()
	pkgdebug.LogK8s("GetPodEvictionInfo called", map[string]interface{}{"context": contextName, "namespace": namespace, "name": name})
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	result, err := a.k8sClient.GetPodEvictionInfo(contextName, namespace, name)
	if err != nil {
		pkgdebug.LogK8s("GetPodEvictionInfo error", map[string]interface{}{"error": err.Error()})
	} else {
		pkgdebug.LogK8s("GetPodEvictionInfo success", map[string]interface{}{"category": result.Category})
	}
	return result, err
}

func (a *App) ResolveTopLevelOwner(namespace, kind, name string) (*k8s.TopLevelOwner, error) {
	contextName := a.GetCurrentContext()
	pkgdebug.LogK8s("ResolveTopLevelOwner called", map[string]interface{}{"context": contextName, "namespace": namespace, "kind": kind, "name": name})
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	result, err := a.k8sClient.ResolveTopLevelOwner(contextName, namespace, kind, name)
	if err != nil {
		pkgdebug.LogK8s("ResolveTopLevelOwner error", map[string]interface{}{"error": err.Error()})
	} else {
		pkgdebug.LogK8s("ResolveTopLevelOwner success", map[string]interface{}{"kind": result.Kind, "name": result.Name})
	}
	return result, err
}

func (a *App) EvictPod(namespace, name string) error {
	contextName := a.GetCurrentContext()
	pkgdebug.LogK8s("EvictPod called", map[string]interface{}{"context": contextName, "namespace": namespace, "name": name})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	err := a.k8sClient.EvictPod(contextName, namespace, name)
	if err != nil {
		pkgdebug.LogK8s("EvictPod error", map[string]interface{}{"error": err.Error()})
	} else {
		pkgdebug.LogK8s("EvictPod success", nil)
	}
	return err
}

// Nodes: see app_nodes.go
