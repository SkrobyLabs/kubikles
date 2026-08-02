//go:build !headless && helm && !accelerator

package main

import (
	"kubikles/pkg/acceleratorprovision"
	"kubikles/pkg/acceleratorrelease"
	"kubikles/pkg/k8s"
)

var (
	desktopAcceleratorProvisionerFactory = newDesktopAcceleratorProvisioner
	desktopAcceleratorConnectorFactory   = acceleratorprovision.NewConnector
	desktopAcceleratorReconnectorFactory = acceleratorprovision.NewReconnector
	desktopAcceleratorDisposerFactory    = acceleratorprovision.NewDisposalService
	desktopAcceleratorCoordinatorFactory = func(client *k8s.Client, resolver *acceleratorrelease.Resolver, provisioner *acceleratorprovision.Service, connector *acceleratorprovision.Connector, reconnector *acceleratorprovision.Reconnector, disposer *acceleratorprovision.DisposalService) desktopAcceleratorCoordinator {
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
	app.lifecycle = chainedRuntimeLifecycle{accelerator: coordinator, next: app.lifecycle}
}
