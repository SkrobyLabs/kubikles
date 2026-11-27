package terminal

import (
	"sync"
	"time"
	"unicode/utf8"
)

// Terminal output is bridged to the frontend as JSON strings, one IPC event
// each. Forwarding every PTY read (a few KB at most) costs thousands of events
// per second for heavy output, and a multi-byte rune split between two reads is
// turned into U+FFFD by the JSON encoder. outputCoalescer fixes both:
//
//   - the first bytes after an idle period are emitted immediately, so typing
//     echo is not delayed;
//   - while output keeps flowing, chunks are merged and emitted at most once per
//     interval, or earlier once maxBytes are buffered;
//   - an incomplete trailing UTF-8 sequence is held back until its remaining
//     bytes arrive (or carryTimeout expires, so a truncated sequence cannot
//     stall output forever).
const (
	outputFlushInterval = 12 * time.Millisecond
	outputMaxBytes      = 48 * 1024
	outputCarryTimeout  = time.Second
)

type outputCoalescer struct {
	interval     time.Duration
	maxBytes     int
	carryTimeout time.Duration
	emit         func(data string)

	// mu is held across emit so chunks reach the bridge in order; a slow bridge
	// then applies backpressure to the PTY reader instead of growing a queue.
	mu          sync.Mutex
	pending     []byte
	carry       []byte
	lastEmit    time.Time
	carryUntil  time.Time
	timer       *time.Timer
	timerAt     time.Time
	stopped     bool
	emittedMsgs int
}

func newOutputCoalescer(emit func(string)) *outputCoalescer {
	return &outputCoalescer{
		interval:     outputFlushInterval,
		maxBytes:     outputMaxBytes,
		carryTimeout: outputCarryTimeout,
		emit:         emit,
	}
}

// Write accepts raw PTY bytes. It never retains p.
func (c *outputCoalescer) Write(p []byte) {
	if len(p) == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stopped {
		return
	}

	data := p
	if len(c.carry) > 0 {
		data = append(append(make([]byte, 0, len(c.carry)+len(p)), c.carry...), p...)
		c.carry = c.carry[:0]
	}
	cut := completeUTF8Prefix(data)
	c.pending = append(c.pending, data[:cut]...)
	if cut < len(data) {
		c.carry = append(c.carry, data[cut:]...)
		c.carryUntil = time.Now().Add(c.carryTimeout)
	}

	if len(c.pending) == 0 {
		c.armLocked(c.carryTimeout)
		return
	}
	now := time.Now()
	since := now.Sub(c.lastEmit)
	if len(c.pending) >= c.maxBytes || since >= c.interval {
		c.flushLocked(now)
		if len(c.carry) > 0 {
			c.armLocked(c.carryTimeout)
		}
		return
	}
	c.armLocked(c.interval - since)
}

// Flush emits everything buffered, including an incomplete trailing sequence.
func (c *outputCoalescer) Flush() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.flushAllLocked()
}

// Close flushes remaining output and stops the coalescer.
func (c *outputCoalescer) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.flushAllLocked()
	c.shutdownLocked()
}

// Discard stops the coalescer without emitting (the session was closed by the user).
func (c *outputCoalescer) Discard() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pending, c.carry = nil, nil
	c.shutdownLocked()
}

func (c *outputCoalescer) shutdownLocked() {
	c.stopped = true
	if c.timer != nil {
		c.timer.Stop()
		c.timer = nil
	}
}

func (c *outputCoalescer) flushAllLocked() {
	if len(c.carry) > 0 {
		c.pending = append(c.pending, c.carry...)
		c.carry = c.carry[:0]
	}
	c.flushLocked(time.Now())
}

func (c *outputCoalescer) flushLocked(now time.Time) {
	if len(c.pending) == 0 {
		return
	}
	s := string(c.pending)
	c.pending = c.pending[:0]
	if cap(c.pending) > 4*c.maxBytes {
		c.pending = nil
	}
	c.lastEmit = now
	c.emittedMsgs++
	c.emit(s)
}

// armLocked schedules onTimer after d unless an earlier timer is already set.
func (c *outputCoalescer) armLocked(d time.Duration) {
	if d < 0 {
		d = 0
	}
	at := time.Now().Add(d)
	if c.timer != nil && !c.timerAt.IsZero() && !c.timerAt.After(at) {
		return
	}
	if c.timer != nil {
		c.timer.Stop()
	}
	c.timerAt = at
	c.timer = time.AfterFunc(d, c.onTimer)
}

func (c *outputCoalescer) onTimer() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.timer = nil
	c.timerAt = time.Time{}
	if c.stopped {
		return
	}
	now := time.Now()
	c.flushLocked(now)
	if len(c.carry) > 0 {
		if !now.Before(c.carryUntil) {
			c.flushAllLocked()
			return
		}
		c.armLocked(c.carryUntil.Sub(now))
	}
}

// completeUTF8Prefix returns the length of the longest prefix of b that does not
// end inside a multi-byte UTF-8 sequence. Invalid bytes are treated as complete
// so garbage can never be held back.
func completeUTF8Prefix(b []byte) int {
	n := len(b)
	for back := 1; back <= utf8.UTFMax-1 && back <= n; back++ {
		c := b[n-back]
		if c < 0x80 {
			return n
		}
		if utf8.RuneStart(c) {
			need := 0
			switch {
			case c >= 0xF0 && c <= 0xF4:
				need = 4
			case c >= 0xE0 && c <= 0xEF:
				need = 3
			case c >= 0xC2 && c <= 0xDF:
				need = 2
			}
			if need > back {
				return n - back
			}
			return n
		}
	}
	return n
}
