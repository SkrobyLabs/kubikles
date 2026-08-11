package helm

import (
	"fmt"
	"io"

	"helm.sh/helm/v3/pkg/chart"
	"k8s.io/apimachinery/pkg/types"
)

const AcceleratorOwnerLabel = "kubikles.io/owner-id"

// AcceleratorFailure is a closed, value-free classification used by the
// provisioning service. Registry, Helm, manifest, and Kubernetes errors never
// cross this boundary.
type AcceleratorFailure string

const (
	AcceleratorOK         AcceleratorFailure = ""
	AcceleratorPull       AcceleratorFailure = "pull_failed"
	AcceleratorIntegrity  AcceleratorFailure = "integrity_failed"
	AcceleratorRender     AcceleratorFailure = "render_failed"
	AcceleratorConflict   AcceleratorFailure = "release_conflict"
	AcceleratorPermission AcceleratorFailure = "permission_denied"
	AcceleratorInstall    AcceleratorFailure = "install_failed"
	AcceleratorCleanup    AcceleratorFailure = "cleanup_failed"
)

type AcceleratorReleaseRequest struct {
	InstallationID       string
	ChartReference       string
	ChartDigest          string
	BuildVersion         string
	ImageReference       string
	ImageRepository      string
	ImageDigest          string
	AllowVersionMismatch bool
	WorkloadSession      string
	CreatorVerifier      string
	ReleaseName          string
	ReleaseNamespace     string
}

// AcceleratorReleaseRequest can contain the verifier required by Helm, so it
// must never acquire the default struct formatter or JSON representation.
func (r AcceleratorReleaseRequest) String() string { return "<accelerator release request>" }
func (r AcceleratorReleaseRequest) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "<accelerator release request>")
}
func (r AcceleratorReleaseRequest) MarshalJSON() ([]byte, error) {
	return nil, fmt.Errorf("accelerator release request serialization is disabled")
}

// AcceleratorResourceIdentity is the exact rendered identity used for
// ownership inspection. Namespace is empty only for cluster-scoped objects.
type AcceleratorResourceIdentity struct {
	APIVersion string
	Kind       string
	Namespace  string
	Name       string
}

// AcceleratorDeletionIdentity is an immutable record of one Kubernetes object
// whose Create returned successfully during this exact install attempt. The UID
// is a mandatory deletion precondition, never a selector.
type AcceleratorDeletionIdentity struct {
	Resource AcceleratorResourceIdentity
	UID      types.UID
}

type AcceleratorStorageIdentity struct {
	Namespace string
	Name      string
	UID       types.UID
}

type AcceleratorOwnedCleanupStatus string

const (
	AcceleratorOwnedCleanupSucceeded        AcceleratorOwnedCleanupStatus = "succeeded"
	AcceleratorOwnedCleanupAlreadyGone      AcceleratorOwnedCleanupStatus = "already_gone"
	AcceleratorOwnedCleanupOwnershipChanged AcceleratorOwnedCleanupStatus = "ownership_changed"
	AcceleratorOwnedCleanupFailed           AcceleratorOwnedCleanupStatus = "failed"
)

type AcceleratorSweepProofStatus string

const (
	AcceleratorSweepEligible             AcceleratorSweepProofStatus = "eligible"
	AcceleratorSweepActiveOrAmbiguous    AcceleratorSweepProofStatus = "active_or_ambiguous"
	AcceleratorSweepUnsupportedMalformed AcceleratorSweepProofStatus = "unsupported_or_malformed"
	AcceleratorSweepAlreadyGone          AcceleratorSweepProofStatus = "already_gone"
	AcceleratorSweepOwnershipChanged     AcceleratorSweepProofStatus = "ownership_changed"
	AcceleratorSweepCleanupFailed        AcceleratorSweepProofStatus = "cleanup_failed"
)

type AcceleratorSweepCandidate struct {
	name, namespace, session, installationID, renderHash string
	storage                                              AcceleratorStorageIdentity
	resources                                            []AcceleratorDeletionIdentity
	pods                                                 []AcceleratorDeletionIdentity
	authority                                            *acceleratorSweepAuthority
}

// acceleratorSweepAuthority is an unforgeable package-private capability.
// Candidate values constructed outside the closed inspector never reach I/O.
type acceleratorSweepAuthority struct{}

var trustedAcceleratorSweepAuthority = &acceleratorSweepAuthority{}

func (c AcceleratorSweepCandidate) String() string { return "<accelerator sweep candidate>" }
func (c AcceleratorSweepCandidate) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "<accelerator sweep candidate>")
}

// AcceleratorPreparedRelease is an opaque validated chart/render receipt.
// It deliberately exposes no manifest, values, verifier, or Helm release.
type AcceleratorPreparedRelease struct {
	request      AcceleratorReleaseRequest
	chart        *chart.Chart
	values       map[string]interface{}
	manifest     string
	renderHash   string
	resources    []AcceleratorResourceIdentity
	jobName      string
	verifierName string
}

func (p AcceleratorPreparedRelease) String() string { return "<accelerator prepared release>" }
func (p AcceleratorPreparedRelease) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "<accelerator prepared release>")
}
func (p *AcceleratorPreparedRelease) JobName() string {
	if p == nil {
		return ""
	}
	return p.jobName
}
func (p *AcceleratorPreparedRelease) ResourceIdentities() []AcceleratorResourceIdentity {
	if p == nil {
		return nil
	}
	return append([]AcceleratorResourceIdentity(nil), p.resources...)
}

// AcceleratorOwnershipReceipt is created only after exact revision/config/
// manifest proof. It is the sole authority accepted by strict uninstall.
type AcceleratorOwnershipReceipt struct {
	request                AcceleratorReleaseRequest
	renderHash             string
	storageName            string
	storageUID             types.UID
	storageResourceVersion string
	created                []AcceleratorDeletionIdentity
	unresolved             []AcceleratorResourceIdentity
}

// UnresolvedResources returns identities whose Create request was dispatched
// but whose server outcome was not acknowledged. They are never deletion
// authority.
func (r *AcceleratorOwnershipReceipt) UnresolvedResources() []AcceleratorResourceIdentity {
	if r == nil {
		return nil
	}
	return append([]AcceleratorResourceIdentity(nil), r.unresolved...)
}

// CreatedResources returns a copy of the exact successful Create prefix.
func (r *AcceleratorOwnershipReceipt) CreatedResources() []AcceleratorDeletionIdentity {
	if r == nil {
		return nil
	}
	return append([]AcceleratorDeletionIdentity(nil), r.created...)
}

func (r *AcceleratorOwnershipReceipt) StorageIdentity() AcceleratorStorageIdentity {
	if r == nil {
		return AcceleratorStorageIdentity{}
	}
	return AcceleratorStorageIdentity{Namespace: r.request.ReleaseNamespace, Name: r.storageName, UID: r.storageUID}
}

func (r AcceleratorOwnershipReceipt) String() string { return "<accelerator ownership receipt>" }
func (r AcceleratorOwnershipReceipt) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "<accelerator ownership receipt>")
}
