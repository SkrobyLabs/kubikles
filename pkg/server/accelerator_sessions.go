package server

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"kubikles/pkg/agent"
)

const (
	AcceleratorSocketQueueSize          = 64
	AcceleratorSocketWriteTimeout       = 10 * time.Second
	AcceleratorSocketReadLimit    int64 = 64 << 10
	AcceleratorSocketPongTimeout        = 60 * time.Second
	AcceleratorSocketPingInterval       = 30 * time.Second
	AcceleratorSocketDrainTimeout       = 2 * time.Second
)

type AcceleratorSocketGeneration uint64

// AcceleratorSessionLease exposes only the generation fence needed by
// Accelerator-owned producers. It is not an RPC-facing identity API.
type AcceleratorSessionLease struct {
	Generation AcceleratorSocketGeneration
	Connected  bool
}

// AcceleratorSessionTarget is an internal exact-generation enqueue target.
type AcceleratorSessionTarget struct {
	SessionID  agent.SessionID
	Generation AcceleratorSocketGeneration
}

// AcceleratorSessionSnapshot is an immutable observation of one socket generation.
// It deliberately contains only server-issued identity and continuity information.
type AcceleratorSessionSnapshot struct {
	CallContext agent.AuthenticatedCallContext
	Generation  AcceleratorSocketGeneration
	Resumed     bool
}

type AcceleratorSessionObserver interface {
	SessionConnected(AcceleratorSessionSnapshot)
	SessionDisconnected(AcceleratorSessionSnapshot)
	SessionRevoked(AcceleratorSessionSnapshot)
}

type NoopAcceleratorSessionObserver struct{}

func (NoopAcceleratorSessionObserver) SessionConnected(AcceleratorSessionSnapshot)    {}
func (NoopAcceleratorSessionObserver) SessionDisconnected(AcceleratorSessionSnapshot) {}
func (NoopAcceleratorSessionObserver) SessionRevoked(AcceleratorSessionSnapshot)      {}

type AcceleratorConnectedEvent struct {
	SessionID  agent.SessionID             `json:"sessionId"`
	InstanceID string                      `json:"instanceId"`
	Generation AcceleratorSocketGeneration `json:"generation"`
	Resumed    bool                        `json:"resumed"`
}

type acceleratorSocketConnection interface {
	SetWriteDeadline(time.Time) error
	WriteMessage(int, []byte) error
	WriteControl(int, []byte, time.Time) error
	SetReadLimit(int64)
	SetReadDeadline(time.Time) error
	SetPongHandler(func(string) error)
	ReadMessage() (int, []byte, error)
	Close() error
}

type acceleratorSocketConfig struct {
	writeTimeout time.Duration
	pongTimeout  time.Duration
	pingInterval time.Duration
	drainTimeout time.Duration
	now          func() time.Time
	logf         func(string, ...interface{})
}

func defaultAcceleratorSocketConfig() acceleratorSocketConfig {
	return acceleratorSocketConfig{
		writeTimeout: AcceleratorSocketWriteTimeout,
		pongTimeout:  AcceleratorSocketPongTimeout,
		pingInterval: AcceleratorSocketPingInterval,
		drainTimeout: AcceleratorSocketDrainTimeout,
		now:          time.Now,
		logf:         log.Printf,
	}
}

type acceleratorSocketClose struct {
	code   int
	reason string
	drain  bool
	cause  error
}

type acceleratorSocket struct {
	conn            acceleratorSocketConnection
	queue           chan *acceleratorOutboundEvent
	startCh         chan struct{}
	readerStartCh   chan struct{}
	bootstrapDone   chan bool
	activationDone  chan struct{}
	closeCh         chan acceleratorSocketClose
	pumpsDone       chan struct{}
	startOnce       sync.Once
	readerStartOnce sync.Once
	activationOnce  sync.Once
	networkOnce     sync.Once
	hardCloseOnce   sync.Once
	wg              sync.WaitGroup

	enqueueMu          sync.Mutex
	activationMu       sync.Mutex
	accepting          bool
	eligible           atomic.Bool
	activationStarted  atomic.Bool
	activationComplete atomic.Bool
	ready              atomic.Bool
	observed           atomic.Bool
	cancelled          atomic.Bool
	terminationMu      sync.Mutex
	terminationReason  string
	terminationError   string

	registry *AcceleratorSessionRegistry
	snapshot AcceleratorSessionSnapshot
	rpc      *acceleratorRPCConnection
	config   acceleratorSocketConfig
}

