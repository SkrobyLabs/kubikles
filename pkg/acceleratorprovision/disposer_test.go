package acceleratorprovision

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestDisposeIsOneShotAndImmediateDominatesDrain(t *testing.T) {
	workload := connectorWorkload(t)
	service := NewDisposalService(nil)
	started := make(chan struct{})
	var once sync.Once
	var observations, cleanups atomic.Int32
	service.observeDrain = func(ctx context.Context, _ *workloadReceipt, force <-chan struct{}, settle func(DrainObservationStatus) DrainObservationStatus) DrainObservationStatus {
		observations.Add(1)
		once.Do(func() { close(started) })
		select {
		case <-force:
			return settle(DrainEscalated)
		case <-ctx.Done():
			return settle(DrainTimedOut)
		}
	}
	service.cleanupOwned = func(context.Context, *workloadReceipt) (OwnershipStatus, UninstallStatus, DisappearanceStatus) {
		cleanups.Add(1)
		return OwnershipProven, UninstallSucceeded, DisappearanceSucceeded
	}

	drainResult := make(chan DisposalResult, 1)
	go func() { drainResult <- service.DrainAndDispose(context.Background(), workload) }()
	<-started
	now := service.DisposeNow(context.Background(), workload)
	drain := <-drainResult
	if now != drain || drain.Effective != DisposalImmediateEscalation || drain.Observation != DrainEscalated {
		t.Fatalf("now=%#v drain=%#v", now, drain)
	}
	if observations.Load() != 1 || cleanups.Load() != 1 {
		t.Fatalf("observations=%d cleanups=%d", observations.Load(), cleanups.Load())
	}
	if late := service.DrainAndDispose(context.Background(), workload); late != drain || cleanups.Load() != 1 {
		t.Fatalf("late=%#v cleanups=%d", late, cleanups.Load())
	}
	if workload.credential.withCreatorAuthorization(context.Background(), func(context.Context, creatorAuthorizationLease) error { return nil }) == nil {
		t.Fatal("disposed credential remained usable")
	}
}

func TestDisposeFencesNewLeasesBeforeCleanupIO(t *testing.T) {
	workload := connectorWorkload(t)
	service := NewDisposalService(nil)
	entered := make(chan struct{})
	release := make(chan struct{})
	service.cleanupOwned = func(context.Context, *workloadReceipt) (OwnershipStatus, UninstallStatus, DisappearanceStatus) {
		close(entered)
		<-release
		return OwnershipProven, UninstallSucceeded, DisappearanceSucceeded
	}
	done := make(chan DisposalResult, 1)
	go func() { done <- service.DisposeNow(context.Background(), workload) }()
	<-entered
	if _, ok := workload.connectorLease(); ok {
		t.Fatal("disposed workload granted a fresh connector lease")
	}
	if _, ok := workload.resumeConnectorLease(workload.connectorState.receipt); ok {
		t.Fatal("disposed workload granted a resume lease")
	}
	if result := NewConnector("v1.2.3").Connect(context.Background(), workload); result.Reason != WorkloadDisposing {
		t.Fatalf("connect reason=%s", result.Reason)
	}
	close(release)
	result := <-done
	if result.Quiescence != QuiescenceSucceeded || result.Credential != CredentialDestroyed {
		t.Fatalf("result=%#v", result)
	}
}

func TestDisposeCancelsRegisteredOperationAndIgnoresCallerCancellation(t *testing.T) {
	workload := connectorWorkload(t)
	op, finish, reason := workload.beginLifecycleOperation(context.Background())
	if reason != "" {
		t.Fatal(reason)
	}
	service := NewDisposalService(nil)
	service.cleanupOwned = func(context.Context, *workloadReceipt) (OwnershipStatus, UninstallStatus, DisappearanceStatus) {
		return OwnershipProven, UninstallSucceeded, DisappearanceSucceeded
	}
	caller, cancelCaller := context.WithCancel(context.Background())
	cancelCaller()
	done := make(chan DisposalResult, 1)
	go func() { done <- service.DisposeNow(caller, workload) }()
	<-op.Done()
	finish()
	result := <-done
	if result.Quiescence != QuiescenceSucceeded || result.Uninstall != UninstallSucceeded {
		t.Fatalf("result=%#v", result)
	}
}

