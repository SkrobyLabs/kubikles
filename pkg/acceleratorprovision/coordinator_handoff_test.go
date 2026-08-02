package acceleratorprovision

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"kubikles/pkg/acceleratorrelease"
	"kubikles/pkg/agent"
	"kubikles/pkg/server"
)

type blockingCoordinatorAsyncDisposer struct {
	delegate       *DisposalService
	startEntered   chan struct{}
	releaseStart   chan struct{}
	completion     chan *disposalCompletion
	dropCompletion bool
	startOnce      sync.Once
}

func (d *blockingCoordinatorAsyncDisposer) SweepInert(ctx context.Context, snapshot ContextSnapshot) SweepResult {
	return d.delegate.SweepInert(ctx, snapshot)
}

func (d *blockingCoordinatorAsyncDisposer) DisposeNow(ctx context.Context, workload *ProvisionedWorkload) DisposalResult {
	return d.delegate.DisposeNow(ctx, workload)
}

func (d *blockingCoordinatorAsyncDisposer) DrainAndDispose(ctx context.Context, workload *ProvisionedWorkload) DisposalResult {
	return d.delegate.DrainAndDispose(ctx, workload)
}

func (d *blockingCoordinatorAsyncDisposer) startDisposeNow(ctx context.Context, workload *ProvisionedWorkload) *disposalCompletion {
	d.startOnce.Do(func() { close(d.startEntered) })
	<-d.releaseStart
	completion := d.delegate.startDisposeNow(ctx, workload)
	d.completion <- completion
	if d.dropCompletion {
		return nil
	}
	return completion
}

func TestSecretDemandLeaseContextCancellationAndExplicitCloseRace(t *testing.T) {
	coordinator, services, _ := newCoordinatorHarness(t)
	sweepEntered, releaseSweep := make(chan struct{}), make(chan struct{})
	services.sweepHook = func(context.Context, ContextSnapshot) {
		close(sweepEntered)
		<-releaseSweep
	}
	anchor := coordinator.AcquireSecretDemand(context.Background(), "ctx").Lease
	<-sweepEntered

	const leaseCount = 256
	leases := make([]*SecretDemandLease, leaseCount)
	cancels := make([]context.CancelFunc, leaseCount)
	for index := 0; index < leaseCount; index++ {
		ctx, cancel := context.WithCancel(context.Background())
		if index%3 == 0 {
			cancel() // Exercise AfterFunc installation after cancellation.
		}
		leases[index] = coordinator.AcquireSecretDemand(ctx, "ctx").Lease
		cancels[index] = cancel
	}

	start := make(chan struct{})
	var group sync.WaitGroup
	for index, lease := range leases {
		group.Add(1)
		go func(index int, lease *SecretDemandLease) {
			defer group.Done()
			<-start
			if index%3 != 0 {
				cancels[index]()
			}
			lease.Close()
			lease.Close()
		}(index, lease)
	}
	close(start)
	group.Wait()
	for index, lease := range leases {
		lease.stopMu.Lock()
		stopRetained := lease.stopContext != nil
		lease.stopMu.Unlock()
		if !lease.closed.Load() || stopRetained {
			t.Fatalf("lease %d closed=%v callbackRetained=%v", index, lease.closed.Load(), stopRetained)
		}
	}

	if snapshot := coordinator.Snapshot("ctx"); snapshot.DemandCount != 1 || services.sweepCalls.Load() != 1 {
		t.Fatalf("before anchor close snapshot=%#v sweep=%d", snapshot, services.sweepCalls.Load())
	}
	anchor.Close()
	anchor.Close()
	if snapshot := coordinator.Snapshot("ctx"); snapshot.DemandCount != 0 {
		t.Fatalf("final demand count=%d", snapshot.DemandCount)
	}
	close(releaseSweep)
	stopCoordinator(t, coordinator)
}

func TestSecretDemandLeaseReplaceableSignalsAndNestedCounts(t *testing.T) {
	coordinator, services, clock := newCoordinatorHarness(t)
	workload := coordinatorWorkload(t)
	session, _ := coordinatorSession(workload, clock, 1)
	connectEntered, releaseConnect := make(chan struct{}), make(chan struct{})
	services.provisionHook = func(context.Context, int, Request) Result { return available(workload) }
	services.connectHook = func(context.Context, int, *ProvisionedWorkload) ConnectResult {
		close(connectEntered)
		<-releaseConnect
		return ConnectResult{Availability: Available, Session: session}
	}
	demand := coordinator.AcquireSecretDemand(context.Background(), "ctx").Lease
	initialEdge := demand.Changes()
	<-connectEntered
	close(releaseConnect)
	select {
	case <-initialEdge:
	case <-time.After(time.Second):
		t.Fatal("active publication did not close initial edge")
	}
	waitCoordinatorState(t, coordinator, CoordinatorActive)
	activeEdge := demand.Changes()
	if initialEdge == activeEdge {
		t.Fatal("active publication did not replace change channel")
	}
	sessionLease, ok := demand.TrySession()
	if !ok || coordinator.Snapshot("ctx").SessionLeases != 1 {
		t.Fatal("TrySession did not pin exactly one nested lease")
	}
	sessionLease.Close()
	sessionLease.Close()
	if snapshot := coordinator.Snapshot("ctx"); snapshot.SessionLeases != 0 || snapshot.DemandCount != 1 {
		t.Fatalf("after nested Close snapshot=%#v", snapshot)
	}
	demand.Close()
	demand.Close()
	select {
	case <-activeEdge:
	case <-time.After(time.Second):
		t.Fatal("final demand did not close active edge")
	}
	if snapshot := coordinator.Snapshot("ctx"); snapshot.DemandCount != 0 || snapshot.SessionLeases != 0 {
		t.Fatalf("final snapshot=%#v", snapshot)
	}
	stopCoordinator(t, coordinator)
}

