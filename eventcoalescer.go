package main

import (
	"sync"
	"sync/atomic"
	"time"

	"kubikles/pkg/crashlog"
)

const (
	// maxAdaptiveInterval caps how far the frame interval backs off under load.
	maxAdaptiveInterval = 250 * time.Millisecond

	// backgroundFrameInterval is the flush cadence while the window is hidden.
	backgroundFrameInterval = time.Second

	// maxQueuedEmissions bounds how many flushed-but-not-yet-delivered emissions
	// may sit in the ordered queue. Beyond it the timer path stops flushing, so
	// events keep coalescing in the pending map (latest wins) instead of piling
	// up as stale per-key versions behind a slow bridge.
	maxQueuedEmissions = 2

	// flushWaitTimeout bounds how long synchronous flushes wait for delivery.
	flushWaitTimeout = 2 * time.Second
)

// queuedEmission is one ordered delivery to the frontend.
type queuedEmission struct {
	gen     uint64 // generation at enqueue time; stale generations are dropped
	name    string
	payload interface{}
}

// EventCoalescer batches and coalesces resource events within a frame window.
// Like a game engine's frame buffer, it collects all events within ~16ms
// and emits them as a single batch, deduplicating updates to the same resource.
//
// Benefits:
// - Reduces IPC overhead (fewer Go->JS bridge calls)
// - Natural deduplication (rapid updates to same resource = one event)
// - Enables batch React updates on frontend
// - Maintains 60fps responsiveness (16ms max latency)
//
// Large cluster support:
//   - maxBatchSize caps each flush to prevent massive IPC payloads
//   - Remaining events carry over and auto-drain via re-armed timer
//   - The frame interval backs off while batches are large and recovers when
//     the load drops (see nextAdaptiveInterval)
//   - While the window is hidden the interval is slow (SetBackground)
//
// Ordering and delivery:
// Producers never run the bridge call (JSON marshal, JS escaping, main thread
// hop) themselves. A flush or DELETE appends an emission to an ordered queue
// while holding mu, and a single drain goroutine delivers the queue in FIFO
// order. Because every transition (pending map -> queue) happens under mu, a
// MODIFIED that was flushed before a DELETE is queued before it and delivered
// before it, and a MODIFIED still pending when the DELETE arrives is purged
// from the map first, so a DELETE is never overtaken by an older event for the
// same key. Watch goroutines only ever contend on a short critical section.
type EventCoalescer struct {
	app           *App
	frameInterval time.Duration // configured (user) interval
	maxBatchSize  int
	events        map[string]ResourceEvent // key: resourceType:namespace:name
	mu            sync.Mutex
	timer         *time.Timer
	pending       bool

	// adaptiveInterval is the load-driven interval, 0 when idle. Always >=
	// frameInterval when non-zero.
	adaptiveInterval time.Duration
	background       bool

	// Ordered delivery. outq, queued, draining and waiters are guarded by mu.
	outq     []queuedEmission
	queued   int // enqueued but not yet delivered or dropped
	draining bool
	waiters  []chan struct{}

	// onForeground runs (outside mu) when the window returns from background mode.
	onForeground func()

	// gen invalidates queued and in-flight emissions on Clear. emitMu is held
	// around each delivery so Clear can wait for an in-flight one: once Clear
	// returns, nothing from before it reaches the frontend.
	gen    atomic.Uint64
	emitMu sync.Mutex
}

// NewEventCoalescer creates a new event coalescer with the given frame interval.
// Default interval is 16ms (~60fps). Use 0 for immediate emission (bypass coalescing).
// Default maxBatchSize is 500.
func NewEventCoalescer(app *App, frameInterval time.Duration) *EventCoalescer {
	if frameInterval == 0 {
		frameInterval = 16 * time.Millisecond
	}
	return &EventCoalescer{
		app:           app,
		frameInterval: frameInterval,
		maxBatchSize:  500,
		events:        make(map[string]ResourceEvent, 64), // Pre-size for typical batch
	}
}

