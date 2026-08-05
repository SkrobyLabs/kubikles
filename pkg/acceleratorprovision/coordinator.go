package acceleratorprovision

import (
	"context"
	"sync"
	"time"

	"kubikles/pkg/acceleratorrelease"
	"kubikles/pkg/k8s"
)

// Coordinator composes the closed 40A-40E services. It never calls a Secret
// operation: lack of an active session lease means the desktop remains Direct.
type Coordinator struct {
	mu sync.Mutex

	contexts    ContextProvider
	resolver    coordinatorReleaseResolver
	provisioner coordinatorProvisioner
	connector   coordinatorConnector
	reconnector coordinatorReconnector
	disposer    coordinatorDisposer
	clock       coordinatorClock

	slots        map[uint64]*contextSlot
	swept        map[string]struct{}
	currentName  string
	currentEpoch uint64
	switching    bool
	quiesced     bool
	closed       bool

	workers sync.WaitGroup

	disposalMu sync.Mutex
	disposals  map[*ProvisionedWorkload]*coordinatorDisposal

	// terminalCleanupObserver is populated only by the disposable Kind harness.
	// It observes a completed drain disposal; it never participates in runtime
	// lifecycle decisions.
	terminalCleanupObserver func()
}

type coordinatorDisposal struct {
	done            chan struct{}
	fenced          chan struct{}
	fenceOnce       sync.Once
	drain           bool
	immediateIssued bool
	owners          map[*contextSlot]uint64
}

func newCoordinatorDisposal(drain bool) *coordinatorDisposal {
	return &coordinatorDisposal{done: make(chan struct{}), fenced: make(chan struct{}), drain: drain, immediateIssued: !drain}
}

func (o *coordinatorDisposal) markFenced() {
	if o != nil {
		o.fenceOnce.Do(func() { close(o.fenced) })
	}
}

func (o *coordinatorDisposal) waitFenced() {
	if o != nil && o.fenced != nil {
		<-o.fenced
	}
}

func NewDesktopCoordinator(k8sClient *k8s.Client, resolver *acceleratorrelease.Resolver, provisioner *Service, connector *Connector, reconnector *Reconnector, disposer *DisposalService) *Coordinator {
	return newCoordinator(desktopContexts{client: k8sClient}, resolver, provisioner, connector, reconnector, disposer, processResumeClock{})
}

func newCoordinator(contexts ContextProvider, resolver coordinatorReleaseResolver, provisioner coordinatorProvisioner, connector coordinatorConnector, reconnector coordinatorReconnector, disposer coordinatorDisposer, clock coordinatorClock) *Coordinator {
	coordinator := &Coordinator{
		contexts: contexts, resolver: resolver, provisioner: provisioner, connector: connector,
		reconnector: reconnector, disposer: disposer, clock: clock, slots: make(map[uint64]*contextSlot), currentEpoch: 1,
		disposals: make(map[*ProvisionedWorkload]*coordinatorDisposal), swept: make(map[string]struct{}),
	}
	if contexts != nil {
		coordinator.currentName = contexts.CurrentContext()
	}
	if clock == nil {
		coordinator.clock = processResumeClock{}
	}
	return coordinator
}

