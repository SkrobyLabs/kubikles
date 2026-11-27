package store

import (
	"reflect"
	goruntime "runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	clienttesting "k8s.io/client-go/testing"
)

func TestPagedListStreamsProgressAndSyncs(t *testing.T) {
	f := newFixture(t)
	rem := int64(3)
	f.api.remaining = &rem
	f.api.pages = [][]v1.Pod{
		{pod("a", "p1", "1"), pod("a", "p2", "2")},
		{pod("a", "p3", "3")},
	}
	f.api.listRV = "500"

	l := f.acquire(t, podsKey, Hint{})
	defer l.Release()
	waitSynced(t, l)

	st := l.State()
	if st.Phase != PhaseWatching || st.Entries != 3 || st.Loaded != 3 || st.Expected != 3 || st.RV != "500" {
		t.Fatalf("state = %+v", st)
	}
	snap, _ := l.Snapshot()
	for _, e := range snap {
		if e.Obj.(*v1.Pod).ManagedFields != nil {
			t.Fatalf("managed fields not stripped on %s", e.Name)
		}
	}
	got := f.sinks.statuses()
	if len(got) == 0 || got[0] != "connected" {
		t.Fatalf("statuses = %v", got)
	}
}

func TestPagesBecomeVisibleBeforeListCompletes(t *testing.T) {
	f := newFixture(t)
	release := make(chan struct{})
	f.api.pages = [][]v1.Pod{{pod("a", "p1", "1")}, {pod("a", "p2", "2")}}
	// Hold the second page until the test has seen the first one.
	f.cs.PrependReactor("list", "pods", func(clienttesting.Action) (bool, runtime.Object, error) {
		f.api.mu.Lock()
		idx := f.api.pageIdx
		f.api.mu.Unlock()
		if idx == 1 {
			<-release
		}
		return false, nil, nil
	})
	l := f.acquire(t, podsKey, Hint{})
	defer l.Release()
	eventually(t, "first page visible", func() bool { return l.State().Entries == 1 })
	if st := l.State(); st.Phase != PhaseLoading || st.Synced {
		t.Fatalf("mid-load state = %+v", st)
	}
	close(release)
	waitSynced(t, l)
	if l.State().Entries != 2 {
		t.Fatalf("entries = %d", l.State().Entries)
	}
}

func TestWatchEventsAreOrderedAndBookmarksAdvanceRV(t *testing.T) {
	f := newFixture(t)
	f.api.setItems("10", pod("a", "p1", "1"))
	w := f.api.newWatcher()

	l := f.acquire(t, podsKey, Hint{})
	defer l.Release()
	waitSynced(t, l)

	c := &collector{}
	sub := l.Subscribe(c.fn, SubOptions{})
	defer sub.Cancel()
	if len(sub.Snapshot) != 1 {
		t.Fatalf("snapshot = %d", len(sub.Snapshot))
	}

	w.Add(podPtr("a", "p2", "11"))
	w.Modify(podPtr("a", "p1", "12"))
	w.Delete(podPtr("a", "p2", "13"))
	w.Action(watch.Bookmark, &metav1.PartialObjectMetadata{ObjectMeta: metav1.ObjectMeta{ResourceVersion: "20"}})
	eventually(t, "changes", func() bool { return len(c.all()) == 3 })

	want := []string{"Added:p2", "Modified:p1", "Deleted:p2"}
	if got := c.summary(); !reflect.DeepEqual(got, want) {
		t.Fatalf("changes = %v, want %v", got, want)
	}
	var last uint64
	for _, ch := range c.all() {
		if ch.Seq <= last {
			t.Fatalf("seq not increasing: %+v", c.all())
		}
		last = ch.Seq
	}
	// The bookmark moved the version past the last object event.
	eventually(t, "bookmark rv", func() bool { return l.State().RV == "20" })

	// A reconnect resumes from the bookmark version.
	w.Stop()
	eventually(t, "reconnect wait", func() bool { return len(f.clock.Waits()) >= 1 })
	f.clock.Advance(10 * time.Second)
	eventually(t, "reconnect", func() bool { return len(f.api.rvs()) >= 2 })
	if rvs := f.api.rvs(); rvs[0] != "10" || rvs[1] != "20" {
		t.Fatalf("watch resource versions = %v, want [10 20]", rvs)
	}
}