// nextAdaptiveInterval returns the interval to use after a flush.
//
// Heavy flushes (the batch hit the cap, or filled at least a quarter of it)
// double the interval up to maxAdaptiveInterval, which both lets more updates
// to the same key coalesce and cuts how often the bridge is invoked. Light
// flushes (under a tenth of the cap) halve it, and once it falls back to the
// configured interval 0 is returned, meaning "no adaptation". Moderate
// batches hold the current value so it does not oscillate.
func nextAdaptiveInterval(current, base time.Duration, batchLen, maxBatch int, capped bool) time.Duration {
	if current < base {
		current = base
	}
	heavy := capped || (maxBatch > 0 && batchLen*4 >= maxBatch)
	if heavy {
		next := current * 2
		ceiling := maxAdaptiveInterval
		if base > ceiling {
			ceiling = base
		}
		if next > ceiling {
			next = ceiling
		}
		return next
	}
	light := maxBatch <= 0 || batchLen*10 < maxBatch
	if light {
		next := current / 2
		if next <= base {
			return 0
		}
		return next
	}
	if current == base {
		return 0
	}
	return current
}

// currentIntervalLocked is the delay to arm the next frame timer with.
func (c *EventCoalescer) currentIntervalLocked() time.Duration {
	if c.background {
		if c.frameInterval > backgroundFrameInterval {
			return c.frameInterval
		}
		return backgroundFrameInterval
	}
	if c.adaptiveInterval > c.frameInterval {
		return c.adaptiveInterval
	}
	return c.frameInterval
}

// armLocked (re)starts the frame timer. Caller holds mu.
func (c *EventCoalescer) armLocked() {
	if c.timer != nil {
		c.timer.Stop()
	}
	c.pending = true
	c.timer = time.AfterFunc(c.currentIntervalLocked(), c.onTimer)
}

// Emit queues a resource event for coalesced emission.
// If this is the first event in a frame, starts the frame timer.
// Subsequent events within the frame window are batched.
// Updates to the same resource within a frame are coalesced (latest wins).
//
// DELETE events bypass coalescing and are queued for delivery immediately.
// Any pending ADDED/MODIFIED for the same resource is removed from the buffer
// first. Otherwise the sequence could be:
//  1. MODIFIED queued in buffer
//  2. DELETE arrives -> delivered -> frontend removes resource
//  3. Frame timer fires -> MODIFIED delivered -> frontend adds resource back!
//
// The purge and the enqueue happen under the same lock acquisition as flush's
// map-to-queue move, so the queue order is the happens-before order.
func (c *EventCoalescer) Emit(event ResourceEvent) {
	c.mu.Lock()

	// Fast path: if coalescing is disabled (frameInterval very small), emit immediately
	if c.frameInterval < time.Millisecond {
		c.mu.Unlock()
		c.emitDirect(event)
		return
	}

	key := c.eventKey(event)

	if event.Type == "DELETED" {
		delete(c.events, key)
		c.enqueueLocked("resource-event", event)
		c.mu.Unlock()
		return
	}

	// Store event (overwrites previous if same key - that's the coalescing magic)
	c.events[key] = event

	// If no timer running, start one
	if !c.pending {
		c.armLocked()
	}

	c.mu.Unlock()
}

// eventKey generates a unique key for coalescing.
// Events with the same key are deduplicated (latest wins).
func (c *EventCoalescer) eventKey(event ResourceEvent) string {
	// Format: resourceType:namespace:name
	// Using : as separator since it's not valid in k8s names
	return event.ResourceType + ":" + event.Namespace + ":" + event.Name
}

// enqueueLocked appends an emission and makes sure a drain goroutine runs.
// Caller holds mu.
func (c *EventCoalescer) enqueueLocked(name string, payload interface{}) {
	c.outq = append(c.outq, queuedEmission{gen: c.gen.Load(), name: name, payload: payload})
	c.queued++
	if !c.draining {
		c.draining = true
		go c.drain()
	}
}

