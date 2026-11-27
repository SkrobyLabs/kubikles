package rows

import (
	"sort"
	"time"

	"golang.org/x/text/collate"

	"kubikles/pkg/rows/query"
	"kubikles/pkg/store"
)

// viewRow is one object in a view's scope with everything the index needs precomputed, so
// comparisons and window projection never touch the object again.
type viewRow struct {
	id      string
	e       *store.Entry
	cells   []interface{}
	sortKey interface{}
	ts      string
	name    string
	ns      string
	del     bool
	// seq is the view version at which the projected row last changed.
	seq uint64
	// fvals are the select-facet values, aligned with compiled.facetCols.
	fvals   []string
	inIndex bool
}

// compiled is everything derived from a ViewSpec that the engine needs: column projection,
// compiled search and filters, namespace scope. It is immutable.
type compiled struct {
	spec      ViewSpec
	def       *Spec
	cols      []*Column
	colKeys   []string
	sortCol   *Column
	sortIdx   int // index of the sort column in cols, -1 when not projected
	search    func(*Row) bool
	filter    func(*Row) bool
	timeBound bool // the filter depends on the clock
	hide      []func(*Row) bool
	nsSet     map[string]struct{} // nil: every namespace
	facetCols []*Column
	ovPlans   []overlayPlan // one per Spec.Overlays entry, same order
	// dvPlan says which parts of the view the derived pod source touches; nil without one.
	dvPlan *overlayPlan
	// notifyPods says the view reports pod transitions (ViewSpec.NotifyOn "pods").
	notifyPods bool
}

// overlayPlan says which parts of a view an overlay source touches, so a tick re-derives only
// those.
// needsPods reports whether anything in the view reads the derived pod source.
func (p *overlayPlan) needsPods() bool {
	return p != nil && (len(p.cols) > 0 || p.sort || p.filter || len(p.facets) > 0)
}

type overlayPlan struct {
	name string
	key  func(namespace, name string) string
	// cols are the indexes (into compiled.cols) of the projected columns fed by the overlay.
	cols []int
	// sort is set when the view's sort column is fed by the overlay; filter when a column
	// filter reads an overlay column (then membership can change on a tick).
	sort   bool
	filter bool
	// facets are the indexes (into compiled.facetCols) of select columns fed by the overlay.
	facets []int
}

// effect describes what a batch did to the visible list, so the notifier can decide whether a
// client's window is affected.
type effect struct {
	counts   bool // total or filtered changed
	visible  bool // any projected row, order or count changed
	shiftPos int  // smallest index position of an insert or removal, -1 if none
	modPos   []int
	dropped  []string // ids removed from the scope
}

func newEffect() effect { return effect{shiftPos: -1} }

func (e *effect) shift(p int) {
	e.visible = true
	if e.shiftPos < 0 || p < e.shiftPos {
		e.shiftPos = p
	}
}

// facet counts the values of one select column over the whole scope.
type facet struct {
	col        *Column
	counts     map[string]int
	predefined map[string]bool
	over       bool // more than MaxSelectFilterValues distinct values; counts dropped
}

func newFacet(col *Column) *facet {
	f := &facet{col: col, counts: make(map[string]int, len(col.Predefined)), predefined: make(map[string]bool, len(col.Predefined))}
	for _, v := range col.Predefined {
		f.counts[v] = 0
		f.predefined[v] = true
	}
	return f
}

func (f *facet) add(v string) {
	if f.over || v == "" {
		return
	}
	f.counts[v]++
	if len(f.counts) > MaxSelectFilterValues {
		f.over = true
		f.counts = nil
	}
}

func (f *facet) remove(v string) {
	if f.over || v == "" {
		return
	}
	n := f.counts[v] - 1
	if n <= 0 && !f.predefined[v] {
		delete(f.counts, v)
		return
	}
	if n < 0 {
		n = 0
	}
	f.counts[v] = n
}

// core is the engine state of one view revision: the scope (every object visible to the view
// before search and column filters), the sorted index of those that pass, and the facets. It has
// no locking; the owning View serialises access.
type core struct {
	cfg    *compiled
	coll   *collate.Collator
	plan   sortPlan
	rows   map[string]*viewRow
	index  []*viewRow
	facets []*facet
	// ov is the current snapshot of every overlay source; rows read it through Row.OV.
	ov map[string]*OverlaySnapshot
	// ovIDs maps, per overlay (same order as cfg.ovPlans), the record key of an object to its
	// row id, so a tick that names changed keys finds the rows without scanning the scope.
	ovIDs []map[string]string
	// pods is the derived-column source of a workload view (nil otherwise).
	pods *podIndex
}

