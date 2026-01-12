package main

import (
	"sync"
	"testing"
	"time"
)

func TestNewLogCoalescer_DefaultInterval(t *testing.T) {
	app := &App{}
	c := NewLogCoalescer(app, 0)

	if c.frameInterval != 16*time.Millisecond {
		t.Errorf("expected default frame interval 16ms, got %v", c.frameInterval)
	}
	if c.app != app {
		t.Error("app reference not set correctly")
	}
	if c.streams == nil {
		t.Error("streams map not initialized")
	}
}

func TestNewLogCoalescer_CustomInterval(t *testing.T) {
	app := &App{}
	c := NewLogCoalescer(app, 50*time.Millisecond)

	if c.frameInterval != 50*time.Millisecond {
		t.Errorf("expected custom frame interval 50ms, got %v", c.frameInterval)
	}
}

func TestLogCoalescer_EmitLine_CreatesBuffer(t *testing.T) {
	app := &App{}
	c := NewLogCoalescer(app, 100*time.Millisecond)

	c.EmitLine("stream-1", "test line")

	c.mu.Lock()
	buf, exists := c.streams["stream-1"]
	c.mu.Unlock()

	if !exists {
		t.Fatal("expected buffer to be created for stream-1")
	}
	if len(buf.lines) != 1 {
		t.Errorf("expected 1 line, got %d", len(buf.lines))
	}
	if buf.lines[0] != "test line" {
		t.Errorf("expected 'test line', got %q", buf.lines[0])
	}
	if !buf.pending {
		t.Error("expected pending to be true")
	}
}

func TestLogCoalescer_EmitLine_BatchesLines(t *testing.T) {
	app := &App{}
	c := NewLogCoalescer(app, 100*time.Millisecond)

	// Emit multiple lines to same stream
	c.EmitLine("stream-1", "line 1")
	c.EmitLine("stream-1", "line 2")
	c.EmitLine("stream-1", "line 3")

	c.mu.Lock()
	buf := c.streams["stream-1"]
	lineCount := len(buf.lines)
	c.mu.Unlock()

	if lineCount != 3 {
		t.Errorf("expected 3 lines batched, got %d", lineCount)
	}
}

func TestLogCoalescer_MultipleStreams(t *testing.T) {
	app := &App{}
	c := NewLogCoalescer(app, 100*time.Millisecond)

	// Emit to multiple streams
	c.EmitLine("stream-1", "line 1a")
	c.EmitLine("stream-2", "line 2a")
	c.EmitLine("stream-1", "line 1b")
	c.EmitLine("stream-3", "line 3a")
	c.EmitLine("stream-2", "line 2b")

	c.mu.Lock()
	stream1 := c.streams["stream-1"]
	stream2 := c.streams["stream-2"]
	stream3 := c.streams["stream-3"]
	c.mu.Unlock()

	if len(stream1.lines) != 2 {
		t.Errorf("stream-1: expected 2 lines, got %d", len(stream1.lines))
	}
	if len(stream2.lines) != 2 {
		t.Errorf("stream-2: expected 2 lines, got %d", len(stream2.lines))
	}
	if len(stream3.lines) != 1 {
		t.Errorf("stream-3: expected 1 line, got %d", len(stream3.lines))
	}
}

func TestLogCoalescer_FlushStream(t *testing.T) {
	app := &App{}
	c := NewLogCoalescer(app, 1*time.Hour) // Long interval

	c.EmitLine("stream-1", "line 1")
	c.EmitLine("stream-1", "line 2")
	c.EmitLine("stream-2", "line A")

	// Force flush stream-1
	c.FlushStream("stream-1")

	c.mu.Lock()
	buf1 := c.streams["stream-1"]
	buf2 := c.streams["stream-2"]
	c.mu.Unlock()

	// stream-1 should be flushed (empty lines, pending false)
	if len(buf1.lines) != 0 {
		t.Errorf("stream-1 should be empty after flush, got %d lines", len(buf1.lines))
	}
	if buf1.pending {
		t.Error("stream-1 pending should be false after flush")
	}

	// stream-2 should be unaffected
	if len(buf2.lines) != 1 {
		t.Errorf("stream-2 should still have 1 line, got %d", len(buf2.lines))
	}
}

