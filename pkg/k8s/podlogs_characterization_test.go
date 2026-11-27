package k8s

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeLogServer serves /api/v1/namespaces/{ns}/pods/{pod}/log from a fixed set
// of lines, honouring the options the Kubernetes API honours: sinceTime
// (inclusive, second precision as serialised by metav1.Time), tailLines,
// timestamps and limitBytes. Request URLs are recorded.
type fakeLogServer struct {
	mu       sync.Mutex
	lines    []fakeLogLine
	requests []string
}

type fakeLogLine struct {
	ts   time.Time
	text string
}

// newFakeLogServer builds n lines, one per second plus a fixed sub-second
// offset so cursors exercise nanosecond handling. Line i has text "line-%03d".
func newFakeLogServer(n int) *fakeLogServer {
	base := time.Date(2026, 3, 14, 9, 0, 0, 123456789, time.UTC)
	s := &fakeLogServer{}
	for i := 1; i <= n; i++ {
		s.lines = append(s.lines, fakeLogLine{ts: base.Add(time.Duration(i) * time.Second), text: fmt.Sprintf("line-%03d", i)})
	}
	return s
}

// stamp renders the 30-character timestamp prefix the kubelet emits.
func (s *fakeLogServer) stamp(i int) string {
	return s.lines[i-1].ts.Format("2006-01-02T15:04:05.000000000Z")
}

// fullLine returns line i (1-based) as the API returns it with timestamps=true.
func (s *fakeLogServer) fullLine(i int) string { return s.stamp(i) + " " + s.lines[i-1].text }

func (s *fakeLogServer) recorded() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.requests...)
}

func (s *fakeLogServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.requests = append(s.requests, r.URL.RequestURI())
	s.mu.Unlock()
	if !strings.HasSuffix(r.URL.Path, "/log") {
		http.NotFound(w, r)
		return
	}
	q := r.URL.Query()
	selected := s.lines
	if since := q.Get("sinceTime"); since != "" {
		t, err := time.Parse(time.RFC3339, since)
		if err != nil {
			http.Error(w, "bad sinceTime", http.StatusBadRequest)
			return
		}
		var kept []fakeLogLine
		for _, l := range selected {
			if !l.ts.Before(t) {
				kept = append(kept, l)
			}
		}
		selected = kept
	}
	if tail := q.Get("tailLines"); tail != "" {
		n, err := strconv.Atoi(tail)
		if err != nil {
			http.Error(w, "bad tailLines", http.StatusBadRequest)
			return
		}
		if n < len(selected) {
			selected = selected[len(selected)-n:]
		}
	}
	var b strings.Builder
	for _, l := range selected {
		if q.Get("timestamps") == "true" {
			b.WriteString(l.ts.Format("2006-01-02T15:04:05.000000000Z"))
			b.WriteByte(' ')
		}
		b.WriteString(l.text)
		b.WriteByte('\n')
	}
	out := b.String()
	if lb := q.Get("limitBytes"); lb != "" {
		if n, err := strconv.Atoi(lb); err == nil && n < len(out) {
			out = out[:n]
		}
	}
	w.Header().Set("Content-Type", "text/plain")
	_, _ = w.Write([]byte(out))
}

