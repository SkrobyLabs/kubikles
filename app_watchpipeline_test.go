package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kubikles/pkg/events"
	"kubikles/pkg/k8s"
)

// These tests pin what the frontend receives from the watch pipeline after
// the single-marshal rewrite: managedFields are gone, everything else is
// preserved, and identity (namespace, coalescing key) comes from metadata
// rather than a decoded map. They cover both watch loops.

const managedFieldsJSON = `[{"manager":"kubectl","operation":"Update","apiVersion":"v1","fieldsType":"FieldsV1","fieldsV1":{"f:metadata":{"f:labels":{}}}}]`

func TestWatchPipelineStripsManagedFieldsFromPods(t *testing.T) {
	pod := podObject("with-mf", "ns1", "5")
	pod["metadata"].(map[string]interface{})["labels"] = map[string]interface{}{"app": "web"}
	var mf interface{}
	_ = json.Unmarshal([]byte(managedFieldsJSON), &mf)
	pod["metadata"].(map[string]interface{})["managedFields"] = mf
	pod["spec"] = map[string]interface{}{"nodeName": "n1", "containers": []interface{}{map[string]interface{}{"name": "c", "image": "i"}}}

	h := startWatchLoop(t, "", func(h *watchHarness, c *scriptedStream) {
		c.send("ADDED", pod)
		c.block()
	})
	waitUntil(t, "event", 5*time.Second, func() bool { return h.log.hasResource("ADDED", "with-mf") })

	got := h.log.snapshot()[0]
	if got.Namespace != "ns1" {
		t.Fatalf("namespace = %q, want taken from object metadata", got.Namespace)
	}
	res := resourceMap(got)
	md := res["metadata"].(map[string]interface{})
	if _, ok := md["managedFields"]; ok {
		t.Fatal("managedFields reached the frontend")
	}
	if md["name"] != "with-mf" || md["resourceVersion"] != "5" {
		t.Fatalf("metadata changed: %v", md)
	}
	if md["labels"].(map[string]interface{})["app"] != "web" {
		t.Fatalf("labels lost: %v", md)
	}
	spec := res["spec"].(map[string]interface{})
	if spec["nodeName"] != "n1" {
		t.Fatalf("spec lost: %v", spec)
	}
}

func TestWatchPipelineResourceIsRawJSONOnTheWire(t *testing.T) {
	// The envelope must serialize the resource inline (not as a quoted string
	// or base64), with the field names the frontend expects.
	ev := ResourceEvent{Type: "ADDED", ResourceType: "pods", Namespace: "n", Name: "x", Resource: json.RawMessage(`{"metadata":{"name":"x"}}`)}
	out, err := json.Marshal(ev)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"type":"ADDED","resourceType":"pods","namespace":"n","resource":{"metadata":{"name":"x"}}}`
	if string(out) != want {
		t.Fatalf("wire JSON = %s\nwant      %s", out, want)
	}
	batch, _ := json.Marshal([]ResourceEvent{ev, ev})
	if !strings.HasPrefix(string(batch), `[{"type":"ADDED"`) || strings.Contains(string(batch), `"name":"x"}`+`"`) {
		t.Fatalf("unexpected batch JSON: %s", batch)
	}
}

// crdWatchServer serves one scripted watch for example.com/v1 widgets.
func startCRDWatchLoop(t *testing.T, script func(w http.ResponseWriter, f http.Flusher, r *http.Request)) (*App, *eventLog) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/apis/example.com/v1/widgets" || r.URL.Query().Get("watch") != "true" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		f := w.(http.Flusher)
		f.Flush()
		script(w, f, r)
	}))
	t.Cleanup(srv.Close)

	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".kube"), 0o700); err != nil {
		t.Fatal(err)
	}
	kubeconfig := fmt.Sprintf("apiVersion: v1\nkind: Config\nclusters:\n- name: c\n  cluster:\n    server: %s\nusers:\n- name: u\n  user: {}\ncontexts:\n- name: ctx\n  context: {cluster: c, user: u}\ncurrent-context: ctx\n", srv.URL)
	if err := os.WriteFile(filepath.Join(home, ".kube", "config"), []byte(kubeconfig), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("KUBECONFIG", "")

	client, err := k8s.NewClient()
	if err != nil {
		t.Fatal(err)
	}
	log := &eventLog{}
	app := &App{k8sClient: client, eventStats: make(map[string]*WatcherEventStats)}
	app.emitter = events.EmitterFunc(log.emit)
	app.eventCoalescer = NewEventCoalescer(app, 5*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	app.watcherManager = NewResourceWatcherManager(ctx, app)
	key := "crd:example.com/v1/widgets:"
	app.watcherManager.watchers[key] = &ResourceWatcher{Key: key, RefCount: 1, Cancel: cancel}
	done := make(chan struct{})
	go func() {
		defer close(done)
		app.watchCRDLoop(ctx, "example.com", "v1", "widgets", "")
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("watchCRDLoop did not stop")
		}
	})
	return app, log
}

