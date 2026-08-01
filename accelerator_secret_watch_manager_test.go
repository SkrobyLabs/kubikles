package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"kubikles/pkg/agent"
	"kubikles/pkg/k8s"
	"kubikles/pkg/server"
)

type secretWatchLeaseStore struct {
	mu     sync.Mutex
	leases map[agent.SessionID]server.AcceleratorSessionLease
}

func (s *secretWatchLeaseStore) lookup(id agent.SessionID) (server.AcceleratorSessionLease, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	lease, ok := s.leases[id]
	return lease, ok
}

func (s *secretWatchLeaseStore) set(id agent.SessionID, lease server.AcceleratorSessionLease) {
	s.mu.Lock()
	s.leases[id] = lease
	s.mu.Unlock()
}

type capturedSecretWatchEvent struct {
	targets []server.AcceleratorSessionTarget
	event   server.Event
}

type secretWatchEventCollector struct {
	mu     sync.Mutex
	events []capturedSecretWatchEvent
	wake   chan struct{}
}

type secretWatchGenerationGate struct {
	connected        chan server.AcceleratorSessionSnapshot
	disconnected     chan server.AcceleratorSessionSnapshot
	generation2Ready chan struct{}
	release          chan struct{}
}

func (g *secretWatchGenerationGate) SessionConnected(snapshot server.AcceleratorSessionSnapshot) {
	g.connected <- snapshot
	if snapshot.Generation == 2 {
		close(g.generation2Ready)
		<-g.release
	}
}

func (g *secretWatchGenerationGate) SessionDisconnected(snapshot server.AcceleratorSessionSnapshot) {
	g.disconnected <- snapshot
}

func (*secretWatchGenerationGate) SessionRevoked(server.AcceleratorSessionSnapshot) {}

func newSecretWatchEventCollector() *secretWatchEventCollector {
	return &secretWatchEventCollector{wake: make(chan struct{}, 64)}
}

func (c *secretWatchEventCollector) emit(targets []server.AcceleratorSessionTarget, event server.Event) {
	c.mu.Lock()
	c.events = append(c.events, capturedSecretWatchEvent{targets: append([]server.AcceleratorSessionTarget(nil), targets...), event: event})
	c.mu.Unlock()
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

func (c *secretWatchEventCollector) snapshot() []capturedSecretWatchEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]capturedSecretWatchEvent(nil), c.events...)
}

func (c *secretWatchEventCollector) waitFor(t *testing.T, predicate func([]capturedSecretWatchEvent) bool) []capturedSecretWatchEvent {
	t.Helper()
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	for {
		events := c.snapshot()
		if predicate(events) {
			return events
		}
		select {
		case <-c.wake:
		case <-timer.C:
			t.Fatalf("timed out waiting for watch events: %#v", events)
		}
	}
}

type blockingSecretWatch struct {
	result      chan watch.Event
	stopStarted chan struct{}
	allowStop   chan struct{}
	stopOnce    sync.Once
}

func newBlockingSecretWatch() *blockingSecretWatch {
	return &blockingSecretWatch{result: make(chan watch.Event, 16), stopStarted: make(chan struct{}), allowStop: make(chan struct{})}
}

func (w *blockingSecretWatch) Stop() {
	w.stopOnce.Do(func() { close(w.stopStarted) })
	<-w.allowStop
}

func (w *blockingSecretWatch) ResultChan() <-chan watch.Event { return w.result }

func secretWatchCall(session string) agent.AuthenticatedCallContext {
	return agent.AuthenticatedCallContext{PrincipalID: agent.PrincipalID("principal-" + session), SessionID: agent.SessionID(session)}
}

func waitSecretWatchSignal(t *testing.T, signal <-chan struct{}, name string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", name)
	}
}

func TestSecretWatchSpecIDKnownVectors(t *testing.T) {
	vectors := []struct {
		namespace string
		exclude   bool
		canonical string
		want      SecretWatchSpecID
	}{
		{"", false, "6b7562696b6c65732f616363656c657261746f722f7365637265742d77617463682d737065632f7631000000000000", "JL7ABILXpK37JlnkPqBX_C5u2tRTQXcmhNViKv2inFA"},
		{"", true, "6b7562696b6c65732f616363656c657261746f722f7365637265742d77617463682d737065632f7631000000000001", "O_6VvhU-5troX55SCA8RnjMDlP8LzAMIqecVgZJtysc"},
		{"a", false, "6b7562696b6c65732f616363656c657261746f722f7365637265742d77617463682d737065632f763100000000016100", "mlml7A109_UQ-SQbVuok0HDbjapIHeSN5B-Ixq9qIUk"},
		{"a", true, "6b7562696b6c65732f616363656c657261746f722f7365637265742d77617463682d737065632f763100000000016101", "bPP_QJ0RdJd0wd7SQubrEUeHKT6QoN5UzOutgNJAM0s"},
		{"team", false, "6b7562696b6c65732f616363656c657261746f722f7365637265742d77617463682d737065632f763100000000047465616d00", "bV1iH_xKsZUA3ktgzNIol6W8h-vDjNBRRJDgIA79_l8"},
		{"team", true, "6b7562696b6c65732f616363656c657261746f722f7365637265742d77617463682d737065632f763100000000047465616d01", "yPYiyr7uVHfeARtua3hig9lQX_Baf2ZXkrLsI4WOvlk"},
	}
	seen := make(map[SecretWatchSpecID]struct{}, len(vectors))
	for _, vector := range vectors {
		t.Run(fmt.Sprintf("%q/%t", vector.namespace, vector.exclude), func(t *testing.T) {
			got := (secretWatchSpec{namespace: vector.namespace, excludeHelmReleases: vector.exclude}).id()
			if got != vector.want || len(got) != 43 {
				t.Fatalf("ID = %q (%d), want %q (canonical bytes %s)", got, len(got), vector.want, vector.canonical)
			}
			if second := (secretWatchSpec{namespace: vector.namespace, excludeHelmReleases: vector.exclude}).id(); second != got {
				t.Fatalf("ID is unstable: %q / %q", got, second)
			}
		})
		if _, duplicate := seen[vector.want]; duplicate {
			t.Fatalf("canonical vectors collided at %q", vector.want)
		}
		seen[vector.want] = struct{}{}
	}
	if (secretWatchSpec{namespace: "team"}).id() != (secretWatchSpec{namespace: "team"}).id() {
		t.Fatal("spec ID unexpectedly depends on session state")
	}
}