func (c *Coordinator) AcquireSecretDemand(ctx context.Context, contextName string) DemandResult {
	if c == nil || contextName == "" {
		return DemandResult{Reason: DemandWrongContext}
	}
	c.mu.Lock()
	if c.closed || c.quiesced {
		c.mu.Unlock()
		return DemandResult{Reason: DemandRuntimeClosing}
	}
	if c.switching {
		c.mu.Unlock()
		return DemandResult{Reason: DemandSwitching}
	}
	if contextName != c.currentName {
		c.mu.Unlock()
		return DemandResult{Reason: DemandWrongContext}
	}
	slot := c.slots[c.currentEpoch]
	if slot == nil {
		slot = newContextSlot(contextName, c.currentEpoch)
		c.slots[c.currentEpoch] = slot
	}
	c.mu.Unlock()

	slot.mu.Lock()
	if slot.state == CoordinatorClosed || slot.contextName != contextName {
		slot.mu.Unlock()
		return DemandResult{Reason: DemandWrongContext}
	}
	first := slot.demandCount == 0
	if first && slot.demandSettled {
		slot.demandEpoch++
		slot.demandSettled = false
		slot.mismatchRecreateUsed = false
		slot.replacementAttempt = false
	}
	slot.demandCount++
	lease := &SecretDemandLease{coordinator: c, slot: slot, demandEpoch: slot.demandEpoch}
	if slot.enabled && slot.state == CoordinatorDraining {
		if slot.idle != nil {
			c.startResumeLocked(slot, slot.idle)
		} else if slot.normalEnded {
			c.startResumeLocked(slot, nil)
		}
	}
	slot.mu.Unlock()
	attachDemandContext(ctx, lease)
	return DemandResult{Accepted: true, Reason: DemandAccepted, Lease: lease}
}

func (c *Coordinator) Snapshot(contextName string) CoordinatorSnapshot {
	if c == nil {
		return CoordinatorSnapshot{State: CoordinatorClosed}
	}
	c.mu.Lock()
	var slot *contextSlot
	if contextName == c.currentName {
		slot = c.slots[c.currentEpoch]
	}
	c.mu.Unlock()
	if slot == nil {
		return CoordinatorSnapshot{State: CoordinatorDirectOnly}
	}
	slot.mu.Lock()
	defer slot.mu.Unlock()
	return CoordinatorSnapshot{State: slot.state, Enabled: slot.enabled, Namespace: slot.namespace, DemandCount: slot.demandCount, SessionLeases: slot.sessionLeaseCount, Available: slot.state == CoordinatorActive && slot.session != nil, Workload: safeWorkloadProjection(slot.workload)}
}

func safeWorkloadProjection(workload *ProvisionedWorkload) *ProvisionedWorkload {
	if workload == nil {
		return nil
	}
	return &ProvisionedWorkload{
		ContextName: workload.ContextName, ReleaseNamespace: workload.ReleaseNamespace,
		ReleaseName: workload.ReleaseName, WorkloadSessionID: workload.WorkloadSessionID,
		Job: workload.Job, Pod: workload.Pod, BuildVersion: workload.BuildVersion,
		ImageDigest: workload.ImageDigest, ChartDigest: workload.ChartDigest,
	}
}

// Enable is the sole lifecycle authority. Secret demand only leases an already
// active source, so merely opening Secrets cannot provision a workload.
func (c *Coordinator) Enable(contextName, namespace string) {
	if c == nil || contextName == "" {
		return
	}
	c.mu.Lock()
	if c.closed || c.quiesced || c.switching || contextName != c.currentName {
		c.mu.Unlock()
		return
	}
	s := c.slots[c.currentEpoch]
	if s == nil {
		s = newContextSlot(contextName, c.currentEpoch)
		c.slots[c.currentEpoch] = s
	}
	c.mu.Unlock()
	s.mu.Lock()
	if s.state == CoordinatorClosed || (s.enabled && s.namespace == namespace) {
		s.mu.Unlock()
		return
	}
	// A namespace change is a new exact target: fence the old session first,
	// then dispose its private receipt before a fresh activation can begin.
	if s.enabled && s.workload != nil {
		if s.workerCancel != nil {
			s.workerCancel()
		}
		s.operationEpoch++
		s.advanceSessionLeaseEpochLocked(false)
		workload := s.workload
		s.enabled, s.namespace, s.state = true, namespace, CoordinatorDisposing
		s.mu.Unlock()
		go func() {
			c.disposeNowOwned(s, workload)
			s.mu.Lock()
			if s.enabled && s.state != CoordinatorClosed && s.workload == nil {
				s.state = CoordinatorDirectOnly
				c.startActivationLocked(s, 0)
			}
			s.mu.Unlock()
		}()
		return
	}
	s.enabled, s.namespace, s.mismatchLatched = true, namespace, false
	s.unavailableUntil = time.Time{}
	if s.state == CoordinatorDirectOnly || s.state == CoordinatorUnavailable {
		c.startActivationLocked(s, 0)
	}
	s.mu.Unlock()
}

