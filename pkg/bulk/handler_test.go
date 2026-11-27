package bulk

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

const testToken = "tok-123"

type fakeCaller struct {
	calls  []string
	result interface{}
	err    error
	args   []json.RawMessage
}

func (f *fakeCaller) CallMethod(name string, args []json.RawMessage) (interface{}, error) {
	f.calls = append(f.calls, name)
	f.args = args
	return f.result, f.err
}

func newTestHandler(c Caller, mod func(*Deps)) http.Handler {
	d := Deps{Caller: c, Token: testToken, GOOS: "testos"}
	if mod != nil {
		mod(&d)
	}
	return NewHandler(d)
}

func authed(method, target string) *http.Request {
	r := httptest.NewRequest(method, target, nil)
	r.Header.Set(HeaderMarker, "1")
	r.Header.Set(HeaderToken, testToken)
	return r
}

func serve(h http.Handler, r *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

func TestGuards(t *testing.T) {
	h := newTestHandler(&fakeCaller{}, func(d *Deps) {
		d.AllowHost = func(host string) bool { return host == "example.com" }
	})
	ping := Base + "/probe/ping"

	cases := []struct {
		name   string
		mutate func(r *http.Request)
		want   int
	}{
		{"ok", func(r *http.Request) { r.Host = "example.com" }, 200},
		{"no marker", func(r *http.Request) { r.Host = "example.com"; r.Header.Del(HeaderMarker) }, 403},
		{"wrong marker", func(r *http.Request) { r.Host = "example.com"; r.Header.Set(HeaderMarker, "0") }, 403},
		{"no token", func(r *http.Request) { r.Host = "example.com"; r.Header.Del(HeaderToken) }, 403},
		{"wrong token", func(r *http.Request) { r.Host = "example.com"; r.Header.Set(HeaderToken, "nope") }, 403},
		{"bad host", func(r *http.Request) { r.Host = "evil.test" }, 403},
		{"post", func(r *http.Request) { r.Host = "example.com"; r.Method = http.MethodPost }, 405},
		{"options", func(r *http.Request) { r.Host = "example.com"; r.Method = http.MethodOptions }, 405},
		{"head", func(r *http.Request) { r.Host = "example.com"; r.Method = http.MethodHead }, 200},
	}
	for _, c := range cases {
		r := authed(http.MethodGet, ping)
		c.mutate(r)
		rec := serve(h, r)
		if rec.Code != c.want {
			t.Errorf("%s: code %d want %d", c.name, rec.Code, c.want)
		}
		if rec.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("%s: Cache-Control %q", c.name, rec.Header().Get("Cache-Control"))
		}
		if rec.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Errorf("%s: CORS header present", c.name)
		}
		if rec.Header().Get("Content-Type") == "" && rec.Body.Len() > 0 {
			t.Errorf("%s: missing Content-Type", c.name)
		}
	}
}

func TestNeverNotModified(t *testing.T) {
	h := newTestHandler(&fakeCaller{}, nil)
	r := authed(http.MethodGet, Base+"/probe/ping")
	r.Header.Set("If-None-Match", `"x"`)
	r.Header.Set("If-Modified-Since", time.Now().UTC().Format(http.TimeFormat))
	rec := serve(h, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("code %d", rec.Code)
	}
}

func TestPing(t *testing.T) {
	h := newTestHandler(&fakeCaller{}, func(d *Deps) { d.Headless = true })
	rec := serve(h, authed(http.MethodGet, Base+"/probe/ping"))
	var got map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["ok"] != true || got["goos"] != "testos" || got["headless"] != true || got["t"] == nil {
		t.Fatalf("ping = %v", got)
	}
}

func callURL(method string, args ...interface{}) string {
	b, _ := json.Marshal(args)
	return Base + "/call/" + method + "?args=" + url.QueryEscape(string(b))
}

