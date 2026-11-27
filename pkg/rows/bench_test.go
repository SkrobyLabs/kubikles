package rows

import (
	"fmt"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"

	"kubikles/pkg/store"
)

const benchEntries = 50000

func benchEnv(b *testing.B, sortKey string) (*testEnv, *fakeShard, string, []*store.Entry) {
	b.Helper()
	env := &testEnv{p: newFakeProvider(), clock: newManualClock()}
	env.m = NewManager(Config{Provider: env.p, Context: func() string { return "ctx" }, Clock: env.clock, NoJanitor: true})
	b.Cleanup(env.m.Close)
	sh := env.p.shard("configmaps")
	entries := make([]*store.Entry, benchEntries)
	for i := range entries {
		e := cm(fmt.Sprintf("ns%d", i%20), fmt.Sprintf("config-%06d", (i*7919)%benchEntries), t0.Add(time.Duration(i%5000)*time.Minute),
			map[string]string{"a": "1", "b": "2"})
		entries[i] = e
		sh.mu.Lock()
		sh.seq++
		e.Seq = sh.seq
		sh.entries[e.ID()] = e
		sh.mu.Unlock()
	}
	res, err := env.m.Open(ViewSpec{ViewID: "b", ClientToken: "t", Type: "configmaps", Namespaces: []string{"*"}, Sort: SortSpec{Key: sortKey, Dir: "asc"}})
	if err != nil {
		b.Fatal(err)
	}
	return env, sh, res.ViewID, entries
}

// One modified object in a 50k-row view: the cost of an incremental update (binary search,
// in-place replace or move) including the store-to-view delivery.
func BenchmarkSingleModification(b *testing.B) {
	for _, key := range []string{"name", "age", "keyCount"} {
		b.Run(key, func(b *testing.B) {
			_, sh, _, entries := benchEnv(b, key)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				e := entries[(i*31)%len(entries)]
				ne := bump(e, func(c *corev1.ConfigMap) { c.Data = map[string]string{"x": fmt.Sprint(i % 7)} })
				entries[(i*31)%len(entries)] = ne
				sh.upsert(ne)
			}
		})
	}
}

// A heartbeat (new resourceVersion, nothing projected changes): must stay far cheaper than a change.
func BenchmarkHeartbeat(b *testing.B) {
	_, sh, _, entries := benchEnv(b, "name")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e := entries[(i*31)%len(entries)]
		ne := bump(e, nil)
		entries[(i*31)%len(entries)] = ne
		sh.upsert(ne)
	}
}

// Full rebuild: build every row, sort 50k.
func BenchmarkFullResort(b *testing.B) {
	for _, key := range []string{"name", "age", "size"} {
		b.Run(key, func(b *testing.B) {
			env, _, id, _ := benchEnv(b, key)
			v, _ := env.m.get(id)
			sh := env.p.shard("configmaps")
			all, _ := (&fakeLease{s: sh}).Snapshot()
			cfg := v.cfg
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				buildCore(cfg, all, nil, env.clock.Now(), 1)
			}
		})
	}
}

// Re-sorting through the view API (Update): rebuild plus install, no lock held during the sort.
func BenchmarkUpdateSort(b *testing.B) {
	env, _, id, _ := benchEnv(b, "name")
	dirs := []string{"desc", "asc"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s := SortSpec{Key: "name", Dir: dirs[i%2]}
		if _, err := env.m.Update(id, ViewPatch{Sort: &s}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkWindowFetch(b *testing.B) {
	env, _, id, _ := benchEnv(b, "name")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := env.m.Window(id, (i*100)%40000, 100, 0); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSearchUpdate(b *testing.B) {
	env, _, id, _ := benchEnv(b, "name")
	qs := []string{"config-0001", "ns3 name:/0042$/", ""}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q := qs[i%len(qs)]
		if _, err := env.m.Update(id, ViewPatch{Search: &q}); err != nil {
			b.Fatal(err)
		}
	}
}
