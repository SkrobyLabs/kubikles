package acceleratorprovision

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"kubikles/pkg/acceleratorsecret"
	"kubikles/pkg/agent"
	"kubikles/pkg/server"
)

type recordedSessionSocket struct {
	mu            sync.Mutex
	order         *[]string
	messages      chan socketMessage
	writes        chan []byte
	closed        chan struct{}
	closeOnce     sync.Once
	readLimit     int64
	readLimitSet  chan struct{}
	readLimitOnce sync.Once
}

type socketMessage struct {
	kind    int
	payload []byte
	err     error
}

type blockedWriterSessionSocket struct {
	closed          chan struct{}
	closeOnce       sync.Once
	writeEntered    chan struct{}
	writeEnteredOne sync.Once
	writeRelease    chan struct{}
	controlCalled   chan struct{}
	controlOnce     sync.Once
	mu              sync.Mutex
	inWrite         bool
	deadlineRaced   bool
}

func newBlockedWriterSessionSocket() *blockedWriterSessionSocket {
	return &blockedWriterSessionSocket{closed: make(chan struct{}), writeEntered: make(chan struct{}), writeRelease: make(chan struct{}), controlCalled: make(chan struct{})}
}

func (s *blockedWriterSessionSocket) SetWriteDeadline(time.Time) error {
	s.mu.Lock()
	s.deadlineRaced = s.deadlineRaced || s.inWrite
	s.mu.Unlock()
	return nil
}
func (*blockedWriterSessionSocket) SetPongHandler(func(string) error) {}
func (s *blockedWriterSessionSocket) WriteMessage(kind int, _ []byte) error {
	if kind != websocket.TextMessage {
		return errors.New("invalid message type")
	}
	s.mu.Lock()
	s.inWrite = true
	s.mu.Unlock()
	s.writeEnteredOne.Do(func() { close(s.writeEntered) })
	<-s.writeRelease
	s.mu.Lock()
	s.inWrite = false
	s.mu.Unlock()
	return nil
}
func (s *blockedWriterSessionSocket) WriteControl(kind int, _ []byte, _ time.Time) error {
	if kind == websocket.CloseMessage {
		s.controlOnce.Do(func() { close(s.controlCalled) })
	}
	return nil
}
func (*blockedWriterSessionSocket) SetReadLimit(int64) {}
func (s *blockedWriterSessionSocket) ReadMessage() (int, []byte, error) {
	<-s.closed
	return 0, nil, errors.New("closed")
}
func (s *blockedWriterSessionSocket) Close() error {
	s.closeOnce.Do(func() { close(s.closed) })
	return nil
}
func (s *blockedWriterSessionSocket) concurrentDeadline() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.deadlineRaced
}

func newRecordedSessionSocket(order *[]string) *recordedSessionSocket {
	return &recordedSessionSocket{order: order, messages: make(chan socketMessage, 128), writes: make(chan []byte, 128), closed: make(chan struct{}), readLimitSet: make(chan struct{})}
}
func (s *recordedSessionSocket) record(value string) {
	s.mu.Lock()
	*s.order = append(*s.order, value)
	s.mu.Unlock()
}
func (s *recordedSessionSocket) SetWriteDeadline(time.Time) error {
	s.record("websocket-deadline")
	return nil
}
func (s *recordedSessionSocket) SetPongHandler(func(string) error) {}
func (s *recordedSessionSocket) WriteMessage(kind int, payload []byte) error {
	if kind != websocket.TextMessage {
		return errors.New("invalid message type")
	}
	s.record("websocket-write")
	select {
	case s.writes <- append([]byte(nil), payload...):
		return nil
	case <-s.closed:
		return errors.New("closed")
	}
}
func (s *recordedSessionSocket) WriteControl(kind int, _ []byte, _ time.Time) error {
	if kind == websocket.CloseMessage {
		s.record("websocket-close-control")
	}
	return nil
}
func (s *recordedSessionSocket) SetReadLimit(limit int64) {
	s.mu.Lock()
	s.readLimit = limit
	s.mu.Unlock()
	s.readLimitOnce.Do(func() { close(s.readLimitSet) })
}
func (s *recordedSessionSocket) readLimitValue() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readLimit
}
func (s *recordedSessionSocket) ReadMessage() (int, []byte, error) {
	select {
	case message := <-s.messages:
		return message.kind, message.payload, message.err
	case <-s.closed:
		return 0, nil, errors.New("raw peer close secret")
	}
}
func (s *recordedSessionSocket) Close() error {
	s.closeOnce.Do(func() {
		s.record("websocket-close")
		close(s.closed)
	})
	return nil
}

