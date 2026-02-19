package main

import (
	"fmt"

	"kubikles/pkg/debug"
	"kubikles/pkg/k8s"

	appsv1 "k8s.io/api/apps/v1"
)

// =============================================================================
// ReplicaSets
// =============================================================================

func (a *App) ListReplicaSets(requestId, namespace string) ([]appsv1.ReplicaSet, error) {
	currentContext := a.GetCurrentContext()
	debug.LogK8s("ListReplicaSets called", map[string]interface{}{"context": currentContext, "ns": namespace})
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	if requestId != "" {
		ctx, seq := a.listRequestManager.StartRequest(requestId)
		defer a.listRequestManager.CompleteRequest(requestId, seq)

		result, err := a.k8sClient.ListReplicaSetsWithContext(ctx, currentContext, namespace, a.listProgressCallback("replicasets"))
		if err == k8s.ErrRequestCancelled {
			return nil, nil
		}
		return result, err
	}
	return a.k8sClient.ListReplicaSets(currentContext, namespace)
}

func (a *App) GetReplicaSetYaml(namespace, name string) (string, error) {
	debug.LogK8s("GetReplicaSetYaml called", map[string]interface{}{"ns": namespace, "name": name})
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetReplicaSetYaml(namespace, name)
}

func (a *App) UpdateReplicaSetYaml(namespace, name, yamlContent string) error {
	debug.LogK8s("UpdateReplicaSetYaml called", map[string]interface{}{"ns": namespace, "name": name})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateReplicaSetYaml(namespace, name, yamlContent)
}

func (a *App) ScaleReplicaSet(namespace, name string, replicas int32) error {
	debug.LogK8s("ScaleReplicaSet called", map[string]interface{}{"ns": namespace, "name": name, "replicas": replicas})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ScaleReplicaSet(namespace, name, replicas)
}

func (a *App) DeleteReplicaSet(namespace, name string) error {
	currentContext := a.GetCurrentContext()
	debug.LogK8s("DeleteReplicaSet called", map[string]interface{}{"context": currentContext, "ns": namespace, "name": name})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteReplicaSet(currentContext, namespace, name)
}

func (a *App) RestartStatefulSet(namespace, name string) error {
	contextName := a.GetCurrentContext()
	debug.LogK8s("RestartStatefulSet called", map[string]interface{}{"context": contextName, "ns": namespace, "name": name})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	err := a.k8sClient.RestartStatefulSet(contextName, namespace, name)
	if err != nil {
		debug.LogK8s("RestartStatefulSet error", map[string]interface{}{"error": err.Error()})
	} else {
		debug.LogK8s("RestartStatefulSet success", nil)
	}
	return err
}

func (a *App) DeleteStatefulSet(namespace, name string) error {
	contextName := a.GetCurrentContext()
	debug.LogK8s("DeleteStatefulSet called", map[string]interface{}{"context": contextName, "ns": namespace, "name": name})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	err := a.k8sClient.DeleteStatefulSet(contextName, namespace, name)
	if err != nil {
		debug.LogK8s("DeleteStatefulSet error", map[string]interface{}{"error": err.Error()})
	} else {
		debug.LogK8s("DeleteStatefulSet success", nil)
	}
	return err
}
