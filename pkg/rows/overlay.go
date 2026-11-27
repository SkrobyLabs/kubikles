package rows

import (
	"sort"
	"sync"
)

// Overlays are columns whose values do not come from the stored object but from a second,
// periodically refreshed source keyed by object (pod and node metrics). The hub below holds the
// latest snapshot of each source per kube context, tells subscribed views what changed between
// two snapshots and reports whether anybody still needs the source, so the poller behind it can
// stop when no view is open or the window is hidden.

// OverlayRecord is what a source knows about one object: a fixed-order vector of numbers. The
// layout is part of the overlay's contract with the spec's cells and the frontend.
type OverlayRecord []float64

func (r OverlayRecord) equal(o OverlayRecord) bool {
	if len(r) != len(o) {
		return false
	}
	for i := range r {
		if r[i] != o[i] {
			return false
		}
	}
	return true
}

// OverlaySnapshot is one immutable state of a source.
type OverlaySnapshot struct {
	// Seq numbers the snapshots of one source within a context.
	Seq uint64
	// Available is nil until the source has been asked, then whether it could answer (the
	// "metricsAvailable" state of the legacy lists).
	Available *bool
	// Records are the per-object values, keyed as the spec's Overlay.Key produces.
	Records map[string]OverlayRecord
}

// Lookup returns the record for a key; it is safe on a nil snapshot and returns nil when the
// source has nothing for the object.
func (s *OverlaySnapshot) Lookup(key string) OverlayRecord {
	if s == nil {
		return nil
	}
	return s.Records[key]
}

// AvailableState returns the availability, nil when unknown. Safe on a nil snapshot.
func (s *OverlaySnapshot) AvailableState() *bool {
	if s == nil {
		return nil
	}
	return s.Available
}

// OverlayUpdate is what subscribers receive when a source publishes.
type OverlayUpdate struct {
	Snap *OverlaySnapshot
	// All says every object may have changed (first snapshot, or no usable previous one).
	All bool
	// Changed lists the keys whose record appeared, vanished or differs from the previous
	// snapshot. Empty with All false means only availability moved.
	Changed []string
	// AvailabilityChanged reports a change of Snap.Available.
	AvailabilityChanged bool
}

// OverlayInfo is the part of a source a view reports to its client.
type OverlayInfo struct {
	Available *bool `json:"available"`
}

type overlayKey struct{ context, name string }

type overlayEntry struct {
	snap   *OverlaySnapshot
	subs   map[*OverlaySub]struct{}
	wanted int
}

// OverlayHub keeps the latest snapshot per (context, source) and the subscribers of each.
type OverlayHub struct {
	// pubMu serialises Publish including the delivery to subscribers, so snapshots of a source
	// reach every subscriber in Seq order.
	pubMu   sync.Mutex
	mu      sync.Mutex
	gen     uint64 // incremented by Reset
	entries map[overlayKey]*overlayEntry
	// onDemand is called, outside the lock, when the number of subscribers that want a source
	// goes from zero to some or back to zero.
	onDemand func(contextName, name string, wanted bool)
}

// NewOverlayHub creates a hub. onDemand may be nil.
func NewOverlayHub(onDemand func(contextName, name string, wanted bool)) *OverlayHub {
	return &OverlayHub{entries: map[overlayKey]*overlayEntry{}, onDemand: onDemand}
}

// SetDemandHook replaces the demand callback (wiring happens after construction).
func (h *OverlayHub) SetDemandHook(f func(contextName, name string, wanted bool)) {
	h.mu.Lock()
	h.onDemand = f
	h.mu.Unlock()
}

// OverlaySub is one subscription.
type OverlaySub struct {
	hub    *OverlayHub
	key    overlayKey
	fn     func(OverlayUpdate)
	wanted bool
	dead   bool
}

func (h *OverlayHub) entry(k overlayKey) *overlayEntry {
	e := h.entries[k]
	if e == nil {
		e = &overlayEntry{subs: map[*OverlaySub]struct{}{}}
		h.entries[k] = e
	}
	return e
}