func TestLogCoalescer_EmitDone(t *testing.T) {
	app := &App{}
	c := NewLogCoalescer(app, 1*time.Hour)

	c.EmitLine("stream-1", "line 1")
	c.EmitLine("stream-1", "line 2")

	// Emit done should flush and cleanup
	c.EmitDone("stream-1")

	c.mu.Lock()
	_, exists := c.streams["stream-1"]
	c.mu.Unlock()

	if exists {
		t.Error("stream-1 buffer should be removed after EmitDone")
	}
}

func TestLogCoalescer_EmitDone_NonexistentStream(t *testing.T) {
	app := &App{}
	c := NewLogCoalescer(app, 100*time.Millisecond)

	// Should not panic
	c.EmitDone("nonexistent-stream")
}

func TestLogCoalescer_EmitError(t *testing.T) {
	app := &App{}
	c := NewLogCoalescer(app, 1*time.Hour)

	c.EmitLine("stream-1", "line 1")

	// Emit error should flush pending lines first
	c.EmitError("stream-1", "connection lost")

	c.mu.Lock()
	buf := c.streams["stream-1"]
	c.mu.Unlock()

	// Buffer should be empty (flushed)
	if len(buf.lines) != 0 {
		t.Errorf("expected empty buffer after error, got %d lines", len(buf.lines))
	}
}

func TestLogCoalescer_Cleanup(t *testing.T) {
	app := &App{}
	c := NewLogCoalescer(app, 1*time.Hour)

	c.EmitLine("stream-1", "line 1")
	c.EmitLine("stream-2", "line 2")

	// Cleanup stream-1 without emitting
	c.Cleanup("stream-1")

	c.mu.Lock()
	_, exists1 := c.streams["stream-1"]
	_, exists2 := c.streams["stream-2"]
	c.mu.Unlock()

	if exists1 {
		t.Error("stream-1 should be removed after cleanup")
	}
	if !exists2 {
		t.Error("stream-2 should still exist")
	}
}

func TestLogCoalescer_Cleanup_NonexistentStream(t *testing.T) {
	app := &App{}
	c := NewLogCoalescer(app, 100*time.Millisecond)

	// Should not panic
	c.Cleanup("nonexistent-stream")
}

func TestLogCoalescer_TimerFlushes(t *testing.T) {
	app := &App{}
	c := NewLogCoalescer(app, 20*time.Millisecond)

	c.EmitLine("stream-1", "line 1")
	c.EmitLine("stream-1", "line 2")

	// Wait for timer to fire
	time.Sleep(50 * time.Millisecond)

	c.mu.Lock()
	buf := c.streams["stream-1"]
	lineCount := len(buf.lines)
	pending := buf.pending
	c.mu.Unlock()

	if lineCount != 0 {
		t.Errorf("expected 0 lines after timer flush, got %d", lineCount)
	}
	if pending {
		t.Error("pending should be false after timer flush")
	}
}

func TestLogCoalescer_ConcurrentEmit(t *testing.T) {
	app := &App{}
	c := NewLogCoalescer(app, 100*time.Millisecond)

	var wg sync.WaitGroup
	numGoroutines := 10
	linesPerGoroutine := 100

	// Spawn goroutines that emit lines concurrently
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			streamID := "stream-" + string(rune('a'+id))
			for j := 0; j < linesPerGoroutine; j++ {
				c.EmitLine(streamID, "line")
			}
		}(i)
	}

	wg.Wait()

	// Should have numGoroutines streams, each with linesPerGoroutine lines
	c.mu.Lock()
	streamCount := len(c.streams)
	for _, buf := range c.streams {
		if len(buf.lines) != linesPerGoroutine {
			t.Errorf("expected %d lines per stream, got %d", linesPerGoroutine, len(buf.lines))
		}
	}
	c.mu.Unlock()

	if streamCount != numGoroutines {
		t.Errorf("expected %d streams, got %d", numGoroutines, streamCount)
	}
}

func TestLogCoalescer_ConcurrentSameStream(t *testing.T) {
	app := &App{}
	c := NewLogCoalescer(app, 100*time.Millisecond)

	var wg sync.WaitGroup
	numGoroutines := 10
	linesPerGoroutine := 100

	// All goroutines emit to the same stream
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < linesPerGoroutine; j++ {
				c.EmitLine("shared-stream", "line")
			}
		}()
	}

	wg.Wait()

	c.mu.Lock()
	buf := c.streams["shared-stream"]
	lineCount := len(buf.lines)
	c.mu.Unlock()

	expectedLines := numGoroutines * linesPerGoroutine
	if lineCount != expectedLines {
		t.Errorf("expected %d lines, got %d", expectedLines, lineCount)
	}
}

