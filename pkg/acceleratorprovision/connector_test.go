package acceleratorprovision

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/httpstream"
	"k8s.io/client-go/kubernetes/fake"
	"kubikles/pkg/agent"
	"kubikles/pkg/server"
)

func connectorWorkload(t *testing.T) *ProvisionedWorkload {
	t.Helper()
	workload, job, pod := exactWorkloadFixture()
	credential := knownCredential(t)
	workload.credential = credential
	workload.snapshot = fakeSnapshot{identity: "snapshot", namespace: workload.ReleaseNamespace, client: fakeClientset(job, pod)}
	workload.connectorState = &workloadConnectorState{}
	workload.connectorState.receipt = &workloadReceipt{contextName: workload.ContextName, releaseNamespace: workload.ReleaseNamespace, releaseName: workload.ReleaseName, workloadSessionID: workload.WorkloadSessionID, job: workload.Job, pod: workload.Pod, buildVersion: workload.BuildVersion, imageDigest: workload.ImageDigest, chartDigest: workload.ChartDigest, snapshot: workload.snapshot, owner: workload}
	return workload
}

type causedRecordedTunnel struct {
	*recordedTunnel
	cause error
}

func (t *causedRecordedTunnel) terminalCause() error { return t.cause }

type resumeLifecycleTrace struct {
	mu            sync.Mutex
	events        []string
	activeTunnels int
	activeSockets int
}

func (t *resumeLifecycleTrace) record(event string) {
	t.mu.Lock()
	t.events = append(t.events, event)
	t.mu.Unlock()
}

func (t *resumeLifecycleTrace) counts() (int, int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.activeTunnels, t.activeSockets
}

func (t *resumeLifecycleTrace) snapshot() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]string(nil), t.events...)
}

type resumeLifecycleTunnel struct {
	trace              *resumeLifecycleTrace
	done               chan struct{}
	doneOnce           sync.Once
	waitOnce           sync.Once
	socketOpened       *atomic.Bool
	socketClosed       <-chan struct{}
	waitForSocketClose bool
	port               int
	cause              error
}

func (t *resumeLifecycleTunnel) Port() int             { return t.port }
func (t *resumeLifecycleTunnel) Done() <-chan struct{} { return t.done }
func (t *resumeLifecycleTunnel) terminalCause() error  { return t.cause }
func (t *resumeLifecycleTunnel) Stop() {
	if t.waitForSocketClose && t.socketOpened.Load() {
		select {
		case <-t.socketClosed:
		case <-time.After(time.Second):
		}
	}
	t.trace.record("tunnel-stop")
	t.doneOnce.Do(func() { close(t.done) })
}
func (t *resumeLifecycleTunnel) Wait(context.Context) error {
	t.trace.record("tunnel-wait")
	t.waitOnce.Do(func() {
		t.trace.mu.Lock()
		t.trace.activeTunnels--
		t.trace.mu.Unlock()
	})
	return nil
}

func eventIndex(events []string, wanted string) int {
	for index, event := range events {
		if event == wanted {
			return index
		}
	}
	return -1
}

