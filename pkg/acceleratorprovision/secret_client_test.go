package acceleratorprovision

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"kubikles/pkg/acceleratorsecret"
	"kubikles/pkg/k8s"
)

func secretClientFixture(t *testing.T) (*secretRPCClient, *recordedSessionSocket, *SessionLease, *contextSlot) {
	t.Helper()
	order := []string{}
	socket := newRecordedSessionSocket(&order)
	client, lease, slot := secretClientFixtureForSocket(t, socket, newRecordedTunnel(&order))
	return client, socket, lease, slot
}

func secretClientFixtureForSocket(t *testing.T, socket sessionSocket, active tunnel) (*secretRPCClient, *SessionLease, *contextSlot) {
	t.Helper()
	session := sessionFixture(t, socket, active)
	slot := newContextSlot("ctx", 1)
	slot.state = CoordinatorActive
	slot.session = session
	slot.sessionLeaseEpoch = 1
	slot.sessionLeaseCount = 1
	slot.sessionLeaseRevoked = make(chan struct{})
	coordinator := &Coordinator{}
	state := &sessionLeaseState{coordinator: coordinator, slot: slot, leaseEpoch: 1, session: session, revoked: slot.sessionLeaseRevoked, closedCh: make(chan struct{})}
	lease := &SessionLease{state: state}
	client, err := newSecretRPCClient(lease, bytes.NewReader(make([]byte, 16)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		client.Close(context.Background())
		_ = session.Close(context.Background())
	})
	return client, lease, slot
}

func readSecretCall(t *testing.T, socket *recordedSessionSocket) acceleratorsecret.CallFrame {
	t.Helper()
	select {
	case raw := <-socket.writes:
		call, err := acceleratorsecret.DecodeCall(raw)
		if err != nil {
			t.Fatal(err)
		}
		return call
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Secret call")
		return acceleratorsecret.CallFrame{}
	}
}

func sendSecretResult(t *testing.T, socket *recordedSessionSocket, id string, value interface{}) {
	t.Helper()
	payload, err := acceleratorsecret.EncodeResultOK(id, value)
	if err != nil {
		t.Fatal(err)
	}
	socket.messages <- socketMessage{kind: websocket.TextMessage, payload: payload}
}

func rawStringArg(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var value string
	if json.Unmarshal(raw, &value) != nil {
		t.Fatalf("argument=%s", raw)
	}
	return value
}

func TestSecretClientSafeTypedSurface(t *testing.T) {
	want := []string{"CancelListRequest", "Close", "Done", "GetSecretData", "GetSecretYaml", "ListSecretsMetadata", "Reason", "SubscribeSecretWatcher", "UnsubscribeSecretWatcher"}
	clientType := reflect.TypeOf((*SecretRPCClient)(nil)).Elem()
	if clientType.NumMethod() != len(want) {
		t.Fatalf("SecretRPCClient methods=%d", clientType.NumMethod())
	}
	for index, name := range want {
		if clientType.Method(index).Name != name {
			t.Fatalf("method[%d]=%s want=%s", index, clientType.Method(index).Name, name)
		}
	}
	if _, err := NewSecretRPCClient(nil); err == nil || err.Error() != string(acceleratorsecret.ReasonSessionUnavailable) {
		t.Fatalf("nil lease error=%v", err)
	}
	client, _, lease, slot := secretClientFixture(t)
	copyLease := *lease
	if _, err := NewSecretRPCClient(&copyLease); err == nil {
		t.Fatal("copied claimed lease attached a second client")
	}
	if _, err := NewSecretRPCClient(lease); err == nil {
		t.Fatal("claimed lease attached twice")
	}
	if rendered := fmt.Sprintf("%+v %#v", client, client); strings.Contains(rendered, client.nonce) || rendered != "<accelerator secret client> <accelerator secret client>" {
		t.Fatalf("unsafe client formatter=%q", rendered)
	}
	slot.mu.Lock()
	slot.advanceSessionLeaseEpochLocked(false)
	slot.mu.Unlock()
	select {
	case <-client.Done():
	case <-time.After(time.Second):
		t.Fatal("lease revocation did not terminate client")
	}
	if client.Reason() != acceleratorsecret.ReasonSessionUnavailable || lease.Session() != nil {
		t.Fatalf("revoked reason/session=%s/%v", client.Reason(), lease.Session())
	}
}