func TestLogCoalescer_LineOrder(t *testing.T) {
	app := &App{}
	c := NewLogCoalescer(app, 100*time.Millisecond)

	// Emit lines in order
	c.EmitLine("stream-1", "line-1")
	c.EmitLine("stream-1", "line-2")
	c.EmitLine("stream-1", "line-3")

	c.mu.Lock()
	buf := c.streams["stream-1"]
	lines := make([]string, len(buf.lines))
	copy(lines, buf.lines)
	c.mu.Unlock()

	expected := []string{"line-1", "line-2", "line-3"}
	for i, line := range lines {
		if line != expected[i] {
			t.Errorf("line %d: expected %q, got %q", i, expected[i], line)
		}
	}
}

func TestLogCoalescer_BufferPreallocation(t *testing.T) {
	app := &App{}
	c := NewLogCoalescer(app, 100*time.Millisecond)

	c.EmitLine("stream-1", "line")

	c.mu.Lock()
	buf := c.streams["stream-1"]
	capacity := cap(buf.lines)
	c.mu.Unlock()

	// Buffer should be pre-allocated to at least 64
	if capacity < 64 {
		t.Errorf("expected buffer capacity >= 64, got %d", capacity)
	}
}

func TestLogCoalescer_FlushNonexistentStream(t *testing.T) {
	app := &App{}
	c := NewLogCoalescer(app, 100*time.Millisecond)

	// Should not panic
	c.FlushStream("nonexistent")
}

func TestLogCoalescer_EmitAfterFlush(t *testing.T) {
	app := &App{}
	c := NewLogCoalescer(app, 100*time.Millisecond)

	c.EmitLine("stream-1", "line-1")
	c.FlushStream("stream-1")
	c.EmitLine("stream-1", "line-2")

	c.mu.Lock()
	buf := c.streams["stream-1"]
	lineCount := len(buf.lines)
	c.mu.Unlock()

	// Should have new line after flush
	if lineCount != 1 {
		t.Errorf("expected 1 line after re-emit, got %d", lineCount)
	}
}

func TestLogCoalescer_RapidEmitDone(t *testing.T) {
	app := &App{}
	c := NewLogCoalescer(app, 100*time.Millisecond)

	// Rapid emit and done
	for i := 0; i < 100; i++ {
		streamID := "stream"
		c.EmitLine(streamID, "line")
		c.EmitDone(streamID)
	}

	// All streams should be cleaned up
	c.mu.Lock()
	count := len(c.streams)
	c.mu.Unlock()

	if count != 0 {
		t.Errorf("expected 0 streams after rapid emit/done, got %d", count)
	}
}

func TestLogStreamBatchEvent_Fields(t *testing.T) {
	event := LogStreamBatchEvent{
		StreamID: "test-stream",
		Lines:    []string{"line1", "line2", "line3"},
	}

	if event.StreamID != "test-stream" {
		t.Errorf("expected StreamID 'test-stream', got %q", event.StreamID)
	}
	if len(event.Lines) != 3 {
		t.Errorf("expected 3 lines, got %d", len(event.Lines))
	}
}

func TestLogCoalescer_HighThroughput(t *testing.T) {
	app := &App{}
	c := NewLogCoalescer(app, 16*time.Millisecond) // Realistic 60fps

	// Simulate high-throughput logging (10k lines in 100ms)
	done := make(chan struct{})
	go func() {
		for i := 0; i < 10000; i++ {
			c.EmitLine("high-volume", "log line")
		}
		close(done)
	}()

	select {
	case <-done:
		// Success - completed without deadlock
	case <-time.After(5 * time.Second):
		t.Fatal("high throughput test timed out - possible deadlock")
	}
}

func TestLogCoalescer_MultipleTimerCycles(t *testing.T) {
	app := &App{}
	c := NewLogCoalescer(app, 10*time.Millisecond)

	// Emit, wait for flush, emit again
	c.EmitLine("stream-1", "cycle-1")
	time.Sleep(30 * time.Millisecond)

	c.EmitLine("stream-1", "cycle-2")
	time.Sleep(30 * time.Millisecond)

	c.EmitLine("stream-1", "cycle-3")
	time.Sleep(30 * time.Millisecond)

	// Buffer should be empty after all cycles
	c.mu.Lock()
	buf := c.streams["stream-1"]
	lineCount := len(buf.lines)
	c.mu.Unlock()

	if lineCount != 0 {
		t.Errorf("expected 0 lines after multiple cycles, got %d", lineCount)
	}
}