func TestActivationBarrierFenceMatrix(t *testing.T) {
	type barrierRow struct {
		name   string
		phase  string
		action string
	}
	rows := []barrierRow{
		{name: "sweep versus final demand", phase: "sweep", action: "close"},
		{name: "resolve versus final demand", phase: "resolve", action: "close"},
		{name: "provision versus final demand", phase: "provision", action: "close"},
		{name: "connect versus final demand", phase: "connect", action: "close"},
		{name: "final publication versus context fence", phase: "connect", action: "fence"},
		{name: "final publication versus shutdown", phase: "connect", action: "shutdown"},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			coordinator, services, clock := newCoordinatorHarness(t)
			workload := coordinatorWorkload(t)
			session, _ := coordinatorSession(workload, clock, 1)
			entered, release := make(chan struct{}), make(chan struct{})
			services.provisionHook = func(context.Context, int, Request) Result { return available(workload) }
			services.connectHook = func(context.Context, int, *ProvisionedWorkload) ConnectResult {
				return ConnectResult{Availability: Available, Session: session}
			}
			switch row.phase {
			case "sweep":
				services.sweepHook = func(context.Context, ContextSnapshot) { close(entered); <-release }
			case "resolve":
				services.resolveHook = func(context.Context, int) acceleratorrelease.Resolution {
					close(entered)
					<-release
					return coordinatorResolution()
				}
			case "provision":
				services.provisionHook = func(context.Context, int, Request) Result {
					close(entered)
					<-release
					return available(workload)
				}
			case "connect":
				services.connectHook = func(context.Context, int, *ProvisionedWorkload) ConnectResult {
					close(entered)
					<-release
					return ConnectResult{Availability: Available, Session: session}
				}
			}
			demand := coordinator.AcquireSecretDemand(context.Background(), "ctx").Lease
			<-entered
			switch row.action {
			case "close":
				demand.Close()
			case "fence":
				coordinator.FenceContextSwitch("ctx")
				coordinator.ContextSwitched("other", true)
			case "shutdown":
				coordinator.Quiesce(context.Background())
			}
			close(release)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			coordinator.StopProducers(ctx)
			coordinator.Close(context.Background())

			disposeWant := int32(0)
			if row.phase == "provision" || row.phase == "connect" {
				disposeWant = 1
			}
			if got := services.disposeCalls.Load(); got != disposeWant {
				t.Fatalf("DisposeNow=%d want=%d events=%v", got, disposeWant, services.events())
			}
			if row.phase == "connect" {
				select {
				case <-session.Done():
				case <-time.After(time.Second):
					t.Fatal("stale connected session remained open")
				}
			}
			demand.slot.mu.Lock()
			available := demand.slot.state == CoordinatorActive && demand.slot.session != nil
			worker := demand.slot.workerDone
			demand.slot.mu.Unlock()
			if available || worker != nil {
				t.Fatalf("stale completion published=%v workerRetained=%v", available, worker != nil)
			}
		})
	}
}

func TestFailureDisposalProofMatrix(t *testing.T) {
	t.Run("40B failure without workload skips 40E", func(t *testing.T) {
		coordinator, services, clock := newCoordinatorHarness(t)
		services.provisionHook = func(context.Context, int, Request) Result {
			return unavailable(InstallFailed, CleanupFailed)
		}
		demand := coordinator.AcquireSecretDemand(context.Background(), "ctx").Lease
		retry := <-clock.sleeps
		if retry.duration != time.Second || services.provisionCalls.Load() != 1 || services.disposeCalls.Load() != 0 {
			t.Fatalf("delay=%s provision=%d dispose=%d", retry.duration, services.provisionCalls.Load(), services.disposeCalls.Load())
		}
		demand.Close()
		stopCoordinator(t, coordinator)
	})

	for _, reason := range []ResumeReason{ResumeExplicitlyClosed, ResumeGraceExpired} {
		t.Run("40D "+string(reason)+" waits for disposal", func(t *testing.T) {
			coordinator, services, clock := newCoordinatorHarness(t)
			first, second := coordinatorWorkload(t), coordinatorWorkload(t)
			prior, socket := coordinatorSession(first, clock, 1)
			fresh, _ := coordinatorSession(second, clock, 1)
			services.provisionHook = func(_ context.Context, call int, _ Request) Result {
				if call == 1 {
					return available(first)
				}
				return available(second)
			}
			services.connectHook = func(_ context.Context, call int, _ *ProvisionedWorkload) ConnectResult {
				if call == 1 {
					return ConnectResult{Availability: Available, Session: prior}
				}
				return ConnectResult{Availability: Available, Session: fresh}
			}
			services.resumeHook = func(context.Context, int, ResumeRequest, *coordinatorIdleToken) ResumeResult {
				return unavailableResume(reason)
			}
			disposeEntered, releaseDispose := make(chan struct{}), make(chan struct{})
			services.disposeHook = func(_ bool, workload *ProvisionedWorkload) {
				if workload == first {
					close(disposeEntered)
					<-releaseDispose
				}
			}
			demand := coordinator.AcquireSecretDemand(context.Background(), "ctx").Lease
			waitCoordinatorState(t, coordinator, CoordinatorActive)
			change := demand.Changes()
			_ = socket.Close()
			<-change
			<-disposeEntered
			if services.provisionCalls.Load() != 1 {
				t.Fatal("fresh Provision crossed incomplete 40E disposal")
			}
			if _, ok := demand.TrySession(); ok {
				t.Fatal("terminal Resume failure retained session authority")
			}
			close(releaseDispose)
			clock.advance(t, UnavailableCooldown)
			waitCoordinatorState(t, coordinator, CoordinatorActive)
			if services.provisionCalls.Load() != 2 || services.disposeCalls.Load() != 1 {
				t.Fatalf("provision=%d dispose=%d", services.provisionCalls.Load(), services.disposeCalls.Load())
			}
			demand.Close()
			services.disposeHook = nil
			stopCoordinator(t, coordinator)
		})
	}

	t.Run("40E cleanup failure remains Direct then uses fresh workload", func(t *testing.T) {
		coordinator, services, clock := newCoordinatorHarness(t)
		first, second := coordinatorWorkload(t), coordinatorWorkload(t)
		secondSession, _ := coordinatorSession(second, clock, 1)
		services.provisionHook = func(_ context.Context, call int, _ Request) Result {
			if call == 1 {
				return available(first)
			}
			return available(second)
		}
		services.connectHook = func(_ context.Context, call int, _ *ProvisionedWorkload) ConnectResult {
			if call == 1 {
				return unavailableConnect(InvalidWorkload)
			}
			return ConnectResult{Availability: Available, Session: secondSession}
		}
		services.disposeResult = func(bool, *ProvisionedWorkload) DisposalResult {
			return DisposalResult{Requested: DisposalImmediate, Effective: DisposalImmediate, Ownership: OwnershipUnproven, Uninstall: UninstallFailed, Disappearance: DisappearanceResourcesRemaining}
		}
		demand := coordinator.AcquireSecretDemand(context.Background(), "ctx").Lease
		clock.advance(t, UnavailableCooldown)
		waitCoordinatorState(t, coordinator, CoordinatorActive)
		if services.provisionCalls.Load() != 2 || services.disposeCalls.Load() != 1 {
			t.Fatalf("cleanup failure provision=%d dispose=%d", services.provisionCalls.Load(), services.disposeCalls.Load())
		}
		demand.Close()
		stopCoordinator(t, coordinator)
	})
}