func TestIdenticalResourceVersionIsNotAChange(t *testing.T) {
	f := newFixture(t)
	f.api.setItems("10", pod("a", "p1", "1"))
	w := f.api.newWatcher()
	l := f.acquire(t, podsKey, Hint{})
	defer l.Release()
	waitSynced(t, l)
	c := &collector{}
	sub := l.Subscribe(c.fn, SubOptions{})
	defer sub.Cancel()
	w.Modify(podPtr("a", "p1", "1"))
	w.Add(podPtr("a", "p9", "9"))
	eventually(t, "second event", func() bool { return len(c.all()) == 1 })
	if got := c.summary(); !reflect.DeepEqual(got, []string{"Added:p9"}) {
		t.Fatalf("changes = %v", got)
	}
}

func TestGoneTriggersRelistThatDiffs(t *testing.T) {
	f := newFixture(t)
	f.api.setItems("10", pod("a", "keep", "1"), pod("a", "change", "2"), pod("a", "gone", "3"))
	w := f.api.newWatcher()
	l := f.acquire(t, podsKey, Hint{})
	defer l.Release()
	waitSynced(t, l)

	c := &collector{}
	sub := l.Subscribe(c.fn, SubOptions{})
	defer sub.Cancel()

	f.api.setItems("90", pod("a", "keep", "1"), pod("a", "change", "7"), pod("a", "fresh", "8"))
	f.api.newWatcher()
	w.Error(&metav1.Status{Status: metav1.StatusFailure, Reason: metav1.StatusReasonGone, Code: 410, Message: "too old"})
	f.clock.Advance(time.Minute)

	eventually(t, "relist changes", func() bool { return len(c.all()) == 3 })
	got := c.summary()
	sort.Strings(got)
	want := []string{"Added:fresh", "Deleted:gone", "Modified:change"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("changes = %v, want %v (unchanged entries must emit nothing)", got, want)
	}
	if st := l.State(); st.Entries != 3 || st.RV != "90" {
		t.Fatalf("state = %+v", st)
	}
}

// A rejected watch version is not an error, but a server that keeps rejecting it must not turn
// the shard into a continuous LIST loop: the first relist is immediate, later ones back off
// until a watch delivers events.
func TestWatchOpenRejectingStaleVersionFirstRelistImmediateThenBacksOff(t *testing.T) {
	f := newFixture(t)
	f.api.setItems("10", pod("a", "p1", "1"))
	f.api.mu.Lock()
	f.api.watchErr = apierrors.NewGone("too old resource version")
	f.api.mu.Unlock()
	l := f.acquire(t, podsKey, Hint{})
	defer l.Release()

	// list, rejected watch, immediate relist, rejected watch, then the first wait.
	eventually(t, "first spaced relist", func() bool { return len(f.clock.Waits()) >= 1 })
	if w := f.clock.Waits()[0]; w != 2*time.Second {
		t.Fatalf("first wait = %v, want 2s", w)
	}
	if n := f.api.calls(); n != 2 {
		t.Fatalf("list calls = %d, want the initial list plus one immediate relist before the wait", n)
	}
	before := f.api.calls()
	time.Sleep(30 * time.Millisecond)
	if f.api.calls() != before {
		t.Fatal("shard relisted while waiting")
	}
	f.clock.Advance(2 * time.Second)
	eventually(t, "third list", func() bool { return f.api.calls() >= 3 })
	eventually(t, "second wait", func() bool { return len(f.clock.Waits()) >= 2 })
	if w := f.clock.Waits()[1]; w != 4*time.Second {
		t.Fatalf("second wait = %v, want 4s", w)
	}
	if errs := f.sinks.errors(); len(errs) != 0 {
		t.Fatalf("stale version surfaced as error: %v", errs)
	}
}