func TestSecretClientAllSixMethodShapes(t *testing.T) {
	client, socket, _, _ := secretClientFixture(t)

	listResult := make(chan []k8s.SecretListItem, 1)
	listErr := make(chan error, 1)
	go func() {
		items, err := client.ListSecretsMetadata(context.Background(), "desktop-request", "team", true)
		listResult <- items
		listErr <- err
	}()
	listCall := readSecretCall(t, socket)
	if listCall.Operation != acceleratorsecret.OperationListSecretsMetadata || len(listCall.Args) != 3 || rawStringArg(t, listCall.Args[0]) == "desktop-request" || rawStringArg(t, listCall.Args[1]) != "team" || string(listCall.Args[2]) != "true" {
		t.Fatalf("list call=%#v", listCall)
	}
	var summary acceleratorsecret.SecretListItem
	summary.Metadata.Name, summary.Metadata.Namespace = "safe", "team"
	summary.Type, summary.DataKeys = "Opaque", 2
	sendSecretResult(t, socket, listCall.ID, []acceleratorsecret.SecretListItem{summary})
	if err := <-listErr; err != nil {
		t.Fatal(err)
	}
	items := <-listResult
	if len(items) != 1 || items[0].Metadata.Name != "safe" || items[0].Metadata.Labels != nil || items[0].Metadata.Annotations != nil {
		t.Fatalf("list result=%#v", items)
	}

	dataResult := make(chan []k8s.DataEntry, 1)
	dataErr := make(chan error, 1)
	go func() {
		value, err := client.GetSecretData(context.Background(), "team", "safe")
		dataResult <- value
		dataErr <- err
	}()
	dataCall := readSecretCall(t, socket)
	if dataCall.Operation != acceleratorsecret.OperationGetSecretData || rawStringArg(t, dataCall.Args[0]) != "team" || rawStringArg(t, dataCall.Args[1]) != "safe" {
		t.Fatalf("data call=%#v", dataCall)
	}
	sendSecretResult(t, socket, dataCall.ID, []k8s.DataEntry{{Key: "key", Value: "detail-marker"}})
	if err := <-dataErr; err != nil {
		t.Fatal(err)
	}
	if got := <-dataResult; len(got) != 1 || got[0].Value != "detail-marker" {
		t.Fatalf("data=%#v", got)
	}

	yamlResult := make(chan string, 1)
	yamlErr := make(chan error, 1)
	go func() {
		value, err := client.GetSecretYaml(context.Background(), "team", "safe")
		yamlResult <- value
		yamlErr <- err
	}()
	yamlCall := readSecretCall(t, socket)
	if yamlCall.Operation != acceleratorsecret.OperationGetSecretYAML {
		t.Fatalf("yaml call=%#v", yamlCall)
	}
	sendSecretResult(t, socket, yamlCall.ID, "yaml-detail-marker")
	if err := <-yamlErr; err != nil {
		t.Fatal(err)
	}
	if got := <-yamlResult; got != "yaml-detail-marker" {
		t.Fatalf("yaml=%q", got)
	}

	blockedList := make(chan error, 1)
	go func() {
		_, err := client.ListSecretsMetadata(context.Background(), "cancel-owner", "team", false)
		blockedList <- err
	}()
	ownedListCall := readSecretCall(t, socket)
	remoteID := rawStringArg(t, ownedListCall.Args[0])
	cancelResult := make(chan bool, 1)
	cancelErr := make(chan error, 1)
	go func() {
		value, err := client.CancelListRequest(context.Background(), "cancel-owner")
		cancelResult <- value
		cancelErr <- err
	}()
	cancelCall := readSecretCall(t, socket)
	if cancelCall.Operation != acceleratorsecret.OperationCancelListRequest || rawStringArg(t, cancelCall.Args[0]) != remoteID {
		t.Fatalf("cancel call=%#v remote=%q", cancelCall, remoteID)
	}
	sendSecretResult(t, socket, cancelCall.ID, true)
	if err := <-blockedList; err == nil || err.Error() != string(acceleratorsecret.ReasonCanceled) {
		t.Fatalf("local list cancel=%v", err)
	}
	canceled, cancelCallErr := <-cancelResult, <-cancelErr
	if cancelCallErr != nil || !canceled {
		t.Fatalf("cancel result=%v/%v", canceled, cancelCallErr)
	}

	subscriptionResult := make(chan acceleratorsecret.SecretWatchSubscription, 1)
	leaseResult := make(chan SecretWatchLease, 1)
	subscribeErr := make(chan error, 1)
	go func() {
		subscription, watchLease, err := client.SubscribeSecretWatcher(context.Background(), "team", true)
		subscriptionResult <- subscription
		leaseResult <- watchLease
		subscribeErr <- err
	}()
	subscribeCall := readSecretCall(t, socket)
	if subscribeCall.Operation != acceleratorsecret.OperationSubscribeSecretWatcher || rawStringArg(t, subscribeCall.Args[0]) != "team" || string(subscribeCall.Args[1]) != "true" {
		t.Fatalf("subscribe call=%#v", subscribeCall)
	}
	wantSubscription := acceleratorsecret.SecretWatchSubscription{WatcherSpecID: acceleratorsecret.SecretWatchSpecIDFor("team", true)}
	sendSecretResult(t, socket, subscribeCall.ID, wantSubscription)
	if err := <-subscribeErr; err != nil {
		t.Fatal(err)
	}
	if got := <-subscriptionResult; got != wantSubscription {
		t.Fatalf("subscription=%#v", got)
	}
	watchLease := <-leaseResult

	unsubscribeErr := make(chan error, 1)
	go func() {
		unsubscribeErr <- client.UnsubscribeSecretWatcher(context.Background(), wantSubscription.WatcherSpecID)
	}()
	unsubscribeCall := readSecretCall(t, socket)
	if unsubscribeCall.Operation != acceleratorsecret.OperationUnsubscribeSecretWatcher || rawStringArg(t, unsubscribeCall.Args[0]) != string(wantSubscription.WatcherSpecID) {
		t.Fatalf("unsubscribe call=%#v", unsubscribeCall)
	}
	sendSecretResult(t, socket, unsubscribeCall.ID, nil)
	if err := <-unsubscribeErr; err != nil {
		t.Fatal(err)
	}
	if watchLease.Reason() != acceleratorsecret.ReasonClosed {
		t.Fatalf("watch reason=%s", watchLease.Reason())
	}
}

