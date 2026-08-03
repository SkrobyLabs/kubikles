//go:build !accelerator

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"kubikles/pkg/acceleratorprovision"
	"kubikles/pkg/acceleratorsecret"
	"kubikles/pkg/k8s"
)

type fakeSecretReason acceleratorsecret.SecretClientReason

func (e fakeSecretReason) Error() string { return string(e) }
func (e fakeSecretReason) SecretClientReason() acceleratorsecret.SecretClientReason {
	return acceleratorsecret.SecretClientReason(e)
}

type fakeRouterClient struct {
	done             chan struct{}
	doneObserved     chan struct{}
	closeOnce        sync.Once
	doneObservedOnce sync.Once
	doneCalls        atomic.Int32
	reasonMu         sync.Mutex
	reason           acceleratorsecret.SecretClientReason
	data             func(context.Context, string, string) ([]k8s.DataEntry, error)
	yaml             func(context.Context, string, string) (string, error)
	list             func(context.Context, string, string, bool) ([]k8s.SecretListItem, error)
	cancel           func(context.Context, string) (bool, error)
	subscribe        func(context.Context, string, bool) (acceleratorsecret.SecretWatchSubscription, acceleratorprovision.SecretWatchLease, error)
	unsub            func(context.Context, acceleratorsecret.SecretWatchSpecID) error
	onClose          func()
	onCloseOnce      sync.Once
}

func newFakeRouterClient() *fakeRouterClient { return &fakeRouterClient{done: make(chan struct{})} }
func (c *fakeRouterClient) ListSecretsMetadata(ctx context.Context, id, namespace string, exclude bool) ([]k8s.SecretListItem, error) {
	if c.list != nil {
		return c.list(ctx, id, namespace, exclude)
	}
	return []k8s.SecretListItem{{DataKeys: 1}}, nil
}
func (c *fakeRouterClient) GetSecretData(ctx context.Context, namespace, name string) ([]k8s.DataEntry, error) {
	if c.data != nil {
		return c.data(ctx, namespace, name)
	}
	return []k8s.DataEntry{{Key: "remote"}}, nil
}
func (c *fakeRouterClient) GetSecretYaml(ctx context.Context, namespace, name string) (string, error) {
	if c.yaml != nil {
		return c.yaml(ctx, namespace, name)
	}
	return "remote", nil
}
func (c *fakeRouterClient) CancelListRequest(ctx context.Context, id string) (bool, error) {
	if c.cancel != nil {
		return c.cancel(ctx, id)
	}
	return true, nil
}
func (c *fakeRouterClient) SubscribeSecretWatcher(ctx context.Context, namespace string, exclude bool) (acceleratorsecret.SecretWatchSubscription, acceleratorprovision.SecretWatchLease, error) {
	if c.subscribe != nil {
		return c.subscribe(ctx, namespace, exclude)
	}
	return acceleratorsecret.SecretWatchSubscription{}, nil, fakeSecretReason(acceleratorsecret.ReasonRemoteUnavailable)
}
func (c *fakeRouterClient) UnsubscribeSecretWatcher(ctx context.Context, id acceleratorsecret.SecretWatchSpecID) error {
	if c.unsub != nil {
		return c.unsub(ctx, id)
	}
	return nil
}
func (c *fakeRouterClient) Done() <-chan struct{} {
	c.doneCalls.Add(1)
	if c.doneObserved != nil {
		c.doneObservedOnce.Do(func() { close(c.doneObserved) })
	}
	return c.done
}
func (c *fakeRouterClient) Reason() acceleratorsecret.SecretClientReason {
	c.reasonMu.Lock()
	defer c.reasonMu.Unlock()
	return c.reason
}
func (c *fakeRouterClient) Close(context.Context) {
	if c.onClose != nil {
		c.onCloseOnce.Do(c.onClose)
	}
	c.terminate(acceleratorsecret.ReasonClosed)
}
func (c *fakeRouterClient) terminate(reason acceleratorsecret.SecretClientReason) {
	c.closeOnce.Do(func() {
		c.reasonMu.Lock()
		c.reason = reason
		c.reasonMu.Unlock()
		close(c.done)
	})
}

type fakeRouterSession struct {
	identity secretRouterSessionIdentity
	current  atomic.Bool
	client   acceleratorprovision.SecretRPCClient
	created  atomic.Int32
	closed   atomic.Int32
}

func newFakeRouterSession(generation int, client acceleratorprovision.SecretRPCClient) *fakeRouterSession {
	s := &fakeRouterSession{identity: secretRouterSessionIdentity{session: new(acceleratorprovision.ConnectedSession), generation: generation}, client: client}
	s.current.Store(true)
	return s
}
func (s *fakeRouterSession) Identity() secretRouterSessionIdentity { return s.identity }
func (s *fakeRouterSession) Current() bool                         { return s.current.Load() }
func (s *fakeRouterSession) NewClient() (acceleratorprovision.SecretRPCClient, error) {
	s.created.Add(1)
	return s.client, nil
}
func (s *fakeRouterSession) Close() { s.closed.Add(1); s.current.Store(false) }

type invalidatingRouterSession struct {
	identity      secretRouterSessionIdentity
	current       atomic.Bool
	client        acceleratorprovision.SecretRPCClient
	identityCalls atomic.Int32
	created       atomic.Int32
	closed        atomic.Int32
}

func newInvalidatingRouterSession(generation int, client acceleratorprovision.SecretRPCClient) *invalidatingRouterSession {
	session := &invalidatingRouterSession{
		identity: secretRouterSessionIdentity{session: new(acceleratorprovision.ConnectedSession), generation: generation},
		client:   client,
	}
	session.current.Store(true)
	return session
}

func (s *invalidatingRouterSession) Identity() secretRouterSessionIdentity {
	s.identityCalls.Add(1)
	if !s.current.Load() {
		return secretRouterSessionIdentity{}
	}
	return s.identity
}
func (s *invalidatingRouterSession) Current() bool { return s.current.Load() }
func (s *invalidatingRouterSession) NewClient() (acceleratorprovision.SecretRPCClient, error) {
	s.created.Add(1)
	return s.client, nil
}
func (s *invalidatingRouterSession) Close() {
	s.closed.Add(1)
	s.current.Store(false)
}
func (s *invalidatingRouterSession) invalidate() { s.current.Store(false) }

type fakeRouterDemand struct {
	mu      sync.Mutex
	change  chan struct{}
	session secretRouterSessionLease
	closed  atomic.Bool
}

type invalidChangesDemand struct {
	snapshotCalls atomic.Int32
	closed        atomic.Int32
	changes       <-chan struct{}
	valid         bool
}

func (d *invalidChangesDemand) Changes() <-chan struct{} {
	return d.changes
}
func (d *invalidChangesDemand) CurrentChanges() (<-chan struct{}, bool) {
	d.snapshotCalls.Add(1)
	return d.changes, d.valid
}
func (*invalidChangesDemand) TrySession() (secretRouterSessionLease, bool) { return nil, false }
func (d *invalidChangesDemand) Close()                                     { d.closed.Add(1) }

type rotatingChangesDemand struct {
	mu             sync.Mutex
	current        chan struct{}
	second         chan struct{}
	third          chan struct{}
	session        secretRouterSessionLease
	snapshotCalls  atomic.Int32
	initialFetched chan struct{}
	initialOnce    sync.Once
	closed         atomic.Bool
}

