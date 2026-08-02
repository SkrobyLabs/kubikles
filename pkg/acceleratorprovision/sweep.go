package acceleratorprovision

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"time"

	"k8s.io/client-go/rest"
	"kubikles/pkg/helm"
	"kubikles/pkg/k8s"
)

const (
	SweepTimeout           = 2 * time.Minute
	SweepReleaseLimit      = 100
	SweepReleaseProbeLimit = 101
	SweepCandidateLimit    = 20
)

var sweepReleaseNameRE = regexp.MustCompile(`^kubikles-accelerator-[0-9a-f]{32}$`)

func trustedProductionSweepSnapshot(snapshot ContextSnapshot) bool {
	_, ok := snapshot.(*k8s.AcceleratorContextSnapshot)
	return ok
}

type SweepStatus string

const (
	SweepCompleted       SweepStatus = "completed"
	SweepBoundedLimit    SweepStatus = "bounded_limit"
	SweepInvalidSnapshot SweepStatus = "invalid_snapshot"
	SweepFailed          SweepStatus = "failed"
	SweepTimedOut        SweepStatus = "timeout"
)

type SweepCandidateStatus string

const (
	SweepCleaned                SweepCandidateStatus = "cleaned"
	SweepActiveOrAmbiguous      SweepCandidateStatus = "active_or_ambiguous"
	SweepUnsupportedMalformed   SweepCandidateStatus = "unsupported_or_malformed"
	SweepAlreadyGone            SweepCandidateStatus = "already_gone"
	SweepOwnershipChanged       SweepCandidateStatus = "ownership_changed"
	SweepCandidateCleanupFailed SweepCandidateStatus = "cleanup_failed"
)

type SweepCandidateResult struct {
	ReleaseName string               `json:"releaseName"`
	Status      SweepCandidateStatus `json:"status"`
}

type SweepResult struct {
	Status     SweepStatus            `json:"status"`
	Candidates []SweepCandidateResult `json:"candidates,omitempty"`
}

func (SweepResult) String() string { return "<accelerator sweep result>" }
func (SweepResult) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "<accelerator sweep result>")
}
func (r SweepResult) MarshalJSON() ([]byte, error) {
	type safe SweepResult
	return json.Marshal(safe(r))
}

type acceleratorInertSweeper interface {
	ListAcceleratorSweepReleaseNames(context.Context, *rest.Config, string) ([]string, bool)
	InspectAcceleratorSweepCandidate(context.Context, *rest.Config, string, string) (*helm.AcceleratorSweepCandidate, helm.AcceleratorSweepProofStatus)
	CleanupAcceleratorSweepCandidate(context.Context, *rest.Config, *helm.AcceleratorSweepCandidate) helm.AcceleratorSweepProofStatus
}

func (s *DisposalService) SweepInert(ctx context.Context, snapshot ContextSnapshot) SweepResult {
	if s == nil || s.sweeper == nil || s.acceptSweepSnapshot == nil || ctx == nil || snapshot == nil || !s.acceptSweepSnapshot(snapshot) || snapshot.Identity() == "" || snapshot.Namespace() == "" || snapshot.RESTConfig() == nil || snapshot.Clientset() == nil {
		return SweepResult{Status: SweepInvalidSnapshot}
	}
	op, cancel := s.withPhaseTimeout(ctx, SweepTimeout)
	defer cancel()
	names, ok := s.sweeper.ListAcceleratorSweepReleaseNames(op, snapshot.RESTConfig(), snapshot.Namespace())
	if !ok {
		if op.Err() != nil {
			return SweepResult{Status: SweepTimedOut}
		}
		return SweepResult{Status: SweepFailed}
	}
	if len(names) > SweepReleaseLimit {
		return SweepResult{Status: SweepBoundedLimit}
	}
	sort.Strings(names)
	candidates := make([]string, 0, SweepCandidateLimit+1)
	for _, name := range names {
		if sweepReleaseNameRE.MatchString(name) {
			candidates = append(candidates, name)
			if len(candidates) > SweepCandidateLimit {
				return SweepResult{Status: SweepBoundedLimit}
			}
		}
	}
	result := SweepResult{Status: SweepCompleted, Candidates: make([]SweepCandidateResult, 0, len(candidates))}
	for _, name := range candidates {
		if op.Err() != nil {
			result.Status = SweepTimedOut
			return result
		}
		releaseGate := s.gates.acquire(op, releaseMutationGateKey(snapshot))
		if releaseGate == nil {
			result.Status = SweepTimedOut
			return result
		}
		if op.Err() != nil {
			releaseGate()
			result.Status = SweepTimedOut
			return result
		}
		candidate, proof := s.sweeper.InspectAcceleratorSweepCandidate(op, snapshot.RESTConfig(), snapshot.Namespace(), name)
		status := mapSweepProof(proof)
		if proof == helm.AcceleratorSweepEligible {
			if op.Err() != nil {
				releaseGate()
				result.Status = SweepTimedOut
				return result
			}
			// Cleanup performs the explicit second proof on op, checks op again,
			// then (and only then) detaches its bounded uninstall+absence phase.
			proof = s.sweeper.CleanupAcceleratorSweepCandidate(op, snapshot.RESTConfig(), candidate)
			status = mapSweepProof(proof)
		}
		releaseGate()
		result.Candidates = append(result.Candidates, SweepCandidateResult{ReleaseName: name, Status: status})
	}
	return result
}

func mapSweepProof(status helm.AcceleratorSweepProofStatus) SweepCandidateStatus {
	switch status {
	case helm.AcceleratorSweepEligible:
		return SweepCleaned
	case helm.AcceleratorSweepActiveOrAmbiguous:
		return SweepActiveOrAmbiguous
	case helm.AcceleratorSweepAlreadyGone:
		return SweepAlreadyGone
	case helm.AcceleratorSweepOwnershipChanged:
		return SweepOwnershipChanged
	case helm.AcceleratorSweepCleanupFailed:
		return SweepCandidateCleanupFailed
	default:
		return SweepUnsupportedMalformed
	}
}
