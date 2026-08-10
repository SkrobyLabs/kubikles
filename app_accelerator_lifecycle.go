//go:build !accelerator

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"kubikles/pkg/acceleratorprovision"
)

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
func (a *App) EnableAccelerator(contextName, namespaceOverride, optionsJSON string) error {
	options := acceleratorprovision.DeploymentOptions{}
	if optionsJSON != "" {
		decoder := json.NewDecoder(bytes.NewBufferString(optionsJSON))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&options); err != nil {
			return fmt.Errorf("invalid Accelerator artifact options")
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			return fmt.Errorf("invalid Accelerator artifact options")
		}
	}
	if err := acceleratorprovision.ValidateDeploymentOptions(options); err != nil {
		return err
	}
	if a != nil && a.runtimeMode == RuntimeModeDesktop {
		if lifecycle, ok := a.acceleratorLifecycle.(interface {
			EnableWithOptions(string, string, acceleratorprovision.DeploymentOptions)
		}); ok {
			lifecycle.EnableWithOptions(contextName, namespaceOverride, options)
			return nil
		}
		if lifecycle, ok := a.acceleratorLifecycle.(interface{ Enable(string, string) }); ok {
			if options != (acceleratorprovision.DeploymentOptions{}) {
				return fmt.Errorf("Accelerator artifact options are unavailable")
			}
			lifecycle.Enable(contextName, namespaceOverride)
		}
	}
	return nil
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
