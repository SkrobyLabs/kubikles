package acceleratorprovision

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/httpstream"
	"kubikles/pkg/agent"
	"kubikles/pkg/server"
)

func TestResumeDeterministicScheduleAndSingleOwner(t *testing.T) {
	clock := &fakeResumeClock{now: time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)}
	session, workload := resumableSessionFixture(t, clock)
	reconnector := NewReconnector("v1.2.3")
	clock.onSleep = func(time.Duration) {
		if workload.connectorState.active != 0 {
			t.Fatal("resume lease retained during backoff")
		}
	}
	var attempts atomic.Int32
	reconnector.attempt = func(_ context.Context, _ connectorLease, _ connectionExpectation) (*ConnectedSession, *connectAttemptFailure) {
		if attempts.Add(1) == 6 {
			return nil, &connectAttemptFailure{phase: attemptConnectedFrame, cause: protocolAttemptError{}}
		}
		return nil, &connectAttemptFailure{phase: attemptInfo, cause: syscall.ECONNREFUSED}
	}
	result := reconnector.Resume(context.Background(), ResumeRequest{Prior: session, Workload: workload})
	if result.Reason != ResumeProtocolFailed || result.Session != nil || attempts.Load() != 6 {
		t.Fatalf("result=%#v attempts=%d", result, attempts.Load())
	}
	wantSleeps := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 5 * time.Second, 5 * time.Second}
	if !reflect.DeepEqual(clock.sleeps, wantSleeps) {
		t.Fatalf("sleeps=%v want=%v", clock.sleeps, wantSleeps)
	}
	if workload.connectorState.active != 0 {
		t.Fatal("credential/workload lease retained across terminal result")
	}
	if second := reconnector.Resume(context.Background(), ResumeRequest{Prior: session, Workload: workload}); second.Reason != ResumeSuperseded {
		t.Fatalf("second owner=%#v", second)
	}
}

func TestResumeGraceDeadlineAndCancellation(t *testing.T) {
	clock := &fakeResumeClock{now: time.Date(2026, 8, 2, 18, 0, 0, 0, time.UTC)}
	session, workload := resumableSessionFixture(t, clock)
	clock.advance(agent.AcceleratorIdleReconnectGrace)
	reconnector := NewReconnector("v1.2.3")
	reconnector.attempt = func(context.Context, connectorLease, connectionExpectation) (*ConnectedSession, *connectAttemptFailure) {
		t.Fatal("attempt started at deadline")
		return nil, nil
	}
	if result := reconnector.Resume(context.Background(), ResumeRequest{Prior: session, Workload: workload}); result.Reason != ResumeGraceExpired {
		t.Fatalf("deadline result=%#v", result)
	}

	clock2 := &fakeResumeClock{now: time.Now()}
	session2, workload2 := resumableSessionFixture(t, clock2)
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if result := NewReconnector("v1.2.3").Resume(cancelled, ResumeRequest{Prior: session2, Workload: workload2}); result.Reason != ResumeCancelled {
		t.Fatalf("cancellation result=%#v", result)
	}
}

