package rows

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"sort"
	"testing"

	"kubikles/pkg/store"
)

// goldenOverlay is the overlay half of a golden file: per metrics state the cells, filter values,
// numeric values, sort orders and column-filter matches the legacy code produced.
type goldenOverlay struct {
	Name    string   `json:"name"`
	Columns []string `json:"columns"`
	States  []struct {
		Name              string                            `json:"name"`
		Available         *bool                             `json:"available"`
		Raw               json.RawMessage                   `json:"raw"`
		Records           map[string]string                 `json:"records"`
		Cells             map[string]map[string]interface{} `json:"cells"`
		FilterValues      map[string]map[string]string      `json:"filterValues"`
		Numeric           map[string]map[string]interface{} `json:"numeric"`
		SortOrders        map[string]map[string][]string    `json:"sortOrders"`
		ColumnFilterCases []struct {
			Name    string                      `json:"name"`
			Filters map[string]ColumnFilterSpec `json:"filters"`
			Matches []string                    `json:"matches"`
		} `json:"columnFilterCases"`
	} `json:"states"`
}

type rawPodMetric struct {
	Namespace       string `json:"namespace"`
	Name            string `json:"name"`
	CPUUsage        int64  `json:"cpuUsage"`
	MemoryUsage     int64  `json:"memoryUsage"`
	CPURequested    int64  `json:"cpuRequested"`
	MemRequested    int64  `json:"memRequested"`
	CPUCommitted    int64  `json:"cpuCommitted"`
	MemCommitted    int64  `json:"memCommitted"`
	NodeCPUCapacity int64  `json:"nodeCpuCapacity"`
	NodeMemCapacity int64  `json:"nodeMemCapacity"`
}

type rawNodeMetric struct {
	Name         string `json:"name"`
	CPUUsage     int64  `json:"cpuUsage"`
	MemoryUsage  int64  `json:"memoryUsage"`
	CPUCapacity  int64  `json:"cpuCapacity"`
	MemCapacity  int64  `json:"memCapacity"`
	CPURequested int64  `json:"cpuRequested"`
	MemRequested int64  `json:"memRequested"`
	CPUCommitted int64  `json:"cpuCommitted"`
	MemCommitted int64  `json:"memCommitted"`
	PodCount     int64  `json:"podCount"`
	PodCapacity  int64  `json:"podCapacity"`
}

func (m rawPodMetric) input() PodMetricInput {
	return PodMetricInput{Namespace: m.Namespace, Name: m.Name, CPUUsage: m.CPUUsage, MemoryUsage: m.MemoryUsage,
		CPURequested: m.CPURequested, MemRequested: m.MemRequested, CPUCommitted: m.CPUCommitted, MemCommitted: m.MemCommitted,
		NodeCPUCapacity: m.NodeCPUCapacity, NodeMemCapacity: m.NodeMemCapacity}
}

func (m rawNodeMetric) input() NodeMetricInput {
	return NodeMetricInput{Name: m.Name, CPUUsage: m.CPUUsage, MemoryUsage: m.MemoryUsage, CPUCapacity: m.CPUCapacity,
		MemCapacity: m.MemCapacity, CPURequested: m.CPURequested, MemRequested: m.MemRequested, CPUCommitted: m.CPUCommitted,
		MemCommitted: m.MemCommitted, PodCount: m.PodCount, PodCapacity: m.PodCapacity}
}

// goldenRecords runs the Go transform over a state's raw backend result.
func goldenRecords(t testing.TB, typ string, raw json.RawMessage) map[string]OverlayRecord {
	t.Helper()
	if len(raw) == 0 || string(raw) == "null" {
		return map[string]OverlayRecord{}
	}
	switch typ {
	case "pods":
		var r struct {
			Metrics []rawPodMetric `json:"metrics"`
		}
		must(t, json.Unmarshal(raw, &r))
		in := make([]PodMetricInput, len(r.Metrics))
		for i, m := range r.Metrics {
			in[i] = m.input()
		}
		return PodRecords(in)
	case "nodes":
		var r struct {
			Metrics []rawNodeMetric `json:"metrics"`
		}
		must(t, json.Unmarshal(raw, &r))
		in := make([]NodeMetricInput, len(r.Metrics))
		for i, m := range r.Metrics {
			in[i] = m.input()
		}
		return NodeRecords(in)
	case "namespaces":
		var r struct {
			Pods struct {
				Metrics []rawPodMetric `json:"metrics"`
			} `json:"pods"`
			Nodes struct {
				Metrics []rawNodeMetric `json:"metrics"`
			} `json:"nodes"`
		}
		must(t, json.Unmarshal(raw, &r))
		pods := make([]PodMetricInput, len(r.Pods.Metrics))
		for i, m := range r.Pods.Metrics {
			pods[i] = m.input()
		}
		nodes := make([]NodeMetricInput, len(r.Nodes.Metrics))
		for i, m := range r.Nodes.Metrics {
			nodes[i] = m.input()
		}
		return NamespaceRecords(pods, nodes)
	}
	t.Fatalf("no overlay transform for %s", typ)
	return nil
}

