package rows

import (
	"sort"
	"sync"
	"sync/atomic"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"kubikles/pkg/store"
)

// bulkThreshold is the batch size above which a view resyncs from a snapshot instead of applying
// changes one by one.
const bulkThreshold = 2000

// maxStaleReported caps the selection-stale uids carried in one notice.
const maxStaleReported = 64

// Lease is the part of a store lease a view needs; it exists so views can be tested against fake
// shards.
type Lease interface {
	Snapshot() ([]*store.Entry, uint64)
	Subscribe(fn func([]store.Change), opts store.SubOptions) *store.Subscription
	State() store.State
	Columns() []metav1.TableColumnDefinition
	Release()
}

// Provider hands out leases on store shards.
type Provider interface {
	Acquire(key store.Key, hint store.Hint) (Lease, error)
}

// StoreProvider adapts a store.Manager.
type StoreProvider struct{ M *store.Manager }

func (p StoreProvider) Acquire(key store.Key, hint store.Hint) (Lease, error) {
	l, err := p.M.Acquire(key, hint)
	if err != nil {
		return nil, err
	}
	return l, nil
}

type viewSub struct {
	v       *View
	key     store.Key
	lease   Lease
	sub     *store.Subscription
	snapSeq uint64
	dead    atomic.Bool
	// syncedAt is the shard sequence at which the shard was first seen synced, plus one (zero:
	// not seen synced yet). Changes up to it are the initial list, not events that happened.
	syncedAt atomic.Uint64
}

// settled reports whether a change with the given sequence happened after the shard's initial
// list, so a transition may be derived from it.
func (s *viewSub) settled(seq uint64) bool {
	at := s.syncedAt.Load()
	if at == 0 {
		st := s.lease.State()
		if !st.Synced {
			return false
		}
		at = st.Seq + 1
		s.syncedAt.Store(at)
	}
	return seq >= at
}

func (s *viewSub) stop() {
	s.dead.Store(true)
	if s.sub != nil {
		s.sub.Cancel()
	}
	s.lease.Release()
}

type pendingBatch struct {
	vs    *viewSub
	batch []store.Change
}

// View is one open list: a query over a shard, a sorted index of the matching entries, and the
// bookkeeping that makes windows cheap to serve and cheap to follow.
type View struct {
	m   *ViewManager
	id  string
	def *Spec

	// opMu serialises structural operations (spec updates, resyncs).
	opMu sync.Mutex

	mu      sync.Mutex
	spec    ViewSpec
	cfg     *compiled
	core    *core
	version uint64
	// lowWater is the version at which the current projection was built; a client that has an
	// older version must be sent every row.
	lowWater uint64
	subs     []*viewSub
	closed   bool
	active   bool
	tcols    map[string]int

	rebuilding  bool
	pending     []pendingBatch
	broadDenied bool

	// ov is the latest snapshot of each overlay source (by name) and ovSubs the subscriptions
	// that keep it current, one per Spec.Overlays entry in order.
	ov     map[string]*OverlaySnapshot
	ovSubs []*OverlaySub
	// pods is the current pod feed of a workload view (see Spec.Derived).
	pods *podGen
	// trans collects the pod transitions of a view with NotifyOn "pods" until the next notice.
	trans transitionLog

	touched time.Time

	// Last served window, for deciding whether a change concerns the client.
	winSet    bool
	winOffset int
	winLimit  int
	// Observability counters (guarded by mu).
	statWindows, statRows, statNotices, statBytes uint64
	statLastWindow                                int
	winUIDs                                       map[string]struct{}
	served                                        map[string]uint64

	// selection holds the selected uids that were in scope when last checked; a uid leaves it
	// once reported stale, so each departure is reported once.
	selection map[string]struct{}
	selStale  []string

	// Notice state.
	dirty     bool
	dirtyWin  bool
	closedWhy string
	lastSig   stateSig
}

type stateSig struct {
	total, filtered, loaded, expected int
	phase, errMsg                     string
	forbidden                         bool
	overlays                          string
}

