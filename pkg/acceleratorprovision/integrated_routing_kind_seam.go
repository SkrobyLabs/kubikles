//go:build helm && accelerator_provision_kind

package acceleratorprovision

import (
	"context"
	"errors"
	"sync/atomic"

	"kubikles/pkg/acceleratorrelease"
	"kubikles/pkg/k8s"
)

// integratedRoutingKindResolver is compiled only into the disposable Kind
// smoke. It keeps release resolution static while the production provision,
// connect, resume, disposal, session, and Secret RPC paths stay unchanged.
type integratedRoutingKindResolver struct {
	resolution acceleratorrelease.Resolution
}

// IntegratedRoutingKindCoordinatorProbe contains a cumulative attempt count
// and value-free closed stage buckets. It is unavailable outside the special
// Kind test build.
type IntegratedRoutingKindCoordinatorProbe struct {
	coordinator                   *Coordinator
	provisions                    atomic.Int32
	terminalCleanupStarts         atomic.Int32
	terminalCleanups              atomic.Int32
	provisionAvailable            atomic.Bool
	provisionUnavailable          atomic.Bool
	provisionContextInput         atomic.Bool
	provisionChartPull            atomic.Bool
	provisionChartIntegrityRender atomic.Bool
	provisionInstallPolicy        atomic.Bool
	provisionImagePull            atomic.Bool
	provisionJobPod               atomic.Bool
	provisionTimeoutCancel        atomic.Bool
	provisionUnknown              atomic.Bool
	connectAvailable              atomic.Bool
	connectTunnelUnavailable      atomic.Bool
	connectAcceleratorUnavailable atomic.Bool
	connectVersionMismatch        atomic.Bool
	connectAuthoritative          atomic.Bool
	connectCancelled              atomic.Bool
}

// TerminalCleanupStartCount reports terminal drain disposals that crossed the
// post-grace disposal boundary in the Kind-only harness.
func (p *IntegratedRoutingKindCoordinatorProbe) TerminalCleanupStartCount() int {
	if p == nil {
		return 0
	}
	return int(p.terminalCleanupStarts.Load())
}

// IntegratedRoutingKindActiveWorkload is the exact, redacted identity needed
// by the disposable Kind harness to observe owned cleanup. It carries neither
// a credential nor any production API capability.
type IntegratedRoutingKindActiveWorkload struct {
	ReleaseNamespace string
	ReleaseName      string
	JobName          string
	JobUID           string
	PodName          string
	PodUID           string
}

// ActiveWorkload returns a copy of the retained active workload identity. A
// missing, switched, quiesced, or otherwise fenced coordinator is unavailable.
func (p *IntegratedRoutingKindCoordinatorProbe) ActiveWorkload() (IntegratedRoutingKindActiveWorkload, bool) {
	if p == nil || p.coordinator == nil {
		return IntegratedRoutingKindActiveWorkload{}, false
	}
	coordinator := p.coordinator
	coordinator.mu.Lock()
	if coordinator.closed || coordinator.quiesced || coordinator.switching || coordinator.currentName == "" {
		coordinator.mu.Unlock()
		return IntegratedRoutingKindActiveWorkload{}, false
	}
	slot := coordinator.slots[coordinator.currentEpoch]
	if slot == nil {
		coordinator.mu.Unlock()
		return IntegratedRoutingKindActiveWorkload{}, false
	}
	slot.mu.Lock()
	workload := slot.workload
	if slot.state != CoordinatorActive || workload == nil || workload.ReleaseNamespace == "" || workload.ReleaseName == "" || workload.Job.Name == "" || workload.Job.UID == "" || workload.Pod.Name == "" || workload.Pod.UID == "" {
		slot.mu.Unlock()
		coordinator.mu.Unlock()
		return IntegratedRoutingKindActiveWorkload{}, false
	}
	identity := IntegratedRoutingKindActiveWorkload{
		ReleaseNamespace: workload.ReleaseNamespace,
		ReleaseName:      workload.ReleaseName,
		JobName:          workload.Job.Name,
		JobUID:           workload.Job.UID,
		PodName:          workload.Pod.Name,
		PodUID:           workload.Pod.UID,
	}
	slot.mu.Unlock()
	coordinator.mu.Unlock()
	return identity, true
}

