package rows

import (
	"reflect"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"kubikles/pkg/store"
)

// Defaults for the lease and caps.
const (
	DefaultIdleTimeout  = 60 * time.Second
	DefaultMaxPerClient = 32
	DefaultMaxTotal     = 256

	pollEvery     = 250 * time.Millisecond
	reapEvery     = 5 * time.Second
	timeTickEvery = 30 * time.Second

	namespaceWait = 1500 * time.Millisecond
)

// Clock supplies the time; tests inject a manual one.
type Clock interface{ Now() time.Time }

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

// Config configures a ViewManager.
type Config struct {
	Provider Provider
	// Context returns the kube context new views use.
	Context func() string
	Clock   Clock
	// OnDirty is called (possibly with a view lock held, so it must not block or call back into
	// the manager) when a view gains a pending notice.
	OnDirty func()
	// Broad reports a warm cluster-wide shard of a type, for the namespace planner.
	Broad func(contextName, resourceType string) (*Observation, bool)
	// CRD resolves custom resources for views of "crd:" types. Optional; without it the base
	// columns (name, namespace, age) are used.
	CRD CRDResolver
	// Overlays carries the periodic sources (metrics) behind overlay columns. Optional; without
	// it overlay columns show their "no data" values.
	Overlays *OverlayHub

	IdleTimeout  time.Duration
	MaxPerClient int
	MaxTotal     int
	// ForceFullWindows makes every window carry all rows: the debug assertion mode that proves
	// the delta logic against full windows.
	ForceFullWindows bool
	// NoJanitor disables the background goroutine; tests drive Poll, Reap and TimeTick directly.
	NoJanitor bool
}

// ViewManager owns the open views.
type ViewManager struct {
	cfg Config

	mu      sync.Mutex
	views   map[string]*View
	byToken map[string]int
	closed  []ViewNotice

	stop chan struct{}
	wg   sync.WaitGroup
}

// NewManager creates a manager and, unless disabled, starts its janitor.
func NewManager(cfg Config) *ViewManager {
	if cfg.Clock == nil {
		cfg.Clock = realClock{}
	}
	if cfg.IdleTimeout <= 0 {
		cfg.IdleTimeout = DefaultIdleTimeout
	}
	if cfg.MaxPerClient <= 0 {
		cfg.MaxPerClient = DefaultMaxPerClient
	}
	if cfg.MaxTotal <= 0 {
		cfg.MaxTotal = DefaultMaxTotal
	}
	m := &ViewManager{cfg: cfg, views: map[string]*View{}, byToken: map[string]int{}, stop: make(chan struct{})}
	if !cfg.NoJanitor {
		m.wg.Add(1)
		go m.janitor()
	}
	return m
}

func (m *ViewManager) now() time.Time { return m.cfg.Clock.Now() }

func (m *ViewManager) onDirty() {
	if m.cfg.OnDirty != nil {
		m.cfg.OnDirty()
	}
}

func (m *ViewManager) janitor() {
	defer m.wg.Done()
	t := time.NewTicker(pollEvery)
	defer t.Stop()
	var sinceReap, sinceTime time.Duration
	for {
		select {
		case <-m.stop:
			return
		case <-t.C:
		}
		m.Poll()
		sinceReap += pollEvery
		sinceTime += pollEvery
		if sinceReap >= reapEvery {
			sinceReap = 0
			m.Reap()
		}
		if sinceTime >= timeTickEvery {
			sinceTime = 0
			m.TimeTick()
		}
	}
}

// Close stops the janitor and closes every view.
func (m *ViewManager) Close() {
	select {
	case <-m.stop:
	default:
		close(m.stop)
	}
	m.wg.Wait()
	m.closeAll("", false)
}

func (m *ViewManager) snapshotViews() []*View {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*View, 0, len(m.views))
	for _, v := range m.views {
		out = append(out, v)
	}
	return out
}

// Poll refreshes the load state of every view; shard progress changes without entry changes.
func (m *ViewManager) Poll() {
	for _, v := range m.snapshotViews() {
		v.refreshState()
	}
}

// TimeTick re-evaluates time-dependent filters (age) of every view.
func (m *ViewManager) TimeTick() {
	for _, v := range m.snapshotViews() {
		v.reevaluateTime()
	}
}

