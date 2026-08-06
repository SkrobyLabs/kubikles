package acceleratorprovision

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"kubikles/pkg/agent"
	"kubikles/pkg/server"
)

type fakeResumeClock struct {
	mu      sync.Mutex
	now     time.Time
	sleeps  []time.Duration
	onSleep func(time.Duration)
}

func (c *fakeResumeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}
func (c *fakeResumeClock) Sleep(ctx context.Context, duration time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	c.sleeps = append(c.sleeps, duration)
	c.now = c.now.Add(duration)
	onSleep := c.onSleep
	c.mu.Unlock()
	if onSleep != nil {
		onSleep(duration)
	}
	return nil
}
func (c *fakeResumeClock) advance(duration time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(duration)
	c.mu.Unlock()
}

func resumableSessionFixture(t *testing.T, clock resumeClock) (*ConnectedSession, *ProvisionedWorkload) {
	t.Helper()
	workload := connectorWorkload(t)
	order := []string{}
	socket := newRecordedSessionSocket(&order)
	tunnel := newRecordedTunnel(&order)
	session := newConnectedSession(workload.connectorState.receipt, server.AuthenticatedAcceleratorInfo{Capabilities: agent.V1Capabilities()}, connectedIdentity{sessionID: "session-a", instanceID: "instance-a", generation: 1}, socket, tunnel, clock)
	_ = socket.Close()
	select {
	case <-session.Done():
	case <-time.After(time.Second):
		t.Fatal("session did not end")
	}
	return session, workload
}

func TestDisconnectRecordIsFirstSocketNonCurrentTransition(t *testing.T) {
	base := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	clock := &fakeResumeClock{now: base}
	session, _ := resumableSessionFixture(t, clock)
	session.mu.RLock()
	record := *session.disconnect
	session.mu.RUnlock()
	if record.at != base || record.reason != SessionPeerClosed || record.at.Add(agent.AcceleratorIdleReconnectGrace) != base.Add(2*time.Minute) || agent.AcceleratorIdleReconnectGrace != 2*time.Minute {
		t.Fatalf("disconnect authority mismatch: %#v", record)
	}
	clock.advance(time.Hour)
	session.beginTermination(SessionTunnelClosed, false)
	session.mu.RLock()
	after := *session.disconnect
	session.mu.RUnlock()
	if after.at != base || after.reason != SessionPeerClosed {
		t.Fatal("later terminal signal overwrote disconnect record")
	}
	encoded, err := json.Marshal(session)
	if err != nil {
		t.Fatal(err)
	}
	for _, unsafe := range []string{base.Format(time.RFC3339), base.Add(2 * time.Minute).Format(time.RFC3339)} {
		if string(encoded) == unsafe || fmt.Sprint(session) == unsafe {
			t.Fatal("disconnect time escaped safe surface")
		}
	}
}

func TestResumeRequiresExactEndedSessionAndWorkload(t *testing.T) {
	clock := &fakeResumeClock{now: time.Now()}
	session, workload := resumableSessionFixture(t, clock)
	reconnector := NewReconnector("v1.2.3")
	var attempts int
	reconnector.attempt = func(context.Context, connectorLease, connectionExpectation) (*ConnectedSession, *connectAttemptFailure) {
		attempts++
		return nil, &connectAttemptFailure{phase: attemptInfo, cause: protocolAttemptError{}}
	}

	copiedSession := &ConnectedSession{self: session}
	copiedWorkload := *workload
	differentWorkload := connectorWorkload(t)
	mutatedSession, mutatedWorkload := resumableSessionFixture(t, clock)
	mutatedWorkload.Pod.UID = "mutated"
	disposedWorkload := connectorWorkload(t)
	disposedSession, _ := resumableSessionFixture(t, clock)
	disposedSession.receipt = disposedWorkload.connectorState.receipt
	disposedSession.disconnect.workloadNonce = disposedSession.receipt
	disposedWorkload.connectorState.closed = true
	for name, request := range map[string]ResumeRequest{
		"nil prior":       {},
		"nil workload":    {Prior: session},
		"copied session":  {Prior: copiedSession, Workload: workload},
		"copied workload": {Prior: session, Workload: &copiedWorkload},
		"different":       {Prior: session, Workload: differentWorkload},
		"mutated":         {Prior: mutatedSession, Workload: mutatedWorkload},
		"disposed":        {Prior: disposedSession, Workload: disposedWorkload},
	} {
		t.Run(name, func(t *testing.T) {
			if result := reconnector.Resume(context.Background(), request); result.Reason != ResumeInvalid || result.Session != nil {
				t.Fatalf("result=%#v", result)
			}
		})
	}
	if attempts != 0 {
		t.Fatalf("invalid inputs performed %d attempts", attempts)
	}

	activeWorkload := connectorWorkload(t)
	active := newConnectedSession(activeWorkload.connectorState.receipt, server.AuthenticatedAcceleratorInfo{Capabilities: agent.V1Capabilities()}, connectedIdentity{sessionID: "session-active", instanceID: "instance-a", generation: 1}, newRecordedSessionSocket(&[]string{}), newRecordedTunnel(&[]string{}), clock)
	if result := reconnector.Resume(context.Background(), ResumeRequest{Prior: active, Workload: activeWorkload}); result.Reason != ResumeSessionIneligible || attempts != 0 {
		t.Fatalf("active result=%#v attempts=%d", result, attempts)
	}
	if err := active.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	closedClock := &fakeResumeClock{now: time.Now()}
	closedWorkload := connectorWorkload(t)
	closed := newConnectedSession(closedWorkload.connectorState.receipt, server.AuthenticatedAcceleratorInfo{Capabilities: agent.V1Capabilities()}, connectedIdentity{sessionID: "session-a", instanceID: "instance-a", generation: 1}, newRecordedSessionSocket(&[]string{}), newRecordedTunnel(&[]string{}), closedClock)
	if err := closed.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if result := NewReconnector("v1.2.3").Resume(context.Background(), ResumeRequest{Prior: closed, Workload: closedWorkload}); result.Reason != ResumeSessionIneligible {
		t.Fatalf("explicit close result=%#v", result)
	}
}