func TestSecretClientCancellationAndLateResult(t *testing.T) {
	client, socket, _, _ := secretClientFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	listDone := make(chan error, 1)
	go func() {
		_, err := client.ListSecretsMetadata(ctx, "same-owner", "team", false)
		listDone <- err
	}()
	listCall := readSecretCall(t, socket)
	if _, err := client.ListSecretsMetadata(context.Background(), "same-owner", "team", false); err == nil || err.Error() != string(acceleratorsecret.ReasonCapacity) {
		t.Fatalf("duplicate list error=%v", err)
	}
	cancel()
	if err := <-listDone; err == nil || err.Error() != string(acceleratorsecret.ReasonCanceled) {
		t.Fatalf("canceled list=%v", err)
	}
	bestEffort := readSecretCall(t, socket)
	if bestEffort.Operation != acceleratorsecret.OperationCancelListRequest || rawStringArg(t, bestEffort.Args[0]) != rawStringArg(t, listCall.Args[0]) {
		t.Fatalf("best effort=%#v", bestEffort)
	}
	var late acceleratorsecret.SecretListItem
	late.Metadata.Name = "late-sensitive-marker"
	sendSecretResult(t, socket, listCall.ID, []acceleratorsecret.SecretListItem{late})
	if client.Reason() != "" {
		t.Fatalf("late canceled result terminated client: %s", client.Reason())
	}

	type outcome struct {
		value string
		err   error
	}
	results := make(chan outcome, acceleratorsecret.MaxConcurrentCalls)
	for index := 0; index < acceleratorsecret.MaxConcurrentCalls; index++ {
		go func(index int) {
			value, err := client.GetSecretYaml(context.Background(), "team", fmt.Sprintf("secret-%d", index))
			results <- outcome{value: value, err: err}
		}(index)
	}
	calls := make([]acceleratorsecret.CallFrame, 0, acceleratorsecret.MaxConcurrentCalls)
	var priorCounter uint64
	for index := 0; index < acceleratorsecret.MaxConcurrentCalls; index++ {
		call := readSecretCall(t, socket)
		_, counter, ok := acceleratorsecret.ParseCallID(call.ID)
		if !ok || counter <= priorCounter {
			t.Fatalf("wire call order=%d after %d", counter, priorCounter)
		}
		priorCounter = counter
		calls = append(calls, call)
	}
	if _, err := client.GetSecretYaml(context.Background(), "team", "ninth"); err == nil || err.Error() != string(acceleratorsecret.ReasonCapacity) {
		t.Fatalf("ninth call=%v", err)
	}
	for index := len(calls) - 1; index >= 0; index-- {
		sendSecretResult(t, socket, calls[index].ID, rawStringArg(t, calls[index].Args[1]))
	}
	seen := make(map[string]struct{}, len(calls))
	for range calls {
		result := <-results
		if result.err != nil {
			t.Fatal(result.err)
		}
		seen[result.value] = struct{}{}
	}
	if len(seen) != len(calls) {
		t.Fatalf("out-of-order results=%v", seen)
	}

	responseCtx, responseCancel := context.WithCancel(context.Background())
	responseEntered, responseRelease := make(chan struct{}), make(chan struct{})
	client.beforeRemoveCall = func() {
		close(responseEntered)
		<-responseRelease
	}
	responseDone := make(chan outcome, 1)
	go func() {
		value, err := client.GetSecretYaml(responseCtx, "team", "response-wins")
		responseDone <- outcome{value: value, err: err}
	}()
	responseCall := readSecretCall(t, socket)
	responseCancel()
	select {
	case <-responseEntered:
	case <-time.After(time.Second):
		t.Fatal("response race did not enter cancel removal")
	}
	encoded, err := json.Marshal("response-wins")
	if err != nil {
		t.Fatal(err)
	}
	if !client.handleResult(acceleratorsecret.ResultFrame{ID: responseCall.ID, Status: acceleratorsecret.ResultStatusOK, Result: encoded}) {
		t.Fatal("current response was rejected")
	}
	close(responseRelease)
	result := <-responseDone
	if result.err != nil || result.value != "response-wins" {
		t.Fatalf("response/cancel winner=%#v", result)
	}
	client.beforeRemoveCall = nil
	latePayload, _ := json.Marshal("late-response-private-marker")
	if !client.handleResult(acceleratorsecret.ResultFrame{ID: responseCall.ID, Status: acceleratorsecret.ResultStatusOK, Result: latePayload}) {
		t.Fatal("late response was not safely discarded")
	}
	clear(latePayload)
	client.mu.Lock()
	remainingCalls, remainingLists := len(client.calls), len(client.lists)
	client.mu.Unlock()
	if remainingCalls != 0 || remainingLists != 0 || client.Reason() != "" {
		t.Fatalf("response race residue=%d/%d reason=%s", remainingCalls, remainingLists, client.Reason())
	}
}

