package acceleratorprovision

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"kubikles/pkg/debug"
	"kubikles/pkg/helm"
	"kubikles/pkg/k8s"
)

const (
	SweepTimeout           = 2 * time.Minute
	SweepReleaseLimit      = 100
	SweepReleaseProbeLimit = 101
	SweepCandidateLimit    = 20
	SweepSecretScanLimit   = 10000
	SweepSecretPageSize    = 500
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
	Namespace   string               `json:"namespace,omitempty"`
	Status      SweepCandidateStatus `json:"status"`
}

type sweepCandidate struct {
	namespace string
	name      string
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

func (s *DisposalService) SweepInert(ctx context.Context, snapshot ContextSnapshot) (result SweepResult) {
	if s == nil || s.sweeper == nil || s.acceptSweepSnapshot == nil || ctx == nil || snapshot == nil || !s.acceptSweepSnapshot(snapshot) || snapshot.Identity() == "" || snapshot.Namespace() == "" || snapshot.RESTConfig() == nil || snapshot.Clientset() == nil {
		return SweepResult{Status: SweepInvalidSnapshot}
	}
	logScope := acceleratorSweepLogScope(snapshot)
	debug.LogHelm("Accelerator stale release sweep started", logScope)
	defer func() {
		details := acceleratorSweepLogScope(snapshot)
		details["status"] = result.Status
		details["candidateCount"] = len(result.Candidates)
		cleaned := 0
		for _, candidate := range result.Candidates {
			if candidate.Status == SweepCleaned {
				cleaned++
			}
		}
		details["cleanedCount"] = cleaned
		debug.LogHelm("Accelerator stale release sweep finished", details)
	}()
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
	result = SweepResult{Status: SweepCompleted, Candidates: make([]SweepCandidateResult, 0, len(candidates))}
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
		details := acceleratorSweepLogScope(snapshot)
		details["releaseName"] = name
		details["status"] = status
		switch status {
		case SweepCleaned:
			debug.LogHelm("Kubikles removed stale Accelerator release", details)
		case SweepAlreadyGone:
			debug.LogHelm("Accelerator stale release was already absent", details)
		case SweepCandidateCleanupFailed:
			debug.LogHelm("Kubikles failed to remove stale Accelerator release", details)
		default:
			debug.LogHelm("Accelerator stale release sweep retained candidate", details)
		}
	}
	return result
}