type acceleratorOutboundEvent struct {
	event     Event
	once      sync.Once
	payload   []byte
	sensitive bool
	err       error
}

func (e *acceleratorOutboundEvent) marshal() ([]byte, error) {
	e.once.Do(func() {
		if e.payload == nil {
			e.payload, e.err = json.Marshal(e.event)
		}
		if e.err != nil {
			log.Print("Accelerator WebSocket outbound event serialization failed")
		}
	})
	return e.payload, e.err
}

func (e *acceleratorOutboundEvent) clear() {
	if e != nil && e.sensitive {
		clear(e.payload)
		e.payload = nil
	}
}

func newAcceleratorSocket(r *AcceleratorSessionRegistry, conn acceleratorSocketConnection, snapshot AcceleratorSessionSnapshot, dispatcher *AcceleratorRPCDispatcher) *acceleratorSocket {
	s := &acceleratorSocket{
		conn:           conn,
		queue:          make(chan *acceleratorOutboundEvent, AcceleratorSocketQueueSize),
		startCh:        make(chan struct{}),
		readerStartCh:  make(chan struct{}),
		bootstrapDone:  make(chan bool, 1),
		activationDone: make(chan struct{}),
		closeCh:        make(chan acceleratorSocketClose, 1),
		pumpsDone:      make(chan struct{}),
		accepting:      true,
		registry:       r,
		snapshot:       snapshot,
		config:         r.config,
	}
	if dispatcher != nil {
		s.rpc = dispatcher.attachWithLogger(snapshot, s.enqueueResult, s.pumpsDone, r.config.logf)
	}
	s.queue <- &acceleratorOutboundEvent{event: Event{Type: "event", Name: "connected", Data: AcceleratorConnectedEvent{
		SessionID: snapshot.CallContext.SessionID, InstanceID: r.instanceID,
		Generation: snapshot.Generation, Resumed: snapshot.Resumed,
	}}}
	// Pump accounting and goroutines are established before the socket is
	// published in the registry. Both pumps remain paused until activation.
	s.wg.Add(2)
	go s.writer()
	go s.reader()
	go func() {
		s.wg.Wait()
		<-s.activationDone
		close(s.pumpsDone)
		r.socketFinished(s)
	}()
	return s
}

func (s *acceleratorSocket) activate()    { s.startOnce.Do(func() { close(s.startCh) }) }
func (s *acceleratorSocket) startReader() { s.readerStartOnce.Do(func() { close(s.readerStartCh) }) }
func (s *acceleratorSocket) finishActivation() {
	s.activationOnce.Do(func() {
		s.activationComplete.Store(true)
		close(s.activationDone)
	})
}
func (s *acceleratorSocket) settleUnstartedActivation() {
	s.activationMu.Lock()
	if s.activationStarted.CompareAndSwap(false, true) {
		s.finishActivation()
	}
	s.activationMu.Unlock()
}
func (s *acceleratorSocket) closeNetwork() {
	s.networkOnce.Do(func() { _ = s.conn.Close() })
}

func (s *acceleratorSocket) recordTermination(reason string, err error) {
	if reason == "" {
		reason = "connection ended"
	}
	s.terminationMu.Lock()
	if s.terminationReason == "" {
		s.terminationReason = reason
		var marshalErr *json.MarshalerError
		if err != nil && !errors.As(err, &marshalErr) {
			s.terminationError = err.Error()
		}
	}
	s.terminationMu.Unlock()
}

func (s *acceleratorSocket) terminationDetails() (string, string) {
	s.terminationMu.Lock()
	defer s.terminationMu.Unlock()
	return s.terminationReason, s.terminationError
}

// enqueue never performs network I/O and never waits for a writer. The first
// overflow atomically closes the producer side and wakes the writer, which owns
// the best-effort 1013 close frame.
func (s *acceleratorSocket) enqueue(event Event) bool {
	return s.enqueueOutbound(&acceleratorOutboundEvent{event: event})
}

func (s *acceleratorSocket) enqueueResult(payload []byte) bool {
	outbound := &acceleratorOutboundEvent{payload: payload, sensitive: true}
	if s.enqueueOutbound(outbound) {
		return true
	}
	outbound.clear()
	return false
}

