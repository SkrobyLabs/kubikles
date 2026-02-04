package main

import (
	"fmt"

	v1 "k8s.io/api/core/v1"

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

		result, err := a.k8sClient.ListPodsWithContext(ctx, namespace)
		if err == k8s.ErrRequestCancelled {
			return nil, nil // Return empty for canceled requests
		}
		return result, err
	}
	return a.k8sClient.ListPods(namespace)
}

// Nodes: see app_nodes.go
