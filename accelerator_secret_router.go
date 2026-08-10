//go:build !accelerator

package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"sync"

	"kubikles/pkg/acceleratorprovision"
	"kubikles/pkg/acceleratorsecret"
	"kubikles/pkg/helm"
	"kubikles/pkg/k8s"
)

const (
	integratedSecretReadyEvent       = "accelerator:secret-source-ready"
	integratedSecretUnavailableEvent = "accelerator:secret-source-unavailable"
	integratedSecretResourceEvent    = "accelerator:secret-resource"
	integratedSecretStatusEvent      = "accelerator:secret-watcher-status"
	integratedSecretErrorEvent       = "accelerator:secret-watcher-error"
)

type integratedSecretSourceSignal struct {
	SourceToken SecretReadSourceToken `json:"sourceToken"`
}

type integratedSecretResourceSignal struct {
	SourceToken   SecretReadSourceToken               `json:"sourceToken"`
	Type          string                              `json:"type"`
	ResourceType  string                              `json:"resourceType"`
	Namespace     string                              `json:"namespace"`
	WatcherSpecID acceleratorsecret.SecretWatchSpecID `json:"watcherSpecId"`
	Resource      acceleratorsecret.SecretListItem    `json:"resource"`
}

type integratedSecretStatusSignal struct {
	SourceToken   SecretReadSourceToken               `json:"sourceToken"`
	WatcherSpecID acceleratorsecret.SecretWatchSpecID `json:"watcherSpecId"`
	Status        string                              `json:"status"`
}

type integratedSecretErrorSignal struct {
	SourceToken   SecretReadSourceToken               `json:"sourceToken"`
	WatcherSpecID acceleratorsecret.SecretWatchSpecID `json:"watcherSpecId"`
	Code          string                              `json:"code"`
	Recoverable   bool                                `json:"recoverable"`
}

type secretRouterDemandLease interface {
	Changes() <-chan struct{}
	CurrentChanges() (<-chan struct{}, bool)
	TrySession() (secretRouterSessionLease, bool)
	Close()
}

type secretRouterSessionLease interface {
	Identity() secretRouterSessionIdentity
	Current() bool
	NewClient() (acceleratorprovision.SecretRPCClient, error)
	Close()
}

type secretRouterSessionIdentity struct {
	session    *acceleratorprovision.ConnectedSession
	generation int
}

func (i secretRouterSessionIdentity) valid() bool { return i.session != nil && i.generation > 0 }

type productionSecretDemandLease struct {
	lease     *acceleratorprovision.SecretDemandLease
	newClient func(*acceleratorprovision.SessionLease) (acceleratorprovision.SecretRPCClient, error)
}

func (l *productionSecretDemandLease) Changes() <-chan struct{} { return l.lease.Changes() }
func (l *productionSecretDemandLease) CurrentChanges() (<-chan struct{}, bool) {
	return l.lease.CurrentChanges()
}
func (l *productionSecretDemandLease) Close() { l.lease.Close() }
func (l *productionSecretDemandLease) TrySession() (secretRouterSessionLease, bool) {
	lease, ok := l.lease.TrySession()
	if !ok {
		return nil, false
	}
	return &productionSecretSessionLease{lease: lease, newClient: l.newClient}, true
}

type productionSecretSessionLease struct {
	lease     *acceleratorprovision.SessionLease
	newClient func(*acceleratorprovision.SessionLease) (acceleratorprovision.SecretRPCClient, error)
}

func (l *productionSecretSessionLease) Identity() secretRouterSessionIdentity {
	session := l.lease.Session()
	if session == nil {
		return secretRouterSessionIdentity{}
	}
	return secretRouterSessionIdentity{session: session, generation: session.Identity().Generation}
}
func (l *productionSecretSessionLease) Current() bool { return l.lease.Session() != nil }
func (l *productionSecretSessionLease) NewClient() (acceleratorprovision.SecretRPCClient, error) {
	return l.newClient(l.lease)
}
func (l *productionSecretSessionLease) Close() { l.lease.Close() }

type integratedSecretRouterDependencies struct {
	acquire               func(context.Context, string) (secretRouterDemandLease, bool)
	directList            func(context.Context, string, string, bool) ([]k8s.SecretListItem, error)
	directHelmReleaseList func(context.Context, string, string) ([]helm.Release, error)
	directData            func(string, string) ([]k8s.DataEntry, error)
	directYAML            func(string, string) (string, error)
	ready                 func(integratedSecretSourceSignal)
	unavailable           func(integratedSecretSourceSignal)
	resource              func(integratedSecretResourceSignal)
	status                func(integratedSecretStatusSignal)
	watchError            func(integratedSecretErrorSignal)
	entropy               io.Reader
}

