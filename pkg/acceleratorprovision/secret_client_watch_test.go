package acceleratorprovision

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"kubikles/pkg/acceleratorsecret"
	"kubikles/pkg/agent"
	"kubikles/pkg/server"
)

func sendSecretEvent(t *testing.T, socket *recordedSessionSocket, name string, data interface{}) {
	t.Helper()
	payload, err := json.Marshal(struct {
		Type string      `json:"type"`
		Name string      `json:"name"`
		Data interface{} `json:"data"`
	}{Type: "event", Name: name, Data: data})
	if err != nil {
		t.Fatal(err)
	}
	socket.messages <- socketMessage{kind: websocket.TextMessage, payload: payload}
}

func subscribeSecretWatch(t *testing.T, client *secretRPCClient, socket *recordedSessionSocket, namespace string, exclude bool) (acceleratorsecret.SecretWatchSubscription, SecretWatchLease) {
	t.Helper()
	type result struct {
		subscription acceleratorsecret.SecretWatchSubscription
		lease        SecretWatchLease
		err          error
	}
	completed := make(chan result, 1)
	go func() {
		subscription, lease, err := client.SubscribeSecretWatcher(context.Background(), namespace, exclude)
		completed <- result{subscription: subscription, lease: lease, err: err}
	}()
	call := readSecretCall(t, socket)
	if call.Operation != acceleratorsecret.OperationSubscribeSecretWatcher {
		t.Fatalf("subscribe operation=%s", call.Operation)
	}
	want := acceleratorsecret.SecretWatchSubscription{WatcherSpecID: acceleratorsecret.SecretWatchSpecIDFor(namespace, exclude)}
	sendSecretResult(t, socket, call.ID, want)
	got := <-completed
	if got.err != nil || got.subscription != want || got.lease == nil {
		t.Fatalf("subscribe=%#v/%v/%v", got.subscription, got.lease, got.err)
	}
	return want, got.lease
}

func TestSecretSubscriptionExactIdentityAndEarlyEvent(t *testing.T) {
	client, socket, _, _ := secretClientFixture(t)
	type result struct {
		subscription acceleratorsecret.SecretWatchSubscription
		lease        SecretWatchLease
		err          error
	}
	completed := make(chan result, 1)
	go func() {
		subscription, lease, err := client.SubscribeSecretWatcher(context.Background(), "team", true)
		completed <- result{subscription, lease, err}
	}()
	call := readSecretCall(t, socket)
	want := acceleratorsecret.SecretWatchSubscription{WatcherSpecID: acceleratorsecret.SecretWatchSpecIDFor("team", true)}
	var item acceleratorsecret.SecretListItem
	item.Metadata.Name, item.Metadata.Namespace = "safe", "team"
	resource := acceleratorsecret.SecretResourceEvent{Type: "ADDED", ResourceType: acceleratorsecret.SecretResourceType, Namespace: "team", WatcherSpecID: want.WatcherSpecID, Resource: item}
	sendSecretEvent(t, socket, acceleratorsecret.EventResource, resource)
	sendSecretResult(t, socket, call.ID, want)
	got := <-completed
	if got.err != nil || got.subscription != want || got.lease == nil {
		t.Fatalf("early subscribe=%#v/%v/%v", got.subscription, got.lease, got.err)
	}
	select {
	case event := <-got.lease.Events():
		decoded, ok := event.Resource()
		if !ok || decoded.WatcherSpecID != want.WatcherSpecID || decoded.Resource.Metadata.Name != "safe" {
			t.Fatalf("early event=%#v/%v", decoded, ok)
		}
		if _, ok := event.Status(); ok {
			t.Fatal("resource event also contained status")
		}
	case <-time.After(time.Second):
		t.Fatal("early event was not retained")
	}

	other, otherLease := subscribeSecretWatch(t, client, socket, "other", false)
	sendSecretEvent(t, socket, acceleratorsecret.EventWatcherStatus, acceleratorsecret.SecretWatcherStatus{WatcherSpecID: other.WatcherSpecID, Status: acceleratorsecret.WatchStatusConnected})
	select {
	case event := <-otherLease.Events():
		status, ok := event.Status()
		if !ok || status.WatcherSpecID != other.WatcherSpecID {
			t.Fatalf("other status=%#v/%v", status, ok)
		}
	case <-time.After(time.Second):
		t.Fatal("other watch did not receive status")
	}
	select {
	case event := <-got.lease.Events():
		t.Fatalf("other spec crossed into first watch: %v", event)
	default:
	}

	mismatchClient, mismatchSocket, _, _ := secretClientFixture(t)
	mismatchDone := make(chan error, 1)
	go func() {
		_, _, err := mismatchClient.SubscribeSecretWatcher(context.Background(), "mismatch", true)
		mismatchDone <- err
	}()
	mismatchCall := readSecretCall(t, mismatchSocket)
	sendSecretResult(t, mismatchSocket, mismatchCall.ID, acceleratorsecret.SecretWatchSubscription{WatcherSpecID: acceleratorsecret.SecretWatchSpecIDFor("foreign", true)})
	if err := <-mismatchDone; err == nil || err.Error() != string(acceleratorsecret.ReasonProtocol) {
		t.Fatalf("mismatch error=%v", err)
	}
	if mismatchClient.Reason() != acceleratorsecret.ReasonProtocol || mismatchClient.session.EndReason() != SessionProtocolFailed {
		t.Fatalf("mismatch terminal=%s/%s", mismatchClient.Reason(), mismatchClient.session.EndReason())
	}
}