func TestImmediateReacquireWaitsForCancelledWorkerCleanup(t *testing.T) {
	coordinator, services, clock := newCoordinatorHarness(t)
	first, second := coordinatorWorkload(t), coordinatorWorkload(t)
	secondSession, _ := coordinatorSession(second, clock, 1)
	connectEntered, releaseConnect := make(chan struct{}), make(chan struct{})
	var firstDisposed bool
	services.provisionHook = func(_ context.Context, call int, _ Request) Result {
		if call == 2 && !firstDisposed {
			t.Fatal("replacement provision overlapped stale workload cleanup")
		}
		if call == 1 {
			return available(first)
		}
		return available(second)
	}
	services.connectHook = func(_ context.Context, call int, _ *ProvisionedWorkload) ConnectResult {
		if call == 1 {
			close(connectEntered)
			<-releaseConnect
			return unavailableConnect(TunnelUnavailable)
		}
		return ConnectResult{Availability: Available, Session: secondSession}
	}
	services.disposeHook = func(_ bool, workload *ProvisionedWorkload) {
		if workload == first {
			firstDisposed = true
		}
	}

	firstDemand := coordinator.AcquireSecretDemand(context.Background(), "ctx").Lease
	<-connectEntered
	firstDemand.Close()
	secondDemand := coordinator.AcquireSecretDemand(context.Background(), "ctx").Lease
	close(releaseConnect)
	waitCoordinatorState(t, coordinator, CoordinatorActive)
	if services.provisionCalls.Load() != 2 || services.disposeCalls.Load() != 1 || !firstDisposed {
		t.Fatalf("provision=%d dispose=%d firstDisposed=%v", services.provisionCalls.Load(), services.disposeCalls.Load(), firstDisposed)
	}
	secondDemand.Close()
	stopCoordinator(t, coordinator)
}

func TestDemandDuringDisposalStartsFreshOnlyAfterExactCleanup(t *testing.T) {
	coordinator, services, clock := newCoordinatorHarness(t)
	first, second := coordinatorWorkload(t), coordinatorWorkload(t)
	firstSession, firstSocket := coordinatorSession(first, clock, 1)
	secondSession, _ := coordinatorSession(second, clock, 1)
	disposeEntered, releaseDispose := make(chan struct{}), make(chan struct{})
	services.provisionHook = func(_ context.Context, call int, _ Request) Result {
		if call == 1 {
			return available(first)
		}
		return available(second)
	}
	services.connectHook = func(_ context.Context, call int, _ *ProvisionedWorkload) ConnectResult {
		if call == 1 {
			return ConnectResult{Availability: Available, Session: firstSession}
		}
		return ConnectResult{Availability: Available, Session: secondSession}
	}
	services.resumeHook = func(context.Context, int, ResumeRequest, *coordinatorIdleToken) ResumeResult {
		return unavailableResume(ResumeGraceExpired)
	}
	services.disposeHook = func(_ bool, workload *ProvisionedWorkload) {
		if workload == first {
			close(disposeEntered)
			<-releaseDispose
		}
	}

	firstDemand := coordinator.AcquireSecretDemand(context.Background(), "ctx").Lease
	waitCoordinatorState(t, coordinator, CoordinatorActive)
	_ = firstSocket.Close()
	<-disposeEntered
	firstDemand.Close()
	secondDemand := coordinator.AcquireSecretDemand(context.Background(), "ctx").Lease
	if services.provisionCalls.Load() != 1 {
		t.Fatal("fresh provision started before disposal completion")
	}
	close(releaseDispose)
	waitCoordinatorState(t, coordinator, CoordinatorActive)
	if services.provisionCalls.Load() != 2 || services.disposeCalls.Load() != 1 {
		t.Fatalf("provision=%d dispose=%d", services.provisionCalls.Load(), services.disposeCalls.Load())
	}
	secondDemand.Close()
	services.disposeHook = nil
	stopCoordinator(t, coordinator)
}