func TestForbiddenSetsIncompleteWithoutHotLoop(t *testing.T) {
	f := newFixture(t)
	f.api.mu.Lock()
	f.api.listErr = apierrors.NewForbidden(schema.GroupResource{Resource: "pods"}, "", nil)
	f.api.mu.Unlock()
	l := f.acquire(t, podsKey, Hint{})
	defer l.Release()

	eventually(t, "forbidden state", func() bool { return l.State().Forbidden })
	st := l.State()
	if st.Phase != PhaseIncomplete || st.Synced || st.Error == "" {
		t.Fatalf("state = %+v", st)
	}
	time.Sleep(50 * time.Millisecond)
	if n := f.api.calls(); n != 1 {
		t.Fatalf("list calls = %d, want 1 (no hot loop)", n)
	}
	if errs := f.sinks.errors(); len(errs) != 1 {
		t.Fatalf("errors = %v", errs)
	}
	// Still no retry until the wait elapses.
	f.clock.Advance(time.Minute)
	time.Sleep(20 * time.Millisecond)
	if n := f.api.calls(); n != 1 {
		t.Fatalf("list calls after 1m = %d", n)
	}
	// Access granted: the retry after the wait succeeds.
	f.api.mu.Lock()
	f.api.listErr = nil
	f.api.mu.Unlock()
	f.api.setItems("5", pod("a", "p1", "1"))
	f.clock.Advance(3 * time.Minute)
	waitSynced(t, l)
	if l.State().Forbidden {
		t.Fatal("forbidden flag not cleared")
	}
}

func TestBackoffDoublesAndCaps(t *testing.T) {
	f := newFixture(t)
	f.api.mu.Lock()
	f.api.listErr = apierrors.NewServiceUnavailable("down")
	f.api.mu.Unlock()
	l := f.acquire(t, podsKey, Hint{})
	defer l.Release()

	var want []time.Duration
	for i := 1; i <= 10; i++ {
		d := time.Second << uint(min(i, 7))
		if d > 2*time.Minute {
			d = 2 * time.Minute
		}
		want = append(want, d)
	}
	for i := range want {
		i := i
		eventually(t, "wait registered", func() bool { return len(f.clock.Waits()) > i })
		if st := l.State(); st.Phase != PhaseBackoff {
			t.Fatalf("phase while waiting = %s", st.Phase)
		}
		f.clock.Advance(want[i])
	}
	got := f.clock.Waits()[:len(want)]
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("backoff = %v, want %v", got, want)
	}
	if want[len(want)-1] != 2*time.Minute {
		t.Fatal("test expects the cap to be reached")
	}
}

func TestPrematureCloseCountsAsFailure(t *testing.T) {
	f := newFixture(t)
	f.api.setItems("10", pod("a", "p1", "1"))
	w := f.api.newWatcher()
	l := f.acquire(t, podsKey, Hint{})
	defer l.Release()
	waitSynced(t, l)
	w.Stop() // closes without delivering anything, immediately
	eventually(t, "premature error", func() bool { return len(f.sinks.errors()) == 1 })
	if e := f.sinks.errors()[0]; !strings.Contains(e, "closed unexpectedly") {
		t.Fatalf("error = %q", e)
	}
	eventually(t, "backoff wait", func() bool { return len(f.clock.Waits()) >= 1 })
	if d := f.clock.Waits()[0]; d != 2*time.Second {
		t.Fatalf("first premature delay = %v, want 2s", d)
	}
}

func TestLingerKeepsShardAcrossReacquire(t *testing.T) {
	f := newFixture(t)
	f.api.setItems("10", pod("a", "p1", "1"))
	l1 := f.acquire(t, podsKey, Hint{})
	waitSynced(t, l1)
	l1.Release()

	f.clock.Advance(4 * time.Second)
	l2 := f.acquire(t, podsKey, Hint{})
	if l2.sh != l1.sh {
		t.Fatal("re-acquire within linger created a new shard")
	}
	if n := f.api.calls(); n != 1 {
		t.Fatalf("list calls = %d, want 1", n)
	}
	// The old expiry timer must not stop a shard that is leased again.
	f.clock.Advance(10 * time.Second)
	select {
	case <-l2.Done():
		t.Fatal("leased shard was stopped by a stale linger timer")
	default:
	}
	l2.Release()
	f.clock.Advance(6 * time.Second)
	select {
	case <-l2.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("idle shard did not stop after linger")
	}
	if n := len(f.m.Stats().Shards); n != 0 {
		t.Fatalf("shards = %d", n)
	}
}

func TestLeaseLingerOverride(t *testing.T) {
	f := newFixture(t)
	l := f.acquire(t, podsKey, Hint{Linger: ViewLinger})
	waitSynced(t, l)
	l.Release()
	f.clock.Advance(20 * time.Second)
	select {
	case <-l.Done():
		t.Fatal("view linger should still be running at 20s")
	default:
	}
	f.clock.Advance(15 * time.Second)
	select {
	case <-l.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("shard did not stop after the view linger")
	}
}

