package main

import (
	"kubikles/pkg/k8s"

	"fmt"

	appsv1 "k8s.io/api/apps/v1"
)

// =============================================================================
// StatefulSets
// =============================================================================

func (a *App) ListStatefulSets(requestId, namespace string) ([]appsv1.StatefulSet, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	if requestId != "" {
		ctx, seq := a.listRequestManager.StartRequest(requestId)
		defer a.listRequestManager.CompleteRequest(requestId, seq)

		result, err := a.k8sClient.ListStatefulSetsWithContext(ctx, "", namespace)
		if err == k8s.ErrRequestCancelled {
			return nil, nil
		}
		return result, err
	}
	return a.k8sClient.ListStatefulSets("", namespace)
}

func (a *App) GetStatefulSetYaml(namespace, name string) (string, error) {
	a.logDebug("GetStatefulSetYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetStatefulSetYaml(namespace, name)
}

func (a *App) UpdateStatefulSetYaml(namespace, name, yamlContent string) error {
	a.logDebug("UpdateStatefulSetYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateStatefulSetYaml(namespace, name, yamlContent)
}

func (a *App) ScaleStatefulSet(namespace, name string, replicas int32) error {
	a.logDebug("ScaleStatefulSet called: ns=%s, name=%s, replicas=%d", namespace, name, replicas)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ScaleStatefulSet(namespace, name, replicas)
}