func newRotatingChangesDemand(session secretRouterSessionLease) *rotatingChangesDemand {
	return &rotatingChangesDemand{
		current:        make(chan struct{}),
		second:         make(chan struct{}),
		third:          make(chan struct{}),
		session:        session,
		initialFetched: make(chan struct{}),
	}
}

func (d *rotatingChangesDemand) Changes() <-chan struct{} {
	changes, _ := d.CurrentChanges()
	return changes
}
func (d *rotatingChangesDemand) CurrentChanges() (<-chan struct{}, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	call := d.snapshotCalls.Add(1)
	if d.closed.Load() {
		return nil, false
	}
	if call == 1 {
		d.initialOnce.Do(func() { close(d.initialFetched) })
	}
	if call == 2 {
		second := d.current
		close(second)
		d.current = d.third
		return second, true
	}
	return d.current, true
}
func (d *rotatingChangesDemand) TrySession() (secretRouterSessionLease, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.session, d.snapshotCalls.Load() >= 3 && d.session != nil && d.session.Current()
}
func (d *rotatingChangesDemand) Close() { d.closed.Store(true) }
func (d *rotatingChangesDemand) rotateToSecond() {
	d.mu.Lock()
	close(d.current)
	d.current = d.second
	d.mu.Unlock()
}

type clientFirstCleanupWatch struct {
	subscription acceleratorsecret.SecretWatchSubscription
	events       chan acceleratorprovision.SecretWatchEvent
	done         chan struct{}
	clientClosed <-chan struct{}
	closed       atomic.Int32
}

func (w *clientFirstCleanupWatch) Subscription() acceleratorsecret.SecretWatchSubscription {
	return w.subscription
}
func (w *clientFirstCleanupWatch) Events() <-chan acceleratorprovision.SecretWatchEvent {
	return w.events
}
func (w *clientFirstCleanupWatch) Done() <-chan struct{} { return w.done }
func (w *clientFirstCleanupWatch) Reason() acceleratorsecret.SecretClientReason {
	return acceleratorsecret.ReasonClosed
}
func (w *clientFirstCleanupWatch) Close(context.Context) {
	<-w.clientClosed
	w.closed.Add(1)
}

type fakeRouterWatch struct {
	subscription acceleratorsecret.SecretWatchSubscription
	events       chan acceleratorprovision.SecretWatchEvent
	done         chan struct{}
	closeOnce    sync.Once
	closed       atomic.Int32
	reason       acceleratorsecret.SecretClientReason
}

func newFakeRouterWatch(id acceleratorsecret.SecretWatchSpecID) *fakeRouterWatch {
	return &fakeRouterWatch{subscription: acceleratorsecret.SecretWatchSubscription{WatcherSpecID: id}, events: make(chan acceleratorprovision.SecretWatchEvent), done: make(chan struct{})}
}
func (w *fakeRouterWatch) Subscription() acceleratorsecret.SecretWatchSubscription {
	return w.subscription
}
func (w *fakeRouterWatch) Events() <-chan acceleratorprovision.SecretWatchEvent { return w.events }
func (w *fakeRouterWatch) Done() <-chan struct{}                                { return w.done }
func (w *fakeRouterWatch) Reason() acceleratorsecret.SecretClientReason         { return w.reason }
func (w *fakeRouterWatch) Close(context.Context) {
	w.closed.Add(1)
	w.terminate(acceleratorsecret.ReasonClosed)
}
func (w *fakeRouterWatch) terminate(reason acceleratorsecret.SecretClientReason) {
	w.reason = reason
	w.closeOnce.Do(func() {
		close(w.events)
		close(w.done)
	})
}

func newFakeRouterDemand(session secretRouterSessionLease) *fakeRouterDemand {
	return &fakeRouterDemand{change: make(chan struct{}), session: session}
}
func (d *fakeRouterDemand) Changes() <-chan struct{} {
	changes, _ := d.CurrentChanges()
	return changes
}
func (d *fakeRouterDemand) CurrentChanges() (<-chan struct{}, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed.Load() || d.change == nil {
		return nil, false
	}
	return d.change, true
}
func (d *fakeRouterDemand) TrySession() (secretRouterSessionLease, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.session, d.session != nil && d.session.Current()
}
func (d *fakeRouterDemand) Close() { d.closed.Store(true); d.signal(nil) }
func (d *fakeRouterDemand) signal(session secretRouterSessionLease) {
	d.mu.Lock()
	d.session = session
	close(d.change)
	d.change = make(chan struct{})
	d.mu.Unlock()
}
func (d *fakeRouterDemand) replaceSilently(session secretRouterSessionLease) {
	d.mu.Lock()
	d.session = session
	d.mu.Unlock()
}

func waitRouterSignal[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for router signal")
		var zero T
		return zero
	}
}

func routerFixture(t *testing.T, demand *fakeRouterDemand) (*integratedSecretRouter, <-chan integratedSecretSourceSignal, <-chan integratedSecretSourceSignal) {
	return routerFixtureWith(t, demand, nil)
}

func routerFixtureWith(t *testing.T, demand *fakeRouterDemand, configure func(*integratedSecretRouterDependencies)) (*integratedSecretRouter, <-chan integratedSecretSourceSignal, <-chan integratedSecretSourceSignal) {
	t.Helper()
	ready := make(chan integratedSecretSourceSignal, 4)
	unavailable := make(chan integratedSecretSourceSignal, 4)
	dependencies := integratedSecretRouterDependencies{
		acquire: func(context.Context, string) (secretRouterDemandLease, bool) { return demand, true },
		directList: func(context.Context, string, string, bool) ([]k8s.SecretListItem, error) {
			return []k8s.SecretListItem{{DataKeys: 2}}, nil
		},
		directData:  func(string, string) ([]k8s.DataEntry, error) { return []k8s.DataEntry{{Key: "direct"}}, nil },
		directYAML:  func(string, string) (string, error) { return "direct", nil },
		ready:       func(signal integratedSecretSourceSignal) { ready <- signal },
		unavailable: func(signal integratedSecretSourceSignal) { unavailable <- signal },
		entropy:     bytes.NewReader(make([]byte, 16)),
	}
	if configure != nil {
		configure(&dependencies)
	}
	router, err := newIntegratedSecretRouter(dependencies)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { router.Close(context.Background()) })
	return router, ready, unavailable
}

