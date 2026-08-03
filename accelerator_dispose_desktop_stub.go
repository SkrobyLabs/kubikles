//go:build !accelerator && (headless || !helm)

package main

import "kubikles/pkg/acceleratorprovision"

// The unavailable/headless seam remains side-effect free and unwired.
func newDesktopAcceleratorDisposer(*acceleratorprovision.Service) *acceleratorprovision.DisposalService {
	return acceleratorprovision.NewDisposalService(nil)
}
