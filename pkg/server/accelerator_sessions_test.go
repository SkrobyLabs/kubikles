package server

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"kubikles/pkg/acceleratorsecret"
	"kubikles/pkg/agent"
)

type failingAcceleratorJSON struct{}

func (failingAcceleratorJSON) MarshalJSON() ([]byte, error) {
	return nil, errors.New("credential-sentinel payload-sentinel")
}

type countedAcceleratorJSON struct{ marshals atomic.Int32 }

func (v *countedAcceleratorJSON) MarshalJSON() ([]byte, error) {
	v.marshals.Add(1)
	return []byte(`{"scope":"owners-only"}`), nil
}

type fakeAcceleratorRead struct {
	messageType int
	payload     []byte
	err         error
}

type fakeAcceleratorConn struct {
	mu                   sync.Mutex
	writes               []Event
	payloads             [][]byte
	controls             []int
	closeCodes           []int
	writeDeadlines       []time.Time
	readDeadlines        []time.Time
	readLimit            int64
	pong                 func(string) error
	writeErr             error
	controlErr           error
	deadlineErr          error
	enforceWriteDeadline bool
	enforceReadDeadline  bool
	blockWrite           chan struct{}
	blockControl         chan struct{}
	afterWrite           func(Event)
	read                 chan fakeAcceleratorRead
	readDeadlineChanged  chan struct{}
	closed               chan struct{}
	closeOnce            sync.Once
	closeEntered         chan struct{}
	closeEnteredOnce     sync.Once
	blockClose           <-chan struct{}
	writers              atomic.Int32
	maxWriters           atomic.Int32
}

func newFakeAcceleratorConn() *fakeAcceleratorConn {
	return &fakeAcceleratorConn{read: make(chan fakeAcceleratorRead, 8), readDeadlineChanged: make(chan struct{}), closed: make(chan struct{}), closeEntered: make(chan struct{})}
}

func (c *fakeAcceleratorConn) SetWriteDeadline(deadline time.Time) error {
	c.mu.Lock()
	c.writeDeadlines = append(c.writeDeadlines, deadline)
	err := c.deadlineErr
	c.mu.Unlock()
	return err
}

func (c *fakeAcceleratorConn) enterWriter() func() {
	current := c.writers.Add(1)
	for {
		old := c.maxWriters.Load()
		if current <= old || c.maxWriters.CompareAndSwap(old, current) {
			break
		}
	}
	return func() { c.writers.Add(-1) }
}

func (c *fakeAcceleratorConn) WriteMessage(messageType int, payload []byte) error {
	done := c.enterWriter()
	defer done()
	if c.blockWrite != nil {
		var deadline <-chan time.Time
		var timer *time.Timer
		if c.enforceWriteDeadline {
			c.mu.Lock()
			writeDeadline := c.writeDeadlines[len(c.writeDeadlines)-1]
			c.mu.Unlock()
			timer = time.NewTimer(time.Until(writeDeadline))
			deadline = timer.C
			defer timer.Stop()
		}
		select {
		case <-c.blockWrite:
		case <-c.closed:
			return errors.New("closed")
		case <-deadline:
			return errors.New("write deadline exceeded")
		}
	}
	c.mu.Lock()
	if c.writeErr != nil {
		err := c.writeErr
		c.mu.Unlock()
		return err
	}
	if messageType != websocket.TextMessage {
		c.mu.Unlock()
		return fmt.Errorf("unexpected message type %d", messageType)
	}
	var event Event
	if err := json.Unmarshal(payload, &event); err != nil {
		c.mu.Unlock()
		return err
	}
	c.writes = append(c.writes, event)
	c.payloads = append(c.payloads, append([]byte(nil), payload...))
	afterWrite := c.afterWrite
	c.mu.Unlock()
	if afterWrite != nil {
		afterWrite(event)
	}
	return nil
}

func (c *fakeAcceleratorConn) WriteControl(messageType int, payload []byte, _ time.Time) error {
	done := c.enterWriter()
	defer done()
	c.mu.Lock()
	c.controls = append(c.controls, messageType)
	if messageType == websocket.CloseMessage && len(payload) >= 2 {
		c.closeCodes = append(c.closeCodes, int(binary.BigEndian.Uint16(payload[:2])))
	}
	err := c.controlErr
	c.mu.Unlock()
	if c.blockControl != nil {
		select {
		case <-c.blockControl:
		case <-c.closed:
			return errors.New("closed")
		}
	}
	return err
}

func (c *fakeAcceleratorConn) SetReadLimit(limit int64) {
	c.mu.Lock()
	c.readLimit = limit
	c.mu.Unlock()
}

func (c *fakeAcceleratorConn) SetReadDeadline(deadline time.Time) error {
	c.mu.Lock()
	c.readDeadlines = append(c.readDeadlines, deadline)
	err := c.deadlineErr
	var changed chan struct{}
	if c.enforceReadDeadline {
		changed = c.readDeadlineChanged
		c.readDeadlineChanged = make(chan struct{})
	}
	c.mu.Unlock()
	if changed != nil {
		close(changed)
	}
	return err
}

func (c *fakeAcceleratorConn) SetPongHandler(handler func(string) error) {
	c.mu.Lock()
	c.pong = handler
	c.mu.Unlock()
}

func (c *fakeAcceleratorConn) ReadMessage() (int, []byte, error) {
	if c.enforceReadDeadline {
		for {
			c.mu.Lock()
			deadline := c.readDeadlines[len(c.readDeadlines)-1]
			changed := c.readDeadlineChanged
			c.mu.Unlock()
			timer := time.NewTimer(time.Until(deadline))
			select {
			case result := <-c.read:
				timer.Stop()
				return result.messageType, result.payload, result.err
			case <-c.closed:
				timer.Stop()
				return 0, nil, errors.New("closed")
			case <-timer.C:
				return 0, nil, errors.New("read deadline exceeded")
			case <-changed:
				timer.Stop()
			}
		}
	}
	select {
	case result := <-c.read:
		return result.messageType, result.payload, result.err
	case <-c.closed:
		return 0, nil, errors.New("closed")
	}
}

func (c *fakeAcceleratorConn) Close() error {
	c.closeEnteredOnce.Do(func() { close(c.closeEntered) })
	if c.blockClose != nil {
		<-c.blockClose
	}
	c.closeOnce.Do(func() { close(c.closed) })
	return nil
}

func (c *fakeAcceleratorConn) writeCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.writes)
}

func (c *fakeAcceleratorConn) events() []Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]Event(nil), c.writes...)
}

func (c *fakeAcceleratorConn) lastPayload() []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.payloads) == 0 {
		return nil
	}
	return append([]byte(nil), c.payloads[len(c.payloads)-1]...)
}

func (c *fakeAcceleratorConn) controlCount(messageType int) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	count := 0
	for _, got := range c.controls {
		if got == messageType {
			count++
		}
	}
	return count
}

func (c *fakeAcceleratorConn) hasCloseCode(code int) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, got := range c.closeCodes {
		if got == code {
			return true
		}
	}
	return false
}

type recordingAcceleratorObserver struct {
	mu      sync.Mutex
	events  []string
	snaps   []AcceleratorSessionSnapshot
	onEvent func(string, AcceleratorSessionSnapshot)
}

func (o *recordingAcceleratorObserver) record(kind string, snapshot AcceleratorSessionSnapshot) {
	o.mu.Lock()
	o.events = append(o.events, fmt.Sprintf("%s:%d", kind, snapshot.Generation))
	o.snaps = append(o.snaps, snapshot)
	hook := o.onEvent
	o.mu.Unlock()
	if hook != nil {
		hook(kind, snapshot)
	}
}

func (o *recordingAcceleratorObserver) SessionConnected(snapshot AcceleratorSessionSnapshot) {
	o.record("connect", snapshot)
}
func (o *recordingAcceleratorObserver) SessionDisconnected(snapshot AcceleratorSessionSnapshot) {
	o.record("disconnect", snapshot)
}
func (o *recordingAcceleratorObserver) SessionRevoked(snapshot AcceleratorSessionSnapshot) {
	o.record("revoke", snapshot)
}
func (o *recordingAcceleratorObserver) eventCopy() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]string(nil), o.events...)
}
func (o *recordingAcceleratorObserver) snapshotCopy() []AcceleratorSessionSnapshot {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]AcceleratorSessionSnapshot(nil), o.snaps...)
}
func (o *recordingAcceleratorObserver) eventsForSession(id agent.SessionID) []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	var events []string
	for index, snapshot := range o.snaps {
		if snapshot.CallContext.SessionID == id {
			events = append(events, o.events[index])
		}
	}
	return events
}

func acceleratorTestConfig() acceleratorSocketConfig {
	return acceleratorSocketConfig{
		writeTimeout: 100 * time.Millisecond,
		pongTimeout:  100 * time.Millisecond,
		pingInterval: 20 * time.Millisecond,
		drainTimeout: 75 * time.Millisecond,
		now:          time.Now,
	}
}