func TestIntegratedSecretRouterDemandToVerifiedSource(t *testing.T) {
	firstClient := newFakeRouterClient()
	first := newFakeRouterSession(1, firstClient)
	demand := newFakeRouterDemand(first)
	router, ready, unavailable := routerFixture(t, demand)
	router.Retain(context.Background(), "ctx")
	router.Retain(context.Background(), "ctx")
	firstReady := waitRouterSignal(t, ready)
	if firstReady.SourceToken == "" || first.created.Load() != 1 {
		t.Fatalf("first publication=%q client constructions=%d", firstReady.SourceToken, first.created.Load())
	}
	data, err := router.GetSecretData(context.Background(), firstReady.SourceToken, "ns", "name")
	if err != nil || len(data) != 1 || data[0].Key != "remote" {
		t.Fatalf("remote data=%v err=%v", data, err)
	}
	demand.signal(first)
	select {
	case duplicate := <-ready:
		t.Fatalf("current generation re-delivered ready=%q", duplicate.SourceToken)
	case <-time.After(20 * time.Millisecond):
	}

	firstClient.terminate(acceleratorsecret.ReasonProtocol)
	if got := waitRouterSignal(t, unavailable); got.SourceToken != firstReady.SourceToken {
		t.Fatalf("unavailable=%q", got.SourceToken)
	}
	first.current.Store(true) // Model a new SessionLease for the same current session.
	demand.signal(first)
	time.Sleep(20 * time.Millisecond)
	if first.created.Load() != 1 {
		t.Fatalf("rejected generation reconstructed %d times", first.created.Load())
	}

	secondClient := newFakeRouterClient()
	second := newFakeRouterSession(2, secondClient)
	demand.signal(second)
	secondReady := waitRouterSignal(t, ready)
	if secondReady.SourceToken == firstReady.SourceToken || second.created.Load() != 1 {
		t.Fatalf("replacement=%q constructions=%d", secondReady.SourceToken, second.created.Load())
	}
	router.Release()
	if demand.closed.Load() {
		t.Fatal("shared demand closed before final consumer")
	}
	router.Release()
	if !demand.closed.Load() {
		t.Fatal("final consumer did not close demand")
	}
}

func TestIntegratedSecretRouterRetiresWithCachedIdentityAfterLeaseInvalidation(t *testing.T) {
	type controlEvent struct {
		kind  string
		token SecretReadSourceToken
	}
	for _, test := range []struct {
		name string
		wake func(*fakeRouterDemand, *invalidatingRouterSession, *fakeRouterClient, *invalidatingRouterSession)
	}{
		{
			name: "demand change",
			wake: func(demand *fakeRouterDemand, old *invalidatingRouterSession, _ *fakeRouterClient, replacement *invalidatingRouterSession) {
				old.invalidate()
				demand.signal(replacement)
			},
		},
		{
			name: "client done",
			wake: func(demand *fakeRouterDemand, old *invalidatingRouterSession, oldClient *fakeRouterClient, replacement *invalidatingRouterSession) {
				demand.replaceSilently(replacement)
				old.invalidate()
				oldClient.terminate(acceleratorsecret.ReasonRemoteUnavailable)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			oldClient := newFakeRouterClient()
			oldClient.doneObserved = make(chan struct{})
			oldSession := newInvalidatingRouterSession(1, oldClient)
			replacementClient := newFakeRouterClient()
			replacementClient.doneObserved = make(chan struct{})
			replacementSession := newInvalidatingRouterSession(2, replacementClient)
			demand := newFakeRouterDemand(oldSession)
			controls := make(chan controlEvent, 8)
			router, _, _ := routerFixtureWith(t, demand, func(deps *integratedSecretRouterDependencies) {
				deps.ready = func(signal integratedSecretSourceSignal) {
					controls <- controlEvent{kind: "ready", token: signal.SourceToken}
				}
				deps.unavailable = func(signal integratedSecretSourceSignal) {
					controls <- controlEvent{kind: "unavailable", token: signal.SourceToken}
				}
			})

			router.Retain(context.Background(), "ctx")
			initial := waitRouterSignal(t, controls)
			if initial.kind != "ready" || initial.token == "" {
				t.Fatal("initial source did not become ready")
			}
			waitRouterSignal(t, oldClient.doneObserved)
			test.wake(demand, oldSession, oldClient, replacementSession)

			lost := waitRouterSignal(t, controls)
			fresh := waitRouterSignal(t, controls)
			if lost.kind != "unavailable" || lost.token != initial.token {
				t.Fatal("invalidated source did not publish exactly one matching unavailable event")
			}
			if fresh.kind != "ready" || fresh.token == "" || fresh.token == initial.token {
				t.Fatal("replacement did not publish one fresh ready token")
			}
			waitRouterSignal(t, replacementClient.doneObserved)

			if _, err := router.GetSecretData(context.Background(), initial.token, "ns", "old"); !errors.Is(err, ErrIntegratedSecretReadsUnavailable) {
				t.Fatal("invalidated source token remained authoritative")
			}
			if data, err := router.GetSecretData(context.Background(), fresh.token, "ns", "fresh"); err != nil || len(data) != 1 || data[0].Key != "remote" {
				t.Fatal("replacement source was not authoritative")
			}

			// Model a delayed old wake after replacement activation. The existing
			// identity equality guard must fence it without retiring the replacement.
			router.retireCurrent(oldSession.identity)
			select {
			case <-controls:
				t.Fatal("stale old-source wake retired or republished the replacement")
			default:
			}
			if data, err := router.GetSecretData(context.Background(), fresh.token, "ns", "after-stale"); err != nil || len(data) != 1 {
				t.Fatal("stale old-source wake fenced the replacement token")
			}
			if oldSession.identityCalls.Load() != 1 || replacementSession.identityCalls.Load() != 1 {
				t.Fatal("loss handling re-read an invalidated live session identity")
			}
			if oldClient.doneCalls.Load() != 1 || replacementClient.doneCalls.Load() != 1 {
				t.Fatal("closed client Done entered a busy loop")
			}
			if oldSession.closed.Load() != 1 || oldSession.Current() || !replacementSession.Current() {
				t.Fatal("session lease retirement did not preserve the replacement")
			}
		})
	}
}

func TestIntegratedSecretRouteFallbackTable(t *testing.T) {
	for _, reason := range []acceleratorsecret.SecretClientReason{acceleratorsecret.ReasonCapacity, acceleratorsecret.ReasonForbidden, acceleratorsecret.ReasonRemoteUnavailable} {
		t.Run(string(reason), func(t *testing.T) {
			client := newFakeRouterClient()
			client.data = func(context.Context, string, string) ([]k8s.DataEntry, error) { return nil, fakeSecretReason(reason) }
			demand := newFakeRouterDemand(newFakeRouterSession(1, client))
			router, ready, _ := routerFixture(t, demand)
			router.Retain(context.Background(), "ctx")
			token := waitRouterSignal(t, ready).SourceToken
			data, err := router.GetSecretData(context.Background(), token, "ns", "name")
			if err != nil || len(data) != 1 || data[0].Key != "direct" {
				t.Fatalf("fallback data=%v err=%v", data, err)
			}
		})
	}

	client := newFakeRouterClient()
	client.data = func(context.Context, string, string) ([]k8s.DataEntry, error) {
		return nil, fakeSecretReason(acceleratorsecret.ReasonProtocol)
	}
	demand := newFakeRouterDemand(newFakeRouterSession(1, client))
	router, ready, unavailable := routerFixture(t, demand)
	router.Retain(context.Background(), "ctx")
	token := waitRouterSignal(t, ready).SourceToken
	if _, err := router.GetSecretData(context.Background(), token, "ns", "name"); err == nil {
		t.Fatal("terminal protocol error succeeded")
	}
	if got := waitRouterSignal(t, unavailable); got.SourceToken != token {
		t.Fatalf("terminal unavailable=%q", got.SourceToken)
	}
}

