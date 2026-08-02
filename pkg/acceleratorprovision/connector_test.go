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
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/gorilla/websocket"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
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
