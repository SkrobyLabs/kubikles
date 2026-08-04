//go:build !accelerator

package main

import (
	"bytes"
	"context"
	"errors"
	"log"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"kubikles/pkg/agent"
	"kubikles/pkg/k8s"
	"kubikles/pkg/server"
)

type idleLifecycleOrder struct {
	mu     sync.Mutex
	events []string
}

func (o *idleLifecycleOrder) add(event string) {
	o.mu.Lock()
	o.events = append(o.events, event)
	o.mu.Unlock()
}
func (o *idleLifecycleOrder) snapshot() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]string(nil), o.events...)
}

type idleLifecycleQuiescer struct{ order *idleLifecycleOrder }

func (s *idleLifecycleQuiescer) Quiesce() { s.order.add("quiesce") }

func bindRootIdleLifecycle(t *testing.T, lifecycle *acceleratorDisposableLifecycle, srv interface{ Quiesce() }, registry *server.AcceleratorSessionRegistry, cleaner acceleratorSessionStateCleaner) {
	t.Helper()
	if err := lifecycle.bind(srv, registry, cleaner); err != nil {
		t.Fatalf("bind: %v", err)
	}
}

func TestAcceleratorDisposableLifecycleCleanupOrder(t *testing.T) {
	order := &idleLifecycleOrder{}
	registry := server.NewAcceleratorSessionRegistry("idle-order", nil)
	lifecycle := &acceleratorDisposableLifecycle{cancel: func() { order.add("request-exit") }}
	bindRootIdleLifecycle(t, lifecycle, &idleLifecycleQuiescer{order: order}, registry, acceleratorSessionStateCleanerFunc(func(context.Context) error {
		order.add("downstream-clear")
		return nil
	}))
	lifecycle.closeSessions = func(ctx context.Context) error {
		order.add("socket-close/wait")
		return registry.Close(ctx)
	}

	if err := agent.ExpireDisposableIdle(context.Background(), lifecycle); err != nil {
		t.Fatal(err)
	}
	want := []string{"quiesce", "socket-close/wait", "downstream-clear", "request-exit"}
	if got := order.snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("cleanup order = %v, want %v", got, want)
	}
	if err := lifecycle.ClearSessionAndWatcherState(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestAcceleratorDisposableLifecycleErrorsStillExit(t *testing.T) {
	registryErr := errors.New("DISTINCTIVE_PRIVATE_REGISTRY_ERROR")
	cleanerErr := errors.New("DISTINCTIVE_PRIVATE_CLEANER_ERROR")
	credential := "Bearer DISTINCTIVE_PRIVATE_CREDENTIAL"
	order := &idleLifecycleOrder{}
	registry := server.NewAcceleratorSessionRegistry("idle-errors", nil)
	var cancels atomic.Int32
	lifecycle := &acceleratorDisposableLifecycle{cancel: func() { cancels.Add(1); order.add("request-exit") }}
	bindRootIdleLifecycle(t, lifecycle, &idleLifecycleQuiescer{order: order}, registry, acceleratorSessionStateCleanerFunc(func(context.Context) error {
		order.add("downstream-clear")
		return cleanerErr
	}))
	lifecycle.closeSessions = func(context.Context) error {
		order.add("socket-close/wait")
		return registryErr
	}

	err := agent.ExpireDisposableIdle(context.Background(), lifecycle)
	if !errors.Is(err, registryErr) || !errors.Is(err, cleanerErr) {
		t.Fatalf("joined cleanup error = %v", err)
	}
	if cancels.Load() != 1 {
		t.Fatalf("exit cancellation count = %d", cancels.Load())
	}
	lifecycle.RequestProcessExit()
	if cancels.Load() != 1 {
		t.Fatalf("exit cancellation repeated = %d", cancels.Load())
	}

	var logs bytes.Buffer
	oldWriter := log.Writer()
	oldFlags := log.Flags()
	log.SetOutput(&logs)
	log.SetFlags(0)
	reportAcceleratorIdleCleanupFailure()
	log.SetOutput(oldWriter)
	log.SetFlags(oldFlags)
	got := logs.String()
	if got != "Accelerator idle cleanup failed\n" {
		t.Fatalf("safe report = %q", got)
	}
	for _, secret := range []string{registryErr.Error(), cleanerErr.Error(), credential, "SessionID", "Generation"} {
		if strings.Contains(got, secret) {
			t.Fatalf("safe report contains private material %q: %q", secret, got)
		}
	}
}

func TestAcceleratorDisposableLifecycleBindValidation(t *testing.T) {
	var cancels atomic.Int32
	lifecycle := &acceleratorDisposableLifecycle{cancel: func() { cancels.Add(1) }}
	if err := lifecycle.ClearSessionAndWatcherState(context.Background()); !errors.Is(err, errAcceleratorIdleLifecycleUnbound) {
		t.Fatalf("cleanup-before-bind error = %v", err)
	}
	lifecycle.RequestProcessExit()
	if cancels.Load() != 1 {
		t.Fatalf("exit helper did not cancel before bind: %d", cancels.Load())
	}

	serverFake := &idleLifecycleQuiescer{order: &idleLifecycleOrder{}}
	registry := server.NewAcceleratorSessionRegistry("bind", nil)
	cleaner := acceleratorSessionStateCleanerFunc(func(context.Context) error { return nil })
	for name, attempt := range map[string]func() error{
		"nil server": func() error {
			return (&acceleratorDisposableLifecycle{cancel: func() {}}).bind(nil, registry, cleaner)
		},
		"nil registry": func() error {
			return (&acceleratorDisposableLifecycle{cancel: func() {}}).bind(serverFake, nil, cleaner)
		},
		"nil cleaner": func() error {
			return (&acceleratorDisposableLifecycle{cancel: func() {}}).bind(serverFake, registry, nil)
		},
		"nil cancel": func() error { return (&acceleratorDisposableLifecycle{}).bind(serverFake, registry, cleaner) },
	} {
		t.Run(name, func(t *testing.T) {
			if err := attempt(); !errors.Is(err, errAcceleratorIdleLifecycleUnbound) {
				t.Fatalf("bind error = %v", err)
			}
		})
	}
	valid := &acceleratorDisposableLifecycle{cancel: func() {}}
	if err := valid.bind(serverFake, registry, cleaner); err != nil {
		t.Fatal(err)
	}
	if err := valid.bind(serverFake, registry, cleaner); !errors.Is(err, errAcceleratorIdleLifecycleRebind) {
		t.Fatalf("rebind error = %v", err)
	}
}

func TestClearAcceleratorSessionStateNilSafe(t *testing.T) {
	if err := (*App)(nil).clearAcceleratorSessionState(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := (&App{}).clearAcceleratorSessionState(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestAcceleratorSecretMembershipReconnectAndShutdownIntegration(t *testing.T) {
	one, two := secretWatchCall("lifecycle-one"), secretWatchCall("lifecycle-two")
	leases := &secretWatchLeaseStore{leases: map[agent.SessionID]server.AcceleratorSessionLease{
		one.SessionID: {Generation: 40, Connected: true},
		two.SessionID: {Generation: 3, Connected: true},
	}}
	active := newBlockingSecretWatch()
	started := make(chan struct{})
	collector := newSecretWatchEventCollector()
	manager := newAcceleratorSecretWatchManager(func(context.Context, string, string, k8s.SecretListOptions) (watch.Interface, error) {
		close(started)
		return active, nil
	}, leases.lookup, collector.emit)
	manager.logger = func(string) {}
	manager.sleep = func(ctx context.Context, _ time.Duration) error { <-ctx.Done(); return ctx.Err() }
	subscription, err := manager.Subscribe(one, "evidence", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Subscribe(two, "evidence", false); err != nil {
		t.Fatal(err)
	}
	waitSecretWatchSignal(t, started, "combined lifecycle watch start")

	resourceEvents := func(events []capturedSecretWatchEvent) []capturedSecretWatchEvent {
		var resources []capturedSecretWatchEvent
		for _, event := range events {
			if event.event.Name == "resource-event" {
				resources = append(resources, event)
			}
		}
		return resources
	}
	emitSecret := func(name, rv string) {
		active.result <- watch.Event{Type: watch.Added, Object: &v1.Secret{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "evidence", ResourceVersion: rv}, Type: v1.SecretTypeOpaque}}
	}
	emitSecret("before-disconnect", "40")
	before := collector.waitFor(t, func(events []capturedSecretWatchEvent) bool { return len(resourceEvents(events)) == 1 })
	first := resourceEvents(before)[0]
	if len(first.targets) != 2 {
		t.Fatalf("initial shared targets = %#v", first.targets)
	}

	manager.SessionDisconnected(server.AcceleratorSessionSnapshot{CallContext: one, Generation: 40})
	manager.mu.Lock()
	preservedGeneration, preserved := manager.sessionSpecs[one.SessionID][subscription.WatcherSpecID]
	manager.mu.Unlock()
	if !preserved || preservedGeneration != 40 {
		t.Fatalf("disconnect/grace lost membership: present=%v generation=%d", preserved, preservedGeneration)
	}
	leasing := server.AcceleratorSessionLease{Generation: 41, Connected: true}
	leases.set(one.SessionID, leasing)
	manager.SessionConnected(server.AcceleratorSessionSnapshot{CallContext: one, Generation: 41, Resumed: true})
	emitSecret("after-reconnect", "41")
	after := collector.waitFor(t, func(events []capturedSecretWatchEvent) bool { return len(resourceEvents(events)) == 2 })
	resources := resourceEvents(after)
	if got := resources[0].event.Data.(AcceleratorSecretResourceEvent).Resource.Metadata.Name; got != "before-disconnect" {
		t.Fatalf("first delivered resource = %q", got)
	}
	if got := resources[1].event.Data.(AcceleratorSecretResourceEvent).Resource.Metadata.Name; got != "after-reconnect" {
		t.Fatalf("future delivered resource = %q", got)
	}
	var oldGeneration, newGeneration server.AcceleratorSocketGeneration
	for _, target := range resources[0].targets {
		if target.SessionID == one.SessionID {
			oldGeneration = target.Generation
		}
	}
	for _, target := range resources[1].targets {
		if target.SessionID == one.SessionID {
			newGeneration = target.Generation
		}
	}
	if oldGeneration != 40 || newGeneration != 41 {
		t.Fatalf("N/N+1 event targets = %d/%d, want 40/41", oldGeneration, newGeneration)
	}

	startRace := make(chan struct{})
	var race sync.WaitGroup
	race.Add(3)
	go func() {
		defer race.Done()
		<-startRace
		_ = manager.Unsubscribe(one, subscription.WatcherSpecID)
	}()
	go func() {
		defer race.Done()
		<-startRace
		manager.SessionRevoked(server.AcceleratorSessionSnapshot{CallContext: one, Generation: 41})
	}()
	go func() {
		defer race.Done()
		<-startRace
		emitSecret("concurrent", "42")
	}()
	close(startRace)
	race.Wait()
	emitSecret("after-selective-removal", "43")
	finalEvents := collector.waitFor(t, func(events []capturedSecretWatchEvent) bool {
		for _, event := range resourceEvents(events) {
			if event.event.Data.(AcceleratorSecretResourceEvent).Resource.Metadata.Name == "after-selective-removal" {
				return true
			}
		}
		return false
	})
	for _, event := range resourceEvents(finalEvents) {
		if event.event.Data.(AcceleratorSecretResourceEvent).Resource.Metadata.Name != "after-selective-removal" {
			continue
		}
		if len(event.targets) != 1 || event.targets[0] != (server.AcceleratorSessionTarget{SessionID: two.SessionID, Generation: 3}) {
			t.Fatalf("post-race selective targets = %#v", event.targets)
		}
	}

	app := &App{runtimeMode: RuntimeModeAccelerator, acceleratorSecretWatches: manager}
	registry := server.NewAcceleratorSessionRegistry("combined-lifecycle", nil)
	lifecycleOrder := &idleLifecycleOrder{}
	exit := make(chan struct{})
	clearWasCompleteAtExit := false
	lifecycle := &acceleratorDisposableLifecycle{cancel: func() {
		manager.mu.Lock()
		done := manager.clearDone
		manager.mu.Unlock()
		if done != nil {
			select {
			case <-done:
				clearWasCompleteAtExit = true
			default:
			}
		}
		close(exit)
	}}
	bindRootIdleLifecycle(t, lifecycle, &idleLifecycleQuiescer{order: lifecycleOrder}, registry, acceleratorSessionStateCleanerFunc(app.clearAcceleratorSessionState))
	expired := make(chan error, 1)
	go func() { expired <- agent.ExpireDisposableIdle(context.Background(), lifecycle) }()
	waitSecretWatchSignal(t, active.stopStarted, "idle Secret ClearAll stream stop")
	select {
	case <-exit:
		t.Fatal("idle process cancellation occurred before Secret ClearAll finished")
	case err := <-expired:
		t.Fatalf("idle cleanup returned while live Secret stream blocked: %v", err)
	default:
	}
	close(active.allowStop)
	if err := <-expired; err != nil {
		t.Fatal(err)
	}
	waitSecretWatchSignal(t, exit, "idle process cancellation")
	if !clearWasCompleteAtExit {
		t.Fatal("Secret ClearAll was not complete before idle process cancellation")
	}

	externalCall := secretWatchCall("external-shutdown")
	externalLeases := &secretWatchLeaseStore{leases: map[agent.SessionID]server.AcceleratorSessionLease{externalCall.SessionID: {Generation: 1, Connected: true}}}
	externalWatch := newBlockingSecretWatch()
	externalStarted := make(chan struct{})
	externalManager := newAcceleratorSecretWatchManager(func(context.Context, string, string, k8s.SecretListOptions) (watch.Interface, error) {
		close(externalStarted)
		return externalWatch, nil
	}, externalLeases.lookup, func([]server.AcceleratorSessionTarget, server.Event) {})
	externalManager.sleep = func(ctx context.Context, _ time.Duration) error { <-ctx.Done(); return ctx.Err() }
	if _, err := externalManager.Subscribe(externalCall, "external", false); err != nil {
		t.Fatal(err)
	}
	waitSecretWatchSignal(t, externalStarted, "external shutdown watch start")
	steps := []string{}
	externalLifecycle := &recordingLifecycle{steps: &steps, counts: make(map[string]int)}
	externalApp := &App{runtimeMode: RuntimeModeAccelerator, lifecycle: externalLifecycle, acceleratorSecretWatches: externalManager}
	canceledContext, cancel := context.WithCancel(context.Background())
	cancel()
	shutdownDone := make(chan struct{})
	go func() {
		externalApp.runShutdownPhases(canceledContext)
		close(shutdownDone)
	}()
	waitSecretWatchSignal(t, externalWatch.stopStarted, "external shutdown Secret stream stop")
	select {
	case <-shutdownDone:
		t.Fatal("external App shutdown returned before live Secret stream stopped")
	default:
	}
	close(externalWatch.allowStop)
	waitSecretWatchSignal(t, shutdownDone, "external App shutdown completion")
	if got, want := steps, []string{"quiesce", "stop-producers", "close"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("external shutdown steps = %v, want %v", got, want)
	}
}