func (c *Coordinator) Retry(contextName string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	s, current := c.slots[c.currentEpoch], c.currentName
	c.mu.Unlock()
	if s == nil || current != contextName {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.enabled || s.state == CoordinatorClosed {
		return
	}
	s.mismatchLatched, s.unavailableUntil = false, time.Time{}
	if s.state == CoordinatorUnavailable || s.state == CoordinatorDirectOnly {
		c.startActivationLocked(s, 0)
	}
}

// Disable synchronously revokes Integrated leases before asynchronous exact
// cleanup, making Direct fallback authoritative immediately.
func (c *Coordinator) Disable(contextName string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	s, current := c.slots[c.currentEpoch], c.currentName
	c.mu.Unlock()
	if s == nil || current != contextName {
		return
	}
	s.mu.Lock()
	if !s.enabled {
		s.mu.Unlock()
		return
	}
	s.enabled = false
	s.advanceSessionLeaseEpochLocked(false)
	if s.workerCancel != nil {
		s.workerCancel()
	}
	s.operationEpoch++
	workload := s.workload
	s.state = CoordinatorDisposing
	s.mu.Unlock()
	if workload != nil {
		go func() {
			c.drainAndDisposeOwned(s, workload)
			s.mu.Lock()
			if !s.enabled && s.state == CoordinatorDisposing && s.workload == nil && s.state != CoordinatorClosed {
				s.state = CoordinatorDirectOnly
			}
			s.mu.Unlock()
		}()
	} else {
		s.mu.Lock()
		if s.state != CoordinatorClosed {
			s.state = CoordinatorDirectOnly
		}
		s.mu.Unlock()
	}
}

func (c *Coordinator) releaseDemand(lease *SecretDemandLease) {
	if c == nil || lease == nil || lease.slot == nil {
		return
	}
	s := lease.slot
	s.mu.Lock()
	if s.demandEpoch != lease.demandEpoch || s.demandCount == 0 {
		s.mu.Unlock()
		return
	}
	s.demandCount--
	if s.demandCount != 0 {
		s.mu.Unlock()
		return
	}
	if s.enabled {
		s.settleDemandLocked()
		s.mu.Unlock()
		return
	}
	switch s.state {
	case CoordinatorSweeping, CoordinatorResolving, CoordinatorProvisioning, CoordinatorConnecting, CoordinatorReconnecting, CoordinatorUnavailable:
		if s.state == CoordinatorReconnecting && s.terminalCleanupPending {
			s.mu.Unlock()
			return
		}
		if s.workerCancel != nil {
			s.workerCancel()
		}
		s.pendingWorkerRun = nil
		s.operationEpoch++
		s.state = CoordinatorDirectOnly
		s.settleDemandLocked()
	default:
		if s.sessionLeaseCount == 0 {
			s.settleDemandLocked()
		}
	}
	s.mu.Unlock()
}

func (c *Coordinator) releaseSession(lease *sessionLeaseState) {
	if c == nil || lease == nil || lease.slot == nil {
		return
	}
	s := lease.slot
	s.mu.Lock()
	if s.sessionLeaseEpoch == lease.leaseEpoch && s.sessionLeaseCount > 0 {
		s.sessionLeaseCount--
	}
	if !s.enabled && s.sessionLeaseCount == 0 && s.demandCount == 0 && s.state == CoordinatorActive {
		s.settleDemandLocked()
		c.startIdleLocked(s)
	}
	s.mu.Unlock()
}

func (c *Coordinator) startActivationLocked(s *contextSlot, delay time.Duration) {
	if s == nil || !s.enabled || s.state == CoordinatorClosed || (s.mismatchLatched && delay <= 0) {
		return
	}
	doSweep := !s.sweepAttempted
	if doSweep {
		s.sweepAttempted = true
	}
	state := CoordinatorResolving
	if delay > 0 {
		state = CoordinatorUnavailable
	} else if doSweep {
		state = CoordinatorSweeping
	}
	c.startWorkerLocked(s, state, func(ctx context.Context, fence operationFence) {
		c.runActivation(ctx, s, fence, doSweep, delay)
	})
}

func (c *Coordinator) startWorkerLocked(s *contextSlot, state CoordinatorState, run func(context.Context, operationFence)) {
	if s.workerCancel != nil {
		s.workerCancel()
		s.operationEpoch++
		s.state = state
		s.pendingWorkerState = state
		s.pendingWorkerRun = run
		return
	}
	c.launchWorkerLocked(s, state, run)
}

func (c *Coordinator) launchWorkerLocked(s *contextSlot, state CoordinatorState, run func(context.Context, operationFence)) {
	s.operationEpoch++
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	s.workerCancel = cancel
	s.workerDone = done
	s.state = state
	fence := s.fenceLocked()
	c.workers.Add(1)
	go func() {
		defer c.workers.Done()
		defer close(done)
		defer func() {
			s.mu.Lock()
			if s.workerDone == done {
				s.workerDone = nil
				s.workerCancel = nil
				pendingState, pendingRun := s.pendingWorkerState, s.pendingWorkerRun
				s.pendingWorkerRun = nil
				if pendingRun != nil && s.state != CoordinatorClosed {
					c.launchWorkerLocked(s, pendingState, pendingRun)
				}
			}
			s.mu.Unlock()
		}()
		run(ctx, fence)
	}()
}

func (c *Coordinator) runActivation(ctx context.Context, s *contextSlot, fence operationFence, doSweep bool, initialDelay time.Duration) {
	if initialDelay > 0 {
		if c.clock.Sleep(ctx, initialDelay) != nil || !c.completeMismatchCooldown(s, fence) {
			return
		}
	}
	if doSweep {
		var snapshot ContextSnapshot
		if c.contexts != nil {
			snapshot, _ = c.contexts.SnapshotCurrentContext(s.contextName)
		}
		if c.disposer != nil && c.claimSweep(snapshot) {
			_ = c.disposer.SweepInert(ctx, snapshot)
		}
		if !c.transition(s, fence, CoordinatorResolving) {
			return
		}
	}
	for {
		for attempt := 0; attempt < MaxTransientActivationAttempts; attempt++ {
			if ctx.Err() != nil || !c.transition(s, fence, CoordinatorResolving) {
				return
			}
			attemptCtx, cancel := context.WithTimeout(ctx, ActivationAttemptTimeout)
			resolution := c.resolve(attemptCtx)
			if resolution.Availability != acceleratorrelease.Available {
				cancel()
				class := classifyReleaseFailure(resolution.Reason)
				if c.latchReplacementFailure(s, fence) {
					return
				}
				if !c.afterAttemptFailure(ctx, s, fence, class, attempt) {
					return
				}
				if class == failureTemporary && attempt+1 < MaxTransientActivationAttempts {
					continue
				}
				break
			}
			if !c.transition(s, fence, CoordinatorProvisioning) {
				cancel()
				return
			}
			provisioned := c.provision(attemptCtx, s.contextName, resolution)
			if provisioned.Availability != Available || provisioned.Workload == nil {
				cancel()
				class := classifyProvisionFailure(provisioned.Reason)
				if c.latchReplacementFailure(s, fence) {
					return
				}
				if !c.afterAttemptFailure(ctx, s, fence, class, attempt) {
					return
				}
				if class == failureTemporary && attempt+1 < MaxTransientActivationAttempts {
					continue
				}
				break
			}
			workload := provisioned.Workload
			if !c.retainWorkloadAndTransition(s, fence, workload, CoordinatorConnecting) {
				cancel()
				c.disposeNow(workload)
				return
			}
			connected := c.connect(attemptCtx, workload)
			cancel()
			if connected.Availability == Available && connected.Session != nil {
				if c.publishSession(s, fence, workload, connected.Session) {
					return
				}
				_ = connected.Session.Close(context.Background())
				c.disposeIfRetained(s, workload)
				return
			}
			if connected.Session != nil {
				_ = connected.Session.Close(context.Background())
			}
			class := classifyConnectFailure(connected.Reason)
			if !c.disposeRetained(ctx, s, fence, workload) {
				return
			}
			if c.latchReplacementFailure(s, fence) {
				return
			}
			if class == failureVersionMismatch {
				if c.consumeMismatchRecreate(s, fence) {
					attempt = -1
					continue
				}
				return
			}
			if !c.afterAttemptFailure(ctx, s, fence, class, attempt) {
				return
			}
			if class == failureTemporary && attempt+1 < MaxTransientActivationAttempts {
				continue
			}
			break
		}
		// Explicit lifecycle failures settle. Retry is user controlled; there is
		// no background cooldown loop that can surprise an idle connection.
		s.mu.Lock()
		if s.matchesLocked(fence) && s.enabled {
			s.state = CoordinatorUnavailable
		}
		s.mu.Unlock()
		return
	}
}

func (c *Coordinator) disposeIfRetained(s *contextSlot, workload *ProvisionedWorkload) {
	if s == nil || workload == nil {
		return
	}
	if done := c.startCoordinatedDisposeNow(s, workload); done != nil {
		<-done
		current := c.isCurrentSlot(s)
		s.mu.Lock()
		if s.workload == nil {
			if s.state != CoordinatorClosed {
				s.state = CoordinatorDirectOnly
				if current && s.demandCount > 0 {
					c.startActivationLocked(s, 0)
				}
			}
		}
		s.mu.Unlock()
	}
}

func (c *Coordinator) claimSweep(snapshot ContextSnapshot) bool {
	if snapshot == nil || snapshot.Identity() == "" || snapshot.Namespace() == "" {
		return true
	}
	key := snapshot.Identity() + "\x00" + snapshot.Namespace()
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.swept[key]; exists {
		return false
	}
	c.swept[key] = struct{}{}
	return true
}

func (c *Coordinator) resolve(ctx context.Context) acceleratorrelease.Resolution {
	if c.resolver == nil {
		return acceleratorrelease.Resolution{Availability: acceleratorrelease.Unavailable, Reason: acceleratorrelease.InvalidLocalBuild}
	}
	return c.resolver.Resolve(ctx)
}

func (c *Coordinator) provision(ctx context.Context, contextName string, resolution acceleratorrelease.Resolution) Result {
	if c.provisioner == nil {
		return unavailable(ContextUnavailable, CleanupNotNeeded)
	}
	c.mu.Lock()
	s := c.slots[c.currentEpoch]
	c.mu.Unlock()
	namespace := ""
	if s != nil {
		s.mu.Lock()
		namespace = s.namespace
		s.mu.Unlock()
	}
	return c.provisioner.Provision(ctx, Request{ContextName: contextName, NamespaceOverride: namespace, Resolution: resolution})
}

func (c *Coordinator) connect(ctx context.Context, workload *ProvisionedWorkload) ConnectResult {
	if c.connector == nil {
		return unavailableConnect(InvalidWorkload)
	}
	return c.connector.Connect(ctx, workload)
}

func (c *Coordinator) transition(s *contextSlot, fence operationFence, state CoordinatorState) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.matchesLocked(fence) || !s.enabled {
		return false
	}
	s.state = state
	return true
}

