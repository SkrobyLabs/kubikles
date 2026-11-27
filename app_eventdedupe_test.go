package main

import (
	"testing"
	"time"
)

func TestEventDeduperRules(t *testing.T) {
	d := newEventDeduper()
	obj := func(rv, phase string) []byte {
		return []byte(`{"metadata":{"name":"a","namespace":"ns","resourceVersion":"` + rv + `"},"status":{"phase":"` + phase + `"}}`)
	}
	if !d.shouldEmit("ADDED", "ns", "a", "1", obj("1", "Pending")) {
		t.Fatal("ADDED must always pass")
	}
	if d.shouldEmit("MODIFIED", "ns", "a", "2", obj("2", "Pending")) {
		t.Fatal("a MODIFIED that only changes resourceVersion must be dropped")
	}
	if !d.shouldEmit("MODIFIED", "ns", "a", "3", obj("3", "Running")) {
		t.Fatal("a content change must pass")
	}
	if d.shouldEmit("MODIFIED", "ns", "a", "4", obj("4", "Running")) {
		t.Fatal("repeat of the last content must be dropped")
	}
	// A different object with the same content is independent.
	if !d.shouldEmit("MODIFIED", "ns", "b", "5", obj("5", "Running")) {
		t.Fatal("first event of another key must pass")
	}
	if !d.shouldEmit("DELETED", "ns", "a", "6", obj("6", "Running")) {
		t.Fatal("DELETED must pass")
	}
	// After a delete the same content is a fresh object again.
	if !d.shouldEmit("MODIFIED", "ns", "a", "7", obj("7", "Running")) {
		t.Fatal("event after delete must pass")
	}
	d.reset()
	if !d.shouldEmit("MODIFIED", "ns", "a", "8", obj("8", "Running")) {
		t.Fatal("event after reset must pass")
	}
}

func TestHashIgnoresOnlyTheEventsResourceVersion(t *testing.T) {
	a := []byte(`{"metadata":{"resourceVersion":"5"},"x":{"resourceVersion":"9"}}`)
	b := []byte(`{"metadata":{"resourceVersion":"6"},"x":{"resourceVersion":"9"}}`)
	c := []byte(`{"metadata":{"resourceVersion":"6"},"x":{"resourceVersion":"10"}}`)
	if hashIgnoringResourceVersion(a, "5") != hashIgnoringResourceVersion(b, "6") {
		t.Fatal("objects differing only in their own resourceVersion must hash alike")
	}
	if hashIgnoringResourceVersion(b, "6") == hashIgnoringResourceVersion(c, "6") {
		t.Fatal("an embedded resourceVersion of another object is content")
	}
	if hashIgnoringResourceVersion(a, "") == 0 {
		t.Fatal("empty rv must still hash")
	}
}

func TestWatchLoopDedupeDropsResourceVersionOnlyModified(t *testing.T) {
	legacyEventDedupe.Store(true)
	t.Cleanup(func() { legacyEventDedupe.Store(false) })
	h := startWatchLoop(t, "", func(h *watchHarness, c *scriptedStream) {
		// The coalescer merges events of one key within a frame, so each step waits until the
		// previous event was emitted.
		c.send("ADDED", podObject("a", "ns1", "10"))
		waitUntil(t, "first", 5*time.Second, func() bool { return h.log.hasResource("ADDED", "a") })
		c.send("MODIFIED", podObject("a", "ns1", "11")) // only the version moved
		c.send("ADDED", podObject("s1", "ns1", "12"))
		waitUntil(t, "s1", 5*time.Second, func() bool { return h.log.hasResource("ADDED", "s1") })
		changed := podObject("a", "ns1", "13")
		changed["status"] = map[string]interface{}{"phase": "Running"}
		c.send("MODIFIED", changed)
		c.send("ADDED", podObject("sentinel", "ns1", "14"))
		c.block()
	})
	waitUntil(t, "sentinel", 5*time.Second, func() bool { return h.log.hasResource("ADDED", "sentinel") })
	modified := 0
	for _, e := range h.log.snapshot() {
		if e.Type == "MODIFIED" {
			modified++
		}
	}
	if modified != 1 {
		t.Fatalf("emitted %d MODIFIED events, want 1 (the content change only)", modified)
	}
	// The dropped event still advanced the stored version, so a reconnect resumes correctly.
	if rv := h.storedRV(); rv != "14" {
		t.Fatalf("stored resourceVersion = %q, want 14", rv)
	}
}

func TestWatchLoopWithoutDedupeEmitsEveryModified(t *testing.T) {
	legacyEventDedupe.Store(false)
	h := startWatchLoop(t, "", func(h *watchHarness, c *scriptedStream) {
		c.send("ADDED", podObject("a", "ns1", "10"))
		waitUntil(t, "first", 5*time.Second, func() bool { return h.log.hasResource("ADDED", "a") })
		c.send("MODIFIED", podObject("a", "ns1", "11"))
		c.send("ADDED", podObject("sentinel", "ns1", "13"))
		c.block()
	})
	waitUntil(t, "sentinel", 5*time.Second, func() bool { return h.log.hasResource("ADDED", "sentinel") })
	if !h.log.hasResource("MODIFIED", "a") {
		t.Fatal("with dedupe off the resourceVersion-only MODIFIED must be emitted")
	}
}
