package server

import (
	"bytes"
	"context"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

const acceleratorTestCreatorToken = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

type incrementingEntropy struct {
	mu   sync.Mutex
	next byte
}

func (e *incrementingEntropy) Read(buffer []byte) (int, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for i := range buffer {
		e.next++
		buffer[i] = e.next
	}
	return len(buffer), nil
}

type acceleratorWebSocketFixture struct {
	t             *testing.T
	registry      *AcceleratorSessionRegistry
	observer      *recordingAcceleratorObserver
	creator       *CreatorAuthenticator
	browser       *BrowserSessionManager
	browserBearer BrowserBearer
	authenticator *AcceleratorWebSocketAuthenticator
	server        *Server
	http          *httptest.Server
	url           string
}

func newAcceleratorWebSocketFixture(t *testing.T) *acceleratorWebSocketFixture {
	return newAcceleratorWebSocketFixtureWithClock(t, time.Now)
}

func newAcceleratorWebSocketFixtureWithClock(t *testing.T, now func() time.Time) *acceleratorWebSocketFixture {
	t.Helper()
	token, err := ParseCreatorToken(acceleratorTestCreatorToken)
	if err != nil {
		t.Fatal(err)
	}
	creator, err := NewCreatorAuthenticator(DeriveCreatorVerifier(token).Encoded())
	if err != nil {
		t.Fatal(err)
	}
	observer := &recordingAcceleratorObserver{}
	registry := newAcceleratorSessionRegistry("accel-test", observer, acceleratorTestConfig())
	browser := newBrowserSessionManager(now, &incrementingEntropy{}, registry)
	ticket, _, err := browser.Mint()
	if err != nil {
		t.Fatal(err)
	}
	bearer, _, err := browser.Exchange(ticket)
	if err != nil {
		t.Fatal(err)
	}
	authenticator := &AcceleratorWebSocketAuthenticator{Creator: creator, BrowserSessions: browser, Registry: registry}
	options := AcceleratorOptions(0, nil, CreatorOrBrowserGuard(creator, browser))
	options.BrowserSessions = browser
	options.AcceleratorSessions = registry
	options.AcceleratorWebSocketAuthenticator = authenticator
	server, err := NewWithOptions(nil, embed.FS{}, options)
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server.Handler())
	fixture := &acceleratorWebSocketFixture{
		t: t, registry: registry, observer: observer, creator: creator, browser: browser,
		browserBearer: bearer, authenticator: authenticator, server: server, http: httpServer,
		url: "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws",
	}
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

func (f *acceleratorWebSocketFixture) browserProtocols(bearer BrowserBearer) []string {
	return []string{AcceleratorWebSocketProtocol, AcceleratorBrowserCredentialProtocolPrefix + bearer.encoded()}
}

func dialAccelerator(t *testing.T, rawURL string, protocols []string, header http.Header) (*websocket.Conn, *http.Response, error) {
	t.Helper()
	dialer := websocket.Dialer{Subprotocols: protocols, HandshakeTimeout: time.Second}
	return dialer.Dial(rawURL, header)
}

func closeDialResponse(response *http.Response) {
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
}

func requireRejectedDial(t *testing.T, rawURL string, protocols []string, header http.Header) {
	t.Helper()
	connection, response, err := dialAccelerator(t, rawURL, protocols, header)
	if connection != nil {
		_ = connection.Close()
	}
	if err == nil || response == nil || response.StatusCode == http.StatusSwitchingProtocols {
		t.Fatalf("dial unexpectedly upgraded: conn=%v response=%v err=%v", connection != nil, response, err)
	}
	body, _ := io.ReadAll(response.Body)
	closeDialResponse(response)
	joined := string(body) + fmt.Sprint(response.Header)
	if strings.Contains(joined, acceleratorTestCreatorToken) || strings.Contains(joined, AcceleratorBrowserCredentialProtocolPrefix) {
		t.Fatalf("credential leaked in rejection: %q", joined)
	}
	for _, protocol := range protocols {
		if protocol != "" && strings.Contains(joined, protocol) {
			t.Fatalf("offered protocol reflected in rejection: %q", joined)
		}
		if bearer := strings.TrimPrefix(protocol, AcceleratorBrowserCredentialProtocolPrefix); bearer != protocol && strings.Contains(joined, bearer) {
			t.Fatalf("browser bearer reflected in rejection: %q", joined)
		}
	}
}

// T1: real Gorilla handshakes cover both exact accepted forms and the complete
// credential/protocol/outer-boundary rejection matrix.
func TestAcceleratorWebSocketAuthenticationMatrix(t *testing.T) {
	t.Run("valid creator", func(t *testing.T) {
		fixture := newAcceleratorWebSocketFixture(t)
		header := fixture.creatorHeader()
		header.Set("Origin", strings.Replace(strings.TrimSuffix(fixture.url, "/ws"), "ws", "http", 1))
		connection, response, err := dialAccelerator(t, fixture.url, nil, header)
		if err != nil {
			t.Fatal(err)
		}
		closeDialResponse(response)
		defer connection.Close()
	})
	t.Run("valid browser", func(t *testing.T) {
		fixture := newAcceleratorWebSocketFixture(t)
		connection, response, err := dialAccelerator(t, fixture.url, fixture.browserProtocols(fixture.browserBearer), nil)
		if err != nil {
			t.Fatal(err)
		}
		closeDialResponse(response)
		defer connection.Close()
	})

	tests := []struct {
		name      string
		path      string
		protocols func(*acceleratorWebSocketFixture) []string
		header    func(*acceleratorWebSocketFixture) http.Header
		prepare   func(*acceleratorWebSocketFixture)
	}{
		{name: "missing credentials"},
		{name: "wrong creator", header: func(*acceleratorWebSocketFixture) http.Header {
			return http.Header{"Authorization": []string{"Bearer BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB"}}
		}},
		{name: "dual credentials", protocols: func(f *acceleratorWebSocketFixture) []string { return f.browserProtocols(f.browserBearer) }, header: func(f *acceleratorWebSocketFixture) http.Header { return f.creatorHeader() }},
		{name: "query credential", path: "?token=hidden", header: func(f *acceleratorWebSocketFixture) http.Header { return f.creatorHeader() }},
		{name: "bare query", path: "?", header: func(f *acceleratorWebSocketFixture) http.Header { return f.creatorHeader() }},
		{name: "cookie", header: func(f *acceleratorWebSocketFixture) http.Header {
			h := f.creatorHeader()
			h.Set("Cookie", "token=hidden")
			return h
		}},
		{name: "reversed protocols", protocols: func(f *acceleratorWebSocketFixture) []string {
			p := f.browserProtocols(f.browserBearer)
			return []string{p[1], p[0]}
		}},
		{name: "missing safe protocol", protocols: func(f *acceleratorWebSocketFixture) []string {
			return []string{AcceleratorBrowserCredentialProtocolPrefix + f.browserBearer.encoded()}
		}},
		{name: "extra protocol", protocols: func(f *acceleratorWebSocketFixture) []string {
			return append(f.browserProtocols(f.browserBearer), "extra")
		}},
		{name: "duplicate protocol", protocols: func(f *acceleratorWebSocketFixture) []string {
			p := f.browserProtocols(f.browserBearer)
			return []string{p[0], p[0], p[1]}
		}},
		{name: "safe protocol case", protocols: func(f *acceleratorWebSocketFixture) []string {
			p := f.browserProtocols(f.browserBearer)
			p[0] = strings.ToUpper(p[0])
			return p
		}},
		{name: "credential prefix case", protocols: func(f *acceleratorWebSocketFixture) []string {
			return []string{AcceleratorWebSocketProtocol, strings.ToUpper(AcceleratorBrowserCredentialProtocolPrefix) + f.browserBearer.encoded()}
		}},
		{name: "replaced random bearer", protocols: func(f *acceleratorWebSocketFixture) []string { return f.browserProtocols(BrowserBearer{}) }},
		{name: "revoked browser", protocols: func(f *acceleratorWebSocketFixture) []string { return f.browserProtocols(f.browserBearer) }, prepare: func(f *acceleratorWebSocketFixture) { f.browser.Revoke(context.Background()) }},
		{name: "wrong path", path: "/nested", header: func(f *acceleratorWebSocketFixture) http.Header { return f.creatorHeader() }},
		{name: "trailing slash alias", path: "/", header: func(f *acceleratorWebSocketFixture) http.Header { return f.creatorHeader() }},
		{name: "origin mismatch", header: func(f *acceleratorWebSocketFixture) http.Header {
			h := f.creatorHeader()
			h.Set("Origin", "http://localhost:1")
			return h
		}},
		{name: "quiescing", header: func(f *acceleratorWebSocketFixture) http.Header { return f.creatorHeader() }, prepare: func(f *acceleratorWebSocketFixture) { f.server.Quiesce() }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newAcceleratorWebSocketFixture(t)
			if test.prepare != nil {
				test.prepare(fixture)
			}
			protocols := []string(nil)
			if test.protocols != nil {
				protocols = test.protocols(fixture)
			}
			header := http.Header(nil)
			if test.header != nil {
				header = test.header(fixture)
			}
			requireRejectedDial(t, fixture.url+test.path, protocols, header)
		})
	}
	aliasFixture := newAcceleratorWebSocketFixture(t)
	requireRejectedDial(t, strings.TrimSuffix(aliasFixture.url, "/ws")+"/api/ws", nil, aliasFixture.creatorHeader())

	fixture := newAcceleratorWebSocketFixture(t)
	request := httptest.NewRequest(http.MethodGet, "http://localhost/ws", nil)
	request.Header["Sec-WebSocket-Protocol"] = []string{
		AcceleratorWebSocketProtocol,
		AcceleratorBrowserCredentialProtocolPrefix + fixture.browserBearer.encoded(),
	}
	if _, ok := fixture.authenticator.authenticate(request); ok {
		t.Fatal("multiple protocol headers accepted")
	}
	request = httptest.NewRequest(http.MethodGet, "http://localhost/ws", nil)
	request.Header.Set("Sec-WebSocket-Protocol", AcceleratorWebSocketProtocol+",,	"+AcceleratorBrowserCredentialProtocolPrefix+fixture.browserBearer.encoded())
	if _, ok := fixture.authenticator.authenticate(request); ok {
		t.Fatal("comma-malformed protocols accepted")
	}
	verifier := DeriveBrowserBearerVerifier(fixture.browserBearer)
	request = httptest.NewRequest(http.MethodGet, "http://localhost/ws", nil)
	request.Header.Set("Sec-WebSocket-Protocol", AcceleratorWebSocketProtocol+", "+AcceleratorBrowserCredentialProtocolPrefix+base64.RawURLEncoding.EncodeToString(verifier[:]))
	if _, ok := fixture.authenticator.authenticate(request); ok {
		t.Fatal("verifier accepted as bearer")
	}
	request = httptest.NewRequest(http.MethodGet, "http://localhost/ws", nil)
	request.Header["Authorization"] = []string{"Bearer " + acceleratorTestCreatorToken, "Bearer " + acceleratorTestCreatorToken}
	if _, ok := fixture.authenticator.authenticate(request); ok {
		t.Fatal("multiple Authorization headers accepted")
	}

	nonUpgrade, err := http.NewRequest(http.MethodGet, strings.Replace(fixture.url, "ws", "http", 1), nil)
	if err != nil {
		t.Fatal(err)
	}
	nonUpgrade.Header = fixture.creatorHeader()
	response, err := http.DefaultClient.Do(nonUpgrade)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode == http.StatusSwitchingProtocols {
		t.Fatal("non-upgrade request switched protocols")
	}
	closeDialResponse(response)
	wrongMethod, err := http.NewRequest(http.MethodPost, strings.Replace(fixture.url, "ws", "http", 1), nil)
	if err != nil {
		t.Fatal(err)
	}
	wrongMethod.Header = fixture.creatorHeader()
	response, err = http.DefaultClient.Do(wrongMethod)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode == http.StatusSwitchingProtocols {
		t.Fatal("wrong method switched protocols")
	}
	closeDialResponse(response)
	invalidHost := httptest.NewRequest(http.MethodGet, "http://example.invalid/ws", nil)
	invalidHost.Host = "example.invalid"
	invalidHost.Header = fixture.creatorHeader()
	invalidHost.Header.Set("Connection", "Upgrade")
	invalidHost.Header.Set("Upgrade", "websocket")
	invalidHost.Header.Set("Sec-WebSocket-Version", "13")
	invalidHost.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	invalidHostResponse := httptest.NewRecorder()
	fixture.server.Handler().ServeHTTP(invalidHostResponse, invalidHost)
	if invalidHostResponse.Code != http.StatusBadRequest {
		t.Fatalf("invalid Host status = %d", invalidHostResponse.Code)
	}
}

