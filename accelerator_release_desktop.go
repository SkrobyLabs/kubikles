//go:build !headless

package main

import "kubikles/pkg/acceleratorrelease"

var desktopAcceleratorReleaseResolverFactory = acceleratorrelease.New

// newDesktopAcceleratorReleaseResolver is deliberately dormant until lifecycle composition.
func newDesktopAcceleratorReleaseResolver() *acceleratorrelease.Resolver {
	return desktopAcceleratorReleaseResolverFactory(BuildVersion)
}