func TestIntegratedSecretRouteMatrixAndFallbackTable(t *testing.T) {
	for _, reason := range []acceleratorsecret.SecretClientReason{acceleratorsecret.ReasonCapacity, acceleratorsecret.ReasonForbidden, acceleratorsecret.ReasonRemoteUnavailable} {
		t.Run("fallback-"+string(reason), func(t *testing.T) {
			client := newFakeRouterClient()
			client.list = func(context.Context, string, string, bool) ([]k8s.SecretListItem, error) {
				return nil, fakeSecretReason(reason)
			}
			client.data = func(context.Context, string, string) ([]k8s.DataEntry, error) { return nil, fakeSecretReason(reason) }
			client.yaml = func(context.Context, string, string) (string, error) { return "", fakeSecretReason(reason) }
			var directList, directData, directYAML atomic.Int32
			demand := newFakeRouterDemand(newFakeRouterSession(1, client))
			router, ready, _ := routerFixtureWith(t, demand, func(deps *integratedSecretRouterDependencies) {
				deps.directList = func(context.Context, string, string, bool) ([]k8s.SecretListItem, error) {
					directList.Add(1)
					return []k8s.SecretListItem{{DataKeys: 3}}, nil
				}
				deps.directData = func(string, string) ([]k8s.DataEntry, error) {
					directData.Add(1)
					return []k8s.DataEntry{{Key: "direct"}}, nil
				}
				deps.directYAML = func(string, string) (string, error) {
					directYAML.Add(1)
					return "direct", nil
				}
			})
			router.Retain(context.Background(), "ctx")
			token := waitRouterSignal(t, ready).SourceToken
			rows, listErr := router.ListSecretsMetadata(context.Background(), token, "list", "ns", false)
			data, dataErr := router.GetSecretData(context.Background(), token, "ns", "name")
			yamlValue, yamlErr := router.GetSecretYaml(context.Background(), token, "ns", "name")
			if listErr != nil || len(rows) != 1 || rows[0].DataKeys != 3 || dataErr != nil || len(data) != 1 || data[0].Key != "direct" || yamlErr != nil || yamlValue != "direct" {
				t.Fatal("closed fallback result mismatch")
			}
			if directList.Load() != 1 || directData.Load() != 1 || directYAML.Load() != 1 {
				t.Fatal("closed fallback did not run exactly once")
			}
		})
	}

	for _, test := range []struct {
		name string
		err  error
		ctx  func() context.Context
	}{
		{name: "reason-canceled", err: fakeSecretReason(acceleratorsecret.ReasonCanceled)},
		{name: "reason-deadline", err: fakeSecretReason(acceleratorsecret.ReasonDeadline)},
		{name: "caller-canceled", err: errors.New("untyped"), ctx: func() context.Context { ctx, cancel := context.WithCancel(context.Background()); cancel(); return ctx }},
		{name: "caller-deadline", err: errors.New("untyped"), ctx: func() context.Context {
			ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
			defer cancel()
			return ctx
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := newFakeRouterClient()
			client.data = func(context.Context, string, string) ([]k8s.DataEntry, error) { return nil, test.err }
			var direct atomic.Int32
			demand := newFakeRouterDemand(newFakeRouterSession(1, client))
			router, ready, unavailable := routerFixtureWith(t, demand, func(deps *integratedSecretRouterDependencies) {
				deps.directData = func(string, string) ([]k8s.DataEntry, error) { direct.Add(1); return nil, nil }
			})
			router.Retain(context.Background(), "ctx")
			token := waitRouterSignal(t, ready).SourceToken
			ctx := context.Background()
			if test.ctx != nil {
				ctx = test.ctx()
			}
			if _, err := router.GetSecretData(ctx, token, "ns", "name"); err == nil || direct.Load() != 0 {
				t.Fatal("cancellation or deadline used Direct fallback")
			}
			if value, err := router.GetSecretYaml(context.Background(), token, "ns", "name"); err != nil || value != "remote" {
				t.Fatal("cancellation or deadline retired current generation")
			}
			select {
			case <-unavailable:
				t.Fatal("cancellation or deadline published unavailable")
			case <-time.After(20 * time.Millisecond):
			}
		})
	}

	terminal := []struct {
		name string
		err  error
	}{
		{name: "protocol", err: fakeSecretReason(acceleratorsecret.ReasonProtocol)},
		{name: "session", err: fakeSecretReason(acceleratorsecret.ReasonSessionUnavailable)},
		{name: "watch-gap", err: fakeSecretReason(acceleratorsecret.ReasonWatchGap)},
		{name: "closed", err: fakeSecretReason(acceleratorsecret.ReasonClosed)},
		{name: "unknown", err: errors.New("untyped")},
	}
	for _, test := range terminal {
		t.Run("terminal-"+test.name, func(t *testing.T) {
			client := newFakeRouterClient()
			client.data = func(context.Context, string, string) ([]k8s.DataEntry, error) { return nil, test.err }
			session := newFakeRouterSession(1, client)
			demand := newFakeRouterDemand(session)
			router, ready, unavailable := routerFixture(t, demand)
			router.Retain(context.Background(), "ctx")
			token := waitRouterSignal(t, ready).SourceToken
			if _, err := router.GetSecretData(context.Background(), token, "ns", "name"); err == nil {
				t.Fatal("terminal failure succeeded")
			}
			if waitRouterSignal(t, unavailable).SourceToken != token {
				t.Fatal("terminal failure unavailable mismatch")
			}
			session.current.Store(true)
			demand.signal(session)
			time.Sleep(20 * time.Millisecond)
			if session.created.Load() != 1 {
				t.Fatal("terminal generation latch reconstructed client")
			}
		})
	}
}

func TestIntegratedSecretListCancellationTracksActualOwner(t *testing.T) {
	t.Run("remote", func(t *testing.T) {
		started := make(chan struct{})
		client := newFakeRouterClient()
		client.list = func(ctx context.Context, _, _ string, _ bool) ([]k8s.SecretListItem, error) {
			close(started)
			<-ctx.Done()
			return nil, ctx.Err()
		}
		var remoteCancel atomic.Int32
		client.cancel = func(context.Context, string) (bool, error) { remoteCancel.Add(1); return true, nil }
		demand := newFakeRouterDemand(newFakeRouterSession(1, client))
		router, ready, _ := routerFixture(t, demand)
		router.Retain(context.Background(), "ctx")
		token := waitRouterSignal(t, ready).SourceToken
		done := make(chan error, 1)
		go func() {
			_, err := router.ListSecretsMetadata(context.Background(), token, "owner", "ns", false)
			done <- err
		}()
		waitRouterSignal(t, started)
		if canceled, err := router.CancelListRequest(context.Background(), token, "owner"); err != nil || !canceled {
			t.Fatal("remote list cancel failed")
		}
		if err := waitRouterSignal(t, done); err == nil || remoteCancel.Load() != 1 {
			t.Fatal("remote list owner mismatch")
		}
	})

	t.Run("direct-fallback", func(t *testing.T) {
		directStarted := make(chan struct{})
		client := newFakeRouterClient()
		client.list = func(context.Context, string, string, bool) ([]k8s.SecretListItem, error) {
			return nil, fakeSecretReason(acceleratorsecret.ReasonRemoteUnavailable)
		}
		var remoteCancel atomic.Int32
		client.cancel = func(context.Context, string) (bool, error) { remoteCancel.Add(1); return true, nil }
		demand := newFakeRouterDemand(newFakeRouterSession(1, client))
		router, ready, _ := routerFixtureWith(t, demand, func(deps *integratedSecretRouterDependencies) {
			deps.directList = func(ctx context.Context, _, _ string, _ bool) ([]k8s.SecretListItem, error) {
				close(directStarted)
				<-ctx.Done()
				return nil, ctx.Err()
			}
		})
		router.Retain(context.Background(), "ctx")
		token := waitRouterSignal(t, ready).SourceToken
		done := make(chan error, 1)
		go func() {
			_, err := router.ListSecretsMetadata(context.Background(), token, "owner", "ns", false)
			done <- err
		}()
		waitRouterSignal(t, directStarted)
		if canceled, err := router.CancelListRequest(context.Background(), token, "owner"); err != nil || !canceled {
			t.Fatal("Direct fallback cancel failed")
		}
		if err := waitRouterSignal(t, done); err == nil || remoteCancel.Load() != 0 {
			t.Fatal("Direct fallback cancel crossed owners")
		}
	})
}

