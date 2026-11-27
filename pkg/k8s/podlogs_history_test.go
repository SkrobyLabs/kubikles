package k8s

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// streamLogServer generates a large log on the fly and counts the bytes it
// writes, so tests can assert how much of the log a call actually pulled. Lines
// end "now" and are step apart. Per-container logs are selected by the
// "container" query parameter; every container shares the same generator but
// has its own text prefix, so merged views can be told apart.
type streamLogServer struct {
	n       int
	step    time.Duration
	end     time.Time
	served  atomic.Int64
	reqs    atomic.Int64
	dupEach int // when > 1, groups of dupEach consecutive lines share a timestamp
}

func newStreamLogServer(n int, step time.Duration) *streamLogServer {
	return &streamLogServer{n: n, step: step, end: time.Now().UTC().Truncate(time.Second)}
}

func (s *streamLogServer) ts(i int) time.Time { // i in [0,n)
	k := i
	if s.dupEach > 1 {
		k = i / s.dupEach
	}
	return s.end.Add(-time.Duration(s.n-k) * s.step)
}

func (s *streamLogServer) stamp(i int) string {
	return s.ts(i).Format("2006-01-02T15:04:05.000000000Z")
}

func (s *streamLogServer) text(container string, i int) string {
	return fmt.Sprintf("%sline-%07d payload-payload-payload-payload", container, i)
}

func (s *streamLogServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.reqs.Add(1)
	q := r.URL.Query()
	from := 0
	if since := q.Get("sinceTime"); since != "" {
		t, err := time.Parse(time.RFC3339, since)
		if err != nil {
			http.Error(w, "bad sinceTime", http.StatusBadRequest)
			return
		}
		from = sort.Search(s.n, func(i int) bool { return !s.ts(i).Before(t) })
	}
	if tail := q.Get("tailLines"); tail != "" {
		n, _ := strconv.Atoi(tail)
		if s.n-n > from {
			from = s.n - n
		}
	}
	limit := int64(-1)
	if lb := q.Get("limitBytes"); lb != "" {
		limit, _ = strconv.ParseInt(lb, 10, 64)
	}
	prefix := ""
	if c := q.Get("container"); c != "" {
		prefix = c + ":"
	}
	withTS := q.Get("timestamps") == "true"
	flusher, _ := w.(http.Flusher)
	var written int64
	var chunk []byte
	flush := func() bool {
		if len(chunk) == 0 {
			return true
		}
		if limit >= 0 && written+int64(len(chunk)) > limit {
			chunk = chunk[:limit-written]
		}
		m, err := w.Write(chunk)
		s.served.Add(int64(m))
		written += int64(m)
		chunk = chunk[:0]
		if flusher != nil {
			flusher.Flush()
		}
		return err == nil && (limit < 0 || written < limit) && r.Context().Err() == nil
	}
	for i := from; i < s.n; i++ {
		if withTS {
			chunk = append(chunk, s.stamp(i)...)
			chunk = append(chunk, ' ')
		}
		chunk = append(chunk, s.text(prefix, i)...)
		chunk = append(chunk, '\n')
		if len(chunk) > 4096 && !flush() {
			return
		}
	}
	flush()
}

func (s *streamLogServer) fullBytes() int64 {
	return int64(s.n) * int64(len(s.stamp(0))+1+len(s.text("", 0))+1)
}

func TestLogLinesBeforeTailIsBounded(t *testing.T) {
	srv := newStreamLogServer(200000, 100*time.Millisecond)
	c := newAPIServerClient(t, srv)

	got, more, err := c.GetPodLogsBefore("ns", "pod", "", false, false, "2099-01-01T00:00:00Z", 200)
	if err != nil {
		t.Fatal(err)
	}
	lines := logLines(got)
	if len(lines) != 200 || !more {
		t.Fatalf("lines=%d more=%v", len(lines), more)
	}
	if want := srv.text("", srv.n-1); lines[199] != want {
		t.Fatalf("last line %q want %q", lines[199], want)
	}
	if want := srv.text("", srv.n-200); lines[0] != want {
		t.Fatalf("first line %q want %q", lines[0], want)
	}
	if served := srv.served.Load(); served > srv.fullBytes()/20 {
		t.Fatalf("served %d bytes of a %d byte log", served, srv.fullBytes())
	}
}