func acceleratorTestCall(suffix string) agent.AuthenticatedCallContext {
	return agent.AuthenticatedCallContext{PrincipalID: agent.PrincipalID("principal-" + suffix), SessionID: agent.SessionID("session-" + suffix)}
}

func waitAccelerator(t *testing.T, what string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func closeAcceleratorSocket(t *testing.T, socket *acceleratorSocket) {
	t.Helper()
	socket.requestClose(acceleratorSocketClose{code: websocket.CloseNormalClosure, reason: "test complete"})
	select {
	case <-socket.pumpsDone:
	case <-time.After(2 * time.Second):
		t.Fatal("socket pumps did not stop")
	}
}

func TestAcceleratorSocketConstants(t *testing.T) {
	if AcceleratorSocketQueueSize != 64 || AcceleratorSocketWriteTimeout != 10*time.Second || AcceleratorSocketReadLimit != 64<<10 || AcceleratorSocketPongTimeout != 60*time.Second || AcceleratorSocketPingInterval != 30*time.Second || AcceleratorSocketDrainTimeout != 2*time.Second {
		t.Fatal("bounded socket contract changed")
	}
}

func TestAcceleratorTargetedEventDelivery(t *testing.T) {
	registry := newAcceleratorSessionRegistry("instance", nil, acceleratorTestConfig())
	owner, ownerConn := acceleratorTestCall("owner"), newFakeAcceleratorConn()
	other, otherConn := acceleratorTestCall("other"), newFakeAcceleratorConn()
	ownerSocket, ok := registry.register(owner, ownerConn)
	if !ok {
		t.Fatal("owner register rejected")
	}
	otherSocket, ok := registry.register(other, otherConn)
	if !ok {
		t.Fatal("other register rejected")
	}
	waitAccelerator(t, "connected frames", func() bool { return ownerConn.writeCount() == 1 && otherConn.writeCount() == 1 })
	registry.EmitEventToTargets([]AcceleratorSessionTarget{{SessionID: owner.SessionID, Generation: ownerSocket.snapshot.Generation}, {SessionID: owner.SessionID, Generation: ownerSocket.snapshot.Generation}, {SessionID: other.SessionID, Generation: ownerSocket.snapshot.Generation + 1}}, Event{Type: "event", Name: "resource-event", Data: map[string]string{"safe": "summary"}})
	waitAccelerator(t, "owner targeted event", func() bool { return ownerConn.writeCount() == 2 })
	if otherConn.writeCount() != 1 {
		t.Fatalf("non-owner writes=%d", otherConn.writeCount())
	}
	_ = otherSocket
}

// T9: generation-qualified targeting is owner-only, survives a disconnected
// lease as an inactive record, and shares one serialized outbound frame.
func TestAcceleratorTargetedDeliveryGenerationFenceAndSharedFrame(t *testing.T) {
	registry := newAcceleratorSessionRegistry("instance", nil, acceleratorTestConfig())
	firstCall, firstConn := acceleratorTestCall("target-first"), newFakeAcceleratorConn()
	secondCall, secondConn := acceleratorTestCall("target-second"), newFakeAcceleratorConn()
	first, ok := registry.register(firstCall, firstConn)
	if !ok {
		t.Fatal("first owner register rejected")
	}
	second, ok := registry.register(secondCall, secondConn)
	if !ok {
		t.Fatal("second owner register rejected")
	}
	waitAccelerator(t, "connected frames before targeted delivery", func() bool {
		return firstConn.writeCount() == 1 && secondConn.writeCount() == 1
	})

	data := &countedAcceleratorJSON{}
	event := Event{Type: "event", Name: "generation-fenced", Data: data}
	registry.EmitEventToTargets([]AcceleratorSessionTarget{
		{SessionID: "missing", Generation: 1},
		{SessionID: firstCall.SessionID, Generation: first.snapshot.Generation},
		{SessionID: secondCall.SessionID, Generation: second.snapshot.Generation},
		{SessionID: firstCall.SessionID, Generation: first.snapshot.Generation},
	}, event)
	waitAccelerator(t, "both exact-generation owners", func() bool {
		return firstConn.writeCount() == 2 && secondConn.writeCount() == 2
	})
	if got := data.marshals.Load(); got != 1 {
		t.Fatalf("shared outbound marshal count = %d, want 1", got)
	}
	if !bytes.Equal(firstConn.lastPayload(), secondConn.lastPayload()) {
		t.Fatalf("owners received different payload bytes: %q != %q", firstConn.lastPayload(), secondConn.lastPayload())
	}
	for name, conn := range map[string]*fakeAcceleratorConn{"first": firstConn, "second": secondConn} {
		frames := conn.events()
		if len(frames) < 2 || frames[0].Name != "connected" || frames[1].Name != "generation-fenced" {
			t.Fatalf("%s frame ordering = %#v", name, frames)
		}
	}

	// Replacing an owner makes the old generation stale. A current target must
	// still be accepted regardless of whether its stale sibling comes first.
	currentConn := newFakeAcceleratorConn()
	current, ok := registry.register(firstCall, currentConn)
	if !ok {
		t.Fatal("replacement owner register rejected")
	}
	waitAccelerator(t, "replacement connected frame", func() bool { return currentConn.writeCount() == 1 })
	stale := AcceleratorSessionTarget{SessionID: firstCall.SessionID, Generation: first.snapshot.Generation}
	active := AcceleratorSessionTarget{SessionID: firstCall.SessionID, Generation: current.snapshot.Generation}
	registry.EmitEventToTargets([]AcceleratorSessionTarget{stale, active}, Event{Type: "event", Name: "stale-first"})
	waitAccelerator(t, "stale-first current delivery", func() bool { return currentConn.writeCount() == 2 })
	registry.EmitEventToTargets([]AcceleratorSessionTarget{active, stale}, Event{Type: "event", Name: "current-first"})
	waitAccelerator(t, "current-first current delivery", func() bool { return currentConn.writeCount() == 3 })
	if got := currentConn.events(); got[1].Name != "stale-first" || got[2].Name != "current-first" {
		t.Fatalf("replacement delivery frames = %#v", got)
	}
	if got := firstConn.writeCount(); got != 2 { // connected plus the pre-replacement targeted frame only
		t.Fatalf("stale owner received %d frames", got)
	}

	current.requestClose(acceleratorSocketClose{code: websocket.CloseNormalClosure, reason: "disconnect lease"})
	select {
	case <-current.pumpsDone:
	case <-time.After(2 * time.Second):
		t.Fatal("replacement pumps did not stop")
	}
	lease, found := registry.LookupSessionLease(firstCall.SessionID)
	if !found || lease.Generation != current.snapshot.Generation || lease.Connected {
		t.Fatalf("disconnected retained lease = %+v found=%v", lease, found)
	}
	before := currentConn.writeCount()
	registry.EmitEventToTargets([]AcceleratorSessionTarget{active}, Event{Type: "event", Name: "disconnected-drop"})
	if got := currentConn.writeCount(); got != before {
		t.Fatalf("disconnected generation received %d additional frames", got-before)
	}
	registry.RevokeBrowserSession(context.Background(), firstCall.SessionID)
	if _, found := registry.LookupSessionLease(firstCall.SessionID); found {
		t.Fatal("revoked lease remained visible")
	}
	registry.EmitEventToTargets([]AcceleratorSessionTarget{active}, Event{Type: "event", Name: "revoked-drop"})
	closeAcceleratorSocket(t, second)
}

// T9: a slow targeted owner cannot make a target producer block or prevent a
// healthy owner from receiving its exact-generation event; concurrent shutdown
// leaves target publication safe.
func TestAcceleratorTargetedDeliverySlowOwnerAndShutdownRace(t *testing.T) {
	registry := newAcceleratorSessionRegistry("instance", nil, acceleratorTestConfig())
	slowConn := newFakeAcceleratorConn()
	slowConn.blockWrite = make(chan struct{}, 1)
	slowConn.blockWrite <- struct{}{} // allow bootstrap, then block its writer
	slowCall := acceleratorTestCall("target-slow")
	slow, ok := registry.register(slowCall, slowConn)
	if !ok {
		t.Fatal("slow owner register rejected")
	}
	fastCall, fastConn := acceleratorTestCall("target-fast"), newFakeAcceleratorConn()
	fast, ok := registry.register(fastCall, fastConn)
	if !ok {
		t.Fatal("fast owner register rejected")
	}
	registry.EmitEventToTargets([]AcceleratorSessionTarget{{SessionID: slowCall.SessionID, Generation: slow.snapshot.Generation}}, Event{Type: "event", Name: "block"})
	waitAccelerator(t, "slow targeted writer blocked", func() bool { return slowConn.writers.Load() == 1 })
	start := time.Now()
	for i := 0; i < AcceleratorSocketQueueSize+1; i++ {
		registry.EmitEventToTargets([]AcceleratorSessionTarget{{SessionID: slowCall.SessionID, Generation: slow.snapshot.Generation}}, Event{Type: "event", Name: "fill", Data: i})
	}
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("slow targeted overflow blocked producer for %s", elapsed)
	}
	registry.EmitEventToTargets([]AcceleratorSessionTarget{{SessionID: fastCall.SessionID, Generation: fast.snapshot.Generation}}, Event{Type: "event", Name: "healthy"})
	waitAccelerator(t, "healthy targeted delivery", func() bool { return fastConn.writeCount() == 2 })
	waitAccelerator(t, "slow overflow close", func() bool {
		select {
		case <-slowConn.closed:
			return true
		default:
			return false
		}
	})

	var operations sync.WaitGroup
	operations.Add(3)
	go func() {
		defer operations.Done()
		registry.EmitEventToTargets([]AcceleratorSessionTarget{{SessionID: fastCall.SessionID, Generation: fast.snapshot.Generation}}, Event{Type: "event", Name: "race"})
	}()
	go func() { defer operations.Done(); registry.Quiesce() }()
	go func() { defer operations.Done(); _ = registry.Close(context.Background()) }()
	completed := make(chan struct{})
	go func() { operations.Wait(); close(completed) }()
	select {
	case <-completed:
	case <-time.After(2 * time.Second):
		t.Fatal("target delivery/quiesce/close race did not complete")
	}
}

