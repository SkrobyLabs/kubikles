package rows

import (
	"errors"
	"fmt"
	"math/rand"
	"reflect"
	"sort"
	"sync"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"kubikles/pkg/store"
)

func openCM(t *testing.T, env *testEnv, mutate ...func(*ViewSpec)) (string, ViewOpenResult) {
	t.Helper()
	spec := ViewSpec{ViewID: "v1", ClientToken: "tok", Type: "configmaps", Namespaces: []string{"*"}, Sort: SortSpec{Key: "name", Dir: "asc"}}
	for _, f := range mutate {
		f(&spec)
	}
	res, err := env.m.Open(spec)
	if err != nil {
		t.Fatal(err)
	}
	return res.ViewID, res
}

func windowIDs(t *testing.T, env *testEnv, id string, offset, limit int, have uint64) ViewWindow {
	t.Helper()
	w, err := env.m.Window(id, offset, limit, have)
	if err != nil {
		t.Fatal(err)
	}
	return w
}

var t0 = time.Date(2025, 5, 30, 0, 0, 0, 0, time.UTC)

func TestOpenPopulatesAndSorts(t *testing.T) {
	env := newEnv(t)
	sh := env.p.shard("configmaps")
	for i, n := range []string{"c", "a", "b"} {
		sh.upsert(cm("default", n, t0.Add(time.Duration(i)*time.Hour), map[string]string{"k": "v"}))
	}
	id, res := openCM(t, env)
	if !reflect.DeepEqual(res.Columns, []string{"name", "namespace", "keys", "age", "keyCount", "binaryKeys", "size"}) {
		t.Fatalf("columns %v", res.Columns)
	}
	if res.SchemaVersion == "" || res.State.Phase != PhaseComplete || res.State.Total != 3 || res.State.Filtered != 3 {
		t.Fatalf("state %+v", res.State)
	}
	w := windowIDs(t, env, id, 0, 10, 0)
	want := []string{"uid-default-a", "uid-default-b", "uid-default-c"}
	if !reflect.DeepEqual(w.Order, want) || len(w.Rows) != 3 {
		t.Fatalf("order %v rows %d", w.Order, len(w.Rows))
	}
	if got := w.Rows["uid-default-a"].C[0]; got != "a" {
		t.Fatalf("cell %v", got)
	}
	sh.upsert(cm("default", "aa", t0, nil))
	w = windowIDs(t, env, id, 0, 10, w.Version)
	if !reflect.DeepEqual(w.Order, []string{"uid-default-a", "uid-default-aa", "uid-default-b", "uid-default-c"}) {
		t.Fatalf("after insert %v", w.Order)
	}
}

func TestUnknownTypeColumnAndPreset(t *testing.T) {
	env := newEnv(t)
	if _, err := env.m.Open(ViewSpec{Type: "nope"}); err == nil {
		t.Fatal("unknown type must fail")
	}
	if _, err := env.m.Open(ViewSpec{Type: "configmaps", Columns: []string{"zzz"}}); err == nil {
		t.Fatal("unknown column must fail")
	}
	if _, err := env.m.Open(ViewSpec{Type: "configmaps", Hide: []string{"zzz"}}); err == nil {
		t.Fatal("unknown preset must fail")
	}
	if env.m.Count() != 0 {
		t.Fatal("failed opens must not leave views behind")
	}
}

