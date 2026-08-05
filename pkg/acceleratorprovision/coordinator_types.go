package acceleratorprovision

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"kubikles/pkg/acceleratorrelease"
)

const (
	ActivationAttemptTimeout       = 4 * time.Minute
	MaxTransientActivationAttempts = 3
	UnavailableCooldown            = 30 * time.Second
	ShutdownWaitTimeout            = 95 * time.Second
)

var ActivationRetryDelays = [...]time.Duration{time.Second, 2 * time.Second}

type CoordinatorState string

const (
	CoordinatorDirectOnly   CoordinatorState = "direct_only"
	CoordinatorSweeping     CoordinatorState = "sweeping"
	CoordinatorResolving    CoordinatorState = "resolving"
	CoordinatorProvisioning CoordinatorState = "provisioning"
	CoordinatorConnecting   CoordinatorState = "connecting"
	CoordinatorActive       CoordinatorState = "active"
	CoordinatorReconnecting CoordinatorState = "reconnecting"
	CoordinatorDraining     CoordinatorState = "draining"
	CoordinatorDisposing    CoordinatorState = "disposing"
	CoordinatorUnavailable  CoordinatorState = "unavailable"
	CoordinatorClosed       CoordinatorState = "closed"
)

type DemandReason string

const (
	DemandAccepted       DemandReason = "accepted_direct_only"
	DemandWrongContext   DemandReason = "wrong_context"
	DemandSwitching      DemandReason = "context_switching"
	DemandRuntimeClosing DemandReason = "runtime_closing"
)

type DemandResult struct {
	Accepted bool               `json:"accepted"`
	Reason   DemandReason       `json:"reason"`
	Lease    *SecretDemandLease `json:"-"`
}

func (DemandResult) String() string { return "<accelerator demand result>" }
func (DemandResult) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "<accelerator demand result>")
}
func (r DemandResult) MarshalJSON() ([]byte, error) {
	type safe struct {
		Accepted bool         `json:"accepted"`
		Reason   DemandReason `json:"reason"`
	}
	return json.Marshal(safe{Accepted: r.Accepted, Reason: r.Reason})
}

type CoordinatorSnapshot struct {
	State         CoordinatorState        `json:"state"`
	Enabled       bool                    `json:"enabled"`
	Namespace     string                  `json:"namespace"`
	DemandCount   int                     `json:"demandCount"`
	SessionLeases int                     `json:"sessionLeases"`
	Available     bool                    `json:"available"`
	Diagnostics   []CoordinatorDiagnostic `json:"diagnostics,omitempty"`
	// Workload is a defensive display-only projection.  Its private receipt
	// remains the sole authority for connect and disposal operations.
	Workload *ProvisionedWorkload `json:"workload,omitempty"`
}

// CoordinatorDiagnostic is a bounded, display-safe record of a lifecycle
// failure. It deliberately contains only closed reason codes, never raw child
// errors, credentials, Kubernetes objects, or release descriptors.
type CoordinatorDiagnostic struct {
	Timestamp string `json:"timestamp"`
	Phase     string `json:"phase"`
	Reason    string `json:"reason"`
	Attempt   int    `json:"attempt,omitempty"`
}

func (CoordinatorSnapshot) String() string { return "<accelerator coordinator snapshot>" }
func (CoordinatorSnapshot) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "<accelerator coordinator snapshot>")
}

type coordinatorReleaseResolver interface {
	Resolve(context.Context) acceleratorrelease.Resolution
}

type coordinatorProvisioner interface {
	Provision(context.Context, Request) Result
}

type coordinatorConnector interface {
	Connect(context.Context, *ProvisionedWorkload) ConnectResult
}

type coordinatorReconnector interface {
	Resume(context.Context, ResumeRequest) ResumeResult
	resumeIdle(context.Context, ResumeRequest, *coordinatorIdleToken) ResumeResult
}

type coordinatorDisposer interface {
	SweepInert(context.Context, ContextSnapshot) SweepResult
	DisposeNow(context.Context, *ProvisionedWorkload) DisposalResult
	DrainAndDispose(context.Context, *ProvisionedWorkload) DisposalResult
}

type coordinatorAsyncDisposer interface {
	startDisposeNow(context.Context, *ProvisionedWorkload) *disposalCompletion
}

type coordinatorClock interface {
	Now() time.Time
	Sleep(context.Context, time.Duration) error
}

type operationFence struct {
	contextEpoch   uint64
	demandEpoch    uint64
	operationEpoch uint64
}

type contextSlot struct {
	mu           sync.Mutex
	navigationMu sync.Mutex

	contextName    string
	contextEpoch   uint64
	demandEpoch    uint64
	operationEpoch uint64

	state  CoordinatorState
	change chan struct{}

	demandCount         int
	sessionLeaseCount   int
	sessionLeaseEpoch   uint64
	sessionLeaseRevoked chan struct{}
	demandSettled       bool
	enabled             bool
	namespace           string

	sweepAttempted       bool
	mismatchRecreateUsed bool
	replacementAttempt   bool
	mismatchLatched      bool
	unavailableUntil     time.Time

	workerCancel       context.CancelFunc
	workerDone         chan struct{}
	pendingWorkerState CoordinatorState
	pendingWorkerRun   func(context.Context, operationFence)

	workload               *ProvisionedWorkload
	session                *ConnectedSession
	idle                   *coordinatorIdleRelease
	normalEnded            bool
	drainDeadline          time.Time
	terminalCleanupPending bool
	diagnostics            []CoordinatorDiagnostic
}

func newContextSlot(name string, epoch uint64) *contextSlot {
	return &contextSlot{contextName: name, contextEpoch: epoch, demandSettled: true, state: CoordinatorDirectOnly, change: make(chan struct{}), sessionLeaseRevoked: closedSessionLeaseSignal()}
}

func closedSessionLeaseSignal() chan struct{} {
	signal := make(chan struct{})
	close(signal)
	return signal
}

func (s *contextSlot) advanceSessionLeaseEpochLocked(active bool) {
	if s.sessionLeaseRevoked != nil {
		select {
		case <-s.sessionLeaseRevoked:
		default:
			close(s.sessionLeaseRevoked)
		}
	}
	s.sessionLeaseEpoch++
	s.sessionLeaseCount = 0
	if active {
		s.sessionLeaseRevoked = make(chan struct{})
	} else {
		s.sessionLeaseRevoked = closedSessionLeaseSignal()
	}
}

func (s *contextSlot) signalLocked() {
	close(s.change)
	s.change = make(chan struct{})
}

func (s *contextSlot) fenceLocked() operationFence {
	return operationFence{s.contextEpoch, s.demandEpoch, s.operationEpoch}
}

func (s *contextSlot) matchesLocked(f operationFence) bool {
	return s.contextEpoch == f.contextEpoch && s.demandEpoch == f.demandEpoch && s.operationEpoch == f.operationEpoch && s.state != CoordinatorClosed
}

type failureClass uint8

const (
	failureAuthoritative failureClass = iota
	failureTemporary
	failureVersionMismatch
	failureCancelled
)
