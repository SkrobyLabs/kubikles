//go:build !headless

package main

import (
	"os"
	"path/filepath"

	"kubikles/pkg/acceleratorrelease"
)

var (
	desktopUserConfigDir                     = os.UserConfigDir
	desktopAcceleratorReleaseResolverFactory = acceleratorrelease.New
)

// newDesktopAcceleratorReleaseResolver is deliberately dormant until lifecycle composition.
func newDesktopAcceleratorReleaseResolver() (*acceleratorrelease.Resolver, error) {
	root, err := desktopUserConfigDir()
	if err != nil {
		return nil, err
	}
	return desktopAcceleratorReleaseResolverFactory(BuildVersion, filepath.Join(root, "kubikles", "accelerator", "releases")), nil
}