// Incremental application must equal a from-scratch build, for random change sequences.
func TestIncrementalMatchesFullRebuild(t *testing.T) {
	variants := []struct {
		name string
		mut  func(*ViewSpec)
	}{
		{"name-asc", func(s *ViewSpec) {}},
		{"age-desc", func(s *ViewSpec) { s.Sort = SortSpec{Key: "age", Dir: "desc"} }},
		{"keycount-asc-search", func(s *ViewSpec) { s.Sort = SortSpec{Key: "keyCount", Dir: "asc"}; s.Search = "a" }},
		{"keys-desc-filter", func(s *ViewSpec) {
			s.Sort = SortSpec{Key: "keys", Dir: "desc"}
			s.ColumnFilters = []ColumnFilterSpec{{Col: "keyCount", Type: "numeric", Conditions: []FilterCondition{{Operator: ">=", Value: ptr(2)}}}}
		}},
		{"two-namespaces", func(s *ViewSpec) { s.Namespaces = []string{"ns1", "ns2"}; s.Sort = SortSpec{Key: "size", Dir: "asc"} }},
		{"hide-system", func(s *ViewSpec) { s.Hide = []string{"system"}; s.Sort = SortSpec{Key: "namespace", Dir: "desc"} }},
		{"unsorted-regex-filter", func(s *ViewSpec) {
			s.Sort = SortSpec{}
			s.ColumnFilters = []ColumnFilterSpec{{Col: "name", Type: "regex", Conditions: []FilterCondition{{Pattern: "^[a-m]"}}}}
		}},
	}
	for _, v := range variants {
		for seed := int64(1); seed <= 4; seed++ {
			t.Run(fmt.Sprintf("%s/seed%d", v.name, seed), func(t *testing.T) {
				env := newEnv(t)
				sh := env.p.shard("configmaps")
				rng := rand.New(rand.NewSource(seed))
				names := []string{"a", "b", "ab", "Zed", "kube-root-ca.crt", "m", "beta", "éa", "10", "9"}
				namespaces := []string{"ns1", "ns2", "ns3"}
				model := map[string]*store.Entry{}
				mk := func() *store.Entry {
					ns := namespaces[rng.Intn(len(namespaces))]
					name := names[rng.Intn(len(names))]
					data := map[string]string{}
					for i := rng.Intn(4); i > 0; i-- {
						data[fmt.Sprintf("k%d", rng.Intn(6))] = "v"
					}
					return cm(ns, name, t0.Add(time.Duration(rng.Intn(50))*time.Minute), data)
				}
				for i := 0; i < 60; i++ {
					e := mk()
					model[e.ID()] = e
					sh.upsert(e)
				}
				id, _ := openCM(t, env, v.mut)
				check := func(step int) {
					t.Helper()
					view, _ := env.m.get(id)
					view.mu.Lock()
					defer view.mu.Unlock()
					var all []*store.Entry
					for _, e := range model {
						all = append(all, e)
					}
					ref := buildCore(view.cfg, all, nil, env.clock.Now(), 1)
					got := make([]string, len(view.core.index))
					for i, r := range view.core.index {
						got[i] = r.id
					}
					want := make([]string, len(ref.index))
					for i, r := range ref.index {
						want[i] = r.id
					}
					if !reflect.DeepEqual(got, want) {
						t.Fatalf("step %d index diverged:\n got  %v\n want %v", step, got, want)
					}
					if len(view.core.rows) != len(ref.rows) {
						t.Fatalf("step %d scope %d, want %d", step, len(view.core.rows), len(ref.rows))
					}
					for i, f := range view.core.facets {
						if !reflect.DeepEqual(f.counts, ref.facets[i].counts) {
							t.Fatalf("step %d facet %s diverged: %v vs %v", step, f.col.Key, f.counts, ref.facets[i].counts)
						}
					}
					for _, r := range view.core.index {
						if !r.inIndex {
							t.Fatalf("step %d row %s in index but not flagged", step, r.id)
						}
					}
				}
				check(0)
				for step := 1; step <= 300; step++ {
					switch op := rng.Intn(10); {
					case op < 3:
						e := mk()
						model[e.ID()] = e
						sh.upsert(e)
					case op < 8 && len(model) > 0:
						keys := sortedKeys(model)
						old := model[keys[rng.Intn(len(keys))]]
						e := bump(old, func(c *corev1.ConfigMap) {
							c.Data = map[string]string{}
							for i := rng.Intn(4); i > 0; i-- {
								c.Data[fmt.Sprintf("k%d", rng.Intn(6))] = "v"
							}
							if rng.Intn(3) == 0 {
								c.CreationTimestamp.Time = t0.Add(time.Duration(rng.Intn(50)) * time.Minute)
							}
						})
						model[e.ID()] = e
						sh.upsert(e)
					case len(model) > 0:
						keys := sortedKeys(model)
						k := keys[rng.Intn(len(keys))]
						delete(model, k)
						sh.remove(k)
					}
					if step%20 == 0 {
						check(step)
					}
				}
				check(301)
			})
		}
	}
}

func ptr(f float64) *float64 { return &f }

func TestWindowDeltas(t *testing.T) {
	env := newEnv(t)
	sh := env.p.shard("configmaps")
	var entries []*store.Entry
	for i := 0; i < 20; i++ {
		e := cm("default", fmt.Sprintf("n%02d", i), t0, map[string]string{"k": "v"})
		entries = append(entries, e)
		sh.upsert(e)
	}
	id, _ := openCM(t, env)

	w1 := windowIDs(t, env, id, 0, 5, 0)
	if len(w1.Rows) != 5 || len(w1.Order) != 5 {
		t.Fatalf("first window rows=%d order=%d", len(w1.Rows), len(w1.Order))
	}
	// Nothing changed: order only.
	w2 := windowIDs(t, env, id, 0, 5, w1.Version)
	if len(w2.Rows) != 0 || len(w2.Order) != 5 || w2.Version != w1.Version {
		t.Fatalf("unchanged window should carry no rows: %d rows, version %d vs %d", len(w2.Rows), w2.Version, w1.Version)
	}
	// A heartbeat (new resourceVersion, no projected change) stays free.
	sh.upsert(bump(entries[0], nil))
	w3 := windowIDs(t, env, id, 0, 5, w2.Version)
	if len(w3.Rows) != 0 || w3.Version != w2.Version {
		t.Fatalf("heartbeat produced rows=%d version %d->%d", len(w3.Rows), w2.Version, w3.Version)
	}
	// A visible change in the window.
	sh.upsert(bump(entries[1], func(c *corev1.ConfigMap) { c.Data = map[string]string{"a": "1", "b": "2"} }))
	w4 := windowIDs(t, env, id, 0, 5, w3.Version)
	if len(w4.Rows) != 1 || w4.Rows["uid-default-n01"].C[4] != float64(2) || w4.Version <= w3.Version {
		t.Fatalf("modify: rows=%v version %d", w4.Rows, w4.Version)
	}
	// A change below the window moves no row of the window.
	sh.upsert(bump(entries[15], func(c *corev1.ConfigMap) { c.Data = map[string]string{"z": "1"} }))
	w5 := windowIDs(t, env, id, 0, 5, w4.Version)
	if len(w5.Rows) != 0 {
		t.Fatalf("change outside the window leaked rows: %v", w5.Rows)
	}
	// Deleting the first row pulls an unseen row into the window.
	sh.remove(entries[0].ID())
	w6 := windowIDs(t, env, id, 0, 5, w5.Version)
	if len(w6.Rows) != 1 || w6.Order[4] != "uid-default-n05" || w6.Rows["uid-default-n05"].Name != "n05" {
		t.Fatalf("shift-in: order %v rows %v", w6.Order, w6.Rows)
	}
	// A stale have (older than the projection) gets everything.
	if _, err := env.m.Update(id, ViewPatch{Sort: &SortSpec{Key: "name", Dir: "desc"}}); err != nil {
		t.Fatal(err)
	}
	w7 := windowIDs(t, env, id, 0, 5, w6.Version)
	if len(w7.Rows) != 5 {
		t.Fatalf("after a spec update the window must be full, got %d rows", len(w7.Rows))
	}
	// Scrolling to a different window sends rows the client never had even with a current have.
	w8 := windowIDs(t, env, id, 10, 5, w7.Version)
	if len(w8.Rows) != 5 {
		t.Fatalf("new range should carry its rows, got %d", len(w8.Rows))
	}
	// Window beyond the end.
	w9 := windowIDs(t, env, id, 100, 5, 0)
	if len(w9.Order) != 0 || w9.Filtered != 19 {
		t.Fatalf("beyond end: %v", w9)
	}
}

