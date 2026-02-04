package main

import (
	"fmt"

	networkingv1 "k8s.io/api/networking/v1"
)

// =============================================================================
// Ingresses
// =============================================================================

func (a *App) ListIngresses(namespace string) ([]networkingv1.Ingress, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListIngresses(namespace)
}

func (a *App) GetIngressYaml(namespace, name string) (string, error) {
	a.logDebug("GetIngressYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetIngressYaml(namespace, name)
}

func (a *App) UpdateIngressYaml(namespace, name, yamlContent string) error {
	a.logDebug("UpdateIngressYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateIngressYaml(namespace, name, yamlContent)
}

func (a *App) DeleteIngress(namespace, name string) error {
	currentContext := a.GetCurrentContext()
	a.logDebug("DeleteIngress called: context=%s, ns=%s, name=%s", currentContext, namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteIngress(currentContext, namespace, name)
}

// IngressClass operations (cluster-scoped)
func (a *App) ListIngressClasses() ([]networkingv1.IngressClass, error) {
	currentContext := a.GetCurrentContext()
	a.logDebug("ListIngressClasses called: context=%s", currentContext)
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListIngressClasses(currentContext)
}

func (a *App) GetIngressClassYaml(name string) (string, error) {
	a.logDebug("GetIngressClassYaml called: name=%s", name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetIngressClassYaml(name)
}

func (a *App) UpdateIngressClassYaml(name, yamlContent string) error {
	a.logDebug("UpdateIngressClassYaml called: name=%s", name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateIngressClassYaml(name, yamlContent)
}

func (a *App) DeleteIngressClass(name string) error {
	currentContext := a.GetCurrentContext()
	a.logDebug("DeleteIngressClass called: context=%s, name=%s", currentContext, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteIngressClass(currentContext, name)
}