func (s *acceleratorSocket) enqueueOutbound(event *acceleratorOutboundEvent) bool {
	s.enqueueMu.Lock()
	if !s.accepting {
		s.enqueueMu.Unlock()
		return false
	}
	select {
	case s.queue <- event:
		s.enqueueMu.Unlock()
		return true
	default:
		s.accepting = false
		s.enqueueMu.Unlock()
		s.signalClose(acceleratorSocketClose{code: websocket.CloseTryAgainLater, reason: "slow consumer"})
		return false
	}
}

func (s *acceleratorSocket) requestClose(close acceleratorSocketClose) {
	s.eligible.Store(false)
	if s.rpc != nil {
		s.rpc.fail(errAcceleratorRPCUnavailable)
	}
	s.enqueueMu.Lock()
	if s.accepting {
		s.accepting = false
	}
	s.enqueueMu.Unlock()
	pending := !s.ready.Load()
	if pending {
		s.cancelled.Store(true)
	}
	s.signalClose(close)
	if pending {
		// A pending socket has no application frames to drain. Hard-closing it is
		// what guarantees that a post-101 revoke cannot leak connected.
		s.closeNetwork()
	}
}

func (s *acceleratorSocket) signalClose(close acceleratorSocketClose) {
	s.recordTermination(close.reason, close.cause)
	s.settleUnstartedActivation()
	select {
	case s.closeCh <- close:
		s.hardCloseOnce.Do(func() { go s.hardCloseAfter(s.config.drainTimeout) })
	default:
	}
	s.activate()
	s.startReader()
}

func (s *acceleratorSocket) hardCloseAfter(timeout time.Duration) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-timer.C:
		s.closeNetwork()
	case <-s.pumpsDone:
	}
}

func (s *acceleratorSocket) writer() {
	defer s.wg.Done()
	defer s.clearPendingOutbound()
	<-s.startCh
	if s.cancelled.Load() {
		s.bootstrapDone <- false
		return
	}
	connected := <-s.queue
	if err := s.conn.SetWriteDeadline(s.config.now().Add(s.config.writeTimeout)); err != nil {
		connected.clear()
		s.fenceWriterFailure("connected frame write deadline failed", err)
		s.bootstrapDone <- false
		return
	}
	if err := s.writeEvent(connected); err != nil {
		s.fenceWriterFailure("connected frame write failed", err)
		s.bootstrapDone <- false
		return
	}
	s.bootstrapDone <- true
	ping := time.NewTicker(s.config.pingInterval)
	defer ping.Stop()
	defer s.closeNetwork()
	for {
		// Prefer shutdown over another queued event after a close request.
		select {
		case close := <-s.closeCh:
			s.shutdownWriter(close)
			return
		default:
		}
		select {
		case close := <-s.closeCh:
			s.shutdownWriter(close)
			return
		case event := <-s.queue:
			if err := s.conn.SetWriteDeadline(s.config.now().Add(s.config.writeTimeout)); err != nil {
				event.clear()
				s.fenceWriterFailure("event write deadline failed", err)
				return
			}
			if err := s.writeEvent(event); err != nil {
				s.fenceWriterFailure("event write failed", err)
				return
			}
		case <-ping.C:
			deadline := s.config.now().Add(s.config.writeTimeout)
			if err := s.conn.SetWriteDeadline(deadline); err != nil {
				s.fenceWriterFailure("ping write deadline failed", err)
				return
			}
			if err := s.conn.WriteControl(websocket.PingMessage, nil, deadline); err != nil {
				s.fenceWriterFailure("ping write failed", err)
				return
			}
		}
	}
}

// fenceWriterFailure closes every producer-side admission gate before the
// writer drains sensitive ownership or begins its bounded network close.
func (s *acceleratorSocket) fenceWriterFailure(reason string, err error) {
	s.recordTermination(reason, err)
	s.eligible.Store(false)
	if s.rpc != nil {
		s.rpc.fail(errAcceleratorRPCUnavailable)
	}
	s.enqueueMu.Lock()
	s.accepting = false
	s.enqueueMu.Unlock()
	s.clearPendingOutbound()
}

