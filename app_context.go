package main

import (
	"context"
	"fmt"
	"time"

	"kubikles/pkg/debug"
)

// =============================================================================
// K8s Context & Connection
// =============================================================================

// GetK8sInitError returns the error message if K8s client failed to initialize.
// Returns empty string if initialization was successful.
func (a *App) GetK8sInitError() string {
	if a.k8sInitError != nil {
		return a.k8sInitError.Error()
	}
	return ""
}

func (a *App) ListContexts() ([]string, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListContexts()
}

func (a *App) GetCurrentContext() string {
	if a.k8sClient == nil {
		return ""
	}
	return a.k8sClient.GetCurrentContext()
}

func (a *App) SwitchContext(name string) error {
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}

	// Cancel any pending connection test
	a.CancelConnectionTest()

	// Stop all existing watchers before switching context
	// This prevents stale events from the old context being processed
	if a.watcherManager != nil {
		debug.LogK8s("SwitchContext: Stopping all watchers before context switch", nil)
		a.watcherManager.StopAll()
	}

	return a.k8sClient.SwitchContext(name)
}

// TestConnection performs a quick connectivity check to the current cluster.
// timeoutSeconds specifies how long to wait before giving up (recommended: 5-10s).
// Returns nil if reachable, or an error describing the failure.
// Any previous connection test is canceled before starting a new one.
func (a *App) TestConnection(timeoutSeconds int) error {
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}

	if timeoutSeconds <= 0 {
		timeoutSeconds = 5
	}

	// Cancel any previous connection test and store the new cancel func
	a.connTestMutex.Lock()
	if a.connTestCancel != nil {
		a.connTestCancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
	a.connTestCancel = cancel
	a.connTestMutex.Unlock()

	// Ensure cancel is called when done (idempotent, safe to call multiple times)
	defer cancel()

	return a.k8sClient.TestConnection(ctx)
}

// CancelConnectionTest cancels any in-progress connection test.
func (a *App) CancelConnectionTest() {
	a.connTestMutex.Lock()
	defer a.connTestMutex.Unlock()
	if a.connTestCancel != nil {
		a.connTestCancel()
		a.connTestCancel = nil
	}
}
