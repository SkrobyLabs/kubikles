package acceleratorprovision

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"kubikles/pkg/agent"
	"kubikles/pkg/server"
)

const knownCreatorToken = "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8"

type orderedConnectedFrameSocket struct {
	firstDeadlineEntered chan struct{}
	releaseFirstDeadline chan struct{}
	mu                   sync.Mutex
	deadlines            []time.Time
	readCalled           atomic.Bool
	closed               atomic.Bool
}

func (s *orderedConnectedFrameSocket) SetReadLimit(int64) {}
func (s *orderedConnectedFrameSocket) SetReadDeadline(deadline time.Time) error {
	s.mu.Lock()
	index := len(s.deadlines)
	s.deadlines = append(s.deadlines, deadline)
	s.mu.Unlock()
	if index == 0 {
		close(s.firstDeadlineEntered)
		<-s.releaseFirstDeadline
	}
	return nil
}
func (s *orderedConnectedFrameSocket) ReadMessage() (int, []byte, error) {
	s.readCalled.Store(true)
	return websocket.TextMessage, []byte(`{"type":"event","name":"connected","data":{"sessionId":"session-a","instanceId":"instance-a","generation":1,"resumed":false}}`), nil
}
func (s *orderedConnectedFrameSocket) Close() error {
	s.closed.Store(true)
	return nil
}

func TestValidateConnectedInstallsDeadlineBeforeCancellationWake(t *testing.T) {
	socket := &orderedConnectedFrameSocket{firstDeadlineEntered: make(chan struct{}), releaseFirstDeadline: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := validateConnected(ctx, socket, "instance-a")
		result <- err
	}()
	<-socket.firstDeadlineEntered
	cancel()
	close(socket.releaseFirstDeadline)
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("cancellation raced through as a connected frame")
		}
	case <-time.After(time.Second):
		t.Fatal("cancellation did not wake the connected-frame read")
	}
	if socket.readCalled.Load() || !socket.closed.Load() {
		t.Fatalf("read=%t closed=%t", socket.readCalled.Load(), socket.closed.Load())
	}
	socket.mu.Lock()
	defer socket.mu.Unlock()
	if len(socket.deadlines) != 1 || socket.deadlines[0].IsZero() {
		t.Fatalf("deadline order=%v", socket.deadlines)
	}
}

type blockingConnectedFrameSocket struct {
	readStarted        chan struct{}
	closed             chan struct{}
	closeOnce          sync.Once
	inRead             atomic.Bool
	deadlineDuringRead atomic.Bool
	deadlines          atomic.Int32
}

func (*blockingConnectedFrameSocket) SetReadLimit(int64) {}
func (s *blockingConnectedFrameSocket) SetReadDeadline(time.Time) error {
	s.deadlines.Add(1)
	if s.inRead.Load() {
		s.deadlineDuringRead.Store(true)
	}
	return nil
}
func (s *blockingConnectedFrameSocket) ReadMessage() (int, []byte, error) {
	s.inRead.Store(true)
	close(s.readStarted)
	<-s.closed
	s.inRead.Store(false)
	return 0, nil, errors.New("closed")
}
func (s *blockingConnectedFrameSocket) Close() error {
	s.closeOnce.Do(func() { close(s.closed) })
	return nil
}

func TestValidateConnectedCancellationClosesBlockingRead(t *testing.T) {
	socket := &blockingConnectedFrameSocket{readStarted: make(chan struct{}), closed: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := validateConnected(ctx, socket, "instance-a")
		result <- err
	}()
	<-socket.readStarted
	cancel()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("cancelled blocking read returned success")
		}
	case <-time.After(time.Second):
		t.Fatal("Close did not wake the blocking read")
	}
	if socket.deadlineDuringRead.Load() || socket.deadlines.Load() != 1 {
		t.Fatalf("deadlines=%d concurrent=%t", socket.deadlines.Load(), socket.deadlineDuringRead.Load())
	}
}

