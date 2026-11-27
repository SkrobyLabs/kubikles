package server

import (
	"embed"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// pendingBehaviour skips a test that documents desired behaviour not yet
// implemented. KUBIKLES_PENDING_TESTS=1 runs it anyway.
func pendingBehaviour(t *testing.T, reason string) {
	t.Helper()
	if os.Getenv("KUBIKLES_PENDING_TESTS") == "" {
		t.Skip(reason)
	}
}

// startBroadcastServer serves the WebSocket endpoint and runs the broadcast
// loop exactly as Run does, without binding a fixed port or needing assets.
func startBroadcastServer(t *testing.T) (*Server, string) {
	t.Helper()
	s := New(nil, embed.FS{}, 0)
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", s.handleWebSocket)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	t.Cleanup(func() {
		s.closeClients()
		s.wg.Wait()
	})
	return s, "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"
}

// dialClient connects and consumes the initial "connected" event so tests
// only see broadcast traffic afterwards.
func dialClient(t *testing.T, url string) *websocket.Conn {
	t.Helper()
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	var ev Event
	if err := conn.ReadJSON(&ev); err != nil || ev.Name != "connected" {
		t.Fatalf("expected connected event, got %+v err %v", ev, err)
	}
	return conn
}

func waitForClients(t *testing.T, s *Server, n int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		s.clientsMu.RLock()
		got := len(s.clients)
		s.clientsMu.RUnlock()
		if got == n {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("server never registered %d clients", n)
}

func TestBroadcastDeliversEveryEventToEveryClientInOrder(t *testing.T) {
	s, url := startBroadcastServer(t)
	const clients, events = 3, 200
	conns := make([]*websocket.Conn, clients)
	for i := range conns {
		conns[i] = dialClient(t, url)
	}
	waitForClients(t, s, clients)

	go func() {
		for i := 0; i < events; i++ {
			s.EmitEvent("tick", map[string]interface{}{"seq": i})
		}
	}()

	for ci, conn := range conns {
		_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
		for want := 0; want < events; want++ {
			var ev struct {
				Type string `json:"type"`
				Name string `json:"name"`
				Data struct {
					Seq int `json:"seq"`
				} `json:"data"`
			}
			if err := conn.ReadJSON(&ev); err != nil {
				t.Fatalf("client %d read event %d: %v", ci, want, err)
			}
			if ev.Type != "event" || ev.Name != "tick" || ev.Data.Seq != want {
				t.Fatalf("client %d event %d = %+v", ci, want, ev)
			}
		}
	}
}

func TestBroadcastWithNoClientsDoesNotBlock(t *testing.T) {
	s, _ := startBroadcastServer(t)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 500; i++ { // more than the channel buffer
			s.EmitEvent("tick", i)
		}
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("EmitEvent blocked with no clients connected")
	}
}

func TestBroadcastDisconnectedClientIsDroppedAndOthersContinue(t *testing.T) {
	s, url := startBroadcastServer(t)
	gone := dialClient(t, url)
	alive := dialClient(t, url)
	waitForClients(t, s, 2)
	_ = gone.Close()

	for i := 0; i < 20; i++ {
		s.EmitEvent("tick", i)
	}
	_ = alive.SetReadDeadline(time.Now().Add(5 * time.Second))
	for i := 0; i < 20; i++ {
		var ev Event
		if err := alive.ReadJSON(&ev); err != nil {
			t.Fatalf("event %d: %v", i, err)
		}
	}
}

// DESIRED behaviour: one client that stops reading (a hung browser tab, a
// suspended laptop) must not stop other clients from receiving events. Today
// handleBroadcast writes to clients serially with no write deadline, so the
// stalled client's full TCP buffer blocks the loop for everyone.
func TestBroadcastStalledClientDoesNotBlockOthers(t *testing.T) {
	s, url := startBroadcastServer(t)
	_ = dialClient(t, url) // connects and never reads again
	healthy := dialClient(t, url)
	waitForClients(t, s, 2)

	// Large payloads fill the stalled client's socket buffers quickly.
	payload := strings.Repeat("x", 256*1024)
	const events = 150
	go func() {
		for i := 0; i < events; i++ {
			s.EmitEvent("big", map[string]interface{}{"seq": i, "pad": payload})
		}
	}()

	_ = healthy.SetReadDeadline(time.Now().Add(15 * time.Second))
	for want := 0; want < events; want++ {
		var ev struct {
			Data struct {
				Seq int `json:"seq"`
			} `json:"data"`
		}
		if err := healthy.ReadJSON(&ev); err != nil {
			t.Fatalf("healthy client stalled at event %d of %d: %v", want, events, err)
		}
		if ev.Data.Seq != want {
			t.Fatalf("healthy client got seq %d, want %d", ev.Data.Seq, want)
		}
	}
}