func TestNestedSessionLeaseKeepsContinuousDemandEpochAndTerminalMonitor(t *testing.T) {
	coordinator, services, clock := newCoordinatorHarness(t)
	workload := coordinatorWorkload(t)
	first, firstSocket := coordinatorSession(workload, clock, 1)
	second, _ := coordinatorSession(workload, clock, 2)
	services.provisionHook = func(context.Context, int, Request) Result { return available(workload) }
	services.connectHook = func(context.Context, int, *ProvisionedWorkload) ConnectResult {
		return ConnectResult{Availability: Available, Session: first}
	}
	services.resumeHook = func(context.Context, int, ResumeRequest, *coordinatorIdleToken) ResumeResult {
		return ResumeResult{Availability: Available, Session: second}
	}

	initial := coordinator.AcquireSecretDemand(context.Background(), "ctx").Lease
	waitCoordinatorState(t, coordinator, CoordinatorActive)
	sessionLease, ok := initial.TrySession()
	if !ok {
		t.Fatal("missing nested session lease")
	}
	epoch := initial.demandEpoch
	initial.Close()
	returned := coordinator.AcquireSecretDemand(context.Background(), "ctx").Lease
	if returned.demandEpoch != epoch {
		t.Fatalf("continuous demand epoch changed: got=%d want=%d", returned.demandEpoch, epoch)
	}
	change := returned.Changes()
	_ = firstSocket.Close()
	select {
	case <-change:
	case <-time.After(time.Second):
		t.Fatal("terminal monitor did not revoke continuous epoch")
	}
	waitCoordinatorState(t, coordinator, CoordinatorActive)
	lease, ok := returned.TrySession()
	if !ok || lease.Session() != second || services.provisionCalls.Load() != 1 {
		t.Fatalf("terminal monitor lost: leased=%v provision=%d", ok, services.provisionCalls.Load())
	}
	lease.Close()
	sessionLease.Close()
	returned.Close()
	stopCoordinator(t, coordinator)
}

func TestReplacementFailureLatchesWithoutThirdWorkload(t *testing.T) {
	coordinator, services, _ := newCoordinatorHarness(t)
	first, replacement := coordinatorWorkload(t), coordinatorWorkload(t)
	services.provisionHook = func(_ context.Context, call int, _ Request) Result {
		if call == 1 {
			return available(first)
		}
		if call == 2 {
			return available(replacement)
		}
		t.Fatal("third workload was provisioned")
		return Result{}
	}
	services.connectHook = func(_ context.Context, call int, _ *ProvisionedWorkload) ConnectResult {
		if call == 1 {
			return unavailableConnect(ConnectVersionMismatch)
		}
		return unavailableConnect(TunnelUnavailable)
	}
	demand := coordinator.AcquireSecretDemand(context.Background(), "ctx").Lease
	waitCoordinatorState(t, coordinator, CoordinatorUnavailable)
	if services.provisionCalls.Load() != 2 || services.connectCalls.Load() != 2 || services.disposeCalls.Load() != 2 {
		t.Fatalf("provision=%d connect=%d dispose=%d", services.provisionCalls.Load(), services.connectCalls.Load(), services.disposeCalls.Load())
	}
	demand.Close()
	stopCoordinator(t, coordinator)
}

func TestPeerTerminationWinningIdleReleaseResumesBeforeOriginalDeadline(t *testing.T) {
	coordinator, services, clock := newCoordinatorHarness(t)
	workload := coordinatorWorkload(t)
	prior, _ := coordinatorSession(workload, clock, 1)
	workload.connectorState.mu.Lock()
	workload.connectorState.currentSession = prior
	workload.connectorState.mu.Unlock()
	prior.beginTermination(SessionPeerClosed, false)
	<-prior.Done()
	replacement, _ := coordinatorSession(workload, clock, 2)
	services.resumeHook = func(_ context.Context, _ int, request ResumeRequest, idle *coordinatorIdleToken) ResumeResult {
		if idle != nil || request.Prior != prior || request.Workload != workload {
			t.Fatal("peer-winner did not use normal Resume authority")
		}
		return ResumeResult{Availability: Available, Session: replacement}
	}
	slot := newContextSlot("ctx", coordinator.currentEpoch)
	slot.state = CoordinatorDraining
	slot.workload = workload
	slot.session = prior
	coordinator.mu.Lock()
	coordinator.slots[coordinator.currentEpoch] = slot
	coordinator.mu.Unlock()
	fence := slot.fenceLocked()
	go coordinator.runIdleRelease(context.Background(), slot, fence)
	sleep := <-clock.sleeps
	if sleep.duration != agent.AcceleratorIdleReconnectGrace {
		t.Fatalf("peer deadline=%s", sleep.duration)
	}
	demand := coordinator.AcquireSecretDemand(context.Background(), "ctx").Lease
	waitCoordinatorState(t, coordinator, CoordinatorActive)
	close(sleep.release)
	if services.resumeCalls.Load() != 1 || services.drainCalls.Load() != 0 || services.provisionCalls.Load() != 0 {
		t.Fatalf("resume=%d drain=%d provision=%d", services.resumeCalls.Load(), services.drainCalls.Load(), services.provisionCalls.Load())
	}
	demand.Close()
	stopCoordinator(t, coordinator)
}

