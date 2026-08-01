package main

import (
	"context"
	"sync"
	"testing"
	"time"

	"kubikles/pkg/events"
)

type recordingLifecycle struct {
	mu     sync.Mutex
	steps  *[]string
	counts map[string]int
}

func (l *recordingLifecycle) record(step string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	*l.steps = append(*l.steps, step)
	l.counts[step]++
}

func (l *recordingLifecycle) Quiesce(context.Context)       { l.record("quiesce") }
func (l *recordingLifecycle) StopProducers(context.Context) { l.record("stop-producers") }
func (l *recordingLifecycle) Close(context.Context)         { l.record("close") }

func TestRunShutdownPhasesOrder(t *testing.T) {
	steps := []string{}
	lifecycle := &recordingLifecycle{steps: &steps, counts: make(map[string]int)}
	app := &App{lifecycle: lifecycle}
	app.emitter = events.EmitterFunc(func(string, ...interface{}) { steps = append(steps, "flush") })
	app.eventCoalescer = NewEventCoalescer(app, time.Hour)
	app.eventCoalescer.Emit(makeTestEvent("pods", "default", "test", ""))

	app.runShutdownPhases(context.Background())
	if got, want := len(app.eventCoalescer.events), 0; got != want {
		t.Fatalf("pending events = %d, want %d", got, want)
	}
	want := []string{"quiesce", "stop-producers", "flush", "close"}
	if len(steps) != len(want) {
		t.Fatalf("steps = %v, want %v", steps, want)
	}
	for i := range want {
		if steps[i] != want[i] {
			t.Fatalf("steps = %v, want %v", steps, want)
		}
	}
}

func TestRunShutdownPhasesIsIdempotent(t *testing.T) {
	steps := []string{}
	lifecycle := &recordingLifecycle{steps: &steps, counts: make(map[string]int)}
	app := &App{lifecycle: lifecycle}
	app.runShutdownPhases(context.Background())
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() { defer wg.Done(); app.runShutdownPhases(context.Background()) }()
	}
	wg.Wait()
	for _, step := range []string{"quiesce", "stop-producers", "close"} {
		if lifecycle.counts[step] != 1 {
			t.Fatalf("%s calls = %d, want 1", step, lifecycle.counts[step])
		}
	}
}

func TestRunShutdownPhasesAllowsUnstartedApp(t *testing.T) {
	(&App{}).runShutdownPhases(context.Background())
}

func TestStopProducersPreservesCleanupSet(t *testing.T) {
	app := &App{runtimeMode: RuntimeModeDesktop, lifecycle: NoopRuntimeLifecycle{}, eventStats: make(map[string]*WatcherEventStats)}
	watcherStopped := make(chan struct{})
	app.watcherManager = NewResourceWatcherManager(context.Background(), app)
	app.watcherManager.watchers["pods:default"] = &ResourceWatcher{Cancel: func() { close(watcherStopped) }}

	app.stopProducers(context.Background())
	select {
	case <-watcherStopped:
	default:
		t.Fatal("watcher cleanup was not called")
	}
	if len(app.watcherManager.watchers) != 0 {
		t.Fatal("watcher cleanup did not clear watcher map")
	}
}

func TestStopProducersCleansEmbeddedBrowserInServerMode(t *testing.T) {
	app := &App{runtimeMode: RuntimeModeServer}
	app.embeddedBrowser.session = &EmbeddedBrowserSession{Namespace: "default", Status: "running"}

	app.stopProducers(context.Background())

	if app.embeddedBrowser.session != nil {
		t.Fatal("embedded browser session was not cleaned up")
	}
}

func TestMinimalAcceleratorShutdownIsSafeAndOnce(t *testing.T) {
	steps := []string{}
	lifecycle := &recordingLifecycle{steps: &steps, counts: make(map[string]int)}
	app := newMinimalAcceleratorTestApp(t, lifecycle)
	app.embeddedBrowser.session = &EmbeddedBrowserSession{Namespace: "default", Status: "running"}
	app.SetEmitter(&events.NoopEmitter{})
	app.startupServerMode(context.Background())

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			app.runShutdownPhases(context.Background())
		}()
	}
	wg.Wait()

	if got, want := steps, []string{"quiesce", "stop-producers", "close"}; len(got) != len(want) {
		t.Fatalf("shutdown steps = %v, want %v", got, want)
	} else {
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("shutdown steps = %v, want %v", got, want)
			}
		}
	}
	for _, step := range []string{"quiesce", "stop-producers", "close"} {
		if lifecycle.counts[step] != 1 {
			t.Fatalf("%s calls = %d, want 1", step, lifecycle.counts[step])
		}
	}
	if app.embeddedBrowser.session == nil {
		t.Fatal("Accelerator shutdown executed desktop embedded-browser cleanup")
	}
	if len(app.getDisconnectListeners()) != 0 {
		t.Fatalf("disconnect listeners = %d, want zero", len(app.getDisconnectListeners()))
	}
	assertAcceleratorOrdinaryServicesAbsentExceptBrowserFixture(t, app)
}

func assertAcceleratorOrdinaryServicesAbsentExceptBrowserFixture(t *testing.T, app *App) {
	t.Helper()
	session := app.embeddedBrowser.session
	app.embeddedBrowser.session = nil
	assertAcceleratorOrdinaryServicesAbsent(t, app)
	app.embeddedBrowser.session = session
}