func (v *View) ID() string { return v.id }

// ClientToken and ClientID identify the owner.
func (v *View) owner() (token, clientID string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.spec.ClientToken, v.spec.ClientID
}

// --- structural operations -------------------------------------------------------------------

// connect acquires leases for keys and subscribes to each. On failure everything acquired is
// released.
func (v *View) connect(keys []store.Key) (subs []*viewSub, entries []*store.Entry, err error) {
	for _, k := range keys {
		lease, aerr := v.m.cfg.Provider.Acquire(k, store.Hint{Mode: v.def.Needs, Linger: store.ViewLinger, Prune: v.def.FullPrune})
		if aerr != nil {
			err = aerr
			break
		}
		vs := &viewSub{v: v, key: k, lease: lease}
		vs.sub = lease.Subscribe(vs.deliver, store.SubOptions{})
		vs.snapSeq = vs.sub.Seq
		entries = append(entries, vs.sub.Snapshot...)
		subs = append(subs, vs)
	}
	if err != nil {
		for _, s := range subs {
			s.stop()
		}
		return nil, nil, err
	}
	return subs, entries, nil
}

func tableColumns(subs []*viewSub) map[string]int {
	for _, s := range subs {
		if cols := s.lease.Columns(); len(cols) > 0 {
			m := make(map[string]int, len(cols))
			for i, c := range cols {
				m[c.Name] = i
			}
			return m
		}
	}
	return nil
}

// rebuild installs a fresh core for cfg. With reconnect the leases are re-planned and
// re-acquired (the namespace selection or plan changed); otherwise the current leases are
// re-snapshotted. The sort runs without the view lock.
func (v *View) rebuild(cfg *compiled, reconnect bool) error {
	v.opMu.Lock()
	defer v.opMu.Unlock()
	return v.rebuildLocked(cfg, reconnect)
}

// rebuildCurrent rebuilds with whatever cfg is current once opMu is held. Callers that decided to
// rebuild from an observation made under v.mu (a reset batch, a replan) must not carry that
// cfg across the wait for opMu: an Update in between installs a newer spec, and rebuilding
// with the stale one would silently revert it.
func (v *View) rebuildCurrent(reconnect bool) error {
	v.opMu.Lock()
	defer v.opMu.Unlock()
	v.mu.Lock()
	cfg := v.cfg
	v.mu.Unlock()
	return v.rebuildLocked(cfg, reconnect)
}

// rebuildLocked is rebuild for a caller that already holds opMu.
func (v *View) rebuildLocked(cfg *compiled, reconnect bool) error {
	for round := 0; round < 3; round++ {
		again, err := v.rebuildOnce(cfg, reconnect)
		if err != nil || !again {
			return err
		}
		reconnect = false
	}
	return nil
}