func TestTerminalStartRevokesBeforeCleanupAndResume(t *testing.T) {
	coordinator, services, clock := newCoordinatorHarness(t)
	workload := coordinatorWorkload(t)
	readRelease := make(chan struct{})
	socket := &cleanupBlockingSessionSocket{readRelease: readRelease, closeCalled: make(chan struct{})}
	first := newConnectedSession(workload.connectorState.receipt, server.AuthenticatedAcceleratorInfo{Capabilities: agent.V1Capabilities()}, connectedIdentity{sessionID: "session-a", instanceID: "instance-a", generation: 1}, socket, newRecordedTunnel(&[]string{}), clock)
	second, _ := coordinatorSession(workload, clock, 2)
	resumeEntered := make(chan struct{})
	services.provisionHook = func(context.Context, int, Request) Result { return available(workload) }
	services.connectHook = func(context.Context, int, *ProvisionedWorkload) ConnectResult {
		return ConnectResult{Availability: Available, Session: first}
	}
	services.resumeHook = func(context.Context, int, ResumeRequest, *coordinatorIdleToken) ResumeResult {
		close(resumeEntered)
		return ResumeResult{Availability: Available, Session: second}
	}
	demand := coordinator.AcquireSecretDemand(context.Background(), "ctx").Lease
	waitCoordinatorState(t, coordinator, CoordinatorActive)
	change := demand.Changes()
	first.beginTermination(SessionPeerClosed, false)
	select {
	case <-change:
	case <-time.After(time.Second):
		t.Fatal("terminal start did not revoke availability")
	}
	if _, ok := demand.TrySession(); ok {
		t.Fatal("terminal cleanup retained session authority")
	}
	select {
	case <-resumeEntered:
		t.Fatal("Resume started before public session Done")
	default:
	}
	close(readRelease)
	select {
	case <-resumeEntered:
	case <-time.After(time.Second):
		t.Fatal("Resume did not start after terminal cleanup")
	}
	waitCoordinatorState(t, coordinator, CoordinatorActive)
	demand.Close()
	stopCoordinator(t, coordinator)
}

func TestTrySessionRejectsTerminalStartBeforeMonitorRuns(t *testing.T) {
	coordinator, _, clock := newCoordinatorHarness(t)
	workload := coordinatorWorkload(t)
	readRelease := make(chan struct{})
	session := newConnectedSession(workload.connectorState.receipt, server.AuthenticatedAcceleratorInfo{Capabilities: agent.V1Capabilities()}, connectedIdentity{sessionID: "session-a", instanceID: "instance-a", generation: 1}, &cleanupBlockingSessionSocket{readRelease: readRelease, closeCalled: make(chan struct{})}, newRecordedTunnel(&[]string{}), clock)
	slot := newContextSlot("ctx", coordinator.currentEpoch)
	slot.state, slot.workload, slot.session = CoordinatorActive, workload, session
	slot.demandEpoch, slot.demandCount, slot.demandSettled = 1, 1, false
	coordinator.mu.Lock()
	coordinator.slots[coordinator.currentEpoch] = slot
	coordinator.mu.Unlock()
	lease := &SecretDemandLease{coordinator: coordinator, slot: slot, demandEpoch: 1}

	session.beginTermination(SessionPeerClosed, false)
	select {
	case <-session.terminalStarted():
	case <-time.After(time.Second):
		t.Fatal("terminal start was not established")
	}
	if _, ok := lease.TrySession(); ok {
		t.Fatal("TrySession leased terminal-start session before monitor revocation")
	}
	select {
	case <-session.Done():
		t.Fatal("cleanup unexpectedly completed")
	default:
	}
	close(readRelease)
	<-session.Done()
	lease.Close()
}

func TestDemandReturningDuringTerminalDisposalStartsFreshAfterCompletion(t *testing.T) {
	coordinator, services, clock := newCoordinatorHarness(t)
	first, second := coordinatorWorkload(t), coordinatorWorkload(t)
	readRelease := make(chan struct{})
	firstSession := newConnectedSession(first.connectorState.receipt, server.AuthenticatedAcceleratorInfo{Capabilities: agent.V1Capabilities()}, connectedIdentity{sessionID: "session-a", instanceID: "instance-a", generation: 1}, &cleanupBlockingSessionSocket{readRelease: readRelease, closeCalled: make(chan struct{})}, newRecordedTunnel(&[]string{}), clock)
	secondSession, _ := coordinatorSession(second, clock, 1)
	disposeEntered, releaseDispose := make(chan struct{}), make(chan struct{})
	services.provisionHook = func(_ context.Context, call int, _ Request) Result {
		if call == 1 {
			return available(first)
		}
		return available(second)
	}
	services.connectHook = func(_ context.Context, call int, _ *ProvisionedWorkload) ConnectResult {
		if call == 1 {
			return ConnectResult{Availability: Available, Session: firstSession}
		}
		return ConnectResult{Availability: Available, Session: secondSession}
	}
	services.disposeHook = func(_ bool, workload *ProvisionedWorkload) {
		if workload == first {
			close(disposeEntered)
			<-releaseDispose
		}
	}

	firstDemand := coordinator.AcquireSecretDemand(context.Background(), "ctx").Lease
	waitCoordinatorState(t, coordinator, CoordinatorActive)
	change := firstDemand.Changes()
	firstSession.beginTermination(SessionPeerClosed, false)
	<-change
	firstDemand.Close()
	close(readRelease)
	<-disposeEntered
	returned := coordinator.AcquireSecretDemand(context.Background(), "ctx").Lease
	if services.provisionCalls.Load() != 1 {
		t.Fatal("terminal disposal overlapped fresh activation")
	}
	close(releaseDispose)
	waitCoordinatorState(t, coordinator, CoordinatorActive)
	if services.provisionCalls.Load() != 2 || services.disposeCalls.Load() != 1 {
		t.Fatalf("provision=%d dispose=%d", services.provisionCalls.Load(), services.disposeCalls.Load())
	}
	returned.Close()
	services.disposeHook = nil
	stopCoordinator(t, coordinator)
}