func TestBrowserWebSocketRejectsExpiredAndReplacedBearers(t *testing.T) {
	t.Run("exact expiry", func(t *testing.T) {
		start := time.Date(2026, 8, 1, 8, 9, 10, 123, time.UTC)
		clock := &testClock{now: start}
		fixture := newAcceleratorWebSocketFixtureWithClock(t, clock.Now)
		fixture.browser.state.mu.Lock()
		browserID := fixture.browser.state.session.context.SessionID
		fixture.browser.state.mu.Unlock()
		clock.Set(start.Add(BrowserSessionIdleTTL))
		requireRejectedDial(t, fixture.url, fixture.browserProtocols(fixture.browserBearer), nil)
		if got := fixture.observer.eventCopy(); len(got) != 0 {
			t.Fatalf("expired bearer callbacks=%v", got)
		}
		if _, _, found := fixture.registry.snapshot(browserID); found {
			t.Fatal("expired bearer published registry state")
		}
	})

	t.Run("replacement", func(t *testing.T) {
		fixture := newAcceleratorWebSocketFixture(t)
		oldBearer := fixture.browserBearer
		fixture.browser.state.mu.Lock()
		oldID := fixture.browser.state.session.context.SessionID
		fixture.browser.state.mu.Unlock()
		ticket, _, err := fixture.browser.Mint()
		if err != nil {
			t.Fatal(err)
		}
		currentBearer, _, err := fixture.browser.Exchange(ticket)
		if err != nil {
			t.Fatal(err)
		}
		fixture.browser.state.mu.Lock()
		currentID := fixture.browser.state.session.context.SessionID
		fixture.browser.state.mu.Unlock()
		if currentID == oldID {
			t.Fatal("browser replacement reused session identity")
		}
		requireRejectedDial(t, fixture.url, fixture.browserProtocols(oldBearer), nil)
		if got := fixture.observer.eventCopy(); len(got) != 0 {
			t.Fatalf("replaced bearer callbacks=%v", got)
		}
		if _, _, found := fixture.registry.snapshot(oldID); found {
			t.Fatal("replaced bearer published old registry state")
		}
		connection, response, err := dialAccelerator(t, fixture.url, fixture.browserProtocols(currentBearer), nil)
		if err != nil {
			t.Fatal(err)
		}
		defer connection.Close()
		closeDialResponse(response)
		_, connected := readConnectedEvent(t, connection)
		if connected.SessionID != currentID || connected.Generation != 1 || connected.Resumed {
			t.Fatalf("replacement connected=%+v currentID=%q", connected, currentID)
		}
		waitAccelerator(t, "replacement observer", func() bool { return len(fixture.observer.eventCopy()) == 1 })
		if got := fmt.Sprint(fixture.observer.eventCopy()); got != "[connect:1]" {
			t.Fatalf("replacement callbacks=%s", got)
		}
	})
}

