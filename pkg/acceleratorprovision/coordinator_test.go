package acceleratorprovision

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"k8s.io/client-go/rest"
	"kubikles/pkg/acceleratorrelease"
	"kubikles/pkg/agent"
	"kubikles/pkg/server"
)

type coordinatorSleep struct {
	duration time.Duration
	release  chan struct{}
}

type coordinatorHostileSnapshot struct {
	fakeSnapshot
	config *rest.Config
}

func (s coordinatorHostileSnapshot) RESTConfig() *rest.Config { return s.config }

type controlledCoordinatorClock struct {
	mu     sync.Mutex
	now    time.Time
	sleeps chan coordinatorSleep
}

func newControlledCoordinatorClock() *controlledCoordinatorClock {
	return &controlledCoordinatorClock{now: time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC), sleeps: make(chan coordinatorSleep, 64)}
}
func (c *controlledCoordinatorClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}
func (c *controlledCoordinatorClock) Sleep(ctx context.Context, duration time.Duration) error {
	request := coordinatorSleep{duration: duration, release: make(chan struct{})}
	select {
	case c.sleeps <- request:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case <-request.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (c *controlledCoordinatorClock) advance(t *testing.T, want time.Duration) {
	t.Helper()
	select {
	case request := <-c.sleeps:
		if request.duration != want {
			t.Fatalf("sleep=%s want=%s", request.duration, want)
		}
		c.mu.Lock()
		c.now = c.now.Add(request.duration)
		c.mu.Unlock()
		close(request.release)
	case <-time.After(time.Second):
		t.Fatalf("missing sleep %s", want)
	}
}

type coordinatorTestServices struct {
	t *testing.T

	mu       sync.Mutex
	order    []string
	current  string
	snapshot ContextSnapshot
	clock    coordinatorClock

	resolveHook   func(context.Context, int) acceleratorrelease.Resolution
	provisionHook func(context.Context, int, Request) Result
	connectHook   func(context.Context, int, *ProvisionedWorkload) ConnectResult
	resumeHook    func(context.Context, int, ResumeRequest, *coordinatorIdleToken) ResumeResult
	disposeHook   func(bool, *ProvisionedWorkload)
	disposeResult func(bool, *ProvisionedWorkload) DisposalResult
	sweepHook     func(context.Context, ContextSnapshot)

	resolveCalls, provisionCalls, connectCalls, resumeCalls atomic.Int32
	disposeCalls, drainCalls, sweepCalls                    atomic.Int32
}

func (s *coordinatorTestServices) record(event string) {
	s.mu.Lock()
	s.order = append(s.order, event)
	s.mu.Unlock()
}
func (s *coordinatorTestServices) events() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.order...)
}
func (s *coordinatorTestServices) CurrentContext() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.current
}
func (s *coordinatorTestServices) SnapshotCurrentContext(name string) (ContextSnapshot, error) {
	s.record("snapshot")
	if name != s.CurrentContext() {
		return nil, context.Canceled
	}
	return s.snapshot, nil
}
func (s *coordinatorTestServices) Resolve(ctx context.Context) acceleratorrelease.Resolution {
	call := int(s.resolveCalls.Add(1))
	s.record("resolve")
	if s.resolveHook != nil {
		return s.resolveHook(ctx, call)
	}
	return coordinatorResolution()
}
func (s *coordinatorTestServices) Provision(ctx context.Context, request Request) Result {
	call := int(s.provisionCalls.Add(1))
	s.record("provision")
	if s.provisionHook != nil {
		return s.provisionHook(ctx, call, request)
	}
	return unavailable(ContextUnavailable, CleanupNotNeeded)
}
func (s *coordinatorTestServices) Connect(ctx context.Context, workload *ProvisionedWorkload) ConnectResult {
	call := int(s.connectCalls.Add(1))
	s.record("connect")
	if s.connectHook != nil {
		return s.connectHook(ctx, call, workload)
	}
	return unavailableConnect(InvalidWorkload)
}
func (s *coordinatorTestServices) Resume(ctx context.Context, request ResumeRequest) ResumeResult {
	return s.resume(ctx, request, nil)
}
func (s *coordinatorTestServices) resumeIdle(ctx context.Context, request ResumeRequest, idle *coordinatorIdleToken) ResumeResult {
	return s.resume(ctx, request, idle)
}
func (s *coordinatorTestServices) resume(ctx context.Context, request ResumeRequest, idle *coordinatorIdleToken) ResumeResult {
	call := int(s.resumeCalls.Add(1))
	s.record("resume")
	if s.resumeHook != nil {
		return s.resumeHook(ctx, call, request, idle)
	}
	return unavailableResume(ResumeSessionIneligible)
}
func (s *coordinatorTestServices) SweepInert(ctx context.Context, snapshot ContextSnapshot) SweepResult {
	s.sweepCalls.Add(1)
	s.record("sweep")
	if s.sweepHook != nil {
		s.sweepHook(ctx, snapshot)
	}
	return SweepResult{Status: SweepCompleted}
}
func (s *coordinatorTestServices) DisposeNow(_ context.Context, workload *ProvisionedWorkload) DisposalResult {
	s.disposeCalls.Add(1)
	s.record("dispose")
	if s.disposeHook != nil {
		s.disposeHook(false, workload)
	}
	if s.disposeResult != nil {
		return s.disposeResult(false, workload)
	}
	return DisposalResult{Requested: DisposalImmediate}
}
func (s *coordinatorTestServices) startDisposeNow(_ context.Context, workload *ProvisionedWorkload) *disposalCompletion {
	op := &disposalOperation{done: make(chan struct{}), requested: DisposalImmediate, effective: DisposalImmediate}
	s.disposeCalls.Add(1)
	s.record("dispose")
	go func() {
		if s.disposeHook != nil {
			s.disposeHook(false, workload)
		}
		if s.disposeResult != nil {
			op.result = s.disposeResult(false, workload)
		} else {
			op.result = DisposalResult{Requested: DisposalImmediate}
		}
		close(op.done)
	}()
	return &disposalCompletion{operation: op}
}
func (s *coordinatorTestServices) DrainAndDispose(_ context.Context, workload *ProvisionedWorkload) DisposalResult {
	s.drainCalls.Add(1)
	s.record("drain")
	if s.disposeHook != nil {
		s.disposeHook(true, workload)
	}
	if s.disposeResult != nil {
		return s.disposeResult(true, workload)
	}
	return DisposalResult{Requested: DisposalDrain}
}