func (c *Coordinator) retainWorkloadAndTransition(s *contextSlot, fence operationFence, workload *ProvisionedWorkload, state CoordinatorState) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.matchesLocked(fence) || !s.enabled || workload == nil {
		return false
	}
	s.workload = workload
	s.state = state
	return true
}

func (c *Coordinator) publishSession(s *contextSlot, fence operationFence, workload *ProvisionedWorkload, session *ConnectedSession) bool {
	s.mu.Lock()
	if !s.matchesLocked(fence) || !s.enabled || s.workload != workload || session == nil {
		s.mu.Unlock()
		return false
	}
	s.session = session
	s.idle = nil
	s.normalEnded = false
	s.drainDeadline = time.Time{}
	s.advanceSessionLeaseEpochLocked(true)
	if s.replacementAttempt {
		s.replacementAttempt = false
	}
	s.state = CoordinatorActive
	s.signalLocked()
	s.mu.Unlock()
	go c.monitorSession(s, fence, workload, session)
	return true
}

func (c *Coordinator) monitorSession(s *contextSlot, fence operationFence, workload *ProvisionedWorkload, session *ConnectedSession) {
	started := session.terminalStarted()
	if started == nil {
		return
	}
	<-started
	s.mu.Lock()
	if !s.matchesLocked(fence) || s.state != CoordinatorActive || s.session != session || s.workload != workload {
		s.mu.Unlock()
		return
	}
	s.advanceSessionLeaseEpochLocked(false)
	s.signalLocked()
	s.terminalCleanupPending = true
	s.state = CoordinatorReconnecting
	if s.demandCount == 0 {
		s.settleDemandLocked()
	}
	s.mu.Unlock()
	<-session.Done()
	s.mu.Lock()
	if s.contextEpoch != fence.contextEpoch || s.state == CoordinatorClosed || s.session != session || s.workload != workload || !s.terminalCleanupPending {
		s.mu.Unlock()
		return
	}
	s.terminalCleanupPending = false
	if s.demandCount > 0 {
		c.startResumeLocked(s, nil)
		s.mu.Unlock()
		return
	}
	s.state = CoordinatorDisposing
	s.mu.Unlock()
	c.disposeAndContinue(s, fence, workload, false, true)
}

