package acceleratorprovision

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"kubikles/pkg/agent"
	"kubikles/pkg/server"
)

func currentBrowserLaunchSession(t *testing.T) (*ConnectedSession, *recordedSessionSocket) {
	t.Helper()
	workload := connectorWorkload(t)
	receipt := workload.connectorState.receipt
	order := []string{}
	socket := newRecordedSessionSocket(&order)
	active := newRecordedTunnel(&order)
	session := newConnectedSession(receipt, server.AuthenticatedAcceleratorInfo{Capabilities: agent.V1Capabilities()}, connectedIdentity{sessionID: "session-a", instanceID: "instance-a", generation: 1}, socket, active, processResumeClock{})
	if !workload.publishCurrentSession(session) {
		t.Fatal("session publication rejected")
	}
	t.Cleanup(func() { _ = session.Close(context.Background()) })
	return session, socket
}

func TestCreatorPumpInterceptsLaunchReceipt(t *testing.T) {
	session, socket := currentBrowserLaunchSession(t)
	receipt := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	key, ok := browserLaunchReceiptKey(receipt)
	if !ok {
		t.Fatal("receipt rejected")
	}
	updates, unregister, ok := session.registerBrowserLaunchWaiter(receipt)
	if !ok {
		t.Fatal("waiter rejected")
	}
	defer unregister()
	normal := []byte(`{"type":"event","name":"ordinary","data":{"safe":true}}`)
	socket.messages <- socketMessage{kind: websocket.TextMessage, payload: normal}
	for _, status := range []string{"confirmed", "confirmed", "ended", "ended"} {
		payload, _ := json.Marshal(map[string]interface{}{"type": "event", "name": "browser-launch", "data": map[string]string{"receipt": key, "status": status}})
		socket.messages <- socketMessage{kind: websocket.TextMessage, payload: payload}
	}
	select {
	case frame := <-session.frames:
		if string(frame) != string(normal) {
			t.Fatalf("ordinary frame=%s", frame)
		}
	case <-time.After(time.Second):
		t.Fatal("ordinary frame not delivered")
	}
	for _, want := range []browserLaunchStatus{browserLaunchConfirmed, browserLaunchEnded} {
		select {
		case got := <-updates:
			if got != want {
				t.Fatalf("update=%d want=%d", got, want)
			}
		case <-time.After(time.Second):
			t.Fatalf("missing update %d", want)
		}
	}
	select {
	case frame := <-session.frames:
		t.Fatalf("control leaked to application frames: %s", frame)
	default:
	}
}

func TestMalformedBrowserLaunchControlFailsClosed(t *testing.T) {
	session, socket := currentBrowserLaunchSession(t)
	socket.messages <- socketMessage{kind: websocket.TextMessage, payload: []byte(`{"type":"event","name":"browser-launch","data":{"receipt":"raw-secret","status":"confirmed"}}`)}
	select {
	case <-session.Done():
	case <-time.After(time.Second):
		t.Fatal("malformed control did not close session")
	}
	if session.EndReason() != SessionProtocolFailed {
		t.Fatalf("reason=%s", session.EndReason())
	}
}

func browserLaunchSessionAt(t *testing.T, rawURL string) (*ConnectedSession, *recordedSessionSocket) {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	_, portText, err := net.SplitHostPort(parsed.Host)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	workload := connectorWorkload(t)
	order := []string{}
	socket := newRecordedSessionSocket(&order)
	active := newRecordedTunnel(&order)
	active.port = port
	session := newConnectedSession(workload.connectorState.receipt, server.AuthenticatedAcceleratorInfo{Capabilities: agent.V1Capabilities()}, connectedIdentity{sessionID: "session-a", instanceID: "instance-a", generation: 1}, socket, active, processResumeClock{})
	if !workload.publishCurrentSession(session) {
		t.Fatal("session publication rejected")
	}
	t.Cleanup(func() { _ = session.Close(context.Background()) })
	return session, socket
}