// Reap closes views not touched within the idle timeout.
func (m *ViewManager) Reap() {
	now := m.now()
	for _, v := range m.snapshotViews() {
		if now.Sub(v.idleSince()) > m.cfg.IdleTimeout {
			m.closeView(v, "expired", true)
		}
	}
}

func (m *ViewManager) get(id string) (*View, error) {
	m.mu.Lock()
	v := m.views[id]
	m.mu.Unlock()
	if v == nil {
		return nil, ErrViewNotFound
	}
	return v, nil
}

func (m *ViewManager) closeView(v *View, reason string, notify bool) {
	m.mu.Lock()
	if m.views[v.id] != v {
		m.mu.Unlock()
		return
	}
	delete(m.views, v.id)
	token, _ := v.owner()
	if m.byToken[token] > 0 {
		m.byToken[token]--
		if m.byToken[token] == 0 {
			delete(m.byToken, token)
		}
	}
	if notify && reason != "" {
		m.closed = append(m.closed, ViewNotice{V: v.id, Closed: reason})
	}
	m.mu.Unlock()
	v.close(reason)
	if notify {
		m.onDirty()
	}
}

func (m *ViewManager) closeAll(reason string, notify bool) {
	for _, v := range m.snapshotViews() {
		m.closeView(v, reason, notify)
	}
}

// Reset closes every view with a reason ("context"): the frontend re-opens from its retained
// specs. It runs wherever the store is reset.
func (m *ViewManager) Reset(reason string) { m.closeAll(reason, true) }

// OnClientDisconnect closes the views of a server-mode client at once.
func (m *ViewManager) OnClientDisconnect(clientID string) {
	if clientID == "" {
		return
	}
	for _, v := range m.snapshotViews() {
		if _, id := v.owner(); id == clientID {
			m.closeView(v, "client", false)
		}
	}
}

// Count returns the number of open views.
func (m *ViewManager) Count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.views)
}

// TakeNotices returns the pending notices of every active view plus final notices of closed
// views. Notices of inactive views stay pending.
func (m *ViewManager) TakeNotices() []ViewNotice {
	m.mu.Lock()
	out := m.closed
	m.closed = nil
	m.mu.Unlock()
	for _, v := range m.snapshotViews() {
		if !v.isActive() {
			continue
		}
		if n, ok := v.takeNotice(); ok {
			out = append(out, n)
		}
	}
	return out
}

// Open creates (or replaces) a view.
func (m *ViewManager) Open(spec ViewSpec) (ViewOpenResult, error) {
	def, ok := Lookup(spec.Type)
	dynamic := false
	if !ok {
		def, ok = m.dynamicSpec(&spec)
		if !ok {
			return ViewOpenResult{}, &UnknownTypeError{Type: spec.Type}
		}
		dynamic = true
	}
	if spec.ViewID == "" {
		spec.ViewID = uuid.NewString()
	}
	cfg, err := compileView(spec, def)
	if err != nil {
		return ViewOpenResult{}, err
	}

	// Replacing an existing view and inserting the new one happen in one critical section: two
	// concurrent Opens with the same id would otherwise both insert and leak the first view's
	// leases, overlay demand and token count.
	m.mu.Lock()
	old := m.views[spec.ViewID]
	if old != nil {
		delete(m.views, old.id)
		token, _ := old.owner()
		if m.byToken[token] > 0 {
			m.byToken[token]--
			if m.byToken[token] == 0 {
				delete(m.byToken, token)
			}
		}
	}
	if len(m.views) >= m.cfg.MaxTotal || m.byToken[spec.ClientToken] >= m.cfg.MaxPerClient {
		m.mu.Unlock()
		if old != nil {
			old.close("replaced")
		}
		return ViewOpenResult{}, ErrTooManyViews
	}
	v := &View{m: m, id: spec.ViewID, def: def, spec: spec, cfg: cfg, core: newCore(cfg), active: true, touched: m.now()}
	m.views[v.id] = v
	m.byToken[spec.ClientToken]++
	m.mu.Unlock()
	if old != nil {
		old.close("replaced")
	}

	v.attachOverlays()
	if err := v.rebuild(cfg, true); err != nil {
		m.closeView(v, "", false)
		return ViewOpenResult{}, err
	}
	st, err := v.State()
	if err != nil {
		return ViewOpenResult{}, err
	}
	res := ViewOpenResult{
		ViewID: v.id, Columns: cfg.colKeys, SchemaVersion: def.SchemaVersion(), State: st, Filterable: st.Filterable,
	}
	if dynamic {
		res.Schema = def.Schema()
	}
	return res, nil
}

