//go:build !headless && helm && !accelerator

package main

import (
	"kubikles/pkg/acceleratorprovision"
	"kubikles/pkg/helm"
	"kubikles/pkg/k8s"
)

// newDesktopAcceleratorProvisioner is deliberately dormant until downstream
// desktop lifecycle composition calls it. It is not stored on App and performs
// no RNG, registry, Helm, or Kubernetes operation during construction.
func newDesktopAcceleratorProvisioner(k8sClient *k8s.Client, helmClient *helm.Client) *acceleratorprovision.Service {
	return acceleratorprovision.NewDesktopService(k8sClient, helmClient)
}
