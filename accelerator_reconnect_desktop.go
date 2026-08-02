//go:build !headless && helm && !accelerator

package main

import "kubikles/pkg/acceleratorprovision"

// newDesktopAcceleratorReconnector is intentionally dormant until the later
// lifecycle coordinator owns invocation and routing decisions.
func newDesktopAcceleratorReconnector() *acceleratorprovision.Reconnector {
	return acceleratorprovision.NewReconnector(BuildVersion)
}