func TestCallNDJSONFraming(t *testing.T) {
	type item struct {
		Name string `json:"name"`
		Note string `json:"note"`
	}
	items := []item{{"a", "line\nbreak"}, {"b", "unicode   é"}, {"c", ""}}
	fc := &fakeCaller{result: items}
	h := newTestHandler(fc, nil)
	rec := serve(h, authed(http.MethodGet, callURL("ListPods", "req-1", "default")))
	if rec.Code != 200 || !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/x-ndjson") {
		t.Fatalf("code %d ct %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	lines := strings.Split(strings.TrimSuffix(rec.Body.String(), "\n"), "\n")
	if len(lines) != len(items) {
		t.Fatalf("%d lines, want %d: %q", len(lines), len(items), rec.Body.String())
	}
	for i, l := range lines {
		var got item
		if err := json.Unmarshal([]byte(l), &got); err != nil || got != items[i] {
			t.Fatalf("line %d = %q (%v)", i, l, err)
		}
	}
	if len(fc.args) != 2 || string(fc.args[0]) != `"req-1"` {
		t.Fatalf("args = %s", fc.args)
	}
}

func TestCallNonSliceAndNilSlice(t *testing.T) {
	fc := &fakeCaller{result: map[string]int{"n": 3}}
	h := newTestHandler(fc, nil)
	rec := serve(h, authed(http.MethodGet, callURL("GetPodYaml", "ns", "p")))
	if rec.Body.String() != `{"data":{"n":3}}` {
		t.Fatalf("body %q", rec.Body.String())
	}
	var nilSlice []string
	fc.result = nilSlice
	rec = serve(h, authed(http.MethodGet, callURL("ListPods", "r", "")))
	if strings.TrimSpace(rec.Body.String()) != `{"data":null}` {
		t.Fatalf("nil slice body %q", rec.Body.String())
	}
	fc.result = []string{}
	rec = serve(h, authed(http.MethodGet, callURL("ListPods", "r", "")))
	if rec.Body.Len() != 0 || !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/x-ndjson") {
		t.Fatalf("empty slice body %q ct %q", rec.Body.String(), rec.Header().Get("Content-Type"))
	}
	fc.result = "plain string"
	rec = serve(h, authed(http.MethodGet, callURL("GetPodYaml", "ns", "p")))
	if strings.TrimSpace(rec.Body.String()) != `{"data":"plain string"}` {
		t.Fatalf("string body %q", rec.Body.String())
	}
}

func TestCallErrorShapeAndGuards(t *testing.T) {
	fc := &fakeCaller{err: errors.New("boom")}
	h := newTestHandler(fc, nil)
	rec := serve(h, authed(http.MethodGet, callURL("ListPods", "r", "")))
	if rec.Code != 500 || strings.TrimSpace(rec.Body.String()) != `{"error":"boom"}` {
		t.Fatalf("code %d body %q", rec.Code, rec.Body.String())
	}
	fc.calls = nil
	rec = serve(h, authed(http.MethodGet, callURL("DeletePod", "ns", "p")))
	if rec.Code != 403 || len(fc.calls) != 0 {
		t.Fatalf("mutating method reached caller: code %d calls %v", rec.Code, fc.calls)
	}
	rec = serve(h, authed(http.MethodGet, Base+"/call/ListPods?args=%7Bnot"))
	if rec.Code != 400 {
		t.Fatalf("bad args code %d", rec.Code)
	}
	long := Base + "/call/ListPods?args=" + url.QueryEscape(`["`+strings.Repeat("a", MaxArgsBytes)+`"]`)
	rec = serve(h, authed(http.MethodGet, long))
	if rec.Code != http.StatusRequestURITooLong {
		t.Fatalf("long args code %d", rec.Code)
	}
}

// failAfterWriter fails every write once limit bytes have been accepted,
// mimicking the Wails darwin writer after the task was stopped.
type failAfterWriter struct {
	hdr     http.Header
	limit   int
	written int
	writes  int
}

func (f *failAfterWriter) Header() http.Header { return f.hdr }
func (f *failAfterWriter) WriteHeader(int)     {}
func (f *failAfterWriter) Write(p []byte) (int, error) {
	f.writes++
	if f.written >= f.limit {
		return 0, errors.New("task stopped")
	}
	f.written += len(p)
	return len(p), nil
}

