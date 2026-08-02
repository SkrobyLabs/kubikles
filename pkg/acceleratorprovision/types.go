// Package acceleratorprovision owns one fresh, memory-only Accelerator
// workload attempt. It deliberately has no App, UI, logging, or persistence.
package acceleratorprovision

import (
	"encoding/json"
	"fmt"
	"io"

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