func exactBrowserLaunchRequest(t *testing.T, request *http.Request, path string) {
	t.Helper()
	requireCreatorRequest(t, request, http.MethodPost, path)
	if request.URL.RawPath != "" || request.URL.ForceQuery || request.Header.Get("Accept-Encoding") != "identity" || len(request.Header.Values("User-Agent")) != 0 || request.Header.Get("Content-Type") != "" {
		t.Error("Browser launch request metadata was not canonical")
	}
	body, err := io.ReadAll(io.LimitReader(request.Body, 2))
	if err != nil || len(body) != 0 {
		t.Error("Browser launch request body was not empty")
	}
}

func TestBrowserLaunchHTTPStrictMintAndRevoke(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	t.Setenv("ALL_PROXY", "http://127.0.0.1:1")
	t.Setenv("NO_PROXY", "")
	ticket := base64.RawURLEncoding.EncodeToString(bytesOf(0x21, 32))
	receipt := base64.RawURLEncoding.EncodeToString(bytesOf(0x22, 32))
	requests := make(chan string, 2)
	httpServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		exactBrowserLaunchRequest(t, request, request.URL.Path)
		requests <- request.URL.Path
		writer.Header().Set("Cache-Control", "no-store")
		switch request.URL.Path {
		case "/api/accelerator-browser-ticket":
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusCreated)
			_, _ = fmt.Fprintf(writer, "{\"ticket\":%q,\"launchReceipt\":%q,\"expiresAt\":%q}\n", ticket, receipt, time.Now().Add(time.Minute).Format(time.RFC3339Nano))
		case "/api/accelerator-browser-session/revoke":
			writer.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer httpServer.Close()
	session, _ := browserLaunchSessionAt(t, httpServer.URL)
	got, err := session.mintBrowserLaunch(context.Background())
	if err != nil || got.ticket != ticket || got.receipt != receipt || !time.Now().Before(got.expiresAt) {
		t.Fatalf("strict mint rejected: %v %v", got, err)
	}
	target, ok := session.browserLaunchURL(got)
	if !ok || target != httpServer.URL+"/accelerator/browser/#ticket="+ticket {
		t.Fatal("strict mint did not create the one fixed fragment URL")
	}
	if !session.revokeBrowserLaunch(context.Background()) {
		t.Fatal("strict revoke response rejected")
	}
	for _, want := range []string{"/api/accelerator-browser-ticket", "/api/accelerator-browser-session/revoke"} {
		if gotPath := <-requests; gotPath != want {
			t.Fatalf("request path=%s want=%s", gotPath, want)
		}
	}
	client := localClient(endpointOf(t, httpServer.URL))
	transport, ok := client.Transport.(*http.Transport)
	if !ok || transport.Proxy != nil || client.Jar != nil || client.CheckRedirect == nil {
		t.Fatal("Browser launch client gained proxy, cookie, or redirect state")
	}
	if fmt.Sprint(got) != "<redacted>" || fmt.Sprintf("%+v", got) != "<redacted>" || strings.Contains(fmt.Sprint(got), ticket) || strings.Contains(target, receipt) {
		t.Fatal("Browser launch value formatting leaked private material")
	}
}

