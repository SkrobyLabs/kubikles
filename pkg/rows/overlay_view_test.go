package rows

import (
	"fmt"
	"math/rand"
	"reflect"
	"sync"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"kubikles/pkg/store"
)

func podEntry(ns, name string, created time.Time, node string, phase corev1.PodPhase) *store.Entry {
	obj := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name, UID: types.UID("uid-" + ns + "-" + name), ResourceVersion: "1", CreationTimestamp: metav1.NewTime(created)},
		Spec:       corev1.PodSpec{NodeName: node},
		Status:     corev1.PodStatus{Phase: phase},
	}
	return &store.Entry{UID: string(obj.UID), Namespace: ns, Name: name, RV: "1", Obj: obj}
}

// podRecord is a pod metrics record with the given committed and usage percentages; the other
// fields follow so that distinct records differ.
func podRecord(committed, usage float64) OverlayRecord {
	rec := make(OverlayRecord, podRecordLen)
	rec[podCPUPercent], rec[podMemPercent] = usage, usage/2
	rec[podCPUCommittedPercent], rec[podMemCommittedPercent] = committed, committed/2
	rec[podCPUUsage] = usage * 10
	return rec
}

type overlayEnv struct {
	*testEnv
	hub *OverlayHub
	sh  *fakeShard
	mu  sync.Mutex
	// demand records the demand callbacks, in order.
	demand []string
}

func newOverlayEnv(t *testing.T) *overlayEnv {
	oe := &overlayEnv{}
	oe.hub = NewOverlayHub(func(ctx, name string, wanted bool) {
		oe.mu.Lock()
		oe.demand = append(oe.demand, fmt.Sprintf("%s/%s=%v", ctx, name, wanted))
		oe.mu.Unlock()
	})
	oe.testEnv = newEnv(t, func(c *Config) { c.Overlays = oe.hub })
	oe.sh = oe.p.shard("pods")
	return oe
}

func (oe *overlayEnv) demandLog() []string {
	oe.mu.Lock()
	defer oe.mu.Unlock()
	return append([]string(nil), oe.demand...)
}

func (oe *overlayEnv) openPods(t *testing.T, mutate ...func(*ViewSpec)) string {
	t.Helper()
	spec := ViewSpec{ViewID: "pv", ClientToken: "tok", Type: "pods", Namespaces: []string{"*"}, Sort: SortSpec{Key: "name", Dir: "asc"}}
	for _, f := range mutate {
		f(&spec)
	}
	res, err := oe.m.Open(spec)
	if err != nil {
		t.Fatal(err)
	}
	return res.ViewID
}

