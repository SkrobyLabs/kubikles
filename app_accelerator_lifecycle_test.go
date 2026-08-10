//go:build !accelerator

package main

import (
	"testing"

	"kubikles/pkg/acceleratorprovision"
)

type capturingAcceleratorLifecycle struct {
	directOnlyAcceleratorCoordinator
	contextName string
	namespace   string
	options     acceleratorprovision.DeploymentOptions
}

func (l *capturingAcceleratorLifecycle) EnableWithOptions(contextName, namespace string, options acceleratorprovision.DeploymentOptions) {
	l.contextName = contextName
	l.namespace = namespace
	l.options = options
}

func TestEnableAcceleratorValidatesAndForwardsArtifactOptions(t *testing.T) {
	lifecycle := &capturingAcceleratorLifecycle{}
	app := &App{runtimeMode: RuntimeModeDesktop, acceleratorLifecycle: lifecycle}
	options := `{"imageReference":"ghcr.io/example/accelerator:test","chartReference":"ghcr.io/example/charts/accelerator:test"}`
	if err := app.EnableAccelerator("ctx", "accelerator-dev", options); err != nil {
		t.Fatal(err)
	}
	want := acceleratorprovision.DeploymentOptions{ImageReference: "ghcr.io/example/accelerator:test", ChartReference: "ghcr.io/example/charts/accelerator:test"}
	if lifecycle.contextName != "ctx" || lifecycle.namespace != "accelerator-dev" || lifecycle.options != want {
		t.Fatalf("forwarded=%q %q %#v", lifecycle.contextName, lifecycle.namespace, lifecycle.options)
	}
	for _, invalid := range []string{
		`{"imageReference":"https://example.test/image:tag"}`,
		`{"chartReference":"https://example.test/chart:tag"}`,
		`{"imageReference":"ghcr.io/example/image:tag","unknown":true}`,
	} {
		if err := app.EnableAccelerator("ctx", "default", invalid); err == nil {
			t.Fatalf("accepted invalid options %s", invalid)
		}
	}
}
