package main

import (
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"kubikles/pkg/events"
)

// gatedEmitter records emissions and can stall delivery to model a slow
// bridge (Wails evaluateJavaScript on the main thread).
type gatedEmitter struct {
	mu      sync.Mutex
	got     []ResourceEvent
	names   []string
	gate    chan struct{} // when non-nil, every call blocks until it is closed
	perCall time.Duration
	calls   atomic.Int64
}

func (g *gatedEmitter) emitter() events.Emitter {
	return events.EmitterFunc(func(name string, data ...interface{}) {
		g.calls.Add(1)
		g.mu.Lock()
		gate := g.gate
		g.mu.Unlock()
		if gate != nil {
			<-gate
		}
		if g.perCall > 0 {
			time.Sleep(g.perCall)
		}
		g.mu.Lock()
		defer g.mu.Unlock()
		g.names = append(g.names, name)
		switch v := data[0].(type) {
		case ResourceEvent:
			g.got = append(g.got, v)
		case []ResourceEvent:
			g.got = append(g.got, v...)
		}
	})
}

func (g *gatedEmitter) events() []ResourceEvent {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]ResourceEvent(nil), g.got...)
}

func newGatedCoalescer(interval time.Duration, g *gatedEmitter) *EventCoalescer {
	app := &App{}
	app.emitter = g.emitter()
	return NewEventCoalescer(app, interval)
}

