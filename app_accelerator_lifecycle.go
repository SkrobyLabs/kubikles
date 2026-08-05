//go:build !accelerator

package main

import "kubikles/pkg/acceleratorprovision"

//kubikles:dispatch exclude
func (a *App) GetAcceleratorStatus(contextName string) acceleratorprovision.CoordinatorSnapshot {
	if a == nil || a.acceleratorLifecycle == nil || a.runtimeMode != RuntimeModeDesktop {
		return acceleratorprovision.CoordinatorSnapshot{State: acceleratorprovision.CoordinatorDirectOnly}
	}
	if lifecycle, ok := a.acceleratorLifecycle.(interface {
		Snapshot(string) acceleratorprovision.CoordinatorSnapshot
	}); ok {
		return lifecycle.Snapshot(contextName)
	}
	return acceleratorprovision.CoordinatorSnapshot{State: acceleratorprovision.CoordinatorDirectOnly}
}

//kubikles:dispatch exclude
func (a *App) EnableAccelerator(contextName, namespaceOverride string) {
	if a != nil && a.runtimeMode == RuntimeModeDesktop {
		if lifecycle, ok := a.acceleratorLifecycle.(interface{ Enable(string, string) }); ok {
			lifecycle.Enable(contextName, namespaceOverride)
		}
	}
}

//kubikles:dispatch exclude
func (a *App) RetryAccelerator(contextName string) {
	if a != nil && a.runtimeMode == RuntimeModeDesktop {
		if lifecycle, ok := a.acceleratorLifecycle.(interface{ Retry(string) }); ok {
			lifecycle.Retry(contextName)
		}
	}
}

//kubikles:dispatch exclude
func (a *App) DisableAccelerator(contextName string) {
	if a != nil && a.runtimeMode == RuntimeModeDesktop {
		if lifecycle, ok := a.acceleratorLifecycle.(interface{ Disable(string) }); ok {
			lifecycle.Disable(contextName)
		}
	}
}
