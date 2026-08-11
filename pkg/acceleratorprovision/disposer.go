package acceleratorprovision

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"kubikles/pkg/agent"
)

const (
	TransportQuiesceTimeout = 45 * time.Second
	ChartTerminationGrace   = 30 * time.Second
	DrainObservationMargin  = 15 * time.Second
	DrainObservationTimeout = agent.AcceleratorIdleReconnectGrace + ChartTerminationGrace + DrainObservationMargin
	OwnedCleanupTimeout     = 45 * time.Second
	CleanupPollInterval     = 250 * time.Millisecond
)

type DisposalOperation string

const (
	DisposalDrain               DisposalOperation = "drain"
	DisposalImmediate           DisposalOperation = "immediate"
	DisposalImmediateEscalation DisposalOperation = "immediate_escalation"
)

type DrainObservationStatus string

const (
	DrainSkipped   DrainObservationStatus = "skipped"
	DrainComplete  DrainObservationStatus = "complete"
	DrainFailed    DrainObservationStatus = "failed"
	DrainNotFound  DrainObservationStatus = "not_found"
	DrainChanged   DrainObservationStatus = "identity_changed"
	DrainReadError DrainObservationStatus = "read_failed"
	DrainTimedOut  DrainObservationStatus = "timeout"
	DrainEscalated DrainObservationStatus = "escalated"
)

type QuiescenceStatus string

const (
	QuiescenceSucceeded QuiescenceStatus = "succeeded"
	QuiescenceTimedOut  QuiescenceStatus = "timeout"
)

type CredentialDestructionStatus string

const (
	CredentialDestroyed       CredentialDestructionStatus = "destroyed"
	CredentialDestroyTimedOut CredentialDestructionStatus = "timeout"
)

type OwnershipStatus string

const (
	OwnershipProven      OwnershipStatus = "proven"
	OwnershipChanged     OwnershipStatus = "ownership_changed"
	OwnershipAlreadyGone OwnershipStatus = "already_gone"
	OwnershipUnproven    OwnershipStatus = "unproven"
)

type UninstallStatus string

const (
	UninstallSucceeded UninstallStatus = "succeeded"
	UninstallNotNeeded UninstallStatus = "not_needed"
	UninstallFailed    UninstallStatus = "failed"
)

type DisappearanceStatus string

const (
	DisappearanceSucceeded          DisappearanceStatus = "succeeded"
	DisappearanceUIDReplaced        DisappearanceStatus = "uid_replaced"
	DisappearanceResourcesRemaining DisappearanceStatus = "resources_remaining"
	DisappearanceNotChecked         DisappearanceStatus = "not_checked"
)

type DisposalResult struct {
	Requested     DisposalOperation           `json:"requested"`
	Effective     DisposalOperation           `json:"effective"`
	Observation   DrainObservationStatus      `json:"observation"`
	Quiescence    QuiescenceStatus            `json:"quiescence"`
	Credential    CredentialDestructionStatus `json:"credential"`
	Ownership     OwnershipStatus             `json:"ownership"`
	Uninstall     UninstallStatus             `json:"uninstall"`
	Disappearance DisappearanceStatus         `json:"disappearance"`
}

func (DisposalResult) String() string { return "<accelerator disposal result>" }
func (DisposalResult) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "<accelerator disposal result>")
}
func (r DisposalResult) MarshalJSON() ([]byte, error) {
	type safe DisposalResult
	return json.Marshal(safe(r))
}

type disposalOperation struct {
	done               chan struct{}
	force              chan struct{}
	forceOnce          sync.Once
	transportFenced    chan struct{}
	transportFenceOnce sync.Once
	requested          DisposalOperation
	effective          DisposalOperation
	result             DisposalResult
	observationSettled bool
}

func (o *disposalOperation) markTransportFenced() {
	if o != nil && o.transportFenced != nil {
		o.transportFenceOnce.Do(func() { close(o.transportFenced) })
	}
}

type disposalCompletion struct {
	operation *disposalOperation
}

func (c *disposalCompletion) Done() <-chan struct{} {
	if c == nil || c.operation == nil {
		return nil
	}
	return c.operation.done
}

func (c *disposalCompletion) wait() DisposalResult {
	if c == nil || c.operation == nil {
		return invalidDisposal(DisposalImmediate)
	}
	<-c.operation.done
	return c.operation.result
}

func (c *disposalCompletion) waitTransportFenced() {
	if c != nil && c.operation != nil && c.operation.transportFenced != nil {
		<-c.operation.transportFenced
	}
}

type disposalStart struct {
	completion *disposalCompletion
	owner      bool
	state      *workloadConnectorState
	current    *ConnectedSession
	receipt    *workloadReceipt
	cancels    []context.CancelFunc
}