func TestResumeMonotonicDeadlineBudgetsAndEquality(t *testing.T) {
	if !strings.Contains(time.Now().String(), "m=+") {
		t.Fatal("test clock source does not carry a monotonic reading")
	}
	tests := []struct {
		name         string
		delay        time.Duration
		callerBudget time.Duration
		wantAttempt  bool
		wantBudget   time.Duration
		wantReason   ResumeReason
	}{
		{name: "immediate", wantAttempt: true, wantBudget: connectorTimeout, wantReason: ResumeProtocolFailed},
		{name: "119 seconds", delay: 119 * time.Second, wantAttempt: true, wantBudget: time.Second, wantReason: ResumeProtocolFailed},
		{name: "120 seconds equality", delay: 120 * time.Second, wantReason: ResumeGraceExpired},
		{name: "earlier caller deadline", callerBudget: 250 * time.Millisecond, wantAttempt: true, wantBudget: 250 * time.Millisecond, wantReason: ResumeProtocolFailed},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			clock := &fakeResumeClock{now: time.Now().Add(-time.Second)}
			prior, workload := resumableSessionFixture(t, clock)
			clock.advance(test.delay)
			reconnector := NewReconnector("v1.2.3")
			attempts := 0
			reconnector.attempt = func(ctx context.Context, _ connectorLease, _ connectionExpectation) (*ConnectedSession, *connectAttemptFailure) {
				attempts++
				deadline, ok := ctx.Deadline()
				if !ok {
					t.Fatal("attempt lacked bounded deadline")
				}
				got := time.Until(deadline)
				if delta := got - test.wantBudget; delta < -100*time.Millisecond || delta > 100*time.Millisecond {
					t.Fatalf("attempt budget=%v want=%v", got, test.wantBudget)
				}
				return nil, &connectAttemptFailure{phase: attemptConnectedFrame, cause: protocolAttemptError{}}
			}
			ctx := context.Background()
			if test.callerBudget != 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, test.callerBudget)
				defer cancel()
			}
			result := reconnector.Resume(ctx, ResumeRequest{Prior: prior, Workload: workload})
			if result.Reason != test.wantReason || (attempts == 1) != test.wantAttempt {
				t.Fatalf("result=%#v attempts=%d", result, attempts)
			}
		})
	}

	t.Run("clamped sleep stops at equality", func(t *testing.T) {
		clock := &fakeResumeClock{now: time.Now().Add(-time.Second)}
		prior, workload := resumableSessionFixture(t, clock)
		clock.advance(117 * time.Second)
		reconnector := NewReconnector("v1.2.3")
		attempts := 0
		reconnector.attempt = func(context.Context, connectorLease, connectionExpectation) (*ConnectedSession, *connectAttemptFailure) {
			attempts++
			return nil, &connectAttemptFailure{phase: attemptInfo, cause: syscall.ECONNREFUSED}
		}
		result := reconnector.Resume(context.Background(), ResumeRequest{Prior: prior, Workload: workload})
		if result.Reason != ResumeGraceExpired || attempts != 2 || !reflect.DeepEqual(clock.sleeps, []time.Duration{time.Second, 2 * time.Second}) {
			t.Fatalf("result=%#v attempts=%d sleeps=%v", result, attempts, clock.sleeps)
		}
	})

	t.Run("accepted generation never resets deadline", func(t *testing.T) {
		clock := &fakeResumeClock{now: time.Now().Add(-time.Second)}
		prior, workload := resumableSessionFixture(t, clock)
		clock.advance(119 * time.Second)
		reconnector := NewReconnector("v1.2.3")
		attempts := 0
		reconnector.attempt = func(_ context.Context, _ connectorLease, expectation connectionExpectation) (*ConnectedSession, *connectAttemptFailure) {
			attempts++
			expectation.accepted(2)
			return nil, &connectAttemptFailure{phase: attemptPublication, earlyClose: true}
		}
		result := reconnector.Resume(context.Background(), ResumeRequest{Prior: prior, Workload: workload})
		if result.Reason != ResumeGraceExpired || attempts != 1 || !reflect.DeepEqual(clock.sleeps, []time.Duration{time.Second}) {
			t.Fatalf("result=%#v attempts=%d sleeps=%v", result, attempts, clock.sleeps)
		}
	})
}