func (s *acceleratorSocket) clearPendingOutbound() {
	for {
		select {
		case outbound := <-s.queue:
			outbound.clear()
		default:
			return
		}
	}
}

func (s *acceleratorSocket) shutdownWriter(close acceleratorSocketClose) {
	if close.drain {
		deadline := s.config.now().Add(s.config.drainTimeout)
		for s.config.now().Before(deadline) {
			select {
			case event := <-s.queue:
				writeDeadline := s.config.now().Add(s.config.writeTimeout)
				if writeDeadline.After(deadline) {
					writeDeadline = deadline
				}
				if s.conn.SetWriteDeadline(writeDeadline) != nil || s.writeEvent(event) != nil {
					return
				}
			default:
				goto drained
			}
		}
	}
drained:
	deadline := s.config.now().Add(s.config.writeTimeout)
	if close.drain {
		drainDeadline := s.config.now().Add(s.config.drainTimeout)
		if drainDeadline.Before(deadline) {
			deadline = drainDeadline
		}
	}
	_ = s.conn.SetWriteDeadline(deadline)
	_ = s.conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(close.code, close.reason), deadline)
}

func (s *acceleratorSocket) writeEvent(event *acceleratorOutboundEvent) error {
	defer event.clear()
	payload, err := event.marshal()
	if err != nil {
		return err
	}
	return s.conn.WriteMessage(websocket.TextMessage, payload)
}

func (s *acceleratorSocket) reader() {
	defer s.wg.Done()
	<-s.readerStartCh
	s.conn.SetReadLimit(AcceleratorSocketReadLimit)
	if err := s.conn.SetReadDeadline(s.config.now().Add(s.config.pongTimeout)); err != nil {
		s.requestClose(acceleratorSocketClose{code: websocket.CloseGoingAway, reason: "read deadline setup failed", cause: err})
		return
	}
	s.conn.SetPongHandler(func(string) error {
		return s.conn.SetReadDeadline(s.config.now().Add(s.config.pongTimeout))
	})
	for {
		messageType, payload, err := s.conn.ReadMessage()
		if err != nil {
			s.requestClose(acceleratorSocketClose{code: websocket.CloseGoingAway, reason: "connection read failed", cause: err})
			return
		}
		if messageType != websocket.TextMessage || s.rpc == nil || !s.rpc.handle(payload) {
			cause := errAcceleratorRPCProtocol
			if s.rpc != nil && s.rpc.err() != nil {
				cause = s.rpc.err()
			}
			s.requestClose(acceleratorSocketClose{code: websocket.ClosePolicyViolation, reason: "protocol error", cause: cause})
			return
		}
	}
}

type acceleratorSessionRecord struct {
	call         agent.AuthenticatedCallContext
	generation   AcceleratorSocketGeneration
	ever         bool
	everObserved bool
	lastSnapshot AcceleratorSessionSnapshot
	lastObserved AcceleratorSessionSnapshot
	callbackTail chan struct{}
	socket       *acceleratorSocket
}

type AcceleratorSessionRegistry struct {
	mu       sync.Mutex
	records  map[agent.SessionID]*acceleratorSessionRecord
	observer AcceleratorSessionObserver

	// callbackStateMu accounts committed callback batches for Close. It is
	// never held while invoking an observer.
	callbackStateMu      sync.Mutex
	outstandingCallbacks int
	callbacksDone        chan struct{}
	admissionMu          sync.RWMutex
	accepting            atomic.Bool

	instanceID    string
	closed        bool
	activeSockets int
	activeDone    chan struct{}
	config        acceleratorSocketConfig
	quiesceOnce   sync.Once
	quiesceDone   chan struct{}
}

type acceleratorObservation struct {
	kind     uint8
	snapshot AcceleratorSessionSnapshot
}

type acceleratorCallbackBatch struct {
	observations []acceleratorObservation
	previous     <-chan struct{}
	done         chan struct{}
}

const (
	acceleratorObservedConnect uint8 = iota + 1
	acceleratorObservedDisconnect
	acceleratorObservedRevoke
)