func knownCredential(t *testing.T) *creatorCredential {
	t.Helper()
	entropy := make([]byte, 48)
	for index := range entropy {
		entropy[index] = byte(index)
	}
	credential, err := generateCreatorCredential(bytes.NewReader(entropy))
	if err != nil {
		t.Fatal(err)
	}
	return credential
}

func endpointOf(t *testing.T, rawURL string) string {
	t.Helper()
	return strings.TrimPrefix(rawURL, "http://")
}

func requireCreatorRequest(t *testing.T, request *http.Request, method, requestPath string) {
	t.Helper()
	if request.Method != method || request.URL.Path != requestPath || request.URL.RawQuery != "" || request.Host == "" || len(request.Header.Values("Origin")) != 0 || len(request.Header.Values("Cookie")) != 0 {
		t.Errorf("request boundary mismatch: %s %#v", request.Method, request.URL)
	}
	values := request.Header.Values("Authorization")
	if len(values) != 1 || values[0] != "Bearer "+knownCreatorToken {
		t.Errorf("authorization mismatch")
	}
}

func TestAuthenticatedInfoExactContract(t *testing.T) {
	info := server.AuthenticatedAcceleratorInfo{Runtime: "accelerator", Build: agent.BuildIdentity{BuildVersion: "v1.2.3", Commit: "reporting", Dirty: true}, InstanceID: "instance-a", Capabilities: agent.V1Capabilities(), CapabilityDiagnostics: []agent.CapabilityDiagnostic{}}
	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requireCreatorRequest(t, request, http.MethodGet, "/api/accelerator-info")
		if request.Header.Get("Accept-Encoding") != "identity" {
			t.Errorf("compression was not disabled")
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(writer).Encode(info)
	})
	httpServer := httptest.NewServer(handler)
	defer httpServer.Close()
	credential := knownCredential(t)
	var got server.AuthenticatedAcceleratorInfo
	err := credential.withCreatorAuthorization(context.Background(), func(ctx context.Context, authorization creatorAuthorizationLease) error {
		var fetchErr error
		got, fetchErr = fetchInfo(ctx, endpointOf(t, httpServer.URL), authorization)
		return fetchErr
	})
	if err != nil || !validInfo(got, "v1.2.3", "v1.2.3") {
		t.Fatalf("exact info rejected: %#v %v", got, err)
	}

	cases := []struct {
		name        string
		status      int
		contentType string
		cache       string
		body        string
	}{
		{"unauthorized", 401, "application/json", "no-store", `{"error":"unauthorized"}`},
		{"redirect", 302, "application/json", "no-store", `{}`},
		{"wrong content type", 200, "text/plain", "no-store", `{}`},
		{"content type parameter", 200, "application/json; charset=utf-8", "no-store", `{}`},
		{"cache", 200, "application/json", "private", `{}`},
		{"unknown", 200, "application/json", "no-store", `{"runtime":"accelerator","build":{"buildVersion":"v1.2.3","commit":"","dirty":false},"instanceId":"i","capabilities":[],"capabilityDiagnostics":[],"extra":1}`},
		{"duplicate", 200, "application/json", "no-store", `{"runtime":"accelerator","runtime":"accelerator","build":{"buildVersion":"v1.2.3","commit":"","dirty":false},"instanceId":"i","capabilities":[],"capabilityDiagnostics":[]}`},
		{"trailing", 200, "application/json", "no-store", `{}` + `{}`},
		{"oversized", 200, "application/json", "no-store", strings.Repeat("x", infoBodyLimit+1)},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			bad := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", test.contentType)
				writer.Header().Set("Cache-Control", test.cache)
				writer.WriteHeader(test.status)
				_, _ = writer.Write([]byte(test.body))
			}))
			defer bad.Close()
			err := credential.withCreatorAuthorization(context.Background(), func(ctx context.Context, authorization creatorAuthorizationLease) error {
				_, fetchErr := fetchInfo(ctx, endpointOf(t, bad.URL), authorization)
				return fetchErr
			})
			if err == nil {
				t.Fatal("invalid info accepted")
			}
			if strings.Contains(err.Error(), knownCreatorToken) || strings.Contains(err.Error(), endpointOf(t, bad.URL)) {
				t.Fatalf("unsafe error: %v", err)
			}
		})
	}

	mutations := []server.AuthenticatedAcceleratorInfo{
		{Runtime: "ordinary", Build: info.Build, InstanceID: info.InstanceID, Capabilities: info.Capabilities},
		{Runtime: info.Runtime, Build: agent.BuildIdentity{BuildVersion: "v1.2.4"}, InstanceID: info.InstanceID, Capabilities: info.Capabilities},
		{Runtime: info.Runtime, Build: info.Build, InstanceID: "", Capabilities: info.Capabilities},
		{Runtime: info.Runtime, Build: info.Build, InstanceID: info.InstanceID, Capabilities: []agent.Capability{agent.CapabilitySecretsDetail, agent.CapabilitySecretsList, agent.CapabilitySecretsWatch}},
		{Runtime: info.Runtime, Build: info.Build, InstanceID: info.InstanceID, Capabilities: []agent.Capability{agent.CapabilitySecretsList, agent.CapabilitySecretsDetail}},
	}
	for index, candidate := range mutations {
		if validInfo(candidate, "v1.2.3", "v1.2.3") {
			t.Fatalf("invalid info mutation %d accepted", index)
		}
	}
}