func TestNextAdaptiveInterval(t *testing.T) {
	const base = 16 * time.Millisecond
	ms := func(n int) time.Duration { return time.Duration(n) * time.Millisecond }
	tests := []struct {
		name     string
		current  time.Duration
		batch    int
		maxBatch int
		capped   bool
		want     time.Duration
	}{
		{"small batch stays at base", 0, 3, 500, false, 0},
		{"small batch recovers from backed off", ms(64), 3, 500, false, ms(32)},
		{"recovery reaches base and clears", ms(32), 3, 500, false, 0},
		{"capped flush backs off from base", 0, 500, 500, true, ms(32)},
		{"capped flush doubles", ms(32), 500, 500, true, ms(64)},
		{"large batch backs off", 0, 150, 500, false, ms(32)},
		{"back off is capped", ms(200), 500, 500, true, maxAdaptiveInterval},
		{"already at cap stays", maxAdaptiveInterval, 500, 500, true, maxAdaptiveInterval},
		{"moderate batch holds", ms(64), 60, 500, false, ms(64)},
		{"moderate batch at base clears", 0, 60, 500, false, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nextAdaptiveInterval(tt.current, base, tt.batch, tt.maxBatch, tt.capped); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
	// A configured interval above the ceiling is never reduced by backoff.
	if got := nextAdaptiveInterval(0, 400*time.Millisecond, 500, 500, true); got != 400*time.Millisecond {
		t.Fatalf("base above ceiling: got %v", got)
	}
}

func TestEventCoalescerAdaptiveBacksOffAndRecovers(t *testing.T) {
	g := &gatedEmitter{}
	c := newGatedCoalescer(16*time.Millisecond, g)
	c.SetMaxBatchSize(50)
	// Flush by hand; the frame timer is stopped so it cannot interleave.
	flush := func() {
		c.mu.Lock()
		if c.timer != nil {
			c.timer.Stop()
		}
		c.flushLocked(true)
		if c.timer != nil {
			c.timer.Stop()
		}
		c.mu.Unlock()
	}

	for i := 0; i < 120; i++ {
		c.Emit(makeTestEvent("pods", "ns", fmt.Sprintf("p%03d", i), ""))
	}
	flush()
	c.mu.Lock()
	backed := c.adaptiveInterval
	c.mu.Unlock()
	if backed <= c.frameInterval {
		t.Fatalf("capped flush did not back off: adaptive=%v base=%v", backed, c.frameInterval)
	}
	c.mu.Lock()
	next := c.currentIntervalLocked()
	c.mu.Unlock()
	if next != backed {
		t.Fatalf("armed interval = %v, want the backed-off %v", next, backed)
	}

	// Drain the rest, then light flushes bring it back to the base interval.
	c.FlushNow()
	for i := 0; i < 8; i++ {
		c.Emit(makeTestEvent("pods", "ns", "tiny", fmt.Sprint(i)))
		flush()
	}
	c.mu.Lock()
	recovered := c.adaptiveInterval
	c.mu.Unlock()
	if recovered != 0 {
		t.Fatalf("adaptive interval did not recover, still %v", recovered)
	}
	c.Clear()
}

func TestEventCoalescerSmallBatchesKeepBaseInterval(t *testing.T) {
	g := &gatedEmitter{}
	c := newGatedCoalescer(5*time.Millisecond, g)
	for i := 0; i < 20; i++ {
		c.Emit(makeTestEvent("pods", "ns", fmt.Sprintf("p%d", i), ""))
		time.Sleep(8 * time.Millisecond)
	}
	c.FlushNow()
	c.mu.Lock()
	adaptive := c.adaptiveInterval
	c.mu.Unlock()
	if adaptive != 0 {
		t.Fatalf("light traffic must stay at the base interval, adaptive=%v", adaptive)
	}
}

// Emit and DELETE must never wait on the bridge: a stalled emitter used to
// hold the coalescer mutex and freeze every watch goroutine.
func TestEventCoalescerEmitDoesNotBlockOnSlowBridge(t *testing.T) {
	g := &gatedEmitter{gate: make(chan struct{})}
	c := newGatedCoalescer(time.Millisecond, g)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 200; i++ {
			c.Emit(makeTestEvent("pods", "ns", fmt.Sprintf("p%03d", i), "v"))
			if i%10 == 0 {
				c.Emit(makeTestEventWithType("DELETED", "pods", "ns", fmt.Sprintf("p%03d", i)))
			}
		}
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Emit blocked while the bridge was stalled")
	}

	close(g.gate)
	c.FlushNow()
	if len(g.events()) == 0 {
		t.Fatal("nothing delivered after the bridge recovered")
	}
}

// While the bridge is behind, flushes pause and events keep coalescing in the
// pending map (latest wins) instead of queueing stale versions.
func TestEventCoalescerBackpressureKeepsCoalescing(t *testing.T) {
	g := &gatedEmitter{gate: make(chan struct{})}
	c := newGatedCoalescer(2*time.Millisecond, g)

	// Fill the delivery queue: each distinct burst becomes one emission.
	for round := 0; round < maxQueuedEmissions; round++ {
		c.Emit(makeTestEvent("pods", "ns", fmt.Sprintf("seed-%d", round), "v"))
		deadline := time.Now().Add(2 * time.Second)
		for {
			c.mu.Lock()
			n := len(c.events)
			q := c.queued
			c.mu.Unlock()
			if n == 0 && q > round || time.Now().After(deadline) {
				break
			}
			time.Sleep(time.Millisecond)
		}
	}

	// Now the queue is at its bound; further updates to one key must collapse.
	for v := 0; v < 50; v++ {
		c.Emit(makeTestEvent("pods", "ns", "hot", fmt.Sprint(v)))
	}
	time.Sleep(40 * time.Millisecond) // several timer ticks
	c.mu.Lock()
	pending := len(c.events)
	queued := c.queued
	c.mu.Unlock()
	if pending != 1 {
		t.Fatalf("hot key should stay coalesced in the map, pending=%d", pending)
	}
	if queued > maxQueuedEmissions+1 {
		t.Fatalf("queue grew past its bound: %d", queued)
	}

	close(g.gate)
	c.FlushNow()
	var hot []string
	for _, e := range g.events() {
		if e.Name == "hot" {
			hot = append(hot, string(resourceMap(e)["data"].(string)))
		}
	}
	if len(hot) != 1 || hot[0] != "49" {
		t.Fatalf("hot key deliveries = %v, want exactly the latest version", hot)
	}
}

// The core ordering guarantee under -race: per key, delivered versions are
// strictly increasing, and the last delivery is the last thing emitted.
// Keys are recreated after DELETE, timers fire concurrently, and the bridge
// is slow enough to hold emissions in the queue while new ones arrive.
func TestEventCoalescerOrderingStress(t *testing.T) {
	g := &gatedEmitter{perCall: 50 * time.Microsecond}
	c := newGatedCoalescer(time.Millisecond, g)
	c.SetMaxBatchSize(50)

	const producers = 8
	const keysPerProducer = 25
	type lastOp struct {
		typ     string
		version int
	}
	final := make([]map[string]lastOp, producers)

	var wg sync.WaitGroup
	for p := 0; p < producers; p++ {
		final[p] = map[string]lastOp{}
		wg.Add(1)
		go func(p int) {
			defer wg.Done()
			rng := rand.New(rand.NewSource(int64(p) + 1))
			version := 0
			for round := 0; round < 40; round++ {
				for k := 0; k < keysPerProducer; k++ {
					name := fmt.Sprintf("p%d-k%02d", p, k)
					typ := "MODIFIED"
					switch rng.Intn(6) {
					case 0:
						typ = "ADDED"
					case 1:
						typ = "DELETED"
					}
					version++
					e := makeTestEventWithType(typ, "pods", "ns", name)
					e.Resource = rawResource(t, map[string]interface{}{"metadata": map[string]interface{}{"name": name}, "version": version})
					c.Emit(e)
					final[p][name] = lastOp{typ, version}
				}
				if round%7 == 0 {
					time.Sleep(time.Duration(rng.Intn(3)) * time.Millisecond)
				}
			}
		}(p)
	}
	wg.Wait()
	c.FlushNow()

	lastDelivered := map[string]int{}
	lastType := map[string]string{}
	for _, e := range g.events() {
		v := int(resourceMap(e)["version"].(float64))
		if prev, ok := lastDelivered[e.Name]; ok && v <= prev {
			t.Fatalf("%s delivered version %d after %d: ordering violated", e.Name, v, prev)
		}
		lastDelivered[e.Name] = v
		lastType[e.Name] = e.Type
	}
	for p := range final {
		for name, op := range final[p] {
			if lastDelivered[name] != op.version || lastType[name] != op.typ {
				t.Fatalf("%s: final delivery was v%d %s, want v%d %s", name, lastDelivered[name], lastType[name], op.version, op.typ)
			}
		}
	}
}

// A DELETE can never be overtaken by an older event for the same key, even
// when the older event was flushed just before it.
func TestEventCoalescerDeleteNeverOvertaken(t *testing.T) {
	for iter := 0; iter < 200; iter++ {
		g := &gatedEmitter{}
		c := newGatedCoalescer(time.Millisecond, g)
		c.Emit(versionedEvent("MODIFIED", "x", 1))
		if iter%2 == 0 {
			time.Sleep(time.Duration(iter%5) * 500 * time.Microsecond) // land around the timer
		}
		c.Emit(makeTestEventWithType("DELETED", "pods", "default", "x"))
		c.FlushNow()
		time.Sleep(3 * time.Millisecond) // any stray timer flush would show up now
		c.FlushNow()

		got := g.events()
		deleted := false
		for _, e := range got {
			if e.Type == "DELETED" {
				deleted = true
			} else if deleted {
				t.Fatalf("iter %d: %s delivered after DELETED", iter, e.Type)
			}
		}
		if !deleted {
			t.Fatalf("iter %d: DELETED never delivered", iter)
		}
	}
}

func TestEventCoalescerClearDropsQueuedEmissions(t *testing.T) {
	g := &gatedEmitter{gate: make(chan struct{})}
	c := newGatedCoalescer(time.Hour, g)

	c.Emit(makeTestEvent("pods", "ns", "old-1", ""))
	c.mu.Lock()
	c.flushLocked(false) // old-1 is now in flight, blocked inside the emitter
	c.mu.Unlock()
	deadline := time.Now().Add(2 * time.Second)
	for g.calls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	// Queue more behind the stalled one.
	c.Emit(makeTestEventWithType("DELETED", "pods", "ns", "old-2"))
	c.Emit(makeTestEvent("pods", "ns", "old-3", ""))
	c.mu.Lock()
	c.flushLocked(false)
	c.mu.Unlock()

	cleared := make(chan struct{})
	go func() {
		c.Clear()
		close(cleared)
	}()
	time.Sleep(20 * time.Millisecond)
	close(g.gate) // let the in-flight delivery finish
	select {
	case <-cleared:
	case <-time.After(2 * time.Second):
		t.Fatal("Clear did not return")
	}

	c.Emit(makeTestEvent("pods", "ns", "new-1", ""))
	c.FlushNow()

	names := map[string]bool{}
	for _, e := range g.events() {
		names[e.Name] = true
	}
	if names["old-2"] || names["old-3"] {
		t.Fatalf("emissions queued before Clear were delivered after it: %v", names)
	}
	if !names["new-1"] {
		t.Fatalf("post-Clear event missing: %v", names)
	}
}

func TestEventCoalescerBackgroundIsSlowThenFlushesOnForeground(t *testing.T) {
	g := &gatedEmitter{}
	c := newGatedCoalescer(5*time.Millisecond, g)

	c.SetBackground(true)
	c.Emit(makeTestEvent("pods", "ns", "bg-1", ""))
	c.Emit(makeTestEvent("pods", "ns", "bg-2", ""))
	time.Sleep(150 * time.Millisecond)
	if n := len(g.events()); n != 0 {
		t.Fatalf("background mode delivered %d events within 150ms; interval should be about 1s", n)
	}

	// DELETE is never delayed.
	c.Emit(makeTestEventWithType("DELETED", "pods", "ns", "bg-3"))
	c.waitDrained(2 * time.Second)
	if got := g.events(); len(got) != 1 || got[0].Type != "DELETED" {
		t.Fatalf("DELETED should bypass background coalescing, got %v", typesOf(got))
	}

	start := time.Now()
	c.SetBackground(false)
	c.waitDrained(2 * time.Second)
	if time.Since(start) > 500*time.Millisecond {
		t.Fatal("foreground flush was not immediate")
	}
	names := map[string]bool{}
	for _, e := range g.events() {
		names[e.Name] = true
	}
	if !names["bg-1"] || !names["bg-2"] {
		t.Fatalf("pending events were not flushed on return to foreground: %v", names)
	}

	// Normal cadence is back.
	before := len(g.events())
	c.Emit(makeTestEvent("pods", "ns", "fg", ""))
	deadline := time.Now().Add(500 * time.Millisecond)
	for len(g.events()) == before && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	if len(g.events()) == before {
		t.Fatal("normal cadence did not resume")
	}
}

func TestEventCoalescerBackgroundHonoursLargerUserInterval(t *testing.T) {
	c := &EventCoalescer{frameInterval: 3 * time.Second, background: true}
	if got := c.currentIntervalLocked(); got != 3*time.Second {
		t.Fatalf("interval = %v; background must never be faster than the configured interval", got)
	}
}

func TestEventCoalescerSetFrameIntervalStillClamps(t *testing.T) {
	c := NewEventCoalescer(&App{}, 0)
	c.SetFrameInterval(0)
	if c.frameInterval != time.Millisecond {
		t.Fatalf("lower clamp: %v", c.frameInterval)
	}
	c.SetFrameInterval(5000)
	if c.frameInterval != 100*time.Millisecond {
		t.Fatalf("upper clamp: %v", c.frameInterval)
	}
	c.adaptiveInterval = 30 * time.Millisecond
	c.SetFrameInterval(50)
	if c.adaptiveInterval != 0 {
		t.Fatal("adaptive interval below the new base must be reset")
	}
}

func TestSetWindowVisibleTogglesBothCoalescers(t *testing.T) {
	a := &App{}
	a.eventCoalescer = NewEventCoalescer(a, 5*time.Millisecond)
	a.logCoalescer = NewLogCoalescer(a, 5*time.Millisecond)

	a.SetWindowVisible(false)
	a.eventCoalescer.mu.Lock()
	evBg := a.eventCoalescer.background
	a.eventCoalescer.mu.Unlock()
	a.logCoalescer.mu.Lock()
	logBg := a.logCoalescer.background
	a.logCoalescer.mu.Unlock()
	if !evBg || !logBg {
		t.Fatalf("hidden window did not switch to background: events=%v logs=%v", evBg, logBg)
	}

	a.SetWindowVisible(true)
	if a.eventCoalescer.background || a.logCoalescer.background {
		t.Fatal("visible window did not restore foreground")
	}

	// Safe before startup wires the coalescers.
	(&App{}).SetWindowVisible(false)
}