func TestResumeRetryClassificationClosedMatrix(t *testing.T) {
	for name, failure := range map[string]*connectAttemptFailure{
		"refused":           {phase: attemptInfo, cause: syscall.ECONNREFUSED},
		"reset":             {phase: attemptInfo, cause: syscall.ECONNRESET},
		"eof":               {phase: attemptConnectedFrame, cause: io.EOF},
		"unexpected eof":    {phase: attemptConnectedFrame, cause: io.ErrUnexpectedEOF},
		"kube throttled":    {phase: attemptTunnel, cause: apierrors.NewTooManyRequests("redacted", 1)},
		"kube timeout":      {phase: attemptTunnel, cause: apierrors.NewTimeoutError("redacted", 1)},
		"kube internal":     {phase: attemptTunnel, cause: apierrors.NewInternalError(errors.New("redacted"))},
		"kube unavailable":  {phase: attemptTunnel, cause: apierrors.NewServiceUnavailable("redacted")},
		"kube gateway":      {phase: attemptTunnel, cause: &apierrors.StatusError{ErrStatus: metav1.Status{Code: 504}}},
		"upgrade":           {phase: attemptTunnel, cause: &httpstream.UpgradeFailureError{Cause: errors.New("redacted")}},
		"https proxy":       {phase: attemptTunnel, cause: errors.New("proxy: unknown scheme: https")},
		"http 503":          {phase: attemptWebSocket, status: 503},
		"early final close": {phase: attemptPublication, earlyClose: true},
	} {
		if got := retryableResumeFailure(failure); !got {
			t.Fatalf("%s retry=%t want=true", name, got)
		}
	}
	for _, failure := range []*connectAttemptFailure{
		{phase: attemptInfo, status: 401},
		{phase: attemptInfo, status: 500},
		{phase: attemptTunnel, cause: &apierrors.StatusError{ErrStatus: metav1.Status{Code: 404}}},
		{phase: attemptConnectedFrame, cause: protocolAttemptError{}},
		{phase: attemptInfo, cause: errors.New("connection refused EOF 503")},
	} {
		if retryableResumeFailure(failure) {
			t.Fatalf("terminal failure retried: %#v", failure)
		}
	}
}