func TestCancellationStopsProducer(t *testing.T) {
	h := newTestHandler(&fakeCaller{}, nil)
	fw := &failAfterWriter{hdr: http.Header{}, limit: 200 << 10}
	start := time.Now()
	h.ServeHTTP(fw, authed(http.MethodGet, Base+"/probe/bench?objects=100000&mode=ndjson"))
	if time.Since(start) > 2*time.Second {
		t.Fatalf("producer kept running for %v", time.Since(start))
	}
	// 100000 objects are well over 100MB; stopping after the error means only
	// a couple of chunks were attempted.
	if fw.written > 1<<20 {
		t.Fatalf("wrote %d bytes after failure", fw.written)
	}
	if fw.writes > 40 {
		t.Fatalf("%d writes attempted", fw.writes)
	}
}

func TestCallSliceStopsOnWriteError(t *testing.T) {
	big := make([]string, 50000)
	for i := range big {
		big[i] = strings.Repeat("y", 200)
	}
	h := newTestHandler(&fakeCaller{result: big}, nil)
	fw := &failAfterWriter{hdr: http.Header{}, limit: 100 << 10}
	h.ServeHTTP(fw, authed(http.MethodGet, callURL("ListPods", "r", "")))
	if fw.writes > 10 {
		t.Fatalf("%d writes after cancellation", fw.writes)
	}
}

func TestChunksAreAtMost64KB(t *testing.T) {
	h := newTestHandler(&fakeCaller{}, nil)
	rec := &sizeRecorder{ResponseRecorder: httptest.NewRecorder()}
	h.ServeHTTP(rec, authed(http.MethodGet, Base+"/probe/bytes?mb=2"))
	if rec.max > chunkSize {
		t.Fatalf("largest write %d > %d", rec.max, chunkSize)
	}
	if rec.total != 2<<20 {
		t.Fatalf("total %d", rec.total)
	}
}

type sizeRecorder struct {
	*httptest.ResponseRecorder
	max, total int
}

func (s *sizeRecorder) Write(p []byte) (int, error) {
	if len(p) > s.max {
		s.max = len(p)
	}
	s.total += len(p)
	return s.ResponseRecorder.Write(p)
}

// plainWriter is a ResponseWriter that is neither a Flusher nor unwrappable,
// like the Wails writers.
type plainWriter struct {
	hdr  http.Header
	code int
	body strings.Builder
}

func (p *plainWriter) Header() http.Header         { return p.hdr }
func (p *plainWriter) WriteHeader(c int)           { p.code = c }
func (p *plainWriter) Write(b []byte) (int, error) { return p.body.Write(b) }

func TestNonFlusherWriter(t *testing.T) {
	h := newTestHandler(&fakeCaller{}, nil)
	pw := &plainWriter{hdr: http.Header{}}
	h.ServeHTTP(pw, authed(http.MethodGet, Base+"/probe/stream?chunks=4&delayMs=0&size=10"))
	if _, ok := interface{}(pw).(http.Flusher); ok {
		t.Fatal("test writer must not be a Flusher")
	}
	if n := strings.Count(pw.body.String(), "\n"); n != 4 {
		t.Fatalf("lines = %d body %q", n, pw.body.String())
	}
}

func TestGzipRoundTrip(t *testing.T) {
	h := newTestHandler(&fakeCaller{}, func(d *Deps) { d.Compress = true })
	srv := httptest.NewServer(h)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL+Base+"/probe/stream?chunks=5&delayMs=0&size=500", nil)
	req.Header.Set(HeaderMarker, "1")
	req.Header.Set(HeaderToken, testToken)
	req.Header.Set("Accept-Encoding", "gzip")
	tr := &http.Transport{DisableCompression: true}
	resp, err := (&http.Client{Transport: tr}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.Header.Get("Content-Encoding") != "gzip" {
		t.Fatalf("Content-Encoding = %q", resp.Header.Get("Content-Encoding"))
	}
	zr, err := gzip.NewReader(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(data), "\n"); n != 5 {
		t.Fatalf("lines %d", n)
	}

	// Without Accept-Encoding the body is plain.
	req2, _ := http.NewRequest(http.MethodGet, srv.URL+Base+"/probe/ping", nil)
	req2.Header.Set(HeaderMarker, "1")
	req2.Header.Set(HeaderToken, testToken)
	resp2, err := (&http.Client{Transport: tr}).Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.Header.Get("Content-Encoding") != "" {
		t.Fatal("unexpected Content-Encoding")
	}
}

