// Package acceleratorprovision owns one fresh, memory-only Accelerator
// workload attempt. It deliberately has no App, UI, logging, or persistence.
package acceleratorprovision

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"kubikles/pkg/acceleratorrelease"
)

type Availability string

const (
	Available   Availability = "available"
	Unavailable Availability = "unavailable"
)

type UnavailableReason string

const (
	ArtifactUnavailable  UnavailableReason = "artifact_unavailable"
	ContextUnavailable   UnavailableReason = "context_unavailable"
	ContextChanged       UnavailableReason = "context_changed"
	EntropyUnavailable   UnavailableReason = "entropy_unavailable"
	ChartPullFailed      UnavailableReason = "chart_pull_failed"
	ChartIntegrityFailed UnavailableReason = "chart_integrity_failed"
	RenderFailed         UnavailableReason = "render_failed"
	ReleaseConflict      UnavailableReason = "release_conflict"
	PermissionDenied     UnavailableReason = "permission_denied"
	InstallFailed        UnavailableReason = "install_failed"
	JobFailed            UnavailableReason = "job_failed"
	PodFailed            UnavailableReason = "pod_failed"
	ImagePullFailed      UnavailableReason = "image_pull_failed"
	TimedOut             UnavailableReason = "timeout"
	Cancelled            UnavailableReason = "cancelled"
)

type CleanupStatus string

const (
	CleanupNotNeeded         CleanupStatus = "not_needed"
	CleanupSucceeded         CleanupStatus = "succeeded"
	CleanupFailed            CleanupStatus = "failed"
	CleanupOwnershipUnproven CleanupStatus = "ownership_unproven"
)

type ObjectIdentity struct {
	Name string `json:"name"`
	UID  string `json:"uid"`
}
type Request struct {
	ContextName string
	Resolution  acceleratorrelease.Resolution
}
type ProvisionedWorkload struct {
	ContextName       string         `json:"contextName"`
	ReleaseNamespace  string         `json:"releaseNamespace"`
	ReleaseName       string         `json:"releaseName"`
	WorkloadSessionID string         `json:"workloadSessionId"`
	Job               ObjectIdentity `json:"job"`
	Pod               ObjectIdentity `json:"pod"`
	BuildVersion      string         `json:"buildVersion"`
	ImageDigest       string         `json:"imageDigest"`
	ChartDigest       string         `json:"chartDigest"`
	credential        *creatorCredential
	snapshot          ContextSnapshot
	connectorState    *workloadConnectorState
}

// workloadReceipt is deliberately private: public fields are display-only and
// must not become authority for a later connection.
type workloadReceipt struct {
	contextName, releaseNamespace, releaseName, workloadSessionID string
	job, pod                                                      ObjectIdentity
	buildVersion, imageDigest, chartDigest                        string
	snapshot                                                      ContextSnapshot
	owner                                                         *ProvisionedWorkload
}

func (r *workloadReceipt) workload() *ProvisionedWorkload {
	if r == nil {
		return nil
	}
	return &ProvisionedWorkload{
		ContextName: r.contextName, ReleaseNamespace: r.releaseNamespace, ReleaseName: r.releaseName,
		WorkloadSessionID: r.workloadSessionID, Job: r.job, Pod: r.pod, BuildVersion: r.buildVersion,
		ImageDigest: r.imageDigest, ChartDigest: r.chartDigest,
	}
}

type workloadConnectorState struct {
	mu                  sync.Mutex
	closed              bool
	active              int
	freshConnectClaimed bool
	receipt             *workloadReceipt
}

// connectorLease deliberately keeps the provisioning-only inputs private.  A
// connector cannot be reconstructed from the safe receipt fields.
type connectorLease struct {
	credential *creatorCredential
	snapshot   ContextSnapshot
	receipt    *workloadReceipt
	release    func()
}

func (w *ProvisionedWorkload) connectorLease() (connectorLease, bool) {
	if w == nil {
		return connectorLease{}, false
	}
	state := w.connectorState
	if state == nil {
		return connectorLease{}, false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.closed || state.freshConnectClaimed || state.receipt == nil || state.receipt.owner != w || w.credential == nil || w.snapshot == nil || !matchesReceipt(w, state.receipt) {
		return connectorLease{}, false
	}
	state.freshConnectClaimed = true
	state.active++
	var once sync.Once
	return connectorLease{credential: w.credential, snapshot: state.receipt.snapshot, receipt: state.receipt, release: func() {
		once.Do(func() {
			state.mu.Lock()
			state.active--
			state.mu.Unlock()
		})
	}}, true
}

func matchesReceipt(w *ProvisionedWorkload, r *workloadReceipt) bool {
	return w != nil && r != nil && w.ContextName == r.contextName && w.ReleaseNamespace == r.releaseNamespace &&
		w.ReleaseName == r.releaseName && w.WorkloadSessionID == r.workloadSessionID && w.Job == r.job && w.Pod == r.pod &&
		w.BuildVersion == r.buildVersion && w.ImageDigest == r.imageDigest && w.ChartDigest == r.chartDigest
}

func (w ProvisionedWorkload) String() string { return "<accelerator workload>" }
func (w ProvisionedWorkload) Format(s fmt.State, _ rune) {
	_, _ = io.WriteString(s, "<accelerator workload>")
}
func (w ProvisionedWorkload) MarshalJSON() ([]byte, error) {
	type safe struct {
		ContextName       string         `json:"contextName"`
		ReleaseNamespace  string         `json:"releaseNamespace"`
		ReleaseName       string         `json:"releaseName"`
		WorkloadSessionID string         `json:"workloadSessionId"`
		Job               ObjectIdentity `json:"job"`
		Pod               ObjectIdentity `json:"pod"`
		BuildVersion      string         `json:"buildVersion"`
		ImageDigest       string         `json:"imageDigest"`
		ChartDigest       string         `json:"chartDigest"`
	}
	return json.Marshal(safe{w.ContextName, w.ReleaseNamespace, w.ReleaseName, w.WorkloadSessionID, w.Job, w.Pod, w.BuildVersion, w.ImageDigest, w.ChartDigest})
}

type Result struct {
	Availability Availability         `json:"availability"`
	Reason       UnavailableReason    `json:"reason,omitempty"`
	Cleanup      CleanupStatus        `json:"cleanup"`
	Workload     *ProvisionedWorkload `json:"workload,omitempty"`
}

func (r Result) String() string { return "<accelerator provision result>" }
func (r Result) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "<accelerator provision result>")
}

func unavailable(reason UnavailableReason, cleanup CleanupStatus) Result {
	return Result{Availability: Unavailable, Reason: reason, Cleanup: cleanup}
}
func available(w *ProvisionedWorkload) Result {
	return Result{Availability: Available, Cleanup: CleanupNotNeeded, Workload: w}
}