type recordedTunnel struct {
	mu       sync.Mutex
	order    *[]string
	done     chan struct{}
	stopOnce sync.Once
	port     int
	block    bool
}

func newRecordedTunnel(order *[]string) *recordedTunnel {
	return &recordedTunnel{order: order, done: make(chan struct{}), port: 43123}
}
func (t *recordedTunnel) record(value string) {
	t.mu.Lock()
	*t.order = append(*t.order, value)
	t.mu.Unlock()
}
func (t *recordedTunnel) Port() int             { return t.port }
func (t *recordedTunnel) Done() <-chan struct{} { return t.done }
func (t *recordedTunnel) Stop() {
	t.stopOnce.Do(func() {
		t.record("port-forward-stop")
		if !t.block {
			close(t.done)
		}
	})
}
func (t *recordedTunnel) Wait(ctx context.Context) error {
	t.record("port-forward-wait")
	select {
	case <-t.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func sessionFixture(t *testing.T, socket sessionSocket, active tunnel) *ConnectedSession {
	t.Helper()
	workload, _, _ := exactWorkloadFixture()
	info := server.AuthenticatedAcceleratorInfo{Capabilities: agent.V1Capabilities()}
	receipt := &workloadReceipt{contextName: workload.ContextName, releaseNamespace: workload.ReleaseNamespace, releaseName: workload.ReleaseName, workloadSessionID: workload.WorkloadSessionID, job: workload.Job, pod: workload.Pod, buildVersion: workload.BuildVersion, imageDigest: workload.ImageDigest, chartDigest: workload.ChartDigest}
	return newConnectedSession(receipt, info, connectedIdentity{sessionID: "session-a", instanceID: "instance-a", generation: 1}, socket, active, processResumeClock{})
}

func TestConnectedSessionSafeSurface(t *testing.T) {
	order := []string{}
	socket := newRecordedSessionSocket(&order)
	active := newRecordedTunnel(&order)
	session := sessionFixture(t, socket, active)
	identity := session.Identity()
	if identity.SessionID != "session-a" || identity.InstanceID != "instance-a" || identity.Generation != 1 {
		t.Fatalf("identity lost connected data: %#v", identity)
	}
	capabilities := session.Capabilities()
	capabilities[0] = "mutated"
	if session.Capabilities()[0] != agent.CapabilitySecretsList {
		t.Fatal("capabilities were not defensive")
	}
	secrets := []string{"127.0.0.1:43123", knownCreatorToken, "Authorization", "raw peer close secret"}
	for _, value := range []interface{}{session, identity, ConnectResult{Availability: Available, Session: session}} {
		for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%q", "%x"} {
			assertNoCredentialCorpus(t, fmt.Sprintf(verb, value), secrets)
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		assertNoCredentialCorpus(t, string(encoded), secrets)
	}
	if err := session.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if socket.readLimitValue() != int64(acceleratorsecret.MaxCreatorResponseFrameBytes) || session.EndReason() != SessionClosed {
		t.Fatalf("unexpected closed state: limit=%d reason=%s", socket.readLimitValue(), session.EndReason())
	}
}

func TestConnectedSessionCloseOrderAndRaces(t *testing.T) {
	order := []string{}
	socket := newRecordedSessionSocket(&order)
	active := newRecordedTunnel(&order)
	session := sessionFixture(t, socket, active)
	const callers = 32
	results := make(chan error, callers)
	for index := 0; index < callers; index++ {
		go func() { results <- session.Close(context.Background()) }()
	}
	for index := 0; index < callers; index++ {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	joined := strings.Join(order, ",")
	closeIndex := strings.Index(joined, "websocket-close")
	stopIndex := strings.Index(joined, "port-forward-stop")
	waitIndex := strings.Index(joined, "port-forward-wait")
	if closeIndex < 0 || stopIndex <= closeIndex || waitIndex <= stopIndex {
		t.Fatalf("transport close order violated: %v", order)
	}
	select {
	case <-session.Done():
	default:
		t.Fatal("Done not closed")
	}

	blockedOrder := []string{}
	blockedSocket := newRecordedSessionSocket(&blockedOrder)
	blockedTunnel := newRecordedTunnel(&blockedOrder)
	blockedTunnel.block = true
	blocked := sessionFixture(t, blockedSocket, blockedTunnel)
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := blocked.Close(cancelled); !errors.Is(err, errSessionCloseTimeout) {
		t.Fatalf("Close exposed non-fixed timeout: %v", err)
	}
}

func TestConnectedSessionTeardownPreservesSoleDataWriter(t *testing.T) {
	t.Run("normal close", func(t *testing.T) {
		socket := newBlockedWriterSessionSocket()
		tunnel := newRecordedTunnel(&[]string{})
		session := sessionFixture(t, socket, tunnel)
		if reason := session.sendApplicationFrame([]byte(`{"type":"call"}`)); reason != "" {
			t.Fatalf("send reason=%s", reason)
		}
		<-socket.writeEntered
		completed := make(chan error, 1)
		go func() { completed <- session.Close(context.Background()) }()
		select {
		case <-socket.controlCalled:
		case <-time.After(time.Second):
			t.Fatal("normal close control was not written")
		}
		close(socket.writeRelease)
		select {
		case err := <-completed:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(time.Second):
			t.Fatal("normal teardown did not complete")
		}
		if socket.concurrentDeadline() {
			t.Fatal("normal teardown mutated write deadline during data write")
		}
	})

	t.Run("browser handoff", func(t *testing.T) {
		socket := newBlockedWriterSessionSocket()
		workload := connectorWorkload(t)
		tunnel := newRecordedTunnel(&[]string{})
		session := newConnectedSession(workload.connectorState.receipt, server.AuthenticatedAcceleratorInfo{Capabilities: agent.V1Capabilities()}, connectedIdentity{sessionID: "session-a", instanceID: "instance-a", generation: 1}, socket, tunnel, processResumeClock{})
		if !workload.publishCurrentSession(session) {
			t.Fatal("session publication rejected")
		}
		if reason := session.sendApplicationFrame([]byte(`{"type":"call"}`)); reason != "" {
			t.Fatalf("send reason=%s", reason)
		}
		<-socket.writeEntered
		type handoffResult struct {
			owned *browserOwnedWorkload
			ok    bool
		}
		completed := make(chan handoffResult, 1)
		go func() {
			owned, ok := session.detachCreatorForBrowser(context.Background())
			completed <- handoffResult{owned: owned, ok: ok}
		}()
		select {
		case <-socket.controlCalled:
		case <-time.After(time.Second):
			t.Fatal("Browser handoff control was not written")
		}
		close(socket.writeRelease)
		select {
		case result := <-completed:
			if !result.ok || result.owned == nil {
				t.Fatal("Browser handoff did not complete")
			}
			result.owned.tunnel.Stop()
		case <-time.After(time.Second):
			t.Fatal("Browser handoff remained blocked")
		}
		if socket.concurrentDeadline() {
			t.Fatal("Browser handoff mutated write deadline during data write")
		}
	})
}

func TestConnectedSessionPumpFailsClosed(t *testing.T) {
	tests := []struct {
		name    string
		message socketMessage
		reason  SessionEndReason
	}{
		{"peer close", socketMessage{err: errors.New("peer")}, SessionPeerClosed},
		{"binary", socketMessage{kind: websocket.BinaryMessage, payload: []byte("{}")}, SessionProtocolFailed},
		{"malformed", socketMessage{kind: websocket.TextMessage, payload: []byte("{")}, SessionProtocolFailed},
		{"oversized", socketMessage{kind: websocket.TextMessage, payload: []byte(`"` + strings.Repeat("x", int(server.AcceleratorSocketReadLimit)) + `"`)}, SessionProtocolFailed},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			order := []string{}
			socket := newRecordedSessionSocket(&order)
			active := newRecordedTunnel(&order)
			socket.messages <- test.message
			session := sessionFixture(t, socket, active)
			select {
			case <-session.Done():
			case <-time.After(time.Second):
				t.Fatal("session did not terminate")
			}
			if session.EndReason() != test.reason {
				t.Fatalf("reason=%s want=%s", session.EndReason(), test.reason)
			}
		})
	}
}

func TestConnectedSessionFrameOverflowFailsClosed(t *testing.T) {
	order := []string{}
	socket := newRecordedSessionSocket(&order)
	active := newRecordedTunnel(&order)
	for index := 0; index < 65; index++ {
		socket.messages <- socketMessage{kind: websocket.TextMessage, payload: []byte(`{"event":true}`)}
	}
	session := sessionFixture(t, socket, active)
	select {
	case <-session.Done():
	case <-time.After(time.Second):
		t.Fatal("overflow did not terminate")
	}
	if session.EndReason() != SessionProtocolFailed {
		t.Fatalf("overflow reason=%s", session.EndReason())
	}
}

func TestDisposalClosesRegisteredOriginalAndResumedSessionsInOrder(t *testing.T) {
	for _, generation := range []int{1, 2} {
		t.Run(fmt.Sprintf("generation-%d", generation), func(t *testing.T) {
			workload := connectorWorkload(t)
			order := []string{}
			session := newConnectedSession(workload.connectorState.receipt, server.AuthenticatedAcceleratorInfo{Capabilities: agent.V1Capabilities()}, connectedIdentity{sessionID: "session", instanceID: "instance", generation: generation}, newRecordedSessionSocket(&order), newRecordedTunnel(&order), &fakeResumeClock{now: time.Date(2026, 8, 2, 20, 0, 0, 0, time.UTC)})
			if !workload.publishCurrentSession(session) {
				t.Fatal("publication")
			}
			service := NewDisposalService(nil)
			service.cleanupOwned = func(context.Context, *workloadReceipt) (OwnershipStatus, UninstallStatus, DisappearanceStatus) {
				return OwnershipAlreadyGone, UninstallNotNeeded, DisappearanceSucceeded
			}
			if result := service.DisposeNow(context.Background(), workload); result.Quiescence != QuiescenceSucceeded {
				t.Fatalf("result=%#v", result)
			}
			websocketClose, tunnelStop := eventIndex(order, "websocket-close"), eventIndex(order, "port-forward-stop")
			if websocketClose < 0 || tunnelStop <= websocketClose || workload.connectorState.currentSession != nil {
				t.Fatalf("order=%v current=%p", order, workload.connectorState.currentSession)
			}
		})
	}
}

func TestStaleSessionTerminalCallbackCannotClearNewerCurrent(t *testing.T) {
	workload := connectorWorkload(t)
	oldOrder, newOrder := []string{}, []string{}
	clock := &fakeResumeClock{now: time.Date(2026, 8, 2, 20, 0, 0, 0, time.UTC)}
	old := newConnectedSession(workload.connectorState.receipt, server.AuthenticatedAcceleratorInfo{Capabilities: agent.V1Capabilities()}, connectedIdentity{sessionID: "old", instanceID: "instance", generation: 1}, newRecordedSessionSocket(&oldOrder), newRecordedTunnel(&oldOrder), clock)
	newer := newConnectedSession(workload.connectorState.receipt, server.AuthenticatedAcceleratorInfo{Capabilities: agent.V1Capabilities()}, connectedIdentity{sessionID: "new", instanceID: "instance", generation: 2}, newRecordedSessionSocket(&newOrder), newRecordedTunnel(&newOrder), clock)
	if !workload.publishCurrentSession(old) || !workload.publishCurrentSession(newer) {
		t.Fatal("publication")
	}
	if err := old.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	workload.connectorState.mu.Lock()
	current := workload.connectorState.currentSession
	workload.connectorState.mu.Unlock()
	if current != newer {
		t.Fatalf("stale callback cleared newer current: %p != %p", current, newer)
	}
	_ = newer.Close(context.Background())
}
