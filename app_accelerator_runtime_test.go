//go:build !accelerator

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"kubikles/pkg/debug"
	"kubikles/pkg/events"
	"kubikles/pkg/k8s"
)

func TestAcceleratorAppConstructsExactMinimalCore(t *testing.T) {
	client := &k8s.Client{}
	manager := NewListRequestManager()
	factoryCalls := 0
	listCalls := 0
	ordinaryCalls := 0

	app, err := newAppWithOptionsAndProfile(AppOptions{
		Mode: RuntimeModeAccelerator,
		KubernetesClientFactory: func() (*k8s.Client, error) {
			factoryCalls++
			return client, nil
		},
	}, appConstructionProfile{
		newListRequestManager: func() *ListRequestManager {
			listCalls++
			return manager
		},
		initializeOrdinaryServices: func(*App) {
			ordinaryCalls++
		},
	})
	if err != nil {
		t.Fatalf("newAppWithOptionsAndProfile() error = %v", err)
	}
	if factoryCalls != 1 || listCalls != 1 || ordinaryCalls != 0 {
		t.Fatalf("construction calls = factory:%d list:%d ordinary:%d, want 1/1/0", factoryCalls, listCalls, ordinaryCalls)
	}
	if app.runtimeMode != RuntimeModeAccelerator || app.k8sClient != client || app.k8sInitError != nil {
		t.Fatalf("minimal core identity = mode:%q client:%p error:%v", app.runtimeMode, app.k8sClient, app.k8sInitError)
	}
	if app.agentRouter == nil || app.lifecycle == nil || app.listRequestManager != manager {
		t.Fatalf("minimal core seams are incomplete: %#v", app)
	}
	if got := app.listRequestManager.GetStats(); got != (ListRequestStats{}) {
		t.Fatalf("initial list stats = %#v, want zero", got)
	}
	assertAcceleratorOrdinaryServicesAbsent(t, app)
}

func TestAcceleratorConstructionFailsBeforeServices(t *testing.T) {
	sentinel := errors.New("client unavailable")
	tests := []struct {
		name    string
		factory KubernetesClientFactory
		wantErr error
	}{
		{name: "missing factory", wantErr: ErrAcceleratorClientFactoryRequired},
		{name: "factory error", factory: func() (*k8s.Client, error) { return nil, sentinel }, wantErr: ErrAcceleratorClientInitialization},
		{name: "nil client", factory: func() (*k8s.Client, error) { return nil, nil }, wantErr: ErrAcceleratorClientInitialization},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			listCalls := 0
			ordinaryCalls := 0
			app, err := newAppWithOptionsAndProfile(AppOptions{
				Mode:                    RuntimeModeAccelerator,
				KubernetesClientFactory: test.factory,
			}, appConstructionProfile{
				newListRequestManager: func() *ListRequestManager {
					listCalls++
					return NewListRequestManager()
				},
				initializeOrdinaryServices: func(*App) {
					ordinaryCalls++
				},
			})
			if app != nil || !errors.Is(err, test.wantErr) {
				t.Fatalf("construction = %#v, %v; want nil app and %v", app, err, test.wantErr)
			}
			if test.name == "factory error" && !errors.Is(err, sentinel) {
				t.Fatalf("factory cause not preserved: %v", err)
			}
			if listCalls != 0 || ordinaryCalls != 0 {
				t.Fatalf("partial construction calls = list:%d ordinary:%d, want zero", listCalls, ordinaryCalls)
			}
		})
	}
}

func TestAcceleratorAppOptionsUsesInClusterFactory(t *testing.T) {
	options := acceleratorAppOptions()
	if options.Mode != RuntimeModeAccelerator {
		t.Fatalf("mode = %q, want %q", options.Mode, RuntimeModeAccelerator)
	}
	if options.KubernetesClientFactory == nil {
		t.Fatal("KubernetesClientFactory is nil")
	}
	if reflect.ValueOf(options.KubernetesClientFactory).Pointer() != reflect.ValueOf(acceleratorKubernetesClientFactory).Pointer() {
		t.Fatal("acceleratorAppOptions does not use acceleratorKubernetesClientFactory")
	}
	if options.AgentRouter != nil || options.Lifecycle != nil {
		t.Fatalf("optional seams = %#v/%#v, want nil normalization defaults", options.AgentRouter, options.Lifecycle)
	}
}

