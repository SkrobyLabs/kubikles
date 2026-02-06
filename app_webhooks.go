package main

import (
	"fmt"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"

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

		result, err := a.k8sClient.ListValidatingWebhookConfigurationsWithContext(ctx)
		if err == k8s.ErrRequestCancelled {
			return nil, nil
		}
		return result, err
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
func (a *App) ListMutatingWebhookConfigurations(requestId string) ([]admissionregistrationv1.MutatingWebhookConfiguration, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	if requestId != "" {
		ctx, seq := a.listRequestManager.StartRequest(requestId)
		defer a.listRequestManager.CompleteRequest(requestId, seq)

		result, err := a.k8sClient.ListMutatingWebhookConfigurationsWithContext(ctx)
		if err == k8s.ErrRequestCancelled {
			return nil, nil
		}
		return result, err
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
