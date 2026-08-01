//go:build accelerator

package main

import (
	"context"
	"fmt"
	"sync"

	"kubikles/pkg/debug"
	"kubikles/pkg/events"
	"kubikles/pkg/k8s"
	"kubikles/pkg/server"
)

// App is the Accelerator's deliberately closed projection of the ordinary App.
type App struct {
	runtimeMode              RuntimeMode
	agentRouter              AgentRouter
	lifecycle                RuntimeLifecycle
	shutdownOnce             sync.Once
	ctx                      context.Context
	k8sClient                *k8s.Client
	k8sInitError             error
	listRequestManager       *ListRequestManager
	emitter                  events.Emitter
	acceleratorSecretWatches *AcceleratorSecretWatchManager
}

func NewApp() *App { app, _ := NewAppWithOptions(AppOptions{Mode: RuntimeModeDesktop}); return app }
func NewAppWithOptions(options AppOptions) (*App, error) {
	return newAppWithOptionsAndProfile(options, acceleratorAppConstructionProfile)
}

type appConstructionProfile struct{ newListRequestManager func() *ListRequestManager }

var acceleratorAppConstructionProfile = appConstructionProfile{newListRequestManager: NewListRequestManager}

func acceleratorAppOptions() AppOptions {
	return AppOptions{Mode: RuntimeModeAccelerator, KubernetesClientFactory: acceleratorKubernetesClientFactory}
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
	return &App{runtimeMode: options.Mode, agentRouter: options.AgentRouter, lifecycle: options.Lifecycle, k8sClient: client, k8sInitError: clientErr, listRequestManager: profile.newListRequestManager()}, nil
}

func (a *App) SetEmitter(emitter events.Emitter) { a.emitter = emitter }
func (a *App) startupServerMode(ctx context.Context) {
	a.ctx = ctx
	if a.emitter != nil {
		debug.Init(a.emitter)
	}
}
func (a *App) getDisconnectListeners() []server.DisconnectListener { return nil }
func (a *App) emitEvent(name string, data ...interface{}) {
	if a.emitter != nil {
		a.emitter.Emit(name, data...)
	}
}
func (a *App) shutdown(ctx context.Context) {
	a.shutdownOnce.Do(func() {
		if a.acceleratorSecretWatches != nil {
			_ = a.acceleratorSecretWatches.ClearAll(context.WithoutCancel(ctx))
		}
		if a.lifecycle != nil {
			a.lifecycle.Quiesce(ctx)
			a.lifecycle.StopProducers(ctx)
			a.lifecycle.Close(ctx)
		}
	})
}