func TestOrdinaryConstructionRunsRequiredGroupsOnce(t *testing.T) {
	for _, mode := range []RuntimeMode{RuntimeModeDesktop, RuntimeModeServer} {
		t.Run(string(mode), func(t *testing.T) {
			factoryCalls := 0
			listCalls := 0
			ordinaryCalls := 0
			app, err := newAppWithOptionsAndProfile(AppOptions{
				Mode: mode,
				KubernetesClientFactory: func() (*k8s.Client, error) {
					factoryCalls++
					return &k8s.Client{}, nil
				},
			}, appConstructionProfile{
				newListRequestManager: func() *ListRequestManager {
					listCalls++
					return NewListRequestManager()
				},
				initializeOrdinaryServices: func(*App) {
					ordinaryCalls++
				},
			})
			if err != nil || app == nil {
				t.Fatalf("ordinary construction = %#v, %v", app, err)
			}
			if factoryCalls != 1 || listCalls != 1 || ordinaryCalls != 1 {
				t.Fatalf("construction calls = factory:%d list:%d ordinary:%d, want 1/1/1", factoryCalls, listCalls, ordinaryCalls)
			}
			if app.acceleratorSecretWatches != nil {
				t.Fatalf("ordinary %s constructed Accelerator Secret watch manager: %#v", mode, app.acceleratorSecretWatches)
			}
		})
	}
}

func TestAcceleratorStartupInitializesOnlyEmitter(t *testing.T) {
	app := newMinimalAcceleratorTestApp(t, NoopRuntimeLifecycle{})
	ctx := context.WithValue(context.Background(), testContextKey{}, "accelerator")
	emitted := 0
	app.SetEmitter(events.EmitterFunc(func(string, ...interface{}) { emitted++ }))

	app.startupServerMode(ctx)
	app.startupServerMode(ctx)

	if app.ctx != ctx {
		t.Fatal("Accelerator startup did not store the supplied context")
	}
	if len(app.getDisconnectListeners()) != 0 {
		t.Fatalf("disconnect listeners = %d, want zero", len(app.getDisconnectListeners()))
	}
	assertAcceleratorOrdinaryServicesAbsent(t, app)

	debug.SetEnabled(true)
	t.Cleanup(func() { debug.SetEnabled(false) })
	debug.Log("test", "bound emitter", nil)
	if emitted != 1 {
		t.Fatalf("debug emitter calls = %d, want 1", emitted)
	}
}

func TestAcceleratorRuntimeTouchesNoPathsOrExecutables(t *testing.T) {
	probeRoot := t.TempDir()
	marker := filepath.Join(probeRoot, "marker")
	if err := os.WriteFile(marker, []byte("unchanged"), 0o600); err != nil {
		t.Fatal(err)
	}
	before := snapshotTree(t, probeRoot)

	for key, path := range map[string]string{
		"HOME":             "home",
		"KUBECONFIG":       "kubeconfig",
		"TMPDIR":           "tmp",
		"XDG_CONFIG_HOME":  "xdg-config",
		"XDG_CACHE_HOME":   "xdg-cache",
		"XDG_DATA_HOME":    "xdg-data",
		"HELM_CONFIG_HOME": "helm-config",
		"HELM_CACHE_HOME":  "helm-cache",
		"HELM_DATA_HOME":   "helm-data",
		"DOCKER_CONFIG":    "docker-config",
	} {
		t.Setenv(key, filepath.Join(probeRoot, path))
	}
	t.Setenv("PATH", "")

	app := newMinimalAcceleratorTestApp(t, NoopRuntimeLifecycle{})
	app.SetEmitter(&events.NoopEmitter{})
	app.startupServerMode(context.Background())
	app.runShutdownPhases(context.Background())

	after := snapshotTree(t, probeRoot)
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("Accelerator runtime changed filesystem: before=%v after=%v", before, after)
	}
}

func TestOrdinaryServerCompositionRegression(t *testing.T) {
	configureOrdinaryTestPaths(t)
	app, err := NewAppWithOptions(AppOptions{
		Mode: RuntimeModeServer,
		KubernetesClientFactory: func() (*k8s.Client, error) {
			return &k8s.Client{}, nil
		},
	})
	if err != nil {
		t.Fatalf("NewAppWithOptions() error = %v", err)
	}
	assertOrdinaryConstructorInventory(t, app)

	app.SetEmitter(&events.NoopEmitter{})
	app.startupServerMode(context.Background())
	assertOrdinaryStartupInventory(t, app)
	if len(app.getDisconnectListeners()) != 2 {
		t.Fatalf("disconnect listeners = %d, want 2", len(app.getDisconnectListeners()))
	}
	app.runShutdownPhases(context.Background())
}