type DisposalService struct {
	gates               *gateSet
	observeDrain        func(context.Context, *workloadReceipt, <-chan struct{}, func(DrainObservationStatus) DrainObservationStatus) DrainObservationStatus
	cleanupOwned        func(context.Context, *workloadReceipt) (OwnershipStatus, UninstallStatus, DisappearanceStatus)
	sweeper             acceleratorInertSweeper
	acceptSweepSnapshot func(ContextSnapshot) bool
	phaseContext        func(context.Context, time.Duration) (context.Context, context.CancelFunc)
	installationOwner   installationOwnerProvider
}

// NewDisposalService creates an explicit, dormant disposal primitive. Passing
// the provisioning service shares its exact-context mutation gate.
func NewDisposalService(provisioner *Service) *DisposalService {
	service := &DisposalService{acceptSweepSnapshot: trustedProductionSweepSnapshot, phaseContext: context.WithTimeout}
	if provisioner != nil {
		service.installationOwner = provisioner.installationOwner
		service.gates = &provisioner.gates
		service.observeDrain = observeExactDrainJobSettled
		if cleaner, ok := provisioner.charts.(interface {
			disposeOwned(context.Context, ContextSnapshot, *preparedChart, *ownedRelease, ObjectIdentity) (OwnershipStatus, UninstallStatus, DisappearanceStatus)
		}); ok {
			service.cleanupOwned = func(ctx context.Context, receipt *workloadReceipt) (OwnershipStatus, UninstallStatus, DisappearanceStatus) {
				if receipt == nil {
					return OwnershipUnproven, UninstallNotNeeded, DisappearanceNotChecked
				}
				return cleaner.disposeOwned(ctx, receipt.snapshot, receipt.prepared, receipt.owned, receipt.pod)
			}
		}
		if adapter, ok := provisioner.charts.(*helmChartInstaller); ok && adapter != nil {
			service.sweeper, _ = adapter.client.(acceleratorInertSweeper)
		}
	}
	return service
}

func (s *DisposalService) DrainAndDispose(ctx context.Context, workload *ProvisionedWorkload) DisposalResult {
	return s.dispose(ctx, workload, DisposalDrain)
}

func (s *DisposalService) DisposeNow(ctx context.Context, workload *ProvisionedWorkload) DisposalResult {
	return s.dispose(ctx, workload, DisposalImmediate)
}

// startDisposeNow is the coordinator-only asynchronous entry point. The
// workload disposal fence is established synchronously; only bounded cleanup
// continues in the returned operation.
func (s *DisposalService) startDisposeNow(ctx context.Context, workload *ProvisionedWorkload) *disposalCompletion {
	start := s.beginDisposal(workload, DisposalImmediate)
	if start.owner {
		go s.executeDisposal(ctx, workload, start)
	}
	return start.completion
}

func invalidDisposal(requested DisposalOperation) DisposalResult {
	return DisposalResult{Requested: requested, Effective: requested, Observation: DrainSkipped, Quiescence: QuiescenceTimedOut, Credential: CredentialDestroyTimedOut, Ownership: OwnershipUnproven, Uninstall: UninstallNotNeeded, Disappearance: DisappearanceNotChecked}
}

func (s *DisposalService) dispose(ctx context.Context, workload *ProvisionedWorkload, requested DisposalOperation) DisposalResult {
	start := s.beginDisposal(workload, requested)
	if start.owner {
		s.executeDisposal(ctx, workload, start)
	}
	return start.completion.wait()
}

func completedInvalidDisposal(requested DisposalOperation) disposalStart {
	op := &disposalOperation{done: make(chan struct{}), requested: requested, effective: requested, result: invalidDisposal(requested)}
	close(op.done)
	return disposalStart{completion: &disposalCompletion{operation: op}}
}

func (s *DisposalService) beginDisposal(workload *ProvisionedWorkload, requested DisposalOperation) disposalStart {
	return s.beginDisposalAuthorized(workload, requested)
}

func (s *DisposalService) beginDisposalAuthorized(workload *ProvisionedWorkload, requested DisposalOperation) disposalStart {
	if s == nil || workload == nil || workload.connectorState == nil {
		return completedInvalidDisposal(requested)
	}
	state := workload.connectorState
	state.mu.Lock()
	if existing := state.disposal; existing != nil {
		completed := false
		select {
		case <-existing.done:
			completed = true
		default:
		}
		if !completed && requested == DisposalImmediate && existing.effective == DisposalDrain && !existing.observationSettled {
			existing.effective = DisposalImmediateEscalation
			existing.forceOnce.Do(func() { close(existing.force) })
		}
		state.mu.Unlock()
		if !completed && requested == DisposalImmediate {
			existing.markTransportFenced()
		}
		return disposalStart{completion: &disposalCompletion{operation: existing}}
	}
	if workload.credential == nil {
		state.mu.Unlock()
		return completedInvalidDisposal(requested)
	}
	op := &disposalOperation{done: make(chan struct{}), force: make(chan struct{}), transportFenced: make(chan struct{}), requested: requested, effective: requested}
	state.disposal = op
	state.disposing = true
	state.closed = true
	cancels := make([]context.CancelFunc, 0, len(state.operations))
	for _, cancel := range state.operations {
		cancels = append(cancels, cancel)
	}
	current := state.currentSession
	receipt := state.receipt
	state.signalChangedLocked()
	state.mu.Unlock()
	op.markTransportFenced()
	return disposalStart{completion: &disposalCompletion{operation: op}, owner: true, state: state, current: current, receipt: receipt, cancels: cancels}
}

