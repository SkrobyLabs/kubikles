//go:build !headless && helm && !accelerator

package main

import "kubikles/pkg/acceleratorprovision"

// newDesktopAcceleratorConnector is intentionally dormant.  It is kept out
// of App construction and lifecycle wiring until the later routing phase.
func newDesktopAcceleratorConnector() *acceleratorprovision.Connector {
	return acceleratorprovision.NewConnector(BuildVersion)
}