func TestSecretSubscriptionGapOverflowAndUnsubscribe(t *testing.T) {
	t.Run("cluster-wide resource namespace", func(t *testing.T) {
		client, socket, _, _ := secretClientFixture(t)
		subscription, lease := subscribeSecretWatch(t, client, socket, "", false)
		var item acceleratorsecret.SecretListItem
		item.Metadata.Name, item.Metadata.Namespace = "safe", "actual"
		sendSecretEvent(t, socket, acceleratorsecret.EventResource, acceleratorsecret.SecretResourceEvent{
			Type: "ADDED", ResourceType: acceleratorsecret.SecretResourceType, Namespace: "actual",
			WatcherSpecID: subscription.WatcherSpecID, Resource: item,
		})
		select {
		case event := <-lease.Events():
			resource, ok := event.Resource()
			if !ok || resource.Namespace != "actual" || resource.Resource.Metadata.Namespace != "actual" {
				t.Fatalf("cluster-wide resource=%#v/%v", resource, ok)
			}
		case <-time.After(time.Second):
			t.Fatal("cluster-wide resource was not delivered")
		}
		select {
		case <-lease.Done():
			t.Fatalf("valid cluster-wide resource closed watch: %s", lease.Reason())
		default:
		}
	})

	t.Run("cluster-wide envelope metadata mismatch", func(t *testing.T) {
		client, socket, _, _ := secretClientFixture(t)
		subscription, lease := subscribeSecretWatch(t, client, socket, "", false)
		var item acceleratorsecret.SecretListItem
		item.Metadata.Namespace = "metadata"
		sendSecretEvent(t, socket, acceleratorsecret.EventResource, acceleratorsecret.SecretResourceEvent{
			Type: "MODIFIED", ResourceType: acceleratorsecret.SecretResourceType, Namespace: "envelope",
			WatcherSpecID: subscription.WatcherSpecID, Resource: item,
		})
		select {
		case <-lease.Done():
		case <-time.After(time.Second):
			t.Fatal("cluster-wide namespace mismatch did not close watch")
		}
		if lease.Reason() != acceleratorsecret.ReasonWatchGap {
			t.Fatalf("cluster-wide mismatch reason=%s", lease.Reason())
		}
		_ = readSecretCall(t, socket)
	})

	t.Run("subscription capacity is fixed", func(t *testing.T) {
		client, socket, _, _ := secretClientFixture(t)
		for index := 0; index < acceleratorsecret.MaxSubscriptions; index++ {
			subscribeSecretWatch(t, client, socket, fmt.Sprintf("namespace-%d", index), false)
		}
		if _, _, err := client.SubscribeSecretWatcher(context.Background(), "one-too-many", false); err == nil || err.Error() != string(acceleratorsecret.ReasonCapacity) {
			t.Fatalf("subscription capacity error=%v", err)
		}
	})

	t.Run("overflow is explicit", func(t *testing.T) {
		client, socket, _, _ := secretClientFixture(t)
		subscription, lease := subscribeSecretWatch(t, client, socket, "overflow", false)
		status := acceleratorsecret.SecretWatcherStatus{WatcherSpecID: subscription.WatcherSpecID, Status: acceleratorsecret.WatchStatusConnected}
		for index := 0; index <= acceleratorsecret.SubscriptionEventSlots; index++ {
			sendSecretEvent(t, socket, acceleratorsecret.EventWatcherStatus, status)
		}
		select {
		case <-lease.Done():
		case <-time.After(time.Second):
			t.Fatal("overflow did not terminate watch")
		}
		if lease.Reason() != acceleratorsecret.ReasonCapacity || len(lease.Events()) != 0 {
			t.Fatalf("overflow reason/events=%s/%d", lease.Reason(), len(lease.Events()))
		}
		unsubscribe := readSecretCall(t, socket)
		if unsubscribe.Operation != acceleratorsecret.OperationUnsubscribeSecretWatcher || rawStringArg(t, unsubscribe.Args[0]) != string(subscription.WatcherSpecID) {
			t.Fatalf("overflow unsubscribe=%#v", unsubscribe)
		}
		if client.Reason() != "" {
			t.Fatalf("watch overflow terminated client: %s", client.Reason())
		}
	})

	tests := []struct {
		name      string
		eventName string
		data      func(acceleratorsecret.SecretWatchSpecID) interface{}
		wantEvent string
	}{
		{name: "reconnecting", eventName: acceleratorsecret.EventWatcherStatus, data: func(id acceleratorsecret.SecretWatchSpecID) interface{} {
			return acceleratorsecret.SecretWatcherStatus{WatcherSpecID: id, Status: acceleratorsecret.WatchStatusReconnecting}
		}, wantEvent: "status"},
		{name: "watch error", eventName: acceleratorsecret.EventWatcherError, data: func(id acceleratorsecret.SecretWatchSpecID) interface{} {
			return acceleratorsecret.SecretWatcherError{WatcherSpecID: id, Code: acceleratorsecret.WatchErrorResourceVersionExpired, Recoverable: true}
		}, wantEvent: "error"},
		{name: "namespace mismatch", eventName: acceleratorsecret.EventResource, data: func(id acceleratorsecret.SecretWatchSpecID) interface{} {
			var item acceleratorsecret.SecretListItem
			item.Metadata.Namespace = "foreign"
			return acceleratorsecret.SecretResourceEvent{Type: "MODIFIED", ResourceType: acceleratorsecret.SecretResourceType, Namespace: "foreign", WatcherSpecID: id, Resource: item}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, socket, _, _ := secretClientFixture(t)
			subscription, lease := subscribeSecretWatch(t, client, socket, "team", false)
			sendSecretEvent(t, socket, test.eventName, test.data(subscription.WatcherSpecID))
			select {
			case <-lease.Done():
			case <-time.After(time.Second):
				t.Fatal("gap did not terminate watch")
			}
			if lease.Reason() != acceleratorsecret.ReasonWatchGap {
				t.Fatalf("gap reason=%s", lease.Reason())
			}
			if test.wantEvent != "" {
				event := <-lease.Events()
				_, status := event.Status()
				_, watchError := event.Error()
				if (test.wantEvent == "status" && !status) || (test.wantEvent == "error" && !watchError) {
					t.Fatalf("gap typed event=%v status=%v error=%v", event, status, watchError)
				}
			}
			unsubscribe := readSecretCall(t, socket)
			if unsubscribe.Operation != acceleratorsecret.OperationUnsubscribeSecretWatcher {
				t.Fatalf("gap unsubscribe=%#v", unsubscribe)
			}
		})
	}

	t.Run("remove before explicit unsubscribe", func(t *testing.T) {
		client, socket, _, _ := secretClientFixture(t)
		subscription, lease := subscribeSecretWatch(t, client, socket, "team", true)
		completed := make(chan error, 1)
		go func() { completed <- client.UnsubscribeSecretWatcher(context.Background(), subscription.WatcherSpecID) }()
		select {
		case <-lease.Done():
		case <-time.After(time.Second):
			t.Fatal("unsubscribe did not close local owner first")
		}
		call := readSecretCall(t, socket)
		var item acceleratorsecret.SecretListItem
		item.Metadata.Namespace = "team"
		sendSecretEvent(t, socket, acceleratorsecret.EventResource, acceleratorsecret.SecretResourceEvent{Type: "ADDED", ResourceType: acceleratorsecret.SecretResourceType, Namespace: "team", WatcherSpecID: subscription.WatcherSpecID, Resource: item})
		sendSecretResult(t, socket, call.ID, nil)
		if err := <-completed; err != nil {
			t.Fatal(err)
		}
		if lease.Reason() != acceleratorsecret.ReasonClosed || client.Reason() != "" {
			t.Fatalf("unsubscribe reasons=%s/%s", lease.Reason(), client.Reason())
		}
		if err := client.UnsubscribeSecretWatcher(context.Background(), subscription.WatcherSpecID); err != nil {
			t.Fatalf("repeated unsubscribe=%v", err)
		}
	})

	t.Run("malformed current event fails closed", func(t *testing.T) {
		client, socket, _, _ := secretClientFixture(t)
		subscription, _ := subscribeSecretWatch(t, client, socket, "team", false)
		payload := []byte(`{"type":"event","name":"watcher-status","data":{"watcherSpecId":"` + string(subscription.WatcherSpecID) + `","status":"connected","extra":"raw-marker"}}`)
		socket.messages <- socketMessage{kind: websocket.TextMessage, payload: payload}
		select {
		case <-client.Done():
		case <-time.After(time.Second):
			t.Fatal("malformed event did not fail closed")
		}
		if client.Reason() != acceleratorsecret.ReasonProtocol || client.session.EndReason() != SessionProtocolFailed {
			t.Fatalf("malformed terminal=%s/%s", client.Reason(), client.session.EndReason())
		}
	})
}

