package rows

import (
	"fmt"
	"sync"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"kubikles/pkg/store"
)

// fakeShard is an in-memory stand-in for a store shard: it holds entries, numbers changes and
// delivers them to subscribers either inline or from a goroutine.
type fakeShard struct {
	mu      sync.Mutex
	entries map[string]*store.Entry
	seq     uint64
	subs    []*fakeSub
	state   store.State
	async   bool
}

type fakeSub struct {
	fn func([]store.Change)
	ns string
	// delivery is serialised per subscriber like the real store
	dmu sync.Mutex
}

func newFakeShard() *fakeShard {
	return &fakeShard{entries: map[string]*store.Entry{}, state: store.State{Phase: store.PhaseWatching, Synced: true}}
}

func (s *fakeShard) upsert(e *store.Entry) {
	s.mu.Lock()
	id := e.ID()
	old := s.entries[id]
	s.seq++
	e.Seq = s.seq
	s.entries[id] = e
	c := store.Change{Type: store.ChangeAdded, New: e, Seq: s.seq}
	if old != nil {
		c = store.Change{Type: store.ChangeModified, Old: old, New: e, Seq: s.seq}
	}
	s.state.Entries = len(s.entries)
	subs := append([]*fakeSub(nil), s.subs...)
	s.mu.Unlock()
	s.publish(subs, []store.Change{c})
}

func (s *fakeShard) remove(id string) {
	s.mu.Lock()
	old := s.entries[id]
	if old == nil {
		s.mu.Unlock()
		return
	}
	delete(s.entries, id)
	s.seq++
	c := store.Change{Type: store.ChangeDeleted, Old: old, Seq: s.seq}
	s.state.Entries = len(s.entries)
	subs := append([]*fakeSub(nil), s.subs...)
	s.mu.Unlock()
	s.publish(subs, []store.Change{c})
}

// batch applies several upserts and removals as one delivered batch.
func (s *fakeShard) batch(upserts []*store.Entry, removes []string) {
	s.mu.Lock()
	var changes []store.Change
	for _, e := range upserts {
		id := e.ID()
		old := s.entries[id]
		s.seq++
		e.Seq = s.seq
		s.entries[id] = e
		c := store.Change{Type: store.ChangeAdded, New: e, Seq: s.seq}
		if old != nil {
			c = store.Change{Type: store.ChangeModified, Old: old, New: e, Seq: s.seq}
		}
		changes = append(changes, c)
	}
	for _, id := range removes {
		if old := s.entries[id]; old != nil {
			delete(s.entries, id)
			s.seq++
			changes = append(changes, store.Change{Type: store.ChangeDeleted, Old: old, Seq: s.seq})
		}
	}
	s.state.Entries = len(s.entries)
	subs := append([]*fakeSub(nil), s.subs...)
	s.mu.Unlock()
	s.publish(subs, changes)
}

func (s *fakeShard) reset() {
	s.mu.Lock()
	subs := append([]*fakeSub(nil), s.subs...)
	s.mu.Unlock()
	s.publish(subs, []store.Change{{Type: store.ChangeReset}})
}

func (s *fakeShard) publish(subs []*fakeSub, changes []store.Change) {
	for _, sub := range subs {
		var mine []store.Change
		for _, c := range changes {
			ns := ""
			switch {
			case c.New != nil:
				ns = c.New.Namespace
			case c.Old != nil:
				ns = c.Old.Namespace
			}
			if c.Type == store.ChangeReset || sub.ns == "" || sub.ns == ns {
				mine = append(mine, c)
			}
		}
		if len(mine) == 0 {
			continue
		}
		deliver := func(sub *fakeSub, mine []store.Change) {
			sub.dmu.Lock()
			defer sub.dmu.Unlock()
			sub.fn(mine)
		}
		if s.async {
			go deliver(sub, mine)
		} else {
			deliver(sub, mine)
		}
	}
}

func (s *fakeShard) setState(f func(*store.State)) {
	s.mu.Lock()
	f(&s.state)
	s.mu.Unlock()
}

type fakeLease struct {
	s        *fakeShard
	ns       string
	released bool
	cols     []metav1.TableColumnDefinition
}

func (l *fakeLease) Snapshot() ([]*store.Entry, uint64) {
	l.s.mu.Lock()
	defer l.s.mu.Unlock()
	var out []*store.Entry
	for _, e := range l.s.entries {
		if l.ns == "" || e.Namespace == l.ns {
			out = append(out, e)
		}
	}
	return out, l.s.seq
}