func TestResumeFixedReasonsAndSecretRedaction(t *testing.T) {
	clock := &fakeResumeClock{now: time.Date(2026, 8, 2, 14, 15, 16, 17, time.UTC)}
	prior, workload := resumableSessionFixture(t, clock)
	request := ResumeRequest{Prior: prior, Workload: workload}
	unsafe := []string{knownCreatorToken, workload.credential.verifier, "Authorization", "127.0.0.1:43123", "https://raw-endpoint.invalid/private", clock.Now().Format(time.RFC3339Nano), "raw connection error body", "raw response body", "raw frame body"}
	var capturedLogs bytes.Buffer
	oldLogOutput := log.Writer()
	log.SetOutput(&capturedLogs)
	defer log.SetOutput(oldLogOutput)
	reconnector := NewReconnector("v1.2.3")
	reconnector.attempt = func(context.Context, connectorLease, connectionExpectation) (*ConnectedSession, *connectAttemptFailure) {
		return nil, &connectAttemptFailure{phase: attemptInfo, cause: errors.New(strings.Join(unsafe, " "))}
	}
	failed := reconnector.Resume(context.Background(), request)
	if failed.Reason != ResumeTransportUnavailable || failed.Session != nil {
		t.Fatalf("failed result=%#v", failed)
	}
	successWorkload := connectorWorkload(t)
	order := []string{}
	successSession := newConnectedSession(successWorkload.connectorState.receipt, server.AuthenticatedAcceleratorInfo{Capabilities: agent.V1Capabilities()}, connectedIdentity{sessionID: "session-success", instanceID: "instance-success", generation: 2}, newRecordedSessionSocket(&order), newRecordedTunnel(&order), clock)
	succeeded := ResumeResult{Availability: Available, Session: successSession}
	for _, value := range []interface{}{request, failed, succeeded, ResumeTransportUnavailable, prior, prior.Identity(), successSession, successSession.Identity()} {
		for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%q"} {
			formatted := fmt.Sprintf(verb, value)
			for _, secret := range unsafe {
				if secret != "" && strings.Contains(formatted, secret) {
					t.Fatalf("formatter exposed secret %q", secret)
				}
			}
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		for _, secret := range unsafe {
			if secret != "" && strings.Contains(string(encoded), secret) {
				t.Fatalf("JSON exposed secret %q", secret)
			}
		}
	}
	for _, secret := range unsafe {
		if secret != "" && strings.Contains(capturedLogs.String(), secret) {
			t.Fatalf("log exposed secret %q", secret)
		}
	}
	if err := successSession.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestResumeGenerationFenceSurvivesPartialAcceptance(t *testing.T) {
	clock := &fakeResumeClock{now: time.Now()}
	prior, workload := resumableSessionFixture(t, clock)
	reconnector := NewReconnector("v1.2.3")
	var attempts int
	reconnector.attempt = func(_ context.Context, _ connectorLease, expectation connectionExpectation) (*ConnectedSession, *connectAttemptFailure) {
		attempts++
		if attempts == 1 {
			if expectation.generationFloor != 1 {
				t.Fatalf("initial floor=%d", expectation.generationFloor)
			}
			expectation.accepted(2)
			return nil, &connectAttemptFailure{phase: attemptPublication, earlyClose: true}
		}
		if expectation.generationFloor != 2 {
			t.Fatalf("advanced floor=%d", expectation.generationFloor)
		}
		expectation.accepted(4)
		order := []string{}
		return newConnectedSession(workload.connectorState.receipt, server.AuthenticatedAcceleratorInfo{Capabilities: agent.V1Capabilities()}, connectedIdentity{sessionID: "session-a", instanceID: "instance-a", generation: 4}, newRecordedSessionSocket(&order), newRecordedTunnel(&order), clock), nil
	}
	result := reconnector.Resume(context.Background(), ResumeRequest{Prior: prior, Workload: workload})
	if result.Session == nil || result.Session.Identity().Generation != 4 || attempts != 2 {
		t.Fatalf("result=%#v attempts=%d", result, attempts)
	}
	if err := result.Session.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestExplicitCloseRevokesResumeInProgress(t *testing.T) {
	clock := &fakeResumeClock{now: time.Now()}
	prior, workload := resumableSessionFixture(t, clock)
	reconnector := NewReconnector("v1.2.3")
	entered := make(chan struct{})
	reconnector.attempt = func(ctx context.Context, _ connectorLease, _ connectionExpectation) (*ConnectedSession, *connectAttemptFailure) {
		close(entered)
		<-ctx.Done()
		return nil, &connectAttemptFailure{phase: attemptInfo, cause: ctx.Err()}
	}
	result := make(chan ResumeResult, 1)
	go func() {
		result <- reconnector.Resume(context.Background(), ResumeRequest{Prior: prior, Workload: workload})
	}()
	<-entered
	if err := prior.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := <-result; got.Reason != ResumeExplicitlyClosed || got.Session != nil {
		t.Fatalf("result=%#v", got)
	}
	if workload.connectorState.active != 0 {
		t.Fatal("resume lease remained active after explicit close")
	}
}

func TestDisposalNormalizesBlockedResumeExit(t *testing.T) {
	clock := &fakeResumeClock{now: time.Date(2026, 8, 2, 18, 0, 0, 0, time.UTC)}
	prior, workload := resumableSessionFixture(t, clock)
	reconnector := NewReconnector("v1.2.3")
	entered := make(chan struct{})
	reconnector.attempt = func(ctx context.Context, _ connectorLease, _ connectionExpectation) (*ConnectedSession, *connectAttemptFailure) {
		close(entered)
		<-ctx.Done()
		return nil, &connectAttemptFailure{phase: attemptInfo, cause: ctx.Err()}
	}
	resumeResult := make(chan ResumeResult, 1)
	go func() {
		resumeResult <- reconnector.Resume(context.Background(), ResumeRequest{Prior: prior, Workload: workload})
	}()
	<-entered
	service := NewDisposalService(nil)
	service.cleanupOwned = func(context.Context, *workloadReceipt) (OwnershipStatus, UninstallStatus, DisappearanceStatus) {
		return OwnershipAlreadyGone, UninstallNotNeeded, DisappearanceSucceeded
	}
	disposed := make(chan DisposalResult, 1)
	go func() { disposed <- service.DisposeNow(context.Background(), workload) }()
	if got := <-resumeResult; got.Reason != ResumeWorkloadDisposing || got.Session != nil {
		t.Fatalf("resume=%#v", got)
	}
	if got := <-disposed; got.Ownership != OwnershipAlreadyGone || got.Uninstall != UninstallNotNeeded {
		t.Fatalf("dispose=%#v", got)
	}
}

func waitForWorkloadDisposing(workload *ProvisionedWorkload) {
	for {
		state := workload.connectorState
		state.mu.Lock()
		if state.disposing {
			state.mu.Unlock()
			return
		}
		if state.changed == nil {
			state.changed = make(chan struct{})
		}
		changed := state.changed
		state.mu.Unlock()
		<-changed
	}
}

func TestResumeSuccessfulCandidatePublicationLinearizesWithDisposalFence(t *testing.T) {
	for _, fenceFirst := range []bool{true, false} {
		name := "publication first"
		if fenceFirst {
			name = "fence first"
		}
		t.Run(name, func(t *testing.T) {
			clock := &fakeResumeClock{now: time.Date(2026, 8, 2, 18, 0, 0, 0, time.UTC)}
			prior, workload := resumableSessionFixture(t, clock)
			service := NewDisposalService(nil)
			service.cleanupOwned = func(context.Context, *workloadReceipt) (OwnershipStatus, UninstallStatus, DisappearanceStatus) {
				return OwnershipAlreadyGone, UninstallNotNeeded, DisappearanceSucceeded
			}
			disposed := make(chan DisposalResult, 1)
			startDisposal := func() {
				go func() { disposed <- service.DisposeNow(context.Background(), workload) }()
				waitForWorkloadDisposing(workload)
			}
			order := []string{}
			candidate := newConnectedSession(workload.connectorState.receipt, server.AuthenticatedAcceleratorInfo{Capabilities: agent.V1Capabilities()}, connectedIdentity{sessionID: "session-a", instanceID: "instance-a", generation: 2}, newRecordedSessionSocket(&order), newRecordedTunnel(&order), clock)
			reconnector := NewReconnector("v1.2.3")
			reconnector.attempt = func(context.Context, connectorLease, connectionExpectation) (*ConnectedSession, *connectAttemptFailure) {
				if fenceFirst {
					startDisposal()
				}
				return candidate, nil
			}
			published := false
			if !fenceFirst {
				reconnector.afterPublish = func(session *ConnectedSession) {
					workload.connectorState.mu.Lock()
					published = workload.connectorState.currentSession == session
					workload.connectorState.mu.Unlock()
					startDisposal()
				}
			}
			result := reconnector.Resume(context.Background(), ResumeRequest{Prior: prior, Workload: workload})
			if result.Reason != ResumeWorkloadDisposing || result.Session != nil {
				t.Fatalf("resume=%#v", result)
			}
			if !fenceFirst && !published {
				t.Fatal("successful Resume candidate never crossed the real publication boundary")
			}
			if got := <-disposed; got.Effective != DisposalImmediate || got.Ownership != OwnershipAlreadyGone {
				t.Fatalf("effective=%s ownership=%s uninstall=%s disappearance=%s", got.Effective, got.Ownership, got.Uninstall, got.Disappearance)
			}
			<-candidate.Done()
		})
	}
}

func TestResumeStaleAttemptCannotAdvanceFence(t *testing.T) {
	clock := &fakeResumeClock{now: time.Now()}
	prior, workload := resumableSessionFixture(t, clock)
	reconnector := NewReconnector("v1.2.3")
	var stale func(int)
	attempts := 0
	reconnector.attempt = func(_ context.Context, _ connectorLease, expectation connectionExpectation) (*ConnectedSession, *connectAttemptFailure) {
		attempts++
		if attempts == 1 {
			stale = expectation.accepted
			return nil, &connectAttemptFailure{phase: attemptPublication, earlyClose: true}
		}
		stale(99)
		if expectation.generationFloor != 1 {
			t.Fatalf("stale attempt advanced generation fence to %d", expectation.generationFloor)
		}
		return nil, &connectAttemptFailure{phase: attemptConnectedFrame, cause: protocolAttemptError{}}
	}
	if result := reconnector.Resume(context.Background(), ResumeRequest{Prior: prior, Workload: workload}); result.Reason != ResumeProtocolFailed || attempts != 2 {
		t.Fatalf("result=%#v attempts=%d", result, attempts)
	}
}

func TestResumeBlockedStaleWebSocketCallbackSelfClosesAfterEpochAdvances(t *testing.T) {
	clock := &fakeResumeClock{now: time.Now()}
	prior, workload := resumableSessionFixture(t, clock)
	reconnector := NewReconnector("v1.2.3")
	releaseOldCallback := make(chan struct{})
	oldCallbackClosed := make(chan struct{})
	oldOrder := []string{}
	attempts := 0
	reconnector.attempt = func(_ context.Context, _ connectorLease, expectation connectionExpectation) (*ConnectedSession, *connectAttemptFailure) {
		attempts++
		if attempts == 1 {
			go func(stale connectionExpectation) {
				<-releaseOldCallback
				socket := newRecordedSessionSocket(&oldOrder)
				active := newRecordedTunnel(&oldOrder)
				candidate := newConnectedSession(workload.connectorState.receipt, server.AuthenticatedAcceleratorInfo{Capabilities: agent.V1Capabilities()}, connectedIdentity{sessionID: "session-a", instanceID: "instance-a", generation: 99}, socket, active, clock)
				stale.accepted(99)
				_ = candidate.Close(context.Background())
				close(oldCallbackClosed)
			}(expectation)
			return nil, &connectAttemptFailure{phase: attemptPublication, earlyClose: true}
		}
		if expectation.generationFloor != 1 {
			t.Fatalf("stale callback advanced new epoch fence to %d", expectation.generationFloor)
		}
		close(releaseOldCallback)
		select {
		case <-oldCallbackClosed:
		case <-time.After(time.Second):
			t.Fatal("stale WebSocket callback did not settle")
		}
		if expectation.generationFloor != 1 {
			t.Fatalf("released stale callback advanced fence to %d", expectation.generationFloor)
		}
		return nil, &connectAttemptFailure{phase: attemptConnectedFrame, cause: protocolAttemptError{}}
	}
	result := reconnector.Resume(context.Background(), ResumeRequest{Prior: prior, Workload: workload})
	if result.Reason != ResumeProtocolFailed || result.Session != nil || attempts != 2 {
		t.Fatalf("result=%#v attempts=%d", result, attempts)
	}
	want := []string{"websocket-close-control", "websocket-close", "port-forward-stop", "port-forward-wait"}
	position := -1
	for _, event := range want {
		next := eventIndex(oldOrder, event)
		if next < 0 || next <= position {
			t.Fatalf("stale candidate did not self-close in ownership order: %v", oldOrder)
		}
		position = next
	}
}

func resumeProtocolServer(t *testing.T) *httptest.Server {
	t.Helper()
	var generations atomic.Int32
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer "+knownCreatorToken {
			writer.Header().Set("Content-Type", "application/json")
			writer.Header().Set("Cache-Control", "no-store")
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
			generation := generations.Add(1)
			_ = connection.WriteJSON(map[string]interface{}{"type": "event", "name": "connected", "data": map[string]interface{}{"sessionId": "session-a", "instanceId": "instance-a", "generation": generation, "resumed": generation > 1}})
			for {
				if _, _, err = connection.ReadMessage(); err != nil {
					return
				}
			}
		default:
			http.NotFound(writer, request)
		}
	}))
}