func TestResumeFailurePhaseCleanupBeforeRetry(t *testing.T) {
	for _, failurePhase := range []string{"pre-tunnel", "ready", "post-ready", "info", "policy", "websocket", "frame", "final-validation", "publication-probe"} {
		t.Run(failurePhase, func(t *testing.T) {
			trace := &resumeLifecycleTrace{}
			var socketOpened atomic.Bool
			socketClosed := make(chan struct{})
			var socketCloseOnce sync.Once
			closeSocket := func() {
				socketCloseOnce.Do(func() {
					trace.mu.Lock()
					trace.events = append(trace.events, "socket-close")
					trace.activeSockets--
					trace.mu.Unlock()
					close(socketClosed)
				})
			}
			closeForProbe := make(chan struct{})
			upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
			protocol := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				switch request.URL.Path {
				case "/api/accelerator-info":
					writer.Header().Set("Content-Type", "application/json")
					writer.Header().Set("Cache-Control", "no-store")
					if failurePhase == "info" {
						writer.WriteHeader(http.StatusServiceUnavailable)
						return
					}
					_ = json.NewEncoder(writer).Encode(server.AuthenticatedAcceleratorInfo{Runtime: "accelerator", Build: agent.BuildIdentity{BuildVersion: "v1.2.3"}, InstanceID: "instance-a", Capabilities: agent.V1Capabilities(), CapabilityDiagnostics: []agent.CapabilityDiagnostic{}})
				case "/api/call":
					writer.Header().Set("Content-Type", "application/json")
					writer.Header().Set("Cache-Control", "no-store")
					if failurePhase == "policy" {
						writer.WriteHeader(http.StatusServiceUnavailable)
						return
					}
					writer.WriteHeader(http.StatusForbidden)
					_, _ = writer.Write([]byte("{\"error\":\"forbidden\"}\n"))
				case "/ws":
					if failurePhase == "websocket" {
						writer.WriteHeader(http.StatusServiceUnavailable)
						return
					}
					connection, err := upgrader.Upgrade(writer, request, nil)
					if err != nil {
						return
					}
					socketOpened.Store(true)
					trace.mu.Lock()
					trace.events = append(trace.events, "socket-open")
					trace.activeSockets++
					trace.mu.Unlock()
					defer closeSocket()
					if failurePhase == "frame" {
						if tcp, ok := connection.UnderlyingConn().(*net.TCPConn); ok {
							_ = tcp.SetLinger(0)
						}
						_ = connection.Close()
						return
					}
					_ = connection.WriteJSON(map[string]interface{}{"type": "event", "name": "connected", "data": map[string]interface{}{"sessionId": "session-a", "instanceId": "instance-a", "generation": 2, "resumed": true}})
					if failurePhase == "publication-probe" {
						<-closeForProbe
						if tcp, ok := connection.UnderlyingConn().(*net.TCPConn); ok {
							_ = tcp.SetLinger(0)
						}
						_ = connection.Close()
						return
					}
					defer connection.Close()
					for {
						if _, _, err = connection.ReadMessage(); err != nil {
							return
						}
					}
				}
			}))
			defer protocol.Close()

			clock := &fakeResumeClock{now: time.Now()}
			prior, workload := resumableSessionFixture(t, clock)
			order := []string{}
			connector := connectorForEndpoint(t, protocol.URL, func(context.Context, *ProvisionedWorkload, ContextSnapshot) UnavailableReason { return "" }, &order)
			validationCalls := 0
			connector.validateDetailed = func(context.Context, *ProvisionedWorkload, ContextSnapshot) workloadValidationResult {
				validationCalls++
				failAt := map[string]int{"pre-tunnel": 1, "post-ready": 2, "final-validation": 3}[failurePhase]
				if validationCalls == failAt && failAt != 0 {
					return workloadValidationResult{reason: TimedOut, cause: context.DeadlineExceeded}
				}
				return workloadValidationResult{}
			}
			_, portText, err := net.SplitHostPort(strings.TrimPrefix(protocol.URL, "http://"))
			if err != nil {
				t.Fatal(err)
			}
			port, err := strconv.Atoi(portText)
			if err != nil {
				t.Fatal(err)
			}
			connector.startTunnel = func(context.Context, ContextSnapshot, string, string) (tunnel, error) {
				trace.mu.Lock()
				trace.activeTunnels++
				trace.events = append(trace.events, "tunnel-open")
				trace.mu.Unlock()
				active := &resumeLifecycleTunnel{trace: trace, done: make(chan struct{}), socketOpened: &socketOpened, socketClosed: socketClosed, waitForSocketClose: failurePhase == "frame" || failurePhase == "final-validation" || failurePhase == "publication-probe", port: port}
				if failurePhase == "ready" {
					active.cause = &httpstream.UpgradeFailureError{Cause: errors.New("redacted")}
					active.doneOnce.Do(func() { close(active.done) })
				}
				return active, nil
			}
			if failurePhase == "publication-probe" {
				connector.beforePublish = func() {
					close(closeForProbe)
					<-socketClosed
				}
			}
			reconnector := NewReconnector("v1.2.3")
			reconnector.connector = connector
			attempts := 0
			reconnector.attempt = func(ctx context.Context, lease connectorLease, expectation connectionExpectation) (*ConnectedSession, *connectAttemptFailure) {
				attempts++
				if attempts > 1 {
					tunnels, sockets := trace.counts()
					if tunnels != 0 || sockets != 0 {
						t.Fatalf("next attempt overlapped resources: tunnels=%d sockets=%d events=%v", tunnels, sockets, trace.snapshot())
					}
					return nil, &connectAttemptFailure{phase: attemptConnectedFrame, cause: protocolAttemptError{}}
				}
				return connector.connectExactAttempt(ctx, lease, expectation)
			}
			slept := false
			clock.onSleep = func(time.Duration) {
				slept = true
				if workload.connectorState.active != 0 {
					t.Fatal("resume lease retained before retry sleep")
				}
				tunnels, sockets := trace.counts()
				if tunnels != 0 || sockets != 0 {
					t.Fatalf("resources retained before retry sleep: tunnels=%d sockets=%d events=%v", tunnels, sockets, trace.snapshot())
				}
			}
			result := reconnector.Resume(context.Background(), ResumeRequest{Prior: prior, Workload: workload})
			if result.Reason != ResumeProtocolFailed || attempts != 2 || !slept {
				t.Fatalf("result=%#v attempts=%d slept=%t events=%v", result, attempts, slept, trace.snapshot())
			}
			events := trace.snapshot()
			stop, wait := eventIndex(events, "tunnel-stop"), eventIndex(events, "tunnel-wait")
			if failurePhase == "pre-tunnel" {
				if stop != -1 || wait != -1 {
					t.Fatalf("pre-tunnel failure opened cleanup resources: %v", events)
				}
				return
			}
			if stop < 0 || wait < 0 || stop > wait {
				t.Fatalf("tunnel cleanup order=%v", events)
			}
			if socketOpened.Load() {
				closed := eventIndex(events, "socket-close")
				if closed < 0 || closed > stop {
					t.Fatalf("socket did not close before tunnel cleanup: %v", events)
				}
			}
		})
	}
}

