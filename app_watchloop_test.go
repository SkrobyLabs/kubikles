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
	"sync"
	"testing"
	"time"

	"kubikles/pkg/events"
	"kubikles/pkg/k8s"
)

// The watch loop is driven through the real client-go REST stack against an
// httptest API server. k8s.Client keeps its clientset private and main cannot
// inject a fake one, so a kubeconfig under a temporary HOME is the most
// realistic seam available without editing production code. The server
// scripts each watch connection, so reconnect behaviour (the resourceVersion
// sent on each request) can be asserted precisely.

// watchConn is one scripted watch connection.
type watchConn struct {
	resourceVersion string
	query           string
}

type scriptedAPIServer struct {
	t      *testing.T
	mu     sync.Mutex
	conns  []watchConn
	script []func(h *watchHarness, c *scriptedStream)
	srv    *httptest.Server
	h      *watchHarness // set before the loop starts, so script closures can read it
	// openFailure, when set, answers the watch connection with this HTTP status
	// (and a matching metav1.Status body) instead of opening a stream.
	openFailure map[int]openFailure
}

type openFailure struct {
	code   int
	reason string
}

type scriptedStream struct {
	w       http.ResponseWriter
	f       http.Flusher
	r       *http.Request
	enc     *json.Encoder
	connIdx int
}

func (s *scriptedStream) send(eventType string, obj map[string]interface{}) {
	_ = s.enc.Encode(map[string]interface{}{"type": eventType, "object": obj})
	s.f.Flush()
}

// block keeps the stream open until the client disconnects.
func (s *scriptedStream) block() { <-s.r.Context().Done() }

func podObject(name, namespace, rv string) map[string]interface{} {
	meta := map[string]interface{}{"name": name, "resourceVersion": rv}
	if namespace != "" {
		meta["namespace"] = namespace
	}
	return map[string]interface{}{"apiVersion": "v1", "kind": "Pod", "metadata": meta}
}

func statusObject(reason string, code int) map[string]interface{} {
	return map[string]interface{}{
		"apiVersion": "v1", "kind": "Status", "status": "Failure",
		"reason": reason, "code": code, "message": reason + " test status",
	}
}

func (s *scriptedAPIServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Pod and custom resource watches share the script; paths are not checked.
	if r.URL.Query().Get("watch") != "true" {
		http.NotFound(w, r)
		return
	}
	s.mu.Lock()
	idx := len(s.conns)
	s.conns = append(s.conns, watchConn{resourceVersion: r.URL.Query().Get("resourceVersion"), query: r.URL.RawQuery})
	var fn func(h *watchHarness, c *scriptedStream)
	if idx < len(s.script) {
		fn = s.script[idx]
	}
	failure, failing := s.openFailure[idx]
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	if failing {
		w.WriteHeader(failure.code)
		_ = json.NewEncoder(w).Encode(statusObject(failure.reason, failure.code))
		return
	}
	w.WriteHeader(http.StatusOK)
	f := w.(http.Flusher)
	f.Flush()
	st := &scriptedStream{w: w, f: f, r: r, enc: json.NewEncoder(w), connIdx: idx}
	if fn == nil {
		st.block()
		return
	}
	fn(s.h, st)
}

func (s *scriptedAPIServer) connections() []watchConn {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]watchConn(nil), s.conns...)
}

// eventLog records what the App emitted to the frontend.
type eventLog struct {
	mu        sync.Mutex
	resources []ResourceEvent
	statuses  []WatcherStatusEvent
	errors    []WatcherErrorEvent
}

func (l *eventLog) emit(name string, data ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, d := range data {
		switch v := d.(type) {
		case ResourceEvent:
			l.resources = append(l.resources, v)
		case []ResourceEvent:
			l.resources = append(l.resources, v...)
		case WatcherStatusEvent:
			l.statuses = append(l.statuses, v)
		case WatcherErrorEvent:
			l.errors = append(l.errors, v)
		}
	}
}

func (l *eventLog) snapshot() []ResourceEvent {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]ResourceEvent(nil), l.resources...)
}

func (l *eventLog) statusList() []WatcherStatusEvent {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]WatcherStatusEvent(nil), l.statuses...)
}

func (l *eventLog) hasResource(eventType, name string) bool {
	for _, e := range l.snapshot() {
		if e.Type == eventType && resourceName(e) == name {
			return true
		}
	}
	return false
}

