package main

import (
	"kubikles/pkg/k8s"

	"fmt"

	appsv1 "k8s.io/api/apps/v1"
)

// =============================================================================
// ReplicaSets
// =============================================================================

func (a *App) ListReplicaSets(requestId, namespace string) ([]appsv1.ReplicaSet, error) {
	currentContext := a.GetCurrentContext()
	a.logDebug("ListReplicaSets called: context=%s, ns=%s", currentContext, namespace)
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	if requestId != "" {
		ctx, seq := a.listRequestManager.StartRequest(requestId)
		defer a.listRequestManager.CompleteRequest(requestId, seq)

		result, err := a.k8sClient.ListReplicaSetsWithContext(ctx, currentContext, namespace)
		if err == k8s.ErrRequestCancelled {
			return nil, nil
		}
		return result, err
	}
	return a.k8sClient.ListReplicaSets(currentContext, namespace)
}

func (a *App) GetReplicaSetYaml(namespace, name string) (string, error) {
	a.logDebug("GetReplicaSetYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetReplicaSetYaml(namespace, name)
}

func (a *App) UpdateReplicaSetYaml(namespace, name, yamlContent string) error {
	a.logDebug("UpdateReplicaSetYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateReplicaSetYaml(namespace, name, yamlContent)
}

func (a *App) DeleteReplicaSet(namespace, name string) error {
	currentContext := a.GetCurrentContext()
	a.logDebug("DeleteReplicaSet called: context=%s, ns=%s, name=%s", currentContext, namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteReplicaSet(currentContext, namespace, name)
}

func (a *App) RestartStatefulSet(namespace, name string) error {
	contextName := a.GetCurrentContext()
	a.logDebug("RestartStatefulSet called: context=%s, ns=%s, name=%s", contextName, namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	err := a.k8sClient.RestartStatefulSet(contextName, namespace, name)
	if err != nil {
		a.logDebug("RestartStatefulSet error: %v", err)
	} else {
		a.logDebug("RestartStatefulSet success")
	}
	return err
}

func (a *App) DeleteStatefulSet(namespace, name string) error {
	contextName := a.GetCurrentContext()
	a.logDebug("DeleteStatefulSet called: context=%s, ns=%s, name=%s", contextName, namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	err := a.k8sClient.DeleteStatefulSet(contextName, namespace, name)
	if err != nil {
		a.logDebug("DeleteStatefulSet error: %v", err)
	} else {
		a.logDebug("DeleteStatefulSet success")
	}
	return err
}
