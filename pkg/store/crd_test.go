package store

import (
	"net/http"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

var widgetGVR = schema.GroupVersionResource{Group: "example.io", Version: "v1", Resource: "widgets"}

func widget(name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "example.io/v1", "kind": "Widget",
		"metadata": map[string]interface{}{"name": name, "namespace": "ns", "uid": "u-" + name, "resourceVersion": "1"},
		"spec":     map[string]interface{}{"blob": strings.Repeat("x", 1000)},
		"status":   map[string]interface{}{"phase": "Ready"},
	}}
}

func widgetDyn(names ...string) *dynamicfake.FakeDynamicClient {
	var objs []runtime.Object
	for _, n := range names {
		objs = append(objs, widget(n))
	}
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(),
		map[schema.GroupVersionResource]string{widgetGVR: "WidgetList"}, objs...)
}

func TestCRDPruneKeepsMetadataAndNamedRoots(t *testing.T) {
	m := New(&fakeProvider{cs: newFixture(t).cs, dyn: widgetDyn("a", "b")}, Options{})
	defer m.Close()
	key := Key{Context: "c", Type: CRDType("example.io", "v1", "widgets")}
	l, err := m.Acquire(key, Hint{Prune: []string{"status"}})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Release()
	waitSynced(t, l)
	snap, _ := l.Snapshot()
	if len(snap) != 2 {
		t.Fatalf("entries = %d", len(snap))
	}
	u := snap[0].Obj.(*unstructured.Unstructured)
	if _, has := u.Object["spec"]; has {
		t.Fatal("spec was not pruned")
	}
	if u.Object["status"] == nil || u.Object["metadata"] == nil || u.Object["kind"] != "Widget" {
		t.Fatalf("kept fields = %v", u.Object)
	}
	if st := m.Stats(); st.EstimatedBytes == 0 {
		t.Fatalf("a small shard must still report an estimated size: %+v", st)
	}
	// A pruned shard never answers for full-object reads.
	if _, _, ok := m.Lookup(key, "ns", "a", ""); ok {
		t.Fatal("Lookup answered from a pruned shard")
	}
	if !m.IsPruned(key.Type) {
		t.Fatal("IsPruned = false")
	}
}

func TestCRDWithoutPruneKeepsWholeObjects(t *testing.T) {
	m := New(&fakeProvider{cs: newFixture(t).cs, dyn: widgetDyn("a")}, Options{})
	defer m.Close()
	key := Key{Context: "c", Type: CRDType("example.io", "v1", "widgets")}
	l, err := m.Acquire(key, Hint{})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Release()
	waitSynced(t, l)
	snap, _ := l.Snapshot()
	if _, has := snap[0].Obj.(*unstructured.Unstructured).Object["spec"]; !has {
		t.Fatal("spec dropped without prune roots")
	}
	if _, _, ok := m.Lookup(key, "ns", "a", ""); !ok {
		t.Fatal("warm full-object shard should answer Lookup")
	}
}

func crdTableServer(t *testing.T, notAcceptable bool) *apiHarness {
	return newAPIHarness(t, func(w http.ResponseWriter, r *http.Request) {
		if notAcceptable {
			http.Error(w, "no tables here", http.StatusNotAcceptable)
			return
		}
		if r.URL.Query().Get("watch") == "true" {
			<-r.Context().Done()
			return
		}
		writeJSON(w, tableJSON(true, "50", "", []interface{}{"a", "Ready", 1}, []interface{}{"b", "Pending", 2}))
	})
}

func TestCRDTableSourceUsesAPIGroupPath(t *testing.T) {
	api := crdTableServer(t, false)
	m := New(&fakeProvider{cs: api.kube()}, Options{})
	defer m.Close()
	key := Key{Context: "c", Type: CRDType("example.io", "v1", "widgets"), Namespace: "ns"}
	l, err := m.Acquire(key, Hint{Mode: ModeTable})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Release()
	eventually(t, "table sync", func() bool { return l.State().Synced && l.State().Entries == 2 })
	if st := l.State(); st.Mode != ModeTable || st.Degraded {
		t.Fatalf("state = %+v", st)
	}
	saw := func() (list, watch bool) {
		for _, q := range api.seen() {
			list = list || strings.HasPrefix(q, "/apis/example.io/v1/namespaces/ns/widgets?")
			watch = watch || (strings.Contains(q, "watch=true") && strings.Contains(q, "resourceVersion=50"))
		}
		return
	}
	eventually(t, "list and watch requests", func() bool { l, w := saw(); return l && w })
	if !api.sawAccepts("as=Table") {
		t.Fatalf("requests = %v", api.seen())
	}
}

func TestCRDFallbackChainTableToPrunedFull(t *testing.T) {
	api := crdTableServer(t, true)
	m := New(&fakeProvider{cs: api.kube(), dyn: widgetDyn("a", "b")}, Options{})
	defer m.Close()
	key := Key{Context: "c", Type: CRDType("example.io", "v1", "widgets")}
	l, err := m.Acquire(key, Hint{Mode: ModeTable, Prune: []string{"status"}})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Release()
	eventually(t, "fallback sync", func() bool { return l.State().Synced && l.State().Entries == 2 })
	st := l.State()
	if st.Mode != ModeFull || !st.Degraded {
		t.Fatalf("state = %+v", st)
	}
	snap, _ := l.Snapshot()
	u := snap[0].Obj.(*unstructured.Unstructured)
	if _, has := u.Object["spec"]; has || u.Object["status"] == nil {
		t.Fatalf("fallback objects were not pruned: %v", u.Object)
	}
}

func TestPruneRootsChangeRebuildsShard(t *testing.T) {
	m := New(&fakeProvider{cs: newFixture(t).cs, dyn: widgetDyn("a")}, Options{})
	defer m.Close()
	key := Key{Context: "c", Type: CRDType("example.io", "v1", "widgets")}
	l1, err := m.Acquire(key, Hint{Prune: []string{"status"}})
	if err != nil {
		t.Fatal(err)
	}
	defer l1.Release()
	waitSynced(t, l1)
	l2, err := m.Acquire(key, Hint{Prune: []string{"spec"}})
	if err != nil {
		t.Fatal(err)
	}
	defer l2.Release()
	eventually(t, "rebuilt with spec", func() bool {
		snap, _ := l2.Snapshot()
		if len(snap) != 1 {
			return false
		}
		u := snap[0].Obj.(*unstructured.Unstructured)
		return u.Object["spec"] != nil && u.Object["status"] == nil
	})
}