func TestForceFullWindowsAssertionMode(t *testing.T) {
	env := newEnv(t, func(c *Config) { c.ForceFullWindows = true })
	sh := env.p.shard("configmaps")
	sh.upsert(cm("default", "a", t0, nil))
	id, _ := openCM(t, env)
	w := windowIDs(t, env, id, 0, 5, 0)
	w2 := windowIDs(t, env, id, 0, 5, w.Version)
	if len(w2.Rows) != 1 {
		t.Fatalf("debug mode must send full windows, got %d", len(w2.Rows))
	}
}

func TestNoticesOnlyWhenWindowAffected(t *testing.T) {
	env := newEnv(t)
	sh := env.p.shard("configmaps")
	var entries []*store.Entry
	for i := 0; i < 30; i++ {
		e := cm("default", fmt.Sprintf("n%02d", i), t0, map[string]string{"k": "v"})
		entries = append(entries, e)
		sh.upsert(e)
	}
	id, _ := openCM(t, env)
	env.m.TakeNotices() // initial state
	windowIDs(t, env, id, 0, 5, 0)
	env.m.TakeNotices()

	// Heartbeat: nothing to tell.
	sh.upsert(bump(entries[2], nil))
	if n := env.m.TakeNotices(); len(n) != 0 {
		t.Fatalf("heartbeat notice: %+v", n)
	}
	// Modify below the window: counts unchanged, window untouched.
	sh.upsert(bump(entries[25], func(c *corev1.ConfigMap) { c.Data = map[string]string{"a": "b"} }))
	if n := env.m.TakeNotices(); len(n) != 0 {
		t.Fatalf("outside modification notice: %+v", n)
	}
	// Modify inside the window.
	sh.upsert(bump(entries[1], func(c *corev1.ConfigMap) { c.Data = map[string]string{"a": "b"} }))
	n := env.m.TakeNotices()
	if len(n) != 1 || !n[0].WindowChanged || n[0].V != id {
		t.Fatalf("inside modification: %+v", n)
	}
	// Insert below the window: counts moved, window did not.
	sh.upsert(cm("default", "zzz", t0, nil))
	n = env.m.TakeNotices()
	if len(n) != 1 || n[0].WindowChanged || n[0].Total != 31 || n[0].Filtered != 31 {
		t.Fatalf("insert below: %+v", n)
	}
	// Insert at the top shifts the window.
	sh.upsert(cm("default", "000", t0, nil))
	n = env.m.TakeNotices()
	if len(n) != 1 || !n[0].WindowChanged {
		t.Fatalf("insert at top: %+v", n)
	}
	// Inactive views keep their notice pending.
	if err := env.m.SetActive(id, false); err != nil {
		t.Fatal(err)
	}
	sh.upsert(cm("default", "001", t0, nil))
	if n := env.m.TakeNotices(); len(n) != 0 {
		t.Fatalf("inactive view notified: %+v", n)
	}
	_ = env.m.SetActive(id, true)
	if n := env.m.TakeNotices(); len(n) != 1 {
		t.Fatalf("reactivation must deliver the pending notice: %+v", n)
	}
}

// A change made while the view was inactive leaves the dirty flag set. Reactivating must signal the
// notifier again, otherwise no later change ever raises OnDirty (the flag only clears on delivery).
func TestReactivationSignalsNotifierWhenAlreadyDirty(t *testing.T) {
	env := newEnv(t)
	sh := env.p.shard("configmaps")
	t0 := time.Now()
	sh.upsert(cm("default", "a", t0, nil))
	id, _ := openCM(t, env)
	env.m.TakeNotices()
	_ = env.m.SetActive(id, false)
	sh.upsert(cm("default", "b", t0, nil))
	env.m.TakeNotices() // skipped: inactive, stays dirty
	env.mu.Lock()
	before := env.dirty
	env.mu.Unlock()
	_ = env.m.SetActive(id, true)
	env.mu.Lock()
	after := env.dirty
	env.mu.Unlock()
	if after <= before {
		t.Fatalf("reactivation of an already dirty view did not signal the notifier (%d -> %d)", before, after)
	}
}

func TestPhaseNoticesFollowShardState(t *testing.T) {
	env := newEnv(t)
	sh := env.p.shard("configmaps")
	sh.setState(func(s *store.State) { s.Phase = store.PhaseLoading; s.Synced = false; s.Loaded = 10; s.Expected = 100 })
	id, res := openCM(t, env)
	if res.State.Phase != PhaseLoading || res.State.Loaded != 10 || res.State.Expected != 100 {
		t.Fatalf("loading state %+v", res.State)
	}
	env.m.TakeNotices()
	sh.setState(func(s *store.State) { s.Loaded = 50 })
	env.m.Poll()
	n := env.m.TakeNotices()
	if len(n) != 1 || n[0].Loaded != 50 || n[0].Phase != PhaseLoading {
		t.Fatalf("progress notice %+v", n)
	}
	sh.setState(func(s *store.State) { s.Phase = store.PhaseWatching; s.Synced = true; s.Loaded = 100 })
	env.m.Poll()
	n = env.m.TakeNotices()
	if len(n) != 1 || n[0].Phase != PhaseComplete {
		t.Fatalf("complete notice %+v", n)
	}
	sh.setState(func(s *store.State) { s.Phase = store.PhaseIncomplete; s.Forbidden = true; s.Error = "forbidden" })
	env.m.Poll()
	st, _ := env.m.State(id)
	if st.Phase != PhaseIncomplete || !st.Forbidden || st.Error != "forbidden" {
		t.Fatalf("forbidden state %+v", st)
	}
}

