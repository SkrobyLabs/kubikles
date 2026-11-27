package store

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/metadata"
	"k8s.io/client-go/rest"

	"kubikles/pkg/k8s"
)

type apiHarness struct {
	t       *testing.T
	srv     *httptest.Server
	mu      sync.Mutex
	accepts []string
	queries []string
	handler func(w http.ResponseWriter, r *http.Request)
}

func newAPIHarness(t *testing.T, h func(w http.ResponseWriter, r *http.Request)) *apiHarness {
	a := &apiHarness{t: t, handler: h}
	a.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		a.accepts = append(a.accepts, r.Header.Get("Accept"))
		a.queries = append(a.queries, r.URL.Path+"?"+r.URL.RawQuery)
		a.mu.Unlock()
		h(w, r)
	}))
	t.Cleanup(a.srv.Close)
	return a
}

func (a *apiHarness) kube() kubernetes.Interface {
	cs, err := kubernetes.NewForConfig(&rest.Config{Host: a.srv.URL, QPS: -1})
	if err != nil {
		a.t.Fatal(err)
	}
	return cs
}

func (a *apiHarness) meta() metadata.Interface {
	md, err := metadata.NewForConfig(&rest.Config{Host: a.srv.URL, QPS: -1})
	if err != nil {
		a.t.Fatal(err)
	}
	return md
}

func (a *apiHarness) seen() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.queries...)
}

func (a *apiHarness) sawAccepts(substr string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, s := range a.accepts {
		if strings.Contains(s, substr) {
			return true
		}
	}
	return false
}

func tableJSON(cols bool, rv, cont string, rows ...[]interface{}) map[string]interface{} {
	meta := map[string]interface{}{"resourceVersion": rv}
	if cont != "" {
		meta["continue"] = cont
	}
	out := map[string]interface{}{"kind": "Table", "apiVersion": "meta.k8s.io/v1", "metadata": meta}
	if cols {
		out["columnDefinitions"] = []map[string]interface{}{{"name": "Name"}, {"name": "Type"}, {"name": "Data"}}
	}
	var rs []map[string]interface{}
	for _, r := range rows {
		name := r[0].(string)
		rs = append(rs, map[string]interface{}{
			"cells": r,
			"object": map[string]interface{}{
				"kind": "PartialObjectMetadata", "apiVersion": "meta.k8s.io/v1",
				"metadata": map[string]interface{}{"name": name, "namespace": "ns", "uid": "uid-" + name, "resourceVersion": rv},
			},
		})
	}
	out["rows"] = rs
	return out
}

func isTableRequest(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "as=Table")
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// tableServer serves secrets as Tables: two list pages, then a watch stream.
func tableServer(t *testing.T) *apiHarness {
	return newAPIHarness(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("watch") == "true" {
			fl := w.(http.Flusher)
			w.Header().Set("Content-Type", "application/json")
			enc := json.NewEncoder(w)
			// First event carries columnDefinitions, later ones do not.
			_ = enc.Encode(map[string]interface{}{"type": "ADDED", "object": tableJSON(true, "201", "", []interface{}{"watch-new", "Opaque", 2})})
			fl.Flush()
			_ = enc.Encode(map[string]interface{}{"type": "MODIFIED", "object": tableJSON(false, "202", "", []interface{}{"first", "Opaque", 9})})
			fl.Flush()
			_ = enc.Encode(map[string]interface{}{"type": "BOOKMARK", "object": map[string]interface{}{
				"kind": "Secret", "apiVersion": "v1", "metadata": map[string]interface{}{"resourceVersion": "300"}}})
			fl.Flush()
			_ = enc.Encode(map[string]interface{}{"type": "DELETED", "object": tableJSON(false, "301", "", []interface{}{"second", "Opaque", 1})})
			fl.Flush()
			<-r.Context().Done()
			return
		}
		if !isTableRequest(r) {
			http.Error(w, "table only", http.StatusNotAcceptable)
			return
		}
		if r.URL.Query().Get("continue") == "" {
			writeJSON(w, tableJSON(true, "200", "more", []interface{}{"first", "Opaque", 1}, []interface{}{"second", "Opaque", 1}))
			return
		}
		writeJSON(w, tableJSON(true, "200", "", []interface{}{"third", "kubernetes.io/tls", 2}))
	})
}

