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

func TestEnableAcceleratorValidatesAndForwardsDevelopmentOptions(t *testing.T) {
	lifecycle := &capturingAcceleratorLifecycle{}
	app := &App{runtimeMode: RuntimeModeDesktop, acceleratorLifecycle: lifecycle}
	options := `{"releaseVersion":"v1.4.0-alpha.1","descriptorURL":"https://artifacts.example.test/release.json","allowVersionMismatch":true}`
	if err := app.EnableAccelerator("ctx", "accelerator-dev", options); err != nil {
		t.Fatal(err)
	}
	want := acceleratorprovision.DeploymentOptions{ReleaseVersion: "v1.4.0-alpha.1", DescriptorURL: "https://artifacts.example.test/release.json", AllowVersionMismatch: true}
	if lifecycle.contextName != "ctx" || lifecycle.namespace != "accelerator-dev" || lifecycle.options != want {
		t.Fatalf("forwarded=%q %q %#v", lifecycle.contextName, lifecycle.namespace, lifecycle.options)
	}
	for _, invalid := range []string{
		`{"releaseVersion":"dev"}`,
		`{"releaseVersion":"v1.4.0","descriptorURL":"http://localhost/release.json"}`,
		`{"releaseVersion":"v1.4.0","unknown":true}`,
	} {
		if err := app.EnableAccelerator("ctx", "default", invalid); err == nil {
			t.Fatalf("accepted invalid options %s", invalid)
		}
	}
}