func (c *Coordinator) afterAttemptFailure(ctx context.Context, s *contextSlot, fence operationFence, class failureClass, attempt int) bool {
	if class == failureCancelled || ctx.Err() != nil {
		return false
	}
	if class == failureTemporary && attempt+1 < MaxTransientActivationAttempts {
		return c.clock.Sleep(ctx, ActivationRetryDelays[attempt]) == nil && c.transition(s, fence, CoordinatorResolving)
	}
	return class == failureTemporary || class == failureAuthoritative
}

func (c *Coordinator) cooldown(ctx context.Context, s *contextSlot, fence operationFence) bool {
	s.mu.Lock()
	if !s.matchesLocked(fence) || s.demandCount == 0 || s.mismatchLatched {
		s.mu.Unlock()
		return false
	}
	s.state = CoordinatorUnavailable
	s.unavailableUntil = c.clock.Now().Add(UnavailableCooldown)
	s.mu.Unlock()
	if c.clock.Sleep(ctx, UnavailableCooldown) != nil {
		return false
	}
	return c.transition(s, fence, CoordinatorResolving)
}

func (c *Coordinator) consumeMismatchRecreate(s *contextSlot, fence operationFence) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.matchesLocked(fence) || s.demandCount == 0 {
		return false
	}
	if !s.mismatchRecreateUsed {
		s.mismatchRecreateUsed = true
		s.replacementAttempt = true
		s.state = CoordinatorResolving
		return true
	}
	s.mismatchLatched = true
	s.unavailableUntil = c.clock.Now().Add(UnavailableCooldown)
	s.state = CoordinatorUnavailable
	return false
}