func TestBrowserLaunchHTTPRejectsNoncanonicalMintResponses(t *testing.T) {
	ticket := base64.RawURLEncoding.EncodeToString(bytesOf(0x31, 32))
	receipt := base64.RawURLEncoding.EncodeToString(bytesOf(0x32, 32))
	validBody := func(expiry time.Time) string {
		return fmt.Sprintf("{\"ticket\":%q,\"launchReceipt\":%q,\"expiresAt\":%q}\n", ticket, receipt, expiry.Format(time.RFC3339Nano))
	}
	tests := []struct {
		name        string
		status      int
		contentType string
		cache       []string
		cookie      string
		encoding    string
		location    string
		body        func(time.Time) string
	}{
		{name: "status", status: http.StatusOK, contentType: "application/json", cache: []string{"no-store"}, body: func(now time.Time) string { return validBody(now.Add(time.Minute)) }},
		{name: "redirect", status: http.StatusFound, contentType: "application/json", cache: []string{"no-store"}, location: "/followed", body: func(now time.Time) string { return validBody(now.Add(time.Minute)) }},
		{name: "content type", status: http.StatusCreated, contentType: "application/json; charset=utf-8", cache: []string{"no-store"}, body: func(now time.Time) string { return validBody(now.Add(time.Minute)) }},
		{name: "missing cache", status: http.StatusCreated, contentType: "application/json", body: func(now time.Time) string { return validBody(now.Add(time.Minute)) }},
		{name: "duplicate cache", status: http.StatusCreated, contentType: "application/json", cache: []string{"no-store", "no-store"}, body: func(now time.Time) string { return validBody(now.Add(time.Minute)) }},
		{name: "cookie", status: http.StatusCreated, contentType: "application/json", cache: []string{"no-store"}, cookie: "browser=unsafe", body: func(now time.Time) string { return validBody(now.Add(time.Minute)) }},
		{name: "encoding", status: http.StatusCreated, contentType: "application/json", cache: []string{"no-store"}, encoding: "gzip", body: func(now time.Time) string { return validBody(now.Add(time.Minute)) }},
		{name: "unknown JSON", status: http.StatusCreated, contentType: "application/json", cache: []string{"no-store"}, body: func(now time.Time) string {
			return fmt.Sprintf("{\"ticket\":%q,\"launchReceipt\":%q,\"expiresAt\":%q,\"extra\":true}", ticket, receipt, now.Add(time.Minute).Format(time.RFC3339Nano))
		}},
		{name: "duplicate JSON", status: http.StatusCreated, contentType: "application/json", cache: []string{"no-store"}, body: func(now time.Time) string {
			return fmt.Sprintf("{\"ticket\":%q,\"ticket\":%q,\"launchReceipt\":%q,\"expiresAt\":%q}", ticket, ticket, receipt, now.Add(time.Minute).Format(time.RFC3339Nano))
		}},
		{name: "trailing JSON", status: http.StatusCreated, contentType: "application/json", cache: []string{"no-store"}, body: func(now time.Time) string { return validBody(now.Add(time.Minute)) + `{}` }},
		{name: "oversized JSON", status: http.StatusCreated, contentType: "application/json", cache: []string{"no-store"}, body: func(time.Time) string { return strings.Repeat("x", browserLaunchBodyLimit+1) }},
		{name: "noncanonical ticket", status: http.StatusCreated, contentType: "application/json", cache: []string{"no-store"}, body: func(now time.Time) string {
			return strings.Replace(validBody(now.Add(time.Minute)), ticket, ticket+"=", 1)
		}},
		{name: "noncanonical receipt", status: http.StatusCreated, contentType: "application/json", cache: []string{"no-store"}, body: func(now time.Time) string {
			return strings.Replace(validBody(now.Add(time.Minute)), receipt, receipt+"=", 1)
		}},
		{name: "expired", status: http.StatusCreated, contentType: "application/json", cache: []string{"no-store"}, body: func(now time.Time) string { return validBody(now.Add(-time.Nanosecond)) }},
		{name: "extended expiry", status: http.StatusCreated, contentType: "application/json", cache: []string{"no-store"}, body: func(now time.Time) string { return validBody(now.Add(time.Minute + time.Second)) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int32
			var followed atomic.Int32
			httpServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.URL.Path == "/followed" {
					followed.Add(1)
					return
				}
				calls.Add(1)
				exactBrowserLaunchRequest(t, request, "/api/accelerator-browser-ticket")
				for _, value := range test.cache {
					writer.Header().Add("Cache-Control", value)
				}
				if test.contentType != "" {
					writer.Header().Set("Content-Type", test.contentType)
				}
				if test.cookie != "" {
					writer.Header().Set("Set-Cookie", test.cookie)
				}
				if test.encoding != "" {
					writer.Header().Set("Content-Encoding", test.encoding)
				}
				if test.location != "" {
					writer.Header().Set("Location", test.location)
				}
				writer.WriteHeader(test.status)
				_, _ = io.WriteString(writer, test.body(time.Now()))
			}))
			defer httpServer.Close()
			session, _ := browserLaunchSessionAt(t, httpServer.URL)
			got, err := session.mintBrowserLaunch(context.Background())
			if err == nil || err.Error() != "browser launch unavailable" || calls.Load() != 1 || followed.Load() != 0 {
				t.Fatalf("noncanonical response accepted/followed: %v %v calls=%d followed=%d", got, err, calls.Load(), followed.Load())
			}
			unsafe := []string{ticket, receipt, endpointOf(t, httpServer.URL), test.body(time.Now())}
			for _, value := range []string{err.Error(), fmt.Sprint(got), fmt.Sprintf("%+v", got), fmt.Sprintf("%#v", got)} {
				assertNoCredentialCorpus(t, value, unsafe)
			}
		})
	}
}

