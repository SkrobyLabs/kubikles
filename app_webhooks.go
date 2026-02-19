package main

import (
	"fmt"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"

	"kubikles/pkg/debug"
	"kubikles/pkg/k8s"
)

// =============================================================================
// Webhooks & Admission Control
// =============================================================================

func (a *App) ListValidatingWebhookConfigurations(requestId string) ([]admissionregistrationv1.ValidatingWebhookConfiguration, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	if requestId != "" {
		ctx, seq := a.listRequestManager.StartRequest(requestId)
		defer a.listRequestManager.CompleteRequest(requestId, seq)

		result, err := a.k8sClient.ListValidatingWebhookConfigurationsWithContext(ctx, a.listProgressCallback("validatingwebhookconfigurations"))
		if err == k8s.ErrRequestCancelled {
			return nil, nil
		}
		return result, err
	}
	return a.k8sClient.ListValidatingWebhookConfigurations()
}

func (a *App) GetValidatingWebhookConfigurationYaml(name string) (string, error) {
	debug.LogK8s("GetValidatingWebhookConfigurationYaml called", map[string]interface{}{"name": name})
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetValidatingWebhookConfigurationYaml(name)
}

func (a *App) UpdateValidatingWebhookConfigurationYaml(name, yamlContent string) error {
	debug.LogK8s("UpdateValidatingWebhookConfigurationYaml called", map[string]interface{}{"name": name})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateValidatingWebhookConfigurationYaml(name, yamlContent)
}

func (a *App) DeleteValidatingWebhookConfiguration(name string) error {
	currentContext := a.GetCurrentContext()
	debug.LogK8s("DeleteValidatingWebhookConfiguration called", map[string]interface{}{"context": currentContext, "name": name})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteValidatingWebhookConfiguration(currentContext, name)
}

// MutatingWebhookConfiguration operations (cluster-scoped)
func (a *App) ListMutatingWebhookConfigurations(requestId string) ([]admissionregistrationv1.MutatingWebhookConfiguration, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	if requestId != "" {
		ctx, seq := a.listRequestManager.StartRequest(requestId)
		defer a.listRequestManager.CompleteRequest(requestId, seq)

		result, err := a.k8sClient.ListMutatingWebhookConfigurationsWithContext(ctx, a.listProgressCallback("mutatingwebhookconfigurations"))
		if err == k8s.ErrRequestCancelled {
			return nil, nil
		}
		return result, err
	}
	return a.k8sClient.ListMutatingWebhookConfigurations()
}

func (a *App) GetMutatingWebhookConfigurationYaml(name string) (string, error) {
	debug.LogK8s("GetMutatingWebhookConfigurationYaml called", map[string]interface{}{"name": name})
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetMutatingWebhookConfigurationYaml(name)
}

func (a *App) UpdateMutatingWebhookConfigurationYaml(name, yamlContent string) error {
	debug.LogK8s("UpdateMutatingWebhookConfigurationYaml called", map[string]interface{}{"name": name})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateMutatingWebhookConfigurationYaml(name, yamlContent)
}

func (a *App) DeleteMutatingWebhookConfiguration(name string) error {
	currentContext := a.GetCurrentContext()
	debug.LogK8s("DeleteMutatingWebhookConfiguration called", map[string]interface{}{"context": currentContext, "name": name})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteMutatingWebhookConfiguration(currentContext, name)
}