func TestConnectExactAttemptFreshAndResumeExpectations(t *testing.T) {
	protocol := resumeProtocolServer(t)
	defer protocol.Close()
	order := []string{}
	connector := connectorForEndpoint(t, protocol.URL, func(context.Context, *ProvisionedWorkload, ContextSnapshot) UnavailableReason { return "" }, &order)
	workload := connectorWorkload(t)
	fresh := connector.Connect(context.Background(), workload)
	if fresh.Session == nil || fresh.Session.Identity().Generation != 1 {
		t.Fatalf("fresh=%#v", fresh)
	}
	_ = fresh.Session.socket.Close()
	select {
	case <-fresh.Session.Done():
	case <-time.After(time.Second):
		t.Fatal("fresh session did not observe forced peer close")
	}
	reconnector := NewReconnector("v1.2.3")
	reconnector.connector = connector
	resumed := reconnector.Resume(context.Background(), ResumeRequest{Prior: fresh.Session, Workload: workload})
	if resumed.Availability != Available || resumed.Session == nil || resumed.Reason != "" {
		t.Fatalf("resume=%#v", resumed)
	}
	identity := resumed.Session.Identity()
	if identity.SessionID != fresh.Session.Identity().SessionID || identity.InstanceID != fresh.Session.Identity().InstanceID || identity.Generation <= fresh.Session.Identity().Generation {
		t.Fatalf("resumed identity=%#v prior=%#v", identity, fresh.Session.Identity())
	}
	if err := resumed.Session.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestResumeSingleOwnerPerGeneration(t *testing.T) {
	clock := &fakeResumeClock{now: time.Now()}
	session, workload := resumableSessionFixture(t, clock)
	reconnector := NewReconnector("v1.2.3")
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	reconnector.attempt = func(context.Context, connectorLease, connectionExpectation) (*ConnectedSession, *connectAttemptFailure) {
		once.Do(func() { close(entered) })
		<-release
		return nil, &connectAttemptFailure{phase: attemptConnectedFrame, cause: protocolAttemptError{}}
	}
	owner := make(chan ResumeResult, 1)
	go func() {
		owner <- reconnector.Resume(context.Background(), ResumeRequest{Prior: session, Workload: workload})
	}()
	<-entered
	for index := 0; index < 16; index++ {
		if result := reconnector.Resume(context.Background(), ResumeRequest{Prior: session, Workload: workload}); result.Reason != ResumeSuperseded {
			t.Fatalf("competitor %d=%#v", index, result)
		}
	}
	close(release)
	if result := <-owner; result.Reason != ResumeProtocolFailed {
		t.Fatalf("owner=%#v", result)
	}
}

func TestResumeVersionMismatchIsTerminalAndRedacted(t *testing.T) {
	clock := &fakeResumeClock{now: time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)}
	session, workload := resumableSessionFixture(t, clock)
	reconnector := NewReconnector("build-N")
	workload.BuildVersion = "build-N"
	workload.connectorState.receipt.buildVersion = "build-N"
	var attempts atomic.Int32
	reconnector.attempt = func(context.Context, connectorLease, connectionExpectation) (*ConnectedSession, *connectAttemptFailure) {
		attempts.Add(1)
		return nil, &connectAttemptFailure{phase: attemptInfo, connectReason: ConnectVersionMismatch, cause: buildVersionMismatchAttemptError{}}
	}
	result := reconnector.Resume(context.Background(), ResumeRequest{Prior: session, Workload: workload})
	if result.Reason != ResumeVersionMismatch || result.Session != nil || attempts.Load() != 1 || len(clock.sleeps) != 0 {
		t.Fatalf("result=%#v attempts=%d sleeps=%v", result, attempts.Load(), clock.sleeps)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	for _, rendered := range []string{fmt.Sprint(result), string(encoded)} {
		if strings.Contains(rendered, "build-N") {
			t.Fatalf("version leaked: %q", rendered)
		}
	}
}