// reserveCallbackBatchLocked is called while the transition is committed
// under r.mu. This prevents Close from observing a gap between state mutation
// and callback accounting.
func (r *AcceleratorSessionRegistry) reserveCallbackBatchLocked(record *acceleratorSessionRecord, observations ...acceleratorObservation) *acceleratorCallbackBatch {
	if len(observations) == 0 {
		return nil
	}
	batch := &acceleratorCallbackBatch{observations: append([]acceleratorObservation(nil), observations...), previous: record.callbackTail, done: make(chan struct{})}
	record.callbackTail = batch.done
	r.callbackStateMu.Lock()
	if r.outstandingCallbacks == 0 {
		r.callbacksDone = make(chan struct{})
	}
	r.outstandingCallbacks++
	r.callbackStateMu.Unlock()
	return batch
}

func (r *AcceleratorSessionRegistry) invokeCallbackBatch(batch *acceleratorCallbackBatch) {
	if batch == nil {
		return
	}
	<-batch.previous
	defer func() {
		close(batch.done)
		r.callbackStateMu.Lock()
		r.outstandingCallbacks--
		if r.outstandingCallbacks == 0 {
			close(r.callbacksDone)
		}
		r.callbackStateMu.Unlock()
	}()
	for _, observation := range batch.observations {
		switch observation.kind {
		case acceleratorObservedConnect:
			r.observer.SessionConnected(observation.snapshot)
		case acceleratorObservedDisconnect:
			r.observer.SessionDisconnected(observation.snapshot)
		case acceleratorObservedRevoke:
			r.observer.SessionRevoked(observation.snapshot)
		}
	}
}

func NewAcceleratorSessionRegistry(instanceID string, observer AcceleratorSessionObserver) *AcceleratorSessionRegistry {
	return newAcceleratorSessionRegistry(instanceID, observer, defaultAcceleratorSocketConfig())
}

func newAcceleratorSessionRegistry(instanceID string, observer AcceleratorSessionObserver, config acceleratorSocketConfig) *AcceleratorSessionRegistry {
	if observer == nil {
		observer = NoopAcceleratorSessionObserver{}
	}
	if config.now == nil {
		config.now = time.Now
	}
	if config.logf == nil {
		config.logf = func(string, ...interface{}) {}
	}
	done := make(chan struct{})
	close(done)
	callbacksDone := make(chan struct{})
	close(callbacksDone)
	r := &AcceleratorSessionRegistry{
		records: make(map[agent.SessionID]*acceleratorSessionRecord), observer: observer,
		instanceID: instanceID, activeDone: done, callbacksDone: callbacksDone, config: config, quiesceDone: make(chan struct{}),
	}
	r.accepting.Store(true)
	return r
}

func (r *AcceleratorSessionRegistry) beginUpgrade() (func(), bool) {
	if !r.accepting.Load() {
		return nil, false
	}
	r.admissionMu.RLock()
	if !r.accepting.Load() {
		r.admissionMu.RUnlock()
		return nil, false
	}
	return r.admissionMu.RUnlock, true
}

func (r *AcceleratorSessionRegistry) socketStartedLocked() {
	if r.activeSockets == 0 {
		r.activeDone = make(chan struct{})
	}
	r.activeSockets++
}

func (r *AcceleratorSessionRegistry) socketEnded() {
	r.mu.Lock()
	r.activeSockets--
	if r.activeSockets == 0 {
		close(r.activeDone)
	}
	r.mu.Unlock()
}

type acceleratorRegistration struct {
	socket           *acceleratorSocket
	old              *acceleratorSocket
	replacementBatch *acceleratorCallbackBatch
	connectedBatch   *acceleratorCallbackBatch
}

func (r *AcceleratorSessionRegistry) prepareRegistration(call agent.AuthenticatedCallContext, conn acceleratorSocketConnection) (*acceleratorRegistration, bool) {
	return r.prepareRegistrationWithHookAndDispatcher(call, conn, nil, nil)
}

func (r *AcceleratorSessionRegistry) prepareRegistrationWithHook(call agent.AuthenticatedCallContext, conn acceleratorSocketConnection, prepared func(AcceleratorSocketGeneration)) (*acceleratorRegistration, bool) {
	return r.prepareRegistrationWithHookAndDispatcher(call, conn, prepared, nil)
}

func (r *AcceleratorSessionRegistry) prepareRPCRegistration(call agent.AuthenticatedCallContext, conn acceleratorSocketConnection, dispatcher *AcceleratorRPCDispatcher) (*acceleratorRegistration, bool) {
	return r.prepareRegistrationWithHookAndDispatcher(call, conn, nil, dispatcher)
}

