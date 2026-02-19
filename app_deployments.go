package main

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"

	"kubikles/pkg/debug"
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

		result, err := a.k8sClient.ListDeploymentsWithContext(ctx, namespace, a.listProgressCallback("deployments"))
		if err == k8s.ErrRequestCancelled {
			return nil, nil
		}
		return result, err
	}
	return a.k8sClient.ListDeployments(namespace)
}

func (a *App) ScaleDeployment(namespace, name string, replicas int32) error {
	debug.LogK8s("ScaleDeployment called", map[string]interface{}{"ns": namespace, "name": name, "replicas": replicas})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ScaleDeployment(namespace, name, replicas)
}

// Pod Logs: see app_logs.go