func TestFacetCountsOverScopeNotFilter(t *testing.T) {
	env := newEnv(t)
	sh := env.p.shard("events")
	mkEv := func(name, typ string) *store.Entry {
		o := &corev1.Event{Type: typ}
		o.Namespace, o.Name, o.UID = "default", name, types.UID("uid-"+name)
		return &store.Entry{UID: "uid-" + name, Namespace: "default", Name: name, RV: "1", Obj: o}
	}
	sh.upsert(mkEv("a", "Normal"))
	sh.upsert(mkEv("b", "Normal"))
	sh.upsert(mkEv("c", "Warning"))
	sh.upsert(mkEv("d", "Odd"))
	res, err := env.m.Open(ViewSpec{ViewID: "e", ClientToken: "t", Type: "events", Namespaces: []string{"*"}, Search: "a"})
	if err != nil {
		t.Fatal(err)
	}
	if res.State.Total != 4 || res.State.Filtered != 1 {
		t.Fatalf("state %+v", res.State)
	}
	f, err := env.m.Facet("e", "type")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(f.Values, []string{"Normal", "Warning", "Odd"}) || f.Counts["Normal"] != 2 || f.Counts["Odd"] != 1 || f.Suppressed {
		t.Fatalf("facet %+v", f)
	}
	if !res.Filterable["type"] || res.Filterable["count"] {
		t.Fatalf("filterable %v", res.Filterable)
	}
	if _, err := env.m.Facet("e", "nope"); err == nil {
		t.Fatal("unknown facet column must fail")
	}
}

func TestFacetSuppressionAndRecovery(t *testing.T) {
	env := newEnv(t)
	sh := env.p.shard("configmaps")
	var entries []*store.Entry
	for i := 0; i < MaxSelectFilterValues+5; i++ {
		e := cm("default", fmt.Sprintf("n%03d", i), t0, map[string]string{fmt.Sprintf("key%03d", i): "v"})
		entries = append(entries, e)
		sh.upsert(e)
	}
	id, res := openCM(t, env)
	if res.Filterable["keys"] {
		t.Fatal("a column with more than 200 distinct values must not be filterable")
	}
	f, _ := env.m.Facet(id, "keys")
	if !f.Suppressed {
		t.Fatal("expected suppression")
	}
	for _, e := range entries[:50] {
		sh.remove(e.ID())
	}
	f, _ = env.m.Facet(id, "keys")
	if f.Suppressed || len(f.Values) != MaxSelectFilterValues+5-50 {
		t.Fatalf("facet should recover after removals: suppressed=%v values=%d", f.Suppressed, len(f.Values))
	}
}

func TestNamespaceScopeUpdateAndPlanning(t *testing.T) {
	env := newEnv(t)
	sh := env.p.shard("configmaps")
	for _, ns := range []string{"a", "b", "c", "d", "e"} {
		sh.upsert(cm(ns, "x", t0, nil))
	}
	id, res := openCM(t, env, func(s *ViewSpec) { s.Namespaces = []string{"b"} })
	if res.State.Total != 1 {
		t.Fatalf("total %d", res.State.Total)
	}
	if keys := env.p.acquired(); len(keys) != 1 || keys[0].Namespace != "b" {
		t.Fatalf("single namespace must be planned per namespace: %v", keys)
	}
	ns := []string{"a", "b", "c"}
	st, err := env.m.Update(id, ViewPatch{Namespaces: &ns})
	if err != nil {
		t.Fatal(err)
	}
	if st.Total != 3 {
		t.Fatalf("after widening: %+v", st)
	}
	all := []string{"*"}
	st, _ = env.m.Update(id, ViewPatch{Namespaces: &all})
	if st.Total != 5 {
		t.Fatalf("after all: %+v", st)
	}
	none := []string{}
	st, _ = env.m.Update(id, ViewPatch{Namespaces: &none})
	if st.Total != 0 || st.Phase != PhaseComplete {
		t.Fatalf("empty selection: %+v", st)
	}
	// Changes in newly selected namespaces flow after the switch.
	one := []string{"d"}
	_, _ = env.m.Update(id, ViewPatch{Namespaces: &one})
	sh.upsert(cm("d", "y", t0, nil))
	sh.upsert(cm("a", "z", t0, nil))
	st, _ = env.m.State(id)
	if st.Total != 2 {
		t.Fatalf("expected only namespace d rows, got %+v", st)
	}
	// Old leases are released after a switch.
	if n := env.p.activeLeases(); n != 1 {
		t.Fatalf("leaked leases: %d active", n)
	}
}