func storeGoroutines() int {
	buf := make([]byte, 1<<20)
	n := goruntime.Stack(buf, true)
	count := 0
	for _, g := range strings.Split(string(buf[:n]), "\n\n") {
		if strings.Contains(g, "kubikles/pkg/store.") && !strings.Contains(g, "testing.") {
			count++
		}
	}
	return count
}

func TestResetStopsAllGoroutines(t *testing.T) {
	f := newFixture(t)
	f.api.setItems("10", pod("a", "p1", "1"))
	var leases []*Lease
	for _, ns := range []string{"", "a", "b"} {
		l := f.acquire(t, Key{Context: "ctx", Type: "pods", Namespace: ns}, Hint{})
		waitSynced(t, l)
		leases = append(leases, l)
		l.Subscribe(func([]Change) {}, SubOptions{})
	}
	if storeGoroutines() == 0 {
		t.Fatal("expected reflector goroutines before reset")
	}
	f.m.Reset()
	for _, l := range leases {
		select {
		case <-l.Done():
		default:
			t.Fatal("lease not marked done by Reset")
		}
		l.Release() // inert, must not panic
	}
	eventually(t, "goroutines gone", func() bool { return storeGoroutines() == 0 })
	if n := len(f.m.Stats().Shards); n != 0 {
		t.Fatalf("shards after reset = %d", n)
	}
}

func TestSubscriberOverflowCollapsesToResetAndResyncs(t *testing.T) {
	f := newFixture(t)
	f.api.setItems("10")
	w := f.api.newWatcher()
	l := f.acquire(t, podsKey, Hint{})
	defer l.Release()
	waitSynced(t, l)

	block := make(chan struct{})
	var mu sync.Mutex
	var batches [][]Change
	sub := l.Subscribe(func(b []Change) {
		<-block
		mu.Lock()
		batches = append(batches, b)
		mu.Unlock()
	}, SubOptions{QueueSize: 4})
	defer sub.Cancel()

	for i := 0; i < 50; i++ {
		w.Add(podPtr("a", "p"+string(rune('A'+i%26))+string(rune('a'+i/26)), "1"))
	}
	eventually(t, "all applied", func() bool { return l.State().Entries == 50 })
	close(block)
	eventually(t, "reset delivered", func() bool {
		mu.Lock()
		defer mu.Unlock()
		for _, b := range batches {
			for _, c := range b {
				if c.Type == ChangeReset {
					return true
				}
			}
		}
		return false
	})
	snap, seq := l.Snapshot()
	if len(snap) != 50 || seq == 0 {
		t.Fatalf("resync snapshot = %d entries, seq %d", len(snap), seq)
	}
	// After the reset is consumed, new changes flow again.
	w.Add(podPtr("a", "late", "2"))
	eventually(t, "post-reset change", func() bool {
		mu.Lock()
		defer mu.Unlock()
		for _, b := range batches {
			for _, c := range b {
				if c.New != nil && c.New.Name == "late" {
					return true
				}
			}
		}
		return false
	})
}

func TestConcurrentAcquireRelease(t *testing.T) {
	f := newFixture(t)
	f.api.setItems("10", pod("a", "p1", "1"), pod("b", "p2", "2"))
	var wg sync.WaitGroup
	for g := 0; g < 16; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 30; i++ {
				key := podsKey
				if (g+i)%3 == 0 {
					key.Namespace = "a"
				}
				l, err := f.m.Acquire(key, Hint{})
				if err != nil {
					t.Error(err)
					return
				}
				_, _ = l.Snapshot()
				_ = l.State()
				sub := l.Subscribe(func([]Change) {}, SubOptions{})
				sub.Cancel()
				l.Release()
				if i%7 == 0 {
					f.clock.Advance(time.Second)
				}
			}
		}(g)
	}
	wg.Wait()
	f.clock.Advance(time.Minute)
	eventually(t, "all shards expire", func() bool { return len(f.m.Stats().Shards) == 0 })
}

