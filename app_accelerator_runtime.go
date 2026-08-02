//go:build !accelerator

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"kubikles/pkg/debug"
	"kubikles/pkg/k8s"
	"kubikles/pkg/terminal"
)

// appConstructionProfile is the bounded construction seam used to prove that
// Accelerator composition creates only its required list lifecycle.
type appConstructionProfile struct {
	newListRequestManager      func() *ListRequestManager
	initializeOrdinaryServices func(*App)
}

var productionAppConstructionProfile = appConstructionProfile{
	newListRequestManager:      NewListRequestManager,
	initializeOrdinaryServices: initializeOrdinaryServices,
}

func acceleratorAppOptions() AppOptions {
	return AppOptions{
		Mode:                    RuntimeModeAccelerator,
		KubernetesClientFactory: acceleratorKubernetesClientFactory,
	}
}

func newAppWithOptionsAndProfile(options AppOptions, profile appConstructionProfile) (*App, error) {
	options, err := normalizeAppOptions(options)
	if err != nil {
		return nil, err
	}

	client, clientErr := options.KubernetesClientFactory()
	if options.Mode == RuntimeModeAccelerator && (clientErr != nil || client == nil) {
		if clientErr != nil {
			return nil, fmt.Errorf("%w: %w", ErrAcceleratorClientInitialization, clientErr)
		}
		return nil, ErrAcceleratorClientInitialization
	}
	if clientErr != nil {
		fmt.Printf("Error initializing K8s client: %v\n", clientErr)
		// Continue - UI will show the error via GetK8sInitError().
	}

	app := &App{
		runtimeMode:  options.Mode,
		agentRouter:  options.AgentRouter,
		lifecycle:    options.Lifecycle,
		k8sClient:    client,
		k8sInitError: clientErr,
	}
	initializeAcceleratorApp(app, profile)
	if options.Mode != RuntimeModeAccelerator {
		profile.initializeOrdinaryServices(app)
		initializeDesktopAcceleratorLifecycle(app)
	}
	return app, nil
}

// initializeAcceleratorApp installs the complete minimal runtime core shared by
// all modes. Ordinary modes add their existing eager services separately.
func initializeAcceleratorApp(app *App, profile appConstructionProfile) {
	app.listRequestManager = profile.newListRequestManager()
}

// initializeOrdinaryServices preserves the eager desktop/server constructor
// behavior. Accelerator mode never calls this function.
func initializeOrdinaryServices(app *App) {
	// Setup prometheus config path.
	configDir, _ := os.UserConfigDir()
	appDir := filepath.Join(configDir, "kubikles")
	os.MkdirAll(appDir, 0755)

	app.helmClient = initHelm()
	app.terminalManager = terminal.NewManager()
	app.aiManager = newAIManager(app.k8sClient)
	app.logStreams = make(map[string]context.CancelFunc)
	app.prometheusConfigs = make(map[string]*k8s.PrometheusInfo)
	app.prometheusConfigPath = filepath.Join(appDir, "prometheus_config.json")
	app.metricsRequestManager = NewMetricsRequestManager()
}

func (a *App) startupAcceleratorServerMode(ctx context.Context) {
	a.ctx = ctx
	if a.emitter != nil {
		debug.Init(a.emitter)
	}
}
