//go:build !windows

package terminal

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"
)

// installFakeKubectl puts a "kubectl" shell script first on PATH. The script
// body is run by /bin/sh and replaces the real kubectl exec session, so the
// PTY read loop is exercised end to end without a cluster.
func installFakeKubectl(t *testing.T, body string) {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(dir, "kubectl"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

type collectedOutput struct {
	mu     sync.Mutex
	chunks []string
	done   chan struct{}
	once   sync.Once
}

func newCollectedOutput() *collectedOutput { return &collectedOutput{done: make(chan struct{})} }

func (c *collectedOutput) emit(ev TerminalEvent) {
	c.mu.Lock()
	if ev.Data != "" {
		c.chunks = append(c.chunks, ev.Data)
	}
	c.mu.Unlock()
	if ev.Done {
		c.once.Do(func() { close(c.done) })
	}
}

func (c *collectedOutput) snapshot() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.chunks...)
}

func (c *collectedOutput) wait(t *testing.T) {
	t.Helper()
	select {
	case <-c.done:
	case <-time.After(10 * time.Second):
		t.Fatal("session did not finish")
	}
}

// Baseline: plain ASCII output written by the child reaches the emitter and
// the session reports completion. Guards the fake-kubectl harness itself.
func TestSessionEmitsOutputAndDone(t *testing.T) {
	installFakeKubectl(t, `printf 'hello'`)
	out := newCollectedOutput()
	s, err := newSession("s1", SessionOptions{Namespace: "ns", Pod: "p"}, out.emit)
	if err != nil {
		t.Fatalf("newSession: %v", err)
	}
	defer s.Close()
	out.wait(t)
	if got := strings.Join(out.snapshot(), ""); got != "hello" {
		t.Fatalf("output = %q, want hello", got)
	}
}

// A multi-byte UTF-8 rune whose bytes
// arrive in separate PTY reads must not be split across emitted events,
// because each event's Data is a JSON string and invalid UTF-8 is replaced
// with U+FFFD on the wire, corrupting the character in the terminal.
//
// The child writes the three bytes of the euro sign (E2 82 AC) as two writes
// with a pause between them so the reader observes them as separate reads.
func TestSessionOutputDoesNotSplitMultiByteRunes(t *testing.T) {
	installFakeKubectl(t, `printf '\342\202'; sleep 0.3; printf '\254'; sleep 0.1`)
	out := newCollectedOutput()
	s, err := newSession("s1", SessionOptions{Namespace: "ns", Pod: "p"}, out.emit)
	if err != nil {
		t.Fatalf("newSession: %v", err)
	}
	defer s.Close()
	out.wait(t)

	chunks := out.snapshot()
	for i, c := range chunks {
		if !utf8.ValidString(c) {
			t.Fatalf("chunk %d (%q) is not valid UTF-8: a rune was split across events (chunks: %q)", i, c, chunks)
		}
	}
	if got := strings.Join(chunks, ""); got != "€" {
		t.Fatalf("reassembled output = %q, want the euro sign", got)
	}
}

// The same holds for 4-byte runes split at every possible
// boundary, which also covers an implementation that carries partial bytes
// over to the next read.
func TestSessionOutputDoesNotSplitFourByteRuneAtAnyBoundary(t *testing.T) {
	// U+1F600: F0 9F 98 80
	for split := 1; split <= 3; split++ {
		first := []string{`\360`, `\237`, `\230`}[:split]
		rest := []string{`\360`, `\237`, `\230`, `\200`}[split:]
		body := "printf '" + strings.Join(first, "") + "'; sleep 0.3; printf '" + strings.Join(rest, "") + "'; sleep 0.1"
		installFakeKubectl(t, body)
		out := newCollectedOutput()
		s, err := newSession("s1", SessionOptions{Namespace: "ns", Pod: "p"}, out.emit)
		if err != nil {
			t.Fatalf("newSession: %v", err)
		}
		out.wait(t)
		s.Close()
		for i, c := range out.snapshot() {
			if !utf8.ValidString(c) {
				t.Fatalf("split after byte %d: chunk %d (%q) is not valid UTF-8", split, i, c)
			}
		}
		if got := strings.Join(out.snapshot(), ""); got != "\U0001F600" {
			t.Fatalf("split after byte %d: output = %q", split, got)
		}
	}
}