func TestSecretWatchSubscribeLifecycle(t *testing.T) {
	one, two, stranger := secretWatchCall("one"), secretWatchCall("two"), secretWatchCall("stranger")
	leases := &secretWatchLeaseStore{leases: map[agent.SessionID]server.AcceleratorSessionLease{
		one.SessionID:      {Generation: 1, Connected: true},
		two.SessionID:      {Generation: 1, Connected: true},
		stranger.SessionID: {Generation: 1, Connected: true},
	}}
	active := newBlockingSecretWatch()
	started := make(chan struct{})
	var sourceOnce sync.Once
	var sourceMu sync.Mutex
	sourceCalls := 0
	manager := newAcceleratorSecretWatchManager(func(context.Context, string, string, k8s.SecretListOptions) (watch.Interface, error) {
		sourceMu.Lock()
		sourceCalls++
		sourceMu.Unlock()
		sourceOnce.Do(func() { close(started) })
		return active, nil
	}, leases.lookup, func([]server.AcceleratorSessionTarget, server.Event) {})
	manager.sleep = func(ctx context.Context, _ time.Duration) error { <-ctx.Done(); return ctx.Err() }

	if _, err := manager.Subscribe(agent.AuthenticatedCallContext{}, "team", true); err == nil {
		t.Fatal("unauthenticated subscribe succeeded")
	}
	disconnected := secretWatchCall("disconnected")
	leases.set(disconnected.SessionID, server.AcceleratorSessionLease{Generation: 1, Connected: false})
	if _, err := manager.Subscribe(disconnected, "team", true); err == nil {
		t.Fatal("disconnected subscribe succeeded")
	}
	first, err := manager.Subscribe(one, "team", true)
	if err != nil {
		t.Fatal(err)
	}
	if again, err := manager.Subscribe(one, "team", true); err != nil || again != first {
		t.Fatalf("same-generation subscribe = %#v, %v; want %#v", again, err, first)
	}
	if secondOwner, err := manager.Subscribe(two, "team", true); err != nil || secondOwner != first {
		t.Fatalf("shared subscribe = %#v, %v; want %#v", secondOwner, err, first)
	}
	waitSecretWatchSignal(t, started, "shared watch start")
	sourceMu.Lock()
	if sourceCalls != 1 {
		t.Fatalf("source calls = %d, want 1", sourceCalls)
	}
	sourceMu.Unlock()

	leases.set(one.SessionID, server.AcceleratorSessionLease{Generation: 2, Connected: true})
	manager.SessionConnected(server.AcceleratorSessionSnapshot{CallContext: one, Generation: 2, Resumed: true})
	if resumed, err := manager.Subscribe(one, "team", true); err != nil || resumed != first {
		t.Fatalf("resumed subscribe = %#v, %v; want %#v", resumed, err, first)
	}
	if err := manager.Unsubscribe(stranger, first.WatcherSpecID); err != nil {
		t.Fatal(err)
	}
	if err := manager.Unsubscribe(one, SecretWatchSpecID("guessed")); err != nil {
		t.Fatal(err)
	}
	manager.mu.Lock()
	if len(manager.streams) != 1 || len(manager.sessionSpecs) != 2 {
		t.Fatalf("guessed/non-owner unsubscribe changed state: streams=%d sessions=%d", len(manager.streams), len(manager.sessionSpecs))
	}
	manager.mu.Unlock()

	leases.set(one.SessionID, server.AcceleratorSessionLease{Generation: 2, Connected: false})
	if err := manager.Unsubscribe(one, first.WatcherSpecID); err != nil {
		t.Fatal(err)
	}
	leases.set(two.SessionID, server.AcceleratorSessionLease{Generation: 1, Connected: false})
	unsubscribed := make(chan error, 1)
	go func() { unsubscribed <- manager.Unsubscribe(two, first.WatcherSpecID) }()
	waitSecretWatchSignal(t, active.stopStarted, "final stream stop")
	select {
	case err := <-unsubscribed:
		t.Fatalf("final unsubscribe returned before stream stop: %v", err)
	default:
	}
	close(active.allowStop)
	if err := <-unsubscribed; err != nil {
		t.Fatal(err)
	}
	if err := manager.Unsubscribe(two, first.WatcherSpecID); err != nil {
		t.Fatalf("repeated unsubscribe = %v, want harmless no-op", err)
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if len(manager.streams) != 0 || len(manager.sessionSpecs) != 0 || len(manager.sessionGenerations) != 0 {
		t.Fatalf("final unsubscribe retained state: streams=%d specs=%d generations=%d", len(manager.streams), len(manager.sessionSpecs), len(manager.sessionGenerations))
	}
}

func TestSecretWatchReconnectGapDropsWithoutReplay(t *testing.T) {
	const creatorToken = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	token, err := server.ParseCreatorToken(creatorToken)
	if err != nil {
		t.Fatal(err)
	}
	creator, err := server.NewCreatorAuthenticator(server.DeriveCreatorVerifier(token).Encoded())
	if err != nil {
		t.Fatal(err)
	}
	active := watch.NewRaceFreeFake()
	var sourceCalls atomic.Int32
	sourceStarted := make(chan struct{})
	var sourceOnce sync.Once
	collector := newSecretWatchEventCollector()
	gate := &secretWatchGenerationGate{
		connected:        make(chan server.AcceleratorSessionSnapshot, 2),
		disconnected:     make(chan server.AcceleratorSessionSnapshot, 2),
		generation2Ready: make(chan struct{}),
		release:          make(chan struct{}),
	}
	var registry *server.AcceleratorSessionRegistry
	manager := newAcceleratorSecretWatchManager(
		func(context.Context, string, string, k8s.SecretListOptions) (watch.Interface, error) {
			sourceCalls.Add(1)
			sourceOnce.Do(func() { close(sourceStarted) })
			return active, nil
		},
		func(id agent.SessionID) (server.AcceleratorSessionLease, bool) {
			return registry.LookupSessionLease(id)
		},
		func(targets []server.AcceleratorSessionTarget, event server.Event) {
			registry.EmitEventToTargets(targets, event)
			collector.emit(targets, event)
		},
	)
	manager.logger = func(string) {}
	manager.sleep = func(ctx context.Context, _ time.Duration) error { <-ctx.Done(); return ctx.Err() }
	registry = server.NewAcceleratorSessionRegistry("gap-evidence", &acceleratorSessionObserverChain{secret: manager, idle: gate})
	authenticator := server.AcceleratorWebSocketAuthenticator{Creator: creator, Registry: registry}
	httpServer := httptest.NewServer(http.HandlerFunc(authenticator.Handler))
	webSocketURL := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws"
	dialer := websocket.Dialer{HandshakeTimeout: time.Second}
	header := http.Header{"Authorization": []string{"Bearer " + creatorToken}}
	var generation1, generation2 *websocket.Conn
	var releaseOnce sync.Once
	releaseGate := func() { releaseOnce.Do(func() { close(gate.release) }) }
	defer func() {
		releaseGate()
		if generation1 != nil {
			_ = generation1.Close()
		}
		if generation2 != nil {
			_ = generation2.Close()
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = registry.Close(ctx)
		_ = manager.ClearAll(ctx)
		httpServer.Close()
	}()

	generation1, response, err := dialer.Dial(webSocketURL, header)
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	if err != nil {
		t.Fatal(err)
	}
	_ = generation1.SetReadDeadline(time.Now().Add(2 * time.Second))
	var initial struct {
		Name string `json:"name"`
		Data struct {
			Generation server.AcceleratorSocketGeneration `json:"generation"`
			Resumed    bool                               `json:"resumed"`
		} `json:"data"`
	}
	if err := generation1.ReadJSON(&initial); err != nil || initial.Name != "connected" || initial.Data.Generation != 1 || initial.Data.Resumed {
		t.Fatalf("generation-one connected=%+v err=%v", initial, err)
	}
	var firstSnapshot server.AcceleratorSessionSnapshot
	select {
	case firstSnapshot = <-gate.connected:
	case <-time.After(2 * time.Second):
		t.Fatal("generation-one observer did not complete")
	}
	call := firstSnapshot.CallContext
	subscription, err := manager.Subscribe(call, "evidence", false)
	if err != nil {
		t.Fatal(err)
	}
	waitSecretWatchSignal(t, sourceStarted, "single watch source")
	manager.mu.Lock()
	retainedStream := manager.streams[subscription.WatcherSpecID]
	manager.mu.Unlock()
	if retainedStream == nil {
		t.Fatal("subscription did not publish its watch stream")
	}
	var initialStatus server.Event
	if err := generation1.ReadJSON(&initialStatus); err != nil || initialStatus.Name != "watcher-status" {
		t.Fatalf("initial watch status=%+v err=%v", initialStatus, err)
	}
	_ = generation1.Close()
	select {
	case disconnected := <-gate.disconnected:
		if disconnected.Generation != 1 || disconnected.CallContext != call {
			t.Fatalf("generation-one disconnect=%+v", disconnected)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("generation-one disconnect not observed")
	}

	generation2, response, err = dialer.Dial(webSocketURL, header)
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	if err != nil {
		t.Fatal(err)
	}
	waitSecretWatchSignal(t, gate.generation2Ready, "generation-two manager advance")
	secondSnapshot := <-gate.connected
	if secondSnapshot.Generation != 2 || !secondSnapshot.Resumed || secondSnapshot.CallContext != call {
		t.Fatalf("generation-two observer snapshot=%+v", secondSnapshot)
	}
	if lease, found := registry.LookupSessionLease(call.SessionID); !found || lease.Generation != 2 || lease.Connected {
		t.Fatalf("observer-gap lease=%+v found=%v", lease, found)
	}
	manager.mu.Lock()
	managerGeneration := manager.sessionGenerations[call.SessionID]
	membershipGeneration := manager.sessionSpecs[call.SessionID][subscription.WatcherSpecID]
	manager.mu.Unlock()
	if managerGeneration != 2 || membershipGeneration != 2 {
		t.Fatalf("observer-gap manager/member generations=%d/%d", managerGeneration, membershipGeneration)
	}

	active.Action(watch.Modified, &v1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "during-gap", Namespace: "evidence", ResourceVersion: "rv-gap"}})
	gapEvents := collector.waitFor(t, func(events []capturedSecretWatchEvent) bool {
		for _, captured := range events {
			resource, ok := captured.event.Data.(AcceleratorSecretResourceEvent)
			if ok && resource.Resource.Metadata.Name == "during-gap" {
				return true
			}
		}
		return false
	})
	var gapTargets []server.AcceleratorSessionTarget
	for _, captured := range gapEvents {
		resource, ok := captured.event.Data.(AcceleratorSecretResourceEvent)
		if ok && resource.Resource.Metadata.Name == "during-gap" {
			gapTargets = captured.targets
		}
	}
	if fmt.Sprint(gapTargets) != fmt.Sprint([]server.AcceleratorSessionTarget{{SessionID: call.SessionID, Generation: 2}}) {
		t.Fatalf("gap event targets=%+v", gapTargets)
	}
	if lease, found := registry.LookupSessionLease(call.SessionID); !found || lease.Generation != 2 || lease.Connected {
		t.Fatalf("post-gap lease=%+v found=%v", lease, found)
	}

	releaseGate()
	deadline := time.Now().Add(2 * time.Second)
	for {
		lease, found := registry.LookupSessionLease(call.SessionID)
		if found && lease.Generation == 2 && lease.Connected {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("generation-two lease did not become connected: %+v found=%v", lease, found)
		}
		time.Sleep(time.Millisecond)
	}
	_ = generation2.SetReadDeadline(time.Now().Add(2 * time.Second))
	var resumed struct {
		Name string `json:"name"`
		Data struct {
			Generation server.AcceleratorSocketGeneration `json:"generation"`
			Resumed    bool                               `json:"resumed"`
		} `json:"data"`
	}
	if err := generation2.ReadJSON(&resumed); err != nil || resumed.Name != "connected" || resumed.Data.Generation != 2 || !resumed.Data.Resumed {
		t.Fatalf("generation-two first frame=%+v err=%v", resumed, err)
	}

	active.Action(watch.Modified, &v1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "after-ready", Namespace: "evidence", ResourceVersion: "rv-after"}})
	collector.waitFor(t, func(events []capturedSecretWatchEvent) bool {
		for _, captured := range events {
			resource, ok := captured.event.Data.(AcceleratorSecretResourceEvent)
			if ok && resource.Resource.Metadata.Name == "after-ready" {
				return true
			}
		}
		return false
	})
	var delivered struct {
		Name string          `json:"name"`
		Data json.RawMessage `json:"data"`
	}
	if err := generation2.ReadJSON(&delivered); err != nil || delivered.Name != "resource-event" || !strings.Contains(string(delivered.Data), `"name":"after-ready"`) || strings.Contains(string(delivered.Data), "during-gap") {
		t.Fatalf("generation-two delivered=%s/%s err=%v", delivered.Name, delivered.Data, err)
	}
	manager.mu.Lock()
	finalStream := manager.streams[subscription.WatcherSpecID]
	finalSpecs := manager.sessionSpecs[call.SessionID]
	manager.mu.Unlock()
	if sourceCalls.Load() != 1 || finalStream != retainedStream || len(finalSpecs) != 1 || finalSpecs[subscription.WatcherSpecID] != 2 {
		t.Fatalf("reconnect replaced watch state: calls=%d streamSame=%v specs=%v", sourceCalls.Load(), finalStream == retainedStream, finalSpecs)
	}
}