func TestClusterWideShardServesNamespacedAcquireButNotReverse(t *testing.T) {
	f := newFixture(t)
	f.api.setItems("10", pod("a", "p1", "1"), pod("b", "p2", "2"), pod("a", "p3", "3"))

	// A namespaced acquire first must not start or reuse anything cluster wide.
	ns := f.acquire(t, Key{Context: "ctx", Type: "pods", Namespace: "a"}, Hint{})
	waitSynced(t, ns)
	if ns.ServedByClusterWide() {
		t.Fatal("namespaced shard was reported as cluster-wide")
	}
	cw := f.acquire(t, podsKey, Hint{})
	waitSynced(t, cw)
	if cw.sh == ns.sh {
		t.Fatal("cluster-wide acquire reused a namespaced shard")
	}
	ns.Release()

	// Now the cluster-wide shard is warm: a namespaced acquire is served from it.
	served := f.acquire(t, Key{Context: "ctx", Type: "pods", Namespace: "b"}, Hint{})
	defer served.Release()
	if served.sh != cw.sh || !served.ServedByClusterWide() {
		t.Fatal("namespaced acquire was not served by the warm cluster-wide shard")
	}
	snap, _ := served.Snapshot()
	if len(snap) != 1 || snap[0].Name != "p2" {
		t.Fatalf("filtered snapshot = %v", snap)
	}
	c := &collector{}
	sub := served.Subscribe(c.fn, SubOptions{})
	defer sub.Cancel()
	if len(sub.Snapshot) != 1 {
		t.Fatalf("subscription snapshot = %d", len(sub.Snapshot))
	}
}

func TestPeekRules(t *testing.T) {
	f := newFixture(t)
	if _, _, ok := Peek[v1.Pod](f.m, podsKey); ok {
		t.Fatal("peek of an absent shard must report not ok")
	}
	f.api.setItems("42", pod("a", "p1", "1"), pod("b", "p2", "2"))
	f.api.newWatcher()
	l := f.acquire(t, podsKey, Hint{})
	defer l.Release()
	waitSynced(t, l)

	calls := f.api.calls()
	items, rv, ok := Peek[v1.Pod](f.m, podsKey)
	if !ok || len(items) != 2 || rv != "42" {
		t.Fatalf("peek = %d items rv %q ok %v", len(items), rv, ok)
	}
	if n, _, ok := Peek[v1.Pod](f.m, Key{Context: "ctx", Type: "pods", Namespace: "a"}); !ok || len(n) != 1 {
		t.Fatalf("namespaced peek from cluster-wide shard = %d ok %v", len(n), ok)
	}
	if _, _, ok := Peek[v1.Node](f.m, podsKey); ok {
		t.Fatal("peek of the wrong type must report not ok")
	}
	if _, _, ok := Peek[v1.Pod](f.m, Key{Context: "other", Type: "pods"}); ok {
		t.Fatal("peek must not cross contexts")
	}
	if f.api.calls() != calls {
		t.Fatal("peek triggered a LIST")
	}

	// A metadata shard cannot carry full objects.
	md := f.acquire(t, Key{Context: "ctx", Type: "nodes"}, Hint{Mode: ModeMetadata})
	defer md.Release()
	waitSynced(t, md)
	if _, _, ok := Peek[v1.Node](f.m, Key{Context: "ctx", Type: "nodes"}); ok {
		t.Fatal("peek of a metadata shard must report not ok")
	}
}

func TestPeekIsNotOkWhileBackingOff(t *testing.T) {
	f := newFixture(t)
	f.api.setItems("42", pod("a", "p1", "1"))
	l := f.acquire(t, podsKey, Hint{})
	defer l.Release()
	waitSynced(t, l)
	f.api.mu.Lock()
	f.api.watchErr = apierrors.NewServiceUnavailable("down")
	f.api.mu.Unlock()
	// Break the current watch so the reflector reconnects into the failure.
	f.m.Resync()
	eventually(t, "backoff", func() bool { return l.State().Phase == PhaseBackoff })
	if _, _, ok := Peek[v1.Pod](f.m, podsKey); ok {
		t.Fatal("stale data must not be peekable during backoff")
	}
}