func TestResumeTunnelFailuresFlowThroughConnectorClassification(t *testing.T) {
	tests := []struct {
		name      string
		cause     error
		postReady bool
		wantRetry bool
	}{
		{name: "upgrade before ready", cause: &httpstream.UpgradeFailureError{Cause: errors.New("raw upgrade")}, wantRetry: true},
		{name: "https proxy before ready", cause: errors.New("proxy: unknown scheme: https"), wantRetry: true},
		{name: "generic before ready", cause: errors.New("raw generic")},
		{name: "upgrade after ready", cause: &httpstream.UpgradeFailureError{Cause: errors.New("raw upgrade")}, postReady: true, wantRetry: true},
		{name: "https proxy after ready", cause: errors.New("proxy: unknown scheme: https"), postReady: true, wantRetry: true},
		{name: "generic after ready", cause: errors.New("raw generic"), postReady: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			clock := &fakeResumeClock{now: time.Now()}
			prior, workload := resumableSessionFixture(t, clock)
			reconnector := NewReconnector("v1.2.3")
			reconnector.connector.validate = func(context.Context, *ProvisionedWorkload, ContextSnapshot) UnavailableReason { return "" }
			reconnector.connector.validateDetailed = nil
			starts := 0
			reconnector.connector.startTunnel = func(context.Context, ContextSnapshot, string, string) (tunnel, error) {
				starts++
				if starts > 1 {
					return nil, errors.New("terminal second attempt")
				}
				if !test.postReady {
					return nil, test.cause
				}
				order := []string{}
				active := newRecordedTunnel(&order)
				active.block = true
				close(active.done)
				return &causedRecordedTunnel{recordedTunnel: active, cause: test.cause}, nil
			}
			result := reconnector.Resume(context.Background(), ResumeRequest{Prior: prior, Workload: workload})
			if test.wantRetry {
				if starts != 2 || len(clock.sleeps) != 1 || result.Reason != ResumeTransportUnavailable {
					t.Fatalf("starts=%d sleeps=%v result=%#v", starts, clock.sleeps, result)
				}
			} else if starts != 1 || len(clock.sleeps) != 0 || result.Reason != ResumeTransportUnavailable {
				t.Fatalf("starts=%d sleeps=%v result=%#v", starts, clock.sleeps, result)
			}
		})
	}
}

func TestResumeReadyTimeoutSettlementControlsRetryOverlap(t *testing.T) {
	for _, test := range []struct {
		name       string
		first      error
		wantStarts int
		wantSleeps int
	}{
		{name: "cleanup unsettled is terminal", first: errTunnelCleanupUnsettledAttempt, wantStarts: 1},
		{name: "cooperative settled timeout retries", first: errTunnelTransientAttempt, wantStarts: 2, wantSleeps: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			clock := &fakeResumeClock{now: time.Now()}
			prior, workload := resumableSessionFixture(t, clock)
			reconnector := NewReconnector("v1.2.3")
			reconnector.connector.validate = func(context.Context, *ProvisionedWorkload, ContextSnapshot) UnavailableReason { return "" }
			reconnector.connector.validateDetailed = nil
			var active atomic.Int32
			starts := 0
			reconnector.connector.startTunnel = func(context.Context, ContextSnapshot, string, string) (tunnel, error) {
				if active.Load() != 0 {
					t.Fatal("new ready attempt overlapped unsettled forwarder")
				}
				starts++
				active.Add(1)
				active.Add(-1)
				if starts == 1 {
					return nil, test.first
				}
				return nil, errTunnelGenericAttempt
			}
			clock.onSleep = func(time.Duration) {
				if active.Load() != 0 || workload.connectorState.active != 0 {
					t.Fatal("settled timeout retained ownership at retry sleep")
				}
			}
			result := reconnector.Resume(context.Background(), ResumeRequest{Prior: prior, Workload: workload})
			if result.Reason != ResumeTransportUnavailable || starts != test.wantStarts || len(clock.sleeps) != test.wantSleeps {
				t.Fatalf("result=%#v starts=%d sleeps=%v", result, starts, clock.sleeps)
			}
		})
	}
}

func TestResumeLostPodTunnelRetriesAfterExactRevalidation(t *testing.T) {
	clock := &fakeResumeClock{now: time.Now()}
	prior, workload := resumableSessionFixture(t, clock)
	reconnector := NewReconnector("v1.2.3")
	validations := 0
	reconnector.connector.validate = func(context.Context, *ProvisionedWorkload, ContextSnapshot) UnavailableReason {
		validations++
		return ""
	}
	reconnector.connector.validateDetailed = nil
	starts := 0
	reconnector.connector.startTunnel = func(context.Context, ContextSnapshot, string, string) (tunnel, error) {
		starts++
		if starts == 1 {
			order := []string{}
			active := newRecordedTunnel(&order)
			active.block = true
			close(active.done)
			return &causedRecordedTunnel{recordedTunnel: active, cause: errTunnelTransientAttempt}, nil
		}
		if validations != 3 {
			t.Fatalf("retry started before exact revalidation: validations=%d", validations)
		}
		return nil, errTunnelGenericAttempt
	}
	result := reconnector.Resume(context.Background(), ResumeRequest{Prior: prior, Workload: workload})
	if result.Reason != ResumeTransportUnavailable || starts != 2 || validations != 3 || !reflect.DeepEqual(clock.sleeps, []time.Duration{time.Second}) {
		t.Fatalf("result=%#v starts=%d validations=%d sleeps=%v", result, starts, validations, clock.sleeps)
	}
}