func TestSecretClientCallBoundsAndDeadlines(t *testing.T) {
	t.Run("exact operation deadlines", func(t *testing.T) {
		want := map[acceleratorsecret.Operation]time.Duration{
			acceleratorsecret.OperationListSecretsMetadata:      60 * time.Second,
			acceleratorsecret.OperationGetSecretData:            30 * time.Second,
			acceleratorsecret.OperationGetSecretYAML:            30 * time.Second,
			acceleratorsecret.OperationCancelListRequest:        5 * time.Second,
			acceleratorsecret.OperationSubscribeSecretWatcher:   10 * time.Second,
			acceleratorsecret.OperationUnsubscribeSecretWatcher: 5 * time.Second,
		}
		for operation, duration := range want {
			if got := acceleratorsecret.OperationTimeout(operation); got != duration {
				t.Fatalf("%s timeout=%s want=%s", operation, got, duration)
			}
		}
	})

	t.Run("caller deadline removes list before late result", func(t *testing.T) {
		client, socket, _, _ := secretClientFixture(t)
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		defer cancel()
		completed := make(chan error, 1)
		go func() {
			_, err := client.ListSecretsMetadata(ctx, "deadline-owner", "team", false)
			completed <- err
		}()
		call := readSecretCall(t, socket)
		if err := <-completed; err == nil || err.Error() != string(acceleratorsecret.ReasonDeadline) {
			t.Fatalf("deadline result=%v", err)
		}
		cancelCall := readSecretCall(t, socket)
		if cancelCall.Operation != acceleratorsecret.OperationCancelListRequest || rawStringArg(t, cancelCall.Args[0]) != rawStringArg(t, call.Args[0]) {
			t.Fatalf("deadline cancel=%#v", cancelCall)
		}
		client.mu.Lock()
		calls, lists := len(client.calls), len(client.lists)
		client.mu.Unlock()
		if calls != 0 || lists != 0 {
			t.Fatalf("deadline ownership=%d/%d", calls, lists)
		}
		var late acceleratorsecret.SecretListItem
		late.Metadata.Name = "late-deadline-private-marker"
		sendSecretResult(t, socket, call.ID, []acceleratorsecret.SecretListItem{late})
		if client.Reason() != "" {
			t.Fatalf("late deadline result changed client reason=%s", client.Reason())
		}
	})

	t.Run("writer saturation fixes local cancel outcome", func(t *testing.T) {
		socket := newBlockedWriterSessionSocket()
		client, _, _ := secretClientFixtureForSocket(t, socket, newRecordedTunnel(&[]string{}))
		listDone := make(chan error, 1)
		go func() {
			_, err := client.ListSecretsMetadata(context.Background(), "saturated-owner", "team", false)
			listDone <- err
		}()
		select {
		case <-socket.writeEntered:
		case <-time.After(time.Second):
			t.Fatal("list call did not occupy writer")
		}
		for index := 0; index < acceleratorsecret.OutboundSocketSlots; index++ {
			if reason := client.session.sendApplicationFrame([]byte(`{"type":"call"}`)); reason != "" {
				t.Fatalf("fill[%d]=%s", index, reason)
			}
		}
		if reason := client.session.sendApplicationFrame([]byte(`{"type":"call"}`)); reason != acceleratorsecret.ReasonCapacity {
			t.Fatalf("overflow reason=%s", reason)
		}
		canceled, err := client.CancelListRequest(context.Background(), "saturated-owner")
		if canceled || err == nil || err.Error() != string(acceleratorsecret.ReasonCapacity) {
			t.Fatalf("cancel RPC saturation=%v/%v", canceled, err)
		}
		if err := <-listDone; err == nil || err.Error() != string(acceleratorsecret.ReasonCanceled) {
			t.Fatalf("local cancel outcome=%v", err)
		}
		client.mu.Lock()
		calls, lists := len(client.calls), len(client.lists)
		client.mu.Unlock()
		if calls != 0 || lists != 0 {
			t.Fatalf("saturated cancel ownership=%d/%d", calls, lists)
		}
		close(socket.writeRelease)
	})

	t.Run("oversized response terminates pending call", func(t *testing.T) {
		client, socket, _, _ := secretClientFixture(t)
		completed := make(chan error, 1)
		go func() {
			_, err := client.GetSecretYaml(context.Background(), "team", "oversized")
			completed <- err
		}()
		_ = readSecretCall(t, socket)
		payload := bytes.Repeat([]byte{'x'}, acceleratorsecret.MaxCreatorResponseFrameBytes+1)
		socket.messages <- socketMessage{kind: websocket.TextMessage, payload: payload}
		select {
		case err := <-completed:
			if err == nil || err.Error() != string(acceleratorsecret.ReasonProtocol) {
				t.Fatalf("oversized result=%v", err)
			}
		case <-time.After(time.Second):
			t.Fatal("oversized response stranded pending call")
		}
		if client.Reason() != acceleratorsecret.ReasonProtocol {
			t.Fatalf("oversized terminal reason=%s", client.Reason())
		}
		client.mu.Lock()
		calls, lists := len(client.calls), len(client.lists)
		client.mu.Unlock()
		if calls != 0 || lists != 0 {
			t.Fatalf("oversized terminal residue=%d/%d", calls, lists)
		}
	})
}