func TestTableSourceListWatchThroughManager(t *testing.T) {
	api := tableServer(t)
	m := New(&fakeProvider{cs: api.kube()}, Options{PageSize: 2})
	defer m.Close()

	key := Key{Context: "ctx", Type: "secrets", Namespace: "ns"}
	l, err := m.Acquire(key, Hint{Mode: ModeTable})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Release()
	eventually(t, "table sync and watch events", func() bool {
		st := l.State()
		return st.Synced && st.RV == "301" && st.Entries == 3
	})

	st := l.State()
	if st.Mode != ModeTable || st.Degraded {
		t.Fatalf("state = %+v", st)
	}
	cols := l.Columns()
	if len(cols) != 3 || cols[0].Name != "Name" {
		t.Fatalf("columns = %+v", cols)
	}
	byName := map[string]*Entry{}
	snap, _ := l.Snapshot()
	for _, e := range snap {
		byName[e.Name] = e
		if _, ok := e.Obj.(*metav1.PartialObjectMetadata); !ok {
			t.Fatalf("table entry object = %T", e.Obj)
		}
	}
	if _, ok := byName["second"]; ok {
		t.Fatal("DELETED row still present")
	}
	if e := byName["first"]; e == nil || e.Cells[2] != float64(9) || e.RV != "202" {
		t.Fatalf("first = %+v", e)
	}
	if e := byName["watch-new"]; e == nil || e.Cells[1] != "Opaque" || e.UID != "uid-watch-new" {
		t.Fatalf("watch-new = %+v", e)
	}
	if !api.sawAccepts("as=Table") {
		t.Fatal("Table Accept header was never sent")
	}
	var sawLimit, sawContinue, sawWatchRV bool
	for _, q := range api.seen() {
		sawLimit = sawLimit || strings.Contains(q, "limit=2")
		sawContinue = sawContinue || strings.Contains(q, "continue=more")
		sawWatchRV = sawWatchRV || (strings.Contains(q, "watch=true") && strings.Contains(q, "resourceVersion=200"))
	}
	if !sawLimit || !sawContinue || !sawWatchRV {
		t.Fatalf("requests = %v", api.seen())
	}
}

func TestTableWatchHandlesColumnsOnlyInFirstEventAndErrors(t *testing.T) {
	api := newAPIHarness(t, func(w http.ResponseWriter, r *http.Request) {
		fl := w.(http.Flusher)
		w.Header().Set("Content-Type", "application/json")
		enc := json.NewEncoder(w)
		_ = enc.Encode(map[string]interface{}{"type": "ADDED", "object": tableJSON(true, "1", "", []interface{}{"a", "Opaque", 1})})
		_ = enc.Encode(map[string]interface{}{"type": "MODIFIED", "object": tableJSON(false, "2", "", []interface{}{"a", "Opaque", 2})})
		_ = enc.Encode(map[string]interface{}{"type": "ERROR", "object": map[string]interface{}{
			"kind": "Status", "apiVersion": "v1", "status": "Failure", "reason": "Expired", "code": 410, "message": "too old"}})
		fl.Flush()
	})
	src := newTableSource(k8s.RESTClientOf(api.kube()), schema.GroupVersionResource{Version: "v1", Resource: "secrets"}, "ns", 0)
	w, err := src.Watch(t.Context(), "")
	if err != nil {
		t.Fatal(err)
	}
	defer w.Stop()
	var evs []watch.Event
	for ev := range w.ResultChan() {
		evs = append(evs, ev)
	}
	if len(evs) != 3 {
		t.Fatalf("events = %d", len(evs))
	}
	first, second := evs[0].Object.(*TableRow), evs[1].Object.(*TableRow)
	if len(first.Columns) != 3 || len(second.Columns) != 3 {
		t.Fatalf("columns must be remembered across events: %d %d", len(first.Columns), len(second.Columns))
	}
	st, ok := evs[2].Object.(*metav1.Status)
	if evs[2].Type != watch.Error || !ok || st.Reason != metav1.StatusReasonExpired {
		t.Fatalf("error event = %+v", evs[2])
	}
}