func noPongConnectorProtocolServer(t *testing.T, generation int, resumed bool) (*httptest.Server, *atomic.Int32, <-chan struct{}) {
	t.Helper()
	var pings atomic.Int32
	pingSeen := make(chan struct{})
	var pingOnce sync.Once
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	protocol := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer "+knownCreatorToken {
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch request.URL.Path {
		case "/api/accelerator-info":
			writer.Header().Set("Content-Type", "application/json")
			writer.Header().Set("Cache-Control", "no-store")
			_ = json.NewEncoder(writer).Encode(server.AuthenticatedAcceleratorInfo{Runtime: "accelerator", Build: agent.BuildIdentity{BuildVersion: "v1.2.3"}, InstanceID: "instance-a", Capabilities: agent.V1Capabilities(), CapabilityDiagnostics: []agent.CapabilityDiagnostic{}})
		case "/api/call":
			writer.Header().Set("Content-Type", "application/json")
			writer.Header().Set("Cache-Control", "no-store")
			writer.WriteHeader(http.StatusForbidden)
			_, _ = writer.Write([]byte("{\"error\":\"forbidden\"}\n"))
		case "/ws":
			connection, err := upgrader.Upgrade(writer, request, nil)
			if err != nil {
				return
			}
			defer connection.Close()
			connection.SetPingHandler(func(string) error {
				pings.Add(1)
				pingOnce.Do(func() { close(pingSeen) })
				return nil
			})
			_ = connection.WriteJSON(map[string]interface{}{"type": "event", "name": "connected", "data": map[string]interface{}{"sessionId": "session-a", "instanceId": "instance-a", "generation": generation, "resumed": resumed}})
			for {
				if _, _, err = connection.ReadMessage(); err != nil {
					return
				}
			}
		}
	}))
	return protocol, &pings, pingSeen
}

func TestFreshConnectDoesNotRequireCandidatePong(t *testing.T) {
	protocol, pings, _ := noPongConnectorProtocolServer(t, 1, false)
	defer protocol.Close()
	order := []string{}
	connector := connectorForEndpoint(t, protocol.URL, func(context.Context, *ProvisionedWorkload, ContextSnapshot) UnavailableReason { return "" }, &order)
	result := connector.Connect(context.Background(), connectorWorkload(t))
	if result.Session == nil || result.Availability != Available || pings.Load() != 0 {
		t.Fatalf("result=%#v pings=%d", result, pings.Load())
	}
	if err := result.Session.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestResumeCandidatePongWaitUsesHandshakeBoundAndCancellationWins(t *testing.T) {
	protocol, _, pingSeen := noPongConnectorProtocolServer(t, 2, true)
	defer protocol.Close()
	clock := &fakeResumeClock{now: time.Now()}
	prior, workload := resumableSessionFixture(t, clock)
	order := []string{}
	reconnector := NewReconnector("v1.2.3")
	reconnector.connector = connectorForEndpoint(t, protocol.URL, func(context.Context, *ProvisionedWorkload, ContextSnapshot) UnavailableReason { return "" }, &order)
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan ResumeResult, 1)
	go func() {
		result <- reconnector.Resume(ctx, ResumeRequest{Prior: prior, Workload: workload})
	}()
	select {
	case <-pingSeen:
	case <-time.After(time.Second):
		cancel()
		t.Fatal("resume candidate did not send Ping")
	}
	select {
	case early := <-result:
		cancel()
		t.Fatalf("resume retained the former 250ms probe bound: %#v", early)
	case <-time.After(300 * time.Millisecond):
	}
	cancel()
	if got := <-result; got.Reason != ResumeCancelled || got.Session != nil || len(clock.sleeps) != 0 {
		t.Fatalf("result=%#v sleeps=%v", got, clock.sleeps)
	}
}

