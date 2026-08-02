//go:build !accelerator

package main

import (
	"context"
	"fmt"
	"time"

	"kubikles/pkg/debug"
	"kubikles/pkg/k8s"

	"github.com/wailsapp/wails/v2/pkg/runtime"
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
	a.contextMutationMu.Lock()
	defer a.contextMutationMu.Unlock()

	oldContext := a.k8sClient.GetCurrentContext()
	if a.acceleratorLifecycle != nil {
		a.acceleratorLifecycle.FenceContextSwitch(oldContext)
	}

	// Cancel any pending connection test
	a.CancelConnectionTest()

	// Stop non-KeepAlive port forwards from the departing context
	if a.portForwardManager != nil {
		debug.LogK8s("SwitchContext: Stopping port forwards for context", map[string]any{"context": oldContext})
		a.portForwardManager.StopAllForContext(oldContext)
	}

	// Discard any buffered events from the old context, then stop all watchers.
	// Order matters: clear coalescer first so the timer can't fire and emit
	// stale events between StopAll and the new context starting.
	if a.eventCoalescer != nil {
		a.eventCoalescer.Clear()
	}
	if a.watcherManager != nil {
		debug.LogK8s("SwitchContext: Stopping all watchers before context switch", nil)
		a.watcherManager.StopAll()
	}

	err := a.k8sClient.SwitchContext(name)
	if a.acceleratorLifecycle != nil {
		a.acceleratorLifecycle.ContextSwitched(name, err == nil)
	}
	return err
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

func (a *App) GetContextDetails() ([]k8s.ContextDetail, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetContextDetails()
}

func (a *App) DeleteContext(name string) error {
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteContext(name)
}

func (a *App) RenameContext(oldName, newName string) error {
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	a.contextMutationMu.Lock()
	defer a.contextMutationMu.Unlock()
	active := a.k8sClient.GetCurrentContext()
	isActive := oldName == active
	if isActive && a.acceleratorLifecycle != nil {
		a.acceleratorLifecycle.FenceContextSwitch(active)
	}
	err := a.k8sClient.RenameContext(oldName, newName)
	if isActive && a.acceleratorLifecycle != nil {
		a.acceleratorLifecycle.ContextSwitched(newName, err == nil)
	}
	return err
}

func (a *App) GetFullContextDetail(name string) (*k8s.FullContextDetail, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetFullContextDetail(name)
}

func (a *App) UpdateContextDetail(name string, req k8s.ContextUpdateRequest) error {
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	a.contextMutationMu.Lock()

	isActive := name == a.k8sClient.GetCurrentContext()
	if isActive && a.acceleratorLifecycle != nil {
		a.acceleratorLifecycle.FenceContextSwitch(name)
	}
	err := a.k8sClient.UpdateContextDetail(name, req)
	if isActive && a.acceleratorLifecycle != nil {
		a.acceleratorLifecycle.ContextSwitched(name, err == nil)
	}
	a.contextMutationMu.Unlock()
	if err != nil {
		return err
	}

	if isActive {
		a.CancelConnectionTest()
		if a.eventCoalescer != nil {
			a.eventCoalescer.Clear()
		}
		if a.watcherManager != nil {
			a.watcherManager.RestartAll()
		}
	}

	return nil
}

func (a *App) SetExtraKubeconfigPaths(paths []string) {
	if a.k8sClient != nil {
		a.contextMutationMu.Lock()
		defer a.contextMutationMu.Unlock()
		active := a.k8sClient.GetCurrentContext()
		if active != "" && a.acceleratorLifecycle != nil {
			a.acceleratorLifecycle.FenceContextSwitch(active)
		}
		a.k8sClient.SetExtraKubeconfigPaths(paths)
		if active != "" && a.acceleratorLifecycle != nil {
			a.acceleratorLifecycle.ContextSwitched(active, true)
		}
		debug.LogConfig("Extra kubeconfig paths", map[string]interface{}{"paths": paths})
	}
}

// SelectKubeconfigFile opens a native file dialog for selecting a kubeconfig file.
func (a *App) SelectKubeconfigFile() (string, error) {
	return runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select Kubeconfig File",
	})
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