func coordinatorResolution() acceleratorrelease.Resolution {
	return acceleratorrelease.Resolution{Availability: acceleratorrelease.Available, Source: acceleratorrelease.SourceNetwork, Release: acceleratorrelease.VerifiedRelease{
		BuildVersion: "v1.2.3", SourceCommit: strings.Repeat("a", 40), DescriptorSHA256: strings.Repeat("b", 64),
		ImageReference: imageRepository + "@sha256:" + strings.Repeat("c", 64), ChartReference: chartRepository + "@sha256:" + strings.Repeat("d", 64),
	}}
}

func newCoordinatorHarness(t *testing.T) (*Coordinator, *coordinatorTestServices, *controlledCoordinatorClock) {
	t.Helper()
	clock := newControlledCoordinatorClock()
	_, job, pod := exactWorkloadFixture()
	services := &coordinatorTestServices{t: t, current: "ctx", snapshot: fakeSnapshot{identity: "snapshot", namespace: "kubikles-system", client: fakeClientset(job, pod)}, clock: clock}
	coordinator := newCoordinator(services, services, services, services, services, services, clock)
	return coordinator, services, clock
}

func coordinatorWorkload(t *testing.T) *ProvisionedWorkload {
	t.Helper()
	return connectorWorkload(t)
}

func coordinatorSession(workload *ProvisionedWorkload, clock resumeClock, generation int) (*ConnectedSession, *recordedSessionSocket) {
	order := []string{}
	socket := newRecordedSessionSocket(&order)
	tunnel := newRecordedTunnel(&order)
	session := newConnectedSession(workload.connectorState.receipt, server.AuthenticatedAcceleratorInfo{Capabilities: agent.V1Capabilities()}, connectedIdentity{sessionID: "session-a", instanceID: "instance-a", generation: generation}, socket, tunnel, clock)
	return session, socket
}