func TestResumePeerCloseDuringFinalValidationAdvancesFenceAndRetries(t *testing.T) {
	var sockets atomic.Int32
	closeFirst := make(chan struct{})
	firstClosed := make(chan struct{})
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	protocol := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer "+knownCreatorToken {
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch request.URL.Path {
		case "/api/accelerator-info":
			writer.Header().Set("Content-Type", "application/json")
			writer.Header().Set("Cache-Control", "no-store")
			_ = json.NewEncoder(writer).Encode(server.AuthenticatedAcceleratorInfo{Runtime: "accelerator", Build: agent.BuildIdentity{BuildVersion: "v1.2.3"}, InstanceID: "instance-a", Capabilities: agent.V1Capabilities(), CapabilityDiagnostics: []agent.CapabilityDiagnostic{}})
		case "/api/call":
			writer.Header().Set("Content-Type", "application/json")
			writer.Header().Set("Cache-Control", "no-store")
			writer.WriteHeader(http.StatusForbidden)
			_, _ = writer.Write([]byte("{\"error\":\"forbidden\"}\n"))
		case "/ws":
			connection, err := upgrader.Upgrade(writer, request, nil)
			if err != nil {
				return
			}
			generation := sockets.Add(1) + 1
			_ = connection.WriteJSON(map[string]interface{}{"type": "event", "name": "connected", "data": map[string]interface{}{"sessionId": "session-a", "instanceId": "instance-a", "generation": generation, "resumed": true}})
			if generation == 2 {
				<-closeFirst
				if tcp, ok := connection.UnderlyingConn().(*net.TCPConn); ok {
					_ = tcp.SetLinger(0)
				}
				_ = connection.Close()
				close(firstClosed)
				return
			}
			defer connection.Close()
			for {
				if _, _, err = connection.ReadMessage(); err != nil {
					return
				}
			}
		}
	}))
	defer protocol.Close()
	clock := &fakeResumeClock{now: time.Now()}
	prior, workload := resumableSessionFixture(t, clock)
	order := []string{}
	validations := 0
	connector := connectorForEndpoint(t, protocol.URL, func(context.Context, *ProvisionedWorkload, ContextSnapshot) UnavailableReason {
		validations++
		if validations == 3 {
			close(closeFirst)
			<-firstClosed
		}
		return ""
	}, &order)
	reconnector := NewReconnector("v1.2.3")
	reconnector.connector = connector
	result := reconnector.Resume(context.Background(), ResumeRequest{Prior: prior, Workload: workload})
	if result.Session == nil || result.Session.Identity().Generation != 3 || !reflect.DeepEqual(clock.sleeps, []time.Duration{time.Second}) {
		t.Fatalf("result=%#v identity=%#v sleeps=%v validations=%d", result, func() SessionIdentity {
			if result.Session != nil {
				return result.Session.Identity()
			}
			return SessionIdentity{}
		}(), clock.sleeps, validations)
	}
	if err := result.Session.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestResumeQueuedGracefulCloseFailsCandidateAndRetriesAboveFence(t *testing.T) {
	var sockets atomic.Int32
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	protocol := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/accelerator-info":
			writer.Header().Set("Content-Type", "application/json")
			writer.Header().Set("Cache-Control", "no-store")
			_ = json.NewEncoder(writer).Encode(server.AuthenticatedAcceleratorInfo{Runtime: "accelerator", Build: agent.BuildIdentity{BuildVersion: "v1.2.3"}, InstanceID: "instance-a", Capabilities: agent.V1Capabilities(), CapabilityDiagnostics: []agent.CapabilityDiagnostic{}})
		case "/api/call":
			writer.Header().Set("Content-Type", "application/json")
			writer.Header().Set("Cache-Control", "no-store")
			writer.WriteHeader(http.StatusForbidden)
			_, _ = writer.Write([]byte("{\"error\":\"forbidden\"}\n"))
		case "/ws":
			connection, err := upgrader.Upgrade(writer, request, nil)
			if err != nil {
				return
			}
			defer connection.Close()
			generation := sockets.Add(1) + 1
			_ = connection.WriteJSON(map[string]interface{}{"type": "event", "name": "connected", "data": map[string]interface{}{"sessionId": "session-a", "instanceId": "instance-a", "generation": generation, "resumed": true}})
			if generation == 2 {
				_ = connection.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(time.Second))
				_, _, _ = connection.ReadMessage()
				return
			}
			for {
				if _, _, err = connection.ReadMessage(); err != nil {
					return
				}
			}
		}
	}))
	defer protocol.Close()
	clock := &fakeResumeClock{now: time.Now()}
	prior, workload := resumableSessionFixture(t, clock)
	order := []string{}
	reconnector := NewReconnector("v1.2.3")
	reconnector.connector = connectorForEndpoint(t, protocol.URL, func(context.Context, *ProvisionedWorkload, ContextSnapshot) UnavailableReason { return "" }, &order)
	result := reconnector.Resume(context.Background(), ResumeRequest{Prior: prior, Workload: workload})
	if result.Session == nil || result.Session.Identity().Generation != 3 || sockets.Load() != 2 || !reflect.DeepEqual(clock.sleeps, []time.Duration{time.Second}) {
		t.Fatalf("result=%#v sockets=%d sleeps=%v", result, sockets.Load(), clock.sleeps)
	}
	if err := result.Session.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestCandidateRetainsApplicationFrameBeforeMatchingPong(t *testing.T) {
	applicationFrame := []byte(`{"type":"event","name":"application","data":{"value":7}}`)
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	protocol := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/accelerator-info":
			writer.Header().Set("Content-Type", "application/json")
			writer.Header().Set("Cache-Control", "no-store")
			_ = json.NewEncoder(writer).Encode(server.AuthenticatedAcceleratorInfo{Runtime: "accelerator", Build: agent.BuildIdentity{BuildVersion: "v1.2.3"}, InstanceID: "instance-a", Capabilities: agent.V1Capabilities(), CapabilityDiagnostics: []agent.CapabilityDiagnostic{}})
		case "/api/call":
			writer.Header().Set("Content-Type", "application/json")
			writer.Header().Set("Cache-Control", "no-store")
			writer.WriteHeader(http.StatusForbidden)
			_, _ = writer.Write([]byte("{\"error\":\"forbidden\"}\n"))
		case "/ws":
			connection, err := upgrader.Upgrade(writer, request, nil)
			if err != nil {
				return
			}
			defer connection.Close()
			_ = connection.WriteJSON(map[string]interface{}{"type": "event", "name": "connected", "data": map[string]interface{}{"sessionId": "session-a", "instanceId": "instance-a", "generation": 2, "resumed": true}})
			_ = connection.WriteMessage(websocket.TextMessage, applicationFrame)
			for {
				if _, _, err = connection.ReadMessage(); err != nil {
					return
				}
			}
		}
	}))
	defer protocol.Close()
	clock := &fakeResumeClock{now: time.Now()}
	prior, workload := resumableSessionFixture(t, clock)
	order := []string{}
	reconnector := NewReconnector("v1.2.3")
	reconnector.connector = connectorForEndpoint(t, protocol.URL, func(context.Context, *ProvisionedWorkload, ContextSnapshot) UnavailableReason { return "" }, &order)
	result := reconnector.Resume(context.Background(), ResumeRequest{Prior: prior, Workload: workload})
	if result.Session == nil || result.Session.Identity().Generation != 2 {
		t.Fatalf("result=%#v", result)
	}
	select {
	case frame := <-result.Session.frames:
		if !reflect.DeepEqual(frame, applicationFrame) {
			t.Fatalf("retained frame=%s want=%s", frame, applicationFrame)
		}
	case <-time.After(time.Second):
		t.Fatal("application frame was not retained by candidate pump")
	}
	if err := result.Session.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func fakeClientset(job *batchv1.Job, pod *corev1.Pod) *fake.Clientset {
	objects := []runtime.Object{job, pod}
	return fake.NewSimpleClientset(objects...)
}