// SweepAllInert removes every provably inactive Kubikles Accelerator release
// visible in the current cluster, regardless of its release namespace. Active
// or ambiguous releases are deliberately retained so another desktop session
// cannot be disrupted.
func (s *DisposalService) SweepAllInert(ctx context.Context, snapshot ContextSnapshot) (result SweepResult) {
	if s == nil || s.sweeper == nil || s.acceptSweepSnapshot == nil || ctx == nil || snapshot == nil || !s.acceptSweepSnapshot(snapshot) || snapshot.Identity() == "" || snapshot.RESTConfig() == nil || snapshot.Clientset() == nil {
		return SweepResult{Status: SweepInvalidSnapshot}
	}
	logScope := acceleratorSweepLogScope(snapshot)
	debug.LogHelm("Accelerator all-namespace stale release sweep started", logScope)
	defer func() {
		details := acceleratorSweepLogScope(snapshot)
		details["status"] = result.Status
		details["candidateCount"] = len(result.Candidates)
		cleaned := 0
		for _, candidate := range result.Candidates {
			if candidate.Status == SweepCleaned {
				cleaned++
			}
		}
		details["cleanedCount"] = cleaned
		debug.LogHelm("Accelerator all-namespace stale release sweep finished", details)
	}()

	op, cancel := s.withPhaseTimeout(ctx, SweepTimeout)
	defer cancel()
	candidates, status := listAllAcceleratorSweepCandidates(op, snapshot)
	if status != SweepCompleted {
		return SweepResult{Status: status}
	}
	result = SweepResult{Status: SweepCompleted, Candidates: make([]SweepCandidateResult, 0, len(candidates))}
	for _, candidate := range candidates {
		if op.Err() != nil {
			result.Status = SweepTimedOut
			return result
		}
		target := namespaceOverrideSnapshot{ContextSnapshot: snapshot, namespace: candidate.namespace}
		releaseGate := s.gates.acquire(op, releaseMutationGateKey(target))
		if releaseGate == nil {
			result.Status = SweepTimedOut
			return result
		}
		if op.Err() != nil {
			releaseGate()
			result.Status = SweepTimedOut
			return result
		}
		proofCandidate, proof := s.sweeper.InspectAcceleratorSweepCandidate(op, snapshot.RESTConfig(), candidate.namespace, candidate.name)
		candidateStatus := mapSweepProof(proof)
		if proof == helm.AcceleratorSweepEligible {
			if op.Err() != nil {
				releaseGate()
				result.Status = SweepTimedOut
				return result
			}
			proof = s.sweeper.CleanupAcceleratorSweepCandidate(op, snapshot.RESTConfig(), proofCandidate)
			candidateStatus = mapSweepProof(proof)
		}
		releaseGate()
		result.Candidates = append(result.Candidates, SweepCandidateResult{ReleaseName: candidate.name, Namespace: candidate.namespace, Status: candidateStatus})
		details := acceleratorSweepLogScope(target)
		details["releaseName"] = candidate.name
		details["status"] = candidateStatus
		switch candidateStatus {
		case SweepCleaned:
			debug.LogHelm("Kubikles removed stale Accelerator release", details)
		case SweepAlreadyGone:
			debug.LogHelm("Accelerator stale release was already absent", details)
		case SweepCandidateCleanupFailed:
			debug.LogHelm("Kubikles failed to remove stale Accelerator release", details)
		default:
			debug.LogHelm("Accelerator stale release sweep retained candidate", details)
		}
	}
	return result
}

func listAllAcceleratorSweepCandidates(ctx context.Context, snapshot ContextSnapshot) ([]sweepCandidate, SweepStatus) {
	if ctx == nil || snapshot == nil || snapshot.Clientset() == nil {
		return nil, SweepInvalidSnapshot
	}
	candidates := make(map[string]sweepCandidate)
	seen := 0
	options := metav1.ListOptions{LabelSelector: "owner=helm", Limit: SweepSecretPageSize}
	for {
		secrets, err := snapshot.Clientset().CoreV1().Secrets("").List(ctx, options)
		if err != nil {
			if ctx.Err() != nil {
				return nil, SweepTimedOut
			}
			return nil, SweepFailed
		}
		seen += len(secrets.Items)
		if seen > SweepSecretScanLimit {
			return nil, SweepBoundedLimit
		}
		for _, secret := range secrets.Items {
			name := secret.Labels["name"]
			if secret.Labels["version"] != "1" || !sweepReleaseNameRE.MatchString(name) || secret.Namespace == "" {
				continue
			}
			key := secret.Namespace + "\x00" + name
			candidates[key] = sweepCandidate{namespace: secret.Namespace, name: name}
			if len(candidates) > SweepCandidateLimit {
				return nil, SweepBoundedLimit
			}
		}
		if secrets.Continue == "" {
			break
		}
		options.Continue = secrets.Continue
	}
	result := make([]sweepCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		result = append(result, candidate)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].namespace == result[j].namespace {
			return result[i].name < result[j].name
		}
		return result[i].namespace < result[j].namespace
	})
	return result, SweepCompleted
}

func acceleratorSweepLogScope(snapshot ContextSnapshot) map[string]interface{} {
	details := map[string]interface{}{"namespace": snapshot.Namespace()}
	if named, ok := snapshot.(interface{ ContextName() string }); ok {
		if contextName := named.ContextName(); contextName != "" {
			details["context"] = contextName
		}
	}
	return details
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