func TestSecretClientTerminalRaceMatrix(t *testing.T) {
	client, socket, lease, slot := secretClientFixture(t)
	_, watchLease := subscribeSecretWatch(t, client, socket, "race", false)
	type pendingOutcome struct {
		value string
		err   error
	}
	pendingDone := make(chan pendingOutcome, 1)
	pendingCtx, pendingCancel := context.WithCancel(context.Background())
	go func() {
		value, err := client.GetSecretYaml(pendingCtx, "race", "pending")
		pendingDone <- pendingOutcome{value: value, err: err}
	}()
	pendingCall := readSecretCall(t, socket)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var workers sync.WaitGroup
	for index := 0; index < 64; index++ {
		workers.Add(4)
		go func() { defer workers.Done(); client.Close(ctx) }()
		go func() { defer workers.Done(); watchLease.Close(ctx) }()
		go func() { defer workers.Done(); lease.Close() }()
		go func() {
			defer workers.Done()
			slot.mu.Lock()
			slot.advanceSessionLeaseEpochLocked(false)
			slot.mu.Unlock()
		}()
	}
	workers.Add(3)
	go func() {
		defer workers.Done()
		pendingCancel()
	}()
	go func() {
		defer workers.Done()
		payload, _ := acceleratorsecret.EncodeResultOK(pendingCall.ID, "pending")
		socket.messages <- socketMessage{kind: websocket.TextMessage, payload: payload}
	}()
	go func() {
		defer workers.Done()
		status := acceleratorsecret.SecretWatcherStatus{WatcherSpecID: watchLease.Subscription().WatcherSpecID, Status: acceleratorsecret.WatchStatusConnected}
		for index := 0; index <= acceleratorsecret.SubscriptionEventSlots; index++ {
			sendSecretEvent(t, socket, acceleratorsecret.EventWatcherStatus, status)
		}
	}()
	client.session.beginTermination(SessionPeerClosed, false)
	workers.Wait()
	select {
	case outcome := <-pendingDone:
		if outcome.err == nil && outcome.value != "pending" {
			t.Fatalf("pending completion=%#v", outcome)
		}
	case <-time.After(time.Second):
		t.Fatal("terminal matrix stranded pending call")
	}
	select {
	case <-client.Done():
	case <-time.After(time.Second):
		t.Fatal("terminal race did not complete client")
	}
	select {
	case <-watchLease.Done():
	case <-time.After(time.Second):
		t.Fatal("terminal race did not complete watch")
	}
	client.mu.Lock()
	calls, lists, subscriptions := len(client.calls), len(client.lists), len(client.subscriptions)
	client.mu.Unlock()
	client.session.arbiter.mu.Lock()
	handler, pendingFrames := client.session.arbiter.handler, len(client.session.arbiter.pending)
	client.session.arbiter.mu.Unlock()
	if calls != 0 || lists != 0 || subscriptions != 0 || slot.sessionLeaseCount != 0 || handler != nil || pendingFrames != 0 || len(watchLease.Events()) != 0 {
		t.Fatalf("terminal ownership=%d/%d/%d leases=%d", calls, lists, subscriptions, slot.sessionLeaseCount)
	}

	for _, test := range []struct {
		name    string
		trigger func(*Coordinator)
	}{
		{name: "context switch", trigger: func(c *Coordinator) { c.FenceContextSwitch("ctx") }},
		{name: "quiesce", trigger: func(c *Coordinator) { c.Quiesce(context.Background()) }},
		{name: "shutdown", trigger: func(c *Coordinator) {
			c.Quiesce(context.Background())
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			c.StopProducers(ctx)
			c.Close(context.Background())
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			coordinator, current, currentSocket, currentLease, currentSlot, demand := coordinatorSecretClientFixture(t)
			_, currentWatch := subscribeSecretWatch(t, current, currentSocket, "terminal", false)
			callDone := make(chan error, 1)
			go func() {
				_, err := current.GetSecretYaml(context.Background(), "terminal", "pending")
				callDone <- err
			}()
			_ = readSecretCall(t, currentSocket)
			test.trigger(coordinator)
			select {
			case err := <-callDone:
				if err == nil || err.Error() != string(acceleratorsecret.ReasonSessionUnavailable) {
					t.Fatalf("pending terminal result=%v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("coordinator terminal stranded pending call")
			}
			select {
			case <-currentWatch.Done():
			case <-time.After(time.Second):
				t.Fatal("coordinator terminal stranded watch")
			}
			assertSecretClientTerminalResidue(t, current, currentLease, currentSlot, currentWatch)
			demand.Close()
		})
	}

	t.Run("generation replacement", func(t *testing.T) {
		current, currentSocket, currentLease, currentSlot := secretClientFixture(t)
		_, currentWatch := subscribeSecretWatch(t, current, currentSocket, "replacement", false)
		callDone := make(chan error, 1)
		go func() {
			_, err := current.GetSecretYaml(context.Background(), "replacement", "pending")
			callDone <- err
		}()
		_ = readSecretCall(t, currentSocket)
		currentSlot.mu.Lock()
		currentSlot.advanceSessionLeaseEpochLocked(true)
		currentSlot.mu.Unlock()
		if err := <-callDone; err == nil || err.Error() != string(acceleratorsecret.ReasonSessionUnavailable) {
			t.Fatalf("replacement pending result=%v", err)
		}
		<-currentWatch.Done()
		assertSecretClientTerminalResidue(t, current, currentLease, currentSlot, currentWatch)
	})

	t.Run("Browser handoff", func(t *testing.T) {
		workload := connectorWorkload(t)
		order := []string{}
		currentSocket := newRecordedSessionSocket(&order)
		currentTunnel := newRecordedTunnel(&order)
		currentSession := newConnectedSession(workload.connectorState.receipt, server.AuthenticatedAcceleratorInfo{Capabilities: agent.V1Capabilities()}, connectedIdentity{sessionID: "session-a", instanceID: "instance-a", generation: 1}, currentSocket, currentTunnel, processResumeClock{})
		if !workload.publishCurrentSession(currentSession) {
			t.Fatal("Browser handoff session publication rejected")
		}
		currentSlot := newContextSlot("ctx", 1)
		currentSlot.state, currentSlot.session, currentSlot.workload = CoordinatorActive, currentSession, workload
		currentSlot.sessionLeaseEpoch, currentSlot.sessionLeaseCount, currentSlot.sessionLeaseRevoked = 1, 1, make(chan struct{})
		state := &sessionLeaseState{coordinator: &Coordinator{}, slot: currentSlot, leaseEpoch: 1, session: currentSession, revoked: currentSlot.sessionLeaseRevoked, closedCh: make(chan struct{})}
		currentLease := &SessionLease{state: state}
		current, err := newSecretRPCClient(currentLease, bytes.NewReader(make([]byte, 16)))
		if err != nil {
			t.Fatal(err)
		}
		_, currentWatch := subscribeSecretWatch(t, current, currentSocket, "browser", false)
		callDone := make(chan error, 1)
		go func() {
			_, err := current.GetSecretYaml(context.Background(), "browser", "pending")
			callDone <- err
		}()
		_ = readSecretCall(t, currentSocket)
		owned, ok := currentSession.detachCreatorForBrowser(context.Background())
		if !ok || owned == nil {
			t.Fatal("Browser handoff failed")
		}
		if err := <-callDone; err == nil || err.Error() != string(acceleratorsecret.ReasonSessionUnavailable) {
			t.Fatalf("Browser pending result=%v", err)
		}
		<-currentWatch.Done()
		assertSecretClientTerminalResidue(t, current, currentLease, currentSlot, currentWatch)
		owned.tunnel.Stop()
	})
}

func coordinatorSecretClientFixture(t *testing.T) (*Coordinator, *secretRPCClient, *recordedSessionSocket, *SessionLease, *contextSlot, *SecretDemandLease) {
	t.Helper()
	coordinator, _, clock := newCoordinatorHarness(t)
	workload := connectorWorkload(t)
	order := []string{}
	socket := newRecordedSessionSocket(&order)
	session := newConnectedSession(workload.connectorState.receipt, server.AuthenticatedAcceleratorInfo{Capabilities: agent.V1Capabilities()}, connectedIdentity{sessionID: "session-a", instanceID: "instance-a", generation: 1}, socket, newRecordedTunnel(&order), clock)
	if !workload.publishCurrentSession(session) {
		t.Fatal("coordinator session publication rejected")
	}
	slot := newContextSlot("ctx", coordinator.currentEpoch)
	slot.state, slot.workload, slot.session = CoordinatorActive, workload, session
	slot.advanceSessionLeaseEpochLocked(true)
	coordinator.mu.Lock()
	coordinator.slots[coordinator.currentEpoch] = slot
	coordinator.mu.Unlock()
	demand := coordinator.AcquireSecretDemand(context.Background(), "ctx").Lease
	lease, ok := demand.TrySession()
	if !ok {
		t.Fatal("coordinator session lease unavailable")
	}
	client, err := newSecretRPCClient(lease, bytes.NewReader(make([]byte, 16)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		client.Close(context.Background())
		demand.Close()
		_ = session.Close(context.Background())
		stopCoordinator(t, coordinator)
	})
	return coordinator, client, socket, lease, slot, demand
}

func assertSecretClientTerminalResidue(t *testing.T, client *secretRPCClient, lease *SessionLease, slot *contextSlot, watch SecretWatchLease) {
	t.Helper()
	select {
	case <-client.Done():
	case <-time.After(time.Second):
		t.Fatal("client terminal did not close")
	}
	client.mu.Lock()
	calls, lists, subscriptions := len(client.calls), len(client.lists), len(client.subscriptions)
	client.mu.Unlock()
	client.session.arbiter.mu.Lock()
	handler, pendingFrames := client.session.arbiter.handler, len(client.session.arbiter.pending)
	client.session.arbiter.mu.Unlock()
	slot.mu.Lock()
	leaseCount := slot.sessionLeaseCount
	slot.mu.Unlock()
	if calls != 0 || lists != 0 || subscriptions != 0 || leaseCount != 0 || lease.Session() != nil || handler != nil || pendingFrames != 0 || len(watch.Events()) != 0 {
		t.Fatalf("terminal residue calls=%d lists=%d subscriptions=%d leases=%d handler=%v frames=%d events=%d", calls, lists, subscriptions, leaseCount, handler != nil, pendingFrames, len(watch.Events()))
	}
}