func TestLogLinesBeforeDeepCutoff(t *testing.T) {
	srv := newStreamLogServer(200000, 100*time.Millisecond)
	c := newAPIServerClient(t, srv)
	idx := 50000

	got, more, err := c.GetPodLogsBefore("ns", "pod", "", true, false, srv.stamp(idx), 100)
	if err != nil {
		t.Fatal(err)
	}
	lines := logLines(got)
	if len(lines) != 100 || !more {
		t.Fatalf("lines=%d more=%v", len(lines), more)
	}
	if want := srv.stamp(idx-1) + " " + srv.text("", idx-1); lines[99] != want {
		t.Fatalf("last %q want %q", lines[99], want)
	}
	// Only the window ending at the cutoff may be fetched, never what follows it.
	if served := srv.served.Load(); served > srv.fullBytes()/20 {
		t.Fatalf("served %d bytes of a %d byte log", served, srv.fullBytes())
	}
}

func TestLogLinesBeforePagingCoversEverythingOnce(t *testing.T) {
	for _, dup := range []int{1, 3} {
		t.Run(fmt.Sprintf("dupEach=%d", dup), func(t *testing.T) {
			srv := newStreamLogServer(137, 2*time.Second)
			srv.dupEach = dup
			c := newAPIServerClient(t, srv)

			var all []string
			cutoff := "2099-01-01T00:00:00Z"
			for page := 0; page < 100; page++ {
				got, more, err := c.GetPodLogsBefore("ns", "pod", "", true, false, cutoff, 10)
				if err != nil {
					t.Fatal(err)
				}
				lines := logLines(got)
				if len(lines) == 0 {
					break
				}
				all = append(lines, all...)
				cutoff = lines[0][:30]
				if !more {
					break
				}
			}
			// With duplicate timestamps at a page edge the whole timestamp group is
			// excluded by the next page; documented edge. Without duplicates every
			// line must be visited exactly once and in order.
			if dup == 1 {
				if len(all) != srv.n {
					t.Fatalf("visited %d of %d lines", len(all), srv.n)
				}
				for i, l := range all {
					if want := srv.stamp(i) + " " + srv.text("", i); l != want {
						t.Fatalf("line %d = %q want %q", i, l, want)
					}
				}
				return
			}
			seen := map[string]bool{}
			for _, l := range all {
				if seen[l] {
					t.Fatalf("duplicate line %q", l)
				}
				seen[l] = true
			}
			if len(all) == 0 {
				t.Fatal("no lines returned")
			}
		})
	}
}

func TestLogLinesBeforeShortLogReachesStart(t *testing.T) {
	srv := newStreamLogServer(5, time.Second)
	c := newAPIServerClient(t, srv)
	got, more, err := c.GetPodLogsBefore("ns", "pod", "", false, false, "2099-01-01T00:00:00Z", 50)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(logLines(got)); n != 5 || more {
		t.Fatalf("lines=%d more=%v", n, more)
	}
}

func TestLogLinesBeforeDenseLogShrinksWindow(t *testing.T) {
	// A tiny scan budget makes the first one-minute window too expensive; the
	// reader must retry a smaller window rather than download it.
	old := maxHistoryScanBytes
	maxHistoryScanBytes = 64 * 1024
	t.Cleanup(func() { maxHistoryScanBytes = old })

	srv := newStreamLogServer(50000, 10*time.Millisecond) // 6000 lines per minute
	c := newAPIServerClient(t, srv)
	got, more, err := c.GetPodLogsBefore("ns", "pod", "", false, false, "2099-01-01T00:00:00Z", 20)
	if err != nil {
		t.Fatal(err)
	}
	lines := logLines(got)
	if len(lines) != 20 || !more || lines[19] != srv.text("", srv.n-1) {
		t.Fatalf("lines=%d more=%v last=%q", len(lines), more, lines[len(lines)-1])
	}
	if srv.reqs.Load() < 2 {
		t.Fatal("expected a retry with a smaller window")
	}
}

func TestLogLinesBeforeCutoffBeforeAll(t *testing.T) {
	srv := newStreamLogServer(30, time.Second)
	c := newAPIServerClient(t, srv)
	got, more, err := c.GetPodLogsBefore("ns", "pod", "", false, false, "2001-01-01T00:00:00Z", 10)
	if err != nil {
		t.Fatal(err)
	}
	if got != "" || more {
		t.Fatalf("got %q more=%v", got, more)
	}
}