// TerminalCleanupCount reports completed terminal drain disposals observed by
// the Kind-only harness.
func (p *IntegratedRoutingKindCoordinatorProbe) TerminalCleanupCount() int {
	if p == nil {
		return 0
	}
	return int(p.terminalCleanups.Load())
}

// IntegratedRoutingKindCoordinatorProbeSnapshot exposes only closed harness
// stages. It never carries workload identity, credentials, errors, or counts
// for individual outcomes.
type IntegratedRoutingKindCoordinatorProbeSnapshot struct {
	ProvisionAttempts             int
	ProvisionAvailable            bool
	ProvisionUnavailable          bool
	ProvisionContextInput         bool
	ProvisionChartPull            bool
	ProvisionChartIntegrityRender bool
	ProvisionInstallPolicy        bool
	ProvisionImagePull            bool
	ProvisionJobPod               bool
	ProvisionTimeoutCancel        bool
	ProvisionUnknown              bool
	ConnectAvailable              bool
	ConnectTunnelUnavailable      bool
	ConnectAcceleratorUnavailable bool
	ConnectVersionMismatch        bool
	ConnectAuthoritative          bool
	ConnectCancelled              bool
}

func (p *IntegratedRoutingKindCoordinatorProbe) ProvisionAttempts() int {
	if p == nil {
		return 0
	}
	return int(p.provisions.Load())
}

func (p *IntegratedRoutingKindCoordinatorProbe) Snapshot() IntegratedRoutingKindCoordinatorProbeSnapshot {
	if p == nil {
		return IntegratedRoutingKindCoordinatorProbeSnapshot{}
	}
	return IntegratedRoutingKindCoordinatorProbeSnapshot{
		ProvisionAttempts:             int(p.provisions.Load()),
		ProvisionAvailable:            p.provisionAvailable.Load(),
		ProvisionUnavailable:          p.provisionUnavailable.Load(),
		ProvisionContextInput:         p.provisionContextInput.Load(),
		ProvisionChartPull:            p.provisionChartPull.Load(),
		ProvisionChartIntegrityRender: p.provisionChartIntegrityRender.Load(),
		ProvisionInstallPolicy:        p.provisionInstallPolicy.Load(),
		ProvisionImagePull:            p.provisionImagePull.Load(),
		ProvisionJobPod:               p.provisionJobPod.Load(),
		ProvisionTimeoutCancel:        p.provisionTimeoutCancel.Load(),
		ProvisionUnknown:              p.provisionUnknown.Load(),
		ConnectAvailable:              p.connectAvailable.Load(),
		ConnectTunnelUnavailable:      p.connectTunnelUnavailable.Load(),
		ConnectAcceleratorUnavailable: p.connectAcceleratorUnavailable.Load(),
		ConnectVersionMismatch:        p.connectVersionMismatch.Load(),
		ConnectAuthoritative:          p.connectAuthoritative.Load(),
		ConnectCancelled:              p.connectCancelled.Load(),
	}
}

type integratedRoutingKindProvisioner struct {
	delegate coordinatorProvisioner
	probe    *IntegratedRoutingKindCoordinatorProbe
}

func (p integratedRoutingKindProvisioner) Provision(ctx context.Context, request Request) Result {
	p.probe.provisions.Add(1)
	result := p.delegate.Provision(ctx, request)
	if result.Availability == Available && result.Workload != nil {
		p.probe.provisionAvailable.Store(true)
	} else {
		if result.Availability == Unavailable && result.Workload == nil {
			p.probe.recordProvisionFailure(result.Reason)
		} else {
			p.probe.provisionUnknown.Store(true)
		}
		p.probe.provisionUnavailable.Store(true)
	}
	return result
}