func (s *DisposalService) executeDisposal(ctx context.Context, workload *ProvisionedWorkload, start disposalStart) {
	op, state, current, receipt := start.completion.operation, start.state, start.current, start.receipt
	for _, cancel := range start.cancels {
		cancel()
	}
	base := context.Background()
	if ctx != nil {
		base = context.WithoutCancel(ctx)
	}
	quiesceCtx, cancelQuiesce := s.withPhaseTimeout(base, TransportQuiesceTimeout)
	quiescence := QuiescenceSucceeded
	if current != nil && current.Close(quiesceCtx) != nil {
		quiescence = QuiescenceTimedOut
	}
	op.markTransportFenced()
	if !waitLifecycleIdle(quiesceCtx, state) {
		quiescence = QuiescenceTimedOut
	}
	credential := CredentialDestroyed
	if !workload.credential.closeAndDestroy(quiesceCtx) {
		credential = CredentialDestroyTimedOut
	}
	cancelQuiesce()

	observation := DrainSkipped
	state.mu.Lock()
	effective := op.effective
	state.mu.Unlock()
	if effective == DisposalDrain && s.observeDrain != nil {
		observeCtx, cancelObserve := s.withPhaseTimeout(base, DrainObservationTimeout)
		observation = s.observeDrain(observeCtx, receipt, op.force, func(status DrainObservationStatus) DrainObservationStatus {
			state.mu.Lock()
			defer state.mu.Unlock()
			if op.observationSettled {
				return observation
			}
			if op.effective != DisposalDrain {
				status = DrainEscalated
			}
			op.observationSettled = true
			observation = status
			return status
		})
		cancelObserve()
		state.mu.Lock()
		if !op.observationSettled {
			if op.effective != DisposalDrain {
				observation = DrainEscalated
			}
			op.observationSettled = true
		}
		state.mu.Unlock()
	} else if effective == DisposalImmediateEscalation {
		observation = DrainEscalated
	}

	ownership, uninstall, disappearance := OwnershipUnproven, UninstallNotNeeded, DisappearanceNotChecked
	cleanupCtx, cancelCleanup := s.withPhaseTimeout(base, OwnedCleanupTimeout)
	gateRelease := func() {}
	if s.gates != nil && receipt != nil && receipt.snapshot != nil {
		if acquired := s.gates.acquire(cleanupCtx, releaseMutationGateKey(receipt.snapshot)); acquired != nil {
			gateRelease = acquired
		} else {
			cleanupCtx = cancelledContext()
		}
	}
	if s.cleanupOwned != nil && cleanupCtx.Err() == nil {
		ownership, uninstall, disappearance = s.cleanupOwned(cleanupCtx, receipt)
	}
	gateRelease()
	cancelCleanup()

	state.mu.Lock()
	result := DisposalResult{Requested: op.requested, Effective: op.effective, Observation: observation, Quiescence: quiescence, Credential: credential, Ownership: ownership, Uninstall: uninstall, Disappearance: disappearance}
	op.result = result
	op.markTransportFenced()
	state.currentSession = nil
	state.receipt = nil
	workload.credential = nil
	workload.snapshot = nil
	close(op.done)
	state.signalChangedLocked()
	state.mu.Unlock()
}

func (s *DisposalService) withPhaseTimeout(parent context.Context, duration time.Duration) (context.Context, context.CancelFunc) {
	if s != nil && s.phaseContext != nil {
		return s.phaseContext(parent, duration)
	}
	return context.WithTimeout(parent, duration)
}

func cancelledContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func waitLifecycleIdle(ctx context.Context, state *workloadConnectorState) bool {
	for {
		state.mu.Lock()
		if len(state.operations) == 0 && state.active == 0 {
			state.mu.Unlock()
			return true
		}
		changed := state.changed
		if changed == nil {
			state.changed = make(chan struct{})
			changed = state.changed
		}
		state.mu.Unlock()
		select {
		case <-changed:
		case <-ctx.Done():
			return false
		}
	}
}
