package main

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"kubikles/pkg/events"
)

// emission is one call to the emitter: its event name and the events it carried.
type emission struct {
	name   string
	events []ResourceEvent
}

type emissionLog struct {
	mu   sync.Mutex
	list []emission
}

func (l *emissionLog) emitter() events.Emitter {
	return events.EmitterFunc(func(name string, data ...interface{}) {
		l.mu.Lock()
		defer l.mu.Unlock()
		if len(data) == 0 {
			return
		}
		switch v := data[0].(type) {
		case ResourceEvent:
			l.list = append(l.list, emission{name, []ResourceEvent{v}})
		case []ResourceEvent:
			l.list = append(l.list, emission{name, append([]ResourceEvent(nil), v...)})
		}
	})
}

func (l *emissionLog) emissions() []emission {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]emission(nil), l.list...)
}

// flat returns every emitted event in emission order.
func (l *emissionLog) flat() []ResourceEvent {
	var out []ResourceEvent
	for _, e := range l.emissions() {
		out = append(out, e.events...)
	}
	return out
}

func newLoggedCoalescer(interval time.Duration) (*EventCoalescer, *emissionLog) {
	l := &emissionLog{}
	app := &App{}
	app.emitter = l.emitter()
	return NewEventCoalescer(app, interval), l
}

func versionedEvent(eventType, name string, version int) ResourceEvent {
	e := makeTestEventWithType(eventType, "pods", "default", name)
	m := resourceMap(e)
	m["version"] = version
	e.Resource, _ = json.Marshal(m)
	return e
}

func versionOf(e ResourceEvent) int {
	v, _ := resourceMap(e)["version"].(float64)
	return int(v)
}

// A burst larger than maxBatchSize must be split without losing or repeating
// any key, and each emission must respect the cap.
func TestEventCoalescerOrdering_BurstSplitEmitsEachKeyExactlyOnce(t *testing.T) {
	c, log := newLoggedCoalescer(time.Hour) // never fires on its own
	c.SetMaxBatchSize(50)

	const total = 173
	for i := 0; i < total; i++ {
		c.Emit(versionedEvent("ADDED", fmt.Sprintf("pod-%03d", i), i))
	}

	// Drive flushes by hand, as the re-armed timer would.
	for range [4]struct{}{} {
		c.flush()
	}
	// flush re-arms a one-hour timer for any remainder; stop it.
	c.Clear()

	emissions := log.emissions()
	sizes := make([]int, len(emissions))
	seen := map[string]int{}
	for i, e := range emissions {
		sizes[i] = len(e.events)
		if len(e.events) > 50 {
			t.Fatalf("emission %d carries %d events, cap is 50", i, len(e.events))
		}
		for _, ev := range e.events {
			seen[resourceNameOf(ev)]++
		}
	}
	if want := []int{50, 50, 50, 23}; fmt.Sprint(sizes) != fmt.Sprint(want) {
		t.Fatalf("emission sizes = %v, want %v", sizes, want)
	}
	if len(seen) != total {
		t.Fatalf("emitted %d distinct keys, want %d", len(seen), total)
	}
	for k, n := range seen {
		if n != 1 {
			t.Fatalf("key %s emitted %d times", k, n)
		}
	}
}

func resourceNameOf(e ResourceEvent) string {
	return e.Name
}

// Emission names: a lone event uses the legacy single-event channel, two or
// more use the batch channel. The remainder of a split burst follows the same
// rule, so a final batch of one is a single event.
func TestEventCoalescerOrdering_EmissionNamesFollowBatchSize(t *testing.T) {
	c, log := newLoggedCoalescer(time.Hour)
	c.SetMaxBatchSize(50)
	for i := 0; i < 51; i++ {
		c.Emit(versionedEvent("ADDED", fmt.Sprintf("p%02d", i), i))
	}
	c.flush()
	c.flush()
	c.Clear()

	em := log.emissions()
	if len(em) != 2 {
		t.Fatalf("emissions = %d, want 2", len(em))
	}
	if em[0].name != "resource-events-batch" || len(em[0].events) != 50 {
		t.Fatalf("first emission = %s x%d", em[0].name, len(em[0].events))
	}
	if em[1].name != "resource-event" || len(em[1].events) != 1 {
		t.Fatalf("second emission = %s x%d", em[1].name, len(em[1].events))
	}
}