func TestSecretWatchPerSessionAdmissionBound(t *testing.T) {
	call := secretWatchCall("bounded")
	leases := &secretWatchLeaseStore{leases: map[agent.SessionID]server.AcceleratorSessionLease{call.SessionID: {Generation: 1, Connected: true}}}
	var sourceCalls atomic.Int32
	manager := newAcceleratorSecretWatchManager(func(context.Context, string, string, k8s.SecretListOptions) (watch.Interface, error) {
		sourceCalls.Add(1)
		return watch.NewRaceFreeFake(), nil
	}, leases.lookup, func([]server.AcceleratorSessionTarget, server.Event) {})
	manager.logger = func(string) {}
	manager.sleep = func(ctx context.Context, _ time.Duration) error { <-ctx.Done(); return ctx.Err() }
	subscriptions := make([]SecretWatchSubscription, 0, acceleratorSecretWatchMaxSpecsPerSession)
	for index := 0; index < acceleratorSecretWatchMaxSpecsPerSession; index++ {
		subscription, err := manager.Subscribe(call, fmt.Sprintf("nonexistent-%d", index), false)
		if err != nil {
			t.Fatalf("subscription %d rejected below bound: %v", index, err)
		}
		subscriptions = append(subscriptions, subscription)
	}
	manager.mu.Lock()
	if got := len(manager.sessionSpecs[call.SessionID]); got != acceleratorSecretWatchMaxSpecsPerSession {
		manager.mu.Unlock()
		t.Fatalf("memberships at bound=%d, want %d", got, acceleratorSecretWatchMaxSpecsPerSession)
	}
	if got := len(manager.streams); got != acceleratorSecretWatchMaxSpecsPerSession {
		manager.mu.Unlock()
		t.Fatalf("streams at bound=%d, want %d", got, acceleratorSecretWatchMaxSpecsPerSession)
	}
	manager.mu.Unlock()
	if _, err := manager.Subscribe(call, "nonexistent-over-limit", false); !errors.Is(err, errAcceleratorSecretWatchLimit) || err.Error() != "Accelerator Secret watch subscription limit reached" {
		t.Fatalf("over-limit subscribe=%v, want fixed admission error", err)
	}
	if existing, err := manager.Subscribe(call, "nonexistent-0", false); err != nil || existing != subscriptions[0] {
		t.Fatalf("idempotent subscribe at limit=%#v, %v", existing, err)
	}
	manager.mu.Lock()
	if got := len(manager.streams); got != acceleratorSecretWatchMaxSpecsPerSession {
		manager.mu.Unlock()
		t.Fatalf("rejected/idempotent subscribe started stream: %d", got)
	}
	manager.mu.Unlock()
	if err := manager.Unsubscribe(call, subscriptions[0].WatcherSpecID); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Subscribe(call, "replacement-after-unsubscribe", false); err != nil {
		t.Fatalf("unsubscribe did not release admission slot: %v", err)
	}
	manager.SessionRevoked(server.AcceleratorSessionSnapshot{CallContext: call, Generation: 1})
	manager.mu.Lock()
	remainingStreams, remainingMemberships := len(manager.streams), len(manager.sessionSpecs)
	manager.mu.Unlock()
	if remainingStreams != 0 || remainingMemberships != 0 {
		t.Fatalf("revoke did not release bound: streams=%d memberships=%d", remainingStreams, remainingMemberships)
	}
	if _, err := manager.Subscribe(call, "after-revoke", false); err != nil {
		t.Fatalf("revoke did not release admission slot: %v", err)
	}
	if err := manager.ClearAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	manager.mu.Lock()
	remainingStreams, remainingMemberships = len(manager.streams), len(manager.sessionSpecs)
	manager.mu.Unlock()
	if remainingStreams != 0 || remainingMemberships != 0 {
		t.Fatalf("ClearAll retained bounded state: streams=%d memberships=%d", remainingStreams, remainingMemberships)
	}
	if got := sourceCalls.Load(); got > acceleratorSecretWatchMaxSpecsPerSession+2 {
		t.Fatalf("admission rejection started extra workers: source calls=%d", got)
	}
}

