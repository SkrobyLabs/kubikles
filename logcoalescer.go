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
		buf.timer = time.AfterFunc(c.frameInterval, func() {
			c.flushStream(streamID)
		})
	}

	c.mu.Unlock()
}

// EmitDone emits a done event for a stream and cleans up.
func (c *LogCoalescer) EmitDone(streamID string) {
	c.mu.Lock()

	// Flush any pending lines first
	if buf, exists := c.streams[streamID]; exists {
		if buf.timer != nil {
			buf.timer.Stop()
		}
		// Flush outside lock
		lines := buf.lines
		buf.lines = nil
		delete(c.streams, streamID)
		c.mu.Unlock()

		// Emit any remaining lines
		if len(lines) > 0 {
			c.emitBatch(streamID, lines)
		}

		// Emit done
		c.app.emitEvent("log-stream", LogStreamEvent{
			StreamID: streamID,
			Done:     true,
		})
		return
	}

	c.mu.Unlock()

	// No buffer existed, just emit done
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

	// Emit outside lock
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