func waitCoordinatorState(t *testing.T, coordinator *Coordinator, state CoordinatorState) CoordinatorSnapshot {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		snapshot := coordinator.Snapshot("ctx")
		if snapshot.State == state {
			return snapshot
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("state=%s want=%s events=%v", coordinator.Snapshot("ctx").State, state, coordinator)
	return CoordinatorSnapshot{}
}

func stopCoordinator(t *testing.T, coordinator *Coordinator) {
	t.Helper()
	coordinator.Quiesce(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	coordinator.StopProducers(ctx)
	coordinator.Close(context.Background())
}

func TestFirstDemandActivationOrderAndDirectAvailability(t *testing.T) {
	coordinator, services, clock := newCoordinatorHarness(t)
	sweepEntered, releaseSweep := make(chan struct{}), make(chan struct{})
	services.sweepHook = func(context.Context, ContextSnapshot) { close(sweepEntered); <-releaseSweep }
	workload := coordinatorWorkload(t)
	session, _ := coordinatorSession(workload, clock, 1)
	services.provisionHook = func(context.Context, int, Request) Result { return available(workload) }
	services.connectHook = func(context.Context, int, *ProvisionedWorkload) ConnectResult {
		return ConnectResult{Availability: Available, Session: session}
	}

	result := coordinator.AcquireSecretDemand(context.Background(), "ctx")
	if !result.Accepted || result.Lease == nil {
		t.Fatalf("demand=%#v", result)
	}
	change := result.Lease.Changes()
	select {
	case <-sweepEntered:
	case <-time.After(time.Second):
		t.Fatal("sweep did not start")
	}
	if _, ok := result.Lease.TrySession(); ok {
		t.Fatal("session published before Connect")
	}
	var extra []*SecretDemandLease
	for index := 0; index < 32; index++ {
		extra = append(extra, coordinator.AcquireSecretDemand(context.Background(), "ctx").Lease)
	}
	if services.sweepCalls.Load() != 1 || services.resolveCalls.Load() != 0 {
		t.Fatalf("sweep=%d resolve=%d", services.sweepCalls.Load(), services.resolveCalls.Load())
	}
	close(releaseSweep)
	waitCoordinatorState(t, coordinator, CoordinatorActive)
	select {
	case <-change:
	case <-time.After(time.Second):
		t.Fatal("active edge not signalled")
	}
	lease, ok := result.Lease.TrySession()
	if !ok || lease.Session() != session {
		t.Fatal("active session not leased")
	}
	if got := services.events(); !reflect.DeepEqual(got[:5], []string{"snapshot", "sweep", "resolve", "provision", "connect"}) {
		t.Fatalf("order=%v", got)
	}
	for _, item := range extra {
		item.Close()
	}
	result.Lease.Close()
	if coordinator.Snapshot("ctx").State != CoordinatorActive {
		t.Fatal("nested session lease did not pin active state")
	}
	lease.Close()
	stopCoordinator(t, coordinator)
}

func TestActivationCompletionFencesAndDisposesStaleOwnedResult(t *testing.T) {
	coordinator, services, clock := newCoordinatorHarness(t)
	workload := coordinatorWorkload(t)
	session, _ := coordinatorSession(workload, clock, 1)
	entered, release := make(chan struct{}), make(chan struct{})
	services.provisionHook = func(context.Context, int, Request) Result { return available(workload) }
	services.connectHook = func(context.Context, int, *ProvisionedWorkload) ConnectResult {
		close(entered)
		<-release
		return ConnectResult{Availability: Available, Session: session}
	}
	demand := coordinator.AcquireSecretDemand(context.Background(), "ctx").Lease
	<-entered
	demand.Close()
	close(release)
	deadline := time.Now().Add(time.Second)
	for services.disposeCalls.Load() != 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if services.disposeCalls.Load() != 1 || coordinator.Snapshot("ctx").Available {
		t.Fatalf("dispose=%d snapshot=%#v", services.disposeCalls.Load(), coordinator.Snapshot("ctx"))
	}
	select {
	case <-session.Done():
	case <-time.After(time.Second):
		t.Fatal("stale session not closed")
	}
	stopCoordinator(t, coordinator)
}

func TestVersionMismatchRecreatesExactlyOnce(t *testing.T) {
	coordinator, services, clock := newCoordinatorHarness(t)
	workloads := []*ProvisionedWorkload{coordinatorWorkload(t), coordinatorWorkload(t), coordinatorWorkload(t)}
	services.provisionHook = func(_ context.Context, call int, _ Request) Result { return available(workloads[call-1]) }
	services.connectHook = func(_ context.Context, call int, workload *ProvisionedWorkload) ConnectResult {
		if call < 3 {
			return unavailableConnect(ConnectVersionMismatch)
		}
		session, _ := coordinatorSession(workload, clock, call)
		return ConnectResult{Availability: Available, Session: session}
	}
	demand := coordinator.AcquireSecretDemand(context.Background(), "ctx").Lease
	waitCoordinatorState(t, coordinator, CoordinatorUnavailable)
	time.Sleep(10 * time.Millisecond)
	if services.provisionCalls.Load() != 2 || services.connectCalls.Load() != 2 || services.disposeCalls.Load() != 2 {
		t.Fatalf("provision=%d connect=%d dispose=%d", services.provisionCalls.Load(), services.connectCalls.Load(), services.disposeCalls.Load())
	}
	demand.Close()
	newDemand := coordinator.AcquireSecretDemand(context.Background(), "ctx").Lease
	clock.advance(t, UnavailableCooldown)
	waitCoordinatorState(t, coordinator, CoordinatorActive)
	if services.provisionCalls.Load() != 3 || services.connectCalls.Load() != 3 {
		t.Fatalf("new epoch provision=%d connect=%d", services.provisionCalls.Load(), services.connectCalls.Load())
	}
	newDemand.Close()
	stopCoordinator(t, coordinator)
}

func TestCoordinatorRetryTimesAndClosedClassification(t *testing.T) {
	coordinator, services, clock := newCoordinatorHarness(t)
	var times []time.Time
	services.resolveHook = func(context.Context, int) acceleratorrelease.Resolution {
		times = append(times, clock.Now())
		if len(times) <= 3 {
			return acceleratorrelease.Resolution{Availability: acceleratorrelease.Unavailable, Reason: acceleratorrelease.NetworkUnavailable}
		}
		return coordinatorResolution()
	}
	workload := coordinatorWorkload(t)
	session, _ := coordinatorSession(workload, clock, 1)
	services.provisionHook = func(context.Context, int, Request) Result { return available(workload) }
	services.connectHook = func(context.Context, int, *ProvisionedWorkload) ConnectResult {
		return ConnectResult{Availability: Available, Session: session}
	}
	demand := coordinator.AcquireSecretDemand(context.Background(), "ctx").Lease
	clock.advance(t, time.Second)
	clock.advance(t, 2*time.Second)
	clock.advance(t, UnavailableCooldown)
	waitCoordinatorState(t, coordinator, CoordinatorActive)
	base := times[0]
	want := []time.Duration{0, time.Second, 3 * time.Second, 33 * time.Second}
	for index := range want {
		if got := times[index].Sub(base); got != want[index] {
			t.Fatalf("attempt %d=%s want=%s", index, got, want[index])
		}
	}
	if classifyReleaseFailure(acceleratorrelease.UnavailableReason("future")) != failureAuthoritative || classifyProvisionFailure(UnavailableReason("future")) != failureAuthoritative || classifyConnectFailure(ConnectUnavailableReason("future")) != failureAuthoritative || classifyResumeFailure(ResumeReason("future")) != failureAuthoritative {
		t.Fatal("unknown child reason was retryable")
	}
	demand.Close()
	stopCoordinator(t, coordinator)
}

func TestCoordinatorFailureClassificationClosedTable(t *testing.T) {
	release := map[acceleratorrelease.UnavailableReason]failureClass{
		acceleratorrelease.InvalidLocalBuild: failureAuthoritative, acceleratorrelease.DescriptorMissing: failureAuthoritative,
		acceleratorrelease.NetworkUnavailable: failureTemporary, acceleratorrelease.OnlineIntegrity: failureAuthoritative,
		acceleratorrelease.CacheInvalid: failureAuthoritative, acceleratorrelease.CacheIO: failureTemporary,
	}
	for reason, want := range release {
		if got := classifyReleaseFailure(reason); got != want {
			t.Fatalf("release %s=%d want=%d", reason, got, want)
		}
	}
	provision := map[UnavailableReason]failureClass{
		ArtifactUnavailable: failureAuthoritative, ContextUnavailable: failureAuthoritative, ContextChanged: failureCancelled,
		EntropyUnavailable: failureAuthoritative, ChartPullFailed: failureTemporary, ChartIntegrityFailed: failureAuthoritative,
		RenderFailed: failureAuthoritative, ReleaseConflict: failureAuthoritative, PermissionDenied: failureAuthoritative,
		InstallFailed: failureTemporary, JobFailed: failureAuthoritative, PodFailed: failureAuthoritative,
		ImagePullFailed: failureTemporary, TimedOut: failureTemporary, Cancelled: failureCancelled,
	}
	for reason, want := range provision {
		if got := classifyProvisionFailure(reason); got != want {
			t.Fatalf("provision %s=%d want=%d", reason, got, want)
		}
	}
	connect := map[ConnectUnavailableReason]failureClass{
		InvalidWorkload: failureAuthoritative, WorkloadChanged: failureAuthoritative, WorkloadUnavailable: failureAuthoritative,
		TunnelUnavailable: failureTemporary, AcceleratorUnavailable: failureTemporary, ConnectVersionMismatch: failureVersionMismatch,
		ConnectCancelled: failureCancelled, WorkloadDisposing: failureCancelled,
	}
	for reason, want := range connect {
		if got := classifyConnectFailure(reason); got != want {
			t.Fatalf("connect %s=%d want=%d", reason, got, want)
		}
	}
	resume := map[ResumeReason]failureClass{
		ResumeInvalid: failureAuthoritative, ResumeSessionIneligible: failureAuthoritative, ResumeSuperseded: failureAuthoritative,
		ResumeWorkloadTerminalOrChanged: failureAuthoritative, ResumeAuthenticationFailed: failureAuthoritative,
		ResumeIdentityMismatch: failureAuthoritative, ResumeProtocolFailed: failureAuthoritative,
		ResumeVersionMismatch: failureVersionMismatch, ResumeTransportUnavailable: failureTemporary,
		ResumeCancelled: failureCancelled, ResumeGraceExpired: failureAuthoritative,
		ResumeExplicitlyClosed: failureAuthoritative, ResumeWorkloadDisposing: failureCancelled,
	}
	for reason, want := range resume {
		if got := classifyResumeFailure(reason); got != want {
			t.Fatalf("resume %s=%d want=%d", reason, got, want)
		}
	}
}

func TestFailedWorkloadDisposedBeforeRetry(t *testing.T) {
	coordinator, services, clock := newCoordinatorHarness(t)
	workloads := []*ProvisionedWorkload{coordinatorWorkload(t), coordinatorWorkload(t)}
	disposeEntered, releaseDispose := make(chan struct{}), make(chan struct{})
	services.provisionHook = func(_ context.Context, call int, _ Request) Result { return available(workloads[call-1]) }
	services.connectHook = func(_ context.Context, call int, workload *ProvisionedWorkload) ConnectResult {
		if call == 1 {
			return unavailableConnect(TunnelUnavailable)
		}
		session, _ := coordinatorSession(workload, clock, 1)
		return ConnectResult{Availability: Available, Session: session}
	}
	services.disposeHook = func(bool, *ProvisionedWorkload) { close(disposeEntered); <-releaseDispose }
	demand := coordinator.AcquireSecretDemand(context.Background(), "ctx").Lease
	<-disposeEntered
	if services.provisionCalls.Load() != 1 {
		t.Fatal("provision overlapped disposal")
	}
	close(releaseDispose)
	clock.advance(t, time.Second)
	waitCoordinatorState(t, coordinator, CoordinatorActive)
	if services.provisionCalls.Load() != 2 {
		t.Fatalf("provision calls=%d", services.provisionCalls.Load())
	}
	demand.Close()
	services.disposeHook = nil
	stopCoordinator(t, coordinator)
}

func TestTransportLossRevokesBeforeResume(t *testing.T) {
	coordinator, services, clock := newCoordinatorHarness(t)
	workload := coordinatorWorkload(t)
	first, firstSocket := coordinatorSession(workload, clock, 1)
	second, _ := coordinatorSession(workload, clock, 2)
	services.provisionHook = func(context.Context, int, Request) Result { return available(workload) }
	services.connectHook = func(context.Context, int, *ProvisionedWorkload) ConnectResult {
		return ConnectResult{Availability: Available, Session: first}
	}
	resumeEntered, releaseResume := make(chan struct{}), make(chan struct{})
	services.resumeHook = func(context.Context, int, ResumeRequest, *coordinatorIdleToken) ResumeResult {
		close(resumeEntered)
		<-releaseResume
		return ResumeResult{Availability: Available, Session: second}
	}
	demand := coordinator.AcquireSecretDemand(context.Background(), "ctx").Lease
	waitCoordinatorState(t, coordinator, CoordinatorActive)
	change := demand.Changes()
	_ = firstSocket.Close()
	select {
	case <-resumeEntered:
	case <-time.After(time.Second):
		t.Fatal("Resume did not start")
	}
	select {
	case <-change:
	default:
		t.Fatal("availability was not revoked before Resume")
	}
	if _, ok := demand.TrySession(); ok {
		t.Fatal("session leased during Resume")
	}
	close(releaseResume)
	waitCoordinatorState(t, coordinator, CoordinatorActive)
	lease, ok := demand.TrySession()
	if !ok || lease.Session().Identity().Generation != 2 {
		t.Fatal("higher Resume generation not published")
	}
	lease.Close()
	demand.Close()
	stopCoordinator(t, coordinator)
}

func TestStaleResumeSuccessCannotPublishOrDoubleDispose(t *testing.T) {
	coordinator, services, clock := newCoordinatorHarness(t)
	workload := coordinatorWorkload(t)
	first, firstSocket := coordinatorSession(workload, clock, 1)
	stale, _ := coordinatorSession(workload, clock, 2)
	services.provisionHook = func(context.Context, int, Request) Result { return available(workload) }
	services.connectHook = func(context.Context, int, *ProvisionedWorkload) ConnectResult {
		return ConnectResult{Availability: Available, Session: first}
	}
	resumeEntered, releaseResume := make(chan struct{}), make(chan struct{})
	services.resumeHook = func(context.Context, int, ResumeRequest, *coordinatorIdleToken) ResumeResult {
		close(resumeEntered)
		<-releaseResume
		return ResumeResult{Availability: Available, Session: stale}
	}
	_ = coordinator.AcquireSecretDemand(context.Background(), "ctx")
	waitCoordinatorState(t, coordinator, CoordinatorActive)
	_ = firstSocket.Close()
	<-resumeEntered
	coordinator.FenceContextSwitch("ctx")
	close(releaseResume)
	select {
	case <-stale.Done():
	case <-time.After(time.Second):
		t.Fatal("stale Resume session was not closed")
	}
	deadline := time.Now().Add(time.Second)
	for services.disposeCalls.Load() < 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	time.Sleep(20 * time.Millisecond)
	if services.disposeCalls.Load() != 1 || coordinator.Snapshot("ctx").Available {
		t.Fatalf("dispose=%d snapshot=%#v", services.disposeCalls.Load(), coordinator.Snapshot("ctx"))
	}
}

func TestFinalDemandIdleReleaseAndGraceReturn(t *testing.T) {
	coordinator, services, clock := newCoordinatorHarness(t)
	workload := coordinatorWorkload(t)
	first, firstSocket := coordinatorSession(workload, clock, 1)
	second, _ := coordinatorSession(workload, clock, 2)
	services.provisionHook = func(context.Context, int, Request) Result { return available(workload) }
	services.connectHook = func(context.Context, int, *ProvisionedWorkload) ConnectResult {
		return ConnectResult{Availability: Available, Session: first}
	}
	services.resumeHook = func(_ context.Context, _ int, request ResumeRequest, idle *coordinatorIdleToken) ResumeResult {
		if idle == nil || request.Workload != workload || request.Prior != first {
			t.Fatal("idle Resume lost exact ownership")
		}
		return ResumeResult{Availability: Available, Session: second}
	}
	demand := coordinator.AcquireSecretDemand(context.Background(), "ctx").Lease
	waitCoordinatorState(t, coordinator, CoordinatorActive)
	sessionLease, _ := demand.TrySession()
	demand.Close()
	if coordinator.Snapshot("ctx").State != CoordinatorActive {
		t.Fatal("nested lease did not delay idle release")
	}
	sessionLease.Close()
	waitCoordinatorState(t, coordinator, CoordinatorDraining)
	var sleep coordinatorSleep
	select {
	case sleep = <-clock.sleeps:
	case <-time.After(time.Second):
		t.Fatal("idle deadline waiter missing")
	}
	if sleep.duration != agent.AcceleratorIdleReconnectGrace || agent.AcceleratorIdleReconnectGrace != 2*time.Minute {
		t.Fatalf("idle wait=%s", sleep.duration)
	}
	_ = firstSocket.Close() // Idle release already won the terminal arbiter.
	returned := coordinator.AcquireSecretDemand(context.Background(), "ctx").Lease
	waitCoordinatorState(t, coordinator, CoordinatorActive)
	close(sleep.release) // stale delivered timer must be fenced.
	time.Sleep(time.Millisecond)
	if services.drainCalls.Load() != 0 || services.provisionCalls.Load() != 1 {
		t.Fatalf("drain=%d provision=%d", services.drainCalls.Load(), services.provisionCalls.Load())
	}
	returned.Close()
	stopCoordinator(t, coordinator)
}

func TestGraceDeadlineWinsDemandRaceOnlyAfterDisposalFence(t *testing.T) {
	coordinator, services, clock := newCoordinatorHarness(t)
	first, second := coordinatorWorkload(t), coordinatorWorkload(t)
	firstSession, _ := coordinatorSession(first, clock, 1)
	secondSession, _ := coordinatorSession(second, clock, 1)
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
	disposeEntered, releaseDispose := make(chan struct{}), make(chan struct{})
	services.disposeHook = func(drain bool, workload *ProvisionedWorkload) {
		if drain && workload == first {
			close(disposeEntered)
			<-releaseDispose
		}
	}

	initial := coordinator.AcquireSecretDemand(context.Background(), "ctx").Lease
	waitCoordinatorState(t, coordinator, CoordinatorActive)
	initial.Close()
	clock.advance(t, agent.AcceleratorIdleReconnectGrace)
	<-disposeEntered
	returned := coordinator.AcquireSecretDemand(context.Background(), "ctx").Lease
	if services.resumeCalls.Load() != 0 || services.provisionCalls.Load() != 1 {
		t.Fatalf("post-fence demand resume=%d provision=%d", services.resumeCalls.Load(), services.provisionCalls.Load())
	}
	close(releaseDispose)
	waitCoordinatorState(t, coordinator, CoordinatorActive)
	if services.drainCalls.Load() != 1 || services.disposeCalls.Load() != 0 || services.provisionCalls.Load() != 2 || services.resumeCalls.Load() != 0 {
		t.Fatalf("drain=%d dispose=%d provision=%d resume=%d", services.drainCalls.Load(), services.disposeCalls.Load(), services.provisionCalls.Load(), services.resumeCalls.Load())
	}
	returned.Close()
	services.disposeHook = nil
	stopCoordinator(t, coordinator)
}

func TestOnlyCoordinatorIdleReleaseIsResumable(t *testing.T) {
	clock := &fakeResumeClock{now: time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)}
	workload := coordinatorWorkload(t)
	session, _ := coordinatorSession(workload, clock, 1)
	release := session.releaseForIdle(context.Background())
	if release == nil || release.deadline != clock.Now().Add(agent.AcceleratorIdleReconnectGrace) {
		t.Fatal("idle release omitted authoritative deadline")
	}
	if _, _, reason := session.claimResume(workload); reason != ResumeSessionIneligible {
		t.Fatalf("public Resume claimed idle session: %s", reason)
	}
	if _, _, reason := session.claimResumeWithIdle(workload, &coordinatorIdleToken{}); reason != ResumeSessionIneligible {
		t.Fatalf("forged idle token accepted: %s", reason)
	}
	if _, _, reason := session.claimResumeWithIdle(workload, release.token); reason != "" {
		t.Fatalf("owned idle token rejected: %s", reason)
	}
	closedWorkload := coordinatorWorkload(t)
	closed, _ := coordinatorSession(closedWorkload, clock, 1)
	_ = closed.Close(context.Background())
	if _, _, reason := closed.claimResumeWithIdle(closedWorkload, release.token); reason != ResumeSessionIneligible {
		t.Fatalf("public Close became resumable: %s", reason)
	}
}

func TestGraceExpiryInvokesDrainAndDisposeOnce(t *testing.T) {
	coordinator, services, clock := newCoordinatorHarness(t)
	workload := coordinatorWorkload(t)
	session, _ := coordinatorSession(workload, clock, 1)
	services.provisionHook = func(context.Context, int, Request) Result { return available(workload) }
	services.connectHook = func(context.Context, int, *ProvisionedWorkload) ConnectResult {
		return ConnectResult{Availability: Available, Session: session}
	}
	demand := coordinator.AcquireSecretDemand(context.Background(), "ctx").Lease
	waitCoordinatorState(t, coordinator, CoordinatorActive)
	demand.Close()
	clock.advance(t, agent.AcceleratorIdleReconnectGrace)
	waitCoordinatorState(t, coordinator, CoordinatorDirectOnly)
	if services.drainCalls.Load() != 1 || services.disposeCalls.Load() != 0 {
		t.Fatalf("drain=%d dispose=%d", services.drainCalls.Load(), services.disposeCalls.Load())
	}
	stopCoordinator(t, coordinator)
}

func TestCoordinatorEventRaceMatrix(t *testing.T) {
	coordinator, services, clock := newCoordinatorHarness(t)
	workload := coordinatorWorkload(t)
	session, socket := coordinatorSession(workload, clock, 1)
	services.provisionHook = func(context.Context, int, Request) Result { return available(workload) }
	services.connectHook = func(context.Context, int, *ProvisionedWorkload) ConnectResult {
		return ConnectResult{Availability: Available, Session: session}
	}
	anchor := coordinator.AcquireSecretDemand(context.Background(), "ctx").Lease
	waitCoordinatorState(t, coordinator, CoordinatorActive)
	var group sync.WaitGroup
	for index := 0; index < 200; index++ {
		group.Add(1)
		go func() {
			defer group.Done()
			lease := coordinator.AcquireSecretDemand(context.Background(), "ctx").Lease
			if sessionLease, ok := lease.TrySession(); ok {
				sessionLease.Close()
			}
			lease.Close()
			lease.Close()
		}()
	}
	group.Wait()
	if services.provisionCalls.Load() != 1 || services.sweepCalls.Load() != 1 {
		t.Fatalf("provision=%d sweep=%d", services.provisionCalls.Load(), services.sweepCalls.Load())
	}
	_ = socket.Close()
	anchor.Close()
	stopCoordinator(t, coordinator)
}

func TestContextSwitchFencesAndReusesExactSweepTombstone(t *testing.T) {
	coordinator, services, clock := newCoordinatorHarness(t)
	workloads := []*ProvisionedWorkload{coordinatorWorkload(t), coordinatorWorkload(t)}
	services.provisionHook = func(_ context.Context, call int, _ Request) Result { return available(workloads[call-1]) }
	services.connectHook = func(_ context.Context, call int, workload *ProvisionedWorkload) ConnectResult {
		session, _ := coordinatorSession(workload, clock, call)
		return ConnectResult{Availability: Available, Session: session}
	}
	_ = coordinator.AcquireSecretDemand(context.Background(), "ctx")
	waitCoordinatorState(t, coordinator, CoordinatorActive)
	disposeEntered, releaseDispose := make(chan struct{}), make(chan struct{})
	var enteredOnce sync.Once
	services.disposeHook = func(bool, *ProvisionedWorkload) { enteredOnce.Do(func() { close(disposeEntered) }); <-releaseDispose }
	returned := make(chan struct{})
	go func() {
		coordinator.FenceContextSwitch("ctx")
		close(returned)
	}()
	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("context fence waited for disposal")
	}
	select {
	case <-disposeEntered:
	case <-time.After(time.Second):
		t.Fatal("context fence did not start disposal")
	}
	coordinator.ContextSwitched("other", true)
	coordinator.FenceContextSwitch("other")
	coordinator.ContextSwitched("ctx", true)
	close(releaseDispose)
	_ = coordinator.AcquireSecretDemand(context.Background(), "ctx")
	waitCoordinatorState(t, coordinator, CoordinatorActive)
	if services.sweepCalls.Load() != 1 || services.provisionCalls.Load() != 2 {
		t.Fatalf("sweep=%d provision=%d", services.sweepCalls.Load(), services.provisionCalls.Load())
	}
	stopCoordinator(t, coordinator)
}