func TestBroadPlanWhenManySelectedAndKnownNamespaces(t *testing.T) {
	env := newEnv(t)
	nsShard := env.p.shard("namespaces")
	for _, n := range []string{"a", "b", "c", "d"} {
		nsShard.upsert(&store.Entry{UID: "ns-" + n, Name: n, RV: "1"})
	}
	_, _ = openCM(t, env, func(s *ViewSpec) { s.Namespaces = []string{"a", "b", "c", "d"} })
	var configmapKeys []store.Key
	for _, k := range env.p.acquired() {
		if k.Type == "configmaps" {
			configmapKeys = append(configmapKeys, k)
		}
	}
	if len(configmapKeys) != 1 || configmapKeys[0].Namespace != "" {
		t.Fatalf("4 of 4 namespaces should plan one cluster-wide shard, got %v", configmapKeys)
	}
}

func TestForbiddenBroadReplansPerNamespace(t *testing.T) {
	env := newEnv(t)
	nsShard := env.p.shard("namespaces")
	for _, n := range []string{"a", "b", "c", "d"} {
		nsShard.upsert(&store.Entry{UID: "ns-" + n, Name: n, RV: "1"})
	}
	sh := env.p.shard("configmaps")
	sh.upsert(cm("a", "x", t0, nil))
	id, _ := openCM(t, env, func(s *ViewSpec) { s.Namespaces = []string{"a", "b", "c", "d"} })
	sh.setState(func(s *store.State) { s.Forbidden = true; s.Phase = store.PhaseIncomplete })
	env.m.Poll()
	// The replan runs asynchronously.
	deadline := time.Now().Add(2 * time.Second)
	for {
		perNS := 0
		for _, k := range env.p.acquired() {
			if k.Type == "configmaps" && k.Namespace != "" {
				perNS++
			}
		}
		if perNS == 4 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected a per-namespace replan, acquired %v", env.p.acquired())
		}
		time.Sleep(5 * time.Millisecond)
	}
	_ = id
}

func TestSelectionStale(t *testing.T) {
	env := newEnv(t)
	sh := env.p.shard("configmaps")
	a, b := cm("default", "a", t0, nil), cm("default", "b", t0, nil)
	sh.upsert(a)
	sh.upsert(b)
	id, _ := openCM(t, env)
	ack, err := env.m.SetSelection(id, []string{a.ID(), b.ID(), "ghost"})
	if err != nil {
		t.Fatal(err)
	}
	if ack.Selected != 2 || !reflect.DeepEqual(ack.Stale, []string{"ghost"}) {
		t.Fatalf("ack %+v", ack)
	}
	env.m.TakeNotices()
	sh.remove(b.ID())
	n := env.m.TakeNotices()
	if len(n) != 1 || !reflect.DeepEqual(n[0].SelectionStale, []string{b.ID()}) {
		t.Fatalf("stale notice %+v", n)
	}
	// A search that hides the selected row makes it stale, like the legacy list pruning its
	// selection to the visible rows, so bulk actions never reach rows the user cannot see.
	s := "zzz"
	_, _ = env.m.Update(id, ViewPatch{Search: &s})
	n = env.m.TakeNotices()
	if len(n) != 1 || !reflect.DeepEqual(n[0].SelectionStale, []string{a.ID()}) {
		t.Fatalf("filtered-out rows are stale: %+v", n)
	}
	if ack, _ := env.m.SetSelection(id, []string{a.ID()}); ack.Selected != 0 || len(ack.Stale) != 1 {
		t.Fatalf("a hidden row cannot be selected: %+v", ack)
	}
	// Clearing the search and selecting again, then hiding the row through a watch update.
	empty := ""
	_, _ = env.m.Update(id, ViewPatch{Search: &empty})
	env.m.TakeNotices()
	if ack, _ := env.m.SetSelection(id, []string{a.ID()}); ack.Selected != 1 {
		t.Fatalf("ack %+v", ack)
	}
	// A namespace change that removes it from scope is stale as well.
	ns := []string{"other"}
	_, _ = env.m.Update(id, ViewPatch{Namespaces: &ns})
	n = env.m.TakeNotices()
	if len(n) != 1 || !reflect.DeepEqual(n[0].SelectionStale, []string{a.ID()}) {
		t.Fatalf("namespace change stale %+v", n)
	}
}

func TestLeaseExpiryAndClosedViews(t *testing.T) {
	env := newEnv(t)
	env.p.shard("configmaps").upsert(cm("default", "a", t0, nil))
	id, _ := openCM(t, env)
	env.clock.Advance(50 * time.Second)
	env.m.Reap()
	if _, err := env.m.Window(id, 0, 1, 0); err != nil { // renews the lease
		t.Fatal(err)
	}
	env.clock.Advance(50 * time.Second)
	env.m.Reap()
	if env.m.Count() != 1 {
		t.Fatal("a touched view must survive")
	}
	env.clock.Advance(61 * time.Second)
	env.m.Reap()
	if env.m.Count() != 0 {
		t.Fatal("an idle view must expire")
	}
	if _, err := env.m.Window(id, 0, 1, 0); err != ErrViewNotFound {
		t.Fatalf("expired view: %v", err)
	}
	if n := env.m.TakeNotices(); len(n) != 1 || n[0].Closed != "expired" {
		t.Fatalf("closed notice %+v", n)
	}
	if env.p.activeLeases() != 0 {
		t.Fatal("expired view leaked a lease")
	}
}

func TestResetClosesViews(t *testing.T) {
	env := newEnv(t)
	env.p.shard("configmaps").upsert(cm("default", "a", t0, nil))
	id, _ := openCM(t, env)
	env.m.Reset("context")
	if _, err := env.m.Window(id, 0, 1, 0); err != ErrViewNotFound {
		t.Fatalf("after reset: %v", err)
	}
	n := env.m.TakeNotices()
	if len(n) != 1 || n[0].V != id || n[0].Closed != "context" {
		t.Fatalf("reset notice %+v", n)
	}
	if env.p.activeLeases() != 0 {
		t.Fatal("reset leaked a lease")
	}
}

