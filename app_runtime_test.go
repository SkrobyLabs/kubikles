//go:build !accelerator

package main

import (
	"errors"
	"testing"

	"kubikles/pkg/agent"
	"kubikles/pkg/k8s"
)

func TestNormalizeAppOptionsRejectsInvalidMode(t *testing.T) {
	for _, mode := range []RuntimeMode{"", "agent", "Desktop", "other"} {
		t.Run(string(mode), func(t *testing.T) {
			_, err := normalizeAppOptions(AppOptions{Mode: mode})
			if !errors.Is(err, ErrInvalidRuntimeMode) {
				t.Fatalf("normalizeAppOptions(%q) error = %v, want ErrInvalidRuntimeMode", mode, err)
			}
		})
	}
	for _, mode := range []RuntimeMode{RuntimeModeDesktop, RuntimeModeServer, RuntimeModeAccelerator} {
		t.Run(string(mode), func(t *testing.T) {
			options := AppOptions{Mode: mode}
			if mode == RuntimeModeAccelerator {
				options.KubernetesClientFactory = func() (*k8s.Client, error) { return &k8s.Client{}, nil }
			}
			if _, err := normalizeAppOptions(options); err != nil {
				t.Fatalf("normalizeAppOptions(%q) error = %v", mode, err)
			}
		})
	}
}

func TestNewAppWithOptionsDesktopKeepsClientErrorRecoverable(t *testing.T) {
	testNewAppWithOptionsKeepsClientErrorRecoverable(t, RuntimeModeDesktop)
}

func TestNewAppWithOptionsServerKeepsClientErrorRecoverable(t *testing.T) {
	testNewAppWithOptionsKeepsClientErrorRecoverable(t, RuntimeModeServer)
}

func testNewAppWithOptionsKeepsClientErrorRecoverable(t *testing.T, mode RuntimeMode) {
	t.Helper()
	want := errors.New("client unavailable")
	app, err := NewAppWithOptions(AppOptions{
		Mode: mode,
		KubernetesClientFactory: func() (*k8s.Client, error) {
			return nil, want
		},
	})
	if err != nil || app == nil {
		t.Fatalf("NewAppWithOptions() = %v, %v; want app and nil error", app, err)
	}
	if !errors.Is(app.k8sInitError, want) || app.runtimeMode != mode || app.agentRouter == nil || app.lifecycle == nil || app.metricsRequestManager == nil || app.listRequestManager == nil {
		t.Fatalf("unexpected recoverable app state: %#v", app)
	}
}

func TestNewAppWithOptionsAcceleratorFailsClosed(t *testing.T) {
	want := errors.New("client unavailable")
	for _, test := range []struct {
		name    string
		factory KubernetesClientFactory
		wantErr error
	}{
		{"missing factory", nil, ErrAcceleratorClientFactoryRequired},
		{"factory error", func() (*k8s.Client, error) { return nil, want }, ErrAcceleratorClientInitialization},
		{"nil client", func() (*k8s.Client, error) { return nil, nil }, ErrAcceleratorClientInitialization},
	} {
		t.Run(test.name, func(t *testing.T) {
			app, err := NewAppWithOptions(AppOptions{Mode: RuntimeModeAccelerator, KubernetesClientFactory: test.factory})
			if app != nil || !errors.Is(err, test.wantErr) {
				t.Fatalf("NewAppWithOptions() = %v, %v; want nil app and %v", app, err, test.wantErr)
			}
			if test.name == "factory error" && !errors.Is(err, want) {
				t.Fatalf("factory cause was not wrapped: %v", err)
			}
		})
	}
}

func TestNewAppWithOptionsAcceleratorUsesInjectedClient(t *testing.T) {
	client := &k8s.Client{}
	app, err := NewAppWithOptions(AppOptions{Mode: RuntimeModeAccelerator, KubernetesClientFactory: func() (*k8s.Client, error) { return client, nil }})
	if err != nil || app == nil || app.k8sClient != client || app.runtimeMode != RuntimeModeAccelerator || app.k8sInitError != nil {
		t.Fatalf("unexpected Accelerator app: %#v, %v", app, err)
	}
}

func TestNewAppCompatibilityConstructorDefaults(t *testing.T) {
	configureOrdinaryTestPaths(t)
	app := NewApp()
	if app == nil || app.runtimeMode != RuntimeModeDesktop || app.agentRouter == nil || app.lifecycle == nil {
		t.Fatalf("NewApp() returned incomplete desktop app: %#v", app)
	}
}

func TestNoopAgentRouterSupportsNothing(t *testing.T) {
	router := NoopAgentRouter{}
	for _, capability := range agent.V1Capabilities() {
		if router.Supports(capability) {
			t.Fatalf("NoopAgentRouter supports %q", capability)
		}
	}
	lifecycle := NoopRuntimeLifecycle{}
	lifecycle.Quiesce(nil)
	lifecycle.StopProducers(nil)
	lifecycle.Close(nil)
}