func TestSecretWatchEventExactBytesAndTargets(t *testing.T) {
	one, two := secretWatchCall("one"), secretWatchCall("two")
	leases := &secretWatchLeaseStore{leases: map[agent.SessionID]server.AcceleratorSessionLease{
		one.SessionID: {Generation: 4, Connected: true},
		two.SessionID: {Generation: 9, Connected: true},
	}}
	active := watch.NewRaceFreeFake()
	collector := newSecretWatchEventCollector()
	manager := newAcceleratorSecretWatchManager(func(context.Context, string, string, k8s.SecretListOptions) (watch.Interface, error) {
		return active, nil
	}, leases.lookup, collector.emit)
	manager.logger = func(string) {}
	manager.sleep = func(ctx context.Context, _ time.Duration) error { <-ctx.Done(); return ctx.Err() }
	subscription, err := manager.Subscribe(one, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Subscribe(two, "", false); err != nil {
		t.Fatal(err)
	}
	created := metav1.NewTime(time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))
	makeSecret := func(marker string) *v1.Secret {
		return &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name: "same", Namespace: "actual", UID: types.UID("uid"), CreationTimestamp: created, ResourceVersion: "7",
				Labels: map[string]string{"PRIVATE_LABEL_" + marker: marker}, Annotations: map[string]string{"kubectl.kubernetes.io/last-applied-configuration": "PRIVATE_ANNOTATION_" + marker},
				ManagedFields: []metav1.ManagedFieldsEntry{{Manager: "PRIVATE_MANAGER_" + marker}}, Finalizers: []string{"PRIVATE_FINALIZER_" + marker}, OwnerReferences: []metav1.OwnerReference{{Name: "PRIVATE_OWNER_" + marker}},
			},
			Type: v1.SecretTypeOpaque, Data: map[string][]byte{"PRIVATE_KEY_" + marker: []byte("PRIVATE_VALUE_" + marker)}, StringData: map[string]string{"PRIVATE_STRING_KEY_" + marker: "PRIVATE_STRING_VALUE_" + marker},
		}
	}
	for _, eventType := range []watch.EventType{watch.Added, watch.Modified, watch.Deleted} {
		active.Action(eventType, makeSecret("A"))
		active.Action(eventType, makeSecret("B_WITH_RADICALLY_DIFFERENT_BYTES"))
	}
	events := collector.waitFor(t, func(events []capturedSecretWatchEvent) bool {
		count := 0
		for _, event := range events {
			if event.event.Name == "resource-event" {
				count++
			}
		}
		return count == 6
	})
	byType := make(map[string][][]byte)
	for _, captured := range events {
		if captured.event.Name != "resource-event" {
			continue
		}
		targets := append([]server.AcceleratorSessionTarget(nil), captured.targets...)
		sort.Slice(targets, func(i, j int) bool { return targets[i].SessionID < targets[j].SessionID })
		wantTargets := []server.AcceleratorSessionTarget{{SessionID: one.SessionID, Generation: 4}, {SessionID: two.SessionID, Generation: 9}}
		if fmt.Sprint(targets) != fmt.Sprint(wantTargets) {
			t.Fatalf("targets = %#v, want %#v", targets, wantTargets)
		}
		encoded, err := json.Marshal(captured.event)
		if err != nil {
			t.Fatal(err)
		}
		resource := captured.event.Data.(AcceleratorSecretResourceEvent)
		byType[resource.Type] = append(byType[resource.Type], encoded)
		want := fmt.Sprintf(`{"type":"event","name":"resource-event","data":{"type":"%s","resourceType":"secrets","namespace":"actual","watcherSpecId":"%s","resource":{"metadata":{"name":"same","namespace":"actual","uid":"uid","creationTimestamp":"2026-08-01T00:00:00Z"},"type":"Opaque","dataKeys":1}}}`, resource.Type, subscription.WatcherSpecID)
		if string(encoded) != want {
			t.Fatalf("%s bytes = %s\nwant = %s", resource.Type, encoded, want)
		}
		for _, marker := range []string{"PRIVATE_", "last-applied", "managedFields", "ownerReferences", "resourceVersion", "sessionId", "generation"} {
			if strings.Contains(string(encoded), marker) {
				t.Fatalf("event leaked %q: %s", marker, encoded)
			}
		}
	}
	for _, eventType := range []string{"ADDED", "MODIFIED", "DELETED"} {
		pair := byType[eventType]
		if len(pair) != 2 || string(pair[0]) != string(pair[1]) {
			t.Fatalf("%s hostile variants differ: %q", eventType, pair)
		}
	}
	if err := manager.ClearAll(context.Background()); err != nil {
		t.Fatal(err)
	}
}