func TestTableWatchNonTableEventReportsNotAcceptable(t *testing.T) {
	api := newAPIHarness(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"type": "ADDED", "object": map[string]interface{}{
			"kind": "Secret", "apiVersion": "v1", "metadata": map[string]interface{}{"name": "x", "resourceVersion": "1"}}})
	})
	src := newTableSource(k8s.RESTClientOf(api.kube()), schema.GroupVersionResource{Version: "v1", Resource: "secrets"}, "", 0)
	w, err := src.Watch(t.Context(), "5")
	if err != nil {
		t.Fatal(err)
	}
	defer w.Stop()
	ev, ok := <-w.ResultChan()
	if !ok || ev.Type != watch.Error {
		t.Fatalf("event = %+v", ev)
	}
	if st := ev.Object.(*metav1.Status); st.Reason != metav1.StatusReasonNotAcceptable {
		t.Fatalf("status = %+v", st)
	}
}

func TestTableListNotAcceptableMapsToModeUnsupported(t *testing.T) {
	api := newAPIHarness(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotAcceptable)
		_ = json.NewEncoder(w).Encode(metav1.Status{TypeMeta: metav1.TypeMeta{Kind: "Status", APIVersion: "v1"},
			Status: metav1.StatusFailure, Reason: metav1.StatusReasonNotAcceptable, Code: 406})
	})
	src := newTableSource(k8s.RESTClientOf(api.kube()), schema.GroupVersionResource{Version: "v1", Resource: "secrets"}, "", 0)
	_, err := src.List(t.Context(), ListOpts{}, func(Page) {})
	if !isModeUnsupported(err) {
		t.Fatalf("err = %v", err)
	}
	if apierrors.IsNotAcceptable(err) == false && !isModeUnsupported(err) {
		t.Fatal("unreachable")
	}
}

func TestTableListOfNonTableKindMapsToModeUnsupported(t *testing.T) {
	api := newAPIHarness(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{"kind": "SecretList", "apiVersion": "v1", "metadata": map[string]interface{}{"resourceVersion": "1"}, "items": []interface{}{}})
	})
	src := newTableSource(k8s.RESTClientOf(api.kube()), schema.GroupVersionResource{Version: "v1", Resource: "secrets"}, "", 0)
	if _, err := src.List(t.Context(), ListOpts{}, func(Page) {}); !isModeUnsupported(err) {
		t.Fatalf("err = %v", err)
	}
}

// A server that refuses Tables degrades the shard to full objects.
func TestManagerFallsBackToFullWhenServerRefusesTable(t *testing.T) {
	api := newAPIHarness(t, func(w http.ResponseWriter, r *http.Request) {
		if isTableRequest(r) {
			http.Error(w, "no tables here", http.StatusNotAcceptable)
			return
		}
		if r.URL.Query().Get("watch") == "true" {
			w.Header().Set("Content-Type", "application/json")
			w.(http.Flusher).Flush()
			<-r.Context().Done()
			return
		}
		writeJSON(w, v1.PodList{TypeMeta: metav1.TypeMeta{Kind: "PodList", APIVersion: "v1"},
			ListMeta: metav1.ListMeta{ResourceVersion: "9"},
			Items:    []v1.Pod{pod("ns", "p1", "9")}})
	})
	m := New(&fakeProvider{cs: api.kube()}, Options{})
	defer m.Close()
	l, err := m.Acquire(Key{Context: "c", Type: "pods", Namespace: "ns"}, Hint{Mode: ModeTable})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Release()
	eventually(t, "fallback sync", func() bool { st := l.State(); return st.Synced && st.Mode == ModeFull })
	st := l.State()
	if !st.Degraded || !strings.Contains(st.DegradedReason, "fell back") || st.Entries != 1 {
		t.Fatalf("state = %+v", st)
	}
	snap, _ := l.Snapshot()
	if _, ok := snap[0].Obj.(*v1.Pod); !ok {
		t.Fatalf("object = %T", snap[0].Obj)
	}
}