func TestIntegratedSecretCallGenerationAndCommitFences(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	client := newFakeRouterClient()
	client.data = func(context.Context, string, string) ([]k8s.DataEntry, error) {
		close(started)
		<-release
		return []k8s.DataEntry{{Key: "stale"}}, nil
	}
	var direct atomic.Int32
	demand := newFakeRouterDemand(newFakeRouterSession(1, client))
	router, ready, unavailable := routerFixtureWith(t, demand, func(deps *integratedSecretRouterDependencies) {
		deps.directData = func(string, string) ([]k8s.DataEntry, error) { direct.Add(1); return nil, nil }
	})
	router.Retain(context.Background(), "ctx")
	token := waitRouterSignal(t, ready).SourceToken
	done := make(chan error, 1)
	go func() { _, err := router.GetSecretData(context.Background(), token, "ns", "name"); done <- err }()
	waitRouterSignal(t, started)
	router.FenceContextSwitch("ctx")
	if waitRouterSignal(t, unavailable).SourceToken != token {
		t.Fatal("generation fence unavailable mismatch")
	}
	close(release)
	if err := waitRouterSignal(t, done); err == nil || direct.Load() != 0 {
		t.Fatal("stale operation committed or fell back")
	}
}

func TestIntegratedSecretWatchOwnershipAndGapTransition(t *testing.T) {
	firstID := acceleratorsecret.SecretWatchSpecIDFor("one", false)
	secondID := acceleratorsecret.SecretWatchSpecIDFor("two", true)
	firstWatch := newFakeRouterWatch(firstID)
	secondWatch := newFakeRouterWatch(secondID)
	client := newFakeRouterClient()
	var subscriptions atomic.Int32
	client.subscribe = func(context.Context, string, bool) (acceleratorsecret.SecretWatchSubscription, acceleratorprovision.SecretWatchLease, error) {
		if subscriptions.Add(1) == 1 {
			return firstWatch.subscription, firstWatch, nil
		}
		return secondWatch.subscription, secondWatch, nil
	}
	var unsubscriptions atomic.Int32
	client.unsub = func(_ context.Context, id acceleratorsecret.SecretWatchSpecID) error {
		unsubscriptions.Add(1)
		if id == firstID {
			firstWatch.terminate(acceleratorsecret.ReasonClosed)
		}
		return nil
	}
	demand := newFakeRouterDemand(newFakeRouterSession(1, client))
	router, ready, unavailable := routerFixture(t, demand)
	router.Retain(context.Background(), "ctx")
	token := waitRouterSignal(t, ready).SourceToken
	first, err := router.SubscribeSecretWatcher(context.Background(), token, "one", false)
	if err != nil || first.WatcherSpecID != firstID {
		t.Fatal("first watch subscribe failed")
	}
	if err = router.UnsubscribeSecretWatcher(context.Background(), token, firstID); err != nil || unsubscriptions.Load() != 1 {
		t.Fatal("planned watch unsubscribe failed")
	}
	select {
	case <-unavailable:
		t.Fatal("planned unsubscribe retired source")
	case <-time.After(20 * time.Millisecond):
	}
	second, err := router.SubscribeSecretWatcher(context.Background(), token, "two", true)
	if err != nil || second.WatcherSpecID != secondID {
		t.Fatal("second watch subscribe failed")
	}
	secondWatch.terminate(acceleratorsecret.ReasonWatchGap)
	if waitRouterSignal(t, unavailable).SourceToken != token {
		t.Fatal("watch gap unavailable mismatch")
	}
	if _, err = router.SubscribeSecretWatcher(context.Background(), token, "three", false); err == nil {
		t.Fatal("stale token subscribed after gap")
	}
}

func TestIntegratedSecretNilWatchLeaseRetiresGeneration(t *testing.T) {
	client := newFakeRouterClient()
	client.subscribe = func(context.Context, string, bool) (acceleratorsecret.SecretWatchSubscription, acceleratorprovision.SecretWatchLease, error) {
		return acceleratorsecret.SecretWatchSubscription{WatcherSpecID: acceleratorsecret.SecretWatchSpecIDFor("one", false)}, nil, nil
	}
	demand := newFakeRouterDemand(newFakeRouterSession(1, client))
	router, ready, unavailable := routerFixture(t, demand)
	router.Retain(context.Background(), "ctx")
	token := waitRouterSignal(t, ready).SourceToken
	if _, err := router.SubscribeSecretWatcher(context.Background(), token, "one", false); err == nil {
		t.Fatal("nil watch lease succeeded")
	}
	if waitRouterSignal(t, unavailable).SourceToken != token {
		t.Fatal("nil watch lease did not retire generation")
	}
}

func TestIntegratedSecretContextSwitchAdmissionUsesActualCurrentContext(t *testing.T) {
	for _, test := range []struct {
		name, actual string
		success      bool
	}{
		{name: "success", actual: "new", success: true},
		{name: "failure", actual: "old", success: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			first := newFakeRouterSession(1, newFakeRouterClient())
			second := newFakeRouterSession(2, newFakeRouterClient())
			firstDemand := newFakeRouterDemand(first)
			secondDemand := newFakeRouterDemand(second)
			acquired := make(chan string, 4)
			var acquireCalls atomic.Int32
			ready := make(chan integratedSecretSourceSignal, 4)
			unavailable := make(chan integratedSecretSourceSignal, 4)
			router, err := newIntegratedSecretRouter(integratedSecretRouterDependencies{
				acquire: func(_ context.Context, contextName string) (secretRouterDemandLease, bool) {
					acquired <- contextName
					if acquireCalls.Add(1) == 1 {
						return firstDemand, true
					}
					return secondDemand, true
				},
				ready:       func(signal integratedSecretSourceSignal) { ready <- signal },
				unavailable: func(signal integratedSecretSourceSignal) { unavailable <- signal },
				entropy:     bytes.NewReader(make([]byte, 16)),
			})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { router.Close(context.Background()) })

			router.Retain(context.Background(), "old")
			if got := waitRouterSignal(t, acquired); got != "old" {
				t.Fatalf("initial acquisition=%q", got)
			}
			firstToken := waitRouterSignal(t, ready).SourceToken
			router.FenceContextSwitch("old")
			if got := waitRouterSignal(t, unavailable).SourceToken; got != firstToken {
				t.Fatalf("old source teardown=%q", got)
			}
			if first.current.Load() || first.closed.Load() == 0 {
				t.Fatal("old session lease remained current across fence")
			}

			// This retain models a Secret consumer mounting after the router fence
			// while the coordinator/context mutation is still in progress.
			router.Retain(context.Background(), "old")
			if acquireCalls.Load() != 1 || len(acquired) != 0 {
				t.Fatal("retain during context switch reacquired the departing context")
			}
			router.ContextSwitched(test.actual, test.success)
			if got := waitRouterSignal(t, acquired); got != test.actual {
				t.Fatalf("actual-current acquisition=%q want=%q", got, test.actual)
			}
			secondToken := waitRouterSignal(t, ready).SourceToken
			if secondToken == "" || secondToken == firstToken || acquireCalls.Load() != 2 || second.created.Load() != 1 {
				t.Fatal("context completion did not create exactly one fresh source")
			}
			if len(acquired) != 0 {
				t.Fatal("context completion launched a duplicate demand")
			}
			router.Release()
			router.Release()
		})
	}
}