func (v *View) rebuildOnce(cfg *compiled, reconnect bool) (again bool, err error) {
	v.mu.Lock()
	if v.closed {
		v.mu.Unlock()
		return false, ErrViewNotFound
	}
	v.rebuilding = true
	old := v.subs
	broadDenied := v.broadDenied
	v.mu.Unlock()

	var (
		subs     []*viewSub
		entries  []*store.Entry
		snapSeqs = map[*viewSub]uint64{}
	)
	if reconnect {
		keys := v.m.planKeys(v.def, cfg.spec, broadDenied)
		subs, entries, err = v.connect(keys)
		if err != nil {
			v.mu.Lock()
			v.rebuilding = false
			v.flushPendingLocked()
			v.mu.Unlock()
			return false, err
		}
		for _, s := range subs {
			snapSeqs[s] = s.snapSeq
		}
	} else {
		subs = old
		for _, s := range subs {
			snap, seq := s.lease.Snapshot()
			entries = append(entries, snap...)
			snapSeqs[s] = seq
		}
	}

	var newPods *podGen
	// The pod feed (a lease on every pod shard plus an index kept current on every pod event) is
	// held only while a column, sort, filter or facet of the view reads it.
	needPods := len(v.def.Derived) > 0 && cfg.dvPlan.needsPods()
	dropPods := false
	v.mu.Lock()
	havePods := v.pods != nil
	v.mu.Unlock()
	switch {
	case needPods && (reconnect || !havePods):
		newPods = v.m.connectPods(v, cfg.spec)
	case !needPods && havePods:
		dropPods = true
	}
	tcols := tableColumns(subs)
	now := v.m.now()
	v.mu.Lock()
	pg := v.pods
	if newPods != nil {
		pg = newPods
	} else if dropPods {
		pg = nil
	}
	var (
		podIdx   *podIndex
		builtGen uint64
	)
	if pg != nil {
		podIdx = pg.idx
		builtGen = podIdx.Gen()
	}
	nextVersion := v.version + 1
	builtOv := make(map[string]*OverlaySnapshot, len(v.ov))
	for k, sn := range v.ov {
		builtOv[k] = sn
	}
	v.mu.Unlock()
	nc := buildCoreDv(cfg, entries, tcols, now, nextVersion, builtOv, podIdx)

	v.mu.Lock()
	if v.closed {
		v.rebuilding = false
		v.mu.Unlock()
		if reconnect {
			for _, s := range subs {
				s.stop()
			}
		}
		newPods.stop()
		return false, ErrViewNotFound
	}
	oldPods := v.pods
	if newPods != nil {
		v.pods = newPods
	} else if dropPods {
		v.pods = nil
	}
	v.version = nextVersion
	v.lowWater = nextVersion
	if !cfg.notifyPods {
		v.trans = transitionLog{}
	}
	v.cfg = cfg
	v.spec = cfg.spec
	v.core = nc
	v.tcols = tcols
	v.subs = subs
	v.served = nil
	for s, seq := range snapSeqs {
		s.snapSeq = seq
	}
	if reconnect {
		for _, s := range old {
			s.dead.Store(true)
		}
	}
	// Replay changes that arrived while the sort ran.
	pend := v.pending
	v.pending = nil
	v.rebuilding = false
	eff := newEffect()
	ver := v.versioner()
	// Overlay snapshots that arrived while the sort ran were not in the core's copy.
	for i := range cfg.ovPlans {
		name := cfg.ovPlans[i].name
		if cur := v.ov[name]; cur != builtOv[name] {
			nc.applyOverlay(i, OverlayUpdate{Snap: cur, All: true}, tcols, now, ver, &eff)
		}
	}
	// Pod changes that arrived while the core was built were not in its cells.
	if pg != nil && pg.idx.Gen() != builtGen {
		nc.applyPods(nil, true, tcols, now, ver, &eff)
	}
	for _, pb := range pend {
		if pb.vs.dead.Load() {
			continue
		}
		for _, c := range pb.batch {
			if c.Type == store.ChangeReset {
				again = true
				continue
			}
			if c.Seq <= pb.vs.snapSeq {
				continue
			}
			v.applyChangeLocked(c, &eff, ver, pb.vs.settled(c.Seq))
		}
	}
	v.afterApplyLocked(&eff, ver, true)
	v.reconcileSelectionLocked()
	v.mu.Unlock()

	if reconnect {
		for _, s := range old {
			s.sub.Cancel()
			s.lease.Release()
		}
	}
	if newPods != nil || dropPods {
		oldPods.stop()
	}
	return again, nil
}

// flushPendingLocked applies changes buffered during a failed rebuild to the unchanged core.
func (v *View) flushPendingLocked() {
	pend := v.pending
	v.pending = nil
	eff := newEffect()
	ver := v.versioner()
	for _, pb := range pend {
		if pb.vs.dead.Load() {
			continue
		}
		for _, c := range pb.batch {
			if c.Type == store.ChangeReset || c.Seq <= pb.vs.snapSeq {
				continue
			}
			v.applyChangeLocked(c, &eff, ver, pb.vs.settled(c.Seq))
		}
	}
	v.afterApplyLocked(&eff, ver, false)
}