func TestNoCompressionWhenDisabled(t *testing.T) {
	h := newTestHandler(&fakeCaller{}, nil)
	r := authed(http.MethodGet, Base+"/probe/ping")
	r.Header.Set("Accept-Encoding", "gzip")
	rec := serve(h, r)
	if rec.Header().Get("Content-Encoding") != "" {
		t.Fatal("desktop mode must not set Content-Encoding")
	}
}

func TestStreamLongerThanWriteTimeout(t *testing.T) {
	h := newTestHandler(&fakeCaller{}, func(d *Deps) { d.ChunkTimeout = 400 * time.Millisecond })
	srv := httptest.NewUnstartedServer(h)
	srv.Config.WriteTimeout = 400 * time.Millisecond
	srv.Start()
	defer srv.Close()

	// 8 lines, 150ms apart: about 1.05s, well past the 400ms WriteTimeout.
	req, _ := http.NewRequest(http.MethodGet, srv.URL+Base+"/probe/stream?chunks=8&delayMs=150&size=10", nil)
	req.Header.Set(HeaderMarker, "1")
	req.Header.Set(HeaderToken, testToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	sc := bufio.NewScanner(resp.Body)
	var arrivals []time.Time
	start := time.Now()
	for sc.Scan() {
		arrivals = append(arrivals, time.Now())
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("stream died: %v after %d lines", err, len(arrivals))
	}
	if len(arrivals) != 8 {
		t.Fatalf("got %d lines", len(arrivals))
	}
	if time.Since(start) < 900*time.Millisecond {
		t.Fatalf("stream finished too early: %v", time.Since(start))
	}
	// Streaming: the first line must arrive long before the last.
	if arrivals[len(arrivals)-1].Sub(arrivals[0]) < 600*time.Millisecond {
		t.Fatalf("lines were buffered: first-to-last %v", arrivals[len(arrivals)-1].Sub(arrivals[0]))
	}
}

func TestBenchModes(t *testing.T) {
	h := newTestHandler(&fakeCaller{}, nil)
	rec := serve(h, authed(http.MethodGet, Base+"/probe/bench?objects=25&mode=json"))
	var doc struct {
		Data []map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil || len(doc.Data) != 25 {
		t.Fatalf("json mode: %v len %d", err, len(doc.Data))
	}
	rec = serve(h, authed(http.MethodGet, Base+"/probe/bench?objects=25&mode=ndjson"))
	if n := strings.Count(rec.Body.String(), "\n"); n != 25 {
		t.Fatalf("ndjson lines %d", n)
	}
	a, _ := json.Marshal(BenchObject(7))
	b, _ := json.Marshal(BenchObject(7))
	if string(a) != string(b) {
		t.Fatal("bench objects must be deterministic")
	}
}

func TestChainRoutesPrefix(t *testing.T) {
	bulk := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "bulk") })
	mw := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "assets") })
	}
	h := Chain(bulk, mw)(http.NotFoundHandler())
	for _, m := range []string{"GET", "POST", "OPTIONS", "PUT"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(m, "/_kubikles/v1/probe/ping", nil))
		if rec.Body.String() != "bulk" {
			t.Fatalf("%s /_kubikles/ not routed to bulk: %q", m, rec.Body.String())
		}
	}
	for _, p := range []string{"/", "/assets/app.js", "/_kubiklesx", "/kubikles/"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", p, nil))
		if rec.Body.String() != "assets" {
			t.Fatalf("%s routed to %q", p, rec.Body.String())
		}
	}
}

type testProvider struct{}

func (testProvider) Mount(mux *http.ServeMux) {
	mux.HandleFunc(Base+"/extra", func(w http.ResponseWriter, r *http.Request) { WriteJSON(w, 200, map[string]bool{"extra": true}) })
}