// T2: the bearer-bearing offered protocol is never selected or reflected.
func TestBrowserWebSocketNegotiatesOnlySafeProtocol(t *testing.T) {
	fixture := newAcceleratorWebSocketFixture(t)
	credentialProtocol := AcceleratorBrowserCredentialProtocolPrefix + fixture.browserBearer.encoded()
	connection, response, err := dialAccelerator(t, fixture.url, []string{AcceleratorWebSocketProtocol, credentialProtocol}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	defer closeDialResponse(response)
	if connection.Subprotocol() != AcceleratorWebSocketProtocol || response.Header.Get("Sec-WebSocket-Protocol") != AcceleratorWebSocketProtocol {
		t.Fatalf("negotiated protocol = %q/%q", connection.Subprotocol(), response.Header.Get("Sec-WebSocket-Protocol"))
	}
	if strings.Contains(fmt.Sprint(response.Header), credentialProtocol) || strings.Contains(fmt.Sprint(response.Header), fixture.browserBearer.encoded()) {
		t.Fatal("browser credential reflected in handshake response")
	}
	creator, creatorResponse, err := dialAccelerator(t, fixture.url, nil, fixture.creatorHeader())
	if err != nil {
		t.Fatal(err)
	}
	defer creator.Close()
	defer closeDialResponse(creatorResponse)
	if creator.Subprotocol() != "" || creatorResponse.Header.Get("Sec-WebSocket-Protocol") != "" {
		t.Fatal("creator negotiated a protocol")
	}
}

func readConnectedEvent(t *testing.T, connection *websocket.Conn) ([]byte, AcceleratorConnectedEvent) {
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

// T3: first-frame ordering, exact DTO shape, retained reconnect/replacement,
// and browser revocation/new-ID reset are all exercised with real sockets.
func TestAcceleratorWebSocketFirstFrameAndGenerations(t *testing.T) {
	fixture := newAcceleratorWebSocketFixture(t)
	dialCreator := func() *websocket.Conn {
		connection, response, err := dialAccelerator(t, fixture.url, nil, fixture.creatorHeader())
		if err != nil {
			t.Fatal(err)
		}
		closeDialResponse(response)
		return connection
	}
	first := dialCreator()
	raw, connected := readConnectedEvent(t, first)
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
	second := dialCreator()
	_, connected = readConnectedEvent(t, second)
	if connected.Generation != 2 || !connected.Resumed {
		t.Fatalf("reconnect = %+v", connected)
	}
	third := dialCreator()
	_, connected = readConnectedEvent(t, third)
	if connected.Generation != 3 || !connected.Resumed {
		t.Fatalf("replacement = %+v", connected)
	}
	_ = second.SetReadDeadline(time.Now().Add(time.Second))
	if _, _, err := second.ReadMessage(); err == nil {
		t.Fatal("replaced socket remained readable")
	}
	_ = second.Close()
	_ = third.Close()

	browser, response, err := dialAccelerator(t, fixture.url, fixture.browserProtocols(fixture.browserBearer), nil)
	if err != nil {
		t.Fatal(err)
	}
	closeDialResponse(response)
	_, browserConnected := readConnectedEvent(t, browser)
	if browserConnected.Generation != 1 || browserConnected.Resumed {
		t.Fatalf("browser first = %+v", browserConnected)
	}
	oldID := browserConnected.SessionID
	fixture.browser.Revoke(context.Background())
	_ = browser.SetReadDeadline(time.Now().Add(time.Second))
	if _, _, err := browser.ReadMessage(); err == nil {
		t.Fatal("revoked browser socket remained open")
	}
	ticket, _, err := fixture.browser.Mint()
	if err != nil {
		t.Fatal(err)
	}
	newBearer, _, err := fixture.browser.Exchange(ticket)
	if err != nil {
		t.Fatal(err)
	}
	newBrowser, response, err := dialAccelerator(t, fixture.url, fixture.browserProtocols(newBearer), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer newBrowser.Close()
	closeDialResponse(response)
	_, browserConnected = readConnectedEvent(t, newBrowser)
	if browserConnected.SessionID == oldID || browserConnected.Generation != 1 || browserConnected.Resumed {
		t.Fatalf("new browser session = %+v oldID=%q", browserConnected, oldID)
	}
}

func TestBrowserWebSocketRevocationAfterUpgradeSendsNoConnectedFrame(t *testing.T) {
	fixture := newAcceleratorWebSocketFixture(t)
	fixture.browser.state.mu.Lock()
	browserID := fixture.browser.state.session.context.SessionID
	fixture.browser.state.mu.Unlock()
	afterUpgrade := make(chan struct{})
	continueHandler := make(chan struct{})
	fixture.authenticator.afterUpgrade = func() {
		close(afterUpgrade)
		<-continueHandler
	}
	type dialResult struct {
		connection *websocket.Conn
		response   *http.Response
		err        error
	}
	dialed := make(chan dialResult, 1)
	go func() {
		connection, response, err := dialAccelerator(t, fixture.url, fixture.browserProtocols(fixture.browserBearer), nil)
		dialed <- dialResult{connection: connection, response: response, err: err}
	}()
	select {
	case <-afterUpgrade:
	case <-time.After(2 * time.Second):
		t.Fatal("upgrade barrier not reached")
	}
	fixture.browser.Revoke(context.Background())
	close(continueHandler)
	result := <-dialed
	closeDialResponse(result.response)
	if result.err != nil {
		t.Fatal(result.err)
	}
	defer result.connection.Close()
	_ = result.connection.SetReadDeadline(time.Now().Add(time.Second))
	if _, _, err := result.connection.ReadMessage(); err == nil {
		t.Fatal("revocation-losing post-101 socket received a frame")
	}
	if _, _, found := fixture.registry.snapshot(browserID); found {
		t.Fatal("revoked browser race retained a registry record")
	}
	if got := fixture.observer.eventCopy(); len(got) != 0 {
		t.Fatalf("lost race emitted observer events: %v", got)
	}
}

func TestBrowserWebSocketFinalValidationFailureEmitsNothing(t *testing.T) {
	fixture := newAcceleratorWebSocketFixture(t)
	fixture.browser.state.mu.Lock()
	browserID := fixture.browser.state.session.context.SessionID
	fixture.browser.state.mu.Unlock()
	prepared := make(chan struct{})
	continueHandler := make(chan struct{})
	fixture.authenticator.afterPrepare = func() {
		close(prepared)
		<-continueHandler
	}
	type dialResult struct {
		connection *websocket.Conn
		response   *http.Response
		err        error
	}
	dialed := make(chan dialResult, 1)
	go func() {
		connection, response, err := dialAccelerator(t, fixture.url, fixture.browserProtocols(fixture.browserBearer), nil)
		dialed <- dialResult{connection: connection, response: response, err: err}
	}()
	select {
	case <-prepared:
	case <-time.After(2 * time.Second):
		t.Fatal("prepared barrier not reached")
	}
	fixture.browser.Revoke(context.Background())
	close(continueHandler)
	result := <-dialed
	closeDialResponse(result.response)
	if result.err != nil {
		t.Fatal(result.err)
	}
	defer result.connection.Close()
	_ = result.connection.SetReadDeadline(time.Now().Add(time.Second))
	if _, _, err := result.connection.ReadMessage(); err == nil {
		t.Fatal("final-validation-losing socket received a frame")
	}
	if _, _, found := fixture.registry.snapshot(browserID); found {
		t.Fatal("final-validation failure retained a registry record")
	}
	if got := fixture.observer.eventCopy(); len(got) != 0 {
		t.Fatalf("final-validation callbacks=%v", got)
	}
}

func TestBrowserWebSocketBootstrapFailureEmitsNoFrameAndDisconnectsObserver(t *testing.T) {
	fixture := newAcceleratorWebSocketFixture(t)
	fixture.browser.state.mu.Lock()
	browserID := fixture.browser.state.session.context.SessionID
	fixture.browser.state.mu.Unlock()
	failing := newFakeAcceleratorConn()
	failing.writeErr = errors.New("bootstrap failed")
	fixture.authenticator.beforeActivate = func(registration *acceleratorRegistration) {
		registration.socket.conn = failing
	}
	connection, response, err := dialAccelerator(t, fixture.url, fixture.browserProtocols(fixture.browserBearer), nil)
	if err != nil {
		t.Fatal(err)
	}
	closeDialResponse(response)
	defer connection.Close()
	_ = connection.SetReadDeadline(time.Now().Add(time.Second))
	if _, _, err := connection.ReadMessage(); err == nil {
		t.Fatal("bootstrap-failing socket received a frame")
	}
	waitAccelerator(t, "bootstrap failure cleanup", func() bool {
		_, active, found := fixture.registry.snapshot(browserID)
		return found && !active
	})
	if failing.writeCount() != 0 {
		t.Fatalf("bootstrap failure wrote %d frames", failing.writeCount())
	}
	if got := fixture.observer.eventCopy(); len(got) != 2 || got[0] != "connect:1" || got[1] != "disconnect:1" {
		t.Fatalf("bootstrap-failure callbacks=%v", got)
	}
}

func TestBrowserWebSocketObserverRunsOutsideManagerAndAdmissionLocks(t *testing.T) {
	fixture := newAcceleratorWebSocketFixture(t)
	done := make(chan struct{})
	fixture.observer.onEvent = func(kind string, _ AcceleratorSessionSnapshot) {
		if kind != "connect" {
			return
		}
		fixture.browser.state.mu.Lock()
		fixture.browser.state.mu.Unlock()
		release, ok := fixture.registry.beginUpgrade()
		if !ok {
			t.Error("observer could not reenter admission")
		} else {
			release()
		}
		fixture.registry.Quiesce()
		close(done)
	}
	connection, response, err := dialAccelerator(t, fixture.url, fixture.browserProtocols(fixture.browserBearer), nil)
	if err != nil {
		t.Fatal(err)
	}
	closeDialResponse(response)
	defer connection.Close()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("browser observer deadlocked on manager/admission lock")
	}
	waitAccelerator(t, "browser quiesce disconnect", func() bool { return len(fixture.observer.eventCopy()) == 2 })
	if got := fmt.Sprint(fixture.observer.eventCopy()); got != "[connect:1 disconnect:1]" {
		t.Fatalf("events=%s", got)
	}
}

func TestAcceleratorWebSocketBareQueryDoesNotPublishSession(t *testing.T) {
	fixture := newAcceleratorWebSocketFixture(t)
	requireRejectedDial(t, fixture.url+"?", nil, fixture.creatorHeader())
	if got := fixture.observer.eventCopy(); len(got) != 0 {
		t.Fatalf("bare query callbacks=%v", got)
	}
	if _, _, found := fixture.registry.snapshot(fixture.creator.context.SessionID); found {
		t.Fatal("bare query published session")
	}
}

func TestAcceleratorWebSocketOversizedInboundAndDiscard(t *testing.T) {
	fixture := newAcceleratorWebSocketFixture(t)
	connection, response, err := dialAccelerator(t, fixture.url, nil, fixture.creatorHeader())
	if err != nil {
		t.Fatal(err)
	}
	closeDialResponse(response)
	defer connection.Close()
	_, _ = readConnectedEvent(t, connection)
	if err := connection.WriteMessage(websocket.TextMessage, []byte(`{"ignored":true}`)); err != nil {
		t.Fatal(err)
	}
	fixture.registry.EmitEvent("after-inbound", "safe")
	_ = connection.SetReadDeadline(time.Now().Add(time.Second))
	var event Event
	if err := connection.ReadJSON(&event); err != nil || event.Name != "after-inbound" {
		t.Fatalf("discard/event = %+v err=%v", event, err)
	}
	sibling, siblingResponse, err := dialAccelerator(t, fixture.url, fixture.browserProtocols(fixture.browserBearer), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sibling.Close()
	closeDialResponse(siblingResponse)
	_, siblingConnected := readConnectedEvent(t, sibling)
	if err := connection.WriteMessage(websocket.TextMessage, bytes.Repeat([]byte("x"), int(AcceleratorSocketReadLimit)+1)); err != nil {
		t.Fatal(err)
	}
	waitAccelerator(t, "oversized disconnect", func() bool {
		_, active, _ := fixture.registry.snapshot(fixture.creator.context.SessionID)
		return !active
	})
	fixture.registry.EmitEvent("oversized-sibling-probe", "safe")
	_ = sibling.SetReadDeadline(time.Now().Add(time.Second))
	if err := sibling.ReadJSON(&event); err != nil || event.Name != "oversized-sibling-probe" {
		t.Fatalf("oversized sibling event=%+v err=%v", event, err)
	}
	if got := fmt.Sprint(fixture.observer.eventsForSession(fixture.creator.context.SessionID)); got != "[connect:1 disconnect:1]" {
		t.Fatalf("oversized creator callbacks=%s", got)
	}
	if got := fmt.Sprint(fixture.observer.eventsForSession(siblingConnected.SessionID)); got != "[connect:1]" {
		t.Fatalf("oversized sibling callbacks=%s", got)
	}
}

func TestAcceleratorWebSocketExactPathEncoding(t *testing.T) {
	fixture := newAcceleratorWebSocketFixture(t)
	parsed, err := url.Parse(fixture.url)
	if err != nil {
		t.Fatal(err)
	}
	parsed.RawPath = "/%77s"
	requireRejectedDial(t, parsed.String(), nil, fixture.creatorHeader())
}

type modeIsolationDisconnectListener struct{ disconnected chan string }

func (l modeIsolationDisconnectListener) OnClientDisconnect(id string) { l.disconnected <- id }

// T10: Accelerator and compatibility WebSockets remain two independent
// transports with their original admission, payload, broadcast, and listener
// behavior.
func TestWebSocketModeIsolation(t *testing.T) {
	accelerator := newAcceleratorWebSocketFixture(t)
	requireRejectedDial(t, accelerator.url, nil, nil)
	acceleratorConnection, response, err := dialAccelerator(t, accelerator.url, nil, accelerator.creatorHeader())
	if err != nil {
		t.Fatal(err)
	}
	closeDialResponse(response)
	defer acceleratorConnection.Close()
	_, connected := readConnectedEvent(t, acceleratorConnection)
	encoded, _ := json.Marshal(connected)
	if strings.Contains(string(encoded), "clientId") || strings.Contains(string(encoded), "serverMode") {
		t.Fatalf("Accelerator used compatibility payload: %s", encoded)
	}

	ordinary, err := NewWithOptions(nil, embed.FS{}, CompatibilityOptions(0, nil))
	if err != nil {
		t.Fatal(err)
	}
	go ordinary.handleBroadcast()
	disconnected := make(chan string, 1)
	ordinary.AddDisconnectListener(modeIsolationDisconnectListener{disconnected: disconnected})
	ordinaryHTTP := httptest.NewServer(ordinary.Handler())
	defer ordinaryHTTP.Close()
	ordinaryURL := "ws" + strings.TrimPrefix(ordinaryHTTP.URL, "http") + "/ws"
	ordinaryConnection, ordinaryResponse, err := dialAccelerator(t, ordinaryURL, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	closeDialResponse(ordinaryResponse)
	var ordinaryConnected Event
	if err := ordinaryConnection.ReadJSON(&ordinaryConnected); err != nil {
		t.Fatal(err)
	}
	ordinaryPayload, _ := json.Marshal(ordinaryConnected.Data)
	if ordinaryConnected.Name != "connected" || !strings.Contains(string(ordinaryPayload), `"serverMode":true`) || !strings.Contains(string(ordinaryPayload), `"clientId":"ws-1"`) {
		t.Fatalf("ordinary connected = %+v %s", ordinaryConnected, ordinaryPayload)
	}
	ordinary.EmitEvent("compatibility", "preserved")
	var broadcast Event
	if err := ordinaryConnection.ReadJSON(&broadcast); err != nil || broadcast.Name != "compatibility" {
		t.Fatalf("ordinary broadcast = %+v err=%v", broadcast, err)
	}
	_ = ordinaryConnection.Close()
	select {
	case id := <-disconnected:
		if id != "ws-1" {
			t.Fatalf("ordinary disconnect ID = %q", id)
		}
	case <-time.After(time.Second):
		t.Fatal("ordinary disconnect listener not called")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := ordinary.Close(ctx); err != nil {
		t.Fatal(err)
	}
}