func TestReadLogHeadStopsEarly(t *testing.T) {
	srv := newStreamLogServer(200000, 100*time.Millisecond)
	c := newAPIServerClient(t, srv)
	got, err := c.GetPodLogsFromStart("ns", "pod", "", false, false, 50)
	if err != nil {
		t.Fatal(err)
	}
	lines := logLines(got)
	if len(lines) != 50 || lines[0] != srv.text("", 0) || lines[49] != srv.text("", 49) {
		t.Fatalf("unexpected head: %d lines", len(lines))
	}
	if strings.HasSuffix(got, "\n") {
		t.Fatal("a full page must not end in a newline")
	}
	if served := srv.served.Load(); served > srv.fullBytes()/20 {
		t.Fatalf("served %d bytes of a %d byte log", served, srv.fullBytes())
	}
}

func TestGetPodLogsSinceTimeReadsOnlyFirstPage(t *testing.T) {
	srv := newStreamLogServer(200000, 100*time.Millisecond)
	c := newAPIServerClient(t, srv)
	got, err := c.GetPodLogs("ns", "pod", "", false, false, srv.ts(1000).Format(time.RFC3339))
	if err != nil {
		t.Fatal(err)
	}
	if n := len(logLines(got)); n != 200 {
		t.Fatalf("got %d lines", n)
	}
	if served := srv.served.Load(); served > srv.fullBytes()/20 {
		t.Fatalf("served %d bytes", served)
	}
}

func TestScanLinesBeforeByteCapKeepsNewest(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 10; i++ {
		fmt.Fprintf(&b, "2026-01-01T00:00:%02d.000000000Z %s\n", i, strings.Repeat("x", 100))
	}
	res, err := scanLinesBefore(strings.NewReader(b.String()), "2027", 20, 350, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !res.trimmed || len(res.lines) == 0 || len(res.lines) >= 10 || res.total != 10 {
		t.Fatalf("trimmed=%v kept=%d total=%d", res.trimmed, len(res.lines), res.total)
	}
	if !strings.HasPrefix(res.lines[len(res.lines)-1], "2026-01-01T00:00:09") {
		t.Fatalf("newest line lost: %q", res.lines[len(res.lines)-1])
	}
}

func TestScanLinesBeforeScanBudget(t *testing.T) {
	src := strings.NewReader(strings.Repeat("2026-01-01T00:00:00.000000000Z hello\n", 1000))
	if _, err := scanLinesBefore(src, "2027", 5, 1<<20, 1024); err != errScanBudget {
		t.Fatalf("err = %v", err)
	}
}

func TestMergedLogsFromStartAndBefore(t *testing.T) {
	srv := newStreamLogServer(2000, time.Second)
	c := newAPIServerClient(t, srv)

	head, err := c.GetAllContainersLogsFromStart("ns", "pod", []string{"a", "b"}, true, false, 6)
	if err != nil {
		t.Fatal(err)
	}
	lines := logLines(head)
	if len(lines) != 6 {
		t.Fatalf("head lines = %d: %q", len(lines), lines)
	}
	// Equal timestamps keep container order (stable merge): a, b, a, b, a, b.
	for i, l := range lines {
		want := []string{"[a]", "[b]"}[i%2]
		if !strings.Contains(l, want) {
			t.Fatalf("line %d %q lacks %s", i, l, want)
		}
	}

	tail, more, err := c.GetAllContainersLogsBefore("ns", "pod", []string{"a", "b"}, false, false, "2099-01-01T00:00:00Z", 10)
	if err != nil {
		t.Fatal(err)
	}
	tl := logLines(tail)
	if len(tl) != 10 || !more {
		t.Fatalf("tail lines=%d more=%v", len(tl), more)
	}
	if !strings.HasPrefix(tl[9], "[b] b:line-0001999") || !strings.HasPrefix(tl[8], "[a] a:line-0001999") {
		t.Fatalf("unexpected tail end: %q", tl[8:])
	}

	pods, _, err := c.GetAllPodsLogsBefore("ns", []PodContainerPair{{PodName: "p1", ContainerNames: []string{"c"}}, {PodName: "p2", ContainerNames: []string{"c", "d"}}}, true, false, false, "2099-01-01T00:00:00Z", 4)
	if err != nil {
		t.Fatal(err)
	}
	if got := logLines(pods); len(got) != 4 || !strings.Contains(got[3], "[p") {
		t.Fatalf("pods page: %q", got)
	}
}

func TestMergedLogsErrorsBecomeLines(t *testing.T) {
	c := newAPIServerClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	got, _, err := c.GetAllContainersLogsBefore("ns", "pod", []string{"a"}, true, false, "2099-01-01T00:00:00Z", 5)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "[a] Error fetching logs") {
		t.Fatalf("got %q", got)
	}
}

