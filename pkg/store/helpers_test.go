package store

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/metadata"
	clienttesting "k8s.io/client-go/testing"

	"kubikles/pkg/watchevents"
)

// manualClock only moves when Advance is called.
type manualClock struct {
	mu     sync.Mutex
	now    time.Time
	timers []*manualTimer
	waits  []time.Duration // every After/AfterFunc duration requested, in order
}

type manualTimer struct {
	at      time.Time
	ch      chan time.Time
	fn      func()
	stopped bool
	fired   bool
}

func newManualClock() *manualClock { return &manualClock{now: time.Unix(1_000_000, 0)} }

func (c *manualClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *manualClock) After(d time.Duration) <-chan time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := &manualTimer{at: c.now.Add(d), ch: make(chan time.Time, 1)}
	c.timers = append(c.timers, t)
	c.waits = append(c.waits, d)
	return t.ch
}

func (c *manualClock) AfterFunc(d time.Duration, f func()) func() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := &manualTimer{at: c.now.Add(d), fn: f}
	c.timers = append(c.timers, t)
	c.waits = append(c.waits, d)
	return func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		was := !t.stopped && !t.fired
		t.stopped = true
		return was
	}
}

func (c *manualClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	var due []*manualTimer
	for _, t := range c.timers {
		if !t.stopped && !t.fired && !t.at.After(c.now) {
			t.fired = true
			due = append(due, t)
		}
	}
	now := c.now
	c.mu.Unlock()
	for _, t := range due {
		if t.fn != nil {
			t.fn()
		} else {
			t.ch <- now
		}
	}
}

func (c *manualClock) Waits() []time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]time.Duration(nil), c.waits...)
}

func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// fakeAPI scripts the list and watch behaviour of a fake clientset.
type fakeAPI struct {
	mu        sync.Mutex
	items     []v1.Pod
	pages     [][]v1.Pod // when set, each LIST call serves the next page and wraps around
	pageIdx   int
	listRV    string
	listErr   error
	listCalls int
	watchers  chan *watch.FakeWatcher
	watchRVs  []string
	watchErr  error
	remaining *int64
}

func newFakeAPI() *fakeAPI {
	return &fakeAPI{watchers: make(chan *watch.FakeWatcher, 16), listRV: "100"}
}

func (f *fakeAPI) install(cs *fake.Clientset) {
	cs.PrependReactor("list", "pods", func(clienttesting.Action) (bool, runtime.Object, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.listCalls++
		if f.listErr != nil {
			return true, nil, f.listErr
		}
		list := &v1.PodList{ListMeta: metav1.ListMeta{ResourceVersion: f.listRV}}
		if f.pages != nil {
			i := f.pageIdx % len(f.pages)
			f.pageIdx++
			list.Items = f.pages[i]
			if i < len(f.pages)-1 {
				list.Continue = "next"
				if f.remaining != nil {
					r := *f.remaining
					list.RemainingItemCount = &r
				}
			}
			return true, list, nil
		}
		list.Items = append([]v1.Pod(nil), f.items...)
		return true, list, nil
	})
	cs.PrependWatchReactor("pods", func(a clienttesting.Action) (bool, watch.Interface, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		wa := a.(clienttesting.WatchActionImpl)
		f.watchRVs = append(f.watchRVs, wa.WatchRestrictions.ResourceVersion)
		if f.watchErr != nil {
			return true, nil, f.watchErr
		}
		select {
		case w := <-f.watchers:
			return true, w, nil
		default:
			return true, watch.NewFake(), nil
		}
	})
}

func (f *fakeAPI) newWatcher() *watch.FakeWatcher {
	w := watch.NewFakeWithChanSize(256, false)
	f.watchers <- w
	return w
}

func (f *fakeAPI) setItems(rv string, pods ...v1.Pod) {
	f.mu.Lock()
	f.items, f.listRV = pods, rv
	f.mu.Unlock()
}

func (f *fakeAPI) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.listCalls
}

func (f *fakeAPI) rvs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.watchRVs...)
}

func pod(ns, name, rv string) v1.Pod {
	return v1.Pod{ObjectMeta: metav1.ObjectMeta{
		Namespace: ns, Name: name, UID: types.UID("uid-" + ns + "-" + name), ResourceVersion: rv,
		ManagedFields: []metav1.ManagedFieldsEntry{{Manager: "kubectl"}},
	}}
}

func podPtr(ns, name, rv string) *v1.Pod { p := pod(ns, name, rv); return &p }

type fakeProvider struct {
	mu   sync.Mutex
	cs   kubernetes.Interface
	dyn  dynamic.Interface
	md   metadata.Interface
	made int
}

func (p *fakeProvider) Kube(string) (kubernetes.Interface, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.made++
	return p.cs, nil
}
func (p *fakeProvider) Dynamic(string) (dynamic.Interface, error) {
	if p.dyn == nil {
		return nil, fmt.Errorf("no dynamic client")
	}
	return p.dyn, nil
}
func (p *fakeProvider) Metadata(string) (metadata.Interface, error) {
	if p.md == nil {
		return nil, ErrNoMetadata
	}
	return p.md, nil
}

type sinks struct {
	mu     sync.Mutex
	status []string
	errs   []string
}

func (s *sinks) opts(o Options) Options {
	o.StatusSink = func(e watchevents.Status) {
		s.mu.Lock()
		s.status = append(s.status, e.Status)
		s.mu.Unlock()
	}
	o.ErrorSink = func(e watchevents.Error) {
		s.mu.Lock()
		s.errs = append(s.errs, e.Error)
		s.mu.Unlock()
	}
	return o
}

func (s *sinks) statuses() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.status...)
}

func (s *sinks) errors() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.errs...)
}

type fixture struct {
	api   *fakeAPI
	cs    *fake.Clientset
	clock *manualClock
	m     *Manager
	sinks *sinks
}

func newFixture(t *testing.T, mutate ...func(*Options)) *fixture {
	t.Helper()
	cs := fake.NewSimpleClientset()
	api := newFakeAPI()
	api.install(cs)
	clock := newManualClock()
	sk := &sinks{}
	opts := Options{Clock: clock, Jitter: func(d time.Duration) time.Duration { return d }}
	for _, f := range mutate {
		f(&opts)
	}
	m := New(&fakeProvider{cs: cs}, sk.opts(opts))
	t.Cleanup(m.Close)
	return &fixture{api: api, cs: cs, clock: clock, m: m, sinks: sk}
}

func (f *fixture) acquire(t *testing.T, key Key, hint Hint) *Lease {
	t.Helper()
	l, err := f.m.Acquire(key, hint)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func waitSynced(t *testing.T, l *Lease) {
	t.Helper()
	eventually(t, "shard synced", func() bool { return l.State().Synced })
}

var podsKey = Key{Context: "ctx", Type: "pods"}

// collector gathers delivered changes.
type collector struct {
	mu      sync.Mutex
	changes []Change
}

func (c *collector) fn(batch []Change) {
	c.mu.Lock()
	c.changes = append(c.changes, batch...)
	c.mu.Unlock()
}

func (c *collector) all() []Change {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]Change(nil), c.changes...)
}

func (c *collector) summary() []string {
	var out []string
	for _, ch := range c.all() {
		name := ""
		switch {
		case ch.New != nil:
			name = ch.New.Name
		case ch.Old != nil:
			name = ch.Old.Name
		}
		out = append(out, ch.Type.String()+":"+name)
	}
	return out
}

var _ = context.Background
