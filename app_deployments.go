package main

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"

	"kubikles/pkg/k8s"
)

// =============================================================================
// Deployments
// =============================================================================

func (a *App) ListDeployments(requestId, namespace string) ([]appsv1.Deployment, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	if requestId != "" {
		ctx, seq := a.listRequestManager.StartRequest(requestId)
		defer a.listRequestManager.CompleteRequest(requestId, seq)

		result, err := a.k8sClient.ListDeploymentsWithContext(ctx, namespace)
		if err == k8s.ErrRequestCancelled {
			return nil, nil
		}
		return result, err
	}
	return a.k8sClient.ListDeployments(namespace)
}

func (a *App) ScaleDeployment(namespace, name string, replicas int32) error {
	a.logDebug("ScaleDeployment called: ns=%s, name=%s, replicas=%d", namespace, name, replicas)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ScaleDeployment(namespace, name, replicas)
}

// Pod Logs: see app_logs.go