func TestBrowserLaunchHTTPRevokeAndCurrentSessionFences(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		cache    []string
		cookie   string
		encoding string
		body     string
		location string
		want     bool
	}{
		{name: "canonical", status: http.StatusNoContent, cache: []string{"no-store"}, want: true},
		{name: "status", status: http.StatusOK, cache: []string{"no-store"}},
		{name: "cache", status: http.StatusNoContent},
		{name: "duplicate cache", status: http.StatusNoContent, cache: []string{"no-store", "no-store"}},
		{name: "cookie", status: http.StatusNoContent, cache: []string{"no-store"}, cookie: "browser=unsafe"},
		{name: "encoding", status: http.StatusNoContent, cache: []string{"no-store"}, encoding: "gzip"},
		{name: "body", status: http.StatusOK, cache: []string{"no-store"}, body: "x"},
		{name: "redirect", status: http.StatusFound, cache: []string{"no-store"}, location: "/followed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var calls, followed atomic.Int32
			httpServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.URL.Path == "/followed" {
					followed.Add(1)
					return
				}
				calls.Add(1)
				exactBrowserLaunchRequest(t, request, "/api/accelerator-browser-session/revoke")
				for _, value := range test.cache {
					writer.Header().Add("Cache-Control", value)
				}
				if test.cookie != "" {
					writer.Header().Set("Set-Cookie", test.cookie)
				}
				if test.encoding != "" {
					writer.Header().Set("Content-Encoding", test.encoding)
				}
				if test.location != "" {
					writer.Header().Set("Location", test.location)
				}
				writer.WriteHeader(test.status)
				_, _ = io.WriteString(writer, test.body)
			}))
			defer httpServer.Close()
			session, _ := browserLaunchSessionAt(t, httpServer.URL)
			if got := session.revokeBrowserLaunch(context.Background()); got != test.want || calls.Load() != 1 || followed.Load() != 0 {
				t.Fatalf("revoke=%v want=%v calls=%d followed=%d", got, test.want, calls.Load(), followed.Load())
			}
		})
	}

	var calls atomic.Int32
	httpServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		exactBrowserLaunchRequest(t, request, request.URL.Path)
		writer.Header().Set("Cache-Control", "no-store")
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusCreated)
		_, _ = fmt.Fprintf(writer, "{\"ticket\":%q,\"launchReceipt\":%q,\"expiresAt\":%q}\n", base64.RawURLEncoding.EncodeToString(bytesOf(0x41, 32)), base64.RawURLEncoding.EncodeToString(bytesOf(0x42, 32)), time.Now().Add(time.Minute).Format(time.RFC3339Nano))
	}))
	defer httpServer.Close()
	session, _ := browserLaunchSessionAt(t, httpServer.URL)
	if err := session.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := session.mintBrowserLaunch(context.Background()); err == nil || err.Error() != "browser launch unavailable" {
		t.Fatalf("non-current mint error=%v", err)
	}
	if session.revokeBrowserLaunch(context.Background()) || calls.Load() != 0 {
		t.Fatal("non-current session reached Browser HTTP")
	}
	receipt := base64.RawURLEncoding.EncodeToString(bytesOf(0x43, 32))
	current, _ := currentBrowserLaunchSession(t)
	_, unregister, ok := current.registerBrowserLaunchWaiter(receipt)
	if !ok {
		t.Fatal("first launch receipt waiter rejected")
	}
	defer unregister()
	if _, _, replay := current.registerBrowserLaunchWaiter(receipt); replay {
		t.Fatal("launch receipt replay registered a second waiter")
	}
}