func TestGenericMethodPolicyCanary(t *testing.T) {
	credential := knownCredential(t)
	positiveCalls := 0
	valid := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requireCreatorRequest(t, request, http.MethodPost, "/api/call")
		body := make([]byte, len(policyCanaryBody))
		_, _ = request.Body.Read(body)
		if string(body) != policyCanaryBody || request.Header.Get("Content-Type") != "application/json" {
			t.Errorf("non-canonical canary: %q", body)
		}
		if strings.Contains(string(body), "Secret") {
			positiveCalls++
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.Header().Set("Cache-Control", "no-store")
		writer.WriteHeader(http.StatusForbidden)
		_, _ = writer.Write([]byte("{\"error\":\"forbidden\"}\n"))
	}))
	defer valid.Close()
	if err := credential.withCreatorAuthorization(context.Background(), func(ctx context.Context, authorization creatorAuthorizationLease) error {
		return policyCanary(ctx, endpointOf(t, valid.URL), authorization)
	}); err != nil || positiveCalls != 0 {
		t.Fatalf("exact canary failed: %v positive=%d", err, positiveCalls)
	}
	for _, body := range []string{`{"error":"unauthorized"}`, `{"error":"forbidden","extra":1}`, `{"error":"forbidden","error":"forbidden"}`, strings.Repeat("x", policyBodyLimit+1)} {
		bad := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.Header().Set("Content-Type", "application/json")
			writer.Header().Set("Cache-Control", "no-store")
			writer.WriteHeader(http.StatusForbidden)
			_, _ = writer.Write([]byte(body))
		}))
		err := credential.withCreatorAuthorization(context.Background(), func(ctx context.Context, authorization creatorAuthorizationLease) error {
			return policyCanary(ctx, endpointOf(t, bad.URL), authorization)
		})
		bad.Close()
		if err == nil {
			t.Fatalf("invalid policy envelope accepted: %q", body)
		}
	}
}

