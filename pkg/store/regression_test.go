package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
)

// listGate blocks LIST calls of a resource until released.
func gateList(cs *fake.Clientset, resource string, gate chan struct{}) {
	cs.PrependReactor("list", resource, func(clienttesting.Action) (bool, runtime.Object, error) {
		<-gate
		return false, nil, nil
	})
}

func TestEvictionOfOwnShardAfterSyncDoesNotDeadlock(t *testing.T) {
	f := newFixture(t)
	f.api.setItems("10", pod("a", "p1", "1"))
	gate := make(chan struct{})
	gateList(f.cs, "pods", gate)
	f.m.SetMaxBytes(1)

	l := f.acquire(t, podsKey, Hint{})
	l.Release() // idle (refs==0, lingering) while its first LIST is still running
	close(gate)

	select {
	case <-l.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("shard that evicts itself after its LIST never stopped")
	}
	done := make(chan struct{})
	go func() { f.m.Reset(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Reset hung after self-eviction")
	}
}

// scriptSource serves a fixed pod list and a scripted watch.
type scriptSource struct {
	mode     Mode
	pods     []*v1.Pod
	rv       string
	watchErr error
	mu       sync.Mutex
	lists    int
}

func (s *scriptSource) Mode() Mode { return s.mode }

func (s *scriptSource) List(_ context.Context, _ ListOpts, onPage func(Page)) (string, error) {
	s.mu.Lock()
	s.lists++
	s.mu.Unlock()
	items := make([]runtime.Object, 0, len(s.pods))
	for _, p := range s.pods {
		items = append(items, p.DeepCopy())
	}
	onPage(Page{Items: items, Loaded: len(items)})
	return s.rv, nil
}

func (s *scriptSource) Watch(context.Context, string) (watch.Interface, error) {
	if s.watchErr != nil {
		return nil, s.watchErr
	}
	return watch.NewFake(), nil
}

func (s *scriptSource) listCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lists
}

func TestModeFallbackDuringWatchRelistsOnFullSource(t *testing.T) {
	tableish := &scriptSource{mode: ModeTable, pods: []*v1.Pod{podPtr("a", "p1", "1")}, rv: "10",
		watchErr: ErrModeUnsupported}
	full := &scriptSource{mode: ModeFull, pods: []*v1.Pod{podPtr("a", "p1", "1"), podPtr("a", "p2", "2")}, rv: "20"}
	cfg := shardConfig{
		key: podsKey, clock: realClock{},
		pageSize: func() int64 { return 0 }, pollEvery: func() time.Duration { return 0 },
		baseDelay: time.Millisecond, maxDelay: time.Millisecond,
		build: func(Key, Mode) (Source, Mode, string, error) { return full, ModeFull, "", nil },
	}
	s := newShard(cfg, tableish, ModeTable, ModeTable, "")
	s.start(context.Background())
	defer s.stop()
	eventually(t, "relist on the full source", func() bool {
		st := s.State()
		return st.Synced && st.Mode == ModeFull && st.Entries == 2
	})
	if full.listCount() == 0 {
		t.Fatal("the full source was never listed")
	}
}

func TestSameResourceVersionDifferentShapeIsAChange(t *testing.T) {
	s := newBenchShard(&scriptSource{mode: ModeFull, rv: "1"})
	table := &Entry{UID: "u1", Namespace: "a", Name: "p1", RV: "5",
		Obj: &metav1.PartialObjectMetadata{ObjectMeta: metav1.ObjectMeta{Name: "p1", UID: types.UID("u1")}}, Cells: []interface{}{"p1"}}
	full := &Entry{UID: "u1", Namespace: "a", Name: "p1", RV: "5", Obj: podPtr("a", "p1", "5")}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.applyLocked(watch.Added, table); !ok {
		t.Fatal("first apply produced no change")
	}
	if _, ok := s.applyLocked(watch.Added, table); ok {
		t.Fatal("same shape and version must stay an idempotent replay")
	}
	c, ok := s.applyLocked(watch.Added, full)
	if !ok || c.Type != ChangeModified {
		t.Fatalf("shape change at the same version = %v, %v", c.Type, ok)
	}
	if _, isPod := s.entries["u1"].Obj.(*v1.Pod); !isPod {
		t.Fatalf("stored object = %T", s.entries["u1"].Obj)
	}
}

func TestStopBeforeStartNeverRunsTheReflector(t *testing.T) {
	src := &scriptSource{mode: ModeFull, rv: "1"}
	s := newBenchShard(src)
	s.stop()
	s.start(context.Background())
	time.Sleep(20 * time.Millisecond)
	if src.listCount() != 0 {
		t.Fatal("reflector ran for a shard that was already stopped")
	}
	select {
	case <-s.done:
		t.Fatal("done closed without a reflector")
	default:
	}
}