func TestCapsAndClientDisconnect(t *testing.T) {
	env := newEnv(t, func(c *Config) { c.MaxPerClient = 2; c.MaxTotal = 3 })
	open := func(id, token, client string) error {
		_, err := env.m.Open(ViewSpec{ViewID: id, ClientToken: token, ClientID: client, Type: "configmaps", Namespaces: []string{"*"}})
		return err
	}
	if open("a", "t1", "ws-1") != nil || open("b", "t1", "ws-1") != nil {
		t.Fatal("opens under the cap")
	}
	if err := open("c", "t1", "ws-1"); err != ErrTooManyViews {
		t.Fatalf("per-client cap: %v", err)
	}
	if err := open("a", "t1", "ws-1"); err != nil {
		t.Fatalf("re-opening an existing id replaces it: %v", err)
	}
	if open("c", "t2", "ws-2") != nil {
		t.Fatal("other client under the cap")
	}
	if err := open("d", "t3", "ws-3"); err != ErrTooManyViews {
		t.Fatalf("global cap: %v", err)
	}
	env.m.OnClientDisconnect("ws-1")
	if env.m.Count() != 1 {
		t.Fatalf("disconnect should close that client's views, left %d", env.m.Count())
	}
	if open("d", "t3", "ws-3") != nil {
		t.Fatal("capacity should be free again")
	}
}

func TestResyncOnResetChange(t *testing.T) {
	env := newEnv(t)
	sh := env.p.shard("configmaps")
	sh.upsert(cm("default", "a", t0, nil))
	id, _ := openCM(t, env)
	// Simulate a lost-changes reset after the shard changed behind the view's back.
	sh.mu.Lock()
	e := cm("default", "b", t0, nil)
	sh.seq++
	e.Seq = sh.seq
	sh.entries[e.ID()] = e
	sh.mu.Unlock()
	sh.reset()
	st, _ := env.m.State(id)
	if st.Total != 2 {
		t.Fatalf("reset must resync from the snapshot, total %d", st.Total)
	}
	// Changes at or below the snapshot sequence are ignored afterwards (no double apply).
	w := windowIDs(t, env, id, 0, 10, 0)
	if len(w.Order) != 2 {
		t.Fatalf("order %v", w.Order)
	}
}

func TestBulkBatchResyncs(t *testing.T) {
	env := newEnv(t)
	sh := env.p.shard("configmaps")
	id, _ := openCM(t, env)
	var ups []*store.Entry
	for i := 0; i < bulkThreshold+50; i++ {
		ups = append(ups, cm("default", fmt.Sprintf("n%05d", i), t0, nil))
	}
	sh.batch(ups, nil)
	st, _ := env.m.State(id)
	if st.Total != bulkThreshold+50 || st.Filtered != bulkThreshold+50 {
		t.Fatalf("bulk batch: %+v", st)
	}
	w := windowIDs(t, env, id, 0, 3, 0)
	if !reflect.DeepEqual(w.Order, []string{"uid-default-n00000", "uid-default-n00001", "uid-default-n00002"}) {
		t.Fatalf("order %v", w.Order)
	}
}

func TestAgeFilterReevaluatesOnTick(t *testing.T) {
	env := newEnv(t)
	sh := env.p.shard("configmaps")
	now := env.clock.Now()
	sh.upsert(cm("default", "young", now.Add(-30*time.Minute), nil))
	id, res := openCM(t, env, func(s *ViewSpec) {
		s.ColumnFilters = []ColumnFilterSpec{{Col: "age", Type: "numeric", Conditions: []FilterCondition{{Operator: ">", Value: ptr(1)}}}}
	})
	if res.State.Filtered != 0 {
		t.Fatalf("young row must be filtered out: %+v", res.State)
	}
	env.m.TakeNotices()
	env.clock.Advance(45 * time.Minute)
	env.m.TimeTick()
	st, _ := env.m.State(id)
	if st.Filtered != 1 {
		t.Fatalf("row aged into the filter: %+v", st)
	}
	if n := env.m.TakeNotices(); len(n) != 1 || n[0].Filtered != 1 {
		t.Fatalf("tick notice %+v", n)
	}
}

func TestPresetHidesFromScope(t *testing.T) {
	env := newEnv(t)
	sh := env.p.shard("configmaps")
	sh.upsert(cm("default", "kube-root-ca.crt", t0, nil))
	sh.upsert(cm("default", "app", t0, nil))
	_, res := openCM(t, env, func(s *ViewSpec) { s.Hide = []string{"system"} })
	if res.State.Total != 1 {
		t.Fatalf("preset must shrink the scope, not only the filtered list: %+v", res.State)
	}
}

func TestUpdateOnlyRecompilesWhenChanged(t *testing.T) {
	env := newEnv(t)
	env.p.shard("configmaps").upsert(cm("default", "a", t0, nil))
	id, _ := openCM(t, env)
	v, _ := env.m.get(id)
	before := v.cfg
	same := SortSpec{Key: "name", Dir: "asc"}
	if _, err := env.m.Update(id, ViewPatch{Sort: &same}); err != nil {
		t.Fatal(err)
	}
	if v.cfg != before {
		t.Fatal("an identical patch must not rebuild the view")
	}
	bad := "name:/"
	if _, err := env.m.Update(id, ViewPatch{Search: &bad}); err != nil {
		t.Fatal(err) // invalid regex is literal text, not an error
	}
	badCols := []string{"nope"}
	if _, err := env.m.Update(id, ViewPatch{Columns: &badCols}); err == nil {
		t.Fatal("unknown column in a patch must fail and leave the view intact")
	}
	if _, err := env.m.Window(id, 0, 1, 0); err != nil {
		t.Fatalf("view must survive a rejected patch: %v", err)
	}
}

