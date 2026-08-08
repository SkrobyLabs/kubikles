package server

import (
	"context"
	"sync"
	"time"

	"kubikles/pkg/agent"
)

// AcceleratorIdleClock is deliberately small so lifecycle tests can control
// the exact reconnect boundary without sleeping.
type AcceleratorIdleClock interface {
	NewTimer(time.Duration) AcceleratorIdleTimer
}
type AcceleratorIdleTimer interface {
	C() <-chan time.Time
	Stop() bool
}
type acceleratorIdleRealClock struct{}
type acceleratorIdleRealTimer struct{ *time.Timer }

func (acceleratorIdleRealClock) NewTimer(d time.Duration) AcceleratorIdleTimer {
	return acceleratorIdleRealTimer{time.NewTimer(d)}
}
func (t acceleratorIdleRealTimer) C() <-chan time.Time { return t.Timer.C }

type AcceleratorIdleCoordinator struct {
	mu                       sync.Mutex
	current                  map[agent.SessionID]AcceleratorSocketGeneration
	ready, stopped, expiring bool
	epoch                    uint64
	timer                    *acceleratorIdleTimerRecord
	clock                    AcceleratorIdleClock
	lifecycle                agent.DisposableIdleLifecycle
	cleanupContext           context.Context
	reportCleanupFailure     func()
	workerWG                 sync.WaitGroup
}

type acceleratorIdleTimerRecord struct {
	timer      AcceleratorIdleTimer
	epoch      uint64
	cancel     chan struct{}
	waiterDone chan struct{}
}

var _ AcceleratorSessionObserver = (*AcceleratorIdleCoordinator)(nil)

func NewAcceleratorIdleCoordinator(lifecycle agent.DisposableIdleLifecycle, ctx context.Context, report func()) *AcceleratorIdleCoordinator {
	return newAcceleratorIdleCoordinator(acceleratorIdleRealClock{}, lifecycle, ctx, report)
}
func newAcceleratorIdleCoordinator(clock AcceleratorIdleClock, lifecycle agent.DisposableIdleLifecycle, ctx context.Context, report func()) *AcceleratorIdleCoordinator {
	if clock == nil {
		clock = acceleratorIdleRealClock{}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if report == nil {
		report = func() {}
	}
	return &AcceleratorIdleCoordinator{
		current: make(map[agent.SessionID]AcceleratorSocketGeneration), clock: clock,
		lifecycle: lifecycle, cleanupContext: ctx, reportCleanupFailure: report,
	}
}
func (c *AcceleratorIdleCoordinator) MarkReady() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ready || c.stopped || c.expiring {
		return
	}
	c.ready = true
	if len(c.current) == 0 {
		c.startGraceLocked()
	}
}
func (c *AcceleratorIdleCoordinator) SessionConnected(s AcceleratorSessionSnapshot) {
	if s.CallContext.SessionID == "" || s.Generation == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sessionConnectedLocked(s)
}
func (c *AcceleratorIdleCoordinator) sessionConnectedLocked(s AcceleratorSessionSnapshot) {
	if c.stopped || c.expiring {
		return
	}
	if old, ok := c.current[s.CallContext.SessionID]; ok && s.Generation <= old {
		return
	}
	c.current[s.CallContext.SessionID] = s.Generation
	c.cancelGraceLocked()
}
func (c *AcceleratorIdleCoordinator) SessionDisconnected(s AcceleratorSessionSnapshot) { c.remove(s) }
func (c *AcceleratorIdleCoordinator) SessionRevoked(s AcceleratorSessionSnapshot)      { c.remove(s) }
func (c *AcceleratorIdleCoordinator) remove(s AcceleratorSessionSnapshot) {
	if s.CallContext.SessionID == "" || s.Generation == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stopped || c.expiring {
		return
	}
	if c.current[s.CallContext.SessionID] != s.Generation {
		return
	}
	delete(c.current, s.CallContext.SessionID)
	if c.ready && len(c.current) == 0 {
		c.startGraceLocked()
	}
}
func (c *AcceleratorIdleCoordinator) cancelGraceLocked() {
	record := c.timer
	if record == nil {
		return
	}
	c.timer = nil
	c.epoch++
	record.timer.Stop()
	close(record.cancel)
	// The waiter acknowledges its exit before attempting to acquire c.mu on
	// the fire path, so cancellation can wait here without deadlocking.
	<-record.waiterDone
}
func (c *AcceleratorIdleCoordinator) startGraceLocked() {
	c.cancelGraceLocked()
	c.epoch++
	record := &acceleratorIdleTimerRecord{
		timer: c.clock.NewTimer(agent.EffectiveAcceleratorIdleReconnectGrace()), epoch: c.epoch,
		cancel: make(chan struct{}), waiterDone: make(chan struct{}),
	}
	c.timer = record
	// Add occurs while c.mu is held. Shutdown first publishes stopped while
	// holding the same lock, then waits after unlocking, so no Add can race Wait.
	c.workerWG.Add(1)
	go func() {
		defer c.workerWG.Done()
		select {
		case <-record.timer.C():
			close(record.waiterDone)
			c.fire(record)
		case <-record.cancel:
			close(record.waiterDone)
		}
	}()
}
func (c *AcceleratorIdleCoordinator) fire(record *acceleratorIdleTimerRecord) {
	c.mu.Lock()
	if c.stopped || c.expiring || !c.ready || len(c.current) != 0 || c.epoch != record.epoch || c.timer != record {
		c.mu.Unlock()
		return
	}
	c.expiring = true
	c.timer = nil
	c.mu.Unlock()
	if c.lifecycle != nil {
		if err := agent.ExpireDisposableIdle(c.cleanupContext, c.lifecycle); err != nil {
			c.reportCleanupFailure()
		}
	}
}
func (c *AcceleratorIdleCoordinator) Shutdown() {
	c.mu.Lock()
	if !c.stopped {
		c.stopped = true
		c.cancelGraceLocked()
	}
	c.mu.Unlock()
	c.workerWG.Wait()
}
