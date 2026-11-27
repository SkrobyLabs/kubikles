package rows

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type pollerEnv struct {
	hub     *OverlayHub
	p       *OverlayPoller
	visible atomic.Bool
	calls   atomic.Int64
	fetch   func() OverlayResult
}

func newPollerEnv(t *testing.T, interval time.Duration) *pollerEnv {
	e := &pollerEnv{hub: NewOverlayHub(nil)}
	e.visible.Store(true)
	e.fetch = func() OverlayResult {
		return OverlayResult{Available: true, Records: map[string]OverlayRecord{"k": {float64(e.calls.Load())}}}
	}
	e.p = NewOverlayPoller(PollerConfig{Hub: e.hub, Interval: func() time.Duration { return interval }, Visible: e.visible.Load})
	e.p.Register("src", func(ctx context.Context, contextName string) OverlayResult {
		e.calls.Add(1)
		return e.fetch()
	})
	t.Cleanup(e.p.Close)
	return e
}

func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// Nobody subscribed: nothing is polled. The first subscriber starts polling at once, the last
// one leaving stops it.
func TestPollerRunsOnlyWhileDemanded(t *testing.T) {
	e := newPollerEnv(t, 5*time.Millisecond)
	time.Sleep(30 * time.Millisecond)
	if n := e.calls.Load(); n != 0 {
		t.Fatalf("polled %d times without demand", n)
	}
	sub, _ := e.hub.Subscribe("ctx", "src", true, func(OverlayUpdate) {})
	eventually(t, "polling to start", func() bool { return e.calls.Load() >= 3 })
	if !e.p.Running("ctx", "src") {
		t.Fatal("loop not running")
	}
	sub.Cancel()
	eventually(t, "the loop to stop", func() bool { return !e.p.Running("ctx", "src") })
	time.Sleep(20 * time.Millisecond) // let an in-flight poll finish
	n := e.calls.Load()
	time.Sleep(40 * time.Millisecond)
	if e.calls.Load() != n {
		t.Fatal("polling continued after the last view left")
	}
}

// An unwanted subscription (an inactive view) does not start the source.
func TestPollerIgnoresUnwantedSubscribers(t *testing.T) {
	e := newPollerEnv(t, 5*time.Millisecond)
	sub, _ := e.hub.Subscribe("ctx", "src", false, func(OverlayUpdate) {})
	time.Sleep(30 * time.Millisecond)
	if e.calls.Load() != 0 {
		t.Fatal("polled for an inactive subscriber")
	}
	sub.SetWanted(true)
	eventually(t, "polling after activation", func() bool { return e.calls.Load() > 0 })
	sub.SetWanted(false)
	eventually(t, "the loop to stop", func() bool { return !e.p.Running("ctx", "src") })
}

// A hidden window is not polled; showing it polls at once.
func TestPollerSkipsWhileHidden(t *testing.T) {
	e := newPollerEnv(t, 5*time.Millisecond)
	e.visible.Store(false)
	_, _ = e.hub.Subscribe("ctx", "src", true, func(OverlayUpdate) {})
	time.Sleep(50 * time.Millisecond)
	if n := e.calls.Load(); n != 0 {
		t.Fatalf("polled %d times while hidden", n)
	}
	e.visible.Store(true)
	e.p.Notify()
	eventually(t, "the catch-up poll", func() bool { return e.calls.Load() >= 1 })
	// Hiding again stops it within one tick.
	e.visible.Store(false)
	e.p.Notify()
	time.Sleep(30 * time.Millisecond)
	n := e.calls.Load()
	time.Sleep(60 * time.Millisecond)
	if e.calls.Load() != n {
		t.Fatalf("polled %d more times while hidden", e.calls.Load()-n)
	}
}

// Availability and records reach subscribers; a Keep result retains the previous records.
func TestPollerPublishesAndKeeps(t *testing.T) {
	e := newPollerEnv(t, time.Hour)
	var mu sync.Mutex
	var last OverlayUpdate
	_, _ = e.hub.Subscribe("ctx", "src", true, func(up OverlayUpdate) { mu.Lock(); last = up; mu.Unlock() })
	eventually(t, "the first snapshot", func() bool { mu.Lock(); defer mu.Unlock(); return last.Snap != nil })
	mu.Lock()
	if last.Snap.Available == nil || !*last.Snap.Available || len(last.Snap.Records) != 1 {
		t.Fatalf("first snapshot %+v", last.Snap)
	}
	mu.Unlock()
	e.fetch = func() OverlayResult { return OverlayResult{Available: false, Keep: true} }
	e.p.Notify() // interval is an hour: wake the loop
	eventually(t, "the failed poll", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return last.Snap.Available != nil && !*last.Snap.Available
	})
	mu.Lock()
	if len(last.Snap.Records) != 1 {
		t.Fatalf("records dropped by a failed poll: %v", last.Snap.Records)
	}
	mu.Unlock()
}
