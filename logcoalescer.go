package main

import (
	"sync"
	"time"
)

// LogCoalescer batches log stream lines within a frame window.
// Busy pods can emit thousands of lines/second - this batches them
// into ~60 updates/second, dramatically reducing IPC overhead while
// maintaining smooth scrolling.
//
// Each stream has its own buffer, so multiple log streams don't interfere.
type LogCoalescer struct {
	app           *App
	frameInterval time.Duration
	streams       map[string]*logStreamBuffer
	mu            sync.Mutex
	// emitMu serializes take-then-emit so concurrent flushes of one stream
	// (timer, FlushStream, SetBackground, EmitDone) cannot reorder lines.
	// Lock order: emitMu before mu. EmitLine never takes it.
	emitMu     sync.Mutex
	background bool // window hidden: use backgroundFrameInterval
}

// logStreamBuffer holds pending lines for a single log stream
type logStreamBuffer struct {
	streamID string
	lines    []string
	timer    *time.Timer
	pending  bool
}

// NewLogCoalescer creates a new log coalescer.
// Default frame interval is 16ms (~60fps).
func NewLogCoalescer(app *App, frameInterval time.Duration) *LogCoalescer {
	if frameInterval == 0 {
		frameInterval = 16 * time.Millisecond
	}
	return &LogCoalescer{
		app:           app,
		frameInterval: frameInterval,
		streams:       make(map[string]*logStreamBuffer),
	}
}

// intervalLocked is the flush delay for new timers. Caller holds mu.
func (c *LogCoalescer) intervalLocked() time.Duration {
	if c.background && c.frameInterval < backgroundFrameInterval {
		return backgroundFrameInterval
	}
	return c.frameInterval
}

// SetBackground slows flushing while the window is hidden and, on return to
// the foreground, flushes every stream's buffered lines immediately.
func (c *LogCoalescer) SetBackground(on bool) {
	c.mu.Lock()
	if c.background == on {
		c.mu.Unlock()
		return
	}
	c.background = on
	var ids []string
	if !on {
		ids = make([]string, 0, len(c.streams))
		for id := range c.streams {
			ids = append(ids, id)
		}
	}
	c.mu.Unlock()

	for _, id := range ids {
		c.FlushStream(id)
	}
}

// EmitLine queues a log line for batched emission.
// Lines within the same frame are batched together.
func (c *LogCoalescer) EmitLine(streamID, line string) {
	c.mu.Lock()

	// Get or create buffer for this stream
	buf, exists := c.streams[streamID]
	if !exists {
		buf = &logStreamBuffer{
			streamID: streamID,
			lines:    make([]string, 0, 64), // Pre-size for typical batch
		}
		c.streams[streamID] = buf
	}

	// Append line
	buf.lines = append(buf.lines, line)

	// Start timer if not already running
	if !buf.pending {
		buf.pending = true
		buf.timer = time.AfterFunc(c.intervalLocked(), func() {
			c.flushStream(streamID)
		})
	}

	c.mu.Unlock()
}

// EmitDone emits a done event for a stream and cleans up.
func (c *LogCoalescer) EmitDone(streamID string) {
	// Held through the Done event so it cannot overtake a concurrent flush.
	c.emitMu.Lock()
	defer c.emitMu.Unlock()

	c.mu.Lock()
	var lines []string
	if buf, exists := c.streams[streamID]; exists {
		if buf.timer != nil {
			buf.timer.Stop()
		}
		lines = buf.lines
		buf.lines = nil
		delete(c.streams, streamID)
	}
	c.mu.Unlock()

	if len(lines) > 0 {
		c.emitBatch(streamID, lines)
	}
	c.app.emitEvent("log-stream", LogStreamEvent{
		StreamID: streamID,
		Done:     true,
	})
}

// EmitError emits an error event for a stream.
func (c *LogCoalescer) EmitError(streamID, errMsg string) {
	// Flush pending lines first
	c.FlushStream(streamID)

	c.app.emitEvent("log-stream", LogStreamEvent{
		StreamID: streamID,
		Error:    errMsg,
	})
}

// flushStream flushes pending lines for a specific stream.
func (c *LogCoalescer) flushStream(streamID string) {
	c.emitMu.Lock()
	defer c.emitMu.Unlock()
	c.mu.Lock()

	buf, exists := c.streams[streamID]
	if !exists || len(buf.lines) == 0 {
		if exists {
			buf.pending = false
		}
		c.mu.Unlock()
		return
	}

	// Take the lines and reset buffer
	lines := buf.lines
	buf.lines = make([]string, 0, 64)
	buf.pending = false

	c.mu.Unlock()

	// Emit outside mu (producers keep appending) but inside emitMu (ordering).
	c.emitBatch(streamID, lines)
}

// FlushStream forces immediate flush of a stream's pending lines.
func (c *LogCoalescer) FlushStream(streamID string) {
	c.mu.Lock()
	if buf, exists := c.streams[streamID]; exists {
		if buf.timer != nil {
			buf.timer.Stop()
		}
	}
	c.mu.Unlock()

	c.flushStream(streamID)
}

// emitBatch emits a batch of log lines.
func (c *LogCoalescer) emitBatch(streamID string, lines []string) {
	if len(lines) == 0 {
		return
	}

	if len(lines) == 1 {
		// Single line - emit directly for minimal overhead
		c.app.emitEvent("log-stream", LogStreamEvent{
			StreamID: streamID,
			Line:     lines[0],
		})
	} else {
		// Multiple lines - emit as batch
		c.app.emitEvent("log-stream-batch", LogStreamBatchEvent{
			StreamID: streamID,
			Lines:    lines,
		})
	}
}

// Cleanup removes a stream's buffer without emitting.
func (c *LogCoalescer) Cleanup(streamID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if buf, exists := c.streams[streamID]; exists {
		if buf.timer != nil {
			buf.timer.Stop()
		}
		delete(c.streams, streamID)
	}
}

// LogStreamBatchEvent is emitted when multiple log lines are batched together.
type LogStreamBatchEvent struct {
	StreamID string   `json:"streamId"`
	Lines    []string `json:"lines"`
}
