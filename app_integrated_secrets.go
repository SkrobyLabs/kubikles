//go:build !accelerator

package main

import (
	"context"

	"kubikles/pkg/k8s"
)

func (a *App) integratedSecretReads() SecretReadRouter {
	if a == nil || a.agentRouter == nil || a.runtimeMode != RuntimeModeDesktop {
		return unavailableSecretReadRouter{}
	}
	reader := a.agentRouter.SecretReads()
	if reader == nil {
		return unavailableSecretReadRouter{}
	}
	return reader
}

func (a *App) integratedSecretContext() context.Context {
	if a != nil && a.ctx != nil {
		return a.ctx
	}
	return context.Background()
}

//kubikles:dispatch exclude
func (a *App) RetainIntegratedSecretReads() {
	a.integratedSecretReads().Retain(a.integratedSecretContext(), a.GetCurrentContext())
}

//kubikles:dispatch exclude
func (a *App) ReleaseIntegratedSecretReads() {
	a.integratedSecretReads().Release()
}

//kubikles:dispatch exclude
func (a *App) ListIntegratedSecretsMetadata(sourceToken, requestID, namespace string, excludeHelmReleases bool) ([]k8s.SecretListItem, error) {
	return a.integratedSecretReads().ListSecretsMetadata(a.integratedSecretContext(), SecretReadSourceToken(sourceToken), requestID, namespace, excludeHelmReleases)
}

//kubikles:dispatch exclude
func (a *App) GetIntegratedSecretData(sourceToken, namespace, name string) ([]k8s.DataEntry, error) {
	return a.integratedSecretReads().GetSecretData(a.integratedSecretContext(), SecretReadSourceToken(sourceToken), namespace, name)
}

//kubikles:dispatch exclude
func (a *App) GetIntegratedSecretYaml(sourceToken, namespace, name string) (string, error) {
	return a.integratedSecretReads().GetSecretYaml(a.integratedSecretContext(), SecretReadSourceToken(sourceToken), namespace, name)
}

//kubikles:dispatch exclude
func (a *App) CancelIntegratedSecretListRequest(sourceToken, requestID string) (bool, error) {
	return a.integratedSecretReads().CancelListRequest(a.integratedSecretContext(), SecretReadSourceToken(sourceToken), requestID)
}

//kubikles:dispatch exclude
func (a *App) SubscribeIntegratedSecretWatcher(sourceToken, namespace string, excludeHelmReleases bool) (string, error) {
	subscription, err := a.integratedSecretReads().SubscribeSecretWatcher(a.integratedSecretContext(), SecretReadSourceToken(sourceToken), namespace, excludeHelmReleases)
	return string(subscription.WatcherSpecID), err
}

//kubikles:dispatch exclude
func (a *App) UnsubscribeIntegratedSecretWatcher(sourceToken, watcherSpecID string) error {
	return a.integratedSecretReads().UnsubscribeSecretWatcher(a.integratedSecretContext(), SecretReadSourceToken(sourceToken), SecretWatchSpecID(watcherSpecID))
}