func TestRouteProviderInheritsGuards(t *testing.T) {
	h := newTestHandler(&fakeCaller{}, func(d *Deps) { d.Routes = []RouteProvider{testProvider{}} })
	if rec := serve(h, authed("GET", Base+"/extra")); rec.Code != 200 {
		t.Fatalf("code %d", rec.Code)
	}
	r := httptest.NewRequest("GET", Base+"/extra", nil)
	if rec := serve(h, r); rec.Code != 403 {
		t.Fatalf("unguarded provider route: %d", rec.Code)
	}
}

// ---- allowlist ----

func dispatchNames(t *testing.T) []string {
	t.Helper()
	b, err := os.ReadFile("../../dispatch_gen.go")
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, m := range regexp.MustCompile(`case "([A-Za-z0-9]+)":`).FindAllStringSubmatch(string(b), -1) {
		out = append(out, m[1])
	}
	if len(out) < 100 {
		t.Fatalf("parsed only %d dispatch names", len(out))
	}
	return out
}

func TestAllowlistNamesExistInDispatcher(t *testing.T) {
	have := map[string]bool{}
	for _, n := range dispatchNames(t) {
		have[n] = true
	}
	for _, n := range ExtraAllowed() {
		if !have[n] {
			t.Errorf("extra allowlist entry %q is not in dispatch_gen.go", n)
		}
	}
}

func TestAllowlistExcludesMutations(t *testing.T) {
	for _, n := range dispatchNames(t) {
		if !IsAllowed(n) {
			continue
		}
		for _, p := range forbiddenPrefixes {
			if strings.HasPrefix(n, p) {
				t.Errorf("%s is allowlisted but starts with %s", n, p)
			}
		}
	}
	for _, n := range []string{"DeletePod", "SetForceHTTP1", "CancelListRequest", "ApplyYAML", "ExecInPod", "", "GetPodLogs", "UpdatePodYaml", "StartLogStream"} {
		if IsAllowed(n) {
			t.Errorf("%q must not be allowed", n)
		}
	}
	// Update...Yaml ends in Yaml but must stay blocked.
	if IsAllowed("UpdatePodYaml") {
		t.Error("UpdatePodYaml allowed")
	}
}

func TestAllowlistCoversFrontendListFunctions(t *testing.T) {
	re := regexp.MustCompile(`\bList[A-Za-z]+\b`)
	for _, f := range []string{"../../frontend/src/hooks/resources/index.ts", "../../frontend/src/hooks/useCustomResources.tsx"} {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		found := re.FindAllString(string(b), -1)
		if len(found) == 0 {
			t.Fatalf("no List functions found in %s", f)
		}
		for _, n := range found {
			if !IsAllowed(n) {
				t.Errorf("%s uses %s which is not allowlisted", f, n)
			}
		}
	}
}

func TestChunkWriterErrorIsSticky(t *testing.T) {
	fw := &failAfterWriter{hdr: http.Header{}, limit: 0}
	fw.written = 0
	cw := NewChunkWriter(fw, time.Second)
	_, _ = cw.Write([]byte("hello"))
	if err := cw.Flush(); err == nil {
		// limit 0 and written 0: first write succeeds, second must fail.
		_, _ = cw.Write([]byte("again"))
		err = cw.Flush()
		if err == nil {
			t.Fatal("expected write error")
		}
	}
	if cw.Err() == nil {
		t.Fatal("error must be sticky")
	}
	if _, err := cw.Write([]byte("x")); err == nil {
		t.Fatal("writes after error must fail")
	}
}

func TestHostChecker(t *testing.T) {
	ok := NewHostChecker([]string{"kubikles.example.com", "*.corp.test"})
	for _, h := range []string{"localhost:8080", "127.0.0.1:3000", "[::1]:80", "10.1.2.3", "kubikles", "kubikles.example.com", "a.corp.test:443", "LOCALHOST"} {
		if !ok(h) {
			t.Errorf("%q should be allowed", h)
		}
	}
	for _, h := range []string{"", "evil.com", "evil.com:8080", "corp.test.evil.com", "other.example.com"} {
		if ok(h) {
			t.Errorf("%q should be rejected", h)
		}
	}
}
