//go:build !headless && helm && !accelerator

package main

import (
	"sync/atomic"
	"testing"

	"kubikles/pkg/helm"
	"kubikles/pkg/k8s"
)

func TestDesktopProvisionerIsDormant(t *testing.T) {
	if service := newDesktopAcceleratorProvisioner(nil, helm.NewClient()); service == nil {
		t.Fatal("constructor returned nil")
	}
	var clientFactoryCalls atomic.Int32
	app, err := newAppWithOptionsAndProfile(AppOptions{
		Mode: RuntimeModeDesktop,
		KubernetesClientFactory: func() (*k8s.Client, error) {
			clientFactoryCalls.Add(1)
			return nil, nil
		},
	}, appConstructionProfile{
		newListRequestManager:      NewListRequestManager,
		initializeOrdinaryServices: func(*App) {},
	})
	if err != nil || app == nil || clientFactoryCalls.Load() != 1 {
		t.Fatalf("ordinary construction changed: app=%#v err=%v calls=%d", app, err, clientFactoryCalls.Load())
	}
	if app.k8sClient != nil || app.helmClient != nil {
		t.Fatal("test profile unexpectedly constructed external clients")
	}
}
