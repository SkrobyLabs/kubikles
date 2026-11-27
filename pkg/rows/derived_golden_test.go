package rows

import (
	"encoding/json"
	"math"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"

	"kubikles/pkg/store"
)

// goldenDerived is the derived half of a golden file: per pod list the cells, numeric values and
// sort orders of the columns that read the pods, as the legacy code produced them.
type goldenDerived struct {
	From    string   `json:"from"`
	Columns []string `json:"columns"`
	States  []struct {
		Name       string                            `json:"name"`
		Pods       []json.RawMessage                 `json:"pods"`
		Cells      map[string]map[string]interface{} `json:"cells"`
		Numeric    map[string]map[string]interface{} `json:"numeric"`
		SortOrders map[string]map[string][]string    `json:"sortOrders"`
	} `json:"states"`
}

func derivedGoldens(t testing.TB) map[string]*golden {
	out := map[string]*golden{}
	for _, typ := range Types() {
		g := loadGolden(t, typ)
		if g.Derived != nil {
			out[typ] = g
		}
	}
	if len(out) == 0 {
		t.Fatal("no golden file has a derived section")
	}
	return out
}

// podEntries decodes the raw pods of a golden state.
func podEntries(t testing.TB, raw []json.RawMessage) []*store.Entry {
	t.Helper()
	out := make([]*store.Entry, 0, len(raw))
	for _, r := range raw {
		p := &corev1.Pod{}
		must(t, json.Unmarshal(r, p))
		out = append(out, entryOf(p, p))
	}
	return out
}

func indexOf(t testing.TB, raw []json.RawMessage) *podIndex {
	x := newPodIndex()
	x.reset(podEntries(t, raw))
	return x
}

func rowWithPods(spec *Spec, e *store.Entry, tcols map[string]int, g *golden, idx *podIndex) Row {
	r := NewRow(e, tcols, goldenNow(g))
	r.pods, r.podSel = idx, spec.Derived[0].Selector
	return r
}

func TestGoldenDerivedCells(t *testing.T) {
	for typ, g := range derivedGoldens(t) {
		spec, _ := Lookup(typ)
		v := variantsFor(t, g)[0]
		for _, st := range g.Derived.States {
			idx := indexOf(t, st.Pods)
			for _, e := range v.entries {
				r := rowWithPods(spec, e, v.tcols, g, idx)
				for _, key := range g.Derived.Columns {
					col := spec.Column(key)
					if got, want := col.Cell(&r), st.Cells[e.UID][key]; jsonOf(got) != jsonOf(want) {
						t.Errorf("%s/%s %s cell %s = %s, want %s", typ, st.Name, e.UID, key, jsonOf(got), jsonOf(want))
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

func TestGoldenDerivedSortOrders(t *testing.T) {
	for typ, g := range derivedGoldens(t) {
		spec, _ := Lookup(typ)
		v := variantsFor(t, g)[0]
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
		for _, st := range g.Derived.States {
			idx := indexOf(t, st.Pods)
			for key, dirs := range st.SortOrders {
				for dir, want := range dirs {
					cfg := testCfg(t, spec, ViewSpec{Namespaces: []string{"*"}, Sort: SortSpec{Key: key, Dir: dir}})
					c := buildCoreDv(cfg, entries, v.tcols, goldenNow(g), 1, nil, idx)
					if got := coreOrder(c); !reflect.DeepEqual(got, want) {
						t.Errorf("%s/%s sort %s %s:\n got  %v\n want %v", typ, st.Name, key, dir, got, want)
					}
				}
			}
		}
	}
}

// changesBetween describes the move from one pod list to another as store changes.
func changesBetween(a, b []*store.Entry) []store.Change {
	byID := map[string]*store.Entry{}
	for _, e := range a {
		byID[e.ID()] = e
	}
	var out []store.Change
	seen := map[string]bool{}
	for _, e := range b {
		seen[e.ID()] = true
		if old := byID[e.ID()]; old != nil {
			out = append(out, store.Change{Type: store.ChangeModified, Old: old, New: e})
		} else {
			out = append(out, store.Change{Type: store.ChangeAdded, New: e})
		}
	}
	for _, e := range a {
		if !seen[e.ID()] {
			out = append(out, store.Change{Type: store.ChangeDeleted, Old: e})
		}
	}
	return out
}

// Applying one pod list after another incrementally gives the same view as building it from
// scratch, for every ordered pair of recorded lists and the sorts the derived columns feed.
func TestGoldenDerivedIncrementalEqualsRebuild(t *testing.T) {
	for typ, g := range derivedGoldens(t) {
		spec, _ := Lookup(typ)
		v := variantsFor(t, g)[0]
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
		for _, sortKey := range []string{"name", "age", "pods"} {
			for _, dir := range []string{"asc", "desc"} {
				for _, a := range g.Derived.States {
					for _, b := range g.Derived.States {
						cfg := testCfg(t, spec, ViewSpec{Namespaces: []string{"*"}, Sort: SortSpec{Key: sortKey, Dir: dir}})
						idx := indexOf(t, a.Pods)
						inc := buildCoreDv(cfg, entries, v.tcols, goldenNow(g), 1, nil, idx)
						deltas := idx.apply(changesBetween(podEntries(t, a.Pods), podEntries(t, b.Pods)))
						ver := uint64(1)
						eff := newEffect()
						inc.applyPods(deltas, false, v.tcols, goldenNow(g), func() uint64 { ver++; return ver }, &eff)
						full := buildCoreDv(cfg, entries, v.tcols, goldenNow(g), 1, nil, indexOf(t, b.Pods))
						if d := sameCores(inc, full); d != "" {
							t.Errorf("%s sort %s %s: %s -> %s: %s", typ, sortKey, dir, a.Name, b.Name, d)
						}
					}
				}
			}
		}
	}
}