func (c *Coordinator) latchReplacementFailure(s *contextSlot, fence operationFence) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.matchesLocked(fence) || !s.replacementAttempt {
		return false
	}
	s.mismatchLatched = true
	s.unavailableUntil = c.clock.Now().Add(UnavailableCooldown)
	s.state = CoordinatorUnavailable
	return true
}

func (s *contextSlot) settleDemandLocked() {
	if s.demandCount != 0 || s.sessionLeaseCount != 0 || s.demandSettled {
		return
	}
	s.demandSettled = true
}

func (c *Coordinator) completeMismatchCooldown(s *contextSlot, fence operationFence) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.matchesLocked(fence) || s.demandCount == 0 || !s.mismatchLatched || c.clock.Now().Before(s.unavailableUntil) {
		return false
	}
	s.mismatchLatched = false
	return true
}

func (c *Coordinator) disposeRetained(ctx context.Context, s *contextSlot, fence operationFence, workload *ProvisionedWorkload) bool {
	s.mu.Lock()
	if !s.matchesLocked(fence) || s.workload != workload {
		retained := s.workload == workload
		s.mu.Unlock()
		if retained {
			c.disposeIfRetained(s, workload)
		}
		return false
	}
	s.state = CoordinatorDisposing
	s.mu.Unlock()
	c.disposeNowOwned(s, workload)
	current := c.isCurrentSlot(s)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.workload != nil {
		return false
	}
	if !s.matchesLocked(fence) || ctx.Err() != nil {
		if s.state != CoordinatorClosed {
			s.state = CoordinatorDirectOnly
			if current && s.demandCount > 0 {
				c.startActivationLocked(s, 0)
			}
		}
		return false
	}
	s.state = CoordinatorResolving
	return s.demandCount > 0
}