func TestIntegratedSecretInvalidChangesTerminatesWithoutSpin(t *testing.T) {
	closedChanges := make(chan struct{})
	close(closedChanges)
	for _, test := range []struct {
		name    string
		changes <-chan struct{}
		valid   bool
		calls   int32
	}{
		{name: "invalid lease", changes: make(chan struct{}), calls: 1},
		{name: "permanently closed current channel", changes: closedChanges, valid: true, calls: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			demand := &invalidChangesDemand{changes: test.changes, valid: test.valid}
			router, err := newIntegratedSecretRouter(integratedSecretRouterDependencies{
				acquire: func(context.Context, string) (secretRouterDemandLease, bool) { return demand, true },
				entropy: bytes.NewReader(make([]byte, 16)),
			})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { router.Close(context.Background()) })
			router.Retain(context.Background(), "ctx")
			deadline := time.Now().Add(time.Second)
			for demand.closed.Load() == 0 && time.Now().Before(deadline) {
				time.Sleep(time.Millisecond)
			}
			if demand.closed.Load() != 1 || demand.snapshotCalls.Load() != test.calls {
				t.Fatalf("invalid Changes cleanup=%d calls=%d", demand.closed.Load(), demand.snapshotCalls.Load())
			}
			time.Sleep(20 * time.Millisecond)
			if demand.snapshotCalls.Load() != test.calls {
				t.Fatal("invalid Changes channel entered a hot loop")
			}
		})
	}
}

