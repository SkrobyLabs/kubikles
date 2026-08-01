//go:build accelerator

package main

import (
	"context"
	"fmt"
	"time"

	"kubikles/pkg/k8s"
)

func (a *App) listSecretsMetadataWithOptions(requestID, namespace string, options k8s.SecretListOptions) ([]k8s.SecretListItem, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	if requestID != "" {
		ctx, seq := a.listRequestManager.StartRequest(requestID)
		defer a.listRequestManager.CompleteRequest(requestID, seq)
		result, err := a.k8sClient.ListSecretsMetadataWithOptions(ctx, namespace, options, nil)
		if err == k8s.ErrRequestCancelled {
			return nil, nil
		}
		return result, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	return a.k8sClient.ListSecretsMetadataWithOptions(ctx, namespace, options)
}
func (a *App) ListSecretsMetadata(requestID, namespace string) ([]k8s.SecretListItem, error) {
	return a.listSecretsMetadataWithOptions(requestID, namespace, k8s.SecretListOptions{})
}
func (a *App) GetSecretData(namespace, name string) ([]k8s.DataEntry, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetSecretData(namespace, name)
}
func (a *App) GetSecretYaml(namespace, name string) (string, error) {
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetSecretYaml(namespace, name)
}
func (a *App) CancelListRequest(requestID string) bool {
	return a.listRequestManager.CancelRequest(requestID)
}