func TestCreatorWebSocketHandshakeExact(t *testing.T) {
	credential := knownCredential(t)
	upgrader := websocket.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}
	websocketServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requireCreatorRequest(t, request, http.MethodGet, "/ws")
		if request.URL.RawQuery != "" || request.Header.Get("Sec-WebSocket-Protocol") != "" || request.Header.Get("Sec-WebSocket-Extensions") != "" {
			t.Errorf("unsafe websocket offer")
		}
		connection, err := upgrader.Upgrade(writer, request, nil)
		if err != nil {
			return
		}
		defer connection.Close()
		_ = connection.WriteJSON(map[string]interface{}{"type": "event", "name": "connected", "data": map[string]interface{}{"sessionId": "session-a", "instanceId": "instance-a", "generation": 1, "resumed": false}})
		for {
			if _, _, err = connection.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer websocketServer.Close()
	var socket *websocket.Conn
	var connected connectedIdentity
	err := credential.withCreatorAuthorization(context.Background(), func(ctx context.Context, authorization creatorAuthorizationLease) error {
		var dialErr error
		socket, dialErr = dialCreator(ctx, endpointOf(t, websocketServer.URL), authorization)
		if dialErr != nil {
			return dialErr
		}
		connected, dialErr = validateConnected(ctx, socket, "instance-a")
		return dialErr
	})
	if err != nil || connected.sessionID != "session-a" || connected.instanceID != "instance-a" || connected.generation != 1 {
		t.Fatalf("creator handshake failed: %#v %v", connected, err)
	}
	_ = socket.Close()

	badFrames := []struct {
		name    string
		kind    int
		payload string
	}{
		{"binary", websocket.BinaryMessage, `{}`},
		{"unknown", websocket.TextMessage, `{"type":"event","name":"connected","data":{"sessionId":"s","instanceId":"instance-a","generation":1,"resumed":false},"extra":1}`},
		{"duplicate", websocket.TextMessage, `{"type":"event","name":"connected","name":"connected","data":{"sessionId":"s","instanceId":"instance-a","generation":1,"resumed":false}}`},
		{"wrong event", websocket.TextMessage, `{"type":"event","name":"other","data":{"sessionId":"s","instanceId":"instance-a","generation":1,"resumed":false}}`},
		{"missing session", websocket.TextMessage, `{"type":"event","name":"connected","data":{"sessionId":"","instanceId":"instance-a","generation":1,"resumed":false}}`},
		{"instance mismatch", websocket.TextMessage, `{"type":"event","name":"connected","data":{"sessionId":"s","instanceId":"other","generation":1,"resumed":false}}`},
		{"generation", websocket.TextMessage, `{"type":"event","name":"connected","data":{"sessionId":"s","instanceId":"instance-a","generation":2,"resumed":false}}`},
		{"resumed", websocket.TextMessage, `{"type":"event","name":"connected","data":{"sessionId":"s","instanceId":"instance-a","generation":1,"resumed":true}}`},
		{"missing resumed", websocket.TextMessage, `{"type":"event","name":"connected","data":{"sessionId":"s","instanceId":"instance-a","generation":1}}`},
		{"oversized", websocket.TextMessage, strings.Repeat("x", connectedFrameLimit+1)},
	}
	for _, test := range badFrames {
		t.Run(test.name, func(t *testing.T) {
			badServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				connection, upgradeErr := upgrader.Upgrade(writer, request, nil)
				if upgradeErr != nil {
					return
				}
				defer connection.Close()
				_ = connection.WriteMessage(test.kind, []byte(test.payload))
			}))
			defer badServer.Close()
			err := credential.withCreatorAuthorization(context.Background(), func(ctx context.Context, authorization creatorAuthorizationLease) error {
				connection, dialErr := dialCreator(ctx, endpointOf(t, badServer.URL), authorization)
				if dialErr != nil {
					return dialErr
				}
				defer connection.Close()
				_, dialErr = validateConnected(ctx, connection, "instance-a")
				return dialErr
			})
			if err == nil {
				t.Fatal("invalid first frame accepted")
			}
		})
	}

	_ = fmt.Sprintf("%s", net.IPv4(127, 0, 0, 1)) // compile-time guard: fixtures remain IPv4-loopback.
}