func TestIntegratedSecretValidRotatedChangesReachLatestSession(t *testing.T) {
	client := newFakeRouterClient()
	session := newFakeRouterSession(3, client)
	demand := newRotatingChangesDemand(session)
	ready := make(chan integratedSecretSourceSignal, 1)
	router, err := newIntegratedSecretRouter(integratedSecretRouterDependencies{
		acquire: func(context.Context, string) (secretRouterDemandLease, bool) { return demand, true },
		ready:   func(signal integratedSecretSourceSignal) { ready <- signal },
		entropy: bytes.NewReader(make([]byte, 16)),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { router.Close(context.Background()) })

	router.Retain(context.Background(), "ctx")
	waitRouterSignal(t, demand.initialFetched)
	demand.rotateToSecond()
	if token := waitRouterSignal(t, ready).SourceToken; token == "" {
		t.Fatal("latest valid session published an empty source token")
	}
	if demand.closed.Load() {
		t.Fatal("valid rotated demand was retired")
	}
	if got := demand.snapshotCalls.Load(); got != 3 {
		t.Fatalf("signal snapshots=%d want=3", got)
	}
	if got := session.created.Load(); got != 1 {
		t.Fatalf("latest session clients=%d want=1", got)
	}
}

func TestIntegratedSecretHostileCorpusOnlyReturnsSecretMaterialFromExplicitDetailCalls(t *testing.T) {
	const (
		contextMarker = "private-context-MARKER"
		dataMarker    = "private-data-MARKER"
		yamlMarker    = "private-yaml-MARKER"
		errorMarker   = "private-upstream-error-MARKER"
	)
	client := newFakeRouterClient()
	client.data = func(context.Context, string, string) ([]k8s.DataEntry, error) {
		return []k8s.DataEntry{{Key: "password", Value: dataMarker}}, nil
	}
	client.yaml = func(context.Context, string, string) (string, error) { return yamlMarker, nil }
	demand := newFakeRouterDemand(newFakeRouterSession(1, client))
	ready := make(chan integratedSecretSourceSignal, 1)
	unavailable := make(chan integratedSecretSourceSignal, 1)
	router, err := newIntegratedSecretRouter(integratedSecretRouterDependencies{
		acquire:     func(context.Context, string) (secretRouterDemandLease, bool) { return demand, true },
		ready:       func(signal integratedSecretSourceSignal) { ready <- signal },
		unavailable: func(signal integratedSecretSourceSignal) { unavailable <- signal },
		entropy:     bytes.NewReader(make([]byte, 16)),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { router.Close(context.Background()) })
	router.Retain(context.Background(), contextMarker)
	readySignal := waitRouterSignal(t, ready)

	data, dataErr := router.GetSecretData(context.Background(), readySignal.SourceToken, "private-ns", "private-name")
	if dataErr != nil || len(data) != 1 || data[0].Value != dataMarker {
		t.Fatalf("explicit data result=%v err=%v", data, dataErr)
	}
	yaml, yamlErr := router.GetSecretYaml(context.Background(), readySignal.SourceToken, "private-ns", "private-name")
	if yamlErr != nil || yaml != yamlMarker {
		t.Fatalf("explicit YAML result=%q err=%v", yaml, yamlErr)
	}

	watchID := acceleratorsecret.SecretWatchSpecIDFor("visible-metadata", false)
	var projected acceleratorsecret.SecretListItem
	projected.Metadata.Name = "visible-name"
	projected.Metadata.Namespace = "visible-metadata"
	projected.DataKeys = 1
	events := []any{
		readySignal,
		integratedSecretResourceSignal{SourceToken: readySignal.SourceToken, Type: "MODIFIED", ResourceType: acceleratorsecret.SecretResourceType, Namespace: "visible-metadata", WatcherSpecID: watchID, Resource: projected},
		integratedSecretStatusSignal{SourceToken: readySignal.SourceToken, WatcherSpecID: watchID, Status: acceleratorsecret.WatchStatusConnected},
		integratedSecretErrorSignal{SourceToken: readySignal.SourceToken, WatcherSpecID: watchID, Code: acceleratorsecret.WatchErrorUnavailable, Recoverable: true},
	}
	var eventArtifacts []string
	for _, event := range events {
		wire, marshalErr := json.Marshal(event)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		eventArtifacts = append(eventArtifacts, fmt.Sprint(event), fmt.Sprintf("%+v", event), string(wire))
	}

	client.data = func(context.Context, string, string) ([]k8s.DataEntry, error) {
		return nil, fmt.Errorf("upstream detail: %s", errorMarker)
	}
	_, fixedErr := router.GetSecretData(context.Background(), readySignal.SourceToken, "private-ns", "private-name")
	if !errors.Is(fixedErr, ErrIntegratedSecretReadsUnavailable) || strings.Contains(fixedErr.Error(), errorMarker) {
		t.Fatalf("router exposed upstream error: %v", fixedErr)
	}
	unavailableSignal := waitRouterSignal(t, unavailable)
	unavailableWire, marshalErr := json.Marshal(unavailableSignal)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	eventArtifacts = append(eventArtifacts, fmt.Sprint(unavailableSignal), string(unavailableWire))

	var logs bytes.Buffer
	priorLogWriter := log.Writer()
	log.SetOutput(&logs)
	defer log.SetOutput(priorLogWriter)
	log.Printf("router=%+v fixed=%v", router, fixedErr)
	routerWire, marshalErr := json.Marshal(router)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	errorWire, marshalErr := json.Marshal(fixedErr)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	privateArtifacts := []string{
		fmt.Sprint(router), fmt.Sprintf("%+v", router), fmt.Sprintf("%#v", router),
		string(routerWire), fmt.Sprint(fixedErr), fmt.Sprintf("%+v", fixedErr), string(errorWire), logs.String(),
	}
	for _, artifact := range privateArtifacts {
		for _, forbidden := range []string{contextMarker, dataMarker, yamlMarker, errorMarker, string(readySignal.SourceToken)} {
			if strings.Contains(artifact, forbidden) {
				t.Fatalf("private router material crossed formatter/JSON/log boundary: %q", artifact)
			}
		}
	}
	for _, artifact := range eventArtifacts {
		for _, forbidden := range []string{contextMarker, dataMarker, yamlMarker, errorMarker} {
			if strings.Contains(artifact, forbidden) {
				t.Fatalf("Secret detail crossed control event boundary: %q", artifact)
			}
		}
	}
	if explicit := fmt.Sprint(data) + yaml; !strings.Contains(explicit, dataMarker) || !strings.Contains(explicit, yamlMarker) {
		t.Fatal("explicit detail methods did not return their authorized payloads")
	}
}

func TestIntegratedSecretLifecycleTeardownClosesClientBeforeWatchLease(t *testing.T) {
	for _, test := range []struct {
		name   string
		invoke func(*integratedSecretRouter)
	}{
		{name: "release", invoke: func(router *integratedSecretRouter) { router.Release() }},
		{name: "context fence", invoke: func(router *integratedSecretRouter) { router.FenceContextSwitch("ctx") }},
		{name: "quiesce", invoke: func(router *integratedSecretRouter) { router.Quiesce(context.Background()) }},
		{name: "close", invoke: func(router *integratedSecretRouter) { router.Close(context.Background()) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			clientClosed := make(chan struct{})
			client := newFakeRouterClient()
			client.onClose = func() { close(clientClosed) }
			id := acceleratorsecret.SecretWatchSpecIDFor("team", false)
			watch := &clientFirstCleanupWatch{
				subscription: acceleratorsecret.SecretWatchSubscription{WatcherSpecID: id},
				events:       make(chan acceleratorprovision.SecretWatchEvent),
				done:         make(chan struct{}),
				clientClosed: clientClosed,
			}
			client.subscribe = func(context.Context, string, bool) (acceleratorsecret.SecretWatchSubscription, acceleratorprovision.SecretWatchLease, error) {
				return watch.subscription, watch, nil
			}
			demand := newFakeRouterDemand(newFakeRouterSession(1, client))
			router, ready, _ := routerFixture(t, demand)
			router.Retain(context.Background(), "ctx")
			token := waitRouterSignal(t, ready).SourceToken
			if _, err := router.SubscribeSecretWatcher(context.Background(), token, "team", false); err != nil {
				t.Fatal("watch setup failed")
			}
			done := make(chan struct{})
			go func() {
				test.invoke(router)
				close(done)
			}()
			select {
			case <-done:
			case <-time.After(250 * time.Millisecond):
				t.Fatal("lifecycle teardown waited for remote unsubscribe")
			}
			if watch.closed.Load() != 1 {
				t.Fatal("watch lease was not closed after local client termination")
			}
		})
	}
}

func TestIntegratedSecretUnavailablePrecedesCleanupSideEffects(t *testing.T) {
	operationStarted := make(chan struct{})
	operationWoken := make(chan struct{})
	clientClosed := make(chan struct{})
	unavailableEntered := make(chan struct{})
	releaseUnavailable := make(chan struct{})
	client := newFakeRouterClient()
	client.data = func(ctx context.Context, _, _ string) ([]k8s.DataEntry, error) {
		close(operationStarted)
		<-ctx.Done()
		close(operationWoken)
		return nil, ctx.Err()
	}
	client.onClose = func() { close(clientClosed) }
	session := newFakeRouterSession(1, client)
	demand := newFakeRouterDemand(session)
	ready := make(chan integratedSecretSourceSignal, 1)
	router, err := newIntegratedSecretRouter(integratedSecretRouterDependencies{
		acquire: func(context.Context, string) (secretRouterDemandLease, bool) { return demand, true },
		ready:   func(signal integratedSecretSourceSignal) { ready <- signal },
		unavailable: func(integratedSecretSourceSignal) {
			close(unavailableEntered)
			<-releaseUnavailable
		},
		entropy: bytes.NewReader(make([]byte, 16)),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { router.Close(context.Background()) })
	router.Retain(context.Background(), "ctx")
	token := waitRouterSignal(t, ready).SourceToken
	operationDone := make(chan error, 1)
	go func() {
		_, callErr := router.GetSecretData(context.Background(), token, "ns", "name")
		operationDone <- callErr
	}()
	waitRouterSignal(t, operationStarted)

	releaseDone := make(chan struct{})
	go func() {
		router.Release()
		close(releaseDone)
	}()
	waitRouterSignal(t, unavailableEntered)
	for name, signal := range map[string]<-chan struct{}{
		"operation cancellation": operationWoken,
		"client close":           clientClosed,
	} {
		select {
		case <-signal:
			t.Fatalf("%s overtook unavailable publication", name)
		default:
		}
	}
	if session.closed.Load() != 0 || demand.closed.Load() {
		t.Fatal("lease cleanup overtook unavailable publication")
	}
	if _, callErr := router.GetSecretData(context.Background(), token, "ns", "late"); !errors.Is(callErr, ErrIntegratedSecretReadsUnavailable) {
		t.Fatalf("detached source remained authoritative while unavailable was emitted: %v", callErr)
	}

	close(releaseUnavailable)
	waitRouterSignal(t, releaseDone)
	waitRouterSignal(t, operationWoken)
	waitRouterSignal(t, clientClosed)
	if callErr := waitRouterSignal(t, operationDone); !errors.Is(callErr, ErrIntegratedSecretReadsUnavailable) {
		t.Fatalf("woken old-source operation error=%v", callErr)
	}
	if session.closed.Load() != 1 || !demand.closed.Load() {
		t.Fatal("ordered cleanup did not close both leases")
	}
}

func TestIntegratedSecretDetachReservesUnavailableBeforeReplacementReady(t *testing.T) {
	firstClient := newFakeRouterClient()
	firstClosed := make(chan struct{})
	firstClient.onClose = func() { close(firstClosed) }
	firstSession := newFakeRouterSession(1, firstClient)
	demand := newFakeRouterDemand(firstSession)
	controls := make(chan string, 4)
	router, err := newIntegratedSecretRouter(integratedSecretRouterDependencies{
		acquire: func(context.Context, string) (secretRouterDemandLease, bool) { return demand, true },
		ready: func(signal integratedSecretSourceSignal) {
			controls <- "ready:" + string(signal.SourceToken)
		},
		unavailable: func(signal integratedSecretSourceSignal) {
			controls <- "unavailable:" + string(signal.SourceToken)
		},
		entropy: bytes.NewReader(make([]byte, 16)),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { router.Close(context.Background()) })
	router.Retain(context.Background(), "ctx")
	firstReady := waitRouterSignal(t, controls)
	if !strings.HasPrefix(firstReady, "ready:") {
		t.Fatalf("initial control=%q", firstReady)
	}
	firstToken := SecretReadSourceToken(strings.TrimPrefix(firstReady, "ready:"))

	// Split detach from cleanup deliberately. The unavailable lane position
	// must already be owned before another valid session can publish ready.
	router.mu.Lock()
	cleanup := router.detachLocked(false)
	router.mu.Unlock()
	if cleanup.unavailableReservation.done == nil {
		t.Fatal("detach did not reserve unavailable on the source-control lane")
	}
	secondClient := newFakeRouterClient()
	secondSession := newFakeRouterSession(2, secondClient)
	demand.signal(secondSession)
	deadline := time.Now().Add(time.Second)
	replacementReserved := false
	for !replacementReserved && time.Now().Before(deadline) {
		router.eventMu.Lock()
		replacementReserved = router.sourceEventTail != cleanup.unavailableReservation.done
		router.eventMu.Unlock()
		time.Sleep(time.Millisecond)
	}
	if secondSession.created.Load() != 1 || !replacementReserved {
		t.Fatal("replacement did not reserve its ready publication behind unavailable")
	}
	select {
	case control := <-controls:
		t.Fatalf("replacement control escaped before reserved unavailable: %q", control)
	default:
	}

	router.runCleanup(cleanup)
	firstControl := waitRouterSignal(t, controls)
	secondControl := waitRouterSignal(t, controls)
	if firstControl != "unavailable:"+string(firstToken) || !strings.HasPrefix(secondControl, "ready:") {
		t.Fatalf("source controls=%q then %q", firstControl, secondControl)
	}
	secondToken := SecretReadSourceToken(strings.TrimPrefix(secondControl, "ready:"))
	if secondToken == "" || secondToken == firstToken {
		t.Fatal("replacement source token was not fresh")
	}
	select {
	case extra := <-controls:
		t.Fatalf("duplicate source control=%q", extra)
	default:
	}
	waitRouterSignal(t, firstClosed)
	if firstSession.closed.Load() != 1 || demand.closed.Load() {
		t.Fatal("old cleanup did not follow unavailable or retired the valid demand")
	}
	if _, callErr := router.GetSecretData(context.Background(), firstToken, "ns", "old"); !errors.Is(callErr, ErrIntegratedSecretReadsUnavailable) {
		t.Fatalf("old backend source remained current: %v", callErr)
	}
	if data, callErr := router.GetSecretData(context.Background(), secondToken, "ns", "new"); callErr != nil || len(data) != 1 {
		t.Fatalf("backend did not converge on replacement: data=%v err=%v", data, callErr)
	}
}

func TestIntegratedSecretExplicitUnsubscribeStillWaitsForItsRPC(t *testing.T) {
	release := make(chan struct{})
	entered := make(chan struct{})
	client := newFakeRouterClient()
	id := acceleratorsecret.SecretWatchSpecIDFor("team", false)
	watch := newFakeRouterWatch(id)
	client.subscribe = func(context.Context, string, bool) (acceleratorsecret.SecretWatchSubscription, acceleratorprovision.SecretWatchLease, error) {
		return watch.subscription, watch, nil
	}
	client.unsub = func(context.Context, acceleratorsecret.SecretWatchSpecID) error {
		close(entered)
		<-release
		return nil
	}
	router, ready, _ := routerFixture(t, newFakeRouterDemand(newFakeRouterSession(1, client)))
	router.Retain(context.Background(), "ctx")
	token := waitRouterSignal(t, ready).SourceToken
	if _, err := router.SubscribeSecretWatcher(context.Background(), token, "team", false); err != nil {
		t.Fatal("watch setup failed")
	}
	done := make(chan error, 1)
	go func() { done <- router.UnsubscribeSecretWatcher(context.Background(), token, id) }()
	waitRouterSignal(t, entered)
	select {
	case <-done:
		t.Fatal("explicit unsubscribe returned before its RPC")
	default:
	}
	close(release)
	if err := waitRouterSignal(t, done); err != nil {
		t.Fatal(err)
	}
}

func TestIntegratedSecretWatchEventsShareSourceEventOrder(t *testing.T) {
	id := acceleratorsecret.SecretWatchSpecIDFor("team", false)
	watch := newFakeRouterWatch(id)
	client := newFakeRouterClient()
	client.subscribe = func(context.Context, string, bool) (acceleratorsecret.SecretWatchSubscription, acceleratorprovision.SecretWatchLease, error) {
		return watch.subscription, watch, nil
	}
	var orderMu sync.Mutex
	order := make([]string, 0, 3)
	resourceEntered := make(chan struct{})
	resourceRelease := make(chan struct{})
	router, ready, unavailable := routerFixtureWith(t, newFakeRouterDemand(newFakeRouterSession(1, client)), func(deps *integratedSecretRouterDependencies) {
		originalUnavailable := deps.unavailable
		deps.unavailable = func(signal integratedSecretSourceSignal) {
			orderMu.Lock()
			order = append(order, "unavailable")
			orderMu.Unlock()
			originalUnavailable(signal)
		}
		deps.resource = func(integratedSecretResourceSignal) {
			orderMu.Lock()
			order = append(order, "resource")
			orderMu.Unlock()
			close(resourceEntered)
			<-resourceRelease
		}
	})
	router.Retain(context.Background(), "ctx")
	token := waitRouterSignal(t, ready).SourceToken
	if _, err := router.SubscribeSecretWatcher(context.Background(), token, "team", false); err != nil {
		t.Fatal("watch setup failed")
	}
	router.mu.Lock()
	owner := router.watches[id]
	router.mu.Unlock()
	if owner == nil {
		t.Fatal("watch owner unavailable")
	}
	go router.publishWatchEvent(id, owner, func() {
		router.deps.resource(integratedSecretResourceSignal{SourceToken: token, WatcherSpecID: id})
	})
	waitRouterSignal(t, resourceEntered)
	fenced := make(chan struct{})
	go func() {
		router.FenceContextSwitch("ctx")
		close(fenced)
	}()
	deadline := time.Now().Add(time.Second)
	for {
		router.mu.Lock()
		detached := router.token == ""
		router.mu.Unlock()
		if detached || time.Now().After(deadline) {
			if !detached {
				t.Fatal("watch source did not detach")
			}
			break
		}
		time.Sleep(time.Millisecond)
	}
	select {
	case <-unavailable:
		t.Fatal("unavailable overtook an event already emitting")
	default:
	}
	close(resourceRelease)
	waitRouterSignal(t, fenced)
	if got := waitRouterSignal(t, unavailable).SourceToken; got != token {
		t.Fatal("unavailable token mismatch")
	}
	router.publishWatchEvent(id, owner, func() {
		orderMu.Lock()
		order = append(order, "late")
		orderMu.Unlock()
	})
	orderMu.Lock()
	defer orderMu.Unlock()
	if len(order) != 2 || order[0] != "resource" || order[1] != "unavailable" {
		t.Fatalf("event order=%v", order)
	}
}