func (r *AcceleratorSessionRegistry) prepareRegistrationWithHookAndDispatcher(call agent.AuthenticatedCallContext, conn acceleratorSocketConnection, prepared func(AcceleratorSocketGeneration), dispatcher *AcceleratorRPCDispatcher) (*acceleratorRegistration, bool) {
	if !call.IsAuthenticated() || conn == nil {
		return nil, false
	}
	for {
		r.mu.Lock()
		if r.closed {
			r.mu.Unlock()
			return nil, false
		}
		record := r.records[call.SessionID]
		if record == nil {
			callbackTail := make(chan struct{})
			close(callbackTail)
			record = &acceleratorSessionRecord{call: call, callbackTail: callbackTail}
			r.records[call.SessionID] = record
		} else if record.call != call || uint64(record.generation) == math.MaxUint64 {
			// A forged identity collision or generation wrap can never take over the
			// retained session. The record remains latched and unchanged.
			r.mu.Unlock()
			return nil, false
		}
		old := record.socket
		if old != nil && old.activationStarted.Load() && !old.activationComplete.Load() {
			activationDone := old.activationDone
			r.mu.Unlock()
			<-activationDone
			continue
		}
		record.generation++
		snapshot := AcceleratorSessionSnapshot{CallContext: record.call, Generation: record.generation, Resumed: record.ever}
		record.ever = true
		record.lastSnapshot = snapshot
		r.socketStartedLocked()
		socket := newAcceleratorSocket(r, conn, snapshot, dispatcher)
		record.socket = socket
		registration := &acceleratorRegistration{socket: socket, old: old}
		if old != nil && old.observed.Load() {
			registration.replacementBatch = r.reserveCallbackBatchLocked(record, acceleratorObservation{kind: acceleratorObservedDisconnect, snapshot: old.snapshot})
		}
		r.mu.Unlock()
		if prepared != nil {
			prepared(socket.snapshot.Generation)
		}

		if old != nil {
			old.requestClose(acceleratorSocketClose{code: websocket.CloseGoingAway, reason: "replaced"})
			<-old.pumpsDone
		}
		return registration, true
	}
}

func (r *AcceleratorSessionRegistry) observeReplacement(registration *acceleratorRegistration) {
	if registration != nil {
		r.invokeCallbackBatch(registration.replacementBatch)
		registration.replacementBatch = nil
	}
}

// beginRegistrationActivation commits the exact generation and callback batch
// without invoking external observers. Registration calls this while its own
// state mutex supplies the final revocation linearization.
func (r *AcceleratorSessionRegistry) beginRegistrationActivation(registration *acceleratorRegistration) bool {
	if registration == nil || registration.socket == nil {
		return false
	}
	socket := registration.socket
	r.mu.Lock()
	record := r.records[socket.snapshot.CallContext.SessionID]
	current := !r.closed && record != nil && record.socket == socket && record.generation == socket.snapshot.Generation && !socket.cancelled.Load()
	socket.activationMu.Lock()
	if !current {
		socket.activationMu.Unlock()
		r.mu.Unlock()
		socket.requestClose(acceleratorSocketClose{code: websocket.CloseGoingAway, reason: "connection closed"})
		return false
	}
	if !socket.activationStarted.CompareAndSwap(false, true) {
		socket.activationMu.Unlock()
		r.mu.Unlock()
		return false
	}
	// Commit the observation while the exact socket is current, but invoke it
	// outside registry locks. The writer remains behind startCh until the
	// observer (including generation-owned producers) has fully caught up.
	record.everObserved = true
	record.lastObserved = socket.snapshot
	registration.connectedBatch = r.reserveCallbackBatchLocked(record, acceleratorObservation{kind: acceleratorObservedConnect, snapshot: socket.snapshot})
	socket.activationMu.Unlock()
	r.mu.Unlock()
	return true
}