func newCore(cfg *compiled) *core {
	coll := newCollator(cfg.spec.Locale)
	c := &core{cfg: cfg, coll: coll, plan: newSortPlan(cfg.def, cfg.spec.Sort.Key, cfg.spec.Sort.Dir, coll), rows: make(map[string]*viewRow)}
	for _, col := range cfg.facetCols {
		c.facets = append(c.facets, newFacet(col))
	}
	if n := len(cfg.ovPlans); n > 0 {
		c.ov = make(map[string]*OverlaySnapshot, n)
		c.ovIDs = make([]map[string]string, n)
		for i := range c.ovIDs {
			c.ovIDs[i] = make(map[string]string)
		}
	}
	return c
}

// newRow wraps an entry for column, search and filter functions, with the overlay snapshots.
func (c *core) newRow(e *store.Entry, tcols map[string]int, now time.Time) Row {
	r := NewRow(e, tcols, now)
	r.OV = c.ov
	if c.pods != nil && len(c.cfg.def.Derived) > 0 {
		r.pods, r.podSel = c.pods, c.cfg.def.Derived[0].Selector
	}
	return r
}

func (c *core) trackOverlay(vr *viewRow) {
	for i := range c.cfg.ovPlans {
		c.ovIDs[i][c.cfg.ovPlans[i].key(vr.ns, vr.name)] = vr.id
	}
}

func (c *core) untrackOverlay(vr *viewRow) {
	for i := range c.cfg.ovPlans {
		k := c.cfg.ovPlans[i].key(vr.ns, vr.name)
		if c.ovIDs[i][k] == vr.id {
			delete(c.ovIDs[i], k)
		}
	}
}

// compileView resolves a spec against its type definition.
func compileView(spec ViewSpec, def *Spec) (*compiled, error) {
	cfg := &compiled{spec: spec, def: def, sortIdx: -1}
	keys := spec.Columns
	if len(keys) == 0 {
		for _, c := range def.Columns {
			keys = append(keys, c.Key)
		}
	}
	seen := map[string]bool{}
	for _, k := range keys {
		col := def.Column(k)
		if col == nil {
			return nil, &UnknownColumnError{Type: def.Type, Column: k}
		}
		if seen[k] {
			continue
		}
		seen[k] = true
		cfg.cols = append(cfg.cols, col)
		cfg.colKeys = append(cfg.colKeys, k)
	}
	if spec.Sort.Key != "" {
		cfg.sortCol = def.Column(spec.Sort.Key)
		for i, k := range cfg.colKeys {
			if k == spec.Sort.Key {
				cfg.sortIdx = i
			}
		}
	}
	for _, kind := range spec.NotifyOn {
		if kind != NotifyPods || def.Type != "pods" {
			return nil, &UnsupportedNotifyError{Type: def.Type, Kind: kind}
		}
		cfg.notifyPods = true
	}
	cfg.search = query.CompileString(spec.Search, def.Search, SearchName)
	filter, timeBound, err := compileColumnFilters(def, spec.ColumnFilters)
	if err != nil {
		return nil, err
	}
	cfg.filter, cfg.timeBound = filter, timeBound
	for _, name := range spec.Hide {
		p := def.preset(name)
		if p == nil {
			return nil, &UnknownPresetError{Type: def.Type, Preset: name}
		}
		cfg.hide = append(cfg.hide, p.Hide)
	}
	if def.Namespaced {
		sel := NamespaceSelection(spec.Namespaces)
		if len(sel) == 1 && sel[0] == "*" {
			cfg.nsSet = nil
		} else {
			cfg.nsSet = make(map[string]struct{}, len(sel))
			for _, ns := range sel {
				cfg.nsSet[ns] = struct{}{}
			}
		}
	}
	for i := range def.Columns {
		if def.Columns[i].FilterType == FilterSelect {
			cfg.facetCols = append(cfg.facetCols, &def.Columns[i])
		}
	}
	for _, ov := range def.Overlays {
		plan := overlayPlan{name: ov.Name, key: ov.Key}
		fed := make(map[string]bool, len(ov.Columns))
		for _, k := range ov.Columns {
			fed[k] = true
		}
		for i, k := range cfg.colKeys {
			if fed[k] {
				plan.cols = append(plan.cols, i)
			}
		}
		plan.sort = cfg.sortCol != nil && fed[cfg.sortCol.Key]
		for _, f := range spec.ColumnFilters {
			if fed[f.Col] && def.Column(f.Col) != nil {
				plan.filter = true
			}
		}
		for i, col := range cfg.facetCols {
			if fed[col.Key] {
				plan.facets = append(plan.facets, i)
			}
		}
		cfg.ovPlans = append(cfg.ovPlans, plan)
	}
	if len(def.Derived) > 0 {
		plan := overlayPlan{name: def.Derived[0].From}
		fed := make(map[string]bool)
		for _, d := range def.Derived {
			for _, k := range d.Columns {
				fed[k] = true
			}
		}
		for i, k := range cfg.colKeys {
			if fed[k] {
				plan.cols = append(plan.cols, i)
			}
		}
		plan.sort = cfg.sortCol != nil && fed[cfg.sortCol.Key]
		for _, f := range spec.ColumnFilters {
			if fed[f.Col] && def.Column(f.Col) != nil {
				plan.filter = true
			}
		}
		for i, col := range cfg.facetCols {
			if fed[col.Key] {
				plan.facets = append(plan.facets, i)
			}
		}
		cfg.dvPlan = &plan
	}
	return cfg, nil
}