func TestOpenSerializationAndBrowserHandoff(t *testing.T) {
	receipt := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	ticket := base64.RawURLEncoding.EncodeToString(bytesOf(0x24, 32))
	key, _ := browserLaunchReceiptKey(receipt)
	var mintCalls, revokeCalls atomic.Int32
	httpServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer "+knownCreatorToken || request.Header.Get("Cookie") != "" {
			t.Error("creator boundary mismatch")
		}
		writer.Header().Set("Cache-Control", "no-store")
		switch request.URL.Path {
		case "/api/accelerator-browser-ticket":
			mintCalls.Add(1)
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(writer).Encode(map[string]string{"ticket": ticket, "launchReceipt": receipt, "expiresAt": time.Now().Add(time.Minute).Format(time.RFC3339Nano)})
		case "/api/accelerator-browser-session/revoke":
			revokeCalls.Add(1)
			writer.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer httpServer.Close()
	parsed, _ := url.Parse(httpServer.URL)
	_, portText, _ := net.SplitHostPort(parsed.Host)
	port, _ := strconv.Atoi(portText)

	coordinator, _, clock := newCoordinatorHarness(t)
	workload := connectorWorkload(t)
	order := []string{}
	socket := newRecordedSessionSocket(&order)
	activeTunnel := newRecordedTunnel(&order)
	activeTunnel.port = port
	session := newConnectedSession(workload.connectorState.receipt, server.AuthenticatedAcceleratorInfo{Capabilities: agent.V1Capabilities()}, connectedIdentity{sessionID: "session-a", instanceID: "instance-a", generation: 1}, socket, activeTunnel, clock)
	if !workload.publishCurrentSession(session) {
		t.Fatal("session publication rejected")
	}
	slot := newContextSlot("ctx", 1)
	slot.state, slot.workload, slot.session = CoordinatorActive, workload, session
	coordinator.slots[1] = slot

	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	var navigateCalls atomic.Int32
	navigator := func(target string) bool {
		navigateCalls.Add(1)
		if !strings.HasPrefix(target, httpServer.URL+"/accelerator/browser/#ticket=") || !strings.HasSuffix(target, ticket) {
			t.Errorf("unexpected transient target")
		}
		once.Do(func() { close(entered) })
		<-release
		payload, _ := json.Marshal(map[string]interface{}{"type": "event", "name": "browser-launch", "data": map[string]string{"receipt": key, "status": "confirmed"}})
		socket.messages <- socketMessage{kind: websocket.TextMessage, payload: payload}
		return true
	}
	results := make(chan BrowserOpenResult, 2)
	go func() { results <- coordinator.OpenAcceleratorBrowser(context.Background(), navigator) }()
	<-entered
	go func() { results <- coordinator.OpenAcceleratorBrowser(context.Background(), navigator) }()
	joinDeadline := time.Now().Add(time.Second)
	for {
		slot.mu.Lock()
		joined := slot.demandCount == 2 && slot.launch != nil
		slot.mu.Unlock()
		if joined || time.Now().After(joinDeadline) {
			if !joined {
				t.Fatal("launch follower did not join")
			}
			break
		}
		time.Sleep(time.Millisecond)
	}
	close(release)
	for index := 0; index < 2; index++ {
		if got := <-results; got != BrowserOpened {
			t.Fatalf("open result=%q", string(got))
		}
	}
	if mintCalls.Load() != 1 || navigateCalls.Load() != 1 || revokeCalls.Load() != 0 {
		t.Fatalf("mint=%d navigate=%d revoke=%d", mintCalls.Load(), navigateCalls.Load(), revokeCalls.Load())
	}
	waitCoordinatorState(t, coordinator, CoordinatorBrowserOwned)
	deadline := time.Now().Add(time.Second)
	for {
		slot.mu.Lock()
		handedOff := slot.browserOwned != nil
		slot.mu.Unlock()
		if handedOff || time.Now().After(deadline) {
			if !handedOff {
				t.Fatal("browser handoff did not settle")
			}
			break
		}
		time.Sleep(time.Millisecond)
	}
	if got := coordinator.OpenAcceleratorBrowser(context.Background(), navigator); got != BrowserAlreadyOpen {
		t.Fatalf("browser-owned open=%q", string(got))
	}
	workload.credential.mu.Lock()
	destroyed := workload.credential.destroyed
	workload.credential.mu.Unlock()
	if !destroyed || tunnelEnded(activeTunnel) {
		t.Fatalf("destroyed=%v tunnelEnded=%v", destroyed, tunnelEnded(activeTunnel))
	}
	if result := NewReconnector("v1.2.3").Resume(context.Background(), ResumeRequest{Prior: session, Workload: workload}); result.Availability != Unavailable {
		t.Fatalf("browser handoff resumed: %#v", result)
	}
	activeTunnel.Stop()
	_ = activeTunnel.Wait(context.Background())
}

