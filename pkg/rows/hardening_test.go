package rows

import (
	"math"
	"sync"
	"testing"
	"time"
)

// Refs with a huge limit must not wrap offset+limit into a negative slice bound.
func TestRefsHugeLimitDoesNotPanic(t *testing.T) {
	env := newEnv(t)
	sh := env.p.shard("configmaps")
	for _, n := range []string{"a", "b", "c"} {
		sh.upsert(cm("default", n, t0, nil))
	}
	id, _ := openCM(t, env)
	refs, _, err := env.m.Refs(id, 1, math.MaxInt, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 2 {
		t.Fatalf("got %d refs, want 2", len(refs))
	}
}

// A reset batch that waits for opMu must rebuild with the spec an Update installed meanwhile.
func TestResetRebuildUsesCurrentSpec(t *testing.T) {
	env := newEnv(t)
	env.p.shard("configmaps").upsert(cm("default", "a", t0, nil))
	id, _ := openCM(t, env)
	v, _ := env.m.get(id)

	v.opMu.Lock() // stands in for an Update that is mid-flight
	done := make(chan struct{})
	go func() {
		env.p.shard("configmaps").reset()
		close(done)
	}()
	time.Sleep(100 * time.Millisecond) // let the reset path reach the opMu wait

	v.mu.Lock()
	next := v.spec
	v.mu.Unlock()
	next.Search = "newer"
	cfg, err := compileView(next, v.def)
	if err != nil {
		t.Fatal(err)
	}
	v.mu.Lock()
	v.cfg, v.spec = cfg, cfg.spec
	v.mu.Unlock()
	v.opMu.Unlock()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("reset rebuild did not finish")
	}
	v.mu.Lock()
	got := v.spec.Search
	v.mu.Unlock()
	if got != "newer" {
		t.Fatalf("rebuild reverted the spec: search = %q", got)
	}
}

// Concurrent Opens of one id leave exactly one view, one token count and one view's leases.
func TestConcurrentOpenSameIDDoesNotLeak(t *testing.T) {
	single := newEnv(t)
	openCM(t, single)
	perView := single.p.activeLeases()

	for round := 0; round < 20; round++ {
		env := newEnv(t)
		env.p.shard("configmaps").upsert(cm("default", "a", t0, nil))
		var wg sync.WaitGroup
		for i := 0; i < 16; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, _ = env.m.Open(ViewSpec{ViewID: "same", ClientToken: "tok", Type: "configmaps", Namespaces: []string{"*"}})
			}()
		}
		wg.Wait()
		env.m.mu.Lock()
		nViews, tok := len(env.m.views), env.m.byToken["tok"]
		env.m.mu.Unlock()
		if nViews != 1 || tok != 1 {
			t.Fatalf("round %d: views=%d byToken=%d, want 1 and 1", round, nViews, tok)
		}
		if got := env.p.activeLeases(); got != perView {
			t.Fatalf("round %d: active leases %d, want %d", round, got, perView)
		}
	}
}

// The hub delivers demand callbacks after unlocking, so they can arrive out of order. The
// poller must follow the hub's real demand, not the stale value it was handed.
func TestPollerDemandFollowsHubNotCallbackValue(t *testing.T) {
	e := newPollerEnv(t, time.Hour)
	// Real demand is on, but the late callback says off.
	sub, _ := e.hub.Subscribe("ctx", "src", true, func(OverlayUpdate) {})
	eventually(t, "loop to start", func() bool { return e.p.Running("ctx", "src") })
	e.p.Demand("ctx", "src", false)
	if !e.p.Running("ctx", "src") {
		t.Fatal("a stale off callback stopped a wanted poller")
	}
	// Real demand is off, but the late callback says on.
	sub.Cancel()
	eventually(t, "loop to stop", func() bool { return !e.p.Running("ctx", "src") })
	e.p.Demand("ctx", "src", true)
	if e.p.Running("ctx", "src") {
		t.Fatal("a stale on callback started an unwanted poller")
	}
}