func TestSessionFrameArbiterSingleAttachAndProtocolFence(t *testing.T) {
	client, socket, _, _ := secretClientFixture(t)
	if _, ok := client.session.attachSecretFrameHandler(func(acceleratorsecret.ServerFrame) bool { return true }); ok {
		t.Fatal("second frame handler attached")
	}
	<-socket.readLimitSet
	if got := socket.readLimitValue(); got != int64(acceleratorsecret.MaxCreatorResponseFrameBytes) {
		t.Fatalf("response read limit=%d", got)
	}
	socket.messages <- socketMessage{kind: websocket.TextMessage, payload: []byte(`{"type":"event","name":"unknown","data":null}`)}
	select {
	case <-client.Done():
	case <-time.After(time.Second):
		t.Fatal("unknown frame did not terminate client")
	}
	if client.Reason() != acceleratorsecret.ReasonProtocol {
		t.Fatalf("protocol session reason=%s", client.Reason())
	}
}

func TestSecretClientTerminalWakesCancellationLoser(t *testing.T) {
	for _, test := range []struct {
		name    string
		context func() (context.Context, func())
	}{
		{name: "cancel", context: func() (context.Context, func()) {
			return context.WithCancel(context.Background())
		}},
		{name: "deadline", context: func() (context.Context, func()) {
			ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
			return ctx, cancel
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			client, socket, _, slot := secretClientFixture(t)
			entered, release := make(chan struct{}), make(chan struct{})
			client.beforeRemoveCall = func() {
				close(entered)
				<-release
			}
			ctx, trigger := test.context()
			result := make(chan error, 1)
			go func() {
				_, err := client.GetSecretYaml(ctx, "team", "pending")
				result <- err
			}()
			_ = readSecretCall(t, socket)
			trigger()
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("caller did not enter cancellation removal barrier")
			}
			terminated := make(chan struct{})
			go func() {
				client.terminate(acceleratorsecret.ReasonSessionUnavailable)
				close(terminated)
			}()
			select {
			case <-terminated:
			case <-time.After(time.Second):
				t.Fatal("terminal path blocked before publishing pending outcome")
			}
			close(release)
			select {
			case err := <-result:
				if err == nil || err.Error() != string(acceleratorsecret.ReasonSessionUnavailable) {
					t.Fatalf("terminal outcome=%v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("cancellation loser remained blocked after terminal cleanup")
			}
			client.mu.Lock()
			calls, lists := len(client.calls), len(client.lists)
			client.mu.Unlock()
			if calls != 0 || lists != 0 || slot.sessionLeaseCount != 0 {
				t.Fatalf("terminal residue calls=%d lists=%d leases=%d", calls, lists, slot.sessionLeaseCount)
			}
		})
	}
}

func TestSessionLeaseCopyCloseAndSynchronousRevocation(t *testing.T) {
	client, _, lease, slot := secretClientFixture(t)
	copyLease := *lease
	copyLease.Close()
	lease.Close()
	if slot.sessionLeaseCount != 0 || lease.Session() != nil || copyLease.Session() != nil {
		t.Fatalf("copy close count/session=%d/%v/%v", slot.sessionLeaseCount, lease.Session(), copyLease.Session())
	}
	select {
	case <-client.Done():
	case <-time.After(time.Second):
		t.Fatal("closed owned lease did not terminate client")
	}
}
