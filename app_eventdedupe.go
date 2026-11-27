package main

import (
	"bytes"
	"hash/fnv"
	"sync/atomic"
)

// legacyEventDedupe is the opt-in (setting performance.legacyEventDedupe, default off) rule that
// drops a MODIFIED watch event whose object is byte-identical to the last one emitted for the
// same key once metadata.resourceVersion is ignored (managedFields are already stripped). It
// makes no-op updates free for the legacy full-object path; the windowed views already skip them.
//
// Objects whose content changes on every update (node heartbeats, lease renewals) are never
// dropped, because only identical content is skipped.
var legacyEventDedupe atomic.Bool

// SetLegacyEventDedupe applies the legacyEventDedupe setting.
func (a *App) SetLegacyEventDedupe(enabled bool) {
	legacyEventDedupe.Store(enabled)
}

// eventDeduper remembers a hash of the last emitted object per key for one watch loop. It is
// used from a single goroutine.
type eventDeduper struct {
	last map[string]uint64
}

func newEventDeduper() *eventDeduper { return &eventDeduper{last: make(map[string]uint64)} }

// shouldEmit reports whether the event must be delivered, and records it. ADDED always passes
// (a replay after a relist must reach the UI), DELETED passes and forgets the key, MODIFIED
// passes unless it equals the last emitted object apart from the resourceVersion.
func (d *eventDeduper) shouldEmit(eventType, namespace, name, resourceVersion string, raw []byte) bool {
	key := namespace + "/" + name
	if eventType == "DELETED" {
		delete(d.last, key)
		return true
	}
	h := hashIgnoringResourceVersion(raw, resourceVersion)
	prev, seen := d.last[key]
	d.last[key] = h
	if eventType == "MODIFIED" && seen && prev == h {
		return false
	}
	return true
}

// reset forgets everything (a relist replays state as ADDED events anyway).
func (d *eventDeduper) reset() {
	for k := range d.last {
		delete(d.last, k)
	}
}

// hashIgnoringResourceVersion hashes raw with every `"resourceVersion":"<rv>"` token left out.
// Tokens of other versions (an object that embeds another object's version) still count.
func hashIgnoringResourceVersion(raw []byte, rv string) uint64 {
	h := fnv.New64a()
	if rv == "" {
		_, _ = h.Write(raw)
		return h.Sum64()
	}
	tok := []byte(`"resourceVersion":"` + rv + `"`)
	for {
		i := bytes.Index(raw, tok)
		if i < 0 {
			_, _ = h.Write(raw)
			break
		}
		_, _ = h.Write(raw[:i])
		raw = raw[i+len(tok):]
	}
	return h.Sum64()
}