type idleRecorder struct{ calls chan struct{} }

func (idleRecorder) OnClientDisconnect(string) {}
func (r idleRecorder) OnNoClients()            { r.calls <- struct{}{} }

func TestIdleListenerFiresOnlyAfterGraceWithNoClients(t *testing.T) {
	old := idleGrace
	idleGrace = 200 * time.Millisecond
	t.Cleanup(func() { idleGrace = old })

	s, url := startBroadcastServer(t)
	rec := idleRecorder{calls: make(chan struct{}, 4)}
	s.AddDisconnectListener(rec)

	// A reconnect inside the grace period cancels the call.
	first := dialClient(t, url)
	waitForClients(t, s, 1)
	_ = first.Close()
	waitForClients(t, s, 0)
	second := dialClient(t, url)
	waitForClients(t, s, 1)
	select {
	case <-rec.calls:
		t.Fatal("idle listener fired although a client reconnected within the grace period")
	case <-time.After(700 * time.Millisecond):
	}

	// With nobody left, it fires once.
	_ = second.Close()
	select {
	case <-rec.calls:
	case <-time.After(5 * time.Second):
		t.Fatal("idle listener never fired after the last client left")
	}
	select {
	case <-rec.calls:
		t.Fatal("idle listener fired twice")
	case <-time.After(700 * time.Millisecond):
	}
}

// A callback of a replaced or cancelled timer carries an old generation and must not fire the
// listeners, even when no client is connected at that moment.
func TestNotifyIdleIgnoresStaleGeneration(t *testing.T) {
	s, _ := startBroadcastServer(t)
	rec := idleRecorder{calls: make(chan struct{}, 4)}
	s.AddDisconnectListener(rec)

	s.clientsMu.Lock()
	s.idleGen = 5
	s.clientsMu.Unlock()

	s.notifyIdle(4)
	select {
	case <-rec.calls:
		t.Fatal("stale generation fired the idle listener")
	default:
	}

	s.notifyIdle(5)
	select {
	case <-rec.calls:
	default:
		t.Fatal("current generation with no clients must fire the idle listener")
	}
}

// A client that connects bumps the generation, so a timer armed before it cannot cancel its
// streams after it arrived.
func TestConnectInvalidatesPendingIdleTimer(t *testing.T) {
	old := idleGrace
	idleGrace = time.Hour
	t.Cleanup(func() { idleGrace = old })
	s, url := startBroadcastServer(t)
	rec := idleRecorder{calls: make(chan struct{}, 4)}
	s.AddDisconnectListener(rec)

	first := dialClient(t, url)
	waitForClients(t, s, 1)
	_ = first.Close()
	waitForClients(t, s, 0)
	s.clientsMu.Lock()
	armed := s.idleGen
	s.clientsMu.Unlock()

	second := dialClient(t, url)
	defer second.Close()
	waitForClients(t, s, 1)
	s.notifyIdle(armed)
	select {
	case <-rec.calls:
		t.Fatal("timer armed before a reconnect fired after it")
	default:
	}
}

func TestBrowserAddr(t *testing.T) {
	for in, want := range map[string]string{
		":8080":          "localhost:8080",
		"0.0.0.0:8080":   "localhost:8080",
		"127.0.0.1:9000": "127.0.0.1:9000",
		"10.1.2.3:80":    "10.1.2.3:80",
	} {
		if got := browserAddr(in); got != want {
			t.Errorf("browserAddr(%q) = %q, want %q", in, got, want)
		}
	}
}