type integratedSecretRouter struct {
	deps  integratedSecretRouterDependencies
	nonce string

	eventMu          sync.Mutex
	sourceEventTail  <-chan struct{}
	mu               sync.Mutex
	consumers        int
	contextName      string
	contextEpoch     uint64
	demandEpoch      uint64
	routeEpoch       uint64
	tokenCounter     uint64
	opCounter        uint64
	quiesced         bool
	closed           bool
	contextSwitching bool
	demandCancel     context.CancelFunc
	demand           secretRouterDemandLease
	session          secretRouterSessionLease
	sessionIdentity  secretRouterSessionIdentity
	rejectedIdentity secretRouterSessionIdentity
	client           acceleratorprovision.SecretRPCClient
	token            SecretReadSourceToken
	operations       map[uint64]context.CancelFunc
	lists            map[string]*secretListRouteOwner
	watches          map[acceleratorsecret.SecretWatchSpecID]*secretWatchRouteOwner
	monitors         sync.WaitGroup
}

func (*integratedSecretRouter) String() string { return "<integrated secret router>" }
func (*integratedSecretRouter) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "<integrated secret router>")
}
func (*integratedSecretRouter) MarshalJSON() ([]byte, error) {
	return []byte(`"<integrated secret router>"`), nil
}

func newIntegratedSecretRouter(deps integratedSecretRouterDependencies) (*integratedSecretRouter, error) {
	if deps.acquire == nil {
		return nil, ErrIntegratedSecretReadsUnavailable
	}
	if deps.entropy == nil {
		deps.entropy = rand.Reader
	}
	nonceBytes := make([]byte, 16)
	if _, err := io.ReadFull(deps.entropy, nonceBytes); err != nil {
		return nil, ErrIntegratedSecretReadsUnavailable
	}
	initialEvent := make(chan struct{})
	close(initialEvent)
	return &integratedSecretRouter{
		deps: deps, nonce: base64.RawURLEncoding.EncodeToString(nonceBytes),
		operations: make(map[uint64]context.CancelFunc), lists: make(map[string]*secretListRouteOwner), watches: make(map[acceleratorsecret.SecretWatchSpecID]*secretWatchRouteOwner),
		sourceEventTail: initialEvent,
	}, nil
}

func (r *integratedSecretRouter) Retain(_ context.Context, contextName string) {
	if r == nil || contextName == "" {
		return
	}
	r.mu.Lock()
	if r.closed || r.quiesced {
		r.mu.Unlock()
		return
	}
	r.consumers++
	if r.consumers != 1 || r.contextSwitching {
		r.mu.Unlock()
		return
	}
	ctx, epoch, start := r.prepareDemandLocked(contextName)
	r.mu.Unlock()
	if start {
		go r.runDemand(ctx, contextName, epoch)
	}
}

// prepareDemandLocked reserves exactly one demand monitor. The caller must
// hold r.mu and start the returned monitor only after releasing it.
func (r *integratedSecretRouter) prepareDemandLocked(contextName string) (context.Context, uint64, bool) {
	if contextName == "" || r.closed || r.quiesced || r.contextSwitching || r.consumers == 0 || r.demand != nil || r.demandCancel != nil {
		return nil, 0, false
	}
	r.contextName = contextName
	r.demandEpoch++
	epoch := r.demandEpoch
	ctx, cancel := context.WithCancel(context.Background())
	r.demandCancel = cancel
	r.monitors.Add(1)
	return ctx, epoch, true
}

func (r *integratedSecretRouter) Release() {
	if r == nil {
		return
	}
	r.mu.Lock()
	if r.consumers == 0 {
		r.mu.Unlock()
		return
	}
	r.consumers--
	if r.consumers != 0 {
		r.mu.Unlock()
		return
	}
	r.demandEpoch++
	cleanup := r.detachLocked(true)
	r.contextName = ""
	r.mu.Unlock()
	r.runCleanup(cleanup)
	r.mu.Lock()
	if r.consumers == 0 && r.demand == nil {
		r.rejectedIdentity = secretRouterSessionIdentity{}
	}
	r.mu.Unlock()
}