func TestLateImmediateReturnsTerminalDrainWithoutRewritingIt(t *testing.T) {
	workload := connectorWorkload(t)
	service := NewDisposalService(nil)
	service.observeDrain = func(_ context.Context, _ *workloadReceipt, _ <-chan struct{}, settle func(DrainObservationStatus) DrainObservationStatus) DrainObservationStatus {
		return settle(DrainComplete)
	}
	service.cleanupOwned = func(context.Context, *workloadReceipt) (OwnershipStatus, UninstallStatus, DisappearanceStatus) {
		return OwnershipProven, UninstallSucceeded, DisappearanceSucceeded
	}
	drained := service.DrainAndDispose(context.Background(), workload)
	late := service.DisposeNow(context.Background(), workload)
	if late != drained || late.Effective != DisposalDrain || late.Observation != DrainComplete {
		t.Fatalf("drained=%#v late=%#v", drained, late)
	}
}

func TestDisposalUsesExactInjectedPhaseBudgets(t *testing.T) {
	workload := connectorWorkload(t)
	service := NewDisposalService(nil)
	var mu sync.Mutex
	var durations []time.Duration
	service.phaseContext = func(parent context.Context, duration time.Duration) (context.Context, context.CancelFunc) {
		mu.Lock()
		durations = append(durations, duration)
		mu.Unlock()
		return context.WithCancel(parent)
	}
	service.observeDrain = func(_ context.Context, _ *workloadReceipt, _ <-chan struct{}, settle func(DrainObservationStatus) DrainObservationStatus) DrainObservationStatus {
		return settle(DrainComplete)
	}
	service.cleanupOwned = func(context.Context, *workloadReceipt) (OwnershipStatus, UninstallStatus, DisappearanceStatus) {
		return OwnershipAlreadyGone, UninstallNotNeeded, DisappearanceSucceeded
	}
	result := service.DrainAndDispose(context.Background(), workload)
	if result.Observation != DrainComplete || result.Ownership != OwnershipAlreadyGone {
		t.Fatalf("result=%#v", result)
	}
	mu.Lock()
	defer mu.Unlock()
	want := []time.Duration{TransportQuiesceTimeout, 2*time.Minute + 45*time.Second, OwnedCleanupTimeout}
	if len(durations) != len(want) {
		t.Fatalf("phase durations=%v", durations)
	}
	for i := range want {
		if durations[i] != want[i] {
			t.Fatalf("phase durations=%v want=%v", durations, want)
		}
	}
}

func TestHundredsConcurrentDisposalsCoalesceImmutableResult(t *testing.T) {
	workload := connectorWorkload(t)
	service := NewDisposalService(nil)
	var cleanups atomic.Int32
	service.observeDrain = func(_ context.Context, _ *workloadReceipt, _ <-chan struct{}, settle func(DrainObservationStatus) DrainObservationStatus) DrainObservationStatus {
		return settle(DrainComplete)
	}
	service.cleanupOwned = func(context.Context, *workloadReceipt) (OwnershipStatus, UninstallStatus, DisappearanceStatus) {
		cleanups.Add(1)
		return OwnershipProven, UninstallSucceeded, DisappearanceSucceeded
	}
	const callers = 400
	start := make(chan struct{})
	results := make([]DisposalResult, callers)
	var wait sync.WaitGroup
	wait.Add(callers)
	for i := range results {
		go func(index int) {
			defer wait.Done()
			<-start
			if index%2 == 0 {
				results[index] = service.DrainAndDispose(context.Background(), workload)
			} else {
				results[index] = service.DisposeNow(context.Background(), workload)
			}
		}(i)
	}
	close(start)
	wait.Wait()
	if cleanups.Load() != 1 {
		t.Fatalf("cleanup count=%d", cleanups.Load())
	}
	for index := 1; index < len(results); index++ {
		if results[index] != results[0] {
			t.Fatalf("result %d mutated: %#v != %#v", index, results[index], results[0])
		}
	}
	late := service.DisposeNow(context.Background(), workload)
	if late != results[0] || cleanups.Load() != 1 {
		t.Fatalf("late=%#v first=%#v cleanups=%d", late, results[0], cleanups.Load())
	}
}

