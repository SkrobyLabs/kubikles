package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/metadata"

	"kubikles/pkg/k8s"
	"kubikles/pkg/store"
)

type fakeStoreClients struct{ cs kubernetes.Interface }

func (f fakeStoreClients) Kube(string) (kubernetes.Interface, error) { return f.cs, nil }
func (f fakeStoreClients) Dynamic(string) (dynamic.Interface, error) { return nil, errors.New("none") }
func (f fakeStoreClients) Metadata(string) (metadata.Interface, error) {
	return nil, store.ErrNoMetadata
}

func TestStoreIsResetByStopAllWatchersAndShownInMetrics(t *testing.T) {
	a := newPerfApp()
	a.watcherManager = NewResourceWatcherManager(context.Background(), a)
	a.store = store.New(fakeStoreClients{cs: fake.NewSimpleClientset()}, store.Options{})
	defer a.closeStore()

	lease, err := a.store.Acquire(store.Key{Context: "c", Type: "pods"}, store.Hint{})
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Release()
	deadline := time.Now().Add(5 * time.Second)
	for !lease.State().Synced && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}

	m := a.GetPerformanceMetrics()
	if m.Store == nil || len(m.Store.Shards) != 1 || m.Store.Shards[0].Type != "pods" {
		t.Fatalf("metrics store = %+v", m.Store)
	}

	// Polling mode calls StopAllWatchers on every mode switch; it must leave open views and
	// shards alone (a context switch resets the store through SwitchContext).
	a.StopAllWatchers()
	select {
	case <-lease.Done():
		t.Fatal("StopAllWatchers tore down a store shard")
	case <-time.After(100 * time.Millisecond):
	}
	if got := a.storeStats(); got == nil || len(got.Shards) != 1 {
		t.Fatalf("shards after StopAllWatchers = %+v", got)
	}

	a.resetStore()
	select {
	case <-lease.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("resetStore did not reset the store")
	}
	if got := a.storeStats(); got == nil || len(got.Shards) != 0 {
		t.Fatalf("shards after reset = %+v", got)
	}
}

// With the windowed-lists setting off nothing else wants a pod or node shard, so the metrics
// pin must not start one; it only keeps a shard that is already open.
func TestPinMetricsShardsNeverCreatesShards(t *testing.T) {
	a := newPerfApp()
	a.store = store.New(fakeStoreClients{cs: fake.NewSimpleClientset()}, store.Options{})
	defer a.closeStore()

	a.pinExistingShards("c", "pods", "nodes")
	if got := a.storeStats(); got == nil || len(got.Shards) != 0 {
		t.Fatalf("pinning with no shard created one: %+v", got)
	}

	lease, err := a.store.Acquire(store.Key{Context: "c", Type: "pods"}, store.Hint{})
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Release()
	a.pinExistingShards("c", "pods", "nodes")
	got := a.storeStats()
	if len(got.Shards) != 1 || got.Shards[0].Type != "pods" {
		t.Fatalf("shards = %+v, want only the pods shard", got.Shards)
	}
	if got.Shards[0].Refs != 2 {
		t.Fatalf("pods shard refs = %d, want 2 (lease plus pin)", got.Shards[0].Refs)
	}
}

func TestMetricsOmitStoreWhenNotRunning(t *testing.T) {
	a := newPerfApp()
	if m := a.GetPerformanceMetrics(); m.Store != nil {
		t.Fatalf("store stats without a store: %+v", m.Store)
	}
	a.resetStore()
	a.resyncStore()
	a.closeStore() // all must tolerate a missing store
}

func TestInitStoreWiresPodNodeSourceAndSinks(t *testing.T) {
	a := newPerfApp()
	a.k8sClient = &k8s.Client{}
	a.initStore()
	defer a.closeStore()
	if a.store == nil {
		t.Fatal("store not created")
	}
	// Without a client the provider reports why instead of panicking.
	if _, err := a.store.Acquire(store.Key{Context: "c", Type: "bogus"}, store.Hint{}); err == nil {
		t.Fatal("expected unsupported type error")
	}
}

func TestStoreMaxBytesEnv(t *testing.T) {
	t.Setenv(envStoreMaxBytes, "")
	want := defaultStoreBudget(int64(containerMemoryLimit()), debugMemoryLimit())
	if got := storeMaxBytes(); got != want {
		t.Fatalf("default = %d, want %d", got, want)
	}
	t.Setenv(envStoreMaxBytes, "12345")
	if got := storeMaxBytes(); got != 12345 {
		t.Fatalf("env = %d", got)
	}
	t.Setenv(envStoreMaxBytes, "0")
	if got := storeMaxBytes(); got != 0 {
		t.Fatalf("zero must mean unlimited, got %d", got)
	}
	t.Setenv(envStoreMaxBytes, "junk")
	if got := storeMaxBytes(); got != want {
		t.Fatalf("junk = %d", got)
	}
}

func TestDefaultStoreBudgetFollowsMemoryLimit(t *testing.T) {
	const mi = int64(1) << 20
	cases := []struct {
		name          string
		cgroup, limit int64
		want          int64
	}{
		{"no limit", 0, 0, store.DefaultMaxBytes},
		{"512Mi pod", 512 * mi, 435 * mi, 435 * mi / 100 * 40},
		{"cgroup only", 512 * mi, 0, 512 * mi / 100 * 40},
		{"gomemlimit only", 0, 1024 * mi, 1024 * mi / 100 * 40},
		{"large machine keeps the cap", 64 << 30, 0, store.DefaultMaxBytes},
	}
	for _, c := range cases {
		if got := defaultStoreBudget(c.cgroup, c.limit); got != c.want {
			t.Errorf("%s: %d, want %d", c.name, got, c.want)
		}
	}
}

// The metrics calls only keep an already-open shard warm; they never start a cluster-wide
// LIST+WATCH themselves (the windowed-lists setting may be off).
func TestMetricsCallsPinOnlyExistingShards(t *testing.T) {
	a := newPerfApp()
	cs := fake.NewSimpleClientset()
	a.k8sClient = k8s.NewClientWithClientset(cs, "c")
	a.store = store.New(fakeStoreClients{cs: cs}, store.Options{})
	defer a.closeStore()
	if a.GetCurrentContext() != "c" {
		t.Skip("current context is not taken from the client in this configuration")
	}

	a.pinMetricsShards("pods", "nodes")
	if n := len(a.storeStats().Shards); n != 0 {
		t.Fatalf("metrics pin created %d shards with an empty store", n)
	}

	lease, err := a.store.Acquire(store.Key{Context: "c", Type: "pods"}, store.Hint{Mode: store.ModeFull})
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Release()
	a.pinMetricsShards("pods", "nodes")
	shards := a.storeStats().Shards
	if len(shards) != 1 || shards[0].Type != "pods" || shards[0].Refs != 2 {
		t.Fatalf("shards = %+v, want only the pods shard, pinned (refs 2)", shards)
	}

	// Without a store the call is a no-op.
	b := newPerfApp()
	b.pinMetricsShards("pods")
}