// Subscribe registers fn for a source and returns the subscription with the snapshot that was
// current at that instant (nil when the source never published). Updates delivered afterwards
// all have a higher Seq than that snapshot. wanted says whether the subscriber needs the source
// to be polled right now.
func (h *OverlayHub) Subscribe(contextName, name string, wanted bool, fn func(OverlayUpdate)) (*OverlaySub, *OverlaySnapshot) {
	h.mu.Lock()
	k := overlayKey{contextName, name}
	e := h.entry(k)
	s := &OverlaySub{hub: h, key: k, fn: fn, wanted: wanted}
	e.subs[s] = struct{}{}
	cb := h.demandLocked(e, e.wanted, boolInt(wanted), k)
	snap := e.snap
	h.mu.Unlock()
	if cb != nil {
		cb()
	}
	return s, snap
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// demandLocked applies a change of the wanted count and returns the callback to run once the
// lock is released, when demand crossed zero.
func (h *OverlayHub) demandLocked(e *overlayEntry, before, delta int, k overlayKey) func() {
	e.wanted += delta
	if h.onDemand == nil || (before > 0) == (e.wanted > 0) {
		return nil
	}
	f, on := h.onDemand, e.wanted > 0
	return func() { f(k.context, k.name, on) }
}

// SetWanted records whether the subscriber needs the source (an inactive view does not).
func (s *OverlaySub) SetWanted(wanted bool) {
	h := s.hub
	h.mu.Lock()
	if s.dead || s.wanted == wanted {
		h.mu.Unlock()
		return
	}
	e := h.entries[s.key]
	before := e.wanted
	s.wanted = wanted
	delta := 1
	if !wanted {
		delta = -1
	}
	cb := h.demandLocked(e, before, delta, s.key)
	h.mu.Unlock()
	if cb != nil {
		cb()
	}
}

// Cancel removes the subscription.
func (s *OverlaySub) Cancel() {
	h := s.hub
	h.mu.Lock()
	if s.dead {
		h.mu.Unlock()
		return
	}
	s.dead = true
	e := h.entries[s.key]
	var cb func()
	if e != nil {
		delete(e.subs, s)
		if s.wanted {
			cb = h.demandLocked(e, e.wanted, -1, s.key)
		}
	}
	h.mu.Unlock()
	if cb != nil {
		cb()
	}
}

// Current returns the latest snapshot of a source, nil when it never published.
func (h *OverlayHub) Current(contextName, name string) *OverlaySnapshot {
	h.mu.Lock()
	defer h.mu.Unlock()
	if e := h.entries[overlayKey{contextName, name}]; e != nil {
		return e.snap
	}
	return nil
}

// Demanded reports whether any subscriber currently wants the source.
func (h *OverlayHub) Demanded(contextName, name string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	e := h.entries[overlayKey{contextName, name}]
	return e != nil && e.wanted > 0
}

// Publish installs a new snapshot, computes the delta against the previous one and delivers it
// to the subscribers in registration-independent but consistent order (one publisher per source
// at a time is the caller's contract). It returns false, and delivers nothing, when the snapshot
// equals the previous one.
func (h *OverlayHub) Publish(contextName, name string, available *bool, records map[string]OverlayRecord) bool {
	return h.publish(contextName, name, available, records, nil)
}

// Generation identifies the hub's state between Resets. A producer reads it before fetching and
// publishes with PublishAt so a result fetched before a Reset is dropped.
func (h *OverlayHub) Generation() uint64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.gen
}

// PublishAt is Publish that does nothing (and returns false) when the hub was Reset since gen was
// read.
func (h *OverlayHub) PublishAt(gen uint64, contextName, name string, available *bool, records map[string]OverlayRecord) bool {
	return h.publish(contextName, name, available, records, &gen)
}

func (h *OverlayHub) publish(contextName, name string, available *bool, records map[string]OverlayRecord, gen *uint64) bool {
	h.pubMu.Lock()
	defer h.pubMu.Unlock()
	h.mu.Lock()
	if gen != nil && *gen != h.gen {
		h.mu.Unlock()
		return false
	}
	k := overlayKey{contextName, name}
	e := h.entry(k)
	prev := e.snap
	next := &OverlaySnapshot{Available: available, Records: records}
	if next.Records == nil {
		next.Records = map[string]OverlayRecord{}
	}
	up, changed := diffSnapshots(prev, next)
	if !changed {
		h.mu.Unlock()
		return false
	}
	e.snap = next
	subs := make([]*OverlaySub, 0, len(e.subs))
	for s := range e.subs {
		subs = append(subs, s)
	}
	h.mu.Unlock()
	for _, s := range subs {
		h.mu.Lock()
		dead := s.dead
		h.mu.Unlock()
		if !dead {
			s.fn(up)
		}
	}
	return true
}

// diffSnapshots numbers next after prev (nil prev: the first snapshot) and describes what changed
// between them. changed is false when nothing did.
func diffSnapshots(prev, next *OverlaySnapshot) (up OverlayUpdate, changed bool) {
	up = OverlayUpdate{Snap: next}
	if prev == nil {
		up.All = true
		up.AvailabilityChanged = next.Available != nil
		next.Seq = 1
		return up, true
	}
	next.Seq = prev.Seq + 1
	up.AvailabilityChanged = !sameBoolPtr(prev.Available, next.Available)
	for key, rec := range next.Records {
		if old, ok := prev.Records[key]; !ok || !old.equal(rec) {
			up.Changed = append(up.Changed, key)
		}
	}
	for key := range prev.Records {
		if _, ok := next.Records[key]; !ok {
			up.Changed = append(up.Changed, key)
		}
	}
	if len(up.Changed) == 0 && !up.AvailabilityChanged {
		return up, false
	}
	sort.Strings(up.Changed)
	return up, true
}

func sameBoolPtr(a, b *bool) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// Reset forgets every snapshot (context switch): the next subscriber starts from "unknown".
// Subscriptions stay; the views that hold them are closed by the same switch.
func (h *OverlayHub) Reset() {
	h.mu.Lock()
	h.gen++
	for k, e := range h.entries {
		e.snap = nil
		if len(e.subs) == 0 {
			delete(h.entries, k)
		}
	}
	h.mu.Unlock()
}

// BoolPtr is a helper for sources and tests.
func BoolPtr(b bool) *bool { return &b }