// T4: replacement publishes its generation before the old pumps can finish.
func TestStaleDisconnectCannotRemoveReplacement(t *testing.T) {
	observer := &recordingAcceleratorObserver{}
	registry := newAcceleratorSessionRegistry("instance", observer, acceleratorTestConfig())
	call := acceleratorTestCall("stable")
	oldConn := newFakeAcceleratorConn()
	old, ok := registry.register(call, oldConn)
	if !ok {
		t.Fatal("first register rejected")
	}
	waitAccelerator(t, "old connected write", func() bool { return oldConn.writeCount() == 1 })
	newConn := newFakeAcceleratorConn()
	current, ok := registry.register(call, newConn)
	if !ok {
		t.Fatal("replacement register rejected")
	}
	waitAccelerator(t, "replacement connected write", func() bool { return newConn.writeCount() == 1 })
	<-old.pumpsDone // the real stale completion races the replacement publication
	snapshot, active, found := registry.snapshot(call.SessionID)
	if !found || !active || snapshot.Generation != 2 {
		t.Fatalf("replacement state = %+v active=%v found=%v", snapshot, active, found)
	}
	registry.EmitEvent("fresh", "value")
	waitAccelerator(t, "replacement event", func() bool { return newConn.writeCount() == 2 })
	if got := observer.eventCopy(); fmt.Sprint(got) != "[connect:1 disconnect:1 connect:2]" {
		t.Fatalf("observer events = %v", got)
	}
	closeAcceleratorSocket(t, current)

	for iteration := 0; iteration < 25; iteration++ {
		registry := newAcceleratorSessionRegistry("instance", nil, acceleratorTestConfig())
		call := acceleratorTestCall(fmt.Sprintf("stale-%d", iteration))
		old, _ := registry.register(call, newFakeAcceleratorConn())
		replaced := make(chan *acceleratorSocket, 1)
		go func() {
			socket, _ := registry.register(call, newFakeAcceleratorConn())
			replaced <- socket
		}()
		<-old.pumpsDone
		newSocket := <-replaced
		snapshot, active, found := registry.snapshot(call.SessionID)
		if !found || !active || snapshot.Generation != 2 {
			t.Fatalf("iteration %d replacement = %+v active=%v found=%v", iteration, snapshot, active, found)
		}
		closeAcceleratorSocket(t, newSocket)
	}
}

