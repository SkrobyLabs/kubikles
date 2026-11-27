package terminal

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"
)

type chunkLog struct {
	mu     sync.Mutex
	chunks []string
	times  []time.Time
}

func (l *chunkLog) emit(s string) {
	l.mu.Lock()
	l.chunks = append(l.chunks, s)
	l.times = append(l.times, time.Now())
	l.mu.Unlock()
}

func (l *chunkLog) snapshot() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.chunks...)
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("condition not met in time")
}

func TestCompleteUTF8Prefix(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
		want int
	}{
		{"empty", nil, 0},
		{"ascii", []byte("abc"), 3},
		{"complete euro", []byte("a\xe2\x82\xac"), 4},
		{"euro missing last", []byte("a\xe2\x82"), 1},
		{"euro lead only", []byte("a\xe2"), 1},
		{"emoji 3 of 4", []byte("x\xf0\x9f\x98"), 1},
		{"emoji 1 of 4", []byte("x\xf0"), 1},
		{"emoji complete", []byte("\xf0\x9f\x98\x80"), 4},
		{"two byte lead", []byte("\xc3"), 0},
		{"two byte complete", []byte("\xc3\xa9"), 2},
		{"stray continuation", []byte("a\x82"), 2},
		{"invalid lead is not held", []byte("a\xff"), 2},
		{"overlong lead c0 not held", []byte("a\xc0"), 2},
	}
	for _, tc := range cases {
		if got := completeUTF8Prefix(tc.in); got != tc.want {
			t.Errorf("%s: got %d want %d", tc.name, got, tc.want)
		}
	}
}

func TestOutputCoalescerFirstWriteAfterIdleIsImmediate(t *testing.T) {
	var l chunkLog
	c := newOutputCoalescer(l.emit)
	defer c.Close()
	c.Write([]byte("x"))
	if got := l.snapshot(); len(got) != 1 || got[0] != "x" {
		t.Fatalf("expected immediate emit, got %q", got)
	}
}

func TestOutputCoalescerMergesBurst(t *testing.T) {
	var l chunkLog
	c := newOutputCoalescer(l.emit)
	c.interval = 40 * time.Millisecond
	c.Write([]byte("first"))
	for i := 0; i < 50; i++ {
		c.Write([]byte("b"))
	}
	waitFor(t, func() bool { return len(strings.Join(l.snapshot(), "")) == 55 })
	chunks := l.snapshot()
	if len(chunks) > 3 {
		t.Fatalf("burst produced %d events: %q", len(chunks), chunks)
	}
	c.Close()
}

func TestOutputCoalescerFlushesAtMaxBytes(t *testing.T) {
	var l chunkLog
	c := newOutputCoalescer(l.emit)
	c.interval = time.Hour
	c.maxBytes = 1024
	c.Write([]byte("a")) // leading edge emit
	before := len(l.snapshot())
	c.Write(bytes.Repeat([]byte("z"), 2048))
	if got := len(l.snapshot()); got != before+1 {
		t.Fatalf("size trigger did not flush: %d events", got)
	}
	c.Close()
}

func TestOutputCoalescerHoldsSplitRuneAcrossWrites(t *testing.T) {
	for _, r := range []string{"€", "\U0001F600", "é"} {
		raw := []byte(r)
		for split := 1; split < len(raw); split++ {
			var l chunkLog
			c := newOutputCoalescer(l.emit)
			c.Write([]byte("ab"))
			c.Write(raw[:split])
			c.Write(raw[split:])
			c.Close()
			chunks := l.snapshot()
			for i, ch := range chunks {
				if !utf8.ValidString(ch) {
					t.Fatalf("%q split %d: chunk %d invalid: %q", r, split, i, ch)
				}
			}
			if got := strings.Join(chunks, ""); got != "ab"+r {
				t.Fatalf("%q split %d: got %q", r, split, got)
			}
		}
	}
}

func TestOutputCoalescerCarryTimesOut(t *testing.T) {
	var l chunkLog
	c := newOutputCoalescer(l.emit)
	c.carryTimeout = 30 * time.Millisecond
	c.Write([]byte("\xe2\x82"))
	if len(l.snapshot()) != 0 {
		t.Fatal("incomplete rune must be held")
	}
	waitFor(t, func() bool { return len(l.snapshot()) == 1 })
	if got := l.snapshot()[0]; got != "\xe2\x82" {
		t.Fatalf("got %q", got)
	}
	c.Close()
}

func TestOutputCoalescerCloseFlushesAndDiscardDoesNot(t *testing.T) {
	var l chunkLog
	c := newOutputCoalescer(l.emit)
	c.interval = time.Hour
	c.Write([]byte("a"))
	c.Write([]byte("tail"))
	c.Close()
	if got := strings.Join(l.snapshot(), ""); got != "atail" {
		t.Fatalf("Close lost data: %q", got)
	}
	c.Write([]byte("late"))
	if got := strings.Join(l.snapshot(), ""); got != "atail" {
		t.Fatalf("write after Close emitted: %q", got)
	}

	var d chunkLog
	c2 := newOutputCoalescer(d.emit)
	c2.interval = time.Hour
	c2.Write([]byte("a"))
	c2.Write([]byte("dropped"))
	c2.Discard()
	if got := strings.Join(d.snapshot(), ""); got != "a" {
		t.Fatalf("Discard emitted: %q", got)
	}
}

func TestOutputCoalescerConcurrentWritersKeepAllBytes(t *testing.T) {
	var l chunkLog
	c := newOutputCoalescer(l.emit)
	var wg sync.WaitGroup
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 500; i++ {
				c.Write([]byte("0123456789"))
			}
		}()
	}
	wg.Wait()
	c.Close()
	if got := len(strings.Join(l.snapshot(), "")); got != 4*500*10 {
		t.Fatalf("got %d bytes", got)
	}
}

// BenchmarkOutputCoalescerEventsPerMB feeds 1 MiB as 4 KiB reads (the old PTY
// read size, one event per read) and reports how many events the coalescer emits.
func BenchmarkOutputCoalescerEventsPerMB(b *testing.B) {
	chunk := bytes.Repeat([]byte("hello terminal output line\r\n"), 4096/28)
	const total = 1 << 20
	var events int
	for i := 0; i < b.N; i++ {
		var l chunkLog
		c := newOutputCoalescer(l.emit)
		for sent := 0; sent < total; sent += len(chunk) {
			c.Write(chunk)
		}
		c.Close()
		events = len(l.snapshot())
	}
	b.ReportMetric(float64(events), "events/MB")
	b.ReportMetric(float64(total/len(chunk)), "events/MB_before")
}
