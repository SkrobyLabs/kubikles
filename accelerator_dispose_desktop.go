//go:build !headless && helm && !accelerator

package main

import "kubikles/pkg/acceleratorprovision"

// newDesktopAcceleratorDisposer is a dormant composition seam. It shares the
// provisioner's exact-context mutation gates and has no automatic call site;
// lifecycle policy remains owned by the later coordinator.
func newDesktopAcceleratorDisposer(provisioner *acceleratorprovision.Service) *acceleratorprovision.DisposalService {
	return acceleratorprovision.NewDisposalService(provisioner)
}