// drain delivers queued emissions in order. At most one runs at a time (the
// draining flag is flipped under mu), and it exits when the queue is empty so
// idle coalescers hold no goroutine.
func (c *EventCoalescer) drain() {
	for {
		c.mu.Lock()
		if len(c.outq) == 0 {
			c.draining = false
			c.releaseWaitersLocked()
			c.mu.Unlock()
			return
		}
		batch := c.outq
		c.outq = nil
		c.mu.Unlock()

		for i := range batch {
			c.deliver(batch[i])
			batch[i] = queuedEmission{} // do not retain payloads while the rest drain

			c.mu.Lock()
			c.queued--
			if c.queued == 0 {
				c.releaseWaitersLocked()
			}
			c.mu.Unlock()
		}
	}
}

// deliver performs one bridge call unless the emission predates a Clear.
func (c *EventCoalescer) deliver(e queuedEmission) {
	c.emitMu.Lock()
	defer c.emitMu.Unlock()
	defer func() {
		if r := recover(); r != nil {
			crashlog.LogError("PANIC delivering %s: %v", e.name, r)
		}
	}()
	if e.gen != c.gen.Load() {
		return
	}
	c.app.emitEvent(e.name, e.payload)
}

func (c *EventCoalescer) releaseWaitersLocked() {
	if c.queued != 0 || len(c.waiters) == 0 {
		return
	}
	for _, w := range c.waiters {
		close(w)
	}
	c.waiters = nil
}

// waitDrained blocks until everything queued so far has been delivered, or
// the timeout elapses (a wedged bridge must not hang shutdown).
func (c *EventCoalescer) waitDrained(timeout time.Duration) {
	c.mu.Lock()
	if c.queued == 0 {
		c.mu.Unlock()
		return
	}
	w := make(chan struct{})
	c.waiters = append(c.waiters, w)
	c.mu.Unlock()

	t := time.NewTimer(timeout)
	defer t.Stop()
	select {
	case <-w:
	case <-t.C:
	}
}

// onTimer is the frame timer callback.
func (c *EventCoalescer) onTimer() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.events) == 0 {
		c.pending = false
		return
	}

	// The bridge is behind: leave events in the map where they keep coalescing,
	// and stretch the interval so we stop waking up for nothing.
	if c.queued >= maxQueuedEmissions {
		c.adaptiveInterval = nextAdaptiveInterval(c.adaptiveInterval, c.frameInterval, c.maxBatchSize, c.maxBatchSize, true)
		c.armLocked()
		return
	}

	c.flushLocked(true)
}

// flush synchronously moves one batch to the delivery queue and waits for it
// to be delivered. Used by FlushNow and tests; the timer path does not wait.
func (c *EventCoalescer) flush() {
	c.mu.Lock()
	c.flushLocked(false)
	c.mu.Unlock()
	c.waitDrained(flushWaitTimeout)
}

// flushLocked moves up to maxBatchSize pending events to the delivery queue
// and re-arms the timer for any remainder. adapt feeds the batch size into
// the adaptive interval (timer flushes only). Caller holds mu.
func (c *EventCoalescer) flushLocked(adapt bool) {
	// Nothing to emit
	if len(c.events) == 0 {
		c.pending = false
		return
	}

	n := len(c.events)
	if c.maxBatchSize > 0 && n > c.maxBatchSize {
		n = c.maxBatchSize
	}

	batch := make([]ResourceEvent, 0, n)
	for k, event := range c.events {
		if c.maxBatchSize > 0 && len(batch) >= c.maxBatchSize {
			break
		}
		batch = append(batch, event)
		delete(c.events, k)
	}

	capped := len(c.events) > 0
	if adapt {
		c.adaptiveInterval = nextAdaptiveInterval(c.adaptiveInterval, c.frameInterval, len(batch), c.maxBatchSize, capped)
	}

	// If there are remaining events, re-arm the timer for the next batch
	if capped {
		c.armLocked()
	} else {
		c.pending = false
	}

	if len(batch) == 1 {
		// Single event - emit directly for backward compatibility
		c.enqueueLocked("resource-event", batch[0])
	} else {
		// Multiple events - emit as batch
		c.enqueueLocked("resource-events-batch", batch)
	}
}

// emitDirect bypasses coalescing and emits immediately.
// Used when coalescing is disabled or for critical events.
func (c *EventCoalescer) emitDirect(event ResourceEvent) {
	c.app.emitEvent("resource-event", event)
}

