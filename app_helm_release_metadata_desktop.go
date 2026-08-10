//go:build !accelerator

package main

import (
	"kubikles/pkg/acceleratorsecret"
	"kubikles/pkg/helm"
)

// ListHelmReleaseMetadata returns only latest release summaries. Raw Helm
// storage Secret payloads are decoded and discarded in the active backend.
func (a *App) ListHelmReleaseMetadata(requestID, namespace string) ([]helm.Release, error) {
	projected, err := a.listHelmReleaseMetadata(requestID, namespace)
	if err != nil {
		return nil, err
	}
	return helmReleasesFromMetadata(projected), nil
}

func helmReleasesFromMetadata(projected []acceleratorsecret.HelmReleaseMetadata) []helm.Release {
	releases := make([]helm.Release, len(projected))
	for index, release := range projected {
		releases[index] = helm.Release{
			Name: release.Name, Namespace: release.Namespace, Revision: release.Revision,
			Status: release.Status, Chart: release.Chart, ChartVersion: release.ChartVersion,
			AppVersion: release.AppVersion, Updated: release.Updated, Description: release.Description,
		}
	}
	return releases
}
