package main

import (
	"fmt"

	v1 "k8s.io/api/core/v1"
)

// =============================================================================
// Events
// =============================================================================

func (a *App) ListEvents(namespace string) ([]v1.Event, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListEvents(namespace)
}

func (a *App) GetEventYAML(namespace, name string) (string, error) {
	a.logDebug("GetEventYAML called: namespace=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetEventYAML(namespace, name)
}

func (a *App) UpdateEventYAML(namespace, name string, yamlContent string) error {
	a.logDebug("UpdateEventYAML called: namespace=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateEventYAML(namespace, name, yamlContent)
}

func (a *App) DeleteEvent(namespace, name string) error {
	contextName := a.GetCurrentContext()
	a.logDebug("DeleteEvent called: context=%s, namespace=%s, name=%s", contextName, namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteEvent(contextName, namespace, name)
}