func TestWatchCRDLoopEmitsRawUnstructuredWithoutManagedFields(t *testing.T) {
	widget := func(name, rv string) string {
		return fmt.Sprintf(`{"apiVersion":"example.com/v1","kind":"Widget","metadata":{"name":%q,"namespace":"team-a","resourceVersion":%q,"managedFields":%s},"spec":{"replicas":3,"big":12345678901}}`, name, rv, managedFieldsJSON)
	}
	app, log := startCRDWatchLoop(t, func(w http.ResponseWriter, f http.Flusher, r *http.Request) {
		fmt.Fprintf(w, `{"type":"ADDED","object":%s}`+"\n", widget("w1", "11"))
		fmt.Fprintf(w, `{"type":"BOOKMARK","object":{"apiVersion":"example.com/v1","kind":"Widget","metadata":{"resourceVersion":"12"}}}`+"\n")
		fmt.Fprintf(w, `{"type":"MODIFIED","object":%s}`+"\n", widget("w2", "13"))
		f.Flush()
		<-r.Context().Done()
	})

	waitUntil(t, "two CRD events", 5*time.Second, func() bool { return len(log.snapshot()) >= 2 })
	got := log.snapshot()
	// Distinct names so the frame coalescer keeps both events; a batch is
	// built from a map, so order within it is unspecified.
	if len(got) != 2 {
		t.Fatalf("got %d events, want 2 (the bookmark must not be emitted)", len(got))
	}
	if got[0].Name != "w2" {
		got[0], got[1] = got[1], got[0]
	}
	if got[0].Type != "MODIFIED" || got[1].Type != "ADDED" {
		t.Fatalf("event types = %s, %s", got[0].Type, got[1].Type)
	}
	for _, e := range got {
		if e.ResourceType != "crd:example.com/v1/widgets" || e.Namespace != "team-a" {
			t.Fatalf("event envelope = %+v", e)
		}
		md := resourceMap(e)["metadata"].(map[string]interface{})
		if _, ok := md["managedFields"]; ok {
			t.Fatal("managedFields reached the frontend for a custom resource")
		}
	}
	spec := resourceMap(got[0])["spec"].(map[string]interface{})
	if spec["replicas"] != float64(3) {
		t.Fatalf("spec = %v", spec)
	}
	// Integers survive verbatim: no YAML or float64 detour on this path.
	if !strings.Contains(string(got[0].Resource), `12345678901`) {
		t.Fatalf("large integer mangled: %s", got[0].Resource)
	}
	app.watcherManager.mutex.RLock()
	rv := app.watcherManager.watchers["crd:example.com/v1/widgets:"].ResourceVersion
	app.watcherManager.mutex.RUnlock()
	if rv != "13" {
		t.Fatalf("stored resourceVersion = %q, want 13", rv)
	}
}

func TestWatchLoopStatsCountedPerWatcher(t *testing.T) {
	h := startWatchLoop(t, "", func(h *watchHarness, c *scriptedStream) {
		c.send("ADDED", podObject("a", "", "1"))
		c.send("MODIFIED", podObject("a", "", "2"))
		c.send("DELETED", podObject("a", "", "3"))
		c.block()
	})
	waitUntil(t, "delete", 5*time.Second, func() bool { return h.log.hasResource("DELETED", "a") })

	h.app.eventStatsMutex.RLock()
	s := h.app.eventStats["pods:"]
	h.app.eventStatsMutex.RUnlock()
	if s == nil {
		t.Fatal("no stats recorded for the watcher key")
	}
	if s.Added != 1 || s.Modified != 1 || s.Deleted != 1 || s.TotalEvents != 3 || s.LastEventMs == 0 {
		t.Fatalf("stats = %+v", *s)
	}
}