func (p *IntegratedRoutingKindCoordinatorProbe) recordProvisionFailure(reason UnavailableReason) {
	switch reason {
	case ArtifactUnavailable, ContextUnavailable, ContextChanged, EntropyUnavailable:
		p.provisionContextInput.Store(true)
	case ChartPullFailed:
		p.provisionChartPull.Store(true)
	case ChartIntegrityFailed, RenderFailed:
		p.provisionChartIntegrityRender.Store(true)
	case InstallFailed, ReleaseConflict, PermissionDenied:
		p.provisionInstallPolicy.Store(true)
	case ImagePullFailed:
		p.provisionImagePull.Store(true)
	case JobFailed, PodFailed:
		p.provisionJobPod.Store(true)
	case TimedOut, Cancelled:
		p.provisionTimeoutCancel.Store(true)
	default:
		p.provisionUnknown.Store(true)
	}
}

type integratedRoutingKindConnector struct {
	delegate coordinatorConnector
	probe    *IntegratedRoutingKindCoordinatorProbe
}

func (c integratedRoutingKindConnector) Connect(ctx context.Context, workload *ProvisionedWorkload) ConnectResult {
	result := c.delegate.Connect(ctx, workload)
	if result.Availability == Available && result.Session != nil {
		c.probe.connectAvailable.Store(true)
		return result
	}
	switch result.Reason {
	case TunnelUnavailable:
		c.probe.connectTunnelUnavailable.Store(true)
	case AcceleratorUnavailable:
		c.probe.connectAcceleratorUnavailable.Store(true)
	case ConnectVersionMismatch:
		c.probe.connectVersionMismatch.Store(true)
	case ConnectCancelled, WorkloadDisposing:
		c.probe.connectCancelled.Store(true)
	default:
		c.probe.connectAuthoritative.Store(true)
	}
	return result
}

func (r integratedRoutingKindResolver) Resolve(context.Context) acceleratorrelease.Resolution {
	return r.resolution
}

// NewIntegratedRoutingKindCoordinator is a test-tag-only constructor for the
// root App composition smoke. The returned coordinator uses the production
// services supplied by desktop composition and only replaces release lookup
// with the harness' locally published immutable fixture.
func NewIntegratedRoutingKindCoordinator(
	client *k8s.Client,
	resolution acceleratorrelease.Resolution,
	provisioner *Service,
	connector *Connector,
	reconnector *Reconnector,
	disposer *DisposalService,
) (*Coordinator, *IntegratedRoutingKindCoordinatorProbe) {
	probe := &IntegratedRoutingKindCoordinatorProbe{}
	coordinator := newCoordinator(desktopContexts{client: client}, integratedRoutingKindResolver{resolution: resolution}, integratedRoutingKindProvisioner{delegate: provisioner, probe: probe}, integratedRoutingKindConnector{delegate: connector, probe: probe}, reconnector, disposer, processResumeClock{})
	probe.coordinator = coordinator
	coordinator.terminalCleanupStartObserver = func() { probe.terminalCleanupStarts.Add(1) }
	coordinator.terminalCleanupObserver = func() { probe.terminalCleanups.Add(1) }
	return coordinator, probe
}

// ForceIntegratedRoutingKindTransportLoss closes only the active production
// session transport. It is absent from production builds and lets the Kind
// smoke prove immediate Direct fallback and source-token replacement.
func ForceIntegratedRoutingKindTransportLoss(coordinator *Coordinator, contextName string) error {
	if coordinator == nil || contextName == "" {
		return errors.New("integrated routing Kind coordinator unavailable")
	}
	coordinator.mu.Lock()
	var slot *contextSlot
	if coordinator.currentName == contextName {
		slot = coordinator.slots[coordinator.currentEpoch]
	}
	coordinator.mu.Unlock()
	if slot == nil {
		return errors.New("integrated routing Kind slot unavailable")
	}
	slot.mu.Lock()
	session := slot.session
	active := slot.state == CoordinatorActive && session != nil
	slot.mu.Unlock()
	if !active || session.socket == nil {
		return errors.New("integrated routing Kind session unavailable")
	}
	return session.socket.Close()
}