// Updates to a key that is still waiting for a later batch must coalesce with
// the newest value; a key re-emitted after its batch left must come out again.
func TestEventCoalescerOrdering_UpdatesDuringSplitKeepLatestAndNothingIsLost(t *testing.T) {
	c, log := newLoggedCoalescer(time.Hour)
	c.SetMaxBatchSize(50)
	const total = 120
	for i := 0; i < total; i++ {
		c.Emit(versionedEvent("ADDED", fmt.Sprintf("pod-%03d", i), 1))
	}
	c.flush() // 50 keys leave, 70 remain
	first := log.flat()
	if len(first) != 50 {
		t.Fatalf("first flush emitted %d", len(first))
	}
	emittedFirst := map[string]bool{}
	for _, e := range first {
		emittedFirst[resourceNameOf(e)] = true
	}

	// Update every key to version 2 while the remainder is pending.
	for i := 0; i < total; i++ {
		c.Emit(versionedEvent("MODIFIED", fmt.Sprintf("pod-%03d", i), 2))
	}
	c.FlushNow()
	c.Clear()

	latest := map[string]int{}
	count := map[string]int{}
	for _, e := range log.flat() {
		n := resourceNameOf(e)
		latest[n] = versionOf(e)
		count[n]++
	}
	if len(latest) != total {
		t.Fatalf("distinct keys emitted = %d, want %d", len(latest), total)
	}
	for n, v := range latest {
		if v != 2 {
			t.Fatalf("%s last emitted at version %d, want 2", n, v)
		}
		want := 1
		if emittedFirst[n] {
			want = 2 // already left once as version 1, then again as version 2
		}
		if count[n] != want {
			t.Fatalf("%s emitted %d times, want %d", n, count[n], want)
		}
	}
}

func TestEventCoalescerOrdering_DeleteAfterFlushFollowsTheFlushedEvent(t *testing.T) {
	c, log := newLoggedCoalescer(time.Hour)
	c.Emit(versionedEvent("MODIFIED", "x", 1))
	c.flush()
	c.Emit(makeTestEventWithType("DELETED", "pods", "default", "x"))
	c.waitDrained(2 * time.Second) // DELETE is queued for the emitter goroutine, not sent inline

	got := log.flat()
	if len(got) != 2 || got[0].Type != "MODIFIED" || got[1].Type != "DELETED" {
		t.Fatalf("emitted %v, want MODIFIED then DELETED", typesOf(got))
	}
}

func TestEventCoalescerOrdering_DeleteBeforeFlushSuppressesPendingEvent(t *testing.T) {
	c, log := newLoggedCoalescer(time.Hour)
	c.Emit(versionedEvent("ADDED", "x", 1))
	c.Emit(versionedEvent("MODIFIED", "x", 2))
	c.Emit(makeTestEventWithType("DELETED", "pods", "default", "x"))
	c.flush()

	got := log.flat()
	if len(got) != 1 || got[0].Type != "DELETED" {
		t.Fatalf("emitted %v, want only DELETED", typesOf(got))
	}
}

// A resource recreated with the same name after a delete must still be
// delivered, after the DELETED.
func TestEventCoalescerOrdering_RecreateAfterDeleteIsDeliveredAfterIt(t *testing.T) {
	c, log := newLoggedCoalescer(time.Hour)
	c.Emit(versionedEvent("ADDED", "x", 1))
	c.Emit(makeTestEventWithType("DELETED", "pods", "default", "x"))
	c.Emit(versionedEvent("ADDED", "x", 2))
	c.flush()

	got := log.flat()
	if len(got) != 2 || got[0].Type != "DELETED" || got[1].Type != "ADDED" || versionOf(got[1]) != 2 {
		t.Fatalf("emitted %v, want DELETED then ADDED(v2)", typesOf(got))
	}
}

func TestEventCoalescerOrdering_DeleteDoesNotAffectOtherKeys(t *testing.T) {
	c, log := newLoggedCoalescer(time.Hour)
	c.Emit(versionedEvent("ADDED", "keep", 1))
	c.Emit(versionedEvent("ADDED", "drop", 1))
	// Same name in another namespace and another resource type must survive.
	other := versionedEvent("ADDED", "drop", 1)
	other.Namespace = "other"
	c.Emit(other)
	otherType := versionedEvent("ADDED", "drop", 1)
	otherType.ResourceType = "services"
	c.Emit(otherType)
	c.Emit(makeTestEventWithType("DELETED", "pods", "default", "drop"))
	c.FlushNow()
	c.Clear()

	names := map[string]bool{}
	for _, e := range log.flat() {
		names[e.ResourceType+"/"+e.Namespace+"/"+resourceNameOf(e)+"/"+e.Type] = true
	}
	if !names["pods/default/keep/ADDED"] || !names["pods/other/drop/ADDED"] || !names["services/default/drop/ADDED"] {
		t.Fatalf("unrelated keys lost: %v", names)
	}
	if names["pods/default/drop/ADDED"] {
		t.Fatalf("deleted key still emitted ADDED: %v", names)
	}
}