// Update applies a patch and returns the new state.
func (m *ViewManager) Update(id string, patch ViewPatch) (ViewState, error) {
	v, err := m.get(id)
	if err != nil {
		return ViewState{}, err
	}
	// opMu is held from reading the base spec through the rebuild, so concurrent patches apply
	// one after the other against the latest spec.
	v.opMu.Lock()
	defer v.opMu.Unlock()
	v.mu.Lock()
	if v.closed {
		v.mu.Unlock()
		return ViewState{}, ErrViewNotFound
	}
	next := v.spec
	oldCfg := v.cfg
	v.touched = m.now()
	v.mu.Unlock()

	if patch.Namespaces != nil {
		next.Namespaces = *patch.Namespaces
	}
	if patch.Search != nil {
		next.Search = *patch.Search
	}
	if patch.ColumnFilters != nil {
		next.ColumnFilters = *patch.ColumnFilters
	}
	if patch.Sort != nil {
		next.Sort = *patch.Sort
	}
	if patch.Columns != nil {
		next.Columns = *patch.Columns
	}
	if patch.Locale != nil {
		next.Locale = *patch.Locale
	}
	if patch.Hide != nil {
		next.Hide = *patch.Hide
	}
	if patch.NotifyOn != nil {
		next.NotifyOn = *patch.NotifyOn
	}
	if reflect.DeepEqual(next, oldCfg.spec) {
		return v.State()
	}
	cfg, err := compileView(next, v.def)
	if err != nil {
		return ViewState{}, err
	}
	reconnect := v.def.Namespaced && !reflect.DeepEqual(NamespaceSelection(next.Namespaces), NamespaceSelection(oldCfg.spec.Namespaces))
	if reconnect {
		v.mu.Lock()
		v.broadDenied = false
		v.mu.Unlock()
	}
	if err := v.rebuildLocked(cfg, reconnect); err != nil {
		return ViewState{}, err
	}
	return v.State()
}

// CloseView closes a view at the client's request.
func (m *ViewManager) CloseView(id string) {
	if v, err := m.get(id); err == nil {
		m.closeView(v, "", false)
	}
}

// SetActive marks a view shown or hidden.
func (m *ViewManager) SetActive(id string, active bool) error {
	v, err := m.get(id)
	if err != nil {
		return err
	}
	v.SetActive(active)
	return nil
}

// Window serves a window of a view.
func (m *ViewManager) Window(id string, offset, limit int, have uint64) (ViewWindow, error) {
	v, err := m.get(id)
	if err != nil {
		return ViewWindow{}, err
	}
	return v.Window(offset, limit, have)
}

// Refs lists row references.
func (m *ViewManager) Refs(id string, offset, limit int, expectedVersion uint64) ([]RowRef, ViewState, error) {
	v, err := m.get(id)
	if err != nil {
		return nil, ViewState{}, err
	}
	return v.Refs(offset, limit, expectedVersion)
}

// Rows returns the unwindowed projection.
func (m *ViewManager) Rows(id string) ([]WindowRowRef, []string, ViewState, error) {
	v, err := m.get(id)
	if err != nil {
		return nil, nil, ViewState{}, err
	}
	return v.RowsSnapshot()
}

// Facet returns the select values of a column.
func (m *ViewManager) Facet(id, column string) (Facet, error) {
	v, err := m.get(id)
	if err != nil {
		return Facet{}, err
	}
	return v.Facet(column)
}

// SetSelection records the selection of a view.
func (m *ViewManager) SetSelection(id string, uids []string) (SelectionAck, error) {
	v, err := m.get(id)
	if err != nil {
		return SelectionAck{}, err
	}
	return v.SetSelection(uids)
}

// State returns a view's current state.
func (m *ViewManager) State(id string) (ViewState, error) {
	v, err := m.get(id)
	if err != nil {
		return ViewState{}, err
	}
	return v.State()
}

// --- planning --------------------------------------------------------------------------------

