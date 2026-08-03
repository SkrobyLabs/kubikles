//go:build !headless && helm && !accelerator

package main

import (
	"context"

	"kubikles/pkg/acceleratorprovision"
	"kubikles/pkg/acceleratorrelease"
	"kubikles/pkg/acceleratorsecret"
	"kubikles/pkg/k8s"
)

var (
	desktopAcceleratorProvisionerFactory  = newDesktopAcceleratorProvisioner
	desktopAcceleratorConnectorFactory    = acceleratorprovision.NewConnector
	desktopAcceleratorReconnectorFactory  = acceleratorprovision.NewReconnector
	desktopAcceleratorDisposerFactory     = acceleratorprovision.NewDisposalService
	desktopAcceleratorSecretClientFactory = acceleratorprovision.NewSecretRPCClient
	desktopAcceleratorCoordinatorFactory  = func(client *k8s.Client, resolver *acceleratorrelease.Resolver, provisioner *acceleratorprovision.Service, connector *acceleratorprovision.Connector, reconnector *acceleratorprovision.Reconnector, disposer *acceleratorprovision.DisposalService) desktopAcceleratorCoordinator {
		return acceleratorprovision.NewDesktopCoordinator(client, resolver, provisioner, connector, reconnector, disposer)
	}
)

func initializeDesktopAcceleratorLifecycle(app *App) {
	if app == nil {
		return
	}
	app.acceleratorLifecycle = directOnlyAcceleratorCoordinator{}
	if app.runtimeMode != RuntimeModeDesktop || app.k8sClient == nil || app.helmClient == nil {
		return
	}
	resolver, err := newDesktopAcceleratorReleaseResolver()
	if err != nil || resolver == nil {
		return
	}
	provisioner := desktopAcceleratorProvisionerFactory(app.k8sClient, app.helmClient)
	connector := desktopAcceleratorConnectorFactory(BuildVersion)
	reconnector := desktopAcceleratorReconnectorFactory(BuildVersion)
	disposer := desktopAcceleratorDisposerFactory(provisioner)
	coordinator := desktopAcceleratorCoordinatorFactory(app.k8sClient, resolver, provisioner, connector, reconnector, disposer)
	app.acceleratorLifecycle = coordinator
	router, err := newIntegratedSecretRouter(integratedSecretRouterDependencies{
		acquire: func(ctx context.Context, contextName string) (secretRouterDemandLease, bool) {
			result := coordinator.AcquireSecretDemand(ctx, contextName)
			if !result.Accepted || result.Lease == nil {
				return nil, false
			}
			return &productionSecretDemandLease{lease: result.Lease, newClient: desktopAcceleratorSecretClientFactory}, true
		},
		directList: func(ctx context.Context, _ string, namespace string, exclude bool) ([]k8s.SecretListItem, error) {
			items, listErr := app.k8sClient.ListSecretsMetadataWithOptions(ctx, namespace, k8s.SecretListOptions{ExcludeHelmReleases: exclude})
			if listErr != nil {
				return nil, listErr
			}
			projected := make([]acceleratorsecret.SecretListItem, len(items))
			for index, item := range items {
				projected[index] = acceleratorsecret.ProjectSecretListItem(item)
			}
			return acceleratorsecret.KubernetesSecretListItems(projected), nil
		},
		directData:  app.acceleratorSecretData,
		directYAML:  app.acceleratorSecretYAML,
		ready:       func(signal integratedSecretSourceSignal) { app.emitEvent(integratedSecretReadyEvent, signal) },
		unavailable: func(signal integratedSecretSourceSignal) { app.emitEvent(integratedSecretUnavailableEvent, signal) },
		resource:    func(signal integratedSecretResourceSignal) { app.emitEvent(integratedSecretResourceEvent, signal) },
		status:      func(signal integratedSecretStatusSignal) { app.emitEvent(integratedSecretStatusEvent, signal) },
		watchError:  func(signal integratedSecretErrorSignal) { app.emitEvent(integratedSecretErrorEvent, signal) },
	})
	if err != nil {
		app.lifecycle = chainedRuntimeLifecycle{accelerator: coordinator, next: app.lifecycle}
		return
	}
	app.agentRouter = secretReadAgentRouter{base: app.agentRouter, secrets: router}
	app.lifecycle = chainedRuntimeLifecycle{accelerator: router, next: chainedRuntimeLifecycle{accelerator: coordinator, next: app.lifecycle}}
}