func connectorProtocolServer(t *testing.T, expectedToken string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var authenticatedCalls atomic.Int32
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer "+expectedToken {
			writer.Header().Set("Content-Type", "application/json")
			writer.Header().Set("Cache-Control", "no-store")
			writer.WriteHeader(http.StatusUnauthorized)
			_, _ = writer.Write([]byte("{\"error\":\"unauthorized\"}\n"))
			return
		}
		authenticatedCalls.Add(1)
		switch request.URL.Path {
		case "/api/accelerator-info":
			writer.Header().Set("Content-Type", "application/json")
			writer.Header().Set("Cache-Control", "no-store")
			_ = json.NewEncoder(writer).Encode(server.AuthenticatedAcceleratorInfo{Runtime: "accelerator", Build: agent.BuildIdentity{BuildVersion: "v1.2.3"}, InstanceID: "instance-a", Capabilities: agent.V1Capabilities(), CapabilityDiagnostics: []agent.CapabilityDiagnostic{}})
		case "/api/call":
			writer.Header().Set("Content-Type", "application/json")
			writer.Header().Set("Cache-Control", "no-store")
			writer.WriteHeader(http.StatusForbidden)
			_, _ = writer.Write([]byte("{\"error\":\"forbidden\"}\n"))
		case "/ws":
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
		default:
			http.NotFound(writer, request)
		}
	})
	return httptest.NewServer(handler), &authenticatedCalls
}

