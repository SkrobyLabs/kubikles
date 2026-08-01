package main

import (
	"context"

	"kubikles/pkg/debug"
)

func (a *App) quiesce(ctx context.Context) {
	if a.lifecycle != nil {
		a.lifecycle.Quiesce(ctx)
	}
}

func (a *App) stopProducers(ctx context.Context) {
	if a.ingressForwardManager != nil {
		a.ingressForwardManager.Cleanup()
	}
	if a.portForwardManager != nil {
		a.portForwardManager.StopAll()
	}
	if a.watcherManager != nil {
		a.watcherManager.StopAll()
	}
	if a.terminalManager != nil {
		a.terminalManager.CloseAllSessions()
	}
	if a.aiManager != nil {
		a.aiManager.CloseAllSessions()
	}
	// Existing embedded-browser cleanup remains best effort outside Accelerator mode.
	if a.runtimeMode != RuntimeModeAccelerator {
		_ = a.StopEmbeddedBrowser()
	}
	if a.lifecycle != nil {
		a.lifecycle.StopProducers(ctx)
	}
}

func (a *App) flushPendingEvents() {
	if a.eventCoalescer != nil {
		a.eventCoalescer.FlushNow()
	}
}

func (a *App) closeRuntime(ctx context.Context) {
	if a.lifecycle != nil {
		a.lifecycle.Close(ctx)
	}
}

func (a *App) runShutdownPhases(ctx context.Context) {
	a.shutdownOnce.Do(func() {
		a.quiesce(ctx)
		a.stopProducers(ctx)
		a.flushPendingEvents()
		a.closeRuntime(ctx)
	})
}

// shutdown is called when the app is closing.
func (a *App) shutdown(ctx context.Context) {
	debug.LogWails("App shutdown initiated", nil)
	a.runShutdownPhases(ctx)
	debug.LogWails("App shutdown complete", nil)
}