func TestRepeatedMismatchCooldownSurvivesCancelledDemandEpochs(t *testing.T) {
	coordinator, services, clock := newCoordinatorHarness(t)
	workloads := []*ProvisionedWorkload{coordinatorWorkload(t), coordinatorWorkload(t), coordinatorWorkload(t)}
	finalSession, _ := coordinatorSession(workloads[2], clock, 1)
	services.provisionHook = func(_ context.Context, call int, _ Request) Result { return available(workloads[call-1]) }
	services.connectHook = func(_ context.Context, call int, _ *ProvisionedWorkload) ConnectResult {
		if call <= 2 {
			return unavailableConnect(ConnectVersionMismatch)
		}
		return ConnectResult{Availability: Available, Session: finalSession}
	}

	demand := coordinator.AcquireSecretDemand(context.Background(), "ctx").Lease
	waitCoordinatorState(t, coordinator, CoordinatorUnavailable)
	demand.Close()
	for attempt := 0; attempt < 2; attempt++ {
		aborted := coordinator.AcquireSecretDemand(context.Background(), "ctx").Lease
		request := <-clock.sleeps
		if request.duration != UnavailableCooldown || services.provisionCalls.Load() != 2 {
			t.Fatalf("attempt=%d delay=%s provision=%d", attempt, request.duration, services.provisionCalls.Load())
		}
		aborted.Close()
	}
	final := coordinator.AcquireSecretDemand(context.Background(), "ctx").Lease
	request := <-clock.sleeps
	if request.duration != UnavailableCooldown || services.provisionCalls.Load() != 2 {
		t.Fatalf("final delay=%s provision=%d", request.duration, services.provisionCalls.Load())
	}
	clock.mu.Lock()
	clock.now = clock.now.Add(request.duration)
	clock.mu.Unlock()
	close(request.release)
	waitCoordinatorState(t, coordinator, CoordinatorActive)
	if services.provisionCalls.Load() != 3 {
		t.Fatalf("provision=%d", services.provisionCalls.Load())
	}
	final.Close()
	stopCoordinator(t, coordinator)
}

func TestReplacementSuccessRestoresOrdinaryFailurePolicyButNotMismatchBudget(t *testing.T) {
	t.Run("ordinary transport failure", func(t *testing.T) {
		coordinator, services, clock := newCoordinatorHarness(t)
		workloads := []*ProvisionedWorkload{coordinatorWorkload(t), coordinatorWorkload(t), coordinatorWorkload(t)}
		replacement, replacementSocket := coordinatorSession(workloads[1], clock, 1)
		fresh, _ := coordinatorSession(workloads[2], clock, 1)
		services.provisionHook = func(_ context.Context, call int, _ Request) Result { return available(workloads[call-1]) }
		services.connectHook = func(_ context.Context, call int, _ *ProvisionedWorkload) ConnectResult {
			switch call {
			case 1:
				return unavailableConnect(ConnectVersionMismatch)
			case 2:
				return ConnectResult{Availability: Available, Session: replacement}
			default:
				return ConnectResult{Availability: Available, Session: fresh}
			}
		}
		services.resumeHook = func(context.Context, int, ResumeRequest, *coordinatorIdleToken) ResumeResult {
			return unavailableResume(ResumeTransportUnavailable)
		}
		demand := coordinator.AcquireSecretDemand(context.Background(), "ctx").Lease
		waitCoordinatorState(t, coordinator, CoordinatorActive)
		change := demand.Changes()
		_ = replacementSocket.Close()
		<-change
		waitCoordinatorState(t, coordinator, CoordinatorActive)
		if services.provisionCalls.Load() != 3 {
			t.Fatalf("ordinary failure provision=%d", services.provisionCalls.Load())
		}
		demand.Close()
		stopCoordinator(t, coordinator)
	})

	t.Run("second mismatch", func(t *testing.T) {
		coordinator, services, clock := newCoordinatorHarness(t)
		workloads := []*ProvisionedWorkload{coordinatorWorkload(t), coordinatorWorkload(t)}
		replacement, replacementSocket := coordinatorSession(workloads[1], clock, 1)
		services.provisionHook = func(_ context.Context, call int, _ Request) Result { return available(workloads[call-1]) }
		services.connectHook = func(_ context.Context, call int, _ *ProvisionedWorkload) ConnectResult {
			if call == 1 {
				return unavailableConnect(ConnectVersionMismatch)
			}
			return ConnectResult{Availability: Available, Session: replacement}
		}
		services.resumeHook = func(context.Context, int, ResumeRequest, *coordinatorIdleToken) ResumeResult {
			return unavailableResume(ResumeVersionMismatch)
		}
		demand := coordinator.AcquireSecretDemand(context.Background(), "ctx").Lease
		waitCoordinatorState(t, coordinator, CoordinatorActive)
		change := demand.Changes()
		_ = replacementSocket.Close()
		<-change
		waitCoordinatorState(t, coordinator, CoordinatorUnavailable)
		if services.provisionCalls.Load() != 2 || services.disposeCalls.Load() != 2 {
			t.Fatalf("second mismatch provision=%d dispose=%d", services.provisionCalls.Load(), services.disposeCalls.Load())
		}
		demand.Close()
		stopCoordinator(t, coordinator)
	})
}

func TestDisposalOwnershipClearsAuthorityBeforeDedupeReleaseAndPrunesSlot(t *testing.T) {
	coordinator, services, _ := newCoordinatorHarness(t)
	workload := coordinatorWorkload(t)
	slot := newContextSlot("ctx", coordinator.currentEpoch)
	slot.state, slot.workload = CoordinatorActive, workload
	coordinator.mu.Lock()
	coordinator.slots[coordinator.currentEpoch] = slot
	coordinator.mu.Unlock()
	disposeEntered, releaseDispose := make(chan struct{}), make(chan struct{})
	services.disposeHook = func(bool, *ProvisionedWorkload) {
		close(disposeEntered)
		<-releaseDispose
	}

	coordinator.FenceContextSwitch("ctx")
	<-disposeEntered
	coordinator.ContextSwitched("other", true)
	closed := make(chan struct{})
	go func() {
		coordinator.Close(context.Background())
		close(closed)
	}()
	close(releaseDispose)
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("competing shutdown did not join disposal")
	}
	coordinator.disposalMu.Lock()
	disposals := len(coordinator.disposals)
	coordinator.disposalMu.Unlock()
	coordinator.mu.Lock()
	_, retainedSlot := coordinator.slots[slot.contextEpoch]
	coordinator.mu.Unlock()
	if services.disposeCalls.Load() != 1 || disposals != 0 || retainedSlot {
		t.Fatalf("dispose=%d map=%d retainedSlot=%v", services.disposeCalls.Load(), disposals, retainedSlot)
	}
}

