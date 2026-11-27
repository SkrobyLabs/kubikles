package rows

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// A loop whose source was stopped while its fetch ran must not publish its (older) result.
func TestPollerStoppedLoopDoesNotPublish(t *testing.T) {
	e := newPollerEnv(t, time.Hour)
	started := make(chan struct{})
	release := make(chan struct{})
	e.p.Register("slow", func(ctx context.Context, contextName string) OverlayResult {
		close(started)
		<-release
		return OverlayResult{Available: true, Records: map[string]OverlayRecord{"k": {1.0}}}
	})
	sub, _ := e.hub.Subscribe("ctx", "slow", true, func(OverlayUpdate) {})
	<-started
	sub.Cancel()
	eventually(t, "the loop to stop", func() bool { return !e.p.Running("ctx", "slow") })
	close(release)
	time.Sleep(50 * time.Millisecond)
	if cur := e.hub.Current("ctx", "slow"); cur != nil {
		t.Fatalf("a stopped loop published %+v", cur)
	}
}

// A result fetched before a Reset must not repopulate the hub.
func TestPollerDropsResultFetchedBeforeReset(t *testing.T) {
	e := newPollerEnv(t, time.Hour)
	started := make(chan struct{})
	release := make(chan struct{})
	e.p.Register("slow", func(ctx context.Context, contextName string) OverlayResult {
		close(started)
		<-release
		return OverlayResult{Available: true, Records: map[string]OverlayRecord{"k": {1.0}}}
	})
	e.hub.Subscribe("ctx", "slow", true, func(OverlayUpdate) {})
	<-started
	e.hub.Reset()
	close(release)
	time.Sleep(50 * time.Millisecond)
	if cur := e.hub.Current("ctx", "slow"); cur != nil {
		t.Fatalf("a poll that began before Reset published %+v", cur)
	}
	gen := e.hub.Generation()
	e.hub.Reset()
	if e.hub.PublishAt(gen, "ctx", "x", BoolPtr(true), nil) {
		t.Fatal("PublishAt accepted a stale generation")
	}
}

// Concurrent publishes of one source reach a subscriber in Seq order.
func TestHubPublishDeliversInOrder(t *testing.T) {
	h := NewOverlayHub(nil)
	var mu sync.Mutex
	var seqs []uint64
	h.Subscribe("c", "s", false, func(up OverlayUpdate) {
		time.Sleep(time.Millisecond)
		mu.Lock()
		seqs = append(seqs, up.Snap.Seq)
		mu.Unlock()
	})
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			h.Publish("c", "s", BoolPtr(true), map[string]OverlayRecord{"k": {float64(i)}})
		}(i)
	}
	wg.Wait()
	for i := 1; i < len(seqs); i++ {
		if seqs[i] <= seqs[i-1] {
			t.Fatalf("delivered out of order: %v", seqs)
		}
	}
}

// Overlays attached to a view that was closed meanwhile are cancelled, not leaked.
func TestAttachOverlaysAfterCloseCancels(t *testing.T) {
	oe := newOverlayEnv(t)
	id := oe.openPods(t)
	v, err := oe.m.get(id)
	if err != nil {
		t.Fatal(err)
	}
	oe.m.CloseView(id)
	oe.mu.Lock()
	oe.demand = nil
	oe.mu.Unlock()
	v.attachOverlays()
	v.mu.Lock()
	leaked := len(v.ovSubs)
	v.mu.Unlock()
	if leaked != 0 {
		t.Fatalf("closed view holds %d overlay subscriptions", leaked)
	}
	if oe.hub.Demanded("ctx", OverlayPodMetrics) {
		t.Fatal("overlay demand stays up after the view closed")
	}
}

// Concurrent patches of different fields all apply.
func TestConcurrentUpdatesKeepEachOther(t *testing.T) {
	for i := 0; i < 100; i++ {
		env := newEnv(t)
		id, _ := openCM(t, env)
		search, ns := "x", []string{"a"}
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); _, _ = env.m.Update(id, ViewPatch{Search: &search}) }()
		go func() { defer wg.Done(); _, _ = env.m.Update(id, ViewPatch{Namespaces: &ns}) }()
		wg.Wait()
		v, _ := env.m.get(id)
		v.mu.Lock()
		got := v.spec
		v.mu.Unlock()
		if got.Search != "x" || fmt.Sprint(got.Namespaces) != "[a]" {
			t.Fatalf("iteration %d lost a patch: search=%q namespaces=%v", i, got.Search, got.Namespaces)
		}
	}
}

// The pod feed is held only while a column, sort, filter or facet reads it.
func TestWorkloadViewLeasesPodsOnlyWhenNeeded(t *testing.T) {
	env := newEnv(t)
	env.p.shard("deployments").upsert(depEntry("a", "web", map[string]string{"app": "web"}))
	env.p.shard("pods").upsert(labelledPod("a", "web-1", map[string]string{"app": "web"}, true))

	_, err := env.m.Open(ViewSpec{ViewID: "dv", ClientToken: "t", Type: "deployments", Namespaces: []string{"*"},
		Columns: []string{"name"}, Sort: SortSpec{Key: "name", Dir: "asc"}})
	if err != nil {
		t.Fatal(err)
	}
	base := env.p.activeLeases()
	v, _ := env.m.get("dv")
	if v.pods != nil {
		t.Fatal("pod feed connected although no column reads it")
	}

	cols := []string{"name", "pods"}
	st, err := env.m.Update("dv", ViewPatch{Columns: &cols})
	if err != nil {
		t.Fatal(err)
	}
	if v.pods == nil || env.p.activeLeases() <= base {
		t.Fatal("pod feed not connected after the pods column was added")
	}
	if av := st.Overlays["pods"].Available; av == nil || !*av {
		t.Fatalf("pods availability = %v", av)
	}
	w, _ := env.m.Window("dv", 0, 10, 0)
	if got := windowCell(t, w, cols, "d-a-web", "pods"); got != float64(1) {
		t.Fatalf("pods cell = %v, want 1", got)
	}

	cols = []string{"name"}
	if _, err := env.m.Update("dv", ViewPatch{Columns: &cols}); err != nil {
		t.Fatal(err)
	}
	if v.pods != nil || env.p.activeLeases() != base {
		t.Fatalf("pod feed not released after the pods column was removed (leases %d, want %d)", env.p.activeLeases(), base)
	}
}