func (r *integratedSecretRouter) runDemand(ctx context.Context, contextName string, epoch uint64) {
	defer r.monitors.Done()
	demand, accepted := r.deps.acquire(ctx, contextName)
	if !accepted || demand == nil {
		return
	}
	r.mu.Lock()
	if r.closed || r.quiesced || r.consumers == 0 || r.demandEpoch != epoch || r.contextName != contextName || r.demand != nil {
		r.mu.Unlock()
		demand.Close()
		return
	}
	r.demand = demand
	r.mu.Unlock()
	changes, valid := demand.CurrentChanges()
	if !valid || changes == nil {
		r.retireDemand(demand, epoch)
		return
	}

	for {
		r.tryActivate(demand, epoch)
		r.mu.Lock()
		if r.closed || r.demandEpoch != epoch || r.demand != demand {
			r.mu.Unlock()
			return
		}
		client := r.client
		session := r.session
		identity := r.sessionIdentity
		var clientDone <-chan struct{}
		if client != nil {
			clientDone = client.Done()
		}
		r.mu.Unlock()
		select {
		case <-ctx.Done():
			return
		case _, open := <-changes:
			if !open {
				for {
					next, nextValid := demand.CurrentChanges()
					if !nextValid || next == nil || next == changes {
						r.retireDemand(demand, epoch)
						return
					}
					changes = next
					select {
					case _, nextOpen := <-changes:
						if !nextOpen {
							continue
						}
					default:
					}
					break
				}
			}
			if session != nil && !session.Current() {
				r.retireCurrent(identity)
			}
		case <-clientDone:
			if client != nil {
				r.retireCurrent(identity)
			}
		}
	}
}

func (r *integratedSecretRouter) retireDemand(demand secretRouterDemandLease, epoch uint64) {
	r.mu.Lock()
	if r.closed || r.demandEpoch != epoch || r.demand != demand {
		r.mu.Unlock()
		return
	}
	r.demandEpoch++
	cleanup := r.detachLocked(true)
	r.mu.Unlock()
	r.runCleanup(cleanup)
}

func (r *integratedSecretRouter) tryActivate(demand secretRouterDemandLease, epoch uint64) {
	r.mu.Lock()
	if r.closed || r.quiesced || r.demandEpoch != epoch || r.demand != demand || r.client != nil {
		r.mu.Unlock()
		return
	}
	r.mu.Unlock()
	lease, ok := demand.TrySession()
	if !ok || lease == nil {
		return
	}
	identity := lease.Identity()
	if !identity.valid() || !lease.Current() {
		lease.Close()
		return
	}
	r.mu.Lock()
	rejected := r.rejectedIdentity.valid() && r.rejectedIdentity == identity
	stale := r.closed || r.quiesced || r.demandEpoch != epoch || r.demand != demand || r.client != nil
	r.mu.Unlock()
	if rejected || stale {
		lease.Close()
		return
	}
	client, err := lease.NewClient()
	if err != nil || client == nil {
		lease.Close()
		return
	}
	r.mu.Lock()
	stale = r.closed || r.quiesced || r.demandEpoch != epoch || r.demand != demand || r.client != nil || !lease.Current()
	if !stale && r.rejectedIdentity.valid() && r.rejectedIdentity == identity {
		stale = true
	}
	if stale {
		r.mu.Unlock()
		client.Close(context.Background())
		lease.Close()
		return
	}
	r.tokenCounter++
	r.routeEpoch++
	r.client = client
	r.session = lease
	r.sessionIdentity = identity
	r.rejectedIdentity = secretRouterSessionIdentity{}
	r.token = SecretReadSourceToken(fmt.Sprintf("s.%s.%016x", r.nonce, r.tokenCounter))
	token := r.token
	eventEpoch := r.routeEpoch
	r.mu.Unlock()
	r.publishReady(token, eventEpoch)
}

func (r *integratedSecretRouter) publishReady(token SecretReadSourceToken, epoch uint64) {
	r.serializeEvent(func() {
		r.mu.Lock()
		current := token != "" && r.token == token && r.routeEpoch == epoch && r.client != nil && !r.closed && !r.quiesced
		r.mu.Unlock()
		if current && r.deps.ready != nil {
			r.deps.ready(integratedSecretSourceSignal{SourceToken: token})
		}
	})
}

func (r *integratedSecretRouter) serializeEvent(emit func()) {
	r.executeReservedEvent(r.reserveEvent(), emit)
}

type integratedSecretEventReservation struct {
	previous <-chan struct{}
	done     chan struct{}
}

func (r *integratedSecretRouter) reserveEvent() integratedSecretEventReservation {
	r.eventMu.Lock()
	previous := r.sourceEventTail
	done := make(chan struct{})
	r.sourceEventTail = done
	r.eventMu.Unlock()
	return integratedSecretEventReservation{previous: previous, done: done}
}

func (r *integratedSecretRouter) executeReservedEvent(reservation integratedSecretEventReservation, emit func()) {
	if reservation.previous == nil || reservation.done == nil {
		return
	}
	<-reservation.previous
	if emit != nil {
		emit()
	}
	close(reservation.done)
}

type integratedSecretCleanup struct {
	token                  SecretReadSourceToken
	epoch                  uint64
	unavailableReservation integratedSecretEventReservation
	operationCancels       []context.CancelFunc
	watches                []*secretWatchRouteOwner
	client                 acceleratorprovision.SecretRPCClient
	session                secretRouterSessionLease
	demand                 secretRouterDemandLease
	demandCancel           context.CancelFunc
}