func TestCoordinatorCombinedBarrierRaceMatrix(t *testing.T) {
	const iterations = 100
	for iteration := 0; iteration < iterations; iteration++ {
		coordinator, services, clock := newCoordinatorHarness(t)
		workload := coordinatorWorkload(t)
		prior, socket := coordinatorSession(workload, clock, 1)
		resumed, _ := coordinatorSession(workload, clock, 2)
		services.provisionHook = func(context.Context, int, Request) Result { return available(workload) }
		services.connectHook = func(context.Context, int, *ProvisionedWorkload) ConnectResult {
			return ConnectResult{Availability: Available, Session: prior}
		}
		resumeEntered, releaseResume := make(chan struct{}), make(chan struct{})
		services.resumeHook = func(context.Context, int, ResumeRequest, *coordinatorIdleToken) ResumeResult {
			close(resumeEntered)
			<-releaseResume
			return ResumeResult{Availability: Available, Session: resumed}
		}
		disposeEntered, releaseDispose := make(chan struct{}), make(chan struct{})
		var disposeOnce sync.Once
		services.disposeHook = func(bool, *ProvisionedWorkload) {
			disposeOnce.Do(func() { close(disposeEntered) })
			_ = prior.Close(context.Background())
			_ = resumed.Close(context.Background())
			<-releaseDispose
		}

		anchor := coordinator.AcquireSecretDemand(context.Background(), "ctx").Lease
		waitCoordinatorState(t, coordinator, CoordinatorActive)
		_ = socket.Close()
		<-resumeEntered

		start := make(chan struct{})
		var group sync.WaitGroup
		group.Add(4)
		go func() {
			defer group.Done()
			<-start
			anchor.Close()
		}()
		go func() {
			defer group.Done()
			<-start
			for index := 0; index < 16; index++ {
				result := coordinator.AcquireSecretDemand(context.Background(), "ctx")
				if result.Lease != nil {
					if sessionLease, ok := result.Lease.TrySession(); ok {
						sessionLease.Close()
					}
					result.Lease.Close()
				}
			}
		}()
		go func() {
			defer group.Done()
			<-start
			coordinator.FenceContextSwitch("ctx")
			coordinator.ContextSwitched("other", true)
		}()
		go func() {
			defer group.Done()
			<-start
			coordinator.Quiesce(context.Background())
		}()
		close(start)
		close(releaseResume)
		group.Wait()
		select {
		case <-disposeEntered:
		case <-time.After(time.Second):
			t.Fatalf("iteration %d disposal did not start", iteration)
		}
		close(releaseDispose)

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		coordinator.StopProducers(ctx)
		cancel()
		coordinator.Close(context.Background())
		select {
		case <-resumed.Done():
		case <-time.After(time.Second):
			t.Fatalf("iteration %d stale Resume remained open", iteration)
		}
		if services.sweepCalls.Load() != 1 || services.provisionCalls.Load() != 1 || services.connectCalls.Load() != 1 || services.resumeCalls.Load() != 1 || services.disposeCalls.Load() != 1 || services.drainCalls.Load() != 0 {
			t.Fatalf("iteration %d sweep=%d provision=%d connect=%d resume=%d dispose=%d drain=%d", iteration, services.sweepCalls.Load(), services.provisionCalls.Load(), services.connectCalls.Load(), services.resumeCalls.Load(), services.disposeCalls.Load(), services.drainCalls.Load())
		}
		anchor.slot.mu.Lock()
		state, demands, sessions := anchor.slot.state, anchor.slot.demandCount, anchor.slot.sessionLeaseCount
		worker, retainedWorkload, retainedSession := anchor.slot.workerDone, anchor.slot.workload, anchor.slot.session
		anchor.slot.mu.Unlock()
		coordinator.disposalMu.Lock()
		disposals := len(coordinator.disposals)
		coordinator.disposalMu.Unlock()
		if state != CoordinatorClosed || demands != 0 || sessions != 0 || worker != nil || retainedWorkload != nil || retainedSession != nil || disposals != 0 {
			t.Fatalf("iteration %d state=%s demand=%d sessions=%d worker=%v workload=%v session=%v disposals=%d", iteration, state, demands, sessions, worker != nil, retainedWorkload != nil, retainedSession != nil, disposals)
		}
	}
}

func TestContextFenceSynchronouslyEstablishesProductionDisposalFence(t *testing.T) {
	_, services, clock := newCoordinatorHarness(t)
	disposer := NewDisposalService(nil)
	cleanupEntered, releaseCleanup := make(chan struct{}), make(chan struct{})
	disposer.cleanupOwned = func(context.Context, *workloadReceipt) (OwnershipStatus, UninstallStatus, DisappearanceStatus) {
		close(cleanupEntered)
		<-releaseCleanup
		return OwnershipProven, UninstallSucceeded, DisappearanceSucceeded
	}
	coordinator := newCoordinator(services, services, services, services, services, disposer, clock)
	workload := coordinatorWorkload(t)
	prior, _ := coordinatorSession(workload, clock, 1)
	slot := newContextSlot("ctx", coordinator.currentEpoch)
	slot.state, slot.workload, slot.session = CoordinatorActive, workload, prior
	coordinator.mu.Lock()
	coordinator.slots[coordinator.currentEpoch] = slot
	coordinator.mu.Unlock()

	coordinator.FenceContextSwitch("ctx")
	if !workload.isDisposing() {
		t.Fatal("context fence returned before 40E disposal fence")
	}
	connector := NewConnector(workload.BuildVersion)
	if result := connector.Connect(context.Background(), workload); result.Reason != WorkloadDisposing {
		t.Fatalf("Connect reason=%s", result.Reason)
	}
	reconnector := NewReconnector(workload.BuildVersion)
	if result := reconnector.Resume(context.Background(), ResumeRequest{Prior: prior, Workload: workload}); result.Reason != ResumeWorkloadDisposing {
		t.Fatalf("Resume reason=%s", result.Reason)
	}
	select {
	case <-cleanupEntered:
	case <-time.After(time.Second):
		t.Fatal("production cleanup did not start")
	}
	close(releaseCleanup)
	deadline := time.After(time.Second)
	for {
		slot.mu.Lock()
		cleared := slot.workload == nil && slot.session == nil
		slot.mu.Unlock()
		if cleared {
			break
		}
		select {
		case <-deadline:
			t.Fatal("disposal completion retained slot authority")
		default:
			time.Sleep(time.Millisecond)
		}
	}
}

