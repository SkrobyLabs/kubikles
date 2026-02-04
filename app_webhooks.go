package main

import (
	"fmt"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
)

// =============================================================================
// Webhooks & Admission Control
// =============================================================================

func (a *App) ListValidatingWebhookConfigurations() ([]admissionregistrationv1.ValidatingWebhookConfiguration, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListValidatingWebhookConfigurations()
}

func (a *App) GetValidatingWebhookConfigurationYaml(name string) (string, error) {
	a.logDebug("GetValidatingWebhookConfigurationYaml called: name=%s", name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetValidatingWebhookConfigurationYaml(name)
}

func (a *App) UpdateValidatingWebhookConfigurationYaml(name, yamlContent string) error {
	a.logDebug("UpdateValidatingWebhookConfigurationYaml called: name=%s", name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateValidatingWebhookConfigurationYaml(name, yamlContent)
}

func (a *App) DeleteValidatingWebhookConfiguration(name string) error {
	currentContext := a.GetCurrentContext()
	a.logDebug("DeleteValidatingWebhookConfiguration called: context=%s, name=%s", currentContext, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteValidatingWebhookConfiguration(currentContext, name)
}

// MutatingWebhookConfiguration operations (cluster-scoped)
func (a *App) ListMutatingWebhookConfigurations() ([]admissionregistrationv1.MutatingWebhookConfiguration, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListMutatingWebhookConfigurations()
}

func (a *App) GetMutatingWebhookConfigurationYaml(name string) (string, error) {
	a.logDebug("GetMutatingWebhookConfigurationYaml called: name=%s", name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetMutatingWebhookConfigurationYaml(name)
}

func (a *App) UpdateMutatingWebhookConfigurationYaml(name, yamlContent string) error {
	a.logDebug("UpdateMutatingWebhookConfigurationYaml called: name=%s", name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateMutatingWebhookConfigurationYaml(name, yamlContent)
}

func (a *App) DeleteMutatingWebhookConfiguration(name string) error {
	currentContext := a.GetCurrentContext()
	a.logDebug("DeleteMutatingWebhookConfiguration called: context=%s, name=%s", currentContext, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteMutatingWebhookConfiguration(currentContext, name)
}