type scriptedSecretWatchStep struct {
	stream watch.Interface
	err    error
}

func closedSecretWatch(events ...watch.Event) watch.Interface {
	fake := watch.NewRaceFreeFake()
	for _, event := range events {
		fake.Action(event.Type, event.Object)
	}
	fake.Stop()
	return fake
}

func TestSecretWatchResourceVersionAndRestart(t *testing.T) {
	markerError := errors.New("PRIVATE_KUBERNETES_ERROR")
	secret20 := &v1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "safe", Namespace: "ns", ResourceVersion: "20"}}
	steps := []scriptedSecretWatchStep{
		{err: markerError},
		{err: markerError},
		{stream: closedSecretWatch(
			watch.Event{Type: watch.Added, Object: &v1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "safe", Namespace: "ns", ResourceVersion: "10"}}},
			watch.Event{Type: watch.Bookmark, Object: &metav1.PartialObjectMetadata{ObjectMeta: metav1.ObjectMeta{ResourceVersion: "11"}}},
		)},
		{stream: closedSecretWatch(watch.Event{Type: watch.Error, Object: &metav1.Status{Reason: metav1.StatusReasonExpired, Code: 410, Message: "PRIVATE_EXPIRED_DETAIL"}})},
		{stream: closedSecretWatch(watch.Event{Type: watch.Error, Object: &metav1.Status{Reason: metav1.StatusReasonGone, Message: "PRIVATE_GONE_DETAIL"}})},
		{stream: closedSecretWatch(
			watch.Event{Type: watch.Added, Object: secret20},
			watch.Event{Type: watch.Error, Object: &metav1.Status{Reason: metav1.StatusReasonInternalError, Message: "PRIVATE_INTERNAL_DETAIL"}},
		)},
		{stream: closedSecretWatch(watch.Event{Type: watch.Added, Object: &v1.Pod{ObjectMeta: metav1.ObjectMeta{ResourceVersion: "PRIVATE_BAD_RV"}}})},
		{stream: closedSecretWatch(watch.Event{Type: watch.EventType("MYSTERY"), Object: &v1.Secret{ObjectMeta: metav1.ObjectMeta{ResourceVersion: "PRIVATE_UNKNOWN_RV"}}})},
		{err: markerError},
	}
	call := secretWatchCall("retry")
	leases := &secretWatchLeaseStore{leases: map[agent.SessionID]server.AcceleratorSessionLease{call.SessionID: {Generation: 1, Connected: true}}}
	collector := newSecretWatchEventCollector()
	var mu sync.Mutex
	var resourceVersions []string
	var sleeps []time.Duration
	var logs []string
	step := 0
	manager := newAcceleratorSecretWatchManager(func(_ context.Context, _ string, rv string, _ k8s.SecretListOptions) (watch.Interface, error) {
		mu.Lock()
		defer mu.Unlock()
		resourceVersions = append(resourceVersions, rv)
		if step >= len(steps) {
			return nil, errors.New("unexpected source call")
		}
		result := steps[step]
		step++
		return result.stream, result.err
	}, leases.lookup, collector.emit)
	stopRetry := errors.New("stop retry")
	manager.sleep = func(_ context.Context, delay time.Duration) error {
		mu.Lock()
		defer mu.Unlock()
		sleeps = append(sleeps, delay)
		if len(sleeps) == len(steps) {
			return stopRetry
		}
		return nil
	}
	manager.logger = func(message string) { mu.Lock(); logs = append(logs, message); mu.Unlock() }
	subscription, err := manager.Subscribe(call, "ns", false)
	if err != nil {
		t.Fatal(err)
	}
	manager.mu.Lock()
	stream := manager.streams[subscription.WatcherSpecID]
	manager.mu.Unlock()
	waitSecretWatchSignal(t, stream.done, "scripted retry completion")

	mu.Lock()
	gotRVs := append([]string(nil), resourceVersions...)
	gotSleeps := append([]time.Duration(nil), sleeps...)
	gotLogs := append([]string(nil), logs...)
	mu.Unlock()
	wantRVs := []string{"", "", "", "11", "", "", "20", "20", "20"}
	if fmt.Sprint(gotRVs) != fmt.Sprint(wantRVs) {
		t.Fatalf("source RVs = %q, want %q", gotRVs, wantRVs)
	}
	wantSleeps := []time.Duration{time.Second, 2 * time.Second, time.Second, time.Second, time.Second, time.Second, time.Second, time.Second, 2 * time.Second}
	if fmt.Sprint(gotSleeps) != fmt.Sprint(wantSleeps) {
		t.Fatalf("retry sleeps = %v, want %v", gotSleeps, wantSleeps)
	}
	if secretWatchRetryDelay(1) != time.Second || secretWatchRetryDelay(2) != 2*time.Second || secretWatchRetryDelay(8) != 2*time.Minute || secretWatchRetryDelay(100) != 2*time.Minute {
		t.Fatalf("bounded retry sequence changed: 1=%v 2=%v 8=%v 100=%v", secretWatchRetryDelay(1), secretWatchRetryDelay(2), secretWatchRetryDelay(8), secretWatchRetryDelay(100))
	}

	events := collector.snapshot()
	statuses := map[string]int{}
	errorsByCode := map[string]int{}
	for _, captured := range events {
		encoded, err := json.Marshal(captured.event)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), "PRIVATE_") {
			t.Fatalf("diagnostic leaked source data: %s", encoded)
		}
		switch data := captured.event.Data.(type) {
		case AcceleratorSecretWatcherStatus:
			statuses[data.Status]++
			if data.Status != secretWatcherConnected && data.Status != secretWatcherReconnecting {
				t.Fatalf("unclosed status %q", data.Status)
			}
		case AcceleratorSecretWatcherError:
			errorsByCode[data.Code]++
			if !data.Recoverable {
				t.Fatalf("non-recoverable retry error: %#v", data)
			}
		}
	}
	if statuses[secretWatcherConnected] != 6 || statuses[secretWatcherReconnecting] != 9 {
		t.Fatalf("statuses = %#v", statuses)
	}
	if errorsByCode[secretWatchUnavailable] != 5 || errorsByCode[secretWatchRVExpired] != 2 || errorsByCode[secretWatchMalformed] != 2 {
		t.Fatalf("error codes = %#v", errorsByCode)
	}
	for _, logMessage := range gotLogs {
		if strings.Contains(logMessage, "PRIVATE_") {
			t.Fatalf("log leaked source error: %q", logMessage)
		}
	}

	blockingStarted := make(chan struct{})
	cancelCollector := newSecretWatchEventCollector()
	cancelManager := newAcceleratorSecretWatchManager(func(ctx context.Context, _ string, _ string, _ k8s.SecretListOptions) (watch.Interface, error) {
		close(blockingStarted)
		<-ctx.Done()
		return nil, ctx.Err()
	}, leases.lookup, cancelCollector.emit)
	cancelManager.logger = func(message string) { t.Errorf("cancellation logged %q", message) }
	if _, err := cancelManager.Subscribe(call, "cancel", false); err != nil {
		t.Fatal(err)
	}
	waitSecretWatchSignal(t, blockingStarted, "blocking source")
	if err := cancelManager.ClearAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := cancelCollector.snapshot(); len(got) != 0 {
		t.Fatalf("cancellation emitted diagnostics: %#v", got)
	}
	if err := manager.ClearAll(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestSecretWatchStartupResourceVersionRejection(t *testing.T) {
	tests := []struct {
		name          string
		startupError  error
		wantRetryRV   string
		wantErrorCode string
	}{
		{name: "expired", startupError: apierrors.NewResourceExpired("PRIVATE_EXPIRED_STARTUP_DETAIL"), wantRetryRV: "", wantErrorCode: secretWatchRVExpired},
		{name: "gone", startupError: apierrors.NewGone("PRIVATE_GONE_STARTUP_DETAIL"), wantRetryRV: "", wantErrorCode: secretWatchRVExpired},
		{name: "ordinary", startupError: errors.New("PRIVATE_ORDINARY_STARTUP_DETAIL"), wantRetryRV: "rejected-rv", wantErrorCode: secretWatchUnavailable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			call := secretWatchCall("startup-" + test.name)
			leases := &secretWatchLeaseStore{leases: map[agent.SessionID]server.AcceleratorSessionLease{call.SessionID: {Generation: 1, Connected: true}}}
			collector := newSecretWatchEventCollector()
			var mu sync.Mutex
			var resourceVersions []string
			var sleeps []time.Duration
			var logs []string
			calls := 0
			manager := newAcceleratorSecretWatchManager(func(_ context.Context, _ string, resourceVersion string, _ k8s.SecretListOptions) (watch.Interface, error) {
				mu.Lock()
				defer mu.Unlock()
				resourceVersions = append(resourceVersions, resourceVersion)
				calls++
				switch calls {
				case 1:
					return closedSecretWatch(watch.Event{Type: watch.Added, Object: &v1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "safe", Namespace: "ns", ResourceVersion: "rejected-rv"}}}), nil
				case 2:
					return nil, test.startupError
				default:
					return nil, errors.New("PRIVATE_FINAL_STARTUP_DETAIL")
				}
			}, leases.lookup, collector.emit)
			manager.sleep = func(_ context.Context, delay time.Duration) error {
				mu.Lock()
				defer mu.Unlock()
				sleeps = append(sleeps, delay)
				if len(sleeps) == 3 {
					return errors.New("stop retry")
				}
				return nil
			}
			manager.logger = func(message string) {
				mu.Lock()
				logs = append(logs, message)
				mu.Unlock()
			}
			subscription, err := manager.Subscribe(call, "ns", false)
			if err != nil {
				t.Fatal(err)
			}
			manager.mu.Lock()
			stream := manager.streams[subscription.WatcherSpecID]
			manager.mu.Unlock()
			waitSecretWatchSignal(t, stream.done, "startup rejection retry completion")

			mu.Lock()
			gotRVs := append([]string(nil), resourceVersions...)
			gotSleeps := append([]time.Duration(nil), sleeps...)
			gotLogs := append([]string(nil), logs...)
			mu.Unlock()
			wantRVs := []string{"", "rejected-rv", test.wantRetryRV}
			if fmt.Sprint(gotRVs) != fmt.Sprint(wantRVs) {
				t.Fatalf("startup retry RVs = %q, want %q", gotRVs, wantRVs)
			}
			wantSleeps := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}
			if fmt.Sprint(gotSleeps) != fmt.Sprint(wantSleeps) {
				t.Fatalf("startup retry sleeps = %v, want %v", gotSleeps, wantSleeps)
			}
			var statuses []string
			var codes []string
			for _, captured := range collector.snapshot() {
				encoded, err := json.Marshal(captured.event)
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(string(encoded), "PRIVATE_") {
					t.Fatalf("startup rejection diagnostic leaked details: %s", encoded)
				}
				switch data := captured.event.Data.(type) {
				case AcceleratorSecretWatcherStatus:
					statuses = append(statuses, data.Status)
				case AcceleratorSecretWatcherError:
					codes = append(codes, data.Code)
				}
			}
			wantStatuses := []string{secretWatcherConnected, secretWatcherReconnecting, secretWatcherReconnecting, secretWatcherReconnecting}
			if fmt.Sprint(statuses) != fmt.Sprint(wantStatuses) {
				t.Fatalf("startup statuses = %q, want %q", statuses, wantStatuses)
			}
			wantCodes := []string{secretWatchUnavailable, test.wantErrorCode, secretWatchUnavailable}
			if fmt.Sprint(codes) != fmt.Sprint(wantCodes) {
				t.Fatalf("startup error codes = %q, want %q", codes, wantCodes)
			}
			for _, message := range gotLogs {
				if strings.Contains(message, "PRIVATE_") {
					t.Fatalf("startup rejection log leaked details: %q", message)
				}
			}
			if len(gotLogs) != 3 {
				t.Fatalf("startup log count = %d, want 3: %q", len(gotLogs), gotLogs)
			}
			if test.wantErrorCode == secretWatchRVExpired && gotLogs[1] != "Accelerator Secret watch resource version expired" {
				t.Fatalf("startup rejection log = %q", gotLogs)
			}
			if err := manager.ClearAll(context.Background()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSecretWatchRestartLoggingRequiresPublishedEmission(t *testing.T) {
	call := secretWatchCall("restart-log")
	ctx, cancel := context.WithCancel(context.Background())
	stream := &secretWatchStream{spec: secretWatchSpec{namespace: "safe"}, id: (secretWatchSpec{namespace: "safe"}).id(), ctx: ctx, cancel: cancel, done: make(chan struct{})}
	var emitted atomic.Int32
	var logged atomic.Int32
	manager := newAcceleratorSecretWatchManager(nil, nil, func([]server.AcceleratorSessionTarget, server.Event) { emitted.Add(1) })
	manager.streams[stream.id] = stream
	manager.sessionSpecs[call.SessionID] = map[SecretWatchSpecID]server.AcceleratorSocketGeneration{stream.id: 1}
	manager.logger = func(string) {
		// Logger re-entry proves the manager mutex was released before the external
		// logging seam was invoked.
		manager.mu.Lock()
		manager.mu.Unlock()
		logged.Add(1)
	}
	restartDone := make(chan struct{})
	go func() {
		manager.emitRestart(stream, secretWatchUnavailable)
		close(restartDone)
	}()
	waitSecretWatchSignal(t, restartDone, "published restart log")
	if emitted.Load() != 2 || logged.Load() != 1 {
		t.Fatalf("published restart emitted/logged=%d/%d, want 2/1", emitted.Load(), logged.Load())
	}

	manager.mu.Lock()
	teardownStarted := make(chan struct{})
	teardownDone := make(chan struct{})
	go func() {
		close(teardownStarted)
		manager.emitRestart(stream, secretWatchUnavailable)
		close(teardownDone)
	}()
	waitSecretWatchSignal(t, teardownStarted, "teardown restart attempt")
	delete(manager.streams, stream.id)
	delete(manager.sessionSpecs, call.SessionID)
	cancel()
	manager.mu.Unlock()
	waitSecretWatchSignal(t, teardownDone, "teardown restart completion")
	if emitted.Load() != 2 || logged.Load() != 1 {
		t.Fatalf("teardown restart emitted/logged=%d/%d, want unchanged 2/1", emitted.Load(), logged.Load())
	}
}

func TestSecretWatchGenerationAndGrace(t *testing.T) {
	call := secretWatchCall("generation")
	var leaseMu sync.Mutex
	lease := server.AcceleratorSessionLease{Generation: 7, Connected: true}
	lookupCount := 0
	captured := make(chan struct{})
	lookup := func(agent.SessionID) (server.AcceleratorSessionLease, bool) {
		leaseMu.Lock()
		lookupCount++
		count := lookupCount
		current := lease
		leaseMu.Unlock()
		if count == 3 {
			close(captured)
		}
		return current, true
	}
	active := newBlockingSecretWatch()
	watchStarted := make(chan struct{})
	manager := newAcceleratorSecretWatchManager(func(context.Context, string, string, k8s.SecretListOptions) (watch.Interface, error) {
		close(watchStarted)
		return active, nil
	}, lookup, func([]server.AcceleratorSessionTarget, server.Event) {})
	manager.sleep = func(ctx context.Context, _ time.Duration) error { <-ctx.Done(); return ctx.Err() }
	subscription, err := manager.Subscribe(call, "ns", false)
	if err != nil {
		t.Fatal(err)
	}
	waitSecretWatchSignal(t, watchStarted, "generation watch start")
	manager.SessionDisconnected(server.AcceleratorSessionSnapshot{CallContext: call, Generation: 7})
	// Hold the manager barrier so Unsubscribe completes its first lease lookup at
	// N, then cannot recheck or mutate until both registry and manager state have
	// atomically advanced to N+1.
	manager.mu.Lock()
	unsubscribed := make(chan error, 1)
	go func() { unsubscribed <- manager.Unsubscribe(call, subscription.WatcherSpecID) }()
	waitSecretWatchSignal(t, captured, "unsubscribe generation capture")
	leaseMu.Lock()
	lease = server.AcceleratorSessionLease{Generation: 8, Connected: true}
	leaseMu.Unlock()
	manager.sessionGenerations[call.SessionID] = 8
	manager.sessionSpecs[call.SessionID][subscription.WatcherSpecID] = 8
	manager.mu.Unlock()
	if err := <-unsubscribed; err != nil {
		t.Fatal(err)
	}
	manager.SessionConnected(server.AcceleratorSessionSnapshot{CallContext: call, Generation: 8, Resumed: true})
	manager.SessionConnected(server.AcceleratorSessionSnapshot{CallContext: call, Generation: 7})
	manager.SessionDisconnected(server.AcceleratorSessionSnapshot{CallContext: call, Generation: 7})
	manager.SessionRevoked(server.AcceleratorSessionSnapshot{CallContext: call, Generation: 7})
	manager.mu.Lock()
	generation, member := manager.sessionSpecs[call.SessionID][subscription.WatcherSpecID]
	_, streamCurrent := manager.streams[subscription.WatcherSpecID]
	manager.mu.Unlock()
	if !member || generation != 8 || !streamCurrent {
		t.Fatalf("N+1 membership lost after stale work: member=%v generation=%d streamCurrent=%v", member, generation, streamCurrent)
	}
	if resumed, err := manager.Subscribe(call, "ns", false); err != nil || resumed != subscription {
		t.Fatalf("resubscribe = %#v, %v; want %#v", resumed, err, subscription)
	}
	revoked := make(chan struct{})
	go func() {
		manager.SessionRevoked(server.AcceleratorSessionSnapshot{CallContext: call, Generation: 8})
		close(revoked)
	}()
	waitSecretWatchSignal(t, active.stopStarted, "current-generation revoke stop")
	close(active.allowStop)
	waitSecretWatchSignal(t, revoked, "current-generation revoke completion")
	manager.SessionRevoked(server.AcceleratorSessionSnapshot{CallContext: call, Generation: 8})
	manager.SessionRevoked(server.AcceleratorSessionSnapshot{CallContext: call, Generation: 7})
	manager.mu.Lock()
	remainingStreams, remainingSessions := len(manager.streams), len(manager.sessionSpecs)
	manager.mu.Unlock()
	if remainingStreams != 0 || remainingSessions != 0 {
		t.Fatalf("repeated/stale revoke recreated state: streams=%d sessions=%d", remainingStreams, remainingSessions)
	}
}

func TestSecretWatchRevocationAndClearAll(t *testing.T) {
	one, two, three := secretWatchCall("one"), secretWatchCall("two"), secretWatchCall("three")
	leases := &secretWatchLeaseStore{leases: map[agent.SessionID]server.AcceleratorSessionLease{
		one.SessionID: {Generation: 1, Connected: true}, two.SessionID: {Generation: 2, Connected: true}, three.SessionID: {Generation: 3, Connected: true},
	}}
	var sourceMu sync.Mutex
	watches := make(map[string]*blockingSecretWatch)
	sourceStarted := make(chan string, 3)
	manager := newAcceleratorSecretWatchManager(func(_ context.Context, namespace, _ string, _ k8s.SecretListOptions) (watch.Interface, error) {
		watcher := newBlockingSecretWatch()
		sourceMu.Lock()
		watches[namespace] = watcher
		sourceMu.Unlock()
		sourceStarted <- namespace
		return watcher, nil
	}, leases.lookup, func([]server.AcceleratorSessionTarget, server.Event) {})
	manager.sleep = func(ctx context.Context, _ time.Duration) error { <-ctx.Done(); return ctx.Err() }
	shared, err := manager.Subscribe(one, "shared", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Subscribe(two, "shared", false); err != nil {
		t.Fatal(err)
	}
	distinct, err := manager.Subscribe(two, "distinct", false)
	if err != nil {
		t.Fatal(err)
	}
	third, err := manager.Subscribe(three, "third", false)
	if err != nil {
		t.Fatal(err)
	}
	if shared.WatcherSpecID == distinct.WatcherSpecID || distinct.WatcherSpecID == third.WatcherSpecID {
		t.Fatal("distinct specs shared an ID")
	}
	started := make(map[string]bool)
	for len(started) != 3 {
		select {
		case namespace := <-sourceStarted:
			started[namespace] = true
		case <-time.After(2 * time.Second):
			t.Fatalf("streams did not start: %#v", started)
		}
	}

	manager.SessionRevoked(server.AcceleratorSessionSnapshot{CallContext: one, Generation: 0})
	revokeDone := make(chan struct{})
	go func() {
		manager.SessionRevoked(server.AcceleratorSessionSnapshot{CallContext: two, Generation: 2})
		close(revokeDone)
	}()
	sourceMu.Lock()
	distinctWatch := watches["distinct"]
	sourceMu.Unlock()
	waitSecretWatchSignal(t, distinctWatch.stopStarted, "selective revoke stream stop")
	select {
	case <-revokeDone:
		t.Fatal("revocation returned before the newly unreferenced stream stopped")
	default:
	}
	close(distinctWatch.allowStop)
	waitSecretWatchSignal(t, revokeDone, "selective revoke completion")
	manager.mu.Lock()
	_, sharedStillPresent := manager.streams[shared.WatcherSpecID]
	_, firstMembership := manager.sessionSpecs[one.SessionID]
	_, secondMembership := manager.sessionSpecs[two.SessionID]
	_, distinctStillPresent := manager.streams[distinct.WatcherSpecID]
	manager.mu.Unlock()
	if !sharedStillPresent || !firstMembership || secondMembership || distinctStillPresent {
		t.Fatalf("selective/stale revoke state: shared=%v first=%v second=%v distinct=%v", sharedStillPresent, firstMembership, secondMembership, distinctStillPresent)
	}

	manager.mu.Lock()
	sharedStream := manager.streams[shared.WatcherSpecID]
	manager.mu.Unlock()
	sourceMu.Lock()
	watchSnapshot := []*blockingSecretWatch{watches["shared"], watches["third"]}
	sourceMu.Unlock()

	// Race an owned unsubscribe and an in-flight targeted emission against the
	// terminal clear. Regardless of lock acquisition order, ClearAll owns one
	// shared completion and both streams are cancelled exactly once.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	clearResult := make(chan error, 1)
	unsubscribeResult := make(chan error, 1)
	emitDone := make(chan struct{})
	go func() { clearResult <- manager.ClearAll(ctx) }()
	go func() { unsubscribeResult <- manager.Unsubscribe(one, shared.WatcherSpecID) }()
	go func() {
		manager.emitFor(sharedStream, "watcher-status", AcceleratorSecretWatcherStatus{WatcherSpecID: shared.WatcherSpecID, Status: secretWatcherConnected})
		close(emitDone)
	}()
	if err := <-clearResult; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled ClearAll = %v, want context canceled", err)
	}
	waitSecretWatchSignal(t, emitDone, "concurrent terminal emission")
	manager.mu.Lock()
	if !manager.closed || len(manager.streams) != 0 || len(manager.sessionSpecs) != 0 || len(manager.sessionGenerations) != 0 {
		t.Fatalf("ClearAll did not close admission/maps before waits: closed=%v streams=%d specs=%d generations=%d", manager.closed, len(manager.streams), len(manager.sessionSpecs), len(manager.sessionGenerations))
	}
	manager.mu.Unlock()
	if _, err := manager.Subscribe(one, "late", false); err == nil {
		t.Fatal("terminal ClearAll admitted a new subscription")
	}

	for _, watcher := range watchSnapshot {
		waitSecretWatchSignal(t, watcher.stopStarted, "ClearAll stream stop")
	}
	laterDone := make(chan error, 1)
	go func() { laterDone <- manager.ClearAll(context.Background()) }()
	select {
	case err := <-laterDone:
		t.Fatalf("later ClearAll returned before shared completion: %v", err)
	default:
	}
	for _, watcher := range watchSnapshot {
		close(watcher.allowStop)
	}
	if err := <-unsubscribeResult; err != nil {
		t.Fatalf("concurrent unsubscribe = %v", err)
	}
	if err := <-laterDone; err != nil {
		t.Fatal(err)
	}
	if err := manager.ClearAll(context.Background()); err != nil {
		t.Fatalf("completed ClearAll was not idempotent: %v", err)
	}
}