// T5: one blocked writer fills exactly its bounded queue and is failed closed
// without delaying the producer or a healthy session.
func TestAcceleratorSocketBoundedQueueDoesNotBlock(t *testing.T) {
	registry := newAcceleratorSessionRegistry("instance", nil, acceleratorTestConfig())
	slowConn := newFakeAcceleratorConn()
	slowConn.blockWrite = make(chan struct{}, 1)
	slowConn.blockWrite <- struct{}{} // permit the mandatory connected bootstrap
	slow, ok := registry.register(acceleratorTestCall("slow"), slowConn)
	if !ok {
		t.Fatal("slow register rejected")
	}
	// Registration waits for the mandatory connected bootstrap. Block the first
	// post-bootstrap event so the writer is occupied while the queue is filled.
	if !slow.enqueue(Event{Type: "event", Name: "block"}) {
		t.Fatal("blocking event rejected")
	}
	waitAccelerator(t, "blocked writer", func() bool { return slowConn.writers.Load() == 1 })
	for i := 0; i < AcceleratorSocketQueueSize; i++ {
		if !slow.enqueue(Event{Type: "event", Name: "fill", Data: i}) {
			t.Fatalf("queue rejected slot %d", i)
		}
	}
	start := time.Now()
	if slow.enqueue(Event{Type: "event", Name: "overflow"}) {
		t.Fatal("overflow accepted")
	}
	if elapsed := time.Since(start); elapsed > 25*time.Millisecond {
		t.Fatalf("overflow blocked producer for %s", elapsed)
	}
	if slowConn.maxWriters.Load() != 1 {
		t.Fatalf("concurrent writers = %d", slowConn.maxWriters.Load())
	}
	select {
	case <-slowConn.closed:
		t.Fatal("overflow performed an inline network close")
	default:
	}
	fastConn := newFakeAcceleratorConn()
	fast, ok := registry.register(acceleratorTestCall("fast"), fastConn)
	if !ok {
		t.Fatal("fast register rejected")
	}
	waitAccelerator(t, "fast connected", func() bool { return fastConn.writeCount() == 1 })
	registry.EmitEvent("isolated", "ok")
	waitAccelerator(t, "fast isolated event", func() bool { return fastConn.writeCount() == 2 })
	waitAccelerator(t, "slow hard close", func() bool {
		select {
		case <-slowConn.closed:
			return true
		default:
			return false
		}
	})
	closeAcceleratorSocket(t, fast)

	controlObserver := &recordingAcceleratorObserver{}
	controlRegistry := newAcceleratorSessionRegistry("instance", controlObserver, acceleratorTestConfig())
	controlConn := newFakeAcceleratorConn()
	controlConn.blockControl = make(chan struct{})
	controlCall := acceleratorTestCall("control")
	controlRegistration, ok := controlRegistry.prepareRegistration(controlCall, controlConn)
	if !ok {
		t.Fatal("pre-activation registration rejected")
	}
	controlSocket := controlRegistration.socket
	// The connected event already occupies slot one.
	for i := 1; i < AcceleratorSocketQueueSize; i++ {
		if !controlSocket.enqueue(Event{Type: "event", Name: "fill", Data: i}) {
			t.Fatalf("pre-activation queue rejected slot %d", i)
		}
	}
	start = time.Now()
	if controlSocket.enqueue(Event{Type: "event", Name: "overflow"}) {
		t.Fatal("pre-activation overflow accepted")
	}
	if elapsed := time.Since(start); elapsed > 25*time.Millisecond {
		t.Fatalf("pre-activation overflow blocked producer for %s", elapsed)
	}
	waitAccelerator(t, "writer-owned 1013", func() bool { return controlConn.hasCloseCode(websocket.CloseTryAgainLater) })
	select {
	case <-controlConn.closed:
		t.Fatal("pre-activation overflow closed the network inline")
	default:
	}
	select {
	case <-controlSocket.activationDone:
	default:
		t.Fatal("pre-activation overflow did not settle activation")
	}
	if controlSocket.cancelled.Load() {
		t.Fatal("pre-activation overflow marked the socket cancelled")
	}
	activationReturned := make(chan bool, 1)
	go func() {
		_, activated := controlRegistry.activateRegistration(controlRegistration)
		activationReturned <- activated
	}()
	select {
	case activated := <-activationReturned:
		if activated {
			t.Fatal("overflowed registration activated")
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("overflowed activation did not return promptly")
	}
	if controlSocket.cancelled.Load() {
		t.Fatal("failed activation marked the overflowed socket cancelled")
	}
	close(controlConn.blockControl)
	<-controlSocket.pumpsDone
	if got := controlObserver.eventCopy(); len(got) != 0 {
		t.Fatalf("pre-activation overflow callbacks=%v", got)
	}
	resumed, ok := controlRegistry.register(controlCall, newFakeAcceleratorConn())
	if !ok || resumed.snapshot.Generation != 2 || !resumed.snapshot.Resumed {
		t.Fatalf("post-overflow generation=%+v accepted=%v", resumed, ok)
	}
	closeAcceleratorSocket(t, resumed)
}

func TestAcceleratorPreActivationOverflowLifecycleRaces(t *testing.T) {
	for iteration := 0; iteration < 40; iteration++ {
		registry := newAcceleratorSessionRegistry("instance", nil, acceleratorTestConfig())
		call := acceleratorTestCall(fmt.Sprintf("overflow-race-%d", iteration))
		registration, ok := registry.prepareRegistration(call, newFakeAcceleratorConn())
		if !ok {
			t.Fatalf("iteration %d prepare rejected", iteration)
		}
		for slot := 1; slot < AcceleratorSocketQueueSize; slot++ {
			if !registration.socket.enqueue(Event{Type: "event", Name: "fill", Data: slot}) {
				t.Fatalf("iteration %d queue rejected slot %d", iteration, slot)
			}
		}

		start := make(chan struct{})
		var operations sync.WaitGroup
		operations.Add(6)
		go func() {
			defer operations.Done()
			<-start
			_ = registration.socket.enqueue(Event{Type: "event", Name: "overflow"})
		}()
		go func() {
			defer operations.Done()
			<-start
			_, _ = registry.completeRegistration(registration)
		}()
		go func() {
			defer operations.Done()
			<-start
			_, _ = registry.register(call, newFakeAcceleratorConn())
		}()
		go func() {
			defer operations.Done()
			<-start
			registry.RevokeBrowserSession(context.Background(), call.SessionID)
		}()
		go func() {
			defer operations.Done()
			<-start
			registry.Quiesce()
		}()
		closeResult := make(chan error, 1)
		go func() {
			defer operations.Done()
			<-start
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			closeResult <- registry.Close(ctx)
		}()
		close(start)
		completed := make(chan struct{})
		go func() {
			operations.Wait()
			close(completed)
		}()
		select {
		case <-completed:
		case <-time.After(2 * time.Second):
			t.Fatalf("iteration %d lifecycle race did not complete", iteration)
		}
		if err := <-closeResult; err != nil {
			t.Fatalf("iteration %d Close: %v", iteration, err)
		}
	}
}

// T6: all liveness failures terminate only their current generation. The fake
// records prove read bounds/deadlines, pong extension, ping ownership, and one
// writer; real oversized-frame behavior is covered by the socket suite below.
func TestAcceleratorSocketLivenessBounds(t *testing.T) {
	for _, test := range []struct {
		name      string
		configure func(*fakeAcceleratorConn)
		trigger   func(*fakeAcceleratorConn)
	}{
		{name: "peer close", trigger: func(c *fakeAcceleratorConn) { c.read <- fakeAcceleratorRead{err: errors.New("peer closed")} }},
		{name: "write error", configure: func(c *fakeAcceleratorConn) {
			c.mu.Lock()
			c.writeErr = errors.New("write failed")
			c.mu.Unlock()
		}},
		{name: "deadline error", configure: func(c *fakeAcceleratorConn) {
			c.mu.Lock()
			c.deadlineErr = errors.New("deadline failed")
			c.mu.Unlock()
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			observer := &recordingAcceleratorObserver{}
			registry := newAcceleratorSessionRegistry("instance", observer, acceleratorTestConfig())
			conn := newFakeAcceleratorConn()
			socket, ok := registry.register(acceleratorTestCall(test.name), conn)
			if !ok {
				t.Fatal("register rejected")
			}
			if test.configure != nil {
				test.configure(conn)
				registry.EmitEvent("liveness", nil)
			}
			if test.trigger != nil {
				test.trigger(conn)
			}
			waitAccelerator(t, "one disconnect", func() bool { return len(observer.eventCopy()) == 2 })
			if got := observer.eventCopy(); got[0] != "connect:1" || got[1] != "disconnect:1" {
				t.Fatalf("events = %v", got)
			}
			<-socket.pumpsDone
		})
	}

	observer := &recordingAcceleratorObserver{}
	registry := newAcceleratorSessionRegistry("instance", observer, acceleratorTestConfig())
	conn := newFakeAcceleratorConn()
	socket, ok := registry.register(acceleratorTestCall("pong"), conn)
	if !ok {
		t.Fatal("register rejected")
	}
	waitAccelerator(t, "read setup", func() bool {
		conn.mu.Lock()
		defer conn.mu.Unlock()
		return conn.readLimit != 0 && conn.pong != nil && len(conn.readDeadlines) != 0
	})
	conn.mu.Lock()
	if conn.readLimit != AcceleratorSocketReadLimit {
		t.Fatalf("read limit = %d", conn.readLimit)
	}
	pong := conn.pong
	before := len(conn.readDeadlines)
	conn.mu.Unlock()
	if err := pong("safe"); err != nil {
		t.Fatal(err)
	}
	conn.mu.Lock()
	after := len(conn.readDeadlines)
	conn.mu.Unlock()
	if after != before+1 {
		t.Fatalf("pong deadlines before/after = %d/%d", before, after)
	}
	waitAccelerator(t, "writer ping", func() bool { return conn.controlCount(websocket.PingMessage) > 0 })
	if conn.maxWriters.Load() != 1 {
		t.Fatalf("concurrent data/control writers = %d", conn.maxWriters.Load())
	}
	closeAcceleratorSocket(t, socket)
}

func TestAcceleratorWriterFailureSynchronouslyFencesRPC(t *testing.T) {
	releaseWorker := make(chan struct{})
	caller := &recordingAcceleratorRPCCaller{entered: make(chan acceleratorRPCCall, 1), release: releaseWorker, result: "worker-private-result"}
	dispatcher := NewAcceleratorRPCDispatcher(caller, MethodAuthorizerFunc(func(agent.AuthenticatedCallContext, string) bool { return true }))
	registry := newAcceleratorSessionRegistry("instance", nil, acceleratorTestConfig())
	connection := newFakeAcceleratorConn()
	releaseClose := make(chan struct{})
	connection.blockClose = releaseClose
	callContext := acceleratorTestCall("writer-failure-rpc")
	registration, ok := registry.prepareRPCRegistration(callContext, connection, dispatcher)
	if !ok {
		t.Fatal("RPC registration rejected")
	}
	socket, ok := registry.completeRegistration(registration)
	if !ok {
		t.Fatal("RPC registration did not bootstrap")
	}
	id, _ := acceleratorsecret.CallID("AAAAAAAAAAAAAAAAAAAAAA", 1)
	request, _ := acceleratorsecret.EncodeGetSecretYAMLCall(id, "namespace", "name")
	connection.read <- fakeAcceleratorRead{messageType: websocket.TextMessage, payload: request}
	select {
	case <-caller.entered:
	case <-time.After(time.Second):
		t.Fatal("RPC worker was not admitted")
	}

	connection.mu.Lock()
	connection.writeErr = errors.New("forced post-bootstrap write failure")
	connection.blockWrite = make(chan struct{})
	connection.mu.Unlock()
	if !socket.enqueue(Event{Type: "event", Name: "force-writer-failure"}) {
		t.Fatal("failure trigger was not queued")
	}
	waitAccelerator(t, "failed writer entered", func() bool { return connection.writers.Load() == 1 })
	acceptedSensitive := []byte(`{"type":"result","result":"accepted-private-marker"}`)
	if !socket.enqueueResult(acceptedSensitive) {
		t.Fatal("pre-fence concurrent result was not admitted")
	}
	close(connection.blockWrite)
	select {
	case <-connection.closeEntered:
	case <-time.After(time.Second):
		t.Fatal("writer failure did not reach delayed network close")
	}
	if socket.eligible.Load() {
		t.Fatal("writer failure left generation eligible")
	}
	if !errors.Is(socket.rpc.err(), errAcceleratorRPCUnavailable) {
		t.Fatalf("writer failure RPC state=%v", socket.rpc.err())
	}
	socket.enqueueMu.Lock()
	accepting, queued := socket.accepting, len(socket.queue)
	socket.enqueueMu.Unlock()
	if accepting || queued != 0 || !allAcceleratorBytesZero(acceptedSensitive) {
		t.Fatalf("writer fence accepting=%v queue=%d sensitive=%q", accepting, queued, acceptedSensitive)
	}
	rejectedSensitive := []byte(`{"type":"result","result":"rejected-private-marker"}`)
	if socket.enqueueResult(rejectedSensitive) || !allAcceleratorBytesZero(rejectedSensitive) {
		t.Fatalf("post-fence sensitive result accepted/retained: %q", rejectedSensitive)
	}
	secondID, _ := acceleratorsecret.CallID("AAAAAAAAAAAAAAAAAAAAAA", 2)
	second, _ := acceleratorsecret.EncodeGetSecretYAMLCall(secondID, "namespace", "second")
	if socket.rpc.handle(second) || caller.count() != 1 {
		t.Fatal("writer failure admitted a later RPC")
	}
	close(releaseWorker)
	socket.rpc.waitWorkers()
	if len(socket.queue) != 0 {
		t.Fatalf("late worker refilled writerless queue: %d", len(socket.queue))
	}
	close(releaseClose)
	select {
	case <-socket.pumpsDone:
	case <-time.After(time.Second):
		t.Fatal("writer failure pumps did not complete")
	}
}

func allAcceleratorBytesZero(payload []byte) bool {
	for _, value := range payload {
		if value != 0 {
			return false
		}
	}
	return true
}

func TestAcceleratorSocketElapsedLivenessIsolation(t *testing.T) {
	config := acceleratorSocketConfig{
		writeTimeout: 35 * time.Millisecond,
		pongTimeout:  60 * time.Millisecond,
		pingInterval: 15 * time.Millisecond,
		drainTimeout: 25 * time.Millisecond,
		now:          time.Now,
	}
	assertFailure := func(t *testing.T, registry *AcceleratorSessionRegistry, observer *recordingAcceleratorObserver, failed agent.AuthenticatedCallContext, healthy *fakeAcceleratorConn, healthySocket *acceleratorSocket) {
		t.Helper()
		waitAccelerator(t, "failed generation disconnect", func() bool {
			return fmt.Sprint(observer.eventsForSession(failed.SessionID)) == "[connect:1 disconnect:1]"
		})
		before := healthy.writeCount()
		registry.EmitEvent("healthy-probe", failed.SessionID)
		waitAccelerator(t, "healthy sibling probe", func() bool { return healthy.writeCount() == before+1 })
		closeAcceleratorSocket(t, healthySocket)
	}

	for _, test := range []struct {
		name    string
		trigger func(*AcceleratorSessionRegistry, *fakeAcceleratorConn)
	}{
		{name: "peer close", trigger: func(_ *AcceleratorSessionRegistry, conn *fakeAcceleratorConn) {
			conn.read <- fakeAcceleratorRead{err: errors.New("peer closed")}
		}},
		{name: "write failure", trigger: func(registry *AcceleratorSessionRegistry, conn *fakeAcceleratorConn) {
			conn.mu.Lock()
			conn.writeErr = errors.New("write failed")
			conn.mu.Unlock()
			registry.EmitEvent("write-failure", nil)
		}},
		{name: "oversized read", trigger: func(_ *AcceleratorSessionRegistry, conn *fakeAcceleratorConn) {
			conn.read <- fakeAcceleratorRead{err: errors.New("websocket: read limit exceeded")}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			observer := &recordingAcceleratorObserver{}
			registry := newAcceleratorSessionRegistry("instance", observer, config)
			failedCall := acceleratorTestCall("elapsed-" + test.name)
			failedConn := newFakeAcceleratorConn()
			if _, ok := registry.register(failedCall, failedConn); !ok {
				t.Fatal("failed-session register rejected")
			}
			healthyConn := newFakeAcceleratorConn()
			healthySocket, ok := registry.register(acceleratorTestCall("healthy-"+test.name), healthyConn)
			if !ok {
				t.Fatal("healthy register rejected")
			}
			test.trigger(registry, failedConn)
			assertFailure(t, registry, observer, failedCall, healthyConn, healthySocket)
		})
	}

	t.Run("no pong timeout and cadence", func(t *testing.T) {
		observer := &recordingAcceleratorObserver{}
		registry := newAcceleratorSessionRegistry("instance", observer, config)
		failedCall := acceleratorTestCall("no-pong")
		failedConn := newFakeAcceleratorConn()
		failedConn.enforceReadDeadline = true
		start := time.Now()
		if _, ok := registry.register(failedCall, failedConn); !ok {
			t.Fatal("no-pong register rejected")
		}
		healthyConn := newFakeAcceleratorConn()
		healthySocket, _ := registry.register(acceleratorTestCall("healthy-no-pong"), healthyConn)
		waitAccelerator(t, "no-pong disconnect", func() bool {
			return fmt.Sprint(observer.eventsForSession(failedCall.SessionID)) == "[connect:1 disconnect:1]"
		})
		if elapsed := time.Since(start); elapsed < config.pongTimeout-10*time.Millisecond || elapsed > 500*time.Millisecond {
			t.Fatalf("no-pong elapsed=%s", elapsed)
		}
		if pings := failedConn.controlCount(websocket.PingMessage); pings < 2 {
			t.Fatalf("ping cadence=%d, want at least 2", pings)
		}
		assertFailure(t, registry, observer, failedCall, healthyConn, healthySocket)
	})

	t.Run("pong extends deadline", func(t *testing.T) {
		observer := &recordingAcceleratorObserver{}
		registry := newAcceleratorSessionRegistry("instance", observer, config)
		failedCall := acceleratorTestCall("pong-extension")
		failedConn := newFakeAcceleratorConn()
		failedConn.enforceReadDeadline = true
		if _, ok := registry.register(failedCall, failedConn); !ok {
			t.Fatal("pong register rejected")
		}
		healthyConn := newFakeAcceleratorConn()
		healthySocket, _ := registry.register(acceleratorTestCall("healthy-pong"), healthyConn)
		waitAccelerator(t, "pong handler", func() bool {
			failedConn.mu.Lock()
			defer failedConn.mu.Unlock()
			return failedConn.pong != nil
		})
		time.Sleep(35 * time.Millisecond)
		failedConn.mu.Lock()
		pong := failedConn.pong
		failedConn.mu.Unlock()
		pongAt := time.Now()
		if err := pong("extended"); err != nil {
			t.Fatal(err)
		}
		time.Sleep(35 * time.Millisecond)
		if _, active, _ := registry.snapshot(failedCall.SessionID); !active {
			t.Fatal("socket expired at original deadline despite pong")
		}
		waitAccelerator(t, "extended no-pong disconnect", func() bool {
			return fmt.Sprint(observer.eventsForSession(failedCall.SessionID)) == "[connect:1 disconnect:1]"
		})
		if elapsed := time.Since(pongAt); elapsed < config.pongTimeout-10*time.Millisecond || elapsed > 500*time.Millisecond {
			t.Fatalf("pong extension elapsed=%s", elapsed)
		}
		if pings := failedConn.controlCount(websocket.PingMessage); pings < 2 {
			t.Fatalf("extended ping cadence=%d, want at least 2", pings)
		}
		assertFailure(t, registry, observer, failedCall, healthyConn, healthySocket)
	})

	t.Run("blocked write deadline", func(t *testing.T) {
		observer := &recordingAcceleratorObserver{}
		registry := newAcceleratorSessionRegistry("instance", observer, config)
		failedCall := acceleratorTestCall("blocked-write")
		failedConn := newFakeAcceleratorConn()
		failedConn.blockWrite = make(chan struct{}, 1)
		failedConn.blockWrite <- struct{}{}
		failedConn.enforceWriteDeadline = true
		if _, ok := registry.register(failedCall, failedConn); !ok {
			t.Fatal("blocked-write register rejected")
		}
		healthyConn := newFakeAcceleratorConn()
		healthySocket, _ := registry.register(acceleratorTestCall("healthy-blocked-write"), healthyConn)
		start := time.Now()
		registry.EmitEvent("blocked-write", nil)
		waitAccelerator(t, "blocked-write disconnect", func() bool {
			return fmt.Sprint(observer.eventsForSession(failedCall.SessionID)) == "[connect:1 disconnect:1]"
		})
		if elapsed := time.Since(start); elapsed < config.writeTimeout-5*time.Millisecond || elapsed > 500*time.Millisecond {
			t.Fatalf("blocked-write elapsed=%s", elapsed)
		}
		assertFailure(t, registry, observer, failedCall, healthyConn, healthySocket)
	})
}

// T7: snapshots retain each generation's own resumed bit and identity, and
// observer inspection re-entry cannot deadlock.
func TestAcceleratorSessionObserverOrdering(t *testing.T) {
	observer := &recordingAcceleratorObserver{}
	registry := newAcceleratorSessionRegistry("instance", observer, acceleratorTestConfig())
	observer.onEvent = func(_ string, snapshot AcceleratorSessionSnapshot) {
		_, _, _ = registry.snapshot(snapshot.CallContext.SessionID)
	}
	call := acceleratorTestCall("ordered")
	first, _ := registry.register(call, newFakeAcceleratorConn())
	second, _ := registry.register(call, newFakeAcceleratorConn())
	closeAcceleratorSocket(t, second)
	waitAccelerator(t, "generation two disconnect", func() bool { return len(observer.eventCopy()) == 4 })
	third, _ := registry.register(call, newFakeAcceleratorConn())
	registry.RevokeBrowserSession(context.Background(), call.SessionID)
	<-third.pumpsDone
	got := observer.eventCopy()
	want := "[connect:1 disconnect:1 connect:2 disconnect:2 connect:3 revoke:3]"
	if fmt.Sprint(got) != want {
		t.Fatalf("events = %v, want %s", got, want)
	}
	snapshots := observer.snapshotCopy()
	if snapshots[0].Resumed || snapshots[1].Resumed || !snapshots[2].Resumed || !snapshots[3].Resumed || !snapshots[4].Resumed || !snapshots[5].Resumed {
		t.Fatalf("resumed snapshots = %+v", snapshots)
	}
	if snapshots[1].CallContext != call || snapshots[1].Generation != first.snapshot.Generation {
		t.Fatalf("old immutable snapshot changed: %+v", snapshots[1])
	}
}

func TestAcceleratorObserversCanReenterLifecycle(t *testing.T) {
	observer := &recordingAcceleratorObserver{}
	registry := newAcceleratorSessionRegistry("instance", observer, acceleratorTestConfig())
	call := acceleratorTestCall("reentrant-connect")
	observer.onEvent = func(kind string, snapshot AcceleratorSessionSnapshot) {
		if kind == "connect" {
			_, _, _ = registry.snapshot(snapshot.CallContext.SessionID)
			registry.Quiesce()
		}
	}
	completed := make(chan struct{})
	go func() { _, _ = registry.register(call, newFakeAcceleratorConn()); close(completed) }()
	select {
	case <-completed:
	case <-time.After(2 * time.Second):
		t.Fatal("connect observer lifecycle inspection deadlocked")
	}
	waitAccelerator(t, "reentrant quiesce disconnect", func() bool { return len(observer.eventCopy()) == 2 })
	if got := fmt.Sprint(observer.eventCopy()); got != "[connect:1 disconnect:1]" {
		t.Fatalf("events=%s", got)
	}

	observer2 := &recordingAcceleratorObserver{}
	registry2 := newAcceleratorSessionRegistry("instance", observer2, acceleratorTestConfig())
	disconnected := make(chan struct{})
	observer2.onEvent = func(kind string, _ AcceleratorSessionSnapshot) {
		if kind == "disconnect" {
			_, _, _ = registry2.snapshot(acceleratorTestCall("reentrant-disconnect").SessionID)
			registry2.Quiesce()
			close(disconnected)
		}
	}
	socket, _ := registry2.register(acceleratorTestCall("reentrant-disconnect"), newFakeAcceleratorConn())
	socket.requestClose(acceleratorSocketClose{code: websocket.CloseNormalClosure, reason: "done"})
	select {
	case <-disconnected:
	case <-time.After(2 * time.Second):
		t.Fatal("disconnect observer quiesce deadlocked")
	}
}

func TestConcurrentRevokeWaitsForItsCallback(t *testing.T) {
	connectEntered := make(chan struct{})
	releaseConnect := make(chan struct{})
	revokeEntered := make(chan struct{})
	releaseRevoke := make(chan struct{})
	observer := &recordingAcceleratorObserver{}
	registry := newAcceleratorSessionRegistry("instance", observer, acceleratorTestConfig())
	observer.onEvent = func(kind string, _ AcceleratorSessionSnapshot) {
		switch kind {
		case "connect":
			close(connectEntered)
			<-releaseConnect
		case "revoke":
			close(revokeEntered)
			<-releaseRevoke
		}
	}
	call := acceleratorTestCall("concurrent-revoke")
	registerDone := make(chan struct{})
	go func() { _, _ = registry.register(call, newFakeAcceleratorConn()); close(registerDone) }()
	select {
	case <-connectEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("connect callback not entered")
	}
	revokeDone := make(chan struct{})
	go func() { registry.RevokeBrowserSession(context.Background(), call.SessionID); close(revokeDone) }()
	select {
	case <-revokeEntered:
		t.Fatal("revoke callback overlapped SessionConnected")
	case <-time.After(25 * time.Millisecond):
	}
	select {
	case <-revokeDone:
		t.Fatal("revoke returned before its callback")
	default:
	}
	close(releaseConnect)
	select {
	case <-registerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("register did not complete after connect callback")
	}
	select {
	case <-revokeEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("revoke callback did not follow SessionConnected")
	}
	select {
	case <-revokeDone:
		t.Fatal("revoke returned before its callback")
	default:
	}
	close(releaseRevoke)
	select {
	case <-revokeDone:
	case <-time.After(2 * time.Second):
		t.Fatal("revoke did not return after callback")
	}
	if got := fmt.Sprint(observer.eventCopy()); got != "[connect:1 revoke:1]" {
		t.Fatalf("events=%s", got)
	}
}

func TestExternalCloseWaitsForDisconnectCallback(t *testing.T) {
	disconnectEntered := make(chan struct{})
	releaseDisconnect := make(chan struct{})
	observer := &recordingAcceleratorObserver{}
	registry := newAcceleratorSessionRegistry("instance", observer, acceleratorTestConfig())
	observer.onEvent = func(kind string, _ AcceleratorSessionSnapshot) {
		if kind == "disconnect" {
			close(disconnectEntered)
			<-releaseDisconnect
		}
	}
	socket, _ := registry.register(acceleratorTestCall("close-callback"), newFakeAcceleratorConn())
	socket.requestClose(acceleratorSocketClose{code: websocket.CloseNormalClosure, reason: "done"})
	select {
	case <-disconnectEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("disconnect callback not entered")
	}
	closeDone := make(chan error, 1)
	go func() { closeDone <- registry.Close(context.Background()) }()
	select {
	case err := <-closeDone:
		t.Fatalf("Close returned before callback release: %v", err)
	case <-time.After(25 * time.Millisecond):
	}
	close(releaseDisconnect)
	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not complete after callback")
	}
	registry.callbackStateMu.Lock()
	outstanding := registry.outstandingCallbacks
	registry.callbackStateMu.Unlock()
	if outstanding != 0 {
		t.Fatalf("outstanding callbacks=%d", outstanding)
	}
}