func resourceName(e ResourceEvent) string {
	md, _ := resourceMap(e)["metadata"].(map[string]interface{})
	n, _ := md["name"].(string)
	return n
}

func waitUntil(t *testing.T, what string, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

type watchHarness struct {
	app    *App
	server *scriptedAPIServer
	log    *eventLog
	rw     *ResourceWatcher
	cancel context.CancelFunc
	done   chan struct{}
}

func (h *watchHarness) storedRV() string {
	h.rw.mu.RLock()
	defer h.rw.mu.RUnlock()
	return h.rw.ResourceVersion
}

func (h *watchHarness) setStoredRV(rv string) {
	h.rw.mu.Lock()
	h.rw.ResourceVersion = rv
	h.rw.mu.Unlock()
}

// newScriptedApp builds an App whose Kubernetes client talks to a scripted API
// server, with a real event coalescer and a capturing emitter.
func newScriptedApp(t *testing.T, script []func(h *watchHarness, c *scriptedStream)) (*App, *scriptedAPIServer, *eventLog) {
	t.Helper()
	srv := &scriptedAPIServer{t: t, script: script}
	srv.srv = httptest.NewServer(srv)
	t.Cleanup(srv.srv.Close)

	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".kube"), 0o700); err != nil {
		t.Fatal(err)
	}
	kubeconfig := fmt.Sprintf(`apiVersion: v1
kind: Config
clusters:
- name: c
  cluster:
    server: %s
users:
- name: u
  user: {}
contexts:
- name: ctx
  context:
    cluster: c
    user: u
current-context: ctx
`, srv.srv.URL)
	if err := os.WriteFile(filepath.Join(home, ".kube", "config"), []byte(kubeconfig), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("KUBECONFIG", "")

	client, err := k8s.NewClient()
	if err != nil {
		t.Fatalf("k8s.NewClient: %v", err)
	}

	log := &eventLog{}
	app := &App{}
	app.k8sClient = client
	app.eventStats = make(map[string]*WatcherEventStats)
	app.emitter = events.EmitterFunc(log.emit)
	app.eventCoalescer = NewEventCoalescer(app, 5*time.Millisecond)

	return app, srv, log
}

// startWatchLoop wires an App to a scripted API server and starts
// watchResourceLoop for ("pods", namespace). Connections beyond the script
// block until the client disconnects.
func startWatchLoop(t *testing.T, namespace string, script ...func(h *watchHarness, c *scriptedStream)) *watchHarness {
	t.Helper()
	app, srv, log := newScriptedApp(t, script)
	ctx, cancel := context.WithCancel(context.Background())
	app.watcherManager = NewResourceWatcherManager(ctx, app)
	rw := &ResourceWatcher{Key: "pods:" + namespace, ResourceType: "pods", Namespace: namespace, RefCount: 1, Cancel: cancel}
	app.watcherManager.watchers[rw.Key] = rw

	h := &watchHarness{app: app, server: srv, log: log, rw: rw, cancel: cancel, done: make(chan struct{})}
	srv.h = h
	go func() {
		defer close(h.done)
		app.watchResourceLoop(ctx, "pods", namespace)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-h.done:
		case <-time.After(5 * time.Second):
			t.Error("watchResourceLoop did not stop after cancellation")
		}
	})
	return h
}

