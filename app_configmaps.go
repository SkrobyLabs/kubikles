package main

import (
	"context"
	"fmt"
	"time"

	v1 "k8s.io/api/core/v1"

	"kubikles/pkg/debug"
	"kubikles/pkg/k8s"
)

// =============================================================================
// ConfigMaps & Secrets
// =============================================================================

func (a *App) ListConfigMaps(requestId, namespace string) ([]v1.ConfigMap, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	if requestId != "" {
		ctx, seq := a.listRequestManager.StartRequest(requestId)
		defer a.listRequestManager.CompleteRequest(requestId, seq)

		result, err := a.k8sClient.ListConfigMapsWithContext(ctx, namespace)
		if err == k8s.ErrRequestCancelled {
			return nil, nil
		}
		return result, err
	}
	return a.k8sClient.ListConfigMaps(namespace)
}

func (a *App) ListSecrets(requestId, namespace string) ([]v1.Secret, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	if requestId != "" {
		ctx, seq := a.listRequestManager.StartRequest(requestId)
		defer a.listRequestManager.CompleteRequest(requestId, seq)

		result, err := a.k8sClient.ListSecretsWithContext(ctx, namespace)
		if err == k8s.ErrRequestCancelled {
			return nil, nil
		}
		return result, err
	}
	return a.k8sClient.ListSecrets(namespace)
}

// ListSecretsMetadata returns a lightweight list of secrets for display purposes.
// It uses the Table API to avoid transferring actual secret data.
func (a *App) ListSecretsMetadata(requestId, namespace string) ([]k8s.SecretListItem, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	if requestId != "" {
		ctx, seq := a.listRequestManager.StartRequest(requestId)
		defer a.listRequestManager.CompleteRequest(requestId, seq)

		result, err := a.k8sClient.ListSecretsMetadataWithContext(ctx, namespace)
		if err == k8s.ErrRequestCancelled {
			return nil, nil
		}
		return result, err
	}
	// For non-cancellable requests, use a default context
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	return a.k8sClient.ListSecretsMetadataWithContext(ctx, namespace)
}

// ConfigMap YAML operations
func (a *App) GetConfigMapYaml(namespace, name string) (string, error) {
	debug.LogConfig("GetConfigMapYaml called", map[string]interface{}{"namespace": namespace, "name": name})
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetConfigMapYaml(namespace, name)
}

func (a *App) UpdateConfigMapYaml(namespace, name, yamlContent string) error {
	debug.LogConfig("UpdateConfigMapYaml called", map[string]interface{}{"namespace": namespace, "name": name})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateConfigMapYaml(namespace, name, yamlContent)
}

func (a *App) DeleteConfigMap(namespace, name string) error {
	contextName := a.GetCurrentContext()
	debug.LogConfig("DeleteConfigMap called", map[string]interface{}{"context": contextName, "namespace": namespace, "name": name})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteConfigMap(contextName, namespace, name)
}

func (a *App) GetConfigMapData(namespace, name string) (map[string]string, error) {
	debug.LogConfig("GetConfigMapData called", map[string]interface{}{"namespace": namespace, "name": name})
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetConfigMapData(namespace, name)
}

func (a *App) UpdateConfigMapData(namespace, name string, data map[string]string) error {
	debug.LogConfig("UpdateConfigMapData called", map[string]interface{}{"namespace": namespace, "name": name})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateConfigMapData(namespace, name, data)
}

// Secret YAML operations
func (a *App) GetSecretYaml(namespace, name string) (string, error) {
	debug.LogConfig("GetSecretYaml called", map[string]interface{}{"namespace": namespace, "name": name})
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetSecretYaml(namespace, name)
}

func (a *App) UpdateSecretYaml(namespace, name, yamlContent string) error {
	debug.LogConfig("UpdateSecretYaml called", map[string]interface{}{"namespace": namespace, "name": name})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateSecretYaml(namespace, name, yamlContent)
}

func (a *App) DeleteSecret(namespace, name string) error {
	contextName := a.GetCurrentContext()
	debug.LogConfig("DeleteSecret called", map[string]interface{}{"context": contextName, "namespace": namespace, "name": name})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteSecret(contextName, namespace, name)
}

func (a *App) GetSecretData(namespace, name string) (map[string]string, error) {
	debug.LogConfig("GetSecretData called", map[string]interface{}{"namespace": namespace, "name": name})
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetSecretData(namespace, name)
}

func (a *App) UpdateSecretData(namespace, name string, data map[string]string) error {
	debug.LogConfig("UpdateSecretData called", map[string]interface{}{"namespace": namespace, "name": name})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateSecretData(namespace, name, data)
}