func TestMetadataSourceListAndWatch(t *testing.T) {
	api := newAPIHarness(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		item := func(name, rv string) map[string]interface{} {
			return map[string]interface{}{"kind": "PartialObjectMetadata", "apiVersion": "meta.k8s.io/v1",
				"metadata": map[string]interface{}{"name": name, "namespace": "ns", "uid": "u-" + name, "resourceVersion": rv,
					"managedFields": []interface{}{map[string]interface{}{"manager": "x"}}}}
		}
		if r.URL.Query().Get("watch") == "true" {
			enc := json.NewEncoder(w)
			_ = enc.Encode(map[string]interface{}{"type": "ADDED", "object": item("w1", "51")})
			w.(http.Flusher).Flush()
			<-r.Context().Done()
			return
		}
		list := map[string]interface{}{"kind": "PartialObjectMetadataList", "apiVersion": "meta.k8s.io/v1",
			"metadata": map[string]interface{}{"resourceVersion": "50"}}
		if r.URL.Query().Get("continue") == "" {
			list["metadata"].(map[string]interface{})["continue"] = "c2"
			list["items"] = []interface{}{item("a", "1"), item("b", "2")}
		} else {
			list["items"] = []interface{}{item("c", "3")}
		}
		_ = json.NewEncoder(w).Encode(list)
	})
	m := New(&fakeProvider{cs: api.kube(), md: api.meta()}, Options{PageSize: 2})
	defer m.Close()
	l, err := m.Acquire(Key{Context: "c", Type: "configmaps", Namespace: "ns"}, Hint{Mode: ModeMetadata})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Release()
	eventually(t, "metadata sync + watch", func() bool { st := l.State(); return st.Synced && st.Entries == 4 })
	st := l.State()
	if st.Mode != ModeMetadata || st.Degraded {
		t.Fatalf("state = %+v", st)
	}
	snap, _ := l.Snapshot()
	for _, e := range snap {
		pm, ok := e.Obj.(*metav1.PartialObjectMetadata)
		if !ok || pm.ManagedFields != nil {
			t.Fatalf("entry %s = %#v", e.Name, e.Obj)
		}
	}
	var sawLimit, sawWatchRV bool
	for _, q := range api.seen() {
		sawLimit = sawLimit || strings.Contains(q, "limit=2")
		sawWatchRV = sawWatchRV || (strings.Contains(q, "watch=true") && strings.Contains(q, "resourceVersion=50"))
	}
	if !sawLimit || !sawWatchRV {
		t.Fatalf("requests = %v", api.seen())
	}
	if !api.sawAccepts("PartialObjectMetadata") {
		t.Fatal("metadata Accept header never sent")
	}
}

func TestDynamicSourceServesCustomResources(t *testing.T) {
	gvr := schema.GroupVersionResource{Group: "example.io", Version: "v1", Resource: "widgets"}
	mk := func(name string) *unstructured.Unstructured {
		return &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "example.io/v1", "kind": "Widget",
			"metadata": map[string]interface{}{"name": name, "namespace": "ns", "uid": "u-" + name, "resourceVersion": "1",
				"managedFields": []interface{}{map[string]interface{}{"manager": "x"}}},
		}}
	}
	dc := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(),
		map[schema.GroupVersionResource]string{gvr: "WidgetList"}, mk("a"), mk("b"))
	m := New(&fakeProvider{cs: newFixture(t).cs, dyn: dc}, Options{})
	defer m.Close()
	l, err := m.Acquire(Key{Context: "c", Type: CRDType("example.io", "v1", "widgets")}, Hint{})
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
	if _, has := u.Object["metadata"].(map[string]interface{})["managedFields"]; has {
		t.Fatal("managed fields not stripped")
	}
	// A live change arrives through the dynamic watch.
	if _, err := dc.Resource(gvr).Namespace("ns").Create(t.Context(), mk("c"), metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	eventually(t, "dynamic watch event", func() bool { return l.State().Entries == 3 })
}

func TestParseCRDType(t *testing.T) {
	g, ok := ParseCRDType(CRDType("a.b", "v1", "things"))
	if !ok || g != (schema.GroupVersionResource{Group: "a.b", Version: "v1", Resource: "things"}) {
		t.Fatalf("got %v %v", g, ok)
	}
	for _, bad := range []string{"pods", "crd:", "crd:a/b", "crd:a//c", "crd:a/b/c/d"} {
		if _, ok := ParseCRDType(bad); ok {
			t.Fatalf("%q parsed", bad)
		}
	}
}

func TestPageSizeZeroSendsNoLimit(t *testing.T) {
	api := newAPIHarness(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("watch") == "true" {
			<-r.Context().Done()
			return
		}
		writeJSON(w, v1.PodList{TypeMeta: metav1.TypeMeta{Kind: "PodList", APIVersion: "v1"}, ListMeta: metav1.ListMeta{ResourceVersion: "1"}})
	})
	m := New(&fakeProvider{cs: api.kube()}, Options{})
	defer m.Close()
	l, err := m.Acquire(Key{Context: "c", Type: "pods"}, Hint{NoProgress: true})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Release()
	waitSynced(t, l)
	for _, q := range api.seen() {
		if strings.Contains(q, "limit=") {
			t.Fatalf("limit sent without progress: %s", q)
		}
	}
}