func (c *Coordinator) isCurrentSlot(s *contextSlot) bool {
	if c == nil || s == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return !c.closed && !c.quiesced && !c.switching && c.slots[c.currentEpoch] == s
}

func (s *contextSlot) clearWorkloadAuthorityLocked() {
	wasAvailable := s.state == CoordinatorActive && s.session != nil
	s.workload = nil
	s.session = nil
	s.idle = nil
	s.normalEnded = false
	s.drainDeadline = time.Time{}
	s.terminalCleanupPending = false
	s.advanceSessionLeaseEpochLocked(false)
	if s.demandCount == 0 {
		s.settleDemandLocked()
	}
	if wasAvailable {
		s.signalLocked()
	}
}

func (c *Coordinator) disposeNow(workload *ProvisionedWorkload) {
	c.disposeNowOwned(nil, workload)
}

func (c *Coordinator) disposeNowOwned(slot *contextSlot, workload *ProvisionedWorkload) {
	if done := c.startCoordinatedDisposeNow(slot, workload); done != nil {
		<-done
	}
}

func (c *Coordinator) drainAndDisposeOwned(slot *contextSlot, workload *ProvisionedWorkload) {
	c.coordinateDisposal(slot, workload, true)
}

func (c *Coordinator) coordinateDisposal(slot *contextSlot, workload *ProvisionedWorkload, drain bool) {
	if c == nil || c.disposer == nil || workload == nil {
		return
	}
	c.disposalMu.Lock()
	ownerEpoch, owned := c.validateDisposalOwnerLocked(slot, workload)
	if !owned {
		c.disposalMu.Unlock()
		return
	}
	if existing := c.disposals[workload]; existing != nil {
		c.registerDisposalOwnerLocked(existing, slot, ownerEpoch)
		// An immediate request is the one intentional second call: 40E uses it
		// to escalate an in-flight drain. All duplicate requests coalesce here.
		if !drain && existing.drain && !existing.immediateIssued {
			existing.immediateIssued = true
			c.disposalMu.Unlock()
			if async, ok := c.disposer.(coordinatorAsyncDisposer); ok {
				func() {
					defer existing.markFenced()
					completion := async.startDisposeNow(context.Background(), workload)
					completion.waitTransportFenced()
				}()
			} else {
				_ = c.disposer.DisposeNow(context.Background(), workload)
				existing.markFenced()
			}
			return
		}
		done := existing.done
		c.disposalMu.Unlock()
		<-done
		return
	}
	operation := newCoordinatorDisposal(drain)
	c.registerDisposalOwnerLocked(operation, slot, ownerEpoch)
	c.disposals[workload] = operation
	c.disposalMu.Unlock()
	if drain {
		_ = c.disposer.DrainAndDispose(context.Background(), workload)
	} else {
		_ = c.disposer.DisposeNow(context.Background(), workload)
	}
	operation.markFenced()
	c.finishCoordinatorDisposal(workload, operation)
}