func TestMetadataModeConvertsInProcessAndIsDegraded(t *testing.T) {
	f := newFixture(t)
	f.api.setItems("10", pod("a", "p1", "1"))
	f.api.newWatcher()
	l := f.acquire(t, podsKey, Hint{Mode: ModeMetadata})
	defer l.Release()
	waitSynced(t, l)
	st := l.State()
	if st.Mode != ModeMetadata || !st.Degraded || st.DegradedReason == "" {
		t.Fatalf("state = %+v", st)
	}
	snap, _ := l.Snapshot()
	pm, ok := snap[0].Obj.(*metav1.PartialObjectMetadata)
	if !ok || pm.Name != "p1" || pm.ManagedFields != nil {
		t.Fatalf("entry object = %#v", snap[0].Obj)
	}
}

func TestTableModeDegradesToFullOnFakeClientset(t *testing.T) {
	f := newFixture(t)
	f.api.setItems("10", pod("a", "p1", "1"))
	l := f.acquire(t, podsKey, Hint{Mode: ModeTable})
	defer l.Release()
	waitSynced(t, l)
	st := l.State()
	if st.Mode != ModeFull || st.Requested != ModeTable || !st.Degraded {
		t.Fatalf("state = %+v", st)
	}
}

func TestDisableTableForcesFull(t *testing.T) {
	yes := true
	f := newFixture(t, func(o *Options) { o.DisableTable = &yes })
	l := f.acquire(t, podsKey, Hint{Mode: ModeTable})
	defer l.Release()
	waitSynced(t, l)
	if st := l.State(); st.Mode != ModeFull || !strings.Contains(st.DegradedReason, EnvDisableTable) {
		t.Fatalf("state = %+v", st)
	}
}

func TestStrongerAcquireUpgradesShard(t *testing.T) {
	f := newFixture(t)
	f.api.setItems("10", pod("a", "p1", "1"))
	md := f.acquire(t, podsKey, Hint{Mode: ModeMetadata})
	waitSynced(t, md)
	c := &collector{}
	sub := md.Subscribe(c.fn, SubOptions{})
	defer sub.Cancel()

	full := f.acquire(t, podsKey, Hint{Mode: ModeFull})
	defer full.Release()
	defer md.Release()
	eventually(t, "upgraded", func() bool {
		st := full.State()
		return st.Synced && st.Mode == ModeFull
	})
	snap, _ := full.Snapshot()
	if _, ok := snap[0].Obj.(*v1.Pod); !ok {
		t.Fatalf("object after upgrade = %T", snap[0].Obj)
	}
	eventually(t, "reset delivered", func() bool {
		for _, ch := range c.all() {
			if ch.Type == ChangeReset {
				return true
			}
		}
		return false
	})
}

func TestByteBudgetEvictsOnlyIdleShardsLRU(t *testing.T) {
	f := newFixture(t)
	f.api.setItems("10")
	var pods []v1.Pod
	for i := 0; i < 200; i++ {
		pods = append(pods, pod("a", "pod-"+strings.Repeat("x", 20)+string(rune('a'+i%26))+string(rune('a'+i/26)), "1"))
	}
	f.api.setItems("10", pods...)

	busy := f.acquire(t, Key{Context: "ctx", Type: "pods", Namespace: "n1"}, Hint{})
	waitSynced(t, busy)
	idle1 := f.acquire(t, Key{Context: "ctx", Type: "pods", Namespace: "n2"}, Hint{})
	waitSynced(t, idle1)
	idle2 := f.acquire(t, Key{Context: "ctx", Type: "pods", Namespace: "n3"}, Hint{})
	waitSynced(t, idle2)

	// Sampling is every 16th object, so each 200-pod shard has samples.
	if b := idle1.sh.estimatedBytes(); b == 0 {
		t.Fatal("no byte estimate")
	}
	idle1.Release()
	f.clock.Advance(time.Second)
	idle2.Release()

	f.m.SetMaxBytes(1) // everything is over budget
	eventually(t, "idle shards evicted", func() bool { return len(f.m.Stats().Shards) == 1 })
	select {
	case <-busy.Done():
		t.Fatal("a leased shard must never be evicted")
	default:
	}
	busy.Release()
}

func TestStatsReportShardState(t *testing.T) {
	f := newFixture(t)
	f.api.setItems("10", pod("a", "p1", "1"))
	l := f.acquire(t, podsKey, Hint{Mode: ModeTable})
	defer l.Release()
	waitSynced(t, l)
	st := f.m.Stats()
	if len(st.Shards) != 1 || st.Entries != 1 {
		t.Fatalf("stats = %+v", st)
	}
	s := st.Shards[0]
	if s.Type != "pods" || s.Phase != "watching" || !s.Degraded || s.Refs != 1 || s.Mode != "full" || s.Requested != "table" {
		t.Fatalf("shard stats = %+v", s)
	}
}

