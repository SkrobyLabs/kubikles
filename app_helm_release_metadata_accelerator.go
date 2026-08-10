//go:build accelerator

package main

import "kubikles/pkg/acceleratorsecret"

// ListAcceleratorHelmReleaseMetadata returns only the closed Accelerator wire DTO. Raw
// Helm storage Secret payloads are decoded and discarded in-process.
//
//kubikles:dispatch exclude
func (a *App) ListAcceleratorHelmReleaseMetadata(requestID, namespace string) ([]acceleratorsecret.HelmReleaseMetadata, error) {
	return a.listHelmReleaseMetadata(requestID, namespace)
}