func TestWatchLoopForwardsAddedModifiedDeleted(t *testing.T) {
	waitFor := func(c *scriptedStream, cond func() bool) {
		deadline := time.Now().Add(5 * time.Second)
		for !cond() && time.Now().Before(deadline) {
			time.Sleep(2 * time.Millisecond)
		}
	}
	h := startWatchLoop(t, "", func(h *watchHarness, c *scriptedStream) {
		c.send("ADDED", podObject("a", "ns1", "10"))
		waitFor(c, func() bool { return h.log.hasResource("ADDED", "a") })
		c.send("MODIFIED", podObject("a", "ns1", "11"))
		waitFor(c, func() bool { return h.log.hasResource("MODIFIED", "a") })
		c.send("ADDED", podObject("b", "ns2", "12"))
		waitFor(c, func() bool { return h.log.hasResource("ADDED", "b") })
		c.send("DELETED", podObject("a", "ns1", "13"))
		c.block()
	})

	waitUntil(t, "DELETED a", 5*time.Second, func() bool { return h.log.hasResource("DELETED", "a") })

	got := h.log.snapshot()
	type row struct{ typ, name, ns string }
	var rows []row
	for _, e := range got {
		if e.ResourceType != "pods" {
			t.Fatalf("ResourceType = %q, want pods", e.ResourceType)
		}
		rows = append(rows, row{e.Type, resourceName(e), e.Namespace})
	}
	want := []row{
		{"ADDED", "a", "ns1"},
		{"MODIFIED", "a", "ns1"},
		{"ADDED", "b", "ns2"},
		{"DELETED", "a", "ns1"},
	}
	if len(rows) != len(want) {
		t.Fatalf("events = %v, want %v", rows, want)
	}
	for i := range want {
		if rows[i] != want[i] {
			t.Fatalf("event %d = %v, want %v (all: %v)", i, rows[i], want[i], rows)
		}
	}
	if rv := h.storedRV(); rv != "13" {
		t.Fatalf("stored resourceVersion = %q, want 13", rv)
	}

	// Typed watch objects carry no kind/apiVersion after decoding. The frontend
	// receives them like this today.
	res := resourceMap(got[0])
	if _, ok := res["kind"]; ok {
		t.Fatalf("typed watch object unexpectedly has kind: %v", res["kind"])
	}
	if _, ok := res["apiVersion"]; ok {
		t.Fatalf("typed watch object unexpectedly has apiVersion: %v", res["apiVersion"])
	}
}

func TestWatchLoopFirstRequestParameters(t *testing.T) {
	h := startWatchLoop(t, "")
	waitUntil(t, "first connection", 5*time.Second, func() bool { return len(h.server.connections()) > 0 })
	c := h.server.connections()[0]
	if c.resourceVersion != "" {
		t.Fatalf("first request must not carry a resourceVersion, got %q", c.resourceVersion)
	}
	for _, want := range []string{"watch=true", "allowWatchBookmarks=true", "timeoutSeconds=300"} {
		if !containsParam(c.query, want) {
			t.Fatalf("query %q is missing %s", c.query, want)
		}
	}
	waitUntil(t, "connected status", 5*time.Second, func() bool {
		for _, s := range h.log.statusList() {
			if s.Status == "connected" && s.ResourceType == "pods" {
				return true
			}
		}
		return false
	})
}

func containsParam(query, param string) bool {
	for _, p := range strings.Split(query, "&") {
		if p == param {
			return true
		}
	}
	return false
}

func TestWatchLoopBookmarkUpdatesResourceVersionWithoutEmitting(t *testing.T) {
	h := startWatchLoop(t, "", func(h *watchHarness, c *scriptedStream) {
		c.send("ADDED", podObject("a", "ns1", "10"))
		c.send("BOOKMARK", podObject("", "", "500"))
		// A sentinel after the bookmark proves the bookmark was consumed
		// before the stream moved on.
		c.send("ADDED", podObject("sentinel", "ns1", "501"))
		c.block()
	})
	waitUntil(t, "sentinel", 5*time.Second, func() bool { return h.log.hasResource("ADDED", "sentinel") })

	if rv := h.storedRV(); rv != "501" {
		t.Fatalf("stored resourceVersion = %q, want 501", rv)
	}
	for _, e := range h.log.snapshot() {
		if e.Type == "BOOKMARK" {
			t.Fatal("BOOKMARK must not be forwarded")
		}
		if resourceName(e) == "" {
			t.Fatalf("bookmark object leaked as an event: %+v", e)
		}
	}
	if n := len(h.log.snapshot()); n != 2 {
		t.Fatalf("emitted %d events, want 2 (a, sentinel)", n)
	}
}

func TestWatchLoopBookmarkAloneAdvancesResourceVersion(t *testing.T) {
	h := startWatchLoop(t, "", func(h *watchHarness, c *scriptedStream) {
		c.send("BOOKMARK", podObject("", "", "777"))
		c.block()
	})
	waitUntil(t, "resourceVersion 777", 5*time.Second, func() bool { return h.storedRV() == "777" })
	time.Sleep(40 * time.Millisecond) // allow a (wrong) flush to show up
	if n := len(h.log.snapshot()); n != 0 {
		t.Fatalf("bookmark produced %d events", n)
	}
}