func overlayGoldens(t testing.TB) map[string]*golden {
	out := map[string]*golden{}
	for _, typ := range Types() {
		g := loadGolden(t, typ)
		if g.Overlay != nil {
			out[typ] = g
		}
	}
	if len(out) == 0 {
		t.Fatal("no golden file has an overlay section")
	}
	return out
}

func snapshotFor(avail *bool, records map[string]OverlayRecord) *OverlaySnapshot {
	return &OverlaySnapshot{Seq: 1, Available: avail, Records: records}
}

func overlayName(typ string) string {
	switch typ {
	case "pods":
		return OverlayPodMetrics
	case "nodes":
		return OverlayNodeMetrics
	}
	return OverlayNamespaceMetrics
}

// The Go transform of a raw metrics result reproduces the legacy transform (rounding included).
func TestGoldenOverlayRecords(t *testing.T) {
	for typ, g := range overlayGoldens(t) {
		for _, st := range g.Overlay.States {
			got := goldenRecords(t, typ, st.Raw)
			text := map[string]string{}
			for k, rec := range got {
				text[k] = recordText(rec)
			}
			if !reflect.DeepEqual(text, nonNilStrings(st.Records)) {
				t.Errorf("%s/%s records differ:\n got  %v\n want %v", typ, st.Name, text, st.Records)
			}
		}
	}
}

func nonNilStrings(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}

func TestGoldenOverlayCells(t *testing.T) {
	for typ, g := range overlayGoldens(t) {
		spec, _ := Lookup(typ)
		for _, v := range variantsFor(t, g) {
			for _, st := range g.Overlay.States {
				ov := map[string]*OverlaySnapshot{overlayName(typ): snapshotFor(st.Available, goldenRecords(t, typ, st.Raw))}
				for _, e := range v.entries {
					r := NewRow(e, v.tcols, goldenNow(g))
					r.OV = ov
					for _, key := range g.Overlay.Columns {
						col := spec.Column(key)
						if got, want := col.Cell(&r), st.Cells[e.UID][key]; jsonOf(got) != jsonOf(want) {
							t.Errorf("%s/%s %s cell %s = %s, want %s", typ, st.Name, e.UID, key, jsonOf(got), jsonOf(want))
						}
						if got, want := col.FilterValue(&r), st.FilterValues[e.UID][key]; got != want {
							t.Errorf("%s/%s %s filter value %s = %q, want %q", typ, st.Name, e.UID, key, got, want)
						}
						wantNum := st.Numeric[e.UID][key]
						gotNum, ok := col.NumericValue(&r)
						switch {
						case wantNum == nil && ok:
							t.Errorf("%s/%s %s numeric %s = %v, want NaN", typ, st.Name, e.UID, key, gotNum)
						case wantNum != nil && !ok:
							t.Errorf("%s/%s %s numeric %s = NaN, want %v", typ, st.Name, e.UID, key, wantNum)
						case wantNum != nil && math.Abs(gotNum-wantNum.(float64)) > 1e-9:
							t.Errorf("%s/%s %s numeric %s = %v, want %v", typ, st.Name, e.UID, key, gotNum, wantNum)
						}
					}
				}
			}
		}
	}
}

func TestGoldenOverlaySortOrders(t *testing.T) {
	for typ, g := range overlayGoldens(t) {
		spec, _ := Lookup(typ)
		excluded := map[string]bool{}
		for _, u := range g.NoSort {
			excluded[u] = true
		}
		v := variantsFor(t, g)[0]
		var entries []*store.Entry
		for _, e := range v.entries {
			if !excluded[e.UID] {
				entries = append(entries, e)
			}
		}
		for _, st := range g.Overlay.States {
			ov := map[string]*OverlaySnapshot{overlayName(typ): snapshotFor(st.Available, goldenRecords(t, typ, st.Raw))}
			for key, dirs := range st.SortOrders {
				for dir, want := range dirs {
					cfg := testCfg(t, spec, ViewSpec{Namespaces: []string{"*"}, Sort: SortSpec{Key: key, Dir: dir}})
					c := buildCoreOv(cfg, entries, v.tcols, goldenNow(g), 1, ov)
					got := make([]string, len(c.index))
					for i, r := range c.index {
						got[i] = r.id
					}
					if !reflect.DeepEqual(got, want) {
						t.Errorf("%s/%s sort %s %s:\n got  %v\n want %v", typ, st.Name, key, dir, got, want)
					}
				}
			}
		}
	}
}