func TestCoordinatorShutdownDisposesContextsConcurrentlyWithin95Seconds(t *testing.T) {
	coordinator, services, clock := newCoordinatorHarness(t)
	first, second := coordinatorWorkload(t), coordinatorWorkload(t)
	coordinator.mu.Lock()
	coordinator.slots[1] = newContextSlot("one", 1)
	coordinator.slots[2] = newContextSlot("two", 2)
	coordinator.slots[1].workload = first
	coordinator.slots[2].workload = second
	coordinator.mu.Unlock()
	entered := make(chan *ProvisionedWorkload, 2)
	release := make(chan struct{})
	services.disposeHook = func(_ bool, workload *ProvisionedWorkload) {
		entered <- workload
		<-release
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	closed := make(chan struct{})
	go func() {
		coordinator.Close(cancelled)
		close(closed)
	}()
	seen := map[*ProvisionedWorkload]bool{}
	for index := 0; index < 2; index++ {
		select {
		case workload := <-entered:
			seen[workload] = true
		case <-time.After(time.Second):
			t.Fatal("shutdown disposals were not concurrent")
		}
	}
	if !seen[first] || !seen[second] {
		t.Fatal("shutdown omitted retained workload")
	}
	clock.advance(t, ShutdownWaitTimeout)
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("shutdown exceeded injected 95 second bound")
	}
	close(release)
}

