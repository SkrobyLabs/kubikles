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

	"kubikles/pkg/agent"
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

type idleLifecycleRevoker struct {
	order   *idleLifecycleOrder
	entered chan struct{}
	release <-chan struct{}
	once    sync.Once
}

func (r *idleLifecycleRevoker) RevokeBrowserSession(context.Context, agent.SessionID) {
	r.order.add("browser-revoke")
	if r.entered != nil {
		r.once.Do(func() { close(r.entered) })
	}
	if r.release != nil {
		<-r.release
	}
}

func newRootBrowserSession(t *testing.T, revoker server.BrowserSessionRevoker) *server.BrowserSessionManager {
	t.Helper()
	entropy := bytes.NewReader(bytes.Repeat([]byte{0x62}, 96))
	manager := server.NewBrowserSessionManagerWithDependencies(func() time.Time {
		return time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	}, entropy, revoker)
	ticket, _, err := manager.Mint()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := manager.Exchange(ticket); err != nil {
		t.Fatal(err)
	}
	return manager
}

func bindRootIdleLifecycle(t *testing.T, lifecycle *acceleratorDisposableLifecycle, srv interface{ Quiesce() }, browser *server.BrowserSessionManager, registry *server.AcceleratorSessionRegistry, cleaner acceleratorSessionStateCleaner) {
	t.Helper()
	if err := lifecycle.bind(srv, browser, registry, cleaner); err != nil {
		t.Fatalf("bind: %v", err)
	}
}

func TestAcceleratorDisposableLifecycleCleanupOrder(t *testing.T) {
	order := &idleLifecycleOrder{}
	entered := make(chan struct{})
	release := make(chan struct{})
	browser := newRootBrowserSession(t, &idleLifecycleRevoker{order: order, entered: entered, release: release})
	registry := server.NewAcceleratorSessionRegistry("idle-order", nil)
	lifecycle := &acceleratorDisposableLifecycle{cancel: func() { order.add("request-exit") }}
	bindRootIdleLifecycle(t, lifecycle, &idleLifecycleQuiescer{order: order}, browser, registry, acceleratorSessionStateCleanerFunc(func(context.Context) error {
		order.add("downstream-clear")
		return nil
	}))
	lifecycle.closeSessions = func(ctx context.Context) error {
		order.add("socket-close/wait")
		return registry.Close(ctx)
	}

	done := make(chan error, 1)
	go func() { done <- agent.ExpireDisposableIdle(context.Background(), lifecycle) }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("browser revocation was not reached")
	}
	if got := order.snapshot(); !reflect.DeepEqual(got, []string{"quiesce", "browser-revoke"}) {
		t.Fatalf("order while browser revoke blocked = %v", got)
	}
	close(release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cleanup did not complete")
	}
	want := []string{"quiesce", "browser-revoke", "socket-close/wait", "downstream-clear", "request-exit"}
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
	browser := server.NewBrowserSessionManager(registry)
	var cancels atomic.Int32
	lifecycle := &acceleratorDisposableLifecycle{cancel: func() { cancels.Add(1); order.add("request-exit") }}
	bindRootIdleLifecycle(t, lifecycle, &idleLifecycleQuiescer{order: order}, browser, registry, acceleratorSessionStateCleanerFunc(func(context.Context) error {
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
	browser := server.NewBrowserSessionManager(nil)
	registry := server.NewAcceleratorSessionRegistry("bind", nil)
	cleaner := acceleratorSessionStateCleanerFunc(func(context.Context) error { return nil })
	for name, attempt := range map[string]func() error{
		"nil server": func() error {
			return (&acceleratorDisposableLifecycle{cancel: func() {}}).bind(nil, browser, registry, cleaner)
		},
		"nil browser": func() error {
			return (&acceleratorDisposableLifecycle{cancel: func() {}}).bind(serverFake, nil, registry, cleaner)
		},
		"nil registry": func() error {
			return (&acceleratorDisposableLifecycle{cancel: func() {}}).bind(serverFake, browser, nil, cleaner)
		},
		"nil cleaner": func() error {
			return (&acceleratorDisposableLifecycle{cancel: func() {}}).bind(serverFake, browser, registry, nil)
		},
		"nil cancel": func() error { return (&acceleratorDisposableLifecycle{}).bind(serverFake, browser, registry, cleaner) },
	} {
		t.Run(name, func(t *testing.T) {
			if err := attempt(); !errors.Is(err, errAcceleratorIdleLifecycleUnbound) {
				t.Fatalf("bind error = %v", err)
			}
		})
	}
	valid := &acceleratorDisposableLifecycle{cancel: func() {}}
	if err := valid.bind(serverFake, browser, registry, cleaner); err != nil {
		t.Fatal(err)
	}
	if err := valid.bind(serverFake, browser, registry, cleaner); !errors.Is(err, errAcceleratorIdleLifecycleRebind) {
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