func (r *integratedSecretRouter) detachLocked(includeDemand bool) integratedSecretCleanup {
	cleanup := integratedSecretCleanup{token: r.token, epoch: r.routeEpoch, client: r.client, session: r.session}
	if cleanup.token != "" {
		// Reserve the old source's exact lane position while admission is still
		// fenced by r.mu. A replacement ready event can only reserve behind it.
		cleanup.unavailableReservation = r.reserveEvent()
	}
	for _, cancel := range r.operations {
		cleanup.operationCancels = append(cleanup.operationCancels, cancel)
	}
	for _, owner := range r.watches {
		cleanup.watches = append(cleanup.watches, owner)
	}
	r.routeEpoch++
	r.token = ""
	r.client = nil
	r.session = nil
	r.sessionIdentity = secretRouterSessionIdentity{}
	r.operations = make(map[uint64]context.CancelFunc)
	r.lists = make(map[string]*secretListRouteOwner)
	r.watches = make(map[acceleratorsecret.SecretWatchSpecID]*secretWatchRouteOwner)
	if includeDemand {
		cleanup.demand = r.demand
		cleanup.demandCancel = r.demandCancel
		r.demand = nil
		r.demandCancel = nil
	}
	return cleanup
}

func (r *integratedSecretRouter) runCleanup(cleanup integratedSecretCleanup) {
	if cleanup.token != "" {
		r.executeReservedEvent(cleanup.unavailableReservation, func() {
			r.mu.Lock()
			detached := r.routeEpoch > cleanup.epoch && r.token != cleanup.token
			r.mu.Unlock()
			if detached && r.deps.unavailable != nil {
				r.deps.unavailable(integratedSecretSourceSignal{SourceToken: cleanup.token})
			}
		})
	}
	for _, cancel := range cleanup.operationCancels {
		cancel()
	}
	for _, owner := range cleanup.watches {
		owner.stopOnce.Do(func() { close(owner.stop) })
	}
	// Closing the client first terminates every local call and subscription and
	// only emits nonblocking best-effort remote cleanup. SecretWatchLease.Close
	// is consequently an idempotent local no-op instead of a teardown timeout.
	if cleanup.client != nil {
		cleanup.client.Close(context.Background())
	}
	for _, owner := range cleanup.watches {
		owner.lease.Close(context.Background())
	}
	if cleanup.session != nil {
		cleanup.session.Close()
	}
	if cleanup.demandCancel != nil {
		cleanup.demandCancel()
	}
	if cleanup.demand != nil {
		cleanup.demand.Close()
	}
}

func (r *integratedSecretRouter) retireCurrent(identity secretRouterSessionIdentity) {
	if r == nil || !identity.valid() {
		return
	}
	r.mu.Lock()
	if r.client == nil || r.sessionIdentity != identity {
		r.mu.Unlock()
		return
	}
	r.rejectedIdentity = identity
	cleanup := r.detachLocked(false)
	r.mu.Unlock()
	r.runCleanup(cleanup)
}

func (r *integratedSecretRouter) FenceContextSwitch(string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.contextSwitching = true
	r.contextEpoch++
	r.demandEpoch++
	cleanup := r.detachLocked(true)
	r.contextName = ""
	r.rejectedIdentity = secretRouterSessionIdentity{}
	r.mu.Unlock()
	r.runCleanup(cleanup)
}

func (r *integratedSecretRouter) ContextSwitched(contextName string, _ bool) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.contextSwitching = false
	r.rejectedIdentity = secretRouterSessionIdentity{}
	r.contextName = contextName
	ctx, epoch, start := r.prepareDemandLocked(contextName)
	r.mu.Unlock()
	if start {
		go r.runDemand(ctx, contextName, epoch)
	}
}

func (r *integratedSecretRouter) Quiesce(context.Context) {
	if r == nil {
		return
	}
	r.mu.Lock()
	if r.quiesced {
		r.mu.Unlock()
		return
	}
	r.quiesced = true
	r.demandEpoch++
	cleanup := r.detachLocked(true)
	r.mu.Unlock()
	r.runCleanup(cleanup)
}

func (r *integratedSecretRouter) StopProducers(ctx context.Context) {
	if r == nil {
		return
	}
	done := make(chan struct{})
	go func() {
		r.monitors.Wait()
		close(done)
	}()
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-done:
	case <-ctx.Done():
	}
}

func (r *integratedSecretRouter) Close(ctx context.Context) {
	if r == nil {
		return
	}
	r.Quiesce(ctx)
	r.StopProducers(ctx)
	r.mu.Lock()
	r.closed = true
	r.consumers = 0
	r.contextName = ""
	r.contextSwitching = false
	r.rejectedIdentity = secretRouterSessionIdentity{}
	r.mu.Unlock()
}
