package main

import (
	"sync"
	"testing"
	"time"
)

// Helper to create a test event with metadata
func makeTestEvent(resourceType, namespace, name string, data string) ResourceEvent {
	return ResourceEvent{
		Type:         "MODIFIED",
		ResourceType: resourceType,
		Namespace:    namespace,
		Resource: map[string]interface{}{
			"metadata": map[string]interface{}{
				"name": name,
			},
			"data": data,
		},
	}
}

// Helper to create a test event with specific type
func makeTestEventWithType(eventType, resourceType, namespace, name string) ResourceEvent {
	return ResourceEvent{
		Type:         eventType,
		ResourceType: resourceType,
		Namespace:    namespace,
		Resource: map[string]interface{}{
			"metadata": map[string]interface{}{
				"name": name,
			},
		},
	}
}

func TestNewEventCoalescer_DefaultInterval(t *testing.T) {
	app := &App{}
	c := NewEventCoalescer(app, 0)

	if c.frameInterval != 16*time.Millisecond {
		t.Errorf("expected default frame interval 16ms, got %v", c.frameInterval)
	}
	if c.app != app {
		t.Error("app reference not set correctly")
	}
	if c.events == nil {
		t.Error("events map not initialized")
	}
}

func TestNewEventCoalescer_CustomInterval(t *testing.T) {
	app := &App{}
	c := NewEventCoalescer(app, 50*time.Millisecond)

	if c.frameInterval != 50*time.Millisecond {
		t.Errorf("expected custom frame interval 50ms, got %v", c.frameInterval)
	}
}

func TestEventCoalescer_EventKey(t *testing.T) {
	c := &EventCoalescer{}

	tests := []struct {
		name     string
		event    ResourceEvent
		expected string
	}{
		{
			name:     "basic event",
			event:    makeTestEvent("pods", "default", "nginx", ""),
			expected: "pods:default:nginx",
		},
		{
			name:     "cluster-scoped resource",
			event:    makeTestEvent("nodes", "", "node-1", ""),
			expected: "nodes::node-1",
		},
		{
			name:     "event without metadata",
			event:    ResourceEvent{ResourceType: "pods", Namespace: "test", Resource: map[string]interface{}{}},
			expected: "pods:test:",
		},
		{
			name:     "event with nil resource",
			event:    ResourceEvent{ResourceType: "pods", Namespace: "test", Resource: nil},
			expected: "pods:test:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := c.eventKey(tt.event)
			if key != tt.expected {
				t.Errorf("expected key %q, got %q", tt.expected, key)
			}
		})
	}
}

func TestEventCoalescer_Coalescing(t *testing.T) {
	app := &App{} // nil ctx means no actual emission
	c := NewEventCoalescer(app, 100*time.Millisecond)

	// Emit multiple updates to the same resource
	c.Emit(makeTestEvent("pods", "default", "nginx", "version1"))
	c.Emit(makeTestEvent("pods", "default", "nginx", "version2"))
	c.Emit(makeTestEvent("pods", "default", "nginx", "version3"))

	// Check that only one event is pending (coalesced)
	c.mu.Lock()
	count := len(c.events)
	c.mu.Unlock()

	if count != 1 {
		t.Errorf("expected 1 coalesced event, got %d", count)
	}

	// Verify the latest version won
	c.mu.Lock()
	for _, event := range c.events {
		if data, ok := event.Resource["data"].(string); ok {
			if data != "version3" {
				t.Errorf("expected version3 (latest), got %s", data)
			}
		}
	}
	c.mu.Unlock()
}

func TestEventCoalescer_Batching(t *testing.T) {
	app := &App{}
	c := NewEventCoalescer(app, 100*time.Millisecond)

	// Emit events for different resources
	c.Emit(makeTestEvent("pods", "default", "nginx", ""))
	c.Emit(makeTestEvent("pods", "default", "redis", ""))
	c.Emit(makeTestEvent("services", "default", "nginx-svc", ""))
	c.Emit(makeTestEvent("deployments", "kube-system", "coredns", ""))

	// Check that all 4 events are pending
	c.mu.Lock()
	count := len(c.events)
	c.mu.Unlock()

	if count != 4 {
		t.Errorf("expected 4 batched events, got %d", count)
	}
}