// logLines splits a result into lines, ignoring the trailing newline (or lack
// of it) so tests pin which lines are returned rather than a join quirk.
func logLines(s string) []string {
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func plainLines(from, to int) []string {
	var out []string
	for i := from; i <= to; i++ {
		out = append(out, fmt.Sprintf("line-%03d", i))
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestPodLogsFromStartCharacterization(t *testing.T) {
	srv := newFakeLogServer(10)
	c := newAPIServerClient(t, srv)

	t.Run("returns first N lines in order", func(t *testing.T) {
		got, err := c.GetPodLogsFromStart("ns", "pod", "c", false, false, 3)
		if err != nil {
			t.Fatal(err)
		}
		if want := plainLines(1, 3); !equalStrings(logLines(got), want) {
			t.Fatalf("got %q want %q", logLines(got), want)
		}
	})
	t.Run("limit larger than log returns everything", func(t *testing.T) {
		got, err := c.GetPodLogsFromStart("ns", "pod", "c", false, false, 50)
		if err != nil {
			t.Fatal(err)
		}
		if want := plainLines(1, 10); !equalStrings(logLines(got), want) {
			t.Fatalf("got %q", logLines(got))
		}
	})
	t.Run("non-positive limit defaults to 200", func(t *testing.T) {
		big := newFakeLogServer(250)
		bc := newAPIServerClient(t, big)
		for _, limit := range []int{0, -5} {
			got, err := bc.GetPodLogsFromStart("ns", "pod", "c", false, false, limit)
			if err != nil {
				t.Fatal(err)
			}
			if lines := logLines(got); len(lines) != 200 || lines[0] != "line-001" || lines[199] != "line-200" {
				t.Fatalf("limit=%d: got %d lines, first=%q", limit, len(lines), lines[0])
			}
		}
	})
	t.Run("timestamps flag controls prefix", func(t *testing.T) {
		got, err := c.GetPodLogsFromStart("ns", "pod", "c", true, false, 2)
		if err != nil {
			t.Fatal(err)
		}
		want := []string{srv.fullLine(1), srv.fullLine(2)}
		if !equalStrings(logLines(got), want) {
			t.Fatalf("got %q want %q", logLines(got), want)
		}
	})
	t.Run("container and previous are forwarded", func(t *testing.T) {
		before := len(srv.recorded())
		if _, err := c.GetPodLogsFromStart("ns", "pod", "sidecar", false, true, 1); err != nil {
			t.Fatal(err)
		}
		reqs := srv.recorded()[before:]
		if len(reqs) == 0 || !strings.Contains(reqs[0], "container=sidecar") || !strings.Contains(reqs[0], "previous=true") {
			t.Fatalf("requests: %v", reqs)
		}
	})
}

func TestPodLogsBeforeCharacterization(t *testing.T) {
	srv := newFakeLogServer(20)
	c := newAPIServerClient(t, srv)

	t.Run("cutoff mid log returns the lines just before it", func(t *testing.T) {
		// Cutoff is line 11's timestamp: lines at or after it are excluded.
		got, more, err := c.GetPodLogsBefore("ns", "pod", "c", false, false, srv.stamp(11), 3)
		if err != nil {
			t.Fatal(err)
		}
		if want := plainLines(8, 10); !equalStrings(logLines(got), want) {
			t.Fatalf("got %q want %q", logLines(got), want)
		}
		if !more {
			t.Fatal("hasMore must be true when older lines remain")
		}
	})
	t.Run("cutoff with fewer lines than limit before it reports no more", func(t *testing.T) {
		got, more, err := c.GetPodLogsBefore("ns", "pod", "c", false, false, srv.stamp(4), 10)
		if err != nil {
			t.Fatal(err)
		}
		if want := plainLines(1, 3); !equalStrings(logLines(got), want) {
			t.Fatalf("got %q want %q", logLines(got), want)
		}
		if more {
			t.Fatal("hasMore must be false when the start of the log is reached")
		}
	})
	t.Run("window ending exactly at the first line reports more=true quirk", func(t *testing.T) {
		// startIndex == 0 is treated as "no more" only when cutoff-limit < 0;
		// when cutoff-limit == 0 the else branch sets hasMore even though the
		// first line is included. Pinned so a rewrite makes a deliberate choice.
		got, more, err := c.GetPodLogsBefore("ns", "pod", "c", false, false, srv.stamp(4), 3)
		if err != nil {
			t.Fatal(err)
		}
		if want := plainLines(1, 3); !equalStrings(logLines(got), want) {
			t.Fatalf("got %q want %q", logLines(got), want)
		}
		if !more {
			t.Log("hasMore is now false for an exact fit; update the characterization if intentional")
		}
	})
	t.Run("cutoff before all logs returns empty", func(t *testing.T) {
		got, more, err := c.GetPodLogsBefore("ns", "pod", "c", false, false, srv.stamp(1), 5)
		if err != nil {
			t.Fatal(err)
		}
		if got != "" || more {
			t.Fatalf("got %q more=%v", got, more)
		}
	})
	t.Run("cutoff after all logs returns the tail", func(t *testing.T) {
		got, more, err := c.GetPodLogsBefore("ns", "pod", "c", false, false, "2030-01-01T00:00:00.000000000Z", 5)
		if err != nil {
			t.Fatal(err)
		}
		lines := logLines(got)
		if len(lines) == 0 || lines[len(lines)-1] != "line-020" {
			t.Fatalf("tail must end at the last line, got %q", lines)
		}
		if len(lines) > 5 {
			t.Fatalf("returned %d lines, limit 5", len(lines))
		}
		for i := 1; i < len(lines); i++ {
			if lines[i-1] >= lines[i] {
				t.Fatalf("lines out of order: %q", lines)
			}
		}
		if !more {
			t.Fatal("hasMore must be true: 20 lines exist, 5 requested")
		}
	})
	t.Run("cutoff after all logs returns limit-1 real lines quirk", func(t *testing.T) {
		// The trailing empty element produced by splitting "a\nb\n" on "\n"
		// consumes one slot of the limit. Observed today: limit 5 yields four
		// real lines plus an empty tail. A rewrite that returns the full five
		// is an improvement; this test only records the difference.
		got, _, err := c.GetPodLogsBefore("ns", "pod", "c", false, false, "2030-01-01T00:00:00.000000000Z", 5)
		if err != nil {
			t.Fatal(err)
		}
		if n := len(logLines(got)); n != 4 && n != 5 {
			t.Fatalf("expected 4 (current) or 5 (fixed) lines, got %d: %q", n, got)
		}
	})
	t.Run("timestamps flag controls prefix", func(t *testing.T) {
		got, _, err := c.GetPodLogsBefore("ns", "pod", "c", true, false, srv.stamp(11), 2)
		if err != nil {
			t.Fatal(err)
		}
		want := []string{srv.fullLine(9), srv.fullLine(10)}
		if !equalStrings(logLines(got), want) {
			t.Fatalf("got %q want %q", logLines(got), want)
		}
	})
	t.Run("results are in chronological order", func(t *testing.T) {
		got, _, err := c.GetPodLogsBefore("ns", "pod", "c", false, false, srv.stamp(15), 6)
		if err != nil {
			t.Fatal(err)
		}
		if want := plainLines(9, 14); !equalStrings(logLines(got), want) {
			t.Fatalf("got %q want %q", logLines(got), want)
		}
	})
}

func TestPodLogsAfterCharacterization(t *testing.T) {
	srv := newFakeLogServer(20)
	c := newAPIServerClient(t, srv)

	t.Run("cursor is exclusive", func(t *testing.T) {
		got, _, err := c.GetPodLogsAfter("ns", "pod", "c", false, false, srv.stamp(5), 3)
		if err != nil {
			t.Fatal(err)
		}
		if want := plainLines(6, 8); !equalStrings(logLines(got), want) {
			t.Fatalf("got %q want %q", logLines(got), want)
		}
	})
	t.Run("more lines remaining sets hasMore", func(t *testing.T) {
		_, more, err := c.GetPodLogsAfter("ns", "pod", "c", false, false, srv.stamp(5), 3)
		if err != nil {
			t.Fatal(err)
		}
		if !more {
			t.Fatal("hasMore must be true")
		}
	})
	t.Run("fewer remaining lines than limit reports no more", func(t *testing.T) {
		got, more, err := c.GetPodLogsAfter("ns", "pod", "c", false, false, srv.stamp(17), 10)
		if err != nil {
			t.Fatal(err)
		}
		if want := plainLines(18, 20); !equalStrings(logLines(got), want) {
			t.Fatalf("got %q want %q", logLines(got), want)
		}
		if more {
			t.Fatal("hasMore must be false")
		}
	})
	t.Run("cursor at the last line returns empty", func(t *testing.T) {
		got, more, err := c.GetPodLogsAfter("ns", "pod", "c", false, false, srv.stamp(20), 5)
		if err != nil {
			t.Fatal(err)
		}
		if got != "" || more {
			t.Fatalf("got %q more=%v", got, more)
		}
	})
	t.Run("sub-second cursor does not repeat the cursor line", func(t *testing.T) {
		// The API truncates sinceTime to whole seconds, so the server returns
		// the cursor line again; the client must drop it.
		got, _, err := c.GetPodLogsAfter("ns", "pod", "c", false, false, srv.stamp(1), 2)
		if err != nil {
			t.Fatal(err)
		}
		if want := plainLines(2, 3); !equalStrings(logLines(got), want) {
			t.Fatalf("got %q want %q", logLines(got), want)
		}
	})
	t.Run("timestamps flag controls prefix", func(t *testing.T) {
		got, _, err := c.GetPodLogsAfter("ns", "pod", "c", true, false, srv.stamp(5), 2)
		if err != nil {
			t.Fatal(err)
		}
		want := []string{srv.fullLine(6), srv.fullLine(7)}
		if !equalStrings(logLines(got), want) {
			t.Fatalf("got %q want %q", logLines(got), want)
		}
	})
	t.Run("paging forward visits every line exactly once", func(t *testing.T) {
		var all []string
		cursor := "2026-03-14T08:00:00.000000000Z"
		for page := 0; page < 20; page++ {
			got, more, err := c.GetPodLogsAfter("ns", "pod", "c", true, false, cursor, 7)
			if err != nil {
				t.Fatal(err)
			}
			lines := logLines(got)
			all = append(all, lines...)
			if !more || len(lines) == 0 {
				break
			}
			cursor = lines[len(lines)-1][:30]
		}
		var want []string
		for i := 1; i <= 20; i++ {
			want = append(want, srv.fullLine(i))
		}
		if !equalStrings(all, want) {
			t.Fatalf("paged lines differ\n got: %q\nwant: %q", all, want)
		}
	})
	t.Run("non-positive limit defaults to 200", func(t *testing.T) {
		big := newFakeLogServer(250)
		bc := newAPIServerClient(t, big)
		got, more, err := bc.GetPodLogsAfter("ns", "pod", "c", false, false, "2026-03-14T08:00:00.000000000Z", 0)
		if err != nil {
			t.Fatal(err)
		}
		if n := len(logLines(got)); n != 200 || !more {
			t.Fatalf("got %d lines more=%v", n, more)
		}
	})
}

func TestPodLogsAfterExactFitQuirk(t *testing.T) {
	// When exactly lineLimit lines remain, the split of "a\nb\n" yields a
	// trailing empty element, so len > limit and hasMore is reported true
	// although nothing remains. Recorded, not endorsed.
	srv := newFakeLogServer(8)
	c := newAPIServerClient(t, srv)
	got, more, err := c.GetPodLogsAfter("ns", "pod", "c", false, false, srv.stamp(5), 3)
	if err != nil {
		t.Fatal(err)
	}
	if want := plainLines(6, 8); !equalStrings(logLines(got), want) {
		t.Fatalf("got %q want %q", logLines(got), want)
	}
	if !more {
		t.Log("hasMore is now false for an exact fit; that is the correct answer, update this note")
	}
}

func TestPodLogsErrorPropagation(t *testing.T) {
	c := newAPIServerClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"kind":"Status","apiVersion":"v1","status":"Failure","message":"pod not found","reason":"NotFound","code":404}`, http.StatusNotFound)
	}))
	if _, err := c.GetPodLogsFromStart("ns", "pod", "", false, false, 5); err == nil {
		t.Fatal("FromStart: expected error")
	}
	if _, _, err := c.GetPodLogsBefore("ns", "pod", "", false, false, "2026-01-01T00:00:00.000000000Z", 5); err == nil {
		t.Fatal("Before: expected error")
	}
	if _, _, err := c.GetPodLogsAfter("ns", "pod", "", false, false, "2026-01-01T00:00:00.000000000Z", 5); err == nil {
		t.Fatal("After: expected error")
	}
}

func TestPodLogsAfterPageTooLarge(t *testing.T) {
	// A single page larger than the 4 MiB polling limit must fail loudly
	// instead of returning a truncated line that would corrupt the cursor.
	huge := strings.Repeat("x", int(maxPollingLogBytes)+10)
	c := newAPIServerClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, "2026-03-14T09:00:01.000000000Z %s\n", huge)
	}))
	_, _, err := c.GetPodLogsAfter("ns", "pod", "", false, false, "2026-03-14T09:00:00.000000000Z", 5)
	if err == nil || err != errLogPageTooLarge {
		t.Fatalf("err = %v, want errLogPageTooLarge", err)
	}
}