func connectorForEndpoint(t *testing.T, rawURL string, validation connectorValidator, order *[]string) *Connector {
	t.Helper()
	_, portText, err := net.SplitHostPort(strings.TrimPrefix(rawURL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	return &Connector{buildVersion: "v1.2.3", validate: validation, startTunnel: func(context.Context, ContextSnapshot, string, string) (tunnel, error) {
		active := newRecordedTunnel(order)
		active.port = port
		return active, nil
	}}
}

func TestConnectorOutcomeMatrix(t *testing.T) {
	protocol, calls := connectorProtocolServer(t, knownCreatorToken)
	defer protocol.Close()
	order := []string{}
	var validations atomic.Int32
	connector := connectorForEndpoint(t, protocol.URL, func(context.Context, *ProvisionedWorkload, ContextSnapshot) UnavailableReason {
		validations.Add(1)
		return ""
	}, &order)
	workload := connectorWorkload(t)
	result := connector.Connect(context.Background(), workload)
	if result.Availability != Available || result.Session == nil || result.Reason != "" {
		t.Fatalf("exact connection failed: %#v", result)
	}
	if validations.Load() != 3 || calls.Load() != 3 {
		t.Fatalf("boundaries=%d authenticated calls=%d", validations.Load(), calls.Load())
	}
	identity := result.Session.Identity()
	if identity.SessionID != "session-a" || identity.InstanceID != "instance-a" || identity.Generation != 1 {
		t.Fatalf("unsafe/incomplete identity: %#v", identity)
	}
	if err := result.Session.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	invalid := []*ProvisionedWorkload{nil, {}, {BuildVersion: "v1.2.3"}}
	for _, candidate := range invalid {
		if got := connector.Connect(context.Background(), candidate); got.Reason != InvalidWorkload || got.Session != nil {
			t.Fatalf("invalid handle accepted: %#v", got)
		}
	}
	wrongVersion := connectorWorkload(t)
	wrongVersion.BuildVersion = "v1.2.4"
	if got := connector.Connect(context.Background(), wrongVersion); got.Reason != InvalidWorkload {
		t.Fatalf("version mismatch reason=%s", got.Reason)
	}
}

func TestConnectRejectsPodReplacementAtEveryBoundary(t *testing.T) {
	protocol, _ := connectorProtocolServer(t, knownCreatorToken)
	defer protocol.Close()
	for _, failAt := range []int32{1, 2, 3} {
		t.Run(strconv.Itoa(int(failAt)), func(t *testing.T) {
			order := []string{}
			var boundary atomic.Int32
			connector := connectorForEndpoint(t, protocol.URL, func(context.Context, *ProvisionedWorkload, ContextSnapshot) UnavailableReason {
				if boundary.Add(1) == failAt {
					return ContextChanged
				}
				return ""
			}, &order)
			result := connector.Connect(context.Background(), connectorWorkload(t))
			if result.Reason != WorkloadChanged || result.Session != nil {
				t.Fatalf("replacement boundary %d result=%#v", failAt, result)
			}
			if failAt > 1 && !strings.Contains(strings.Join(order, ","), "port-forward-stop") {
				t.Fatalf("opened tunnel not stopped: %v", order)
			}
		})
	}
}

func TestConnectorWrongTokenFailsClosedAndRetainsWorkload(t *testing.T) {
	protocol, calls := connectorProtocolServer(t, "different-token")
	defer protocol.Close()
	order := []string{}
	connector := connectorForEndpoint(t, protocol.URL, func(context.Context, *ProvisionedWorkload, ContextSnapshot) UnavailableReason { return "" }, &order)
	workload := connectorWorkload(t)
	result := connector.Connect(context.Background(), workload)
	if result.Reason != AcceleratorUnavailable || result.Session != nil || calls.Load() != 0 {
		t.Fatalf("wrong token result=%#v calls=%d", result, calls.Load())
	}
	if workload.credential == nil || workload.snapshot == nil || workload.connectorState.active != 0 {
		t.Fatal("failure mutated retained workload ownership")
	}
	if !strings.Contains(strings.Join(order, ","), "port-forward-stop") {
		t.Fatalf("tunnel not closed: %v", order)
	}
}

func TestConnectorOwnsEveryReturnedTunnelBeforeClassification(t *testing.T) {
	tests := []struct {
		name       string
		port       int
		startErr   error
		cancelPost bool
		want       ConnectUnavailableReason
	}{
		{name: "post-start cancellation", port: 43123, cancelPost: true, want: ConnectCancelled},
		{name: "live tunnel with start error", port: 43123, startErr: errors.New("raw start failure"), want: TunnelUnavailable},
		{name: "live tunnel with invalid port", port: 0, want: TunnelUnavailable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			order := []string{}
			active := newRecordedTunnel(&order)
			active.port = test.port
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			connector := &Connector{
				buildVersion: "v1.2.3",
				validate:     func(context.Context, *ProvisionedWorkload, ContextSnapshot) UnavailableReason { return "" },
				startTunnel: func(startCtx context.Context, _ ContextSnapshot, _, _ string) (tunnel, error) {
					if test.cancelPost {
						cancel()
						<-startCtx.Done()
					}
					return active, test.startErr
				},
			}
			result := connector.Connect(ctx, connectorWorkload(t))
			if result.Reason != test.want || result.Session != nil {
				t.Fatalf("result=%#v want reason=%s", result, test.want)
			}
			if joined := strings.Join(order, ","); !strings.Contains(joined, "port-forward-stop") || !strings.Contains(joined, "port-forward-wait") {
				t.Fatalf("returned tunnel was not synchronously owned and closed: %v", order)
			}
		})
	}
}

func TestConnectorCancellationWinsPublicationWindow(t *testing.T) {
	protocol, _ := connectorProtocolServer(t, knownCreatorToken)
	defer protocol.Close()
	order := []string{}
	ctx, cancel := context.WithCancel(context.Background())
	connector := connectorForEndpoint(t, protocol.URL, func(context.Context, *ProvisionedWorkload, ContextSnapshot) UnavailableReason { return "" }, &order)
	connector.beforePublish = func() {
		cancel()
		<-ctx.Done()
	}
	result := connector.Connect(ctx, connectorWorkload(t))
	if result.Reason != ConnectCancelled || result.Session != nil {
		t.Fatalf("publication-window cancellation returned stale success: %#v", result)
	}
	joined := strings.Join(order, ",")
	if !strings.Contains(joined, "port-forward-stop") || !strings.Contains(joined, "port-forward-wait") {
		t.Fatalf("publication-window cancellation did not synchronously close transports: %v", order)
	}
}

func TestConnectorRejectsMutatedCopiedAndReconstructedHandles(t *testing.T) {
	mutations := map[string]func(*ProvisionedWorkload){
		"context":          func(w *ProvisionedWorkload) { w.ContextName = "other" },
		"namespace":        func(w *ProvisionedWorkload) { w.ReleaseNamespace = "other" },
		"release":          func(w *ProvisionedWorkload) { w.ReleaseName = "other" },
		"workload session": func(w *ProvisionedWorkload) { w.WorkloadSessionID = "other" },
		"job":              func(w *ProvisionedWorkload) { w.Job.Name = "other" },
		"pod":              func(w *ProvisionedWorkload) { w.Pod.Name = "other" },
		"build":            func(w *ProvisionedWorkload) { w.BuildVersion = "v1.2.4" },
		"image":            func(w *ProvisionedWorkload) { w.ImageDigest = "sha256:" + strings.Repeat("c", 64) },
		"chart":            func(w *ProvisionedWorkload) { w.ChartDigest = "sha256:" + strings.Repeat("d", 64) },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			workload := connectorWorkload(t)
			mutate(workload)
			var starts atomic.Int32
			connector := &Connector{buildVersion: "v1.2.3", validate: func(context.Context, *ProvisionedWorkload, ContextSnapshot) UnavailableReason { return "" }, startTunnel: func(context.Context, ContextSnapshot, string, string) (tunnel, error) {
				starts.Add(1)
				return nil, errors.New("must not start")
			}}
			if result := connector.Connect(context.Background(), workload); result.Reason != InvalidWorkload || result.Session != nil || starts.Load() != 0 {
				t.Fatalf("mutated handle accepted: %#v starts=%d", result, starts.Load())
			}
		})
	}

	original := connectorWorkload(t)
	copied := *original
	connector := &Connector{buildVersion: "v1.2.3", validate: func(context.Context, *ProvisionedWorkload, ContextSnapshot) UnavailableReason { return "" }, startTunnel: func(context.Context, ContextSnapshot, string, string) (tunnel, error) {
		t.Fatal("copied/reconstructed handle reached tunnel")
		return nil, nil
	}}
	if result := connector.Connect(context.Background(), &copied); result.Reason != InvalidWorkload || result.Session != nil {
		t.Fatalf("copied handle accepted: %#v", result)
	}
	encoded, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var reconstructed ProvisionedWorkload
	if err = json.Unmarshal(encoded, &reconstructed); err != nil {
		t.Fatal(err)
	}
	if result := connector.Connect(context.Background(), &reconstructed); result.Reason != InvalidWorkload || result.Session != nil {
		t.Fatalf("reconstructed handle accepted: %#v", result)
	}
}

