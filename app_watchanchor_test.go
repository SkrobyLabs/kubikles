package main

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// These tests cover subscribing a watcher from a list resourceVersion. The
// scripted API server records the resourceVersion each watch request carries,
// which is what decides whether the API server replays existing objects.

func subscribeHarness(t *testing.T, script ...func(h *watchHarness, c *scriptedStream)) (*App, *scriptedAPIServer, *eventLog) {
	t.Helper()
	app, srv, log := newScriptedApp(t, script)
	app.watcherManager = NewResourceWatcherManager(context.Background(), app)
	t.Cleanup(app.watcherManager.StopAll)
	return app, srv, log
}

func waitForConnections(t *testing.T, srv *scriptedAPIServer, n int) []watchConn {
	t.Helper()
	waitUntil(t, fmt.Sprintf("%d watch connections", n), 5*time.Second, func() bool { return len(srv.connections()) >= n })
	return srv.connections()
}

func TestSubscribeFromStartsWatchAfterListResourceVersion(t *testing.T) {
	app, srv, _ := subscribeHarness(t)
	if key := app.SubscribeResourceWatcherFrom("pods", "", "4711"); key != "pods:" {
		t.Fatalf("key = %q", key)
	}
	conns := waitForConnections(t, srv, 1)
	if conns[0].resourceVersion != "4711" {
		t.Fatalf("first watch resourceVersion = %q, want the list version 4711 (a replay would send none)", conns[0].resourceVersion)
	}
}

func TestSubscribeWithoutResourceVersionKeepsReplayingWatch(t *testing.T) {
	for name, subscribe := range map[string]func(a *App) string{
		"legacy binding":     func(a *App) string { return a.SubscribeResourceWatcher("pods", "") },
		"empty anchor":       func(a *App) string { return a.SubscribeResourceWatcherFrom("pods", "", "") },
		"zero is not anchor": func(a *App) string { return a.SubscribeResourceWatcherFrom("pods", "", "0") },
	} {
		t.Run(name, func(t *testing.T) {
			app, srv, _ := subscribeHarness(t)
			subscribe(app)
			if got := waitForConnections(t, srv, 1)[0].resourceVersion; got != "" {
				t.Fatalf("resourceVersion = %q, want none so existing objects are replayed", got)
			}
		})
	}
}

func TestAnchoredWatchDeliversOnlyEventsAfterTheList(t *testing.T) {
	// The server honours the anchor: it streams only a post-list change.
	app, srv, log := subscribeHarness(t, func(_ *watchHarness, c *scriptedStream) {
		c.send("MODIFIED", podObject("changed", "ns1", "4712"))
		c.block()
	})
	app.SubscribeResourceWatcherFrom("pods", "", "4711")
	waitUntil(t, "post-list event", 5*time.Second, func() bool { return log.hasResource("MODIFIED", "changed") })
	for _, e := range log.snapshot() {
		if e.Type == "ADDED" {
			t.Fatalf("unexpected replayed ADDED event for %s", resourceName(e))
		}
	}
	if got := srv.connections()[0].resourceVersion; got != "4711" {
		t.Fatalf("resourceVersion = %q", got)
	}
}

func TestSubscribeFromInStreamGoneFallsBackToUnanchoredWatch(t *testing.T) {
	app, srv, log := subscribeHarness(t,
		func(_ *watchHarness, c *scriptedStream) {
			c.send("ERROR", statusObject("Expired", 410))
		},
		func(_ *watchHarness, c *scriptedStream) {
			c.send("ADDED", podObject("replayed", "ns1", "9000"))
			c.block()
		},
	)
	app.SubscribeResourceWatcherFrom("pods", "", "1")
	conns := waitForConnections(t, srv, 2)
	if conns[0].resourceVersion != "1" || conns[1].resourceVersion != "" {
		t.Fatalf("resourceVersions = %q then %q, want 1 then none", conns[0].resourceVersion, conns[1].resourceVersion)
	}
	waitUntil(t, "replayed event after fallback", 5*time.Second, func() bool { return log.hasResource("ADDED", "replayed") })
	assertReconnectingBeforeSecondConnected(t, log)
}

// The frontend re-lists on reconnecting -> connected, which removes objects deleted during the
// gap, so a stale-anchor reopen must emit reconnecting between the two connected statuses.
func assertReconnectingBeforeSecondConnected(t *testing.T, log *eventLog) {
	t.Helper()
	waitUntil(t, "second connected status", 5*time.Second, func() bool {
		n := 0
		for _, s := range log.statusList() {
			if s.Status == "connected" {
				n++
			}
		}
		return n >= 2
	})
	var seq []string
	for _, s := range log.statusList() {
		seq = append(seq, s.Status)
	}
	connected, reconnecting := 0, false
	for _, st := range seq {
		switch st {
		case "connected":
			connected++
			if connected == 2 && !reconnecting {
				t.Fatalf("no reconnecting status between the two connections: %v", seq)
			}
		case "reconnecting":
			if connected == 1 || connected == 0 {
				reconnecting = true
			}
		}
	}
}

