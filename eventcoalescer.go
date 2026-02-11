package main

import (
	"sync"
	"time"
)

// EventCoalescer batches and coalesces resource events within a frame window.
// Like a game engine's frame buffer, it collects all events within ~16ms
// and emits them as a single batch, deduplicating updates to the same resource.
//
// Benefits:
// - Reduces IPC overhead (fewer Go->JS bridge calls)
// - Natural deduplication (rapid updates to same resource = one event)
// - Enables batch React updates on frontend
// - Maintains 60fps responsiveness (16ms max latency)
type EventCoalescer struct {
	app           *App
	frameInterval time.Duration
	events        map[string]ResourceEvent // key: resourceType:namespace:name
	mu            sync.Mutex
	timer         *time.Timer
	pending       bool
}

// NewEventCoalescer creates a new event coalescer with the given frame interval.
// Default interval is 16ms (~60fps). Use 0 for immediate emission (bypass coalescing).
func NewEventCoalescer(app *App, frameInterval time.Duration) *EventCoalescer {
	if frameInterval == 0 {
		frameInterval = 16 * time.Millisecond
	}
	return &EventCoalescer{
		app:           app,
		frameInterval: frameInterval,
		events:        make(map[string]ResourceEvent, 64), // Pre-size for typical batch
	}
}

// Emit queues a resource event for coalesced emission.
// If this is the first event in a frame, starts the frame timer.
// Subsequent events within the frame window are batched.
// Updates to the same resource within a frame are coalesced (latest wins).
//
// IMPORTANT: DELETE events are emitted immediately and bypass coalescing.
// This prevents a race where MODIFIED events arriving after DELETE could
// overwrite it (since coalescing key doesn't include event type), causing
// deleted resources to never disappear from the UI.
//
// Both flush() and DELETE emit while holding the mutex to guarantee ordering.
// Since Wails EventsEmit is non-blocking (posts to JS event queue), this is
// safe and prevents the race where flush() releases the lock, DELETE emits,
// then flush() emits a stale MODIFIED that re-adds the deleted resource.
func (c *EventCoalescer) Emit(event ResourceEvent) {
	// Fast path: if coalescing is disabled (frameInterval very small), emit immediately
	if c.frameInterval < time.Millisecond {
		c.emitDirect(event)
		return
	}

	// DELETE events must be emitted immediately - never coalesce them.
	// Additionally, we must remove any pending ADDED/MODIFIED events for this
	// resource from the buffer. Otherwise the sequence could be:
	// 1. MODIFIED queued in buffer
	// 2. DELETE arrives -> emitted immediately -> frontend removes resource
	// 3. Frame timer fires -> MODIFIED emitted -> frontend adds resource back!
	//
	// Emit under lock to guarantee ordering with flush() - prevents the race
	// where flush copies MODIFIED, releases lock, DELETE emits first, then
	// flush emits stale MODIFIED causing ghost resources.
	if event.Type == "DELETED" {
		key := c.eventKey(event)
		c.mu.Lock()
		delete(c.events, key)
		c.app.emitEvent("resource-event", event)
		c.mu.Unlock()
		return
	}

	c.mu.Lock()

	// Generate coalescing key: type:namespace:name
	key := c.eventKey(event)

	// Store event (overwrites previous if same key - that's the coalescing magic)
	c.events[key] = event

	// If no timer running, start one
	if !c.pending {
		c.pending = true
		c.timer = time.AfterFunc(c.frameInterval, c.flush)
	}

	c.mu.Unlock()
}

// eventKey generates a unique key for coalescing.
// Events with the same key are deduplicated (latest wins).
func (c *EventCoalescer) eventKey(event ResourceEvent) string {
	// Extract name from resource metadata
	name := ""
	if metadata, ok := event.Resource["metadata"].(map[string]interface{}); ok {
		if n, ok := metadata["name"].(string); ok {
			name = n
		}
	}
	// Format: resourceType:namespace:name
	// Using : as separator since it's not valid in k8s names
	return event.ResourceType + ":" + event.Namespace + ":" + name
}

// flush emits all batched events and resets the buffer.
// Called by the frame timer.
//
// Emits while holding the lock to guarantee ordering with DELETE events.
// Since Wails EventsEmit is non-blocking (posts to JS event queue),
// holding the lock during emit is safe and prevents the race where
// a DELETE slips in between buffer copy and emit.
func (c *EventCoalescer) flush() {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Nothing to emit
	if len(c.events) == 0 {
		c.pending = false
		return
	}

	// Collect events into slice
	batch := make([]ResourceEvent, 0, len(c.events))
	for _, event := range c.events {
		batch = append(batch, event)
	}

	// Clear the map (reuse underlying memory)
	for k := range c.events {
		delete(c.events, k)
	}
	c.pending = false

	// Emit while holding lock - guarantees ordering with DELETE events
	if len(batch) == 1 {
		// Single event - emit directly for backward compatibility
		c.app.emitEvent("resource-event", batch[0])
	} else {
		// Multiple events - emit as batch
		c.app.emitEvent("resource-events-batch", batch)
	}
}

// emitDirect bypasses coalescing and emits immediately.
// Used when coalescing is disabled or for critical events.
func (c *EventCoalescer) emitDirect(event ResourceEvent) {
	c.app.emitEvent("resource-event", event)
}

// FlushNow forces immediate emission of any pending events.
// Useful for cleanup or when switching contexts.
func (c *EventCoalescer) FlushNow() {
	c.mu.Lock()
	if c.timer != nil {
		c.timer.Stop()
	}
	c.mu.Unlock()

	c.flush()
}

// Clear discards all pending events without emitting them.
// Used on context switch to prevent stale events from the old cluster
// being emitted into the new context's state.
func (c *EventCoalescer) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.timer != nil {
		c.timer.Stop()
		c.timer = nil
	}
	for k := range c.events {
		delete(c.events, k)
	}
	c.pending = false
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
	c.mu.Unlock()
}
