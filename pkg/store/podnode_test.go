package store

import (
	"context"
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type countingSource struct{ pods, nodes int }

func (c *countingSource) ListPods(context.Context) (*v1.PodList, error) {
	c.pods++
	return &v1.PodList{Items: []v1.Pod{{ObjectMeta: metav1.ObjectMeta{Name: "from-api"}}}}, nil
}

func (c *countingSource) ListNodes(context.Context) (*v1.NodeList, error) {
	c.nodes++
	return &v1.NodeList{Items: []v1.Node{{ObjectMeta: metav1.ObjectMeta{Name: "node-from-api"}}}}, nil
}

func TestPodNodeSourceWarmServesFromStoreWithZeroAPIActions(t *testing.T) {
	f := newFixture(t)
	f.api.setItems("77", pod("a", "p1", "1"), pod("b", "p2", "2"))
	f.api.newWatcher()
	l := f.acquire(t, podsKey, Hint{})
	defer l.Release()
	waitSynced(t, l)

	fb := &countingSource{}
	src := NewPodNodeSource(f.m, func() string { return "ctx" }, fb)
	before := len(f.cs.Actions())
	list, err := src.ListPods(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Items) != 2 || list.ResourceVersion != "77" || fb.pods != 0 {
		t.Fatalf("items %d rv %q fallback calls %d", len(list.Items), list.ResourceVersion, fb.pods)
	}
	if after := len(f.cs.Actions()); after != before {
		t.Fatalf("warm read issued %d API actions", after-before)
	}

	// The result is freshly assembled: mutating it does not touch the store.
	list.Items[0].Name = "mutated"
	list.Items = list.Items[:0]
	again, _ := src.ListPods(t.Context())
	if len(again.Items) != 2 {
		t.Fatalf("store affected by caller mutation: %d", len(again.Items))
	}
	for _, p := range again.Items {
		if p.Name == "mutated" {
			t.Fatal("entry object was mutated through a returned item")
		}
	}
}

func TestPodNodeSourceColdFallsBackAndNeverStartsShards(t *testing.T) {
	f := newFixture(t)
	fb := &countingSource{}
	src := NewPodNodeSource(f.m, func() string { return "ctx" }, fb)

	pods, err := src.ListPods(t.Context())
	if err != nil || len(pods.Items) != 1 || pods.Items[0].Name != "from-api" || fb.pods != 1 {
		t.Fatalf("pods %v err %v fb %d", pods, err, fb.pods)
	}
	nodes, err := src.ListNodes(t.Context())
	if err != nil || len(nodes.Items) != 1 || fb.nodes != 1 {
		t.Fatalf("nodes %v err %v fb %d", nodes, err, fb.nodes)
	}
	if n := len(f.m.Stats().Shards); n != 0 {
		t.Fatalf("a read started %d shards", n)
	}
	if f.api.calls() != 0 {
		t.Fatal("a read listed from the API server through the store")
	}
}

func TestPodNodeSourceFallsBackForUnsyncedAndOtherContext(t *testing.T) {
	f := newFixture(t)
	f.api.mu.Lock()
	f.api.listErr = errBoom{}
	f.api.mu.Unlock()
	l := f.acquire(t, podsKey, Hint{})
	defer l.Release()
	fb := &countingSource{}
	src := NewPodNodeSource(f.m, func() string { return "ctx" }, fb)
	if _, _ = src.ListPods(t.Context()); fb.pods != 1 {
		t.Fatal("unsynced shard must not be served")
	}
	other := NewPodNodeSource(f.m, func() string { return "other" }, fb)
	if _, _ = other.ListPods(t.Context()); fb.pods != 2 {
		t.Fatal("different context must not be served")
	}
}

type errBoom struct{}

func (errBoom) Error() string { return "boom" }
