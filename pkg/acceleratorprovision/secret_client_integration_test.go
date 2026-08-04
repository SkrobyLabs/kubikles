package acceleratorprovision

import (
	"context"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"kubikles/pkg/acceleratorsecret"
	"kubikles/pkg/agent"
	"kubikles/pkg/k8s"
	"kubikles/pkg/server"
)

type loopbackSecretCaller struct {
	mu            sync.Mutex
	calls         []acceleratorRPCLoopbackCall
	blockList     atomic.Bool
	listEntered   chan string
	listRelease   chan struct{}
	releaseOnce   sync.Once
	detailEntered chan string
	detailRelease map[string]chan struct{}
}

type acceleratorRPCLoopbackCall struct {
	context agent.AuthenticatedCallContext
	method  string
}

type countedLoopbackSessionSocket struct {
	connection *websocket.Conn
	readers    atomic.Int32
	writers    atomic.Int32
	maxReaders atomic.Int32
	maxWriters atomic.Int32
	readCalls  atomic.Int32
	writeCalls atomic.Int32
}

func recordLoopbackMaximum(maximum *atomic.Int32, value int32) {
	for prior := maximum.Load(); value > prior; prior = maximum.Load() {
		if maximum.CompareAndSwap(prior, value) {
			return
		}
	}
}

func (s *countedLoopbackSessionSocket) SetWriteDeadline(deadline time.Time) error {
	return s.connection.SetWriteDeadline(deadline)
}
func (s *countedLoopbackSessionSocket) WriteMessage(kind int, payload []byte) error {
	current := s.writers.Add(1)
	recordLoopbackMaximum(&s.maxWriters, current)
	s.writeCalls.Add(1)
	defer s.writers.Add(-1)
	return s.connection.WriteMessage(kind, payload)
}
func (s *countedLoopbackSessionSocket) WriteControl(kind int, payload []byte, deadline time.Time) error {
	return s.connection.WriteControl(kind, payload, deadline)
}
func (s *countedLoopbackSessionSocket) SetPongHandler(handler func(string) error) {
	s.connection.SetPongHandler(handler)
}
func (s *countedLoopbackSessionSocket) SetReadLimit(limit int64) {
	s.connection.SetReadLimit(limit)
}
func (s *countedLoopbackSessionSocket) ReadMessage() (int, []byte, error) {
	current := s.readers.Add(1)
	recordLoopbackMaximum(&s.maxReaders, current)
	s.readCalls.Add(1)
	defer s.readers.Add(-1)
	return s.connection.ReadMessage()
}
func (s *countedLoopbackSessionSocket) Close() error { return s.connection.Close() }

func assertLoopbackPumpOwnership(t *testing.T, socket *countedLoopbackSessionSocket) {
	t.Helper()
	if socket.maxReaders.Load() != 1 || socket.maxWriters.Load() != 1 || socket.readCalls.Load() == 0 || socket.writeCalls.Load() == 0 {
		t.Fatalf("loopback pump ownership readers=%d/%d writers=%d/%d", socket.maxReaders.Load(), socket.readCalls.Load(), socket.maxWriters.Load(), socket.writeCalls.Load())
	}
}

func (c *loopbackSecretCaller) CallMethod(call agent.AuthenticatedCallContext, method string, args []json.RawMessage) (interface{}, error) {
	c.mu.Lock()
	c.calls = append(c.calls, acceleratorRPCLoopbackCall{context: call, method: method})
	c.mu.Unlock()
	switch method {
	case string(acceleratorsecret.OperationListSecretsMetadata):
		if c.blockList.Load() {
			c.listEntered <- rawLoopbackString(args[0])
			<-c.listRelease
		}
		var item acceleratorsecret.SecretListItem
		item.Metadata.Name, item.Metadata.Namespace = "safe", "team"
		item.Type, item.DataKeys = "Opaque", 1
		return []acceleratorsecret.SecretListItem{item}, nil
	case string(acceleratorsecret.OperationGetSecretData):
		if rawLoopbackString(args[0]) == "error" {
			return nil, errors.New("raw-loopback-error-marker")
		}
		return []k8s.DataEntry{{Key: "key", Value: "loopback-detail-marker"}}, nil
	case string(acceleratorsecret.OperationGetSecretYAML):
		name := rawLoopbackString(args[1])
		if release := c.detailRelease[name]; release != nil {
			c.detailEntered <- name
			<-release
		}
		return "yaml-" + name, nil
	case string(acceleratorsecret.OperationCancelListRequest):
		c.releaseOnce.Do(func() { close(c.listRelease) })
		return true, nil
	case string(acceleratorsecret.OperationSubscribeSecretWatcher):
		return acceleratorsecret.SecretWatchSubscription{WatcherSpecID: acceleratorsecret.SecretWatchSpecIDFor(rawLoopbackString(args[0]), rawLoopbackBool(args[1]))}, nil
	case string(acceleratorsecret.OperationUnsubscribeSecretWatcher):
		return nil, nil
	default:
		return nil, errors.New("raw-loopback-unknown-marker")
	}
}

