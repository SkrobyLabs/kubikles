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
	"kubikles/pkg/agent"
	"kubikles/pkg/server"
)

type recordedSessionSocket struct {
	mu        sync.Mutex
	order     *[]string
	messages  chan socketMessage
	closed    chan struct{}
	closeOnce sync.Once
	readLimit int64
}

type socketMessage struct {
	kind    int
	payload []byte
	err     error
}

func newRecordedSessionSocket(order *[]string) *recordedSessionSocket {
	return &recordedSessionSocket{order: order, messages: make(chan socketMessage, 128), closed: make(chan struct{})}
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
func (s *recordedSessionSocket) WriteControl(kind int, _ []byte, _ time.Time) error {
	if kind == websocket.CloseMessage {
		s.record("websocket-close-control")
	}
	return nil
}
func (s *recordedSessionSocket) SetReadLimit(limit int64) { s.readLimit = limit }
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
	if socket.readLimit != server.AcceleratorSocketReadLimit || session.EndReason() != SessionClosed {
		t.Fatalf("unexpected closed state: limit=%d reason=%s", socket.readLimit, session.EndReason())
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