func (c *Coordinator) startCoordinatedDisposeNow(slot *contextSlot, workload *ProvisionedWorkload) <-chan struct{} {
	if c == nil || c.disposer == nil || workload == nil {
		return nil
	}
	c.disposalMu.Lock()
	ownerEpoch, owned := c.validateDisposalOwnerLocked(slot, workload)
	if !owned {
		c.disposalMu.Unlock()
		return nil
	}
	if existing := c.disposals[workload]; existing != nil {
		c.registerDisposalOwnerLocked(existing, slot, ownerEpoch)
		if existing.drain && !existing.immediateIssued {
			existing.immediateIssued = true
			c.disposalMu.Unlock()
			if async, ok := c.disposer.(coordinatorAsyncDisposer); ok {
				func() {
					defer existing.markFenced()
					completion := async.startDisposeNow(context.Background(), workload)
					completion.waitTransportFenced()
				}()
			} else {
				go func() {
					_ = c.disposer.DisposeNow(context.Background(), workload)
					existing.markFenced()
				}()
			}
			existing.waitFenced()
			return existing.done
		}
		done := existing.done
		c.disposalMu.Unlock()
		existing.waitFenced()
		return done
	}
	operation := newCoordinatorDisposal(false)
	c.registerDisposalOwnerLocked(operation, slot, ownerEpoch)
	c.disposals[workload] = operation
	c.disposalMu.Unlock()
	if async, ok := c.disposer.(coordinatorAsyncDisposer); ok {
		var completion *disposalCompletion
		func() {
			defer operation.markFenced()
			completion = async.startDisposeNow(context.Background(), workload)
			completion.waitTransportFenced()
		}()
		go func() {
			if completion != nil && completion.Done() != nil {
				<-completion.Done()
			}
			c.finishCoordinatorDisposal(workload, operation)
		}()
	} else {
		go func() {
			_ = c.disposer.DisposeNow(context.Background(), workload)
			operation.markFenced()
			c.finishCoordinatorDisposal(workload, operation)
		}()
	}
	operation.waitFenced()
	return operation.done
}

func (c *Coordinator) finishCoordinatorDisposal(workload *ProvisionedWorkload, operation *coordinatorDisposal) {
	operation.markFenced()
	c.disposalMu.Lock()
	if c.disposals[workload] != operation {
		c.disposalMu.Unlock()
		return
	}
	owners := make([]*contextSlot, 0, len(operation.owners))
	for slot, epoch := range operation.owners {
		slot.mu.Lock()
		if slot.contextEpoch == epoch && slot.workload == workload {
			slot.clearWorkloadAuthorityLocked()
		}
		slot.mu.Unlock()
		owners = append(owners, slot)
	}
	delete(c.disposals, workload)
	close(operation.done)
	terminalCleanupObserver := c.terminalCleanupObserver
	terminalCleanup := operation.drain
	c.disposalMu.Unlock()
	for _, slot := range owners {
		c.pruneClosedSlot(slot)
	}
	if terminalCleanup && terminalCleanupObserver != nil {
		terminalCleanupObserver()
	}
}

func (c *Coordinator) validateDisposalOwnerLocked(slot *contextSlot, workload *ProvisionedWorkload) (uint64, bool) {
	if slot == nil {
		return 0, true
	}
	slot.mu.Lock()
	defer slot.mu.Unlock()
	if slot.workload != workload {
		return 0, false
	}
	return slot.contextEpoch, true
}

func (c *Coordinator) registerDisposalOwnerLocked(operation *coordinatorDisposal, slot *contextSlot, epoch uint64) {
	if operation == nil || slot == nil {
		return
	}
	if operation.owners == nil {
		operation.owners = make(map[*contextSlot]uint64)
	}
	operation.owners[slot] = epoch
}

func (c *Coordinator) pruneClosedSlot(slot *contextSlot) {
	if c == nil || slot == nil {
		return
	}
	slot.mu.Lock()
	eligible, epoch := slot.state == CoordinatorClosed && slot.workload == nil && slot.session == nil, slot.contextEpoch
	slot.mu.Unlock()
	if !eligible {
		return
	}
	c.mu.Lock()
	if epoch != c.currentEpoch && c.slots[epoch] == slot {
		delete(c.slots, epoch)
	}
	c.mu.Unlock()
}