func TestColumnSubsetProjection(t *testing.T) {
	env := newEnv(t)
	env.p.shard("configmaps").upsert(cm("default", "a", t0, map[string]string{"k": "v"}))
	id, res := openCM(t, env, func(s *ViewSpec) {
		s.Columns = []string{"size", "name"}
		s.Sort = SortSpec{Key: "age", Dir: "asc"} // sort column not projected
	})
	if !reflect.DeepEqual(res.Columns, []string{"size", "name"}) {
		t.Fatalf("columns %v", res.Columns)
	}
	w := windowIDs(t, env, id, 0, 1, 0)
	row := w.Rows["uid-default-a"]
	if len(row.C) != 2 || row.C[1] != "a" || row.C[0] != float64(9) { // {"k":"v"} is 9 chars, binaryData {} is 2: 11
		// size is data (9) + binaryData (2)
		if row.C[0] != float64(11) {
			t.Fatalf("cells %v", row.C)
		}
	}
}

func TestRefsAndRows(t *testing.T) {
	env := newEnv(t)
	sh := env.p.shard("configmaps")
	for _, n := range []string{"c", "a", "b"} {
		sh.upsert(cm("default", n, t0, nil))
	}
	id, _ := openCM(t, env)
	refs, st, err := env.m.Refs(id, 1, 5, 0)
	if err != nil || len(refs) != 2 || refs[0].Name != "b" || st.Filtered != 3 {
		t.Fatalf("refs %v %+v %v", refs, st, err)
	}
	all, _, _, err := env.m.Rows(id)
	if err != nil || len(all) != 3 || all[0].Name != "a" {
		t.Fatalf("rows %v %v", all, err)
	}

	// Range indexes are only valid at the version the client saw.
	if _, _, err := env.m.Refs(id, 0, 2, st.Version); err != nil {
		t.Fatalf("refs at the current version: %v", err)
	}
	sh.upsert(cm("default", "aa", t0, nil))
	if _, _, err := env.m.Refs(id, 0, 2, st.Version); !errors.Is(err, ErrViewChanged) {
		t.Fatalf("refs at a stale version = %v, want ErrViewChanged", err)
	}
	if _, _, err := env.m.Refs(id, 0, 0, 0); err != nil {
		t.Fatalf("refs without an expected version: %v", err)
	}
}

// Concurrent Open/Update/Close with shard changes must be race free (run with -race).
func TestConcurrentViewsAndChanges(t *testing.T) {
	env := newEnv(t)
	sh := env.p.shard("configmaps")
	sh.async = true
	for i := 0; i < 100; i++ {
		sh.upsert(cm("ns1", fmt.Sprintf("n%03d", i), t0, nil))
	}
	var writer, workers sync.WaitGroup
	stop := make(chan struct{})
	writer.Add(1)
	go func() {
		defer writer.Done()
		rng := rand.New(rand.NewSource(7))
		for {
			select {
			case <-stop:
				return
			default:
			}
			e := cm("ns1", fmt.Sprintf("n%03d", rng.Intn(150)), t0, map[string]string{"k": fmt.Sprint(rng.Intn(5))})
			if rng.Intn(4) == 0 {
				sh.remove(e.ID())
			} else {
				sh.upsert(e)
			}
		}
	}()
	for w := 0; w < 4; w++ {
		workers.Add(1)
		go func(w int) {
			defer workers.Done()
			rng := rand.New(rand.NewSource(int64(w)))
			for i := 0; i < 25; i++ {
				id := fmt.Sprintf("v-%d-%d", w, i%3)
				_, err := env.m.Open(ViewSpec{ViewID: id, ClientToken: fmt.Sprint("t", w), Type: "configmaps", Namespaces: []string{"*"}, Sort: SortSpec{Key: "keyCount", Dir: "asc"}})
				if err != nil {
					t.Error(err)
					return
				}
				for j := 0; j < 5; j++ {
					s := fmt.Sprintf("n%d", rng.Intn(10))
					_, _ = env.m.Update(id, ViewPatch{Search: &s})
					_, _ = env.m.Window(id, 0, 20, 0)
					_, _ = env.m.Facet(id, "keys")
					env.m.TakeNotices()
				}
				if i%2 == 0 {
					env.m.CloseView(id)
				}
			}
		}(w)
	}
	workers.Wait()
	close(stop)
	writer.Wait()
	env.m.Reset("test")
	if env.p.activeLeases() != 0 {
		t.Fatalf("leaked %d leases", env.p.activeLeases())
	}
}