func TestWatchLoopErrorEventResetsResourceVersionWhenExpiredOrGone(t *testing.T) {
	for _, tc := range []struct {
		name   string
		reason string
		code   int
		wantRV string
	}{
		{"expired", "Expired", 410, ""},
		{"gone", "Gone", 410, ""},
		{"other errors keep the version", "InternalError", 500, "10"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := startWatchLoop(t, "", func(h *watchHarness, c *scriptedStream) {
				c.send("ADDED", podObject("a", "ns1", "10"))
				deadline := time.Now().Add(5 * time.Second)
				for h.storedRV() != "10" && time.Now().Before(deadline) {
					time.Sleep(2 * time.Millisecond)
				}
				c.send("ERROR", statusObject(tc.reason, tc.code))
			})
			waitUntil(t, "second connection", 8*time.Second, func() bool { return len(h.server.connections()) >= 2 })

			conns := h.server.connections()
			if conns[0].resourceVersion != "" {
				t.Fatalf("first connection resourceVersion = %q", conns[0].resourceVersion)
			}
			if conns[1].resourceVersion != tc.wantRV {
				t.Fatalf("reconnect resourceVersion = %q, want %q", conns[1].resourceVersion, tc.wantRV)
			}
			if got := h.storedRV(); got != tc.wantRV {
				t.Fatalf("stored resourceVersion = %q, want %q", got, tc.wantRV)
			}
			for _, e := range h.log.snapshot() {
				if e.Type == "ERROR" {
					t.Fatal("ERROR events must not be forwarded as resource events")
				}
			}
		})
	}
}

func TestWatchLoopReconnectResumesFromLastResourceVersion(t *testing.T) {
	h := startWatchLoop(t, "",
		func(h *watchHarness, c *scriptedStream) {
			c.send("ADDED", podObject("a", "ns1", "10"))
			c.send("MODIFIED", podObject("a", "ns1", "11"))
			// Returning ends the stream: the loop sees a closed channel.
			time.Sleep(50 * time.Millisecond)
		},
		func(h *watchHarness, c *scriptedStream) {
			c.send("MODIFIED", podObject("a", "ns1", "12"))
			c.block()
		},
	)
	// The loop waits one second before reconnecting after a healthy stream ends.
	waitUntil(t, "second connection", 10*time.Second, func() bool { return len(h.server.connections()) >= 2 })
	conns := h.server.connections()
	if conns[1].resourceVersion != "11" {
		t.Fatalf("reconnect resourceVersion = %q, want 11", conns[1].resourceVersion)
	}
	waitUntil(t, "resourceVersion 12", 5*time.Second, func() bool { return h.storedRV() == "12" })

	sawReconnecting := false
	for _, s := range h.log.statusList() {
		if s.Status == "reconnecting" {
			sawReconnecting = true
		}
	}
	if !sawReconnecting {
		t.Fatal("expected a reconnecting status event")
	}
	if !h.log.hasResource("MODIFIED", "a") {
		t.Fatal("events after reconnect must still be forwarded")
	}
}

func TestWatchLoopEventNamespaceComesFromObject(t *testing.T) {
	// A cluster-wide watcher (empty namespace) still labels each event with the object's own namespace.
	h := startWatchLoop(t, "", func(h *watchHarness, c *scriptedStream) {
		c.send("ADDED", podObject("cluster-wide", "other-ns", "1"))
		c.block()
	})
	waitUntil(t, "event", 5*time.Second, func() bool { return h.log.hasResource("ADDED", "cluster-wide") })
	if got := h.log.snapshot()[0].Namespace; got != "other-ns" {
		t.Fatalf("Namespace = %q, want other-ns", got)
	}
}

func TestWatchLoopStopsAndReportsStoppedOnCancel(t *testing.T) {
	h := startWatchLoop(t, "")
	waitUntil(t, "connection", 5*time.Second, func() bool { return len(h.server.connections()) > 0 })
	h.cancel()
	select {
	case <-h.done:
	case <-time.After(5 * time.Second):
		t.Fatal("loop did not exit")
	}
	last := h.log.statusList()
	if len(last) == 0 || last[len(last)-1].Status != "stopped" {
		t.Fatalf("last status = %+v, want stopped", last)
	}
}