func TestConnectorFreshConnectIsConcurrentAndSubsequentOneShot(t *testing.T) {
	protocol, calls := connectorProtocolServer(t, knownCreatorToken)
	defer protocol.Close()
	order := []string{}
	connector := connectorForEndpoint(t, protocol.URL, func(context.Context, *ProvisionedWorkload, ContextSnapshot) UnavailableReason { return "" }, &order)
	workload := connectorWorkload(t)
	const callers = 16
	start := make(chan struct{})
	results := make(chan ConnectResult, callers)
	var ready sync.WaitGroup
	ready.Add(callers)
	for index := 0; index < callers; index++ {
		go func() {
			ready.Done()
			<-start
			results <- connector.Connect(context.Background(), workload)
		}()
	}
	ready.Wait()
	close(start)
	available, invalid := 0, 0
	var session *ConnectedSession
	for index := 0; index < callers; index++ {
		result := <-results
		switch result.Reason {
		case "":
			if result.Availability != Available || result.Session == nil {
				t.Fatalf("malformed success: %#v", result)
			}
			available++
			session = result.Session
		case InvalidWorkload:
			invalid++
		default:
			t.Fatalf("unexpected concurrent result: %#v", result)
		}
	}
	if available != 1 || invalid != callers-1 || calls.Load() != 3 {
		t.Fatalf("available=%d invalid=%d authenticated=%d", available, invalid, calls.Load())
	}
	if subsequent := connector.Connect(context.Background(), workload); subsequent.Reason != InvalidWorkload || subsequent.Session != nil || calls.Load() != 3 {
		t.Fatalf("subsequent fresh Connect was not rejected: %#v calls=%d", subsequent, calls.Load())
	}
	if err := session.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestConnectorSealsCallerContextValuesAndHTTPTrace(t *testing.T) {
	protocol, _ := connectorProtocolServer(t, knownCreatorToken)
	defer protocol.Close()
	type contextKey struct{}
	var traceCalls atomic.Int32
	trace := &httptrace.ClientTrace{
		GetConn: func(string) { traceCalls.Add(1) },
		GotConn: func(httptrace.GotConnInfo) { traceCalls.Add(1) },
	}
	ctx := context.WithValue(context.Background(), contextKey{}, "private-caller-value")
	ctx = httptrace.WithClientTrace(ctx, trace)
	order := []string{}
	connector := connectorForEndpoint(t, protocol.URL, func(validateCtx context.Context, _ *ProvisionedWorkload, _ ContextSnapshot) UnavailableReason {
		if validateCtx.Value(contextKey{}) != nil || httptrace.ContextClientTrace(validateCtx) != nil {
			t.Error("caller context values reached connector I/O")
		}
		return ""
	}, &order)
	result := connector.Connect(ctx, connectorWorkload(t))
	if result.Availability != Available || result.Session == nil || traceCalls.Load() != 0 {
		t.Fatalf("sealed connect result=%#v trace calls=%d", result, traceCalls.Load())
	}
	if err := result.Session.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestConnectorSecretAndEndpointRedactionCorpus(t *testing.T) {
	result := unavailableConnect(AcceleratorUnavailable)
	endpoint := "127.0.0.1:49151"
	for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%q", "%x"} {
		output := fmt.Sprintf(verb, result)
		assertNoCredentialCorpus(t, output, []string{knownCreatorToken, endpoint, "Authorization", "Bearer"})
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	assertNoCredentialCorpus(t, string(encoded), []string{knownCreatorToken, endpoint, "Authorization", "Bearer"})
}
