package server

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

const acceleratorTestCreatorToken = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

type acceleratorWebSocketFixture struct {
	registry *AcceleratorSessionRegistry
	creator  *CreatorAuthenticator
	server   *Server
	http     *httptest.Server
	url      string
}

func newAcceleratorWebSocketFixture(t *testing.T) *acceleratorWebSocketFixture {
	t.Helper()
	token, err := ParseCreatorToken(acceleratorTestCreatorToken)
	if err != nil {
		t.Fatal(err)
	}
	creator, err := NewCreatorAuthenticator(DeriveCreatorVerifier(token).Encoded())
	if err != nil {
		t.Fatal(err)
	}
	registry := newAcceleratorSessionRegistry("accel-test", &recordingAcceleratorObserver{}, acceleratorTestConfig())
	options := AcceleratorOptions(0, nil, creator.Guard)
	options.AcceleratorSessions = registry
	options.AcceleratorWebSocketAuthenticator = &AcceleratorWebSocketAuthenticator{Creator: creator, Registry: registry}
	server, err := NewWithOptions(nil, embed.FS{}, options)
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server.Handler())
	fixture := &acceleratorWebSocketFixture{registry: registry, creator: creator, server: server, http: httpServer, url: "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws"}
	t.Cleanup(func() {
		httpServer.Close()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = registry.Close(ctx)
	})
	return fixture
}

func (f *acceleratorWebSocketFixture) creatorHeader() http.Header {
	return http.Header{"Authorization": []string{"Bearer " + acceleratorTestCreatorToken}}
}

func dialCreator(t *testing.T, rawURL string, header http.Header) (*websocket.Conn, *http.Response, error) {
	t.Helper()
	return (&websocket.Dialer{HandshakeTimeout: time.Second}).Dial(rawURL, header)
}

func requireCreatorRejected(t *testing.T, rawURL string, header http.Header) {
	t.Helper()
	connection, response, err := dialCreator(t, rawURL, header)
	if connection != nil {
		_ = connection.Close()
	}
	if response != nil && response.Body != nil {
		defer response.Body.Close()
	}
	if err == nil || response == nil || response.StatusCode == http.StatusSwitchingProtocols {
		t.Fatalf("dial unexpectedly upgraded: conn=%v response=%v err=%v", connection != nil, response, err)
	}
}

func TestAcceleratorWebSocketCreatorAuthenticationMatrix(t *testing.T) {
	fixture := newAcceleratorWebSocketFixture(t)
	valid := fixture.creatorHeader()
	valid.Set("Origin", strings.Replace(strings.TrimSuffix(fixture.url, "/ws"), "ws", "http", 1))
	connection, response, err := dialCreator(t, fixture.url, valid)
	if err != nil {
		t.Fatal(err)
	}
	if response != nil {
		response.Body.Close()
	}
	_ = connection.Close()

	for name, header := range map[string]http.Header{
		"missing authorization":  nil,
		"wrong bearer":           {"Authorization": []string{"Bearer BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB"}},
		"multiple authorization": {"Authorization": []string{"Bearer " + acceleratorTestCreatorToken, "Bearer " + acceleratorTestCreatorToken}},
		"cookie":                 {"Authorization": []string{"Bearer " + acceleratorTestCreatorToken}, "Cookie": []string{"session=hidden"}},
		"subprotocol":            {"Authorization": []string{"Bearer " + acceleratorTestCreatorToken}, "Sec-WebSocket-Protocol": []string{"credential.hidden"}},
	} {
		t.Run(name, func(t *testing.T) { requireCreatorRejected(t, fixture.url, header) })
	}
	requireCreatorRejected(t, fixture.url+"?token=hidden", fixture.creatorHeader())
	// A syntactically bare query remains rejected: its presence must not widen the
	// credential surface or become a session transport.
	requireCreatorRejected(t, fixture.url+"?", fixture.creatorHeader())
	requireCreatorRejected(t, strings.TrimSuffix(fixture.url, "/ws")+"/api/ws", fixture.creatorHeader())
}

func readCreatorConnected(t *testing.T, connection *websocket.Conn) ([]byte, AcceleratorConnectedEvent) {
	t.Helper()
	_ = connection.SetReadDeadline(time.Now().Add(time.Second))
	messageType, raw, err := connection.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if messageType != websocket.TextMessage {
		t.Fatalf("connected frame type = %d", messageType)
	}
	var envelope struct {
		Type string                    `json:"type"`
		Name string                    `json:"name"`
		Data AcceleratorConnectedEvent `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Type != "event" || envelope.Name != "connected" {
		t.Fatalf("first frame = %s", raw)
	}
	return raw, envelope.Data
}

func TestAcceleratorWebSocketCreatorFirstFrameAndGeneration(t *testing.T) {
	fixture := newAcceleratorWebSocketFixture(t)
	dial := func() *websocket.Conn {
		connection, response, err := dialCreator(t, fixture.url, fixture.creatorHeader())
		if err != nil {
			t.Fatal(err)
		}
		if response != nil {
			response.Body.Close()
		}
		return connection
	}
	first := dial()
	raw, connected := readCreatorConnected(t, first)
	if connected.Generation != 1 || connected.Resumed || connected.InstanceID != "accel-test" || connected.SessionID != fixture.creator.context.SessionID {
		t.Fatalf("first connected = %+v", connected)
	}
	want, _ := json.Marshal(Event{Type: "event", Name: "connected", Data: connected})
	if !bytes.Equal(raw, want) {
		t.Fatalf("connected frame = %s, want %s", raw, want)
	}
	_ = first.Close()
	waitAccelerator(t, "clean disconnect", func() bool {
		_, active, found := fixture.registry.snapshot(fixture.creator.context.SessionID)
		return found && !active
	})
	second := dial()
	_, connected = readCreatorConnected(t, second)
	if connected.Generation != 2 || !connected.Resumed {
		t.Fatalf("reconnect = %+v", connected)
	}
	_ = second.Close()
}