func rawLoopbackString(raw json.RawMessage) string {
	var value string
	_ = json.Unmarshal(raw, &value)
	return value
}

func rawLoopbackBool(raw json.RawMessage) bool {
	var value bool
	_ = json.Unmarshal(raw, &value)
	return value
}

func (c *loopbackSecretCaller) callCopy() []acceleratorRPCLoopbackCall {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]acceleratorRPCLoopbackCall(nil), c.calls...)
}

func TestSecretClientLoopbackAuthenticatedCreatorSession(t *testing.T) {
	tokenText := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	token, err := server.ParseCreatorToken(tokenText)
	if err != nil {
		t.Fatal(err)
	}
	creator, err := server.NewCreatorAuthenticator(server.DeriveCreatorVerifier(token).Encoded())
	if err != nil {
		t.Fatal(err)
	}
	registry := server.NewAcceleratorSessionRegistry("loopback-instance", nil)
	authenticator := &server.AcceleratorWebSocketAuthenticator{Creator: creator, Registry: registry}
	caller := &loopbackSecretCaller{
		listEntered: make(chan string, 1), listRelease: make(chan struct{}), detailEntered: make(chan string, 2),
		detailRelease: map[string]chan struct{}{"reverse-a": make(chan struct{}), "reverse-b": make(chan struct{})},
	}
	options := server.AcceleratorOptions(0, nil, creator.Guard)
	options.MethodAuthorizer = server.NewAcceleratorMethodAuthorizer(agent.CapabilityResolution{Capabilities: agent.V1Capabilities()})
	options.AcceleratorSessions = registry
	options.AcceleratorWebSocketAuthenticator = authenticator
	srv, err := server.NewWithOptions(caller, embed.FS{}, options)
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(srv.Handler())
	defer httpServer.Close()
	header := http.Header{"Authorization": []string{"Bearer " + tokenText}}
	connection, response, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(httpServer.URL, "http")+"/ws", header)
	if response != nil && response.Body != nil {
		response.Body.Close()
	}
	if err != nil {
		t.Fatal(err)
	}
	var connected struct {
		Type string                           `json:"type"`
		Name string                           `json:"name"`
		Data server.AcceleratorConnectedEvent `json:"data"`
	}
	if err := connection.ReadJSON(&connected); err != nil || connected.Type != "event" || connected.Name != "connected" {
		t.Fatalf("connected=%#v err=%v", connected, err)
	}
	workload := connectorWorkload(t)
	order := []string{}
	countedSocket := &countedLoopbackSessionSocket{connection: connection}
	session := newConnectedSession(workload.connectorState.receipt, server.AuthenticatedAcceleratorInfo{Capabilities: agent.V1Capabilities()}, connectedIdentity{sessionID: string(connected.Data.SessionID), instanceID: connected.Data.InstanceID, generation: int(connected.Data.Generation)}, countedSocket, newRecordedTunnel(&order), processResumeClock{})
	if !workload.publishCurrentSession(session) {
		t.Fatal("loopback session did not publish")
	}
	slot := newContextSlot("ctx", 1)
	slot.state, slot.session, slot.workload = CoordinatorActive, session, workload
	slot.sessionLeaseEpoch, slot.sessionLeaseCount, slot.sessionLeaseRevoked = 1, 1, make(chan struct{})
	leaseState := &sessionLeaseState{coordinator: &Coordinator{}, slot: slot, leaseEpoch: 1, session: session, revoked: slot.sessionLeaseRevoked, closedCh: make(chan struct{})}
	lease := &SessionLease{state: leaseState}
	client, err := newSecretRPCClient(lease, strings.NewReader(strings.Repeat("\x01", 16)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		client.Close(context.Background())
		_ = session.Close(context.Background())
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = registry.Close(ctx)
	})

	items, err := client.ListSecretsMetadata(context.Background(), "desktop-list-id", "team", true)
	if err != nil || len(items) != 1 || items[0].Metadata.Name != "safe" || items[0].Metadata.Labels != nil {
		t.Fatalf("loopback list=%#v err=%v", items, err)
	}
	entries, err := client.GetSecretData(context.Background(), "team", "safe")
	if err != nil || len(entries) != 1 || entries[0].Value != "loopback-detail-marker" {
		t.Fatalf("loopback data=%#v err=%v", entries, err)
	}
	if _, err := client.GetSecretData(context.Background(), "error", "safe"); err == nil || err.Error() != string(acceleratorsecret.ReasonRemoteUnavailable) || strings.Contains(err.Error(), "raw-loopback-error-marker") {
		t.Fatalf("loopback safe error=%v", err)
	}

	type yamlResult struct {
		name, value string
		err         error
	}
	yamlResults := make(chan yamlResult, 2)
	for _, name := range []string{"reverse-a", "reverse-b"} {
		go func(name string) {
			value, callErr := client.GetSecretYaml(context.Background(), "team", name)
			yamlResults <- yamlResult{name: name, value: value, err: callErr}
		}(name)
	}
	for range []int{0, 1} {
		<-caller.detailEntered
	}
	close(caller.detailRelease["reverse-b"])
	first := <-yamlResults
	if first.name != "reverse-b" || first.value != "yaml-reverse-b" || first.err != nil {
		t.Fatalf("first reverse result=%#v", first)
	}
	close(caller.detailRelease["reverse-a"])
	second := <-yamlResults
	if second.name != "reverse-a" || second.value != "yaml-reverse-a" || second.err != nil {
		t.Fatalf("second reverse result=%#v", second)
	}

	caller.blockList.Store(true)
	listDone := make(chan error, 1)
	go func() {
		_, callErr := client.ListSecretsMetadata(context.Background(), "desktop-cancel-id", "team", false)
		listDone <- callErr
	}()
	remoteListID := <-caller.listEntered
	canceled, err := client.CancelListRequest(context.Background(), "desktop-cancel-id")
	if err != nil || !canceled || remoteListID == "desktop-cancel-id" {
		t.Fatalf("loopback cancel=%v/%v remote=%q", canceled, err, remoteListID)
	}
	if err := <-listDone; err == nil || err.Error() != string(acceleratorsecret.ReasonCanceled) {
		t.Fatalf("loopback local cancel=%v", err)
	}

	subscription, watchLease, err := client.SubscribeSecretWatcher(context.Background(), "team", false)
	if err != nil {
		t.Fatal(err)
	}
	target := []server.AcceleratorSessionTarget{{SessionID: connected.Data.SessionID, Generation: connected.Data.Generation}}
	registry.EmitEventToTargets(target, server.Event{Type: "event", Name: acceleratorsecret.EventWatcherStatus, Data: acceleratorsecret.SecretWatcherStatus{WatcherSpecID: subscription.WatcherSpecID, Status: acceleratorsecret.WatchStatusConnected}})
	select {
	case event := <-watchLease.Events():
		status, ok := event.Status()
		if !ok || status.WatcherSpecID != subscription.WatcherSpecID {
			t.Fatalf("loopback watch status=%#v/%v", status, ok)
		}
	case <-time.After(time.Second):
		t.Fatal("loopback watch event timed out")
	}
	if err := client.UnsubscribeSecretWatcher(context.Background(), subscription.WatcherSpecID); err != nil {
		t.Fatal(err)
	}
	if watchLease.Reason() != acceleratorsecret.ReasonClosed {
		t.Fatalf("loopback watch reason=%s", watchLease.Reason())
	}

	oldNonce := client.nonce
	oldTarget := target
	replacementConnection, replacementResponse, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(httpServer.URL, "http")+"/ws", header)
	if replacementResponse != nil && replacementResponse.Body != nil {
		replacementResponse.Body.Close()
	}
	if err != nil {
		t.Fatal(err)
	}
	var replacementConnected struct {
		Type string                           `json:"type"`
		Name string                           `json:"name"`
		Data server.AcceleratorConnectedEvent `json:"data"`
	}
	if err := replacementConnection.ReadJSON(&replacementConnected); err != nil || replacementConnected.Type != "event" || replacementConnected.Name != "connected" {
		t.Fatalf("replacement connected=%#v err=%v", replacementConnected, err)
	}
	if replacementConnected.Data.SessionID != connected.Data.SessionID || replacementConnected.Data.Generation != connected.Data.Generation+1 || !replacementConnected.Data.Resumed {
		t.Fatalf("replacement generation=%#v after %#v", replacementConnected.Data, connected.Data)
	}
	select {
	case <-client.Done():
	case <-time.After(time.Second):
		t.Fatal("generation replacement did not terminate old client")
	}
	select {
	case <-session.Done():
	case <-time.After(time.Second):
		t.Fatal("generation replacement did not finish old session")
	}
	if client.Reason() != acceleratorsecret.ReasonSessionUnavailable {
		t.Fatalf("old generation terminal reason=%s", client.Reason())
	}
	client.mu.Lock()
	oldCalls, oldLists, oldSubscriptions := len(client.calls), len(client.lists), len(client.subscriptions)
	client.mu.Unlock()
	if oldCalls != 0 || oldLists != 0 || oldSubscriptions != 0 {
		t.Fatalf("old generation residue=%d/%d/%d", oldCalls, oldLists, oldSubscriptions)
	}
	assertLoopbackPumpOwnership(t, countedSocket)

	replacementSocket := &countedLoopbackSessionSocket{connection: replacementConnection}
	replacementSession := newConnectedSession(workload.connectorState.receipt, server.AuthenticatedAcceleratorInfo{Capabilities: agent.V1Capabilities()}, connectedIdentity{sessionID: string(replacementConnected.Data.SessionID), instanceID: replacementConnected.Data.InstanceID, generation: int(replacementConnected.Data.Generation)}, replacementSocket, newRecordedTunnel(&order), processResumeClock{})
	if !workload.publishCurrentSession(replacementSession) {
		t.Fatal("replacement loopback session did not publish")
	}
	replacementSlot := newContextSlot("ctx", 2)
	replacementSlot.state, replacementSlot.session, replacementSlot.workload = CoordinatorActive, replacementSession, workload
	replacementSlot.sessionLeaseEpoch, replacementSlot.sessionLeaseCount, replacementSlot.sessionLeaseRevoked = 1, 1, make(chan struct{})
	replacementState := &sessionLeaseState{coordinator: &Coordinator{}, slot: replacementSlot, leaseEpoch: 1, session: replacementSession, revoked: replacementSlot.sessionLeaseRevoked, closedCh: make(chan struct{})}
	replacementLease := &SessionLease{state: replacementState}
	replacementClient, err := newSecretRPCClient(replacementLease, strings.NewReader(strings.Repeat("\x02", 16)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		replacementClient.Close(context.Background())
		_ = replacementSession.Close(context.Background())
	})
	if replacementClient.nonce == oldNonce {
		t.Fatal("replacement client reused old call nonce")
	}
	replacementSubscription, replacementWatch, err := replacementClient.SubscribeSecretWatcher(context.Background(), "team", false)
	if err != nil {
		t.Fatal(err)
	}
	var staleItem acceleratorsecret.SecretListItem
	staleItem.Metadata.Name, staleItem.Metadata.Namespace = "stale", "team"
	registry.EmitEventToTargets(oldTarget, server.Event{Type: "event", Name: acceleratorsecret.EventResource, Data: acceleratorsecret.SecretResourceEvent{Type: "ADDED", ResourceType: acceleratorsecret.SecretResourceType, Namespace: "team", WatcherSpecID: replacementSubscription.WatcherSpecID, Resource: staleItem}})
	replacementTarget := []server.AcceleratorSessionTarget{{SessionID: replacementConnected.Data.SessionID, Generation: replacementConnected.Data.Generation}}
	registry.EmitEventToTargets(replacementTarget, server.Event{Type: "event", Name: acceleratorsecret.EventWatcherStatus, Data: acceleratorsecret.SecretWatcherStatus{WatcherSpecID: replacementSubscription.WatcherSpecID, Status: acceleratorsecret.WatchStatusConnected}})
	select {
	case event := <-replacementWatch.Events():
		status, ok := event.Status()
		if !ok || status.Status != acceleratorsecret.WatchStatusConnected {
			t.Fatalf("stale generation crossed replacement fence: %v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("replacement generation barrier event timed out")
	}
	registry.EmitEventToTargets(replacementTarget, server.Event{Type: "event", Name: acceleratorsecret.EventWatcherStatus, Data: acceleratorsecret.SecretWatcherStatus{WatcherSpecID: replacementSubscription.WatcherSpecID, Status: acceleratorsecret.WatchStatusReconnecting}})
	select {
	case event := <-replacementWatch.Events():
		status, ok := event.Status()
		if !ok || status.Status != acceleratorsecret.WatchStatusReconnecting {
			t.Fatalf("explicit loopback gap event=%v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("explicit loopback gap event timed out")
	}
	select {
	case <-replacementWatch.Done():
	case <-time.After(time.Second):
		t.Fatal("explicit loopback gap did not terminate watch")
	}
	if replacementWatch.Reason() != acceleratorsecret.ReasonWatchGap {
		t.Fatalf("explicit loopback gap reason=%s", replacementWatch.Reason())
	}
	replacementClient.Close(context.Background())
	if err := replacementSession.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertLoopbackPumpOwnership(t, replacementSocket)

	calls := caller.callCopy()
	seen := make(map[string]bool)
	var trusted agent.AuthenticatedCallContext
	for _, call := range calls {
		seen[call.method] = true
		if !call.context.IsAuthenticated() {
			t.Fatalf("unauthenticated loopback call=%#v", call)
		}
		if trusted.IsAuthenticated() && trusted != call.context {
			t.Fatalf("trusted context changed=%#v/%#v", trusted, call.context)
		}
		trusted = call.context
	}
	for _, operation := range acceleratorsecret.Operations() {
		if !seen[string(operation)] {
			t.Fatalf("operation %s did not reach loopback caller: %#v", operation, seen)
		}
	}
}