func (l *fakeLease) Subscribe(fn func([]store.Change), opts store.SubOptions) *store.Subscription {
	l.s.mu.Lock()
	defer l.s.mu.Unlock()
	var snap []*store.Entry
	for _, e := range l.s.entries {
		if l.ns == "" || e.Namespace == l.ns {
			snap = append(snap, e)
		}
	}
	l.s.subs = append(l.s.subs, &fakeSub{fn: fn, ns: l.ns})
	return &store.Subscription{Snapshot: snap, Seq: l.s.seq}
}

func (l *fakeLease) State() store.State {
	l.s.mu.Lock()
	defer l.s.mu.Unlock()
	return l.s.state
}
func (l *fakeLease) Columns() []metav1.TableColumnDefinition { return l.cols }
func (l *fakeLease) Release()                                { l.released = true }

type fakeProvider struct {
	mu     sync.Mutex
	shards map[string]*fakeShard
	acq    []store.Key
	leases []*fakeLease
}

func newFakeProvider() *fakeProvider { return &fakeProvider{shards: map[string]*fakeShard{}} }

func (p *fakeProvider) shard(typ string) *fakeShard {
	p.mu.Lock()
	defer p.mu.Unlock()
	s := p.shards[typ]
	if s == nil {
		s = newFakeShard()
		p.shards[typ] = s
	}
	return s
}

func (p *fakeProvider) Acquire(key store.Key, hint store.Hint) (Lease, error) {
	p.mu.Lock()
	p.acq = append(p.acq, key)
	p.mu.Unlock()
	if key.Type == "namespaces" && key.Namespace == "" {
		p.mu.Lock()
		_, have := p.shards["namespaces"]
		p.mu.Unlock()
		if !have {
			return nil, fmt.Errorf("no namespaces shard in this fake")
		}
	}
	l := &fakeLease{s: p.shard(key.Type), ns: key.Namespace}
	p.mu.Lock()
	p.leases = append(p.leases, l)
	p.mu.Unlock()
	return l, nil
}

func (p *fakeProvider) acquired() []store.Key {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]store.Key(nil), p.acq...)
}

func (p *fakeProvider) activeLeases() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	n := 0
	for _, l := range p.leases {
		if !l.released {
			n++
		}
	}
	return n
}

type manualClock struct {
	mu sync.Mutex
	t  time.Time
}

func newManualClock() *manualClock {
	return &manualClock{t: time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)}
}
func (c *manualClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}
func (c *manualClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

type testEnv struct {
	t     *testing.T
	p     *fakeProvider
	clock *manualClock
	m     *ViewManager
	dirty int
	mu    sync.Mutex
}

func newEnv(t *testing.T, mutate ...func(*Config)) *testEnv {
	t.Helper()
	env := &testEnv{t: t, p: newFakeProvider(), clock: newManualClock()}
	cfg := Config{
		Provider:  env.p,
		Context:   func() string { return "ctx" },
		Clock:     env.clock,
		NoJanitor: true,
		OnDirty:   func() { env.mu.Lock(); env.dirty++; env.mu.Unlock() },
	}
	for _, f := range mutate {
		f(&cfg)
	}
	env.m = NewManager(cfg)
	t.Cleanup(env.m.Close)
	return env
}

func (e *testEnv) dirtyCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.dirty
}

// cm builds a ConfigMap entry. The uid derives from namespace and name.
func cm(ns, name string, created time.Time, data map[string]string) *store.Entry {
	obj := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns, Name: name, UID: types.UID("uid-" + ns + "-" + name), ResourceVersion: "1",
			CreationTimestamp: metav1.NewTime(created),
		},
		Data: data,
	}
	return &store.Entry{UID: string(obj.UID), Namespace: ns, Name: name, RV: "1", Obj: obj}
}

// bump produces a modified copy of an entry with a new resourceVersion.
func bump(e *store.Entry, mutate func(*corev1.ConfigMap)) *store.Entry {
	old := e.Obj.(*corev1.ConfigMap)
	cp := old.DeepCopy()
	cp.ResourceVersion = old.ResourceVersion + "x"
	if mutate != nil {
		mutate(cp)
	}
	return &store.Entry{UID: e.UID, Namespace: e.Namespace, Name: e.Name, RV: cp.ResourceVersion, Obj: cp}
}

func ids(entries ...*store.Entry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.ID()
	}
	return out
}