// flushAllLocked drains every pending event into the queue regardless of
// maxBatchSize and stops the timer. Caller holds mu.
func (c *EventCoalescer) flushAllLocked() {
	for len(c.events) > 0 {
		c.flushLocked(false)
	}
	if c.timer != nil {
		c.timer.Stop()
	}
	c.pending = false
}

// FlushNow forces immediate emission of all pending events and waits for
// delivery. Drains all events regardless of maxBatchSize.
// Useful for cleanup or when switching contexts.
func (c *EventCoalescer) FlushNow() {
	c.mu.Lock()
	c.flushAllLocked()
	c.mu.Unlock()
	c.waitDrained(flushWaitTimeout)
}

// Clear discards all pending events without emitting them.
// Used on context switch to prevent stale events from the old cluster
// being emitted into the new context's state. Emissions that were already
// queued but not delivered are dropped too, and Clear waits out a delivery
// that is mid-flight, so nothing from before the call arrives after it.
func (c *EventCoalescer) Clear() {
	c.mu.Lock()
	c.gen.Add(1)
	if c.timer != nil {
		c.timer.Stop()
		c.timer = nil
	}
	for k := range c.events {
		delete(c.events, k)
	}
	c.queued -= len(c.outq)
	c.outq = nil
	c.releaseWaitersLocked()
	c.pending = false
	c.adaptiveInterval = 0
	c.mu.Unlock()

	// Wait for an in-flight delivery of the old generation to finish.
	c.emitMu.Lock()
	//nolint:staticcheck // empty critical section is the point: it is a barrier
	c.emitMu.Unlock()
}

// Stats returns current coalescer statistics.
func (c *EventCoalescer) Stats() (pending int, frameMs float64) {
	c.mu.Lock()
	pending = len(c.events)
	c.mu.Unlock()
	return pending, float64(c.frameInterval.Milliseconds())
}

// SetFrameInterval updates the frame interval for batching.
// Takes effect on the next frame. Value is clamped to 1-100ms.
func (c *EventCoalescer) SetFrameInterval(ms int) {
	if ms < 1 {
		ms = 1
	}
	if ms > 100 {
		ms = 100
	}
	c.mu.Lock()
	c.frameInterval = time.Duration(ms) * time.Millisecond
	if c.adaptiveInterval != 0 && c.adaptiveInterval < c.frameInterval {
		c.adaptiveInterval = 0
	}
	c.mu.Unlock()
}

// SetMaxBatchSize updates the maximum number of events emitted per flush.
// Value is clamped to 50-5000. Default is 500.
func (c *EventCoalescer) SetMaxBatchSize(n int) {
	if n < 50 {
		n = 50
	}
	if n > 5000 {
		n = 5000
	}
	c.mu.Lock()
	c.maxBatchSize = n
	c.mu.Unlock()
}

// SetBackground switches between the normal cadence and a slow one used while
// the window is hidden, when nobody can see updates and every bridge call is
// wasted main-thread work. Leaving background mode flushes everything pending
// immediately so the UI is current the moment it reappears. DELETEs are not
// delayed in either mode.
func (c *EventCoalescer) SetBackground(on bool) {
	c.mu.Lock()
	if c.background == on {
		c.mu.Unlock()
		return
	}
	c.background = on
	hook := c.onForeground
	if !on {
		c.adaptiveInterval = 0
		c.flushAllLocked()
	}
	c.mu.Unlock()
	if !on && hook != nil {
		hook()
	}
}

// FrameDelay is the delay the next flush is armed with: the configured interval, the adaptive
// one under load, or the slow background cadence. Other batchers (view notices) follow it so
// the whole UI shares one rhythm.
func (c *EventCoalescer) FrameDelay() time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.currentIntervalLocked()
}

// SetForegroundHook registers a function called when the window leaves background mode, after
// the coalescer has flushed what it held.
func (c *EventCoalescer) SetForegroundHook(f func()) {
	c.mu.Lock()
	c.onForeground = f
	c.mu.Unlock()
}