// --- incremental updates ---------------------------------------------------------------------

// versioner returns a function that bumps the version at most once per batch.
func (v *View) versioner() func() uint64 {
	bumped := false
	return func() uint64 {
		if !bumped {
			v.version++
			bumped = true
		}
		return v.version
	}
}

func (v *View) applyChangeLocked(c store.Change, eff *effect, ver func() uint64, settled bool) {
	now := v.m.now()
	if settled {
		v.noteTransitionLocked(c)
	}
	switch c.Type {
	case store.ChangeAdded, store.ChangeModified:
		if c.New != nil {
			v.core.upsert(c.New, v.tcols, now, ver, eff)
		}
	case store.ChangeDeleted:
		if c.Old != nil {
			v.core.remove(c.Old, eff)
		}
	}
}

// deliver receives batches from a store subscription.
func (s *viewSub) deliver(batch []store.Change) {
	v := s.v
	if s.dead.Load() {
		return
	}
	reset := len(batch) > bulkThreshold
	for _, c := range batch {
		if c.Type == store.ChangeReset {
			reset = true
			break
		}
	}
	v.mu.Lock()
	if v.closed || s.dead.Load() {
		v.mu.Unlock()
		return
	}
	if v.rebuilding {
		v.pending = append(v.pending, pendingBatch{vs: s, batch: batch})
		v.mu.Unlock()
		return
	}
	if reset {
		v.mu.Unlock()
		_ = v.rebuildCurrent(false)
		return
	}
	v.tcols = tableColumns(v.subs)
	eff := newEffect()
	ver := v.versioner()
	for _, c := range batch {
		if c.Seq <= s.snapSeq {
			continue
		}
		v.applyChangeLocked(c, &eff, ver, s.settled(c.Seq))
	}
	v.afterApplyLocked(&eff, ver, false)
	v.mu.Unlock()
}

// afterApplyLocked finalises a batch: stamps the version for counts-only changes and decides
// whether the client needs a notice.
func (v *View) afterApplyLocked(eff *effect, ver func() uint64, structural bool) {
	v.reconcileSelectionLocked()
	if eff.visible || eff.counts {
		ver()
	}
	winHit := structural
	if !winHit && v.winSet {
		end := v.winOffset + v.winLimit
		if eff.shiftPos >= 0 && eff.shiftPos < end {
			winHit = true
		}
		for _, p := range eff.modPos {
			if p >= v.winOffset && p < end {
				winHit = true
				break
			}
		}
	}
	if winHit {
		v.markDirtyLocked(true)
	}
	v.checkStateLocked()
}

func (v *View) markDirtyLocked(win bool) {
	if win {
		v.dirtyWin = true
	}
	if !v.dirty {
		v.dirty = true
		v.m.onDirty()
	}
}

func (v *View) signatureLocked() (stateSig, ViewState) {
	st := v.stateLocked(false)
	return stateSig{st.Total, st.Filtered, st.Loaded, st.Expected, st.Phase, st.Error, st.Forbidden, overlaySig(st.Overlays)}, st
}

// checkStateLocked marks the view dirty when counts or the load phase moved.
func (v *View) checkStateLocked() {
	sig, _ := v.signatureLocked()
	if sig != v.lastSig {
		v.lastSig = sig
		v.markDirtyLocked(false)
	}
}

// refreshState is called periodically: shard load progress changes without any entry changing.
func (v *View) refreshState() {
	v.mu.Lock()
	if v.closed || v.rebuilding {
		v.mu.Unlock()
		return
	}
	v.checkStateLocked()
	replan := v.shouldReplanLocked()
	v.mu.Unlock()
	if replan {
		go func() {
			v.mu.Lock()
			v.broadDenied = true
			v.mu.Unlock()
			_ = v.rebuildCurrent(true)
		}()
	}
}