func TestAcceleratorReplacementConnectedFrameCannotBePreempted(t *testing.T) {
	registry := newAcceleratorSessionRegistry("instance", nil, acceleratorTestConfig())
	call := acceleratorTestCall("pending")
	old, _ := registry.register(call, newFakeAcceleratorConn())
	registration, ok := registry.prepareRegistration(call, newFakeAcceleratorConn())
	if !ok {
		t.Fatal("prepare rejected")
	}
	<-old.pumpsDone
	for i := 0; i < AcceleratorSocketQueueSize; i++ {
		registry.EmitEvent("preempt", i)
	}
	if len(registration.socket.queue) != 1 {
		t.Fatalf("pending queue=%d, want connected only", len(registration.socket.queue))
	}
	socket, ok := registry.completeRegistration(registration)
	if !ok {
		t.Fatal("completion rejected")
	}
	conn := registration.socket.conn.(*fakeAcceleratorConn)
	waitAccelerator(t, "connected", func() bool { return conn.writeCount() == 1 })
	conn.mu.Lock()
	first := conn.writes[0]
	conn.mu.Unlock()
	if first.Name != "connected" {
		t.Fatalf("first frame=%q", first.Name)
	}
	registry.EmitEvent("after-ready", "ok")
	waitAccelerator(t, "ready broadcast", func() bool { return conn.writeCount() == 2 })
	closeAcceleratorSocket(t, socket)
}