func TestEventCoalescer_MixedCoalescingAndBatching(t *testing.T) {
	app := &App{}
	c := NewEventCoalescer(app, 100*time.Millisecond)

	// Emit: 3 updates to pod-a, 2 updates to pod-b, 1 update to svc-a
	c.Emit(makeTestEvent("pods", "default", "pod-a", "v1"))
	c.Emit(makeTestEvent("pods", "default", "pod-b", "v1"))
	c.Emit(makeTestEvent("pods", "default", "pod-a", "v2"))
	c.Emit(makeTestEvent("services", "default", "svc-a", "v1"))
	c.Emit(makeTestEvent("pods", "default", "pod-a", "v3"))
	c.Emit(makeTestEvent("pods", "default", "pod-b", "v2"))

	// Should have 3 events: pod-a(v3), pod-b(v2), svc-a(v1)
	c.mu.Lock()
	count := len(c.events)
	c.mu.Unlock()

	if count != 3 {
		t.Errorf("expected 3 events after coalescing, got %d", count)
	}
}

func TestEventCoalescer_FlushNow(t *testing.T) {
	app := &App{}
	c := NewEventCoalescer(app, 1*time.Hour) // Long interval so timer won't fire

	// Add some events
	c.Emit(makeTestEvent("pods", "default", "nginx", ""))
	c.Emit(makeTestEvent("pods", "default", "redis", ""))

	// Verify events are pending
	c.mu.Lock()
	if len(c.events) != 2 {
		t.Errorf("expected 2 pending events, got %d", len(c.events))
	}
	c.mu.Unlock()

	// Force flush
	c.FlushNow()

	// Verify events are cleared
	c.mu.Lock()
	if len(c.events) != 0 {
		t.Errorf("expected 0 events after flush, got %d", len(c.events))
	}
	if c.pending {
		t.Error("pending should be false after flush")
	}
	c.mu.Unlock()
}

func TestEventCoalescer_Stats(t *testing.T) {
	app := &App{}
	c := NewEventCoalescer(app, 32*time.Millisecond)

	// Add some events
	c.Emit(makeTestEvent("pods", "default", "nginx", ""))
	c.Emit(makeTestEvent("pods", "default", "redis", ""))

	pending, frameMs := c.Stats()

	if pending != 2 {
		t.Errorf("expected 2 pending events, got %d", pending)
	}
	if frameMs != 32 {
		t.Errorf("expected frameMs 32, got %f", frameMs)
	}
}

func TestEventCoalescer_FastPath(t *testing.T) {
	app := &App{}
	// Frame interval < 1ms triggers fast path (immediate emission)
	c := NewEventCoalescer(app, 500*time.Microsecond)

	// Emit an event
	c.Emit(makeTestEvent("pods", "default", "nginx", ""))

	// In fast path, events bypass the buffer entirely
	// Since app.ctx is nil, emitDirect is a no-op, but we can verify
	// the event didn't go into the buffer
	c.mu.Lock()
	count := len(c.events)
	c.mu.Unlock()

	if count != 0 {
		t.Errorf("fast path should bypass buffer, got %d events", count)
	}
}

func TestEventCoalescer_TimerFires(t *testing.T) {
	app := &App{}
	c := NewEventCoalescer(app, 20*time.Millisecond)

	// Add events
	c.Emit(makeTestEvent("pods", "default", "nginx", ""))

	// Verify pending
	c.mu.Lock()
	if !c.pending {
		t.Error("expected pending to be true")
	}
	c.mu.Unlock()

	// Wait for timer to fire
	time.Sleep(50 * time.Millisecond)

	// Verify flushed
	c.mu.Lock()
	if len(c.events) != 0 {
		t.Errorf("expected 0 events after timer, got %d", len(c.events))
	}
	if c.pending {
		t.Error("pending should be false after timer fires")
	}
	c.mu.Unlock()
}

func TestEventCoalescer_ConcurrentEmit(t *testing.T) {
	app := &App{}
	c := NewEventCoalescer(app, 100*time.Millisecond)

	var wg sync.WaitGroup
	numGoroutines := 10
	eventsPerGoroutine := 100

	// Spawn goroutines that emit events concurrently
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < eventsPerGoroutine; j++ {
				// Each goroutine emits to its own resource
				c.Emit(makeTestEvent("pods", "default", "pod-"+string(rune('a'+id)), ""))
			}
		}(i)
	}

	wg.Wait()

	// Should have exactly numGoroutines unique events (one per goroutine's pod)
	c.mu.Lock()
	count := len(c.events)
	c.mu.Unlock()

	if count != numGoroutines {
		t.Errorf("expected %d unique events, got %d", numGoroutines, count)
	}
}

func TestEventCoalescer_EmptyFlush(t *testing.T) {
	app := &App{}
	c := NewEventCoalescer(app, 100*time.Millisecond)

	// Flush with no events should be safe
	c.FlushNow()

	c.mu.Lock()
	if c.pending {
		t.Error("pending should be false after empty flush")
	}
	c.mu.Unlock()
}