// shouldReplanLocked: a cluster-wide shard serving an explicit namespace selection was forbidden,
// so plan per-namespace shards instead (the user may only have access to those).
func (v *View) shouldReplanLocked() bool {
	if v.broadDenied || !v.def.Namespaced || v.cfg.nsSet == nil {
		return false
	}
	for _, s := range v.subs {
		if s.key.Namespace == "" && s.lease.State().Forbidden {
			return true
		}
	}
	return false
}

// --- overlays --------------------------------------------------------------------------------

// attachOverlays subscribes to the overlay sources of the view's type and loads their current
// snapshots. It runs before the first build so the first projection already has them.
func (v *View) attachOverlays() {
	hub := v.m.cfg.Overlays
	if hub == nil || len(v.def.Overlays) == 0 {
		return
	}
	ctxName := ""
	if v.m.cfg.Context != nil {
		ctxName = v.m.cfg.Context()
	}
	subs := make([]*OverlaySub, len(v.def.Overlays))
	ov := make(map[string]*OverlaySnapshot, len(v.def.Overlays))
	v.mu.Lock()
	active := v.active
	v.mu.Unlock()
	for i, o := range v.def.Overlays {
		idx := i
		sub, snap := hub.Subscribe(ctxName, o.Name, active, func(up OverlayUpdate) { v.onOverlay(idx, up) })
		subs[i] = sub
		if snap != nil {
			ov[o.Name] = snap
		}
	}
	v.mu.Lock()
	for name, snap := range ov {
		// A snapshot published between Subscribe and here already reached onOverlay.
		if cur := v.ov[name]; cur == nil || cur.Seq < snap.Seq {
			if v.ov == nil {
				v.ov = map[string]*OverlaySnapshot{}
			}
			v.ov[name] = snap
		}
	}
	if v.closed {
		// A close ran while the subscriptions were being made and did not see them.
		v.mu.Unlock()
		for _, s := range subs {
			s.Cancel()
		}
		return
	}
	v.ovSubs = subs
	v.mu.Unlock()
}

// onOverlay receives a published snapshot of the i-th overlay source of the view's type.
func (v *View) onOverlay(i int, up OverlayUpdate) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.closed || v.cfg == nil || i >= len(v.def.Overlays) {
		return
	}
	name := v.def.Overlays[i].Name
	if cur := v.ov[name]; cur != nil && up.Snap.Seq <= cur.Seq {
		return // already folded in (a snapshot read at subscribe time)
	}
	if v.ov == nil {
		v.ov = map[string]*OverlaySnapshot{}
	}
	v.ov[name] = up.Snap
	if v.rebuilding {
		return // the rebuild notices the newer snapshot when it installs its core
	}
	eff := newEffect()
	ver := v.versioner()
	v.core.applyOverlay(i, up, v.tcols, v.m.now(), ver, &eff)
	v.afterApplyLocked(&eff, ver, false)
}

// --- state -----------------------------------------------------------------------------------

func (v *View) stateLocked(withFilterable bool) ViewState {
	st := ViewState{Version: v.version, Total: len(v.core.rows), Filtered: len(v.core.index)}
	if len(v.cfg.ovPlans) > 0 || v.pods != nil {
		st.Overlays = make(map[string]OverlayInfo, len(v.cfg.ovPlans)+1)
		for _, p := range v.cfg.ovPlans {
			st.Overlays[p.name] = OverlayInfo{Available: v.ov[p.name].AvailableState()}
		}
		if v.pods != nil {
			st.Overlays["pods"] = OverlayInfo{Available: BoolPtr(v.pods.ready())}
		}
	}
	var (
		anyUnsynced, anyRefreshing, anyIncomplete bool
	)
	for _, s := range v.subs {
		ls := s.lease.State()
		st.Loaded += ls.Loaded
		st.Expected += ls.Expected
		if ls.Degraded {
			st.Degraded = true
		}
		if ls.Forbidden {
			st.Forbidden = true
		}
		if ls.Error != "" && st.Error == "" {
			st.Error = ls.Error
		}
		switch {
		case ls.Phase == store.PhaseIncomplete || ls.Forbidden:
			anyIncomplete = true
		case !ls.Synced:
			anyUnsynced = true
		case ls.Phase == store.PhaseRefreshing || ls.Phase == store.PhaseLoading || ls.Phase == store.PhaseBackoff:
			anyRefreshing = true
		}
	}
	switch {
	case anyIncomplete:
		st.Phase = PhaseIncomplete
	case anyUnsynced:
		st.Phase = PhaseLoading
	case anyRefreshing:
		st.Phase = PhaseRefreshing
	default:
		st.Phase = PhaseComplete
	}
	if st.Phase == PhaseComplete {
		st.Error = ""
	}
	if st.Loaded == 0 && st.Phase == PhaseComplete {
		st.Loaded = st.Total
	}
	if withFilterable {
		st.Filterable = v.filterableLocked()
	}
	return st
}

