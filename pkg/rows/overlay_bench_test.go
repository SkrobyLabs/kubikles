package rows

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"

	"kubikles/pkg/store"
)

const benchPods = 50000

// overlayBenchEnv opens a pod view over benchPods pods and returns two alternating record sets:
// publishing them in turn makes the share of rows given by changed differ on every tick.
func overlayBenchEnv(b *testing.B, sortKey string, filter bool) (*overlayEnv, [2]map[string]OverlayRecord) {
	b.Helper()
	oe := &overlayEnv{}
	oe.hub = NewOverlayHub(nil)
	oe.testEnv = &testEnv{p: newFakeProvider(), clock: newManualClock()}
	oe.m = NewManager(Config{Provider: oe.p, Context: func() string { return "ctx" }, Clock: oe.clock, NoJanitor: true, Overlays: oe.hub})
	b.Cleanup(oe.m.Close)
	oe.sh = oe.p.shard("pods")
	rng := rand.New(rand.NewSource(7))
	var sets [2]map[string]OverlayRecord
	sets[0], sets[1] = map[string]OverlayRecord{}, map[string]OverlayRecord{}
	for i := 0; i < benchPods; i++ {
		e := podEntry(fmt.Sprintf("ns%d", i%20), fmt.Sprintf("pod-%06d", (i*7919)%benchPods), t0.Add(time.Duration(i%5000)*time.Minute), "node", corev1.PodRunning)
		oe.sh.mu.Lock()
		oe.sh.seq++
		e.Seq = oe.sh.seq
		oe.sh.entries[e.ID()] = e
		oe.sh.mu.Unlock()
		key := PodKey(e.Namespace, e.Name)
		sets[0][key] = podRecord(float64(rng.Intn(100)), float64(rng.Intn(100)))
		sets[1][key] = podRecord(float64(rng.Intn(100)), float64(rng.Intn(100)))
	}
	spec := ViewSpec{ViewID: "b", ClientToken: "t", Type: "pods", Namespaces: []string{"*"}, Sort: SortSpec{Key: sortKey, Dir: "desc"}}
	if filter {
		spec.ColumnFilters = []ColumnFilterSpec{{Col: "cpu", Type: "numeric", Conditions: []FilterCondition{{Operator: ">=", Value: ptr(50)}}}}
	}
	if _, err := oe.m.Open(spec); err != nil {
		b.Fatal(err)
	}
	oe.hub.Publish("ctx", OverlayPodMetrics, BoolPtr(true), sets[0])
	return oe, sets
}

// One metrics poll on a 50k-pod view in which every pod's sample changes: the delta against the
// previous snapshot, the cell update of every row and, when sorted by cpu, the re-sort.
func BenchmarkOverlayTickAllChanged(b *testing.B) {
	for _, tc := range []struct {
		name   string
		sort   string
		filter bool
	}{
		{"sorted-by-name", "name", false},
		{"sorted-by-cpu", "cpu", false},
		{"sorted-by-cpu-filtered", "cpu", true},
	} {
		b.Run(tc.name, func(b *testing.B) {
			oe, sets := overlayBenchEnv(b, tc.sort, tc.filter)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				oe.hub.Publish("ctx", OverlayPodMetrics, BoolPtr(true), sets[(i+1)%2])
			}
		})
	}
}

// A realistic tick changes a minority of the samples (idle pods repeat the same value).
func BenchmarkOverlayTickTenPercentChanged(b *testing.B) {
	for _, sortKey := range []string{"name", "cpu"} {
		b.Run("sorted-by-"+sortKey, func(b *testing.B) {
			oe, sets := overlayBenchEnv(b, sortKey, false)
			// Mixed sets: the first tenth of the keys differ between the two, the rest are shared.
			mixed := [2]map[string]OverlayRecord{{}, {}}
			n := 0
			for k := range sets[0] {
				mixed[0][k] = sets[0][k]
				if n%10 == 0 {
					mixed[1][k] = sets[1][k]
				} else {
					mixed[1][k] = sets[0][k]
				}
				n++
			}
			oe.hub.Publish("ctx", OverlayPodMetrics, BoolPtr(true), mixed[0])
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				oe.hub.Publish("ctx", OverlayPodMetrics, BoolPtr(true), mixed[(i+1)%2])
			}
		})
	}
}

// The same tick as a full rebuild of the view, for comparison.
func BenchmarkOverlayFullRebuild(b *testing.B) {
	oe, sets := overlayBenchEnv(b, "cpu", false)
	v, _ := oe.m.get("b")
	all, _ := (&fakeLease{s: oe.sh}).Snapshot()
	ov := map[string]*OverlaySnapshot{OverlayPodMetrics: {Seq: 2, Available: BoolPtr(true), Records: sets[1]}}
	cfg := v.cfg
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buildCoreOv(cfg, all, nil, oe.clock.Now(), 1, ov)
	}
}

var _ = store.ModeFull