// build creates the row for an entry, or nil when the entry is outside the view's scope.
func (c *core) build(e *store.Entry, tcols map[string]int, now time.Time) *viewRow {
	r := c.newRow(e, tcols, now)
	cfg := c.cfg
	if cfg.nsSet != nil {
		if _, ok := cfg.nsSet[r.Namespace()]; !ok {
			return nil
		}
	}
	for _, h := range cfg.hide {
		if h(&r) {
			return nil
		}
	}
	vr := &viewRow{id: e.ID(), e: e, name: r.Name(), ns: r.Namespace(), ts: r.CreationTimestamp(), del: r.Deleting()}
	vr.cells = make([]interface{}, len(cfg.cols))
	for i, col := range cfg.cols {
		vr.cells[i] = col.Cell(&r)
	}
	switch {
	case cfg.sortCol == nil:
	case cfg.sortIdx >= 0:
		vr.sortKey = vr.cells[cfg.sortIdx]
	default:
		vr.sortKey = cfg.sortCol.Cell(&r)
	}
	if len(cfg.facetCols) > 0 {
		vr.fvals = make([]string, len(cfg.facetCols))
		for i, col := range cfg.facetCols {
			vr.fvals[i] = col.FilterValue(&r)
		}
	}
	return vr
}

// passes evaluates search and column filters.
func (c *core) passes(vr *viewRow, tcols map[string]int, now time.Time) bool {
	r := c.newRow(vr.e, tcols, now)
	if !c.cfg.search(&r) {
		return false
	}
	return c.cfg.filter == nil || c.cfg.filter(&r)
}

func (c *core) find(vr *viewRow) int {
	i := sort.Search(len(c.index), func(i int) bool { return c.plan.compare(c.index[i], vr) >= 0 })
	if i < len(c.index) && c.index[i] == vr {
		return i
	}
	// The comparator is total, so a miss means the row is not indexed; a linear scan guards
	// against any inconsistency rather than corrupting the index.
	for j, x := range c.index {
		if x == vr {
			return j
		}
	}
	return -1
}

func (c *core) insertIndex(vr *viewRow) int {
	i := sort.Search(len(c.index), func(i int) bool { return c.plan.compare(c.index[i], vr) >= 0 })
	c.index = append(c.index, nil)
	copy(c.index[i+1:], c.index[i:])
	c.index[i] = vr
	vr.inIndex = true
	return i
}

func (c *core) removeIndex(vr *viewRow) int {
	i := c.find(vr)
	if i < 0 {
		return -1
	}
	copy(c.index[i:], c.index[i+1:])
	c.index[len(c.index)-1] = nil
	c.index = c.index[:len(c.index)-1]
	vr.inIndex = false
	return i
}

func (c *core) facetAdd(vr *viewRow) {
	for i, f := range c.facets {
		f.add(vr.fvals[i])
	}
}