func TestGoldenOverlayColumnFilters(t *testing.T) {
	for typ, g := range overlayGoldens(t) {
		spec, _ := Lookup(typ)
		v := variantsFor(t, g)[0]
		for _, st := range g.Overlay.States {
			ov := map[string]*OverlaySnapshot{overlayName(typ): snapshotFor(st.Available, goldenRecords(t, typ, st.Raw))}
			for _, c := range st.ColumnFilterCases {
				var filters []ColumnFilterSpec
				keys := make([]string, 0, len(c.Filters))
				for k := range c.Filters {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				for _, k := range keys {
					f := c.Filters[k]
					f.Col = k
					filters = append(filters, f)
				}
				pred, _, err := compileColumnFilters(spec, filters)
				must(t, err)
				var got []string
				for _, e := range v.entries {
					r := NewRow(e, v.tcols, goldenNow(g))
					r.OV = ov
					if pred == nil || pred(&r) {
						got = append(got, e.UID)
					}
				}
				if !reflect.DeepEqual(got, c.Matches) && !(len(got) == 0 && len(c.Matches) == 0) {
					t.Errorf("%s/%s column filter %q:\n got  %v\n want %v", typ, st.Name, c.Name, got, c.Matches)
				}
			}
		}
	}
}

// coreOrder lists the filtered list of a core as ids.
func coreOrder(c *core) []string {
	out := make([]string, len(c.index))
	for i, r := range c.index {
		out[i] = r.id
	}
	return out
}

// sameCores reports the first difference between an incrementally updated core and a rebuilt one.
func sameCores(inc, full *core) string {
	if got, want := coreOrder(inc), coreOrder(full); !reflect.DeepEqual(got, want) {
		return fmt.Sprintf("order differs:\n inc  %v\n full %v", got, want)
	}
	if len(inc.rows) != len(full.rows) {
		return fmt.Sprintf("scope size %d, want %d", len(inc.rows), len(full.rows))
	}
	for id, f := range full.rows {
		i := inc.rows[id]
		if i == nil {
			return "missing row " + id
		}
		if !cellsEqual(i.cells, f.cells) {
			return fmt.Sprintf("row %s cells %v, want %v", id, i.cells, f.cells)
		}
		if !cellEqual(i.sortKey, f.sortKey) {
			return fmt.Sprintf("row %s sort key %v, want %v", id, i.sortKey, f.sortKey)
		}
		if i.inIndex != f.inIndex {
			return fmt.Sprintf("row %s membership %v, want %v", id, i.inIndex, f.inIndex)
		}
		if !sameStrings(i.fvals, f.fvals) {
			return fmt.Sprintf("row %s facet values %v, want %v", id, i.fvals, f.fvals)
		}
	}
	for i, f := range inc.facets {
		if !reflect.DeepEqual(f.counts, full.facets[i].counts) {
			return fmt.Sprintf("facet %s counts %v, want %v", f.col.Key, f.counts, full.facets[i].counts)
		}
	}
	return ""
}

// Applying one snapshot after another incrementally gives the same view as building it from
// scratch, for every ordered pair of recorded states, every sort the overlay feeds and with and
// without a filter on an overlay column.
func TestGoldenOverlayIncrementalEqualsRebuild(t *testing.T) {
	for typ, g := range overlayGoldens(t) {
		spec, _ := Lookup(typ)
		v := variantsFor(t, g)[0]
		name := overlayName(typ)
		// Rows without a creation timestamp make the comparator inconsistent (the same exclusion
		// as the sort goldens), so an incremental history could order them differently.
		excluded := map[string]bool{}
		for _, u := range g.NoSort {
			excluded[u] = true
		}
		var entries []*store.Entry
		for _, e := range v.entries {
			if !excluded[e.UID] {
				entries = append(entries, e)
			}
		}
		numeric := func(col string) []ColumnFilterSpec {
			return []ColumnFilterSpec{{Col: col, Type: "numeric", Conditions: []FilterCondition{{Operator: ">=", Value: ptr(5.0)}}}}
		}
		type scenario struct {
			sortKey string
			filters []ColumnFilterSpec
		}
		scenarios := []scenario{{"name", nil}, {"age", nil}}
		for _, col := range g.Overlay.Columns {
			scenarios = append(scenarios, scenario{col, nil})
			if col == "cpu" || col == "memory" {
				scenarios = append(scenarios, scenario{col, numeric(col)}, scenario{"name", numeric(col)})
			}
		}
		for _, sc := range scenarios {
			for _, dir := range []string{"asc", "desc"} {
				for ai, a := range g.Overlay.States {
					for bi, b := range g.Overlay.States {
						snapA := snapshotFor(a.Available, goldenRecords(t, typ, a.Raw))
						snapB := snapshotFor(b.Available, goldenRecords(t, typ, b.Raw))
						cfg := testCfg(t, spec, ViewSpec{Namespaces: []string{"*"}, Sort: SortSpec{Key: sc.sortKey, Dir: dir}, ColumnFilters: sc.filters})
						inc := buildCoreOv(cfg, entries, v.tcols, goldenNow(g), 1, map[string]*OverlaySnapshot{name: snapA})
						up, _ := diffSnapshots(snapA, snapB)
						ver := uint64(1)
						eff := newEffect()
						inc.applyOverlay(0, up, v.tcols, goldenNow(g), func() uint64 { ver++; return ver }, &eff)
						full := buildCoreOv(cfg, entries, v.tcols, goldenNow(g), 1, map[string]*OverlaySnapshot{name: snapB})
						if d := sameCores(inc, full); d != "" {
							t.Errorf("%s sort %s %s filters %v: %s -> %s: %s", typ, sc.sortKey, dir, len(sc.filters), g.Overlay.States[ai].Name, g.Overlay.States[bi].Name, d)
						}
					}
				}
			}
		}
	}
}
