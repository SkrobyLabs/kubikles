package main

import (
	"fmt"

	coordinationv1 "k8s.io/api/coordination/v1"
	schedulingv1 "k8s.io/api/scheduling/v1"

	"kubikles/pkg/k8s"
)

// =============================================================================
// Scheduling
// =============================================================================

func (a *App) ListPriorityClasses(requestId string) ([]schedulingv1.PriorityClass, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	if requestId != "" {
		ctx, seq := a.listRequestManager.StartRequest(requestId)
		defer a.listRequestManager.CompleteRequest(requestId, seq)

		result, err := a.k8sClient.ListPriorityClassesWithContext(ctx)
		if err == k8s.ErrRequestCancelled {
			return nil, nil
		}
		return result, err
	}
	return a.k8sClient.ListPriorityClasses()
}

func (a *App) GetPriorityClassYaml(name string) (string, error) {
	a.logDebug("GetPriorityClassYaml called: name=%s", name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetPriorityClassYaml(name)
}

func (a *App) UpdatePriorityClassYaml(name, yamlContent string) error {
	a.logDebug("UpdatePriorityClassYaml called: name=%s", name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdatePriorityClassYaml(name, yamlContent)
}

func (a *App) DeletePriorityClass(name string) error {
	currentContext := a.GetCurrentContext()
	a.logDebug("DeletePriorityClass called: context=%s, name=%s", currentContext, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeletePriorityClass(currentContext, name)
}

// Lease operations (namespaced)
func (a *App) ListLeases(requestId, namespace string) ([]coordinationv1.Lease, error) {
	currentContext := a.GetCurrentContext()
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	if requestId != "" {
		ctx, seq := a.listRequestManager.StartRequest(requestId)
		defer a.listRequestManager.CompleteRequest(requestId, seq)

		result, err := a.k8sClient.ListLeasesWithContext(ctx, currentContext, namespace)
		if err == k8s.ErrRequestCancelled {
			return nil, nil
		}
		return result, err
	}
	return a.k8sClient.ListLeases(currentContext, namespace)
}

func (a *App) GetLeaseYaml(namespace, name string) (string, error) {
	currentContext := a.GetCurrentContext()
	a.logDebug("GetLeaseYaml called: context=%s, ns=%s, name=%s", currentContext, namespace, name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetLeaseYaml(currentContext, namespace, name)
}

func (a *App) UpdateLeaseYaml(namespace, name, yamlContent string) error {
	currentContext := a.GetCurrentContext()
	a.logDebug("UpdateLeaseYaml called: context=%s, ns=%s, name=%s", currentContext, namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateLeaseYaml(currentContext, namespace, name, yamlContent)
}

func (a *App) DeleteLease(namespace, name string) error {
	currentContext := a.GetCurrentContext()
	a.logDebug("DeleteLease called: context=%s, ns=%s, name=%s", currentContext, namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteLease(currentContext, namespace, name)
}