// finishRegistrationActivation invokes the committed observer outside every
// registry/browser/admission lock, then publishes ready+eligible before the
// writer is allowed to expose the mandatory connected frame.
func (r *AcceleratorSessionRegistry) finishRegistrationActivation(registration *acceleratorRegistration) (*acceleratorSocket, bool) {
	socket := registration.socket
	r.observeConnected(registration)
	socket.observed.Store(true)

	r.mu.Lock()
	record := r.records[socket.snapshot.CallContext.SessionID]
	current := !r.closed && record != nil && record.socket == socket && record.generation == socket.snapshot.Generation && !socket.cancelled.Load()
	if current {
		// Readiness and eligibility become visible only after the observer is
		// current. connected already occupies queue slot zero, so an event queued
		// in this interval remains strictly behind the handshake frame.
		socket.ready.Store(true)
		socket.eligible.Store(true)
	}
	r.mu.Unlock()
	if !current {
		socket.activationMu.Lock()
		socket.finishActivation()
		socket.activationMu.Unlock()
		socket.requestClose(acceleratorSocketClose{code: websocket.CloseGoingAway, reason: "connection closed"})
		return nil, false
	}

	socket.activate()
	bootstrapped := <-socket.bootstrapDone
	r.mu.Lock()
	record = r.records[socket.snapshot.CallContext.SessionID]
	current = bootstrapped && !r.closed && record != nil && record.socket == socket && record.generation == socket.snapshot.Generation
	if !current {
		socket.ready.Store(false)
		socket.eligible.Store(false)
	}
	socket.activationMu.Lock()
	socket.finishActivation()
	socket.activationMu.Unlock()
	r.mu.Unlock()
	socket.startReader()
	if !current {
		socket.requestClose(acceleratorSocketClose{code: websocket.CloseGoingAway, reason: "connection closed"})
		return nil, false
	}
	r.config.logf("Accelerator connection established session=%q generation=%d resumed=%t", socket.snapshot.CallContext.SessionID, socket.snapshot.Generation, socket.snapshot.Resumed)
	return socket, true
}

func (r *AcceleratorSessionRegistry) activateRegistration(registration *acceleratorRegistration) (*acceleratorSocket, bool) {
	if !r.beginRegistrationActivation(registration) {
		return nil, false
	}
	return r.finishRegistrationActivation(registration)
}

func (r *AcceleratorSessionRegistry) observeConnected(registration *acceleratorRegistration) {
	if registration != nil {
		r.invokeCallbackBatch(registration.connectedBatch)
		registration.connectedBatch = nil
	}
}

func (r *AcceleratorSessionRegistry) completeRegistration(registration *acceleratorRegistration) (*acceleratorSocket, bool) {
	r.observeReplacement(registration)
	socket, ok := r.activateRegistration(registration)
	if !ok {
		return nil, false
	}
	r.observeConnected(registration)
	return socket, true
}

func (r *AcceleratorSessionRegistry) register(call agent.AuthenticatedCallContext, conn acceleratorSocketConnection) (*acceleratorSocket, bool) {
	registration, ok := r.prepareRegistration(call, conn)
	if !ok {
		return nil, false
	}
	return r.completeRegistration(registration)
}

func (r *AcceleratorSessionRegistry) socketFinished(socket *acceleratorSocket) {
	r.mu.Lock()
	record := r.records[socket.snapshot.CallContext.SessionID]
	current := record != nil && record.socket == socket && record.generation == socket.snapshot.Generation
	if current {
		record.socket = nil
		if r.closed {
			delete(r.records, socket.snapshot.CallContext.SessionID)
		}
	}
	var callbackBatch *acceleratorCallbackBatch
	if current && socket.observed.Load() {
		callbackBatch = r.reserveCallbackBatchLocked(record, acceleratorObservation{kind: acceleratorObservedDisconnect, snapshot: socket.snapshot})
	}
	r.mu.Unlock()
	r.invokeCallbackBatch(callbackBatch)
	if socket.observed.Load() {
		reason, rawError := socket.terminationDetails()
		if rawError != "" {
			r.config.logf("Accelerator connection lost session=%q generation=%d reason=%q error=%q", socket.snapshot.CallContext.SessionID, socket.snapshot.Generation, reason, rawError)
		} else {
			r.config.logf("Accelerator connection lost session=%q generation=%d reason=%q", socket.snapshot.CallContext.SessionID, socket.snapshot.Generation, reason)
		}
	}
	r.socketEnded()
}

func (r *AcceleratorSessionRegistry) snapshot(id agent.SessionID) (AcceleratorSessionSnapshot, bool, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	record := r.records[id]
	if record == nil {
		return AcceleratorSessionSnapshot{}, false, false
	}
	return record.lastSnapshot, record.socket != nil, true
}