// With a split burst, deleting keys that sit in later batches must stop them
// from appearing, and deleting keys that already left must follow them.
func TestEventCoalescerOrdering_DeleteAcrossBatchBoundary(t *testing.T) {
	c, log := newLoggedCoalescer(time.Hour)
	c.SetMaxBatchSize(50)
	const total = 130
	for i := 0; i < total; i++ {
		c.Emit(versionedEvent("ADDED", fmt.Sprintf("pod-%03d", i), 1))
	}
	c.flush() // some keys leave, the rest stay pending
	left := map[string]bool{}
	for _, e := range log.flat() {
		left[resourceNameOf(e)] = true
	}
	for i := 0; i < total; i++ {
		c.Emit(makeTestEventWithType("DELETED", "pods", "default", fmt.Sprintf("pod-%03d", i)))
	}
	c.FlushNow()
	c.Clear()

	lastType := map[string]string{}
	added := map[string]int{}
	for _, e := range log.flat() {
		n := resourceNameOf(e)
		lastType[n] = e.Type
		if e.Type == "ADDED" {
			added[n]++
		}
	}
	if len(lastType) != total {
		t.Fatalf("keys with any event = %d, want %d", len(lastType), total)
	}
	for n, typ := range lastType {
		if typ != "DELETED" {
			t.Fatalf("%s final event is %s, want DELETED", n, typ)
		}
		if added[n] > 1 {
			t.Fatalf("%s ADDED %d times", n, added[n])
		}
		if left[n] && added[n] != 1 {
			t.Fatalf("%s left in the first batch but ADDED seen %d times", n, added[n])
		}
		if !left[n] && added[n] != 0 {
			t.Fatalf("%s was still pending when deleted but ADDED was emitted", n)
		}
	}
}

// Concurrent writers and timer flushes: for every key the final delivered
// event must be the DELETED that was emitted last, never a stale update.
func TestEventCoalescerOrdering_ConcurrentUpdatesThenDeleteNeverResurrect(t *testing.T) {
	for iter := 0; iter < 20; iter++ {
		c, log := newLoggedCoalescer(2 * time.Millisecond)
		c.SetMaxBatchSize(50)

		const writers, keysPerWriter, updates = 4, 40, 5
		var wg sync.WaitGroup
		for w := 0; w < writers; w++ {
			wg.Add(1)
			go func(w int) {
				defer wg.Done()
				for k := 0; k < keysPerWriter; k++ {
					name := fmt.Sprintf("w%d-k%02d", w, k)
					c.Emit(versionedEvent("ADDED", name, 0))
					for u := 1; u <= updates; u++ {
						c.Emit(versionedEvent("MODIFIED", name, u))
					}
					c.Emit(makeTestEventWithType("DELETED", "pods", "default", name))
				}
			}(w)
		}
		wg.Wait()
		c.FlushNow()
		time.Sleep(10 * time.Millisecond) // let any already-armed timer run
		c.FlushNow()

		last := map[string]string{}
		maxVersion := map[string]int{}
		for _, e := range log.flat() {
			n := resourceNameOf(e)
			if last[n] == "DELETED" && e.Type != "DELETED" {
				t.Fatalf("iteration %d: %s received %s after DELETED", iter, n, e.Type)
			}
			last[n] = e.Type
			if v := versionOf(e); v > maxVersion[n] {
				maxVersion[n] = v
			}
		}
		if len(last) != writers*keysPerWriter {
			t.Fatalf("iteration %d: %d keys delivered, want %d", iter, len(last), writers*keysPerWriter)
		}
		for n, typ := range last {
			if typ != "DELETED" {
				t.Fatalf("iteration %d: %s final event %s", iter, n, typ)
			}
		}
	}
}

// Without deletes, a concurrent burst with timer-driven batch splitting must
// deliver every key at least once and the newest version last.
func TestEventCoalescerOrdering_ConcurrentBurstDeliversNewestVersionOfEveryKey(t *testing.T) {
	c, log := newLoggedCoalescer(2 * time.Millisecond)
	c.SetMaxBatchSize(50)
	const writers, keys, rounds = 4, 60, 4
	var wg sync.WaitGroup
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for r := 1; r <= rounds; r++ {
				for k := 0; k < keys; k++ {
					c.Emit(versionedEvent("MODIFIED", fmt.Sprintf("w%d-k%02d", w, k), r))
				}
			}
		}(w)
	}
	wg.Wait()
	c.FlushNow()
	time.Sleep(10 * time.Millisecond)
	c.FlushNow()

	latest := map[string]int{}
	for _, e := range log.flat() {
		latest[resourceNameOf(e)] = versionOf(e)
	}
	if len(latest) != writers*keys {
		t.Fatalf("keys delivered = %d, want %d", len(latest), writers*keys)
	}
	for n, v := range latest {
		if v != rounds {
			t.Fatalf("%s last delivered version %d, want %d", n, v, rounds)
		}
	}
}

func typesOf(events []ResourceEvent) []string {
	out := make([]string, len(events))
	for i, e := range events {
		out[i] = e.Type
	}
	return out
}