func TestSubscribeFromRejectedOpenFallsBackImmediately(t *testing.T) {
	for _, tc := range []struct {
		name   string
		code   int
		reason string
	}{
		{"gone", 410, "Gone"},
		{"expired", 410, "Expired"},
		{"invalid", 422, "Invalid"},
		{"bad request", 400, "BadRequest"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app, srv, log := subscribeHarness(t)
			srv.openFailure = map[int]openFailure{0: {tc.code, tc.reason}}
			started := time.Now()
			app.SubscribeResourceWatcherFrom("pods", "", "7")
			conns := waitForConnections(t, srv, 2)
			if conns[0].resourceVersion != "7" || conns[1].resourceVersion != "" {
				t.Fatalf("resourceVersions = %q then %q, want 7 then none", conns[0].resourceVersion, conns[1].resourceVersion)
			}
			// A stale anchor is not a connectivity failure: no exponential backoff.
			if elapsed := time.Since(started); elapsed > 900*time.Millisecond {
				t.Fatalf("fallback took %v, want an immediate retry", elapsed)
			}
			waitUntil(t, "reconnecting status", 5*time.Second, func() bool {
				for _, s := range log.statusList() {
					if s.Status == "reconnecting" {
						return true
					}
				}
				return false
			})
			log.mu.Lock()
			for _, e := range log.errors {
				t.Errorf("a stale anchor must not surface an error event: %+v", e)
			}
			log.mu.Unlock()
		})
	}
}

func TestSubscribeReusesRunningWatcherAndKeepsItsPosition(t *testing.T) {
	app, srv, _ := subscribeHarness(t)
	first := app.SubscribeResourceWatcherFrom("pods", "", "100")
	waitForConnections(t, srv, 1)
	// A second subscriber lists later and so has a newer version; the running
	// watcher already streams every change and must not be restarted or rewound.
	second := app.SubscribeResourceWatcherFrom("pods", "", "250")
	if first != second {
		t.Fatalf("keys differ: %q vs %q", first, second)
	}
	app.watcherManager.mutex.RLock()
	rw := app.watcherManager.watchers[first]
	app.watcherManager.mutex.RUnlock()
	if got := rw.RefCount; got != 2 {
		t.Fatalf("RefCount = %d, want 2", got)
	}
	time.Sleep(100 * time.Millisecond)
	if conns := srv.connections(); len(conns) != 1 {
		t.Fatalf("connections = %d, want 1: the second subscriber must not restart the watch", len(conns))
	}
	if rv := rwResourceVersion(rw); rv != "100" {
		t.Fatalf("stored resourceVersion = %q, want it untouched by the second subscriber", rv)
	}
	app.UnsubscribeWatcher(first)
	time.Sleep(50 * time.Millisecond)
	if len(srv.connections()) != 1 {
		t.Fatal("watch restarted after one of two subscribers left")
	}
}

func TestReusedWatcherInCleanupDelayIsNotRewound(t *testing.T) {
	app, srv, _ := subscribeHarness(t)
	key := app.SubscribeResourceWatcherFrom("pods", "", "10")
	waitForConnections(t, srv, 1)
	app.UnsubscribeWatcher(key) // refCount 0, cleanup pending
	app.SubscribeResourceWatcherFrom("pods", "", "20")
	app.watcherManager.mutex.RLock()
	rw := app.watcherManager.watchers[key]
	app.watcherManager.mutex.RUnlock()
	if rw.CleanupTimer != nil {
		t.Fatal("pending cleanup was not cancelled")
	}
	if rv := rwResourceVersion(rw); rv != "10" {
		t.Fatalf("stored resourceVersion = %q, want 10", rv)
	}
	if len(srv.connections()) != 1 {
		t.Fatalf("connections = %d, want 1", len(srv.connections()))
	}
}

func TestSubscribeCRDFromStartsWatchAfterListResourceVersion(t *testing.T) {
	app, srv, _ := subscribeHarness(t)
	key := app.SubscribeCRDWatcherFrom("example.io", "v1", "widgets", "ns", "88")
	if key != "crd:example.io/v1/widgets:ns" {
		t.Fatalf("key = %q", key)
	}
	if got := waitForConnections(t, srv, 1)[0].resourceVersion; got != "88" {
		t.Fatalf("CRD watch resourceVersion = %q, want 88", got)
	}

	app2, srv2, _ := subscribeHarness(t)
	app2.SubscribeCRDWatcher("example.io", "v1", "widgets", "ns")
	if got := waitForConnections(t, srv2, 1)[0].resourceVersion; got != "" {
		t.Fatalf("legacy CRD subscription resourceVersion = %q, want none", got)
	}
}

func TestSubscribeFromWithoutManagerReturnsEmptyKey(t *testing.T) {
	app := &App{}
	if app.SubscribeResourceWatcherFrom("pods", "", "1") != "" || app.SubscribeCRDWatcherFrom("g", "v", "r", "", "1") != "" {
		t.Fatal("expected empty keys without a watcher manager")
	}
}

func TestUsableAnchor(t *testing.T) {
	for rv, want := range map[string]bool{"": false, "0": false, "1": true, "123456": true} {
		if got := usableAnchor(rv); got != want {
			t.Errorf("usableAnchor(%q) = %v, want %v", rv, got, want)
		}
	}
}

func TestIsStaleResourceVersionError(t *testing.T) {
	gr := schema.GroupResource{Resource: "pods"}
	for name, tc := range map[string]struct {
		err  error
		want bool
	}{
		"gone":            {apierrors.NewGone("gone"), true},
		"expired":         {apierrors.NewResourceExpired("too old resource version"), true},
		"invalid":         {apierrors.NewInvalid(schema.GroupKind{Kind: "Pod"}, "x", nil), true},
		"bad request":     {apierrors.NewBadRequest("bad"), true},
		"message too old": {errors.New("the resourceVersion is too old"), true},
		"message expired": {errors.New("watch expired"), true},
		"forbidden":       {apierrors.NewForbidden(gr, "x", errors.New("no")), false},
		"network":         {errors.New("connection refused"), false},
	} {
		if got := isStaleResourceVersionError(tc.err); got != tc.want {
			t.Errorf("%s: got %v, want %v", name, got, tc.want)
		}
	}
}

func rwResourceVersion(rw *ResourceWatcher) string {
	rw.mu.RLock()
	defer rw.mu.RUnlock()
	return rw.ResourceVersion
}
