package rows

import (
	"sync"
	"sync/atomic"

	"kubikles/pkg/store"
)

// podGen is one generation of a workload view's pod feed: the leases on the pod shards its
// namespace selection needs, the subscriptions that keep the index current and the index. A view
// swaps in a new generation when its namespace selection changes; deliveries of an older one are
// ignored.
type podGen struct {
	v    *View
	idx  *podIndex
	subs []*podSub
	dead atomic.Bool
	// mu makes the seq filter plus apply of a delivery atomic against resync's snapshot plus
	// reset, as the view's own subscriptions are (they hold v.mu across both).
	mu sync.Mutex
}

type podSub struct {
	g       *podGen
	lease   Lease
	sub     *store.Subscription
	snapSeq atomic.Uint64
}

// connectPods leases the pod shards of a namespace selection and builds the index from their
// snapshots. A pod type that cannot be leased leaves an empty feed: the workload list then shows
// no pods rather than failing to open.
func (m *ViewManager) connectPods(v *View, spec ViewSpec) *podGen {
	g := &podGen{v: v, idx: newPodIndex()}
	def, ok := Lookup("pods")
	if !ok {
		return g
	}
	var entries []*store.Entry
	for _, k := range m.planKeys(def, ViewSpec{Namespaces: spec.Namespaces}, false) {
		lease, err := m.cfg.Provider.Acquire(k, store.Hint{Mode: store.ModeFull, Linger: store.ViewLinger})
		if err != nil {
			continue
		}
		ps := &podSub{g: g, lease: lease}
		ps.sub = lease.Subscribe(ps.deliver, store.SubOptions{})
		ps.snapSeq.Store(ps.sub.Seq)
		entries = append(entries, ps.sub.Snapshot...)
		g.subs = append(g.subs, ps)
	}
	g.idx.reset(entries)
	return g
}

// stop cancels the subscriptions and releases the leases.
func (g *podGen) stop() {
	if g == nil || g.dead.Swap(true) {
		return
	}
	for _, s := range g.subs {
		s.sub.Cancel()
		s.lease.Release()
	}
}

// ready reports whether every pod shard has completed a list (or cannot, being forbidden), so an
// empty Pods cell means "no pods" rather than "not loaded yet".
func (g *podGen) ready() bool {
	for _, s := range g.subs {
		st := s.lease.State()
		if !st.Synced && !st.Forbidden {
			return false
		}
	}
	return true
}

// resync rebuilds the index from the current snapshots of every shard.
func (g *podGen) resync() {
	g.mu.Lock()
	defer g.mu.Unlock()
	var entries []*store.Entry
	for _, s := range g.subs {
		snap, seq := s.lease.Snapshot()
		entries = append(entries, snap...)
		s.snapSeq.Store(seq)
	}
	g.idx.reset(entries)
}

func (s *podSub) deliver(batch []store.Change) {
	g := s.g
	if g.dead.Load() {
		return
	}
	reset := len(batch) > bulkThreshold
	for _, c := range batch {
		if c.Type == store.ChangeReset {
			reset = true
			break
		}
	}
	if reset {
		g.resync()
		g.v.onPods(g, nil, true)
		return
	}
	g.mu.Lock()
	seq := s.snapSeq.Load()
	fresh := batch[:0:0]
	for _, c := range batch {
		if c.Seq > seq {
			fresh = append(fresh, c)
		}
	}
	deltas := g.idx.apply(fresh)
	g.mu.Unlock()
	if len(deltas) > 0 {
		g.v.onPods(g, deltas, false)
	}
}

// onPods folds changes of the pod index into the view.
func (v *View) onPods(g *podGen, deltas []podDelta, all bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.closed || v.cfg == nil || v.rebuilding || v.pods != g {
		// A rebuild in progress compares the index generation when it installs its core; a
		// delivery of a replaced generation is obsolete.
		return
	}
	eff := newEffect()
	ver := v.versioner()
	v.core.applyPods(deltas, all, v.tcols, v.m.now(), ver, &eff)
	v.afterApplyLocked(&eff, ver, false)
}