func TestDisposalSafeOutcomeMatrix(t *testing.T) {
	tests := []struct {
		name          string
		observation   DrainObservationStatus
		ownership     OwnershipStatus
		uninstall     UninstallStatus
		disappearance DisappearanceStatus
	}{
		{"natural", DrainComplete, OwnershipProven, UninstallSucceeded, DisappearanceSucceeded},
		{"forced", DrainFailed, OwnershipProven, UninstallSucceeded, DisappearanceSucceeded},
		{"already absent", DrainNotFound, OwnershipAlreadyGone, UninstallNotNeeded, DisappearanceSucceeded},
		{"ownership changed", DrainChanged, OwnershipChanged, UninstallNotNeeded, DisappearanceNotChecked},
		{"uninstall error", DrainReadError, OwnershipProven, UninstallFailed, DisappearanceResourcesRemaining},
		{"uid replacement", DrainTimedOut, OwnershipProven, UninstallSucceeded, DisappearanceUIDReplaced},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			workload := connectorWorkload(t)
			service := NewDisposalService(nil)
			service.phaseContext = func(parent context.Context, _ time.Duration) (context.Context, context.CancelFunc) {
				return context.WithCancel(parent)
			}
			service.observeDrain = func(_ context.Context, _ *workloadReceipt, _ <-chan struct{}, settle func(DrainObservationStatus) DrainObservationStatus) DrainObservationStatus {
				return settle(test.observation)
			}
			service.cleanupOwned = func(context.Context, *workloadReceipt) (OwnershipStatus, UninstallStatus, DisappearanceStatus) {
				return test.ownership, test.uninstall, test.disappearance
			}
			got := service.DrainAndDispose(context.Background(), workload)
			if got.Observation != test.observation || got.Ownership != test.ownership || got.Uninstall != test.uninstall || got.Disappearance != test.disappearance || got.Credential != CredentialDestroyed {
				t.Fatalf("result=%#v", got)
			}
		})
	}
}

func TestQuiescenceTimeoutStillFencesAndDestroysCredential(t *testing.T) {
	workload := connectorWorkload(t)
	_, finish, reason := workload.beginLifecycleOperation(context.Background())
	if reason != "" {
		t.Fatal(reason)
	}
	service := NewDisposalService(nil)
	var phases int
	service.phaseContext = func(parent context.Context, _ time.Duration) (context.Context, context.CancelFunc) {
		phases++
		ctx, cancel := context.WithCancel(parent)
		if phases == 1 {
			cancel()
		}
		return ctx, cancel
	}
	service.cleanupOwned = func(context.Context, *workloadReceipt) (OwnershipStatus, UninstallStatus, DisappearanceStatus) {
		return OwnershipAlreadyGone, UninstallNotNeeded, DisappearanceResourcesRemaining
	}
	result := service.DisposeNow(context.Background(), workload)
	finish()
	if result.Quiescence != QuiescenceTimedOut || result.Credential != CredentialDestroyed || result.Disappearance != DisappearanceResourcesRemaining || !workload.isDisposing() {
		t.Fatalf("result=%#v disposing=%v", result, workload.isDisposing())
	}
}

func TestDisposeNowVersusCompletionHasDeterministicPrecedence(t *testing.T) {
	t.Run("force before completion", func(t *testing.T) {
		workload := connectorWorkload(t)
		service := NewDisposalService(nil)
		entered := make(chan struct{})
		service.observeDrain = func(_ context.Context, _ *workloadReceipt, force <-chan struct{}, settle func(DrainObservationStatus) DrainObservationStatus) DrainObservationStatus {
			close(entered)
			<-force
			return settle(DrainEscalated)
		}
		service.cleanupOwned = func(context.Context, *workloadReceipt) (OwnershipStatus, UninstallStatus, DisappearanceStatus) {
			return OwnershipProven, UninstallSucceeded, DisappearanceSucceeded
		}
		drain := make(chan DisposalResult, 1)
		go func() { drain <- service.DrainAndDispose(context.Background(), workload) }()
		<-entered
		now := service.DisposeNow(context.Background(), workload)
		if got := <-drain; got != now || got.Effective != DisposalImmediateEscalation || got.Observation != DrainEscalated {
			t.Fatalf("drain=%#v now=%#v", got, now)
		}
	})
	t.Run("completion settled before immediate", func(t *testing.T) {
		workload := connectorWorkload(t)
		service := NewDisposalService(nil)
		complete := make(chan struct{})
		cleanupEntered := make(chan struct{})
		cleanupRelease := make(chan struct{})
		service.observeDrain = func(_ context.Context, _ *workloadReceipt, _ <-chan struct{}, settle func(DrainObservationStatus) DrainObservationStatus) DrainObservationStatus {
			<-complete
			return settle(DrainComplete)
		}
		service.cleanupOwned = func(context.Context, *workloadReceipt) (OwnershipStatus, UninstallStatus, DisappearanceStatus) {
			close(cleanupEntered)
			<-cleanupRelease
			return OwnershipProven, UninstallSucceeded, DisappearanceSucceeded
		}
		drain := make(chan DisposalResult, 1)
		go func() { drain <- service.DrainAndDispose(context.Background(), workload) }()
		close(complete)
		<-cleanupEntered
		nowResult := make(chan DisposalResult, 1)
		go func() { nowResult <- service.DisposeNow(context.Background(), workload) }()
		close(cleanupRelease)
		got, now := <-drain, <-nowResult
		if got != now || got.Effective != DisposalDrain || got.Observation != DrainComplete {
			t.Fatalf("drain=%#v now=%#v", got, now)
		}
	})
}