func (c *core) facetRemove(vr *viewRow) {
	for i, f := range c.facets {
		f.remove(vr.fvals[i])
	}
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// sortKeyChanged reports whether the position-defining fields differ.
func sortKeyChanged(a, b *viewRow) bool {
	return !cellEqual(a.sortKey, b.sortKey) || a.ts != b.ts || a.name != b.name || a.ns != b.ns
}

func rowVisiblyEqual(a, b *viewRow) bool {
	return cellsEqual(a.cells, b.cells) && a.ns == b.ns && a.name == b.name && a.ts == b.ts && a.del == b.del
}

// upsert stores a new or modified entry. version returns the version to stamp on changed rows;
// it is called at most once per batch by the caller's closure.
func (c *core) upsert(e *store.Entry, tcols map[string]int, now time.Time, version func() uint64, eff *effect) {
	id := e.ID()
	old := c.rows[id]
	nr := c.build(e, tcols, now)
	if nr == nil {
		if old != nil {
			c.drop(old, eff)
		}
		return
	}
	if old == nil {
		nr.seq = version()
		c.rows[id] = nr
		c.trackOverlay(nr)
		c.facetAdd(nr)
		eff.counts = true
		eff.visible = true
		if c.passes(nr, tcols, now) {
			eff.shift(c.insertIndex(nr))
		}
		return
	}

	// Existing row: reconcile facets, then membership, then position.
	if !sameStrings(old.fvals, nr.fvals) {
		c.facetRemove(old)
		c.facetAdd(nr)
	}
	c.reconcile(old, nr, c.passes(nr, tcols, now), version, eff)
}

// reconcile replaces the scope row old by nr (same id), moving it in or out of the filtered
// list and to its new position as needed. nowIn says whether nr passes search and filters.
func (c *core) reconcile(old, nr *viewRow, nowIn bool, version func() uint64, eff *effect) {
	id := old.id
	wasIn := old.inIndex
	switch {
	case wasIn && !nowIn:
		nr.seq = version()
		p := c.removeIndex(old)
		c.rows[id] = nr
		eff.counts = true
		eff.shift(p)
	case !wasIn && nowIn:
		nr.seq = version()
		c.rows[id] = nr
		eff.counts = true
		eff.shift(c.insertIndex(nr))
	case wasIn && nowIn:
		if sortKeyChanged(old, nr) {
			p := c.removeIndex(old)
			nr.seq = version()
			c.rows[id] = nr
			eff.shift(p)
			eff.shift(c.insertIndex(nr))
		} else {
			p := c.find(old)
			if rowVisiblyEqual(old, nr) {
				nr.seq = old.seq
			} else {
				nr.seq = version()
				eff.visible = true
				eff.modPos = append(eff.modPos, p)
			}
			c.rows[id] = nr
			nr.inIndex = true
			if p >= 0 {
				c.index[p] = nr
			}
		}
	default:
		// Out of the filtered list before and after: the scope row is replaced silently.
		nr.seq = old.seq
		c.rows[id] = nr
	}
}

// applyOverlay folds a new overlay snapshot into the scope. Only rows named by the update (all
// rows when it says so) are visited, and for each only the overlay's columns are re-derived.
// Position is re-evaluated only when the sort column is fed by the overlay, membership only
// when a column filter reads it; everything else keeps its place and just gets new cells. When
// a sorted view has to move a large share of its rows the index is re-sorted once instead of
// moving rows one by one.
func (c *core) applyOverlay(i int, up OverlayUpdate, tcols map[string]int, now time.Time, version func() uint64, eff *effect) {
	plan := &c.cfg.ovPlans[i]
	if c.ov == nil {
		c.ov = make(map[string]*OverlaySnapshot, len(c.cfg.ovPlans))
	}
	c.ov[plan.name] = up.Snap
	if len(plan.cols) == 0 && len(plan.facets) == 0 && !plan.sort && !plan.filter {
		return
	}
	var targets []*viewRow
	if up.All {
		targets = make([]*viewRow, 0, len(c.rows))
		for _, vr := range c.rows {
			targets = append(targets, vr)
		}
	} else {
		targets = make([]*viewRow, 0, len(up.Changed))
		for _, k := range up.Changed {
			if vr := c.rows[c.ovIDs[i][k]]; vr != nil {
				targets = append(targets, vr)
			}
		}
	}
	c.refreshRows(plan, targets, tcols, now, version, eff)
}

// applyPods folds changes of the pod index into the workload rows. Only workloads whose selector
// matches a changed pod before or after the change are visited (all of them when the index was
// replaced), and for each only the derived columns are re-derived, like an overlay tick.
func (c *core) applyPods(deltas []podDelta, all bool, tcols map[string]int, now time.Time, version func() uint64, eff *effect) {
	plan := c.cfg.dvPlan
	if plan == nil || c.pods == nil {
		return
	}
	if len(plan.cols) == 0 && len(plan.facets) == 0 && !plan.sort && !plan.filter {
		return
	}
	sel := c.cfg.def.Derived[0].Selector
	var targets []*viewRow
	if all {
		targets = make([]*viewRow, 0, len(c.rows))
		for _, vr := range c.rows {
			targets = append(targets, vr)
		}
	} else if len(deltas) > 0 {
		byNS := make(map[string][]podDelta)
		for _, d := range deltas {
			byNS[d.ns] = append(byNS[d.ns], d)
		}
		for _, vr := range c.rows {
			ds := byNS[vr.ns]
			if len(ds) == 0 {
				continue
			}
			r := NewRow(vr.e, tcols, now)
			ms := sel(&r)
			if len(ms) == 0 {
				continue
			}
			for _, d := range ds {
				if (d.old != nil && selects(ms, d.old.labels)) || (d.new != nil && selects(ms, d.new.labels)) {
					targets = append(targets, vr)
					break
				}
			}
		}
	}
	c.refreshRows(plan, targets, tcols, now, version, eff)
}

// refreshRows re-derives the columns of a plan for the target rows and folds the replacements
// into the index (see applyOverlay for the cost rules).
func (c *core) refreshRows(plan *overlayPlan, targets []*viewRow, tcols map[string]int, now time.Time, version func() uint64, eff *effect) {
	var pairs []rowPair
	for _, old := range targets {
		r := c.newRow(old.e, tcols, now)
		nr := *old
		nr.inIndex = false // reconcile (or the bulk path) decides membership of the replacement
		nr.cells = append([]interface{}(nil), old.cells...)
		for _, ci := range plan.cols {
			nr.cells[ci] = c.cfg.cols[ci].Cell(&r)
		}
		if plan.sort {
			if c.cfg.sortIdx >= 0 {
				nr.sortKey = nr.cells[c.cfg.sortIdx]
			} else {
				nr.sortKey = c.cfg.sortCol.Cell(&r)
			}
		}
		if len(plan.facets) > 0 {
			nr.fvals = append([]string(nil), old.fvals...)
			for _, fi := range plan.facets {
				nr.fvals[fi] = c.cfg.facetCols[fi].FilterValue(&r)
			}
		}
		if cellsEqual(old.cells, nr.cells) && cellEqual(old.sortKey, nr.sortKey) && sameStrings(old.fvals, nr.fvals) && !plan.filter {
			continue
		}
		pairs = append(pairs, rowPair{old, &nr})
	}
	if len(pairs) == 0 {
		return
	}
	if !plan.sort && !plan.filter {
		c.replaceInPlace(pairs2(pairs), version, eff)
		return
	}
	bulk := plan.sort && len(pairs) > 64 && len(pairs)*16 > len(c.index)
	if bulk {
		ver := version()
		for _, p := range pairs {
			if !sameStrings(p.old.fvals, p.nr.fvals) {
				c.facetRemove(p.old)
				c.facetAdd(p.nr)
			}
			nowIn := p.old.inIndex
			if plan.filter {
				nowIn = c.passes(p.nr, tcols, now)
			}
			p.nr.inIndex = nowIn
			if !rowVisiblyEqual(p.old, p.nr) || p.old.inIndex != nowIn || !cellEqual(p.old.sortKey, p.nr.sortKey) {
				p.nr.seq = ver
			}
			c.rows[p.old.id] = p.nr
		}
		index := c.index[:0:0]
		for _, vr := range c.rows {
			if vr.inIndex {
				index = append(index, vr)
			}
		}
		sort.Slice(index, func(a, b int) bool { return c.plan.compare(index[a], index[b]) < 0 })
		if len(index) != len(c.index) {
			eff.counts = true
		}
		c.index = index
		eff.shift(0)
		return
	}
	for _, p := range pairs {
		if !sameStrings(p.old.fvals, p.nr.fvals) {
			c.facetRemove(p.old)
			c.facetAdd(p.nr)
		}
		nowIn := p.old.inIndex
		if plan.filter {
			nowIn = c.passes(p.nr, tcols, now)
		}
		c.reconcile(p.old, p.nr, nowIn, version, eff)
	}
}

type rowPair struct{ old, nr *viewRow }

func pairs2(p []rowPair) []rowPair { return p }

// replaceInPlace swaps rows whose position and membership cannot change (the sort and the
// filters do not read what changed). A handful of rows are located by binary search; for many,
// one pass over the index finds them all.
func (c *core) replaceInPlace(pairs []rowPair, version func() uint64, eff *effect) {
	for _, p := range pairs {
		if !sameStrings(p.old.fvals, p.nr.fvals) {
			c.facetRemove(p.old)
			c.facetAdd(p.nr)
		}
	}
	if len(pairs) <= 16 {
		for _, p := range pairs {
			c.reconcile(p.old, p.nr, p.old.inIndex, version, eff)
		}
		return
	}
	byOld := make(map[*viewRow]*viewRow, len(pairs))
	for _, p := range pairs {
		byOld[p.old] = p.nr
	}
	for i, vr := range c.index {
		nr, ok := byOld[vr]
		if !ok {
			continue
		}
		delete(byOld, vr)
		if rowVisiblyEqual(vr, nr) {
			nr.seq = vr.seq
		} else {
			nr.seq = version()
			eff.visible = true
			eff.modPos = append(eff.modPos, i)
		}
		nr.inIndex = true
		c.index[i] = nr
		c.rows[nr.id] = nr
	}
	// What is left was not in the filtered list: replaced silently.
	for old, nr := range byOld {
		nr.seq = old.seq
		c.rows[nr.id] = nr
	}
}

// drop removes a row from the scope and the index.
func (c *core) drop(old *viewRow, eff *effect) {
	delete(c.rows, old.id)
	c.untrackOverlay(old)
	c.facetRemove(old)
	eff.dropped = append(eff.dropped, old.id)
	eff.counts = true
	eff.visible = true
	if old.inIndex {
		eff.shift(c.removeIndex(old))
	}
}

func (c *core) remove(e *store.Entry, eff *effect) {
	if old := c.rows[e.ID()]; old != nil {
		c.drop(old, eff)
	}
}

// buildCore constructs a core over a set of entries: rows, facets, then one sort.
func buildCore(cfg *compiled, entries []*store.Entry, tcols map[string]int, now time.Time, version uint64) *core {
	return buildCoreOv(cfg, entries, tcols, now, version, nil)
}

// buildCoreOv is buildCore with the current overlay snapshots (copied; the core owns its map).
func buildCoreOv(cfg *compiled, entries []*store.Entry, tcols map[string]int, now time.Time, version uint64, ov map[string]*OverlaySnapshot) *core {
	return buildCoreDv(cfg, entries, tcols, now, version, ov, nil)
}

// buildCoreDv is buildCoreOv with the pod index of a workload view's derived columns.
func buildCoreDv(cfg *compiled, entries []*store.Entry, tcols map[string]int, now time.Time, version uint64, ov map[string]*OverlaySnapshot, pods *podIndex) *core {
	c := newCore(cfg)
	c.pods = pods
	if c.ov != nil {
		for k, v := range ov {
			c.ov[k] = v
		}
	}
	for _, e := range entries {
		vr := c.build(e, tcols, now)
		if vr == nil {
			continue
		}
		vr.seq = version
		c.rows[vr.id] = vr
		c.trackOverlay(vr)
		c.facetAdd(vr)
		if c.passes(vr, tcols, now) {
			c.index = append(c.index, vr)
			vr.inIndex = true
		}
	}
	sort.Slice(c.index, func(i, j int) bool { return c.plan.compare(c.index[i], c.index[j]) < 0 })
	return c
}

// reevaluate re-runs search and filters over the scope, for time-dependent filters. It returns
// whether the filtered list changed.
func (c *core) reevaluate(tcols map[string]int, now time.Time, version func() uint64, eff *effect) {
	for _, vr := range c.rows {
		in := c.passes(vr, tcols, now)
		switch {
		case in && !vr.inIndex:
			vr.seq = version()
			eff.counts = true
			eff.shift(c.insertIndex(vr))
		case !in && vr.inIndex:
			eff.counts = true
			eff.shift(c.removeIndex(vr))
		}
	}
}
