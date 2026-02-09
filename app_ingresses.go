package main

import (
	"fmt"

	networkingv1 "k8s.io/api/networking/v1"

	"kubikles/pkg/debug"
	"kubikles/pkg/k8s"
)

// =============================================================================
// Ingresses
// =============================================================================

func (a *App) ListIngresses(requestId, namespace string) ([]networkingv1.Ingress, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	if requestId != "" {
		ctx, seq := a.listRequestManager.StartRequest(requestId)
		defer a.listRequestManager.CompleteRequest(requestId, seq)

		result, err := a.k8sClient.ListIngressesWithContext(ctx, namespace)
		if err == k8s.ErrRequestCancelled {
			return nil, nil
		}
		return result, err
	}
	return a.k8sClient.ListIngresses(namespace)
}

func (a *App) GetIngressYaml(namespace, name string) (string, error) {
	debug.LogK8s("GetIngressYaml called", map[string]interface{}{"ns": namespace, "name": name})
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetIngressYaml(namespace, name)
}

func (a *App) UpdateIngressYaml(namespace, name, yamlContent string) error {
	debug.LogK8s("UpdateIngressYaml called", map[string]interface{}{"ns": namespace, "name": name})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateIngressYaml(namespace, name, yamlContent)
}

func (a *App) DeleteIngress(namespace, name string) error {
	currentContext := a.GetCurrentContext()
	debug.LogK8s("DeleteIngress called", map[string]interface{}{"context": currentContext, "ns": namespace, "name": name})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteIngress(currentContext, namespace, name)
}

// IngressClass operations (cluster-scoped)
func (a *App) ListIngressClasses(requestId string) ([]networkingv1.IngressClass, error) {
	currentContext := a.GetCurrentContext()
	debug.LogK8s("ListIngressClasses called", map[string]interface{}{"context": currentContext})
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	if requestId != "" {
		ctx, seq := a.listRequestManager.StartRequest(requestId)
		defer a.listRequestManager.CompleteRequest(requestId, seq)

		result, err := a.k8sClient.ListIngressClassesWithContext(ctx, currentContext)
		if err == k8s.ErrRequestCancelled {
			return nil, nil
		}
		return result, err
	}
	return a.k8sClient.ListIngressClasses(currentContext)
}

func (a *App) GetIngressClassYaml(name string) (string, error) {
	debug.LogK8s("GetIngressClassYaml called", map[string]interface{}{"name": name})
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetIngressClassYaml(name)
}

func (a *App) UpdateIngressClassYaml(name, yamlContent string) error {
	debug.LogK8s("UpdateIngressClassYaml called", map[string]interface{}{"name": name})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateIngressClassYaml(name, yamlContent)
}

func (a *App) DeleteIngressClass(name string) error {
	currentContext := a.GetCurrentContext()
	debug.LogK8s("DeleteIngressClass called", map[string]interface{}{"context": currentContext, "name": name})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteIngressClass(currentContext, name)
}