func TestAcquireRacingFallbackIsRaceFree(t *testing.T) {
	f := newFixture(t)
	f.api.setItems("10", pod("a", "p1", "1"))
	base := f.acquire(t, Key{Context: "ctx", Type: "pods"}, Hint{Mode: ModeTable})
	defer base.Release()
	waitSynced(t, base)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			l, err := f.m.Acquire(Key{Context: "ctx", Type: "pods", Namespace: "a"}, Hint{Mode: ModeFull})
			if err == nil {
				l.Release()
			}
		}()
		go func() {
			defer wg.Done()
			base.sh.fallbackToFull(errors.New("x"))
		}()
	}
	wg.Wait()
}

func TestSecretsNeverHoldValuesEvenInFullMode(t *testing.T) {
	cs := fake.NewSimpleClientset(&v1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: "a", Name: "s1", UID: "su1", ResourceVersion: "1"},
		Type:       v1.SecretTypeOpaque,
		Data:       map[string][]byte{"password": []byte("hunter2"), "user": []byte("bob")},
		StringData: map[string]string{"x": "y"},
	})
	clock := newManualClock()
	m := New(&fakeProvider{cs: cs}, Options{Clock: clock})
	t.Cleanup(m.Close)
	l, err := m.Acquire(Key{Context: "ctx", Type: "secrets"}, Hint{Mode: ModeTable})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Release()
	waitSynced(t, l)
	snap, _ := l.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("entries = %d", len(snap))
	}
	sec, ok := snap[0].Obj.(*v1.Secret)
	if !ok {
		t.Fatalf("object = %T", snap[0].Obj)
	}
	if len(sec.Data) != 2 || sec.StringData != nil {
		t.Fatalf("keys must be kept and stringData dropped: %+v", sec)
	}
	for k, v := range sec.Data {
		if v != nil {
			t.Fatalf("value of %q survived", k)
		}
	}
	if l.sh.peekable() {
		t.Fatal("a secrets shard must never be served as a warm object cache")
	}
}

// A build failure while falling back to full objects must wait the normal backoff instead of
// retrying in a tight loop.
func TestFallbackBuildFailureBacksOff(t *testing.T) {
	clock := newManualClock()
	src := &scriptSource{mode: ModeTable, rv: "1"}
	listFails := &unsupportedListSource{scriptSource: src}
	cfg := shardConfig{
		key: podsKey, clock: clock,
		pageSize: func() int64 { return 0 }, pollEvery: func() time.Duration { return 0 },
		baseDelay: time.Second, maxDelay: time.Minute,
		build: func(Key, Mode) (Source, Mode, string, error) { return nil, ModeFull, "", errors.New("cannot build") },
	}
	s := newShard(cfg, listFails, ModeTable, ModeTable, "")
	s.start(context.Background())
	defer s.stop()
	eventually(t, "backoff wait", func() bool { return len(clock.Waits()) >= 1 })
	before := listFails.attempts()
	time.Sleep(50 * time.Millisecond)
	if got := listFails.attempts(); got != before {
		t.Fatalf("list attempts grew from %d to %d without a wait: tight loop", before, got)
	}
	if w := clock.Waits()[0]; w != 2*time.Second {
		t.Fatalf("wait = %v, want 2s", w)
	}
}

type unsupportedListSource struct {
	*scriptSource
	n int32
}

func (u *unsupportedListSource) List(context.Context, ListOpts, func(Page)) (string, error) {
	atomic.AddInt32(&u.n, 1)
	return "", ErrModeUnsupported
}

func (u *unsupportedListSource) attempts() int { return int(atomic.LoadInt32(&u.n)) }

// A reflector panic must unregister the shard so a later Acquire builds a fresh one.
func TestPanickedShardIsReplacedOnNextAcquire(t *testing.T) {
	f := newFixture(t)
	f.api.setItems("10", pod("a", "p1", "1"))
	var fired int32
	f.cs.PrependReactor("list", "pods", func(clienttesting.Action) (bool, runtime.Object, error) {
		if atomic.AddInt32(&fired, 1) == 1 {
			panic("boom")
		}
		return false, nil, nil
	})
	first := f.acquire(t, podsKey, Hint{})
	select {
	case <-first.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("panicked shard was never stopped")
	}
	first.Release()
	second := f.acquire(t, podsKey, Hint{})
	defer second.Release()
	if second.sh == first.sh {
		t.Fatal("Acquire returned the panicked shard")
	}
	waitSynced(t, second)
}

// An empty store must encode its shard list as [] (the frontend iterates it).
func TestEmptyStatsEncodeShardsAsEmptyArray(t *testing.T) {
	f := newFixture(t)
	b, err := json.Marshal(f.m.Stats())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"shards":[]`) {
		t.Fatalf("stats = %s", b)
	}
}