func TestContextFenceWaitsForFirstDisposalDelegationBarrier(t *testing.T) {
	for _, dropCompletion := range []bool{false, true} {
		name := "completion"
		if dropCompletion {
			name = "missing completion"
		}
		t.Run(name, func(t *testing.T) {
			_, services, clock := newCoordinatorHarness(t)
			realDisposer := NewDisposalService(nil)
			cleanupEntered, releaseCleanup := make(chan struct{}), make(chan struct{})
			realDisposer.cleanupOwned = func(context.Context, *workloadReceipt) (OwnershipStatus, UninstallStatus, DisappearanceStatus) {
				close(cleanupEntered)
				<-releaseCleanup
				return OwnershipProven, UninstallSucceeded, DisappearanceSucceeded
			}
			wrapper := &blockingCoordinatorAsyncDisposer{
				delegate: realDisposer, startEntered: make(chan struct{}), releaseStart: make(chan struct{}),
				completion: make(chan *disposalCompletion), dropCompletion: dropCompletion,
			}
			coordinator := newCoordinator(services, services, services, services, services, wrapper, clock)
			workload := coordinatorWorkload(t)
			prior, _ := coordinatorSession(workload, clock, 1)
			workload.connectorState.mu.Lock()
			workload.connectorState.currentSession = prior
			workload.connectorState.mu.Unlock()
			slot := newContextSlot("ctx", coordinator.currentEpoch)
			slot.state, slot.workload, slot.session = CoordinatorActive, workload, prior
			coordinator.mu.Lock()
			coordinator.slots[coordinator.currentEpoch] = slot
			coordinator.mu.Unlock()

			firstDone := make(chan struct{})
			go func() {
				coordinator.disposeNowOwned(nil, workload)
				close(firstDone)
			}()
			<-wrapper.startEntered
			fenceReturned := make(chan struct{})
			go func() {
				coordinator.FenceContextSwitch("ctx")
				close(fenceReturned)
			}()

			deadline := time.Now().Add(time.Second)
			for {
				coordinator.disposalMu.Lock()
				op := coordinator.disposals[workload]
				_, joined := op.owners[slot]
				coordinator.disposalMu.Unlock()
				if joined {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("context fence did not join published disposal")
				}
				time.Sleep(time.Millisecond)
			}
			select {
			case <-fenceReturned:
				t.Fatal("context fence returned before real 40E delegation")
			default:
			}

			close(wrapper.releaseStart)
			completion := <-wrapper.completion
			select {
			case <-fenceReturned:
			case <-time.After(time.Second):
				t.Fatal("context fence did not return after real 40E delegation")
			}
			if !workload.isDisposing() {
				t.Fatal("context fence returned before workload disposal fence")
			}
			connector := NewConnector(workload.BuildVersion)
			if result := connector.Connect(context.Background(), workload); result.Reason != WorkloadDisposing {
				t.Fatalf("Connect reason=%s", result.Reason)
			}
			reconnector := NewReconnector(workload.BuildVersion)
			if result := reconnector.Resume(context.Background(), ResumeRequest{Prior: prior, Workload: workload}); result.Reason != ResumeWorkloadDisposing {
				t.Fatalf("Resume reason=%s", result.Reason)
			}
			coordinator.ContextSwitched("other", true)

			select {
			case <-cleanupEntered:
			case <-time.After(time.Second):
				t.Fatal("delegated cleanup did not start")
			}
			if dropCompletion {
				select {
				case <-firstDone:
				case <-time.After(time.Second):
					t.Fatal("missing completion stranded coordinator barrier")
				}
			}
			close(releaseCleanup)
			if completion != nil && completion.Done() != nil {
				<-completion.Done()
			}
			select {
			case <-firstDone:
			case <-time.After(time.Second):
				t.Fatal("coordinator disposal did not complete")
			}

			coordinator.disposalMu.Lock()
			disposals := len(coordinator.disposals)
			coordinator.disposalMu.Unlock()
			coordinator.mu.Lock()
			_, retainedSlot := coordinator.slots[slot.contextEpoch]
			coordinator.mu.Unlock()
			if disposals != 0 || retainedSlot {
				t.Fatalf("map=%d retainedSlot=%v", disposals, retainedSlot)
			}
		})
	}
}

type cleanupBlockingSessionSocket struct {
	readRelease chan struct{}
	closeCalled chan struct{}
	closeOnce   sync.Once
}

func (*cleanupBlockingSessionSocket) SetWriteDeadline(time.Time) error { return nil }
func (*cleanupBlockingSessionSocket) WriteControl(int, []byte, time.Time) error {
	return nil
}
func (*cleanupBlockingSessionSocket) SetPongHandler(func(string) error) {}
func (*cleanupBlockingSessionSocket) SetReadLimit(int64)                {}
func (s *cleanupBlockingSessionSocket) ReadMessage() (int, []byte, error) {
	<-s.readRelease
	return 0, nil, errors.New("peer closed")
}
func (s *cleanupBlockingSessionSocket) Close() error {
	s.closeOnce.Do(func() { close(s.closeCalled) })
	return nil
}
