package server

import (
	"bytes"
	"compress/gzip"
	"embed"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestSlowClientIsDisconnectedAndOthersKeepReceiving(t *testing.T) {
	s, url := startBroadcastServer(t)
	stalled := dialClient(t, url)
	healthy := dialClient(t, url)
	waitForClients(t, s, 2)
	s.clientsMu.RLock()
	for c := range s.clients {
		if c.id == "ws-1" { // the first dialed, i.e. the stalled one
			c.maxQueued = 1 << 20 // small budget so overflow is quick
		}
	}
	s.clientsMu.RUnlock()

	payload := strings.Repeat("x", 128*1024)
	go func() {
		for i := 0; i < 100; i++ {
			s.EmitEvent("big", map[string]interface{}{"seq": i, "pad": payload})
		}
	}()
	_ = healthy.SetReadDeadline(time.Now().Add(15 * time.Second))
	for i := 0; i < 100; i++ {
		var ev Event
		if err := healthy.ReadJSON(&ev); err != nil {
			t.Fatalf("healthy read %d: %v", i, err)
		}
	}
	// The stalled client must eventually be unregistered.
	_ = stalled
	waitForClients(t, s, 1)
}

func TestEmitEventNeverBlocksOnStalledClient(t *testing.T) {
	s, url := startBroadcastServer(t)
	_ = dialClient(t, url)
	waitForClients(t, s, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		p := strings.Repeat("y", 256*1024)
		for i := 0; i < 400; i++ {
			s.EmitEvent("big", p)
		}
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("EmitEvent blocked on a stalled client")
	}
}

func TestShutdownClosesClientsWithoutGoroutineLeak(t *testing.T) {
	before := runtime.NumGoroutine()
	s := New(nil, embed.FS{}, 0)
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", s.handleWebSocket)
	ts := httptest.NewServer(mux)
	url := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"
	conns := []*websocket.Conn{dialClient(t, url), dialClient(t, url), dialClient(t, url)}
	waitForClients(t, s, 3)

	s.closeClients()
	s.wg.Wait()
	for _, c := range conns {
		_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
		if _, _, err := c.ReadMessage(); err == nil {
			t.Fatal("expected close after shutdown")
		}
		_ = c.Close()
	}
	ts.Close()
	// Late EmitEvent after shutdown must not panic or block.
	s.EmitEvent("late", 1)

	deadline := time.Now().Add(3 * time.Second)
	for runtime.NumGoroutine() > before+2 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if n := runtime.NumGoroutine(); n > before+2 {
		t.Fatalf("goroutines: before %d after %d", before, n)
	}
}

type bigCaller struct{ n int }

func (b bigCaller) CallMethod(string, []json.RawMessage) (interface{}, error) {
	return strings.Repeat("abc", b.n), nil
}

func TestAPIGzip(t *testing.T) {
	s := New(bigCaller{5000}, embed.FS{}, 0)
	do := func(enc string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/api/Big", strings.NewReader("[]"))
		if enc != "" {
			req.Header.Set("Accept-Encoding", enc)
		}
		rec := httptest.NewRecorder()
		s.handleAPI(rec, req)
		return rec
	}
	rec := do("gzip, deflate")
	if rec.Header().Get("Content-Encoding") != "gzip" || !strings.Contains(rec.Header().Get("Vary"), "Accept-Encoding") {
		t.Fatalf("headers: %v", rec.Header())
	}
	zr, err := gzip.NewReader(bytes.NewReader(rec.Body.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	plain, _ := io.ReadAll(zr)
	if !strings.Contains(string(plain), `"data":"abcabc`) {
		t.Fatalf("bad body %q", plain[:40])
	}
	for _, enc := range []string{"", "gzip;q=0", "identity"} {
		r := do(enc)
		if r.Header().Get("Content-Encoding") != "" || !strings.Contains(r.Body.String(), `"data":"abc`) {
			t.Fatalf("enc %q should be plain", enc)
		}
	}
	// Small responses are not compressed.
	s2 := New(bigCaller{2}, embed.FS{}, 0)
	req := httptest.NewRequest("POST", "/api/Big", strings.NewReader("[]"))
	req.Header.Set("Accept-Encoding", "gzip")
	rec2 := httptest.NewRecorder()
	s2.handleAPI(rec2, req)
	if rec2.Header().Get("Content-Encoding") != "" {
		t.Fatal("small body compressed")
	}
}

func TestAcceptsGzip(t *testing.T) {
	cases := map[string]bool{"gzip": true, "br, gzip;q=0.5": true, "gzip;q=0": false, "": false, "*": true, "deflate": false}
	for in, want := range cases {
		if got := acceptsGzip(in); got != want {
			t.Errorf("%q = %v", in, got)
		}
	}
}

func TestEnqueueAcceptsSingleOversizedMessageWhenIdle(t *testing.T) {
	c := &wsClient{
		send:      make(chan queuedMessage, 4),
		done:      make(chan struct{}),
		maxQueued: 10,
	}
	if !c.enqueue(queuedMessage{size: 1000}) {
		t.Fatal("an oversized message on an empty queue must be accepted")
	}
	if got := c.queued.Load(); got != 1000 {
		t.Fatalf("queued = %d", got)
	}
}