func TestEventCoalescer_MultipleFlushCycles(t *testing.T) {
	app := &App{}
	c := NewEventCoalescer(app, 10*time.Millisecond)

	// Cycle 1
	c.Emit(makeTestEvent("pods", "default", "nginx", "cycle1"))
	time.Sleep(30 * time.Millisecond)

	// Cycle 2
	c.Emit(makeTestEvent("pods", "default", "nginx", "cycle2"))
	time.Sleep(30 * time.Millisecond)

	// Verify both cycles completed
	c.mu.Lock()
	if len(c.events) != 0 {
		t.Errorf("expected 0 events after multiple cycles, got %d", len(c.events))
	}
	c.mu.Unlock()
}

func TestEventCoalescer_DifferentNamespaces(t *testing.T) {
	app := &App{}
	c := NewEventCoalescer(app, 100*time.Millisecond)

	// Same name, different namespaces = different resources
	c.Emit(makeTestEvent("pods", "default", "nginx", ""))
	c.Emit(makeTestEvent("pods", "production", "nginx", ""))
	c.Emit(makeTestEvent("pods", "staging", "nginx", ""))

	c.mu.Lock()
	count := len(c.events)
	c.mu.Unlock()

	if count != 3 {
		t.Errorf("expected 3 events for different namespaces, got %d", count)
	}
}

func TestEventCoalescer_DifferentResourceTypes(t *testing.T) {
	app := &App{}
	c := NewEventCoalescer(app, 100*time.Millisecond)

	// Same name, same namespace, different types = different resources
	c.Emit(makeTestEvent("pods", "default", "nginx", ""))
	c.Emit(makeTestEvent("services", "default", "nginx", ""))
	c.Emit(makeTestEvent("deployments", "default", "nginx", ""))

	c.mu.Lock()
	count := len(c.events)
	c.mu.Unlock()

	if count != 3 {
		t.Errorf("expected 3 events for different resource types, got %d", count)
	}
}

func TestEventCoalescer_DeleteBypassesCoalescing(t *testing.T) {
	app := &App{}
	c := NewEventCoalescer(app, 100*time.Millisecond)

	// Emit a DELETE event - it should bypass the buffer entirely
	c.Emit(makeTestEventWithType("DELETED", "pods", "default", "nginx"))

	// DELETE events should NOT go into the buffer
	c.mu.Lock()
	count := len(c.events)
	c.mu.Unlock()

	if count != 0 {
		t.Errorf("DELETE events should bypass buffer, got %d events in buffer", count)
	}
}

func TestEventCoalescer_DeleteNotOverwrittenByModified(t *testing.T) {
	// This test verifies the fix for the bug where MODIFIED events
	// could overwrite DELETED events in the coalescing buffer.
	// With the fix, DELETE bypasses the buffer so this can't happen.
	app := &App{}
	c := NewEventCoalescer(app, 100*time.Millisecond)

	// Simulate: pod is being terminated
	// 1. MODIFIED (status update to Terminating)
	c.Emit(makeTestEventWithType("MODIFIED", "pods", "default", "nginx"))
	// 2. DELETED (pod removed from etcd)
	c.Emit(makeTestEventWithType("DELETED", "pods", "default", "nginx"))
	// 3. Another MODIFIED (race condition - late status update)
	c.Emit(makeTestEventWithType("MODIFIED", "pods", "default", "nginx"))

	// Only MODIFIED events should be in buffer (DELETED was emitted immediately)
	c.mu.Lock()
	count := len(c.events)
	var foundType string
	for _, e := range c.events {
		foundType = e.Type
	}
	c.mu.Unlock()

	if count != 1 {
		t.Errorf("expected 1 MODIFIED event in buffer, got %d", count)
	}
	if foundType != "MODIFIED" {
		t.Errorf("expected MODIFIED in buffer, got %s", foundType)
	}
	// The key insight: DELETED was emitted immediately and is NOT in the buffer,
	// so it can't be overwritten by subsequent MODIFIED events.
}

func TestEventCoalescer_AddedAndModifiedCoalesce(t *testing.T) {
	// ADDED and MODIFIED can safely coalesce since they're both "resource exists" events
	app := &App{}
	c := NewEventCoalescer(app, 100*time.Millisecond)

	c.Emit(makeTestEventWithType("ADDED", "pods", "default", "nginx"))
	c.Emit(makeTestEventWithType("MODIFIED", "pods", "default", "nginx"))

	c.mu.Lock()
	count := len(c.events)
	var foundType string
	for _, e := range c.events {
		foundType = e.Type
	}
	c.mu.Unlock()

	// Should coalesce to 1 event (latest wins = MODIFIED)
	if count != 1 {
		t.Errorf("expected 1 coalesced event, got %d", count)
	}
	if foundType != "MODIFIED" {
		t.Errorf("expected MODIFIED (latest), got %s", foundType)
	}
}