func TestPlannerVectors(t *testing.T) {
	names := func(n int) []string {
		out := make([]string, n)
		for i := range out {
			out[i] = fmt.Sprintf("ns-%d", i)
		}
		return out
	}
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	obs := func(mut func(*Observation)) *Observation {
		o := &Observation{Count: 10, Bytes: 1000, DurationMs: 100, ObservedAt: now}
		if mut != nil {
			mut(o)
		}
		return o
	}
	broad := []string{""}
	cases := []struct {
		name      string
		selected  []string
		available []string
		obs       *Observation
		opts      PlanOptions
		want      []string
	}{
		{"star", []string{"*"}, nil, nil, PlanOptions{BroadDenied: true}, broad},
		{"empty", nil, nil, obs(nil), PlanOptions{}, nil},
		{"4 of 8", names(4), names(8), nil, PlanOptions{}, broad},
		{"3 of 4", names(3), names(4), nil, PlanOptions{}, names(3)},
		{"4 of 9", names(4), names(9), nil, PlanOptions{}, names(4)},
		{"sparse measured", names(2), names(1000), obs(nil), PlanOptions{}, broad},
		{"sparse unmeasured", names(2), names(1000), nil, PlanOptions{}, names(2)},
		{"reuse broad", names(1), names(1000), obs(nil), PlanOptions{ReuseBroad: true}, broad},
		{"no reuse", names(1), names(1000), obs(nil), PlanOptions{}, names(1)},
		{"6000 objects", names(8), names(10), obs(func(o *Observation) { o.Count = 6000 }), PlanOptions{}, names(8)},
		{"21 MiB", names(8), names(10), obs(func(o *Observation) { o.Bytes = 21 * 1024 * 1024 }), PlanOptions{}, names(8)},
		{"slow", names(8), names(10), obs(func(o *Observation) { o.DurationMs = 3000 }), PlanOptions{}, names(8)},
		{"coverage favorable", names(8), names(10), obs(func(o *Observation) { o.Count = 1000; o.DurationMs = 800 }), PlanOptions{ScopedDurationMs: ptr(500)}, broad},
		{"coverage unfavorable", names(8), names(10), obs(func(o *Observation) { o.Count = 1000; o.DurationMs = 800 }), PlanOptions{ScopedDurationMs: ptr(100)}, names(8)},
		{"coverage low", names(7), names(10), obs(func(o *Observation) { o.Count = 1000; o.DurationMs = 800 }), PlanOptions{}, names(7)},
		{"stale observation", names(2), names(1000), obs(func(o *Observation) { o.ObservedAt = now.Add(-time.Hour) }), PlanOptions{}, names(2)},
		{"broad denied", names(8), names(10), obs(nil), PlanOptions{BroadDenied: true}, names(8)},
	}
	for _, c := range cases {
		c.opts.Now = now
		if got := ChooseNamespaceScope(c.selected, c.available, c.obs, c.opts); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
	if got := NamespaceSelection([]string{"b", "a", "a"}); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("selection %v", got)
	}
	if got := NamespaceSelection([]string{"a", ""}); !reflect.DeepEqual(got, []string{"*"}) {
		t.Errorf("selection %v", got)
	}
	if NamespaceSelection([]string{}) != nil {
		t.Error("empty selection must stay empty")
	}
}

func TestRegistryValidation(t *testing.T) {
	good := func() *Spec {
		return &Spec{Type: "x-test", Needs: store.ModeFull, Columns: []Column{
			{Key: "name", Kind: KindString, CellType: "string", FilterType: FilterRegex, Cell: func(r *Row) interface{} { return nil }},
			{Key: "status", Kind: KindString, CellType: "string", FilterType: FilterSelect, Cell: func(r *Row) interface{} { return nil }},
		}, Search: []SearchField{nameField()}}
	}
	if err := Register(good()); err != nil {
		t.Fatal(err)
	}
	defer Unregister("x-test")
	if err := Register(good()); err == nil {
		t.Error("duplicate type must fail")
	}
	mutations := map[string]func(*Spec){
		"dup column":          func(s *Spec) { s.Columns = append(s.Columns, s.Columns[0]) },
		"no cell":             func(s *Spec) { s.Columns[0].Cell = nil },
		"bad kind":            func(s *Spec) { s.Columns[0].Kind = "x" },
		"bad filter":          func(s *Spec) { s.Columns[0].FilterType = "x" },
		"predefined regex":    func(s *Spec) { s.Columns[0].Predefined = []string{"a"} },
		"derived unknown":     func(s *Spec) { s.Derived = []Derived{{Columns: []string{"nope"}, From: "pods", Selector: noSelector}} },
		"derived no source":   func(s *Spec) { s.Derived = []Derived{{Columns: []string{"status"}, Selector: noSelector}} },
		"derived no selector": func(s *Spec) { s.Derived = []Derived{{Columns: []string{"status"}, From: "pods"}} },
		"derived other type": func(s *Spec) {
			s.Derived = []Derived{{Columns: []string{"status"}, From: "nodes", Selector: noSelector}}
		},
		"overlay unknown": func(s *Spec) { s.Overlays = []Overlay{{Name: "m", Columns: []string{"nope"}, Key: NodeKey}} },
		"overlay no key":  func(s *Spec) { s.Overlays = []Overlay{{Name: "m", Columns: []string{"status"}}} },
		"no name field":   func(s *Spec) { s.Search = []SearchField{labelsField()} },
		"dup search":      func(s *Spec) { s.Search = []SearchField{nameField(), nameField()} },
		"no cell type":    func(s *Spec) { s.Columns[0].CellType = "" },
		"bad preset":      func(s *Spec) { s.Presets = []Preset{{Name: "p"}} },
	}
	for name, mut := range mutations {
		s := good()
		s.Type = "x-bad"
		mut(s)
		if err := Register(s); err == nil {
			Unregister("x-bad")
			t.Errorf("%s: expected a validation error", name)
		}
	}
	ok := good()
	ok.Type = "x-ok"
	ok.Derived = []Derived{{Columns: []string{"status"}, From: "pods", Selector: noSelector}}
	ok.Overlays = []Overlay{{Name: "metrics", Columns: []string{"status"}, Key: NodeKey}}
	if err := Register(ok); err != nil {
		t.Errorf("valid derived/overlay: %v", err)
	}
	Unregister("x-ok")
	if _, found := Lookup("x-test"); !found {
		t.Error("lookup")
	}
	if !sort.StringsAreSorted(Types()) {
		t.Error("Types must be sorted")
	}
}

func noSelector(*Row) map[string]string { return nil }