func TestPollingRelistsInsteadOfWatching(t *testing.T) {
	f := newFixture(t, func(o *Options) { o.PollInterval = 30 * time.Second })
	f.api.setItems("10", pod("a", "p1", "1"))
	l := f.acquire(t, podsKey, Hint{})
	defer l.Release()
	waitSynced(t, l)
	c := &collector{}
	sub := l.Subscribe(c.fn, SubOptions{})
	defer sub.Cancel()
	f.api.setItems("11", pod("a", "p1", "5"), pod("a", "p2", "6"))
	eventually(t, "poll wait registered", func() bool { return len(f.clock.Waits()) >= 1 })
	f.clock.Advance(31 * time.Second)
	eventually(t, "diff delivered", func() bool { return len(c.all()) == 2 })
	if got := len(f.api.rvs()); got != 0 {
		t.Fatalf("polling mode opened %d watches", got)
	}
}

func TestResyncRelistsAndDiffsWithoutClearing(t *testing.T) {
	f := newFixture(t)
	f.api.setItems("10", pod("a", "p1", "1"), pod("a", "p2", "2"))
	l := f.acquire(t, podsKey, Hint{})
	defer l.Release()
	waitSynced(t, l)
	c := &collector{}
	sub := l.Subscribe(c.fn, SubOptions{})
	defer sub.Cancel()
	before := f.api.calls()
	f.api.setItems("11", pod("a", "p1", "1"), pod("a", "p2", "9"))
	f.m.Resync()
	eventually(t, "relist", func() bool { return f.api.calls() > before && l.State().RV == "11" })
	if got := c.summary(); !reflect.DeepEqual(got, []string{"Modified:p2"}) {
		t.Fatalf("resync changes = %v", got)
	}
}

func TestUnsupportedTypeIsRejected(t *testing.T) {
	f := newFixture(t)
	if _, err := f.m.Acquire(Key{Context: "ctx", Type: "bogus"}, Hint{}); err == nil {
		t.Fatal("expected error")
	}
	if _, err := f.m.Acquire(Key{Context: "ctx", Type: "crd:bad"}, Hint{}); err == nil {
		t.Fatal("expected error for malformed crd type")
	}
}

func TestClusterScopedTypeDropsNamespace(t *testing.T) {
	f := newFixture(t)
	a := f.acquire(t, Key{Context: "ctx", Type: "nodes", Namespace: "x"}, Hint{})
	b := f.acquire(t, Key{Context: "ctx", Type: "nodes"}, Hint{})
	defer a.Release()
	defer b.Release()
	if a.sh != b.sh {
		t.Fatal("cluster-scoped namespace should be normalised away")
	}
}

// The usual way a stale version is reported is on the stream: the server accepts the watch and
// then sends a Gone error event. That error must not count as healthy traffic, or a server that
// keeps doing it gets a relist every round with no pause.
func TestGoneOnTheStreamRepeatedlyBacksOff(t *testing.T) {
	f := newFixture(t)
	f.api.setItems("10", pod("a", "p1", "1"))
	gone := &metav1.Status{Status: metav1.StatusFailure, Reason: metav1.StatusReasonGone, Code: 410, Message: "too old"}
	w1 := f.api.newWatcher()
	l := f.acquire(t, podsKey, Hint{})
	defer l.Release()
	waitSynced(t, l)

	// First rejection: the relist is immediate.
	w2 := f.api.newWatcher()
	w1.Error(gone)
	eventually(t, "immediate relist", func() bool { return f.api.calls() >= 2 })
	if waits := f.clock.Waits(); len(waits) != 0 {
		t.Fatalf("the first relist must not wait, waits = %v", waits)
	}

	// Second rejection in a row, with no object event in between: now it waits.
	f.api.newWatcher()
	w2.Error(gone)
	eventually(t, "paced relist", func() bool { return len(f.clock.Waits()) >= 1 })
	if w := f.clock.Waits()[0]; w != 2*time.Second {
		t.Fatalf("wait before the second relist = %v, want 2s", w)
	}
	if n := f.api.calls(); n != 2 {
		t.Fatalf("list calls = %d, want no relist until the wait is over", n)
	}
}
