//go:build !helm

package helm

import (
	"context"
	"errors"

	"helm.sh/helm/v3/pkg/chart"
	"k8s.io/client-go/rest"
)

var ErrAcceleratorUnavailable = errors.New("accelerator provisioning unavailable")
var ErrAcceleratorIntegrity = errors.New("accelerator chart integrity failed")

type AcceleratorChartRequest struct {
	Reference, Digest, BuildVersion string
	AllowVersionMismatch            bool
}

func PullAcceleratorChart(context.Context, AcceleratorChartRequest) (*chart.Chart, error) {
	return nil, ErrAcceleratorUnavailable
}

func RenderAcceleratorRelease(*chart.Chart, AcceleratorReleaseRequest) (*AcceleratorPreparedRelease, AcceleratorFailure) {
	return nil, AcceleratorRender
}

func (c *Client) PrepareAcceleratorRelease(context.Context, AcceleratorReleaseRequest) (*AcceleratorPreparedRelease, AcceleratorFailure) {
	return nil, AcceleratorPull
}

func (c *Client) InstallAcceleratorRelease(context.Context, *rest.Config, *AcceleratorPreparedRelease) (*AcceleratorOwnershipReceipt, AcceleratorFailure, bool) {
	return nil, AcceleratorInstall, false
}

func (c *Client) InspectAcceleratorOwnership(context.Context, *rest.Config, *AcceleratorPreparedRelease, *AcceleratorOwnershipReceipt) bool {
	return false
}

func (c *Client) DeleteOwnedAcceleratorResources(context.Context, *rest.Config, *AcceleratorPreparedRelease, *AcceleratorOwnershipReceipt) AcceleratorFailure {
	return AcceleratorCleanup
}

func (c *Client) PurgeOwnedAcceleratorRelease(context.Context, *rest.Config, *AcceleratorPreparedRelease, *AcceleratorOwnershipReceipt) AcceleratorFailure {
	return AcceleratorCleanup
}

func (c *Client) UninstallOwnedAcceleratorRelease(context.Context, *rest.Config, *AcceleratorPreparedRelease, *AcceleratorOwnershipReceipt, AcceleratorDeletionIdentity) AcceleratorOwnedCleanupStatus {
	return AcceleratorOwnedCleanupFailed
}

func (c *Client) ListAcceleratorSweepReleaseNames(context.Context, *rest.Config, string) ([]string, bool) {
	return nil, false
}

func (c *Client) InspectAcceleratorSweepCandidate(context.Context, *rest.Config, string, string) (*AcceleratorSweepCandidate, AcceleratorSweepProofStatus) {
	return nil, AcceleratorSweepUnsupportedMalformed
}

func (c *Client) UninstallAcceleratorSweepCandidate(context.Context, *rest.Config, *AcceleratorSweepCandidate) AcceleratorSweepProofStatus {
	return AcceleratorSweepCleanupFailed
}

func (c *Client) CleanupAcceleratorSweepCandidate(context.Context, *rest.Config, *AcceleratorSweepCandidate) AcceleratorSweepProofStatus {
	return AcceleratorSweepCleanupFailed
}

func (c *Client) WaitAcceleratorSweepCandidateGone(context.Context, *rest.Config, *AcceleratorSweepCandidate) bool {
	return false
}