// BenchmarkPodLogsBeforeBytesFetched reports how many bytes the server had to
// send for one "load older lines" call on a 200k-line log, against the size of
// the whole log, which is what the previous implementation downloaded.
func BenchmarkPodLogsBeforeBytesFetched(b *testing.B) {
	srv := newStreamLogServer(200000, 100*time.Millisecond)
	c := newAPIServerClient(b, srv)
	b.Run("Before_tail", func(b *testing.B) {
		srv.served.Store(0)
		for i := 0; i < b.N; i++ {
			if _, _, err := c.GetPodLogsBefore("ns", "pod", "", false, false, "2099-01-01T00:00:00Z", 200); err != nil {
				b.Fatal(err)
			}
		}
		b.ReportMetric(float64(srv.served.Load())/float64(b.N), "bytes_fetched/op")
		b.ReportMetric(float64(srv.fullBytes()), "full_log_bytes")
	})
	b.Run("Before_mid", func(b *testing.B) {
		srv.served.Store(0)
		cut := srv.stamp(100000)
		for i := 0; i < b.N; i++ {
			if _, _, err := c.GetPodLogsBefore("ns", "pod", "", false, false, cut, 200); err != nil {
				b.Fatal(err)
			}
		}
		b.ReportMetric(float64(srv.served.Load())/float64(b.N), "bytes_fetched/op")
		b.ReportMetric(float64(srv.fullBytes()), "full_log_bytes")
	})
	b.Run("FromStart", func(b *testing.B) {
		srv.served.Store(0)
		for i := 0; i < b.N; i++ {
			if _, err := c.GetPodLogsFromStart("ns", "pod", "", false, false, 200); err != nil {
				b.Fatal(err)
			}
		}
		b.ReportMetric(float64(srv.served.Load())/float64(b.N), "bytes_fetched/op")
		b.ReportMetric(float64(srv.fullBytes()), "full_log_bytes")
	})
	b.Run("OldFullDownload", func(b *testing.B) {
		srv.served.Store(0)
		for i := 0; i < b.N; i++ {
			if _, err := c.GetAllPodLogs("ns", "pod", "", true, false); err != nil {
				b.Fatal(err)
			}
		}
		b.ReportMetric(float64(srv.served.Load())/float64(b.N), "bytes_fetched/op")
	})
}

// burstLogServer serves a fixed set of lines and honours sinceTime only.
type burstLogServer struct {
	stamps []time.Time
}

func (s *burstLogServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var since time.Time
	if v := r.URL.Query().Get("sinceTime"); v != "" {
		since, _ = time.Parse(time.RFC3339, v)
	}
	for i, ts := range s.stamps {
		if ts.Before(since) {
			continue
		}
		fmt.Fprintf(w, "%s line-%05d padding-padding-padding\n", ts.Format("2006-01-02T15:04:05.000000000Z"), i)
	}
}

func TestLogLinesBeforeBisectsAfterEmptyWindow(t *testing.T) {
	// Empty 1m window, then an 8m window whose burst exceeds the scan budget.
	// The reader must try the windows in between instead of giving up.
	old := maxHistoryScanBytes
	maxHistoryScanBytes = 64 * 1024
	t.Cleanup(func() { maxHistoryScanBytes = old })

	end := time.Now().UTC().Truncate(time.Second)
	srv := &burstLogServer{}
	for i := 0; i < 2000; i++ { // 8 lines/s from 8m ago to 4m ago
		srv.stamps = append(srv.stamps, end.Add(-8*time.Minute).Add(time.Duration(i)*120*time.Millisecond))
	}
	c := newAPIServerClient(t, srv)
	got, _, err := c.GetPodLogsBefore("ns", "pod", "", false, false, end.Format(time.RFC3339Nano), 20)
	if err != nil {
		t.Fatalf("expected a page, got %v", err)
	}
	lines := logLines(got)
	if len(lines) != 20 || !strings.HasPrefix(lines[19], "line-01999") {
		t.Fatalf("lines=%d last=%q", len(lines), lines[len(lines)-1])
	}
}