func TestDesktopCompositionRegression(t *testing.T) {
	configureOrdinaryTestPaths(t)
	app, err := NewAppWithOptions(AppOptions{
		Mode: RuntimeModeDesktop,
		KubernetesClientFactory: func() (*k8s.Client, error) {
			return &k8s.Client{}, nil
		},
	})
	if err != nil {
		t.Fatalf("NewAppWithOptions() error = %v", err)
	}
	assertOrdinaryConstructorInventory(t, app)
	app.startup(context.Background())
	assertOrdinaryStartupInventory(t, app)
	// Wails runtime calls require the real lifecycle context. Replace the test
	// emitter before shutdown while retaining the desktop startup assertions.
	app.SetEmitter(&events.NoopEmitter{})
	app.runShutdownPhases(context.Background())
}

type testContextKey struct{}

func newMinimalAcceleratorTestApp(t *testing.T, lifecycle RuntimeLifecycle) *App {
	t.Helper()
	app, err := NewAppWithOptions(AppOptions{
		Mode:      RuntimeModeAccelerator,
		Lifecycle: lifecycle,
		KubernetesClientFactory: func() (*k8s.Client, error) {
			return &k8s.Client{}, nil
		},
	})
	if err != nil {
		t.Fatalf("NewAppWithOptions() error = %v", err)
	}
	return app
}

func assertAcceleratorOrdinaryServicesAbsent(t *testing.T, app *App) {
	t.Helper()
	if app.helmClient != nil || app.terminalManager != nil || app.aiManager != nil || app.watcherManager != nil ||
		app.portForwardManager != nil || app.ingressForwardManager != nil || app.eventCoalescer != nil ||
		app.logCoalescer != nil || app.themeManager != nil || app.scanEngine != nil || app.metricsRequestManager != nil {
		t.Fatalf("ordinary service initialized in Accelerator mode: %#v", app)
	}
	if app.logStreams != nil || app.prometheusConfigs != nil || app.prometheusConfigPath != "" || app.eventStats != nil || app.eventWindowStart != 0 {
		t.Fatalf("ordinary state initialized in Accelerator mode: %#v", app)
	}
	if app.embeddedBrowser.session != nil || app.embeddedBrowser.portFwdID != "" {
		t.Fatalf("embedded browser initialized in Accelerator mode: %#v", app.embeddedBrowser)
	}
}

func assertOrdinaryConstructorInventory(t *testing.T, app *App) {
	t.Helper()
	if app.helmClient == nil || app.terminalManager == nil || app.aiManager == nil || app.metricsRequestManager == nil || app.listRequestManager == nil {
		t.Fatalf("ordinary constructor inventory incomplete: %#v", app)
	}
	if app.logStreams == nil || app.prometheusConfigs == nil || app.prometheusConfigPath == "" {
		t.Fatalf("ordinary constructor state incomplete: %#v", app)
	}
}

func assertOrdinaryStartupInventory(t *testing.T, app *App) {
	t.Helper()
	if app.watcherManager == nil || app.portForwardManager == nil || app.ingressForwardManager == nil || app.eventCoalescer == nil ||
		app.logCoalescer == nil || app.themeManager == nil || app.scanEngine == nil || app.eventStats == nil || app.eventWindowStart == 0 {
		t.Fatalf("ordinary startup inventory incomplete: %#v", app)
	}
}

func configureOrdinaryTestPaths(t *testing.T) {
	t.Helper()
	root := t.TempDir()
	for key, path := range map[string]string{
		"HOME":             "home",
		"XDG_CONFIG_HOME":  "xdg-config",
		"XDG_CACHE_HOME":   "xdg-cache",
		"XDG_DATA_HOME":    "xdg-data",
		"HELM_CONFIG_HOME": "helm-config",
		"HELM_CACHE_HOME":  "helm-cache",
		"HELM_DATA_HOME":   "helm-data",
		"DOCKER_CONFIG":    "docker-config",
	} {
		value := filepath.Join(root, path)
		if err := os.MkdirAll(value, 0o755); err != nil {
			t.Fatal(err)
		}
		t.Setenv(key, value)
	}
}

func snapshotTree(t *testing.T, root string) map[string]string {
	t.Helper()
	snapshot := make(map[string]string)
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if info.IsDir() {
			snapshot[rel] = "dir"
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		snapshot[rel] = string(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}