func TestAcceleratorConnectedEligibilityDoesNotLoseBarrierEvent(t *testing.T) {
	registry := newAcceleratorSessionRegistry("instance", nil, acceleratorTestConfig())
	conn := newFakeAcceleratorConn()
	conn.afterWrite = func(event Event) {
		if event.Name == "connected" {
			registry.EmitEvent("barrier", "visible")
		}
	}
	socket, ok := registry.register(acceleratorTestCall("barrier"), conn)
	if !ok {
		t.Fatal("register rejected")
	}
	waitAccelerator(t, "barrier event", func() bool { return conn.writeCount() == 2 })
	conn.mu.Lock()
	names := []string{conn.writes[0].Name, conn.writes[1].Name}
	conn.mu.Unlock()
	if fmt.Sprint(names) != "[connected barrier]" {
		t.Fatalf("frames=%v", names)
	}
	closeAcceleratorSocket(t, socket)
}

func TestAcceleratorConnectedPublicationWaitsForObserverAndReadiness(t *testing.T) {
	connectEntered := make(chan struct{})
	releaseConnect := make(chan struct{})
	callbackComplete := atomic.Bool{}
	currentGeneration := atomic.Uint64{}
	observer := &recordingAcceleratorObserver{}
	observer.onEvent = func(kind string, snapshot AcceleratorSessionSnapshot) {
		if kind != "connect" {
			return
		}
		currentGeneration.Store(uint64(snapshot.Generation))
		if snapshot.Generation == 2 {
			close(connectEntered)
			<-releaseConnect
			callbackComplete.Store(true)
		}
	}
	registry := newAcceleratorSessionRegistry("instance", observer, acceleratorTestConfig())
	call := acceleratorTestCall("callback-gated")
	old, ok := registry.register(call, newFakeAcceleratorConn())
	if !ok {
		t.Fatal("initial registration rejected")
	}
	closeAcceleratorSocket(t, old)

	conn := newFakeAcceleratorConn()
	conn.blockWrite = make(chan struct{})
	var afterWriteFailure atomic.Value
	conn.afterWrite = func(event Event) {
		if event.Name != "connected" {
			return
		}
		if !callbackComplete.Load() || currentGeneration.Load() != 2 {
			afterWriteFailure.Store("connected write preceded generation observer")
		}
		lease, found := registry.LookupSessionLease(call.SessionID)
		if !found || !lease.Connected || lease.Generation != 2 {
			afterWriteFailure.Store(fmt.Sprintf("connected write lease=%+v found=%v", lease, found))
		}
		registry.EmitEventToTargets([]AcceleratorSessionTarget{
			{SessionID: call.SessionID, Generation: 1},
			{SessionID: call.SessionID, Generation: 2},
		}, Event{Type: "event", Name: "immediate-after-connected"})
	}
	registered := make(chan *acceleratorSocket, 1)
	go func() {
		socket, accepted := registry.register(call, conn)
		if !accepted {
			registered <- nil
			return
		}
		registered <- socket
	}()
	waitSecret := func(signal <-chan struct{}, name string) {
		t.Helper()
		select {
		case <-signal:
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for %s", name)
		}
	}
	waitSecret(connectEntered, "generation-two observer")
	if conn.writeCount() != 0 {
		t.Fatal("connected frame published before SessionConnected returned")
	}
	if lease, found := registry.LookupSessionLease(call.SessionID); !found || lease.Connected || lease.Generation != 2 {
		t.Fatalf("pre-observer lease=%+v found=%v", lease, found)
	}
	close(releaseConnect)
	waitAccelerator(t, "ready writer gate", func() bool {
		lease, found := registry.LookupSessionLease(call.SessionID)
		return found && lease.Connected && lease.Generation == 2 && conn.writers.Load() == 1
	})
	registry.EmitEventToTargets([]AcceleratorSessionTarget{{SessionID: call.SessionID, Generation: 2}}, Event{Type: "event", Name: "queued-behind-connected"})
	close(conn.blockWrite)
	var socket *acceleratorSocket
	select {
	case socket = <-registered:
		if socket == nil {
			t.Fatal("generation-two registration rejected")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("generation-two registration did not complete")
	}
	waitAccelerator(t, "callback-gated frames", func() bool { return conn.writeCount() == 3 })
	if failure := afterWriteFailure.Load(); failure != nil {
		t.Fatal(failure)
	}
	conn.mu.Lock()
	names := []string{conn.writes[0].Name, conn.writes[1].Name, conn.writes[2].Name}
	conn.mu.Unlock()
	if got := fmt.Sprint(names); got != "[connected queued-behind-connected immediate-after-connected]" {
		t.Fatalf("callback-gated frames=%s", got)
	}
	registry.EmitEventToTargets([]AcceleratorSessionTarget{{SessionID: call.SessionID, Generation: 1}}, Event{Type: "event", Name: "stale-only"})
	if len(socket.queue) != 0 || conn.writeCount() != 3 {
		t.Fatalf("stale generation reached replacement: queue=%d writes=%d", len(socket.queue), conn.writeCount())
	}
	closeAcceleratorSocket(t, socket)
}

func TestAcceleratorFailedReplacementWaitsForConnectCompletion(t *testing.T) {
	connectEntered := make(chan struct{})
	releaseConnect := make(chan struct{})
	observer := &recordingAcceleratorObserver{}
	registry := newAcceleratorSessionRegistry("instance", observer, acceleratorTestConfig())
	call := acceleratorTestCall("causal-replacement")
	first, ok := registry.register(call, newFakeAcceleratorConn())
	if !ok {
		t.Fatal("generation one rejected")
	}
	closeAcceleratorSocket(t, first)
	waitAccelerator(t, "generation one callbacks", func() bool { return len(observer.eventCopy()) == 2 })
	observer.mu.Lock()
	observer.events = nil
	observer.snaps = nil
	observer.onEvent = func(kind string, snapshot AcceleratorSessionSnapshot) {
		if kind == "connect" && snapshot.Generation == 2 {
			close(connectEntered)
			<-releaseConnect
		}
	}
	observer.mu.Unlock()

	secondDone := make(chan struct{})
	go func() {
		_, _ = registry.register(call, newFakeAcceleratorConn())
		close(secondDone)
	}()
	select {
	case <-connectEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("generation two connect callback not entered")
	}
	failing := newFakeAcceleratorConn()
	failing.writeErr = errors.New("bootstrap failed")
	thirdResult := make(chan bool, 1)
	go func() {
		_, accepted := registry.register(call, failing)
		thirdResult <- accepted
	}()
	select {
	case accepted := <-thirdResult:
		t.Fatalf("generation three completed before generation two connect: %v", accepted)
	case <-time.After(25 * time.Millisecond):
	}
	if got := fmt.Sprint(observer.eventCopy()); got != "[connect:2]" {
		t.Fatalf("callbacks while connect blocked=%s", got)
	}
	close(releaseConnect)
	select {
	case <-secondDone:
	case <-time.After(2 * time.Second):
		t.Fatal("generation two registration did not complete")
	}
	select {
	case accepted := <-thirdResult:
		if accepted {
			t.Fatal("failed generation three accepted")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("generation three did not finish")
	}
	waitAccelerator(t, "causal disconnect", func() bool { return len(observer.eventCopy()) == 4 })
	if got := fmt.Sprint(observer.eventCopy()); got != "[connect:2 disconnect:2 connect:3 disconnect:3]" {
		t.Fatalf("callbacks=%s", got)
	}
}

func TestAcceleratorSerializationFailureIsSanitizedOnce(t *testing.T) {
	var logs bytes.Buffer
	oldOutput, oldFlags, oldPrefix := log.Writer(), log.Flags(), log.Prefix()
	log.SetOutput(&logs)
	log.SetFlags(0)
	log.SetPrefix("")
	defer func() { log.SetOutput(oldOutput); log.SetFlags(oldFlags); log.SetPrefix(oldPrefix) }()
	observer := &recordingAcceleratorObserver{}
	registry := newAcceleratorSessionRegistry("instance", observer, acceleratorTestConfig())
	_, _ = registry.register(acceleratorTestCall("marshal-a"), newFakeAcceleratorConn())
	_, _ = registry.register(acceleratorTestCall("marshal-b"), newFakeAcceleratorConn())
	registry.EmitEvent("payload-sentinel", failingAcceleratorJSON{})
	waitAccelerator(t, "both serialization disconnects", func() bool { return len(observer.eventCopy()) == 4 })
	got := logs.String()
	marker := "Accelerator WebSocket outbound event serialization failed"
	if strings.Count(got, marker) != 1 {
		t.Fatalf("log marker count=%d log=%q", strings.Count(got, marker), got)
	}
	if strings.Contains(got, "credential-sentinel") || strings.Contains(got, "payload-sentinel") {
		t.Fatalf("sensitive serialization detail leaked: %q", got)
	}
}

// T8: revoke removes state before its callback, waits pumps and callback, and
// cannot affect stale/different/new IDs.
func TestRegistryRevokeBrowserSessionWaitsForCleanup(t *testing.T) {
	releaseObserver := make(chan struct{})
	observerEntered := make(chan struct{})
	observer := &recordingAcceleratorObserver{}
	observer.onEvent = func(kind string, _ AcceleratorSessionSnapshot) {
		if kind == "revoke" {
			close(observerEntered)
			<-releaseObserver
		}
	}
	registry := newAcceleratorSessionRegistry("instance", observer, acceleratorTestConfig())
	call := acceleratorTestCall("browser-old")
	socket, ok := registry.register(call, newFakeAcceleratorConn())
	if !ok {
		t.Fatal("register rejected")
	}
	returned := make(chan struct{})
	go func() {
		registry.RevokeBrowserSession(context.Background(), call.SessionID)
		close(returned)
	}()
	select {
	case <-observerEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("revoke observer not entered")
	}
	if _, _, found := registry.snapshot(call.SessionID); found {
		t.Fatal("record visible during revoke callback")
	}
	select {
	case <-socket.pumpsDone:
	default:
		t.Fatal("revoke callback began before pumps stopped")
	}
	select {
	case <-returned:
		t.Fatal("revoke returned before observer")
	default:
	}
	close(releaseObserver)
	<-returned
	registry.RevokeBrowserSession(context.Background(), call.SessionID)
	registry.RevokeBrowserSession(context.Background(), "different")
	if got := observer.eventCopy(); fmt.Sprint(got) != "[connect:1 revoke:1]" {
		t.Fatalf("events = %v", got)
	}
	newCall := acceleratorTestCall("browser-new")
	newSocket, _ := registry.register(newCall, newFakeAcceleratorConn())
	registry.RevokeBrowserSession(context.Background(), call.SessionID)
	if _, active, found := registry.snapshot(newCall.SessionID); !found || !active {
		t.Fatal("stale revoke affected new ID")
	}
	closeAcceleratorSocket(t, newSocket)
}

// T9: upgrade admission and quiesce have one total order; shutdown rejects
// producers immediately and Close waits pumps/callbacks without channel races.
func TestAcceleratorRegistryQuiesceAndClose(t *testing.T) {
	registry := newAcceleratorSessionRegistry("instance", nil, acceleratorTestConfig())
	release, ok := registry.beginUpgrade()
	if !ok {
		t.Fatal("initial admission rejected")
	}
	quiesced := make(chan struct{})
	go func() { registry.Quiesce(); close(quiesced) }()
	select {
	case <-quiesced:
		t.Fatal("quiesce passed an in-flight upgrade")
	case <-time.After(20 * time.Millisecond):
	}
	release()
	<-quiesced
	if _, ok := registry.beginUpgrade(); ok {
		t.Fatal("post-quiesce upgrade admitted")
	}
	registry.EmitEvent("ignored", nil)
	if err := registry.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	fifoConfig := acceleratorTestConfig()
	fifoConfig.drainTimeout = 200 * time.Millisecond
	fifoObserver := &recordingAcceleratorObserver{}
	fifo := newAcceleratorSessionRegistry("instance", fifoObserver, fifoConfig)
	fifoConn := newFakeAcceleratorConn()
	fifoConn.blockWrite = make(chan struct{}, 1)
	fifoConn.blockWrite <- struct{}{}
	fifoSocket, ok := fifo.register(acceleratorTestCall("fifo-drain"), fifoConn)
	if !ok {
		t.Fatal("FIFO drain register rejected")
	}
	fifo.EmitEvent("first", 1)
	waitAccelerator(t, "FIFO writer blocked", func() bool { return fifoConn.writers.Load() == 1 })
	fifo.EmitEvent("second", 2)
	fifo.EmitEvent("third", 3)
	drainStart := time.Now()
	fifo.Quiesce()
	if fifoSocket.enqueue(Event{Type: "event", Name: "post-quiesce-enqueue"}) {
		t.Fatal("post-Quiesce socket enqueue accepted")
	}
	fifo.EmitEvent("post-quiesce-emit", nil)
	close(fifoConn.blockWrite)
	fifoCtx, fifoCancel := context.WithTimeout(context.Background(), time.Second)
	if err := fifo.Close(fifoCtx); err != nil {
		fifoCancel()
		t.Fatal(err)
	}
	fifoCancel()
	if elapsed := time.Since(drainStart); elapsed >= fifoConfig.drainTimeout {
		t.Fatalf("successful FIFO drain reached timeout: %s", elapsed)
	}
	fifoConn.mu.Lock()
	var fifoNames []string
	for _, event := range fifoConn.writes {
		fifoNames = append(fifoNames, event.Name)
	}
	fifoConn.mu.Unlock()
	if got := fmt.Sprint(fifoNames); got != "[connected first second third]" {
		t.Fatalf("FIFO drain frames=%s", got)
	}
	if got := fmt.Sprint(fifoObserver.eventCopy()); got != "[connect:1 disconnect:1]" {
		t.Fatalf("FIFO drain callbacks=%s", got)
	}

	observer := &recordingAcceleratorObserver{}
	draining := newAcceleratorSessionRegistry("instance", observer, acceleratorTestConfig())
	conn := newFakeAcceleratorConn()
	conn.blockWrite = make(chan struct{}, 1)
	conn.blockWrite <- struct{}{} // permit the mandatory connected bootstrap
	if _, ok := draining.register(acceleratorTestCall("blocked"), conn); !ok {
		t.Fatal("blocked drain register rejected")
	}
	draining.EmitEvent("block", nil)
	waitAccelerator(t, "blocked drain writer", func() bool { return conn.writers.Load() == 1 })
	start := time.Now()
	draining.Quiesce()
	if time.Since(start) > 25*time.Millisecond {
		t.Fatal("Quiesce waited for network drain")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := draining.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed < draining.config.drainTimeout || elapsed > 500*time.Millisecond {
		t.Fatalf("bounded hard close elapsed = %s", elapsed)
	}
	if got := observer.eventCopy(); fmt.Sprint(got) != "[connect:1 disconnect:1]" {
		t.Fatalf("shutdown events = %v", got)
	}

	var closedOperations sync.WaitGroup
	for i := 0; i < 20; i++ {
		closedOperations.Add(3)
		go func(value int) { defer closedOperations.Done(); draining.EmitEvent("closed", value) }(i)
		go func(value int) {
			defer closedOperations.Done()
			draining.RevokeBrowserSession(context.Background(), agent.SessionID(fmt.Sprintf("missing-%d", value)))
		}(i)
		go func() { defer closedOperations.Done(); draining.Quiesce() }()
	}
	closedDone := make(chan struct{})
	go func() { closedOperations.Wait(); close(closedDone) }()
	select {
	case <-closedDone:
	case <-time.After(2 * time.Second):
		t.Fatal("post-close collision goroutines did not join")
	}
	closedCtx, closedCancel := context.WithTimeout(context.Background(), time.Second)
	if err := draining.Close(closedCtx); err != nil {
		closedCancel()
		t.Fatal(err)
	}
	closedCancel()

	callbackEntered := make(chan struct{})
	releaseCallback := make(chan struct{})
	blockingObserver := &recordingAcceleratorObserver{}
	blockingObserver.onEvent = func(kind string, _ AcceleratorSessionSnapshot) {
		if kind == "disconnect" {
			close(callbackEntered)
			<-releaseCallback
		}
	}
	waiting := newAcceleratorSessionRegistry("instance", blockingObserver, acceleratorTestConfig())
	_, _ = waiting.register(acceleratorTestCall("close-waits-observer"), newFakeAcceleratorConn())
	closeReturned := make(chan error, 1)
	go func() { closeReturned <- waiting.Close(context.Background()) }()
	select {
	case <-callbackEntered:
	case <-time.After(time.Second):
		t.Fatal("shutdown observer callback not entered")
	}
	select {
	case err := <-closeReturned:
		t.Fatalf("Close returned before observer callback: %v", err)
	default:
	}
	close(releaseCallback)
	if err := <-closeReturned; err != nil {
		t.Fatal(err)
	}

	for iteration := 0; iteration < 12; iteration++ {
		racing := newAcceleratorSessionRegistry("instance", nil, acceleratorTestConfig())
		call := acceleratorTestCall(fmt.Sprintf("shutdown-race-%d", iteration))
		_, _ = racing.register(call, newFakeAcceleratorConn())
		var operations sync.WaitGroup
		operations.Add(3)
		go func() {
			defer operations.Done()
			_, _ = racing.register(call, newFakeAcceleratorConn())
		}()
		go func() {
			defer operations.Done()
			racing.RevokeBrowserSession(context.Background(), call.SessionID)
		}()
		go func() {
			defer operations.Done()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			_ = racing.Close(ctx)
		}()
		completed := make(chan struct{})
		go func() { operations.Wait(); close(completed) }()
		select {
		case <-completed:
		case <-time.After(2 * time.Second):
			t.Fatalf("shutdown race %d deadlocked", iteration)
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		if err := racing.Close(ctx); err != nil {
			cancel()
			t.Fatalf("shutdown race %d final close: %v", iteration, err)
		}
		cancel()
	}
}

func TestBrowserWebSocketExpiryRecheckClearsAndRevokes(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	observer := &recordingAcceleratorObserver{}
	registry := newAcceleratorSessionRegistry("instance", observer, acceleratorTestConfig())
	manager := newBrowserSessionManager(func() time.Time { return now }, &incrementingEntropy{}, registry)
	ticket, _, err := manager.Mint()
	if err != nil {
		t.Fatal(err)
	}
	bearer, _, err := manager.Exchange(ticket)
	if err != nil {
		t.Fatal(err)
	}
	call, ok := manager.Authenticate(bearer)
	if !ok {
		t.Fatal("fresh browser bearer rejected")
	}
	socket, ok := registry.register(call, newFakeAcceleratorConn())
	if !ok {
		t.Fatal("browser socket register rejected")
	}
	now = now.Add(BrowserSessionIdleTTL)
	if manager.withActiveBrowserSession(call.SessionID, func() bool {
		t.Fatal("expired session reached activation")
		return true
	}) {
		t.Fatal("exact idle expiry remained active")
	}
	<-socket.pumpsDone
	if _, _, found := registry.snapshot(call.SessionID); found {
		t.Fatal("expired browser registry record retained")
	}
	if got := observer.eventCopy(); fmt.Sprint(got) != "[connect:1 revoke:1]" {
		t.Fatalf("expiry observer events = %v", got)
	}
}

func TestAcceleratorSessionGenerationOverflowFailsClosed(t *testing.T) {
	registry := newAcceleratorSessionRegistry("instance", nil, acceleratorTestConfig())
	call := acceleratorTestCall("overflow")
	record := &acceleratorSessionRecord{call: call, generation: AcceleratorSocketGeneration(^uint64(0)), ever: true}
	registry.records[call.SessionID] = record
	if socket, ok := registry.register(call, newFakeAcceleratorConn()); ok || socket != nil {
		t.Fatal("overflow registration accepted")
	}
	snapshot, active, found := registry.snapshot(call.SessionID)
	if !found || active || snapshot.Generation != 0 || record.generation != AcceleratorSocketGeneration(^uint64(0)) {
		t.Fatalf("overflow mutated record: %+v active=%v found=%v generation=%d", snapshot, active, found, record.generation)
	}
}