// planKeys decides which shards serve a view.
func (m *ViewManager) planKeys(def *Spec, spec ViewSpec, broadDenied bool) []store.Key {
	ctxName := ""
	if m.cfg.Context != nil {
		ctxName = m.cfg.Context()
	}
	whole := store.Key{Context: ctxName, Type: def.Type}
	if !def.Namespaced {
		return []store.Key{whole}
	}
	sel := NamespaceSelection(spec.Namespaces)
	if len(sel) == 0 {
		return nil
	}
	var available []string
	if len(sel) >= 2 {
		available = m.availableNamespaces(ctxName)
	}
	var obs *Observation
	reuse := false
	if m.cfg.Broad != nil {
		if o, ok := m.cfg.Broad(ctxName, def.Type); ok {
			obs, reuse = o, true
		}
	}
	scope := ChooseNamespaceScope(sel, available, obs, PlanOptions{BroadDenied: broadDenied, ReuseBroad: reuse, Now: m.now()})
	keys := make([]store.Key, 0, len(scope))
	for _, ns := range scope {
		keys = append(keys, store.Key{Context: ctxName, Type: def.Type, Namespace: ns})
	}
	return keys
}

// availableNamespaces reads the namespace names from the namespaces shard, waiting briefly for a
// cold shard. It returns nil when they cannot be had.
func (m *ViewManager) availableNamespaces(ctxName string) []string {
	lease, err := m.cfg.Provider.Acquire(store.Key{Context: ctxName, Type: "namespaces"},
		store.Hint{Mode: store.ModeMetadata, Linger: 10 * time.Second})
	if err != nil {
		return nil
	}
	defer lease.Release()
	deadline := time.Now().Add(namespaceWait)
	for !lease.State().Synced && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !lease.State().Synced {
		return nil
	}
	entries, _ := lease.Snapshot()
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name)
	}
	return names
}

// dynamicSpec builds the spec of a custom resource view. A resource the resolver cannot find
// still gets the base columns, so the view opens and the failing shard reports through
// ViewState.error (or the empty list is shown) instead of failing the open.
func (m *ViewManager) dynamicSpec(spec *ViewSpec) (*Spec, bool) {
	gvr, ok := store.ParseCRDType(spec.Type)
	if !ok {
		return nil, false
	}
	d := CRDDescriptor{Group: gvr.Group, Version: gvr.Version, Resource: gvr.Resource, Namespaced: true}
	if spec.CRD != nil {
		d.Namespaced = spec.CRD.Namespaced
	}
	var printer []PrinterColumn
	if m.cfg.CRD != nil {
		ctxName := ""
		if m.cfg.Context != nil {
			ctxName = m.cfg.Context()
		}
		if info, err := m.cfg.CRD.ResolveCRD(ctxName, d); err == nil {
			d.Namespaced = info.Namespaced
			printer = info.Columns
		}
	}
	return GenericCRDSpec(d, printer), true
}

// ViewStats describes one open view for the performance panel. The counters are cumulative since
// the view opened; the panel derives rates from two readings.
type ViewStats struct {
	ID         string  `json:"id"`
	Type       string  `json:"type"`
	Namespaces int     `json:"namespaces"`
	Phase      string  `json:"phase"`
	Version    uint64  `json:"version"`
	Total      int     `json:"total"`
	Filtered   int     `json:"filtered"`
	WindowRows int     `json:"windowRows"` // rows in the last window served
	Active     bool    `json:"active"`
	Windows    uint64  `json:"windows"`
	RowsServed uint64  `json:"rowsServed"`
	Notices    uint64  `json:"notices"`
	BytesSent  uint64  `json:"bytesSent"` // estimated JSON bytes of served windows
	IdleSecs   float64 `json:"idleSecs"`
}

// Stats returns the open views, sorted by type then id.
func (m *ViewManager) Stats() []ViewStats {
	now := m.now()
	views := m.snapshotViews()
	out := make([]ViewStats, 0, len(views))
	for _, v := range views {
		v.mu.Lock()
		st := ViewStats{
			ID: v.id, Type: v.def.Type, Namespaces: len(v.spec.Namespaces), Version: v.version,
			Total: len(v.core.rows), Filtered: len(v.core.index), WindowRows: v.statLastWindow, Active: v.active,
			Windows: v.statWindows, RowsServed: v.statRows, Notices: v.statNotices, BytesSent: v.statBytes,
			IdleSecs: now.Sub(v.touched).Seconds(),
		}
		// Reading the state here must not touch the view: stats polling would keep it alive.
		st.Phase = v.stateLocked(false).Phase
		v.mu.Unlock()
		out = append(out, st)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Type != out[j].Type {
			return out[i].Type < out[j].Type
		}
		return out[i].ID < out[j].ID
	})
	return out
}