func (v *View) filterableLocked() map[string]bool {
	out := make(map[string]bool, len(v.def.Columns))
	for _, col := range v.def.Columns {
		switch col.FilterType {
		case FilterRegex, FilterNumeric:
			out[col.Key] = true
		case FilterSelect:
			out[col.Key] = false
		default:
			out[col.Key] = false
		}
	}
	for _, f := range v.core.facets {
		out[f.col.Key] = !f.over && len(f.counts) > 0
	}
	return out
}

// State returns the current state of the view.
func (v *View) State() (ViewState, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.closed {
		return ViewState{}, ErrViewNotFound
	}
	v.touched = v.m.now()
	return v.stateLocked(true), nil
}

// --- windows ---------------------------------------------------------------------------------

func (v *View) windowRow(r *viewRow) WindowRow {
	return WindowRow{Seq: r.seq, NS: r.ns, Name: r.name, TS: r.ts, Del: r.del, C: r.cells}
}

// Window returns rows [offset, offset+limit) of the filtered, sorted list. have is the version
// of the last window the client applied. A row is included when it changed after have, when the
// client may not have it (it was not part of the last window served, or was served after have),
// or when have is zero or predates the current projection. Order always lists every uid.
func (v *View) Window(offset, limit int, have uint64) (ViewWindow, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.closed {
		return ViewWindow{}, ErrViewNotFound
	}
	v.touched = v.m.now()
	if offset < 0 {
		offset = 0
	}
	if limit < 0 {
		limit = 0
	}
	n := len(v.core.index)
	start := offset
	if start > n {
		start = n
	}
	end := start + limit
	if end > n {
		end = n
	}
	full := have == 0 || have < v.lowWater || v.m.cfg.ForceFullWindows
	w := ViewWindow{
		Version: v.version, Total: len(v.core.rows), Filtered: n, Offset: offset,
		Order: make([]string, 0, end-start), Rows: make(map[string]WindowRow),
	}
	served := make(map[string]uint64, end-start)
	uids := make(map[string]struct{}, end-start)
	for _, r := range v.core.index[start:end] {
		w.Order = append(w.Order, r.id)
		uids[r.id] = struct{}{}
		prev := v.served[r.id]
		if full || r.seq > have || prev == 0 || prev > have {
			w.Rows[r.id] = v.windowRow(r)
			served[r.id] = v.version
		} else {
			served[r.id] = prev
		}
	}
	v.served = served
	v.winSet, v.winOffset, v.winLimit, v.winUIDs = true, offset, limit, uids
	w.State = v.stateLocked(true)
	v.statWindows++
	v.statRows += uint64(len(w.Rows))
	v.statLastWindow = len(w.Order)
	v.statBytes += estimateWindowBytes(&w)
	return w, nil
}