func TestOverlayTickUpdatesCellsAndOrder(t *testing.T) {
	oe := newOverlayEnv(t)
	for i, n := range []string{"a", "b", "c"} {
		oe.sh.upsert(podEntry("ns", n, t0.Add(time.Duration(i)*time.Minute), "node", corev1.PodRunning))
	}
	id := oe.openPods(t, func(s *ViewSpec) { s.Sort = SortSpec{Key: "cpu", Dir: "desc"} })
	w := windowIDs(t, oe.testEnv, id, 0, 10, 0)
	// No metrics yet: every sort value is -1, so the age tie-break puts the newest first.
	if want := []string{"uid-ns-c", "uid-ns-b", "uid-ns-a"}; !reflect.DeepEqual(w.Order, want) {
		t.Fatalf("before the first tick: %v", w.Order)
	}
	if got := w.State.Overlays[OverlayPodMetrics]; got.Available != nil {
		t.Fatalf("availability before the first answer must be unknown, got %v", *got.Available)
	}
	oe.hub.Publish("ctx", OverlayPodMetrics, BoolPtr(true), map[string]OverlayRecord{
		"ns/a": podRecord(90, 80), "ns/b": podRecord(10, 5),
	})
	w2 := windowIDs(t, oe.testEnv, id, 0, 10, w.Version)
	if want := []string{"uid-ns-a", "uid-ns-b", "uid-ns-c"}; !reflect.DeepEqual(w2.Order, want) {
		t.Fatalf("after the first tick: %v", w2.Order)
	}
	if w2.Version <= w.Version {
		t.Fatalf("version did not advance: %d -> %d", w.Version, w2.Version)
	}
	if got := w2.State.Overlays[OverlayPodMetrics]; got.Available == nil || !*got.Available {
		t.Fatalf("availability not carried: %+v", got)
	}
	// The delta window carries only rows whose cells changed: c has no record.
	if _, ok := w2.Rows["uid-ns-c"]; ok {
		t.Fatal("a row without a record must not be re-sent")
	}
	cols := windowCols(t, oe.testEnv, id)
	if got := w2.Rows["uid-ns-a"].C[cols["cpu"]]; got != float64(90) {
		t.Fatalf("cpu cell %v", got)
	}
	if got := w2.Rows["uid-ns-a"].C[cols["metrics"]]; got != "80,40,90,45,800,0,0,0,0,0,0,0,0,0" && got != recordText(podRecord(90, 80)) {
		t.Fatalf("metrics cell %v", got)
	}
	// Only availability moves: no row changes, but the state and a notice follow.
	oe.m.TakeNotices()
	before := oe.dirtyCount()
	oe.hub.Publish("ctx", OverlayPodMetrics, BoolPtr(false), map[string]OverlayRecord{"ns/a": podRecord(90, 80), "ns/b": podRecord(10, 5)})
	w3 := windowIDs(t, oe.testEnv, id, 0, 10, w2.Version)
	if len(w3.Rows) != 0 {
		t.Fatalf("an availability change alone must not resend rows, got %d", len(w3.Rows))
	}
	if got := w3.State.Overlays[OverlayPodMetrics]; got.Available == nil || *got.Available {
		t.Fatalf("availability false not carried: %+v", got)
	}
	if oe.dirtyCount() == before {
		t.Fatal("an availability change must mark the view dirty")
	}
	notices := oe.m.TakeNotices()
	if len(notices) != 1 || notices[0].Overlays[OverlayPodMetrics].Available == nil || *notices[0].Overlays[OverlayPodMetrics].Available {
		t.Fatalf("notice %+v", notices)
	}
}