func (r *AcceleratorSessionRegistry) EmitEvent(name string, data interface{}) {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}
	sockets := make([]*acceleratorSocket, 0, len(r.records))
	for _, record := range r.records {
		if record.socket != nil && record.socket.eligible.Load() {
			sockets = append(sockets, record.socket)
		}
	}
	r.mu.Unlock()
	event := &acceleratorOutboundEvent{event: Event{Type: "event", Name: name, Data: data}}
	for _, socket := range sockets {
		_ = socket.enqueueOutbound(event)
	}
}

func (r *AcceleratorSessionRegistry) LookupSessionLease(id agent.SessionID) (AcceleratorSessionLease, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	record := r.records[id]
	if record == nil || r.closed {
		return AcceleratorSessionLease{}, false
	}
	return AcceleratorSessionLease{Generation: record.generation, Connected: record.socket != nil && record.socket.ready.Load() && record.socket.eligible.Load()}, true
}

// EmitEventToTargets performs the generation fence and queue publication under
// the registry mutex. Duplicate or stale targets are harmless drops.
func (r *AcceleratorSessionRegistry) EmitEventToTargets(targets []AcceleratorSessionTarget, event Event) {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}
	seen := make(map[agent.SessionID]struct{}, len(targets))
	sockets := make([]*acceleratorSocket, 0, len(targets))
	for _, target := range targets {
		if _, ok := seen[target.SessionID]; ok {
			continue
		}
		record := r.records[target.SessionID]
		if record != nil && record.generation == target.Generation && record.socket != nil && record.socket.ready.Load() && record.socket.eligible.Load() {
			seen[target.SessionID] = struct{}{}
			sockets = append(sockets, record.socket)
		}
	}
	outbound := &acceleratorOutboundEvent{event: event}
	for _, socket := range sockets {
		_ = socket.enqueueOutbound(outbound)
	}
	r.mu.Unlock()
}

func (r *AcceleratorSessionRegistry) RevokeSession(_ context.Context, id agent.SessionID) {
	for {
		r.mu.Lock()
		if r.closed {
			r.mu.Unlock()
			return
		}
		record := r.records[id]
		if record == nil {
			r.mu.Unlock()
			return
		}
		if record.socket != nil && record.socket.activationStarted.Load() && !record.socket.activationComplete.Load() {
			activationDone := record.socket.activationDone
			r.mu.Unlock()
			<-activationDone
			continue
		}
		delete(r.records, id)
		socket := record.socket
		var callbackBatch *acceleratorCallbackBatch
		if record.everObserved {
			callbackBatch = r.reserveCallbackBatchLocked(record, acceleratorObservation{kind: acceleratorObservedRevoke, snapshot: record.lastObserved})
		}
		r.mu.Unlock()
		if socket != nil {
			socket.requestClose(acceleratorSocketClose{code: websocket.CloseGoingAway, reason: "revoked"})
			<-socket.pumpsDone
		}
		r.invokeCallbackBatch(callbackBatch)
		return
	}
}

func (r *AcceleratorSessionRegistry) Quiesce() {
	r.quiesceOnce.Do(func() {
		r.accepting.Store(false)
		// Once the write lock is held, every admission that could produce a 101
		// response has completed registration, and no later admission can begin.
		r.admissionMu.Lock()
		r.mu.Lock()
		r.closed = true
		sockets := make([]*acceleratorSocket, 0, len(r.records))
		for id, record := range r.records {
			if record.socket == nil {
				delete(r.records, id)
				continue
			}
			sockets = append(sockets, record.socket)
		}
		r.mu.Unlock()
		r.admissionMu.Unlock()

		for _, socket := range sockets {
			socket.requestClose(acceleratorSocketClose{code: websocket.CloseGoingAway, reason: "quiescing", drain: true})
		}
		close(r.quiesceDone)
	})
	<-r.quiesceDone
}

func (r *AcceleratorSessionRegistry) Close(ctx context.Context) error {
	r.Quiesce()
	r.mu.Lock()
	activeDone := r.activeDone
	r.mu.Unlock()
	r.callbackStateMu.Lock()
	callbacksDone := r.callbacksDone
	r.callbackStateMu.Unlock()
	select {
	case <-activeDone:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case <-callbacksDone:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