// Refs returns row references for [offset, offset+limit); a limit of zero or less means to the end.
// A non-zero expectedVersion is the version the caller's indexes were taken at: when the list has
// moved on, ErrViewChanged is returned instead of references to rows the caller did not see.
func (v *View) Refs(offset, limit int, expectedVersion uint64) ([]RowRef, ViewState, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.closed {
		return nil, ViewState{}, ErrViewNotFound
	}
	v.touched = v.m.now()
	if expectedVersion != 0 && v.version != expectedVersion {
		return nil, v.stateLocked(false), ErrViewChanged
	}
	n := len(v.core.index)
	if offset < 0 {
		offset = 0
	}
	if offset > n {
		offset = n
	}
	end := n
	// Compare against the remaining count: offset+limit wraps for a huge limit.
	if limit > 0 && limit < n-offset {
		end = offset + limit
	}
	out := make([]RowRef, 0, end-offset)
	for _, r := range v.core.index[offset:end] {
		out = append(out, RowRef{UID: r.id, NS: r.ns, Name: r.name})
	}
	return out, v.stateLocked(false), nil
}

// RowsSnapshot returns the whole filtered list in order. The rows are immutable, so the caller can
// stream them without holding the view.
func (v *View) RowsSnapshot() ([]WindowRowRef, []string, ViewState, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.closed {
		return nil, nil, ViewState{}, ErrViewNotFound
	}
	v.touched = v.m.now()
	out := make([]WindowRowRef, len(v.core.index))
	for i, r := range v.core.index {
		out[i] = WindowRowRef{UID: r.id, WindowRow: v.windowRow(r)}
	}
	return out, v.cfg.colKeys, v.stateLocked(false), nil
}

// WindowRowRef is a projected row with its uid.
type WindowRowRef struct {
	UID string
	WindowRow
}

// Facet returns the select values of a column over the whole scope.
func (v *View) Facet(column string) (Facet, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.closed {
		return Facet{}, ErrViewNotFound
	}
	v.touched = v.m.now()
	for i, f := range v.core.facets {
		if f.col.Key != column {
			continue
		}
		if f.over {
			v.recountFacetLocked(i)
			f = v.core.facets[i]
		}
		return v.facetResultLocked(f), nil
	}
	if v.def.Column(column) == nil {
		return Facet{}, &UnknownColumnError{Type: v.def.Type, Column: column}
	}
	return Facet{Column: column, Counts: map[string]int{}, Values: []string{}, Suppressed: true}, nil
}

// recountFacetLocked rescans the scope for a facet that had overflowed, in case removals brought
// it back under the cap.
func (v *View) recountFacetLocked(i int) {
	f := newFacet(v.core.facets[i].col)
	for _, r := range v.core.rows {
		f.add(r.fvals[i])
		if f.over {
			break
		}
	}
	v.core.facets[i] = f
}

func (v *View) facetResultLocked(f *facet) Facet {
	if f.over || len(f.counts) == 0 {
		return Facet{Column: f.col.Key, Counts: map[string]int{}, Values: []string{}, Suppressed: true}
	}
	counts := make(map[string]int, len(f.counts))
	for k, n := range f.counts {
		counts[k] = n
	}
	values := append([]string(nil), f.col.Predefined...)
	var dynamic []string
	for k := range f.counts {
		if !f.predefined[k] {
			dynamic = append(dynamic, k)
		}
	}
	sort.Slice(dynamic, func(i, j int) bool { return v.core.coll.CompareString(dynamic[i], dynamic[j]) < 0 })
	return Facet{Column: f.col.Key, Values: append(values, dynamic...), Counts: counts}
}

// --- selection -------------------------------------------------------------------------------

// SetSelection records the selected uids. The ack reports those that are not in the filtered list.
func (v *View) SetSelection(uids []string) (SelectionAck, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.closed {
		return SelectionAck{}, ErrViewNotFound
	}
	v.touched = v.m.now()
	v.selection = make(map[string]struct{}, len(uids))
	v.selStale = nil
	ack := SelectionAck{Stale: []string{}}
	for _, u := range uids {
		if r, ok := v.core.rows[u]; ok && r.inIndex {
			v.selection[u] = struct{}{}
			ack.Selected++
		} else {
			ack.Stale = append(ack.Stale, u)
		}
	}
	return ack, nil
}