func windowCols(t *testing.T, env *testEnv, id string) map[string]int {
	t.Helper()
	v, err := env.m.get(id)
	if err != nil {
		t.Fatal(err)
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	out := map[string]int{}
	for i, k := range v.cfg.colKeys {
		out[k] = i
	}
	return out
}

// A tick that does not touch the sort or filter columns must not reposition anything.
func TestOverlayTickKeepsOrderWhenSortIsNotVolatile(t *testing.T) {
	oe := newOverlayEnv(t)
	for i := 0; i < 20; i++ {
		oe.sh.upsert(podEntry("ns", fmt.Sprintf("p%02d", i), t0.Add(time.Duration(i)*time.Minute), "node", corev1.PodRunning))
	}
	id := oe.openPods(t)
	w := windowIDs(t, oe.testEnv, id, 0, 50, 0)
	v, _ := oe.m.get(id)
	oe.hub.Publish("ctx", OverlayPodMetrics, BoolPtr(true), map[string]OverlayRecord{"ns/p05": podRecord(50, 50), "ns/p06": podRecord(60, 60)})
	w2 := windowIDs(t, oe.testEnv, id, 0, 50, w.Version)
	if !reflect.DeepEqual(w.Order, w2.Order) {
		t.Fatalf("order changed:\n%v\n%v", w.Order, w2.Order)
	}
	if len(w2.Rows) != 2 {
		t.Fatalf("expected the two changed rows, got %d", len(w2.Rows))
	}
	v.mu.Lock()
	n := len(v.core.index)
	v.mu.Unlock()
	if n != 20 {
		t.Fatalf("index size %d", n)
	}
}

func TestOverlayDemandFollowsViews(t *testing.T) {
	oe := newOverlayEnv(t)
	oe.sh.upsert(podEntry("ns", "a", t0, "node", corev1.PodRunning))
	id := oe.openPods(t)
	if got := oe.demandLog(); !reflect.DeepEqual(got, []string{"ctx/podmetrics=true"}) {
		t.Fatalf("demand after open: %v", got)
	}
	// A second view of the same type does not change the demand.
	spec := ViewSpec{ViewID: "pv2", ClientToken: "tok", Type: "pods", Namespaces: []string{"*"}}
	if _, err := oe.m.Open(spec); err != nil {
		t.Fatal(err)
	}
	if got := len(oe.demandLog()); got != 1 {
		t.Fatalf("demand callbacks %d", got)
	}
	// Hiding one view keeps the demand; hiding both drops it; showing one raises it again.
	_ = oe.m.SetActive(id, false)
	if got := len(oe.demandLog()); got != 1 {
		t.Fatalf("one view still active, demand log %v", oe.demandLog())
	}
	_ = oe.m.SetActive("pv2", false)
	if got := oe.demandLog(); len(got) != 2 || got[1] != "ctx/podmetrics=false" {
		t.Fatalf("demand log %v", got)
	}
	_ = oe.m.SetActive(id, true)
	if got := oe.demandLog(); len(got) != 3 || got[2] != "ctx/podmetrics=true" {
		t.Fatalf("demand log %v", got)
	}
	oe.m.CloseView(id)
	oe.m.CloseView("pv2")
	if got := oe.demandLog(); len(got) != 4 || got[3] != "ctx/podmetrics=false" {
		t.Fatalf("demand log after close %v", got)
	}
	if oe.hub.Demanded("ctx", OverlayPodMetrics) {
		t.Fatal("no view is open, nothing must be demanded")
	}
}

// A view opened after the source published starts from the current snapshot.
func TestOverlayLateViewSeesCurrentSnapshot(t *testing.T) {
	oe := newOverlayEnv(t)
	oe.sh.upsert(podEntry("ns", "a", t0, "node", corev1.PodRunning))
	oe.hub.Publish("ctx", OverlayPodMetrics, BoolPtr(true), map[string]OverlayRecord{"ns/a": podRecord(33, 22)})
	id := oe.openPods(t)
	w := windowIDs(t, oe.testEnv, id, 0, 10, 0)
	cols := windowCols(t, oe.testEnv, id)
	if got := w.Rows["uid-ns-a"].C[cols["cpu"]]; got != float64(33) {
		t.Fatalf("cpu cell %v", got)
	}
}

// Random object churn interleaved with overlay ticks of every size: the incrementally maintained
// view always equals a view built from scratch, for sorts and filters that do and do not read the
// overlay, and the delta windows reproduce the full window.
func TestOverlayIncrementalMatchesFullRebuild(t *testing.T) {
	variants := []struct {
		name string
		mut  func(*ViewSpec)
	}{
		{"name", func(s *ViewSpec) {}},
		{"cpu-desc", func(s *ViewSpec) { s.Sort = SortSpec{Key: "cpu", Dir: "desc"} }},
		{"memory-asc", func(s *ViewSpec) { s.Sort = SortSpec{Key: "memory", Dir: "asc"} }},
		{"cpu-filter", func(s *ViewSpec) {
			s.Sort = SortSpec{Key: "cpu", Dir: "asc"}
			s.ColumnFilters = []ColumnFilterSpec{{Col: "cpu", Type: "numeric", Conditions: []FilterCondition{{Operator: ">=", Value: ptr(20)}}}}
		}},
		{"name-with-filter", func(s *ViewSpec) {
			s.ColumnFilters = []ColumnFilterSpec{{Col: "memory", Type: "numeric", Conditions: []FilterCondition{{Operator: "<", Value: ptr(30)}}}}
		}},
		{"age-search", func(s *ViewSpec) { s.Sort = SortSpec{Key: "age", Dir: "desc"}; s.Search = "p1" }},
		{"status-two-namespaces", func(s *ViewSpec) { s.Namespaces = []string{"ns1", "ns2"}; s.Sort = SortSpec{Key: "status", Dir: "asc"} }},
	}
	for _, v := range variants {
		for seed := int64(1); seed <= 3; seed++ {
			t.Run(fmt.Sprintf("%s/seed%d", v.name, seed), func(t *testing.T) {
				oe := newOverlayEnv(t)
				rng := rand.New(rand.NewSource(seed))
				namespaces := []string{"ns1", "ns2", "ns3"}
				model := map[string]*store.Entry{}
				mk := func() *store.Entry {
					ns := namespaces[rng.Intn(len(namespaces))]
					phase := []corev1.PodPhase{corev1.PodRunning, corev1.PodPending, corev1.PodFailed}[rng.Intn(3)]
					return podEntry(ns, fmt.Sprintf("p%03d", rng.Intn(400)), t0.Add(time.Duration(rng.Intn(500))*time.Minute), "node", phase)
				}
				for i := 0; i < 300; i++ {
					e := mk()
					model[e.ID()] = e
					oe.sh.upsert(e)
				}
				records := func(share float64) map[string]OverlayRecord {
					out := map[string]OverlayRecord{}
					for _, e := range model {
						if rng.Float64() < share {
							out[PodKey(e.Namespace, e.Name)] = podRecord(float64(rng.Intn(60)), float64(rng.Intn(60)))
						}
					}
					return out
				}
				id := oe.openPods(t, v.mut)
				var client map[string]WindowRow
				var have uint64
				sync := func() {
					w := windowIDs(t, oe.testEnv, id, 0, 5000, have)
					if client == nil || have == 0 {
						client = map[string]WindowRow{}
					}
					for k, r := range w.Rows {
						client[k] = r
					}
					full := windowIDs(t, oe.testEnv, id, 0, 5000, 0)
					for _, k := range full.Order {
						if !reflect.DeepEqual(client[k].C, full.Rows[k].C) {
							t.Fatalf("delta window diverged for %s: %v vs %v", k, client[k].C, full.Rows[k].C)
						}
					}
					have = windowIDs(t, oe.testEnv, id, 0, 5000, 0).Version
				}
				check := func(step int) {
					t.Helper()
					view, _ := oe.m.get(id)
					view.mu.Lock()
					var all []*store.Entry
					for _, e := range model {
						all = append(all, e)
					}
					ref := buildCoreOv(view.cfg, all, nil, oe.clock.Now(), 1, view.ov)
					d := sameCores(view.core, ref)
					view.mu.Unlock()
					if d != "" {
						t.Fatalf("step %d: %s", step, d)
					}
					sync()
				}
				check(0)
				for step := 1; step <= 120; step++ {
					switch op := rng.Intn(10); {
					case op < 2:
						e := mk()
						model[e.ID()] = e
						oe.sh.upsert(e)
					case op < 3 && len(model) > 0:
						keys := sortedKeys(model)
						k := keys[rng.Intn(len(keys))]
						delete(model, k)
						oe.sh.remove(k)
					case op < 4:
						oe.hub.Publish("ctx", OverlayPodMetrics, BoolPtr(rng.Intn(5) != 0), records(1)) // everything changes: the bulk path
					default:
						oe.hub.Publish("ctx", OverlayPodMetrics, BoolPtr(true), records(0.15))
					}
					if step%4 == 0 {
						check(step)
					}
				}
				check(121)
			})
		}
	}
}

// Window and unrelated state must stay readable while ticks arrive from another goroutine.
func TestOverlayConcurrentTicksAndReads(t *testing.T) {
	oe := newOverlayEnv(t)
	for i := 0; i < 200; i++ {
		oe.sh.upsert(podEntry("ns", fmt.Sprintf("p%03d", i), t0.Add(time.Duration(i)*time.Minute), "node", corev1.PodRunning))
	}
	id := oe.openPods(t, func(s *ViewSpec) { s.Sort = SortSpec{Key: "cpu", Dir: "desc"} })
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		rng := rand.New(rand.NewSource(1))
		for i := 0; i < 200; i++ {
			recs := map[string]OverlayRecord{}
			for j := 0; j < 200; j++ {
				recs[fmt.Sprintf("ns/p%03d", j)] = podRecord(float64(rng.Intn(100)), float64(rng.Intn(100)))
			}
			oe.hub.Publish("ctx", OverlayPodMetrics, BoolPtr(true), recs)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			if _, err := oe.m.Window(id, 0, 50, 0); err != nil {
				t.Error(err)
				return
			}
			_, _ = oe.m.Facet(id, "status")
			oe.m.TakeNotices()
		}
	}()
	wg.Wait()
}