type blockingBrowserCoordinatorDisposer struct {
	*coordinatorTestServices
	entered chan struct{}
	release chan struct{}
	calls   atomic.Int32
}

func (d *blockingBrowserCoordinatorDisposer) DisposeAfterBrowser(_ context.Context, owned *browserOwnedWorkload) DisposalResult {
	d.calls.Add(1)
	close(d.entered)
	<-d.release
	owned.tunnel.Stop()
	return DisposalResult{Requested: DisposalBrowser, Effective: DisposalBrowser}
}

func TestBrowserHandoffNewDemandWaitsForOneCleanupAndReactivation(t *testing.T) {
	coordinator, services, clock := newCoordinatorHarness(t)
	disposer := &blockingBrowserCoordinatorDisposer{coordinatorTestServices: services, entered: make(chan struct{}), release: make(chan struct{})}
	coordinator.disposer = disposer
	workload := connectorWorkload(t)
	order := []string{}
	socket := newRecordedSessionSocket(&order)
	activeTunnel := newRecordedTunnel(&order)
	session := newConnectedSession(workload.connectorState.receipt, server.AuthenticatedAcceleratorInfo{Capabilities: agent.V1Capabilities()}, connectedIdentity{sessionID: "session-a", instanceID: "instance-a", generation: 1}, socket, activeTunnel, clock)
	if !workload.publishCurrentSession(session) {
		t.Fatal("session publication rejected")
	}
	receipt := base64.RawURLEncoding.EncodeToString(bytesOf(0x71, 32))
	ticket := base64.RawURLEncoding.EncodeToString(bytesOf(0x72, 32))
	key, _ := browserLaunchReceiptKey(receipt)
	httpServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Cache-Control", "no-store")
		switch request.URL.Path {
		case "/api/accelerator-browser-ticket":
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(writer).Encode(map[string]string{"ticket": ticket, "launchReceipt": receipt, "expiresAt": time.Now().Add(time.Minute).Format(time.RFC3339Nano)})
		case "/api/accelerator-browser-session/revoke":
			writer.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer httpServer.Close()
	parsed, _ := url.Parse(httpServer.URL)
	_, portText, _ := net.SplitHostPort(parsed.Host)
	port, _ := strconv.Atoi(portText)
	secondWorkload := connectorWorkload(t)
	secondOrder := []string{}
	secondSocket := newRecordedSessionSocket(&secondOrder)
	secondTunnel := newRecordedTunnel(&secondOrder)
	secondTunnel.port = port
	secondSession := newConnectedSession(secondWorkload.connectorState.receipt, server.AuthenticatedAcceleratorInfo{Capabilities: agent.V1Capabilities()}, connectedIdentity{sessionID: "session-b", instanceID: "instance-b", generation: 1}, secondSocket, secondTunnel, clock)
	if !secondWorkload.publishCurrentSession(secondSession) {
		t.Fatal("second session publication rejected")
	}
	activationEntered, releaseActivation := make(chan struct{}), make(chan struct{})
	services.provisionHook = func(context.Context, int, Request) Result {
		close(activationEntered)
		<-releaseActivation
		return available(secondWorkload)
	}
	services.connectHook = func(context.Context, int, *ProvisionedWorkload) ConnectResult {
		return ConnectResult{Availability: Available, Session: secondSession}
	}
	var unregisters atomic.Int32
	hold := &coordinatorBrowserHold{epoch: 1, session: session, updates: make(chan browserLaunchStatus), unregister: func() { unregisters.Add(1) }}
	slot := newContextSlot("ctx", coordinator.currentEpoch)
	slot.state, slot.workload, slot.session, slot.browserHold = CoordinatorActive, workload, session, hold
	coordinator.mu.Lock()
	coordinator.slots[coordinator.currentEpoch] = slot
	coordinator.mu.Unlock()
	slot.mu.Lock()
	coordinator.startBrowserHandoffLocked(slot)
	slot.mu.Unlock()
	select {
	case <-disposer.entered:
	case <-time.After(time.Second):
		t.Fatal("browser disposal did not start")
	}
	producerCtx, cancelProducers := context.WithTimeout(context.Background(), time.Second)
	coordinator.StopProducers(producerCtx)
	cancelProducers()
	if producerCtx.Err() == context.DeadlineExceeded {
		t.Fatal("normal Browser disposal occupied producer wait group")
	}
	demand := coordinator.AcquireSecretDemand(context.Background(), "ctx")
	if !demand.Accepted || demand.Lease == nil {
		t.Fatal("new Browser-owned demand was rejected")
	}
	if got := coordinator.Snapshot("ctx"); got.State != CoordinatorBrowserOwned || got.Available {
		t.Fatalf("new demand escaped Direct-only Browser ownership: %#v", got)
	}
	if services.provisionCalls.Load() != 0 {
		t.Fatal("reactivation started before exact Browser cleanup")
	}
	close(disposer.release)
	select {
	case <-activationEntered:
	case <-time.After(time.Second):
		t.Fatal("post-cleanup activation did not start")
	}
	if disposer.calls.Load() != 1 || services.provisionCalls.Load() != 1 || unregisters.Load() != 1 {
		t.Fatalf("browser disposal=%d activation=%d unregister=%d", disposer.calls.Load(), services.provisionCalls.Load(), unregisters.Load())
	}
	slot.mu.Lock()
	retainedWorkload, retainedOwned, retainedHold, retainedLaunch := slot.workload, slot.browserOwned, slot.browserHold, slot.launch
	slot.mu.Unlock()
	if retainedWorkload != nil || retainedOwned != nil || retainedHold != nil || retainedLaunch != nil {
		t.Fatal("Browser cleanup retained slot authority")
	}
	close(releaseActivation)
	waitCoordinatorState(t, coordinator, CoordinatorActive)
	if got := coordinator.OpenAcceleratorBrowser(context.Background(), func(string) bool {
		payload, _ := json.Marshal(map[string]interface{}{"type": "event", "name": "browser-launch", "data": map[string]string{"receipt": key, "status": "confirmed"}})
		secondSocket.messages <- socketMessage{kind: websocket.TextMessage, payload: payload}
		return true
	}); got != BrowserOpened {
		t.Fatalf("second Open after exact cleanup=%s", got)
	}
	coordinator.Quiesce(context.Background())
	demand.Lease.Close()
	stopCoordinator(t, coordinator)
	session.launchMu.Lock()
	firstWaiters := len(session.launchWaiters)
	session.launchMu.Unlock()
	secondSession.launchMu.Lock()
	secondWaiters := len(secondSession.launchWaiters)
	secondSession.launchMu.Unlock()
	slot.mu.Lock()
	worker, launch, hold, owned, retained := slot.workerDone, slot.launch, slot.browserHold, slot.browserOwned, slot.workload
	slot.mu.Unlock()
	coordinator.disposalMu.Lock()
	disposals := len(coordinator.disposals)
	coordinator.disposalMu.Unlock()
	if firstWaiters != 0 || secondWaiters != 0 || worker != nil || launch != nil || hold != nil || owned != nil || retained != nil || disposals != 0 {
		t.Fatalf("residue firstWaiters=%d secondWaiters=%d worker=%v launch=%v hold=%v owned=%v workload=%v disposals=%d", firstWaiters, secondWaiters, worker != nil, launch != nil, hold != nil, owned != nil, retained != nil, disposals)
	}
}

func TestBrowserNavigatorIsFencedByContextAndShutdown(t *testing.T) {
	for _, test := range []struct {
		name  string
		fence func(*Coordinator)
	}{
		{name: "context switch", fence: func(c *Coordinator) { c.FenceContextSwitch("ctx") }},
		{name: "quiesce", fence: func(c *Coordinator) { c.Quiesce(context.Background()) }},
		{name: "close", fence: func(c *Coordinator) { c.Close(context.Background()) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			receipt := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
			ticket := base64.RawURLEncoding.EncodeToString(bytesOf(0x61, 32))
			key, _ := browserLaunchReceiptKey(receipt)
			httpServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Cache-Control", "no-store")
				switch request.URL.Path {
				case "/api/accelerator-browser-ticket":
					writer.Header().Set("Content-Type", "application/json")
					writer.WriteHeader(http.StatusCreated)
					_ = json.NewEncoder(writer).Encode(map[string]string{"ticket": ticket, "launchReceipt": receipt, "expiresAt": time.Now().Add(time.Minute).Format(time.RFC3339Nano)})
				case "/api/accelerator-browser-session/revoke":
					writer.WriteHeader(http.StatusNoContent)
				default:
					http.NotFound(writer, request)
				}
			}))
			defer httpServer.Close()
			parsed, _ := url.Parse(httpServer.URL)
			_, portText, _ := net.SplitHostPort(parsed.Host)
			port, _ := strconv.Atoi(portText)
			coordinator, _, clock := newCoordinatorHarness(t)
			workload := connectorWorkload(t)
			order := []string{}
			socket := newRecordedSessionSocket(&order)
			activeTunnel := newRecordedTunnel(&order)
			activeTunnel.port = port
			session := newConnectedSession(workload.connectorState.receipt, server.AuthenticatedAcceleratorInfo{Capabilities: agent.V1Capabilities()}, connectedIdentity{sessionID: "session-a", instanceID: "instance-a", generation: 1}, socket, activeTunnel, clock)
			if !workload.publishCurrentSession(session) {
				t.Fatal("session publication rejected")
			}
			slot := newContextSlot("ctx", coordinator.currentEpoch)
			slot.state, slot.workload, slot.session = CoordinatorActive, workload, session
			coordinator.mu.Lock()
			coordinator.slots[coordinator.currentEpoch] = slot
			coordinator.mu.Unlock()
			entered, releaseNavigator := make(chan struct{}), make(chan struct{})
			result := make(chan BrowserOpenResult, 1)
			go func() {
				result <- coordinator.OpenAcceleratorBrowser(context.Background(), func(string) bool {
					close(entered)
					<-releaseNavigator
					payload, _ := json.Marshal(map[string]interface{}{"type": "event", "name": "browser-launch", "data": map[string]string{"receipt": key, "status": "confirmed"}})
					socket.messages <- socketMessage{kind: websocket.TextMessage, payload: payload}
					return true
				})
			}()
			<-entered
			fenced := make(chan struct{})
			go func() {
				test.fence(coordinator)
				close(fenced)
			}()
			select {
			case <-fenced:
				t.Fatal("fence returned while navigator side effect was blocked")
			case <-time.After(30 * time.Millisecond):
			}
			close(releaseNavigator)
			select {
			case <-fenced:
			case <-time.After(time.Second):
				t.Fatal("fence did not join navigator side effect")
			}
			select {
			case got := <-result:
				if got != BrowserUnavailable {
					t.Fatalf("launch result=%s", got)
				}
			case <-time.After(time.Second):
				t.Fatal("fenced launch did not terminate")
			}
			stopCoordinator(t, coordinator)
		})
	}
}

func bytesOf(value byte, count int) []byte {
	result := make([]byte, count)
	for index := range result {
		result[index] = value
	}
	return result
}