// reconcileSelectionLocked keeps the selection a subset of the filtered list, as the legacy list
// does by pruning to the visible rows: an id that left the scope or fell out of the search and
// column filters is dropped and reported as stale.
func (v *View) reconcileSelectionLocked() {
	if len(v.selection) == 0 {
		return
	}
	for id := range v.selection {
		if r, ok := v.core.rows[id]; !ok || !r.inIndex {
			delete(v.selection, id)
			v.selStale = append(v.selStale, id)
			v.markDirtyLocked(false)
		}
	}
}

// --- lifecycle -------------------------------------------------------------------------------

// takeNotice builds the pending notice, if any.
func (v *View) takeNotice() (ViewNotice, bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if !v.dirty {
		return ViewNotice{}, false
	}
	st := v.stateLocked(false)
	n := ViewNotice{
		V: v.id, Version: st.Version, Total: st.Total, Filtered: st.Filtered, Phase: st.Phase,
		Loaded: st.Loaded, Expected: st.Expected, WindowChanged: v.dirtyWin, Closed: v.closedWhy,
		Overlays: st.Overlays,
	}
	n.Transitions = v.trans.drain()
	if len(v.selStale) > 0 {
		n.SelectionStale = v.selStale
		if len(n.SelectionStale) > maxStaleReported {
			n.SelectionStale = n.SelectionStale[:maxStaleReported]
		}
		v.selStale = nil
	}
	v.dirty, v.dirtyWin = false, false
	v.statNotices++
	return n, true
}

// SetActive records whether the client currently shows the view. An inactive view keeps its
// state but its notices are suppressed until it is active again (server mode: a hidden tab).
func (v *View) SetActive(active bool) {
	v.mu.Lock()
	v.active = active
	v.touched = v.m.now()
	wake := false
	if active {
		// A view that went dirty while inactive keeps v.dirty set (its notice was skipped), so
		// markDirtyLocked would not signal again: wake the notifier explicitly or every later
		// change stays silent because the dirty flag never clears.
		wake = v.dirty
		v.markDirtyLocked(true)
	}
	subs := v.ovSubs
	v.mu.Unlock()
	if wake {
		v.m.onDirty()
	}
	// An inactive view does not need its overlay sources polled.
	for _, s := range subs {
		s.SetWanted(active)
	}
}

func (v *View) isActive() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.active
}

func (v *View) idleSince() time.Time {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.touched
}

// close releases the leases. reason is reported in a final notice when non-empty.
func (v *View) close(reason string) {
	v.opMu.Lock()
	defer v.opMu.Unlock()
	v.mu.Lock()
	if v.closed {
		v.mu.Unlock()
		return
	}
	v.closed = true
	subs := v.subs
	v.subs = nil
	v.pending = nil
	ovSubs := v.ovSubs
	v.ovSubs = nil
	pods := v.pods
	v.pods = nil
	v.mu.Unlock()
	for _, s := range ovSubs {
		s.Cancel()
	}
	pods.stop()
	for _, s := range subs {
		s.stop()
	}
}

// reevaluateTime re-runs time-dependent filters (age) against the current time.
func (v *View) reevaluateTime() {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.closed || v.rebuilding || !v.cfg.timeBound {
		return
	}
	eff := newEffect()
	ver := v.versioner()
	v.core.reevaluate(v.tcols, v.m.now(), ver, &eff)
	v.afterApplyLocked(&eff, ver, false)
}

// estimateWindowBytes approximates the JSON size of a window without encoding it: the order
// list, the per-row envelope and the cells. It feeds the performance panel, which shows a rate.
func estimateWindowBytes(w *ViewWindow) uint64 {
	n := uint64(160)
	for _, id := range w.Order {
		n += uint64(len(id)) + 3
	}
	for id, r := range w.Rows {
		n += uint64(len(id)) + uint64(len(r.NS)) + uint64(len(r.Name)) + uint64(len(r.TS)) + 56
		for _, c := range r.C {
			switch x := c.(type) {
			case string:
				n += uint64(len(x)) + 3
			case nil:
				n += 5
			default:
				n += 8
			}
		}
	}
	return n
}