func TestCoordinatorRedactionAndDirectOnlyNonRegression(t *testing.T) {
	coordinator, services, clock := newCoordinatorHarness(t)
	var capturedLogs bytes.Buffer
	priorLogWriter := log.Writer()
	log.SetOutput(&capturedLogs)
	defer log.SetOutput(priorLogWriter)
	hostile := []string{
		"Bearer hostile-token", "Authorization: hostile", "https://descriptor.invalid/private", "127.0.0.1:43123",
		"hostile-session-id", "hostile-instance-id", "hostile-workload-id", "kind: Secret\ndata: hostile",
		"apiVersion: v1\nkind: Config\nusers: hostile", "raw child error hostile", "2026-08-02T12:00:00Z",
		"raw Helm manifest hostile", "raw Helm values hostile", "raw descriptor body hostile",
	}
	workload := coordinatorWorkload(t)
	hostile = append(hostile, knownCreatorToken, workload.credential.verifier, workload.credential.session)
	workload.connectorState.receipt.prepared = &preparedChart{implementation: []string{hostile[11], hostile[12]}}
	workload.connectorState.receipt.owned = &ownedRelease{implementation: errors.New(hostile[9])}
	receipt := *workload.connectorState.receipt
	receipt.workloadSessionID = hostile[6]
	order := []string{}
	session := newConnectedSession(&receipt, server.AuthenticatedAcceleratorInfo{Capabilities: agent.V1Capabilities()}, connectedIdentity{sessionID: hostile[4], instanceID: hostile[5], generation: 1}, newRecordedSessionSocket(&order), newRecordedTunnel(&order), clock)
	services.snapshot = coordinatorHostileSnapshot{
		fakeSnapshot: fakeSnapshot{identity: hostile[2], namespace: "kubikles-system"},
		config:       &rest.Config{Host: "https://rest-config.invalid/private", BearerToken: hostile[0], UserAgent: hostile[1], TLSClientConfig: rest.TLSClientConfig{CAData: []byte(hostile[8])}},
	}
	hostile = append(hostile, services.snapshot.RESTConfig().Host)
	session.frames <- []byte(hostile[7])
	services.provisionHook = func(context.Context, int, Request) Result { return available(workload) }
	services.connectHook = func(context.Context, int, *ProvisionedWorkload) ConnectResult {
		return ConnectResult{Availability: Available, Session: session}
	}
	result := coordinator.AcquireSecretDemand(context.Background(), "ctx")
	waitCoordinatorState(t, coordinator, CoordinatorActive)
	snapshot := coordinator.Snapshot("ctx")
	sessionLease, ok := result.Lease.TrySession()
	if !ok {
		t.Fatal("active session was not leaseable")
	}
	encodedResult, _ := json.Marshal(result)
	encodedLease, _ := json.Marshal(result.Lease)
	encodedSessionLease, _ := json.Marshal(sessionLease)
	encodedSession, _ := json.Marshal(session)
	encodedIdentity, _ := json.Marshal(session.Identity())
	corpus := []string{
		fmt.Sprint(result), fmt.Sprintf("%+v", result.Lease), fmt.Sprintf("%#v", sessionLease), fmt.Sprint(snapshot),
		fmt.Sprintf("%#v", session), fmt.Sprintf("%+v", session.Identity()), string(encodedResult), string(encodedLease),
		string(encodedSessionLease), string(encodedSession), string(encodedIdentity),
	}
	sessionLease.Close()
	result.Lease.Close()
	stopCoordinator(t, coordinator)
	corpus = append(corpus, fmt.Sprint(services.events()), capturedLogs.String())
	for _, rendered := range corpus {
		for _, secret := range hostile {
			if strings.Contains(rendered, secret) {
				t.Fatalf("unsafe coordinator output %q", rendered)
			}
		}
	}
}
