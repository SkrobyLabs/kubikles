package compressedassets

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func gz(t *testing.T, content string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func testAssets(t *testing.T) (fstest.MapFS, map[string]string) {
	t.Helper()
	plain := map[string]string{
		"index.html":       "<!doctype html><title>app</title>",
		"assets/app.js":    "console.log('hello');" + strings.Repeat("//pad\n", 200),
		"assets/app.css":   "body{margin:0}",
		"assets/data.json": `{"a":1}`,
		"assets/logo.svg":  "<svg xmlns='http://www.w3.org/2000/svg'/>",
	}
	fs := fstest.MapFS{
		"font.woff2": {Data: []byte("not compressed font bytes")},
	}
	for name, body := range plain {
		fs[name+".gz"] = &fstest.MapFile{Data: gz(t, body)}
	}
	return fs, plain
}

func do(h http.Handler, method, path string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestGzipAwareFileServerServesPrecompressedBytesToGzipClients(t *testing.T) {
	assets, plain := testAssets(t)
	h := GzipAwareFileServer(assets)

	for path, wantType := range map[string]string{
		"/assets/app.js":    "javascript",
		"/assets/app.css":   "css",
		"/assets/data.json": "json",
		"/assets/logo.svg":  "svg",
	} {
		rec := do(h, http.MethodGet, path, map[string]string{"Accept-Encoding": "gzip, deflate"})
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status %d", path, rec.Code)
		}
		if got := rec.Header().Get("Content-Encoding"); got != "gzip" {
			t.Fatalf("%s: Content-Encoding = %q", path, got)
		}
		if got := rec.Header().Get("Vary"); got != "Accept-Encoding" {
			t.Fatalf("%s: Vary = %q", path, got)
		}
		if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, wantType) {
			t.Fatalf("%s: Content-Type = %q, want it to contain %q", path, ct, wantType)
		}
		// The body is the stored gzip stream, byte for byte.
		if !bytes.Equal(rec.Body.Bytes(), assets[strings.TrimPrefix(path, "/")+".gz"].Data) {
			t.Fatalf("%s: body is not the stored gzip bytes", path)
		}
		zr, err := gzip.NewReader(rec.Body)
		if err != nil {
			t.Fatal(err)
		}
		inflated, _ := io.ReadAll(zr)
		if string(inflated) != plain[strings.TrimPrefix(path, "/")] {
			t.Fatalf("%s: inflated body differs", path)
		}
	}
}

func TestGzipAwareFileServerDecompressesForClientsWithoutGzip(t *testing.T) {
	assets, plain := testAssets(t)
	h := GzipAwareFileServer(assets)

	rec := do(h, http.MethodGet, "/assets/app.js", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("Content-Encoding = %q, want none", got)
	}
	if body := rec.Body.String(); body != plain["assets/app.js"] {
		t.Fatalf("decompressed body differs (%d bytes)", len(body))
	}
	// Content-Type is set before the branch, so it is correct here too.
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "javascript") {
		t.Fatalf("Content-Type = %q", ct)
	}
}

func TestGzipAwareFileServerRootAndSPARoutesServeIndex(t *testing.T) {
	assets, plain := testAssets(t)
	h := GzipAwareFileServer(assets)
	for _, path := range []string{"/", "/pods", "/clusters/prod/nodes"} {
		rec := do(h, http.MethodGet, path, nil)
		if rec.Code != http.StatusOK || rec.Body.String() != plain["index.html"] {
			t.Fatalf("%s: status %d body %q", path, rec.Code, rec.Body.String())
		}
		if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
			t.Fatalf("%s: Content-Type = %q", path, ct)
		}
	}
}

func TestGzipAwareFileServerServesUncompressedFilesAsIs(t *testing.T) {
	assets, _ := testAssets(t)
	h := GzipAwareFileServer(assets)
	rec := do(h, http.MethodGet, "/font.woff2", map[string]string{"Accept-Encoding": "gzip"})
	if rec.Code != http.StatusOK || rec.Body.String() != "not compressed font bytes" {
		t.Fatalf("status %d body %q", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("uncompressed file got Content-Encoding %q", got)
	}
}

func TestGzipAwareFileServerMissingAssetIs404(t *testing.T) {
	assets, _ := testAssets(t)
	h := GzipAwareFileServer(assets)
	for _, path := range []string{"/assets/missing.js", "/nope.png"} {
		rec := do(h, http.MethodGet, path, map[string]string{"Accept-Encoding": "gzip"})
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s: status %d, want 404", path, rec.Code)
		}
		if rec.Header().Get("Content-Encoding") == "gzip" {
			t.Fatalf("%s: 404 must not claim gzip encoding", path)
		}
	}
}

func TestGzipAwareFileServerCorruptGzipReports500ForPlainClients(t *testing.T) {
	assets := fstest.MapFS{"bad.js.gz": {Data: []byte("this is not gzip")}}
	h := GzipAwareFileServer(assets)
	rec := do(h, http.MethodGet, "/bad.js", nil)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status %d, want 500", rec.Code)
	}
}

func TestWailsMiddlewareDecompressesAndSetsContentType(t *testing.T) {
	assets, plain := testAssets(t)
	// Assets live under a prefix inside the embedded FS.
	prefixed := fstest.MapFS{}
	for name, f := range assets {
		prefixed["frontend/dist/"+name] = f
	}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	h := WailsMiddleware(prefixed, "frontend/dist")(next)

	rec := do(h, http.MethodGet, "/assets/app.js", map[string]string{"Accept-Encoding": "gzip"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if rec.Header().Get("Content-Encoding") != "" {
		t.Fatal("WebView path must serve decompressed content without Content-Encoding")
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "javascript") {
		t.Fatalf("Content-Type = %q", rec.Header().Get("Content-Type"))
	}
	if rec.Body.String() != plain["assets/app.js"] {
		t.Fatal("decompressed body differs")
	}

	// The empty path maps to index.html.
	rec = do(h, http.MethodGet, "/", nil)
	if rec.Code != http.StatusOK || rec.Body.String() != plain["index.html"] {
		t.Fatalf("root: status %d body %q", rec.Code, rec.Body.String())
	}
	if !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("root Content-Type = %q", rec.Header().Get("Content-Type"))
	}
}

func TestWailsMiddlewareFallsThroughWhenNoGzipVariantOrNotGET(t *testing.T) {
	assets, _ := testAssets(t)
	prefixed := fstest.MapFS{}
	for name, f := range assets {
		prefixed["frontend/dist/"+name] = f
	}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	h := WailsMiddleware(prefixed, "frontend/dist")(next)

	if rec := do(h, http.MethodGet, "/font.woff2", nil); rec.Code != http.StatusTeapot {
		t.Fatalf("missing .gz variant must fall through, got %d", rec.Code)
	}
	if rec := do(h, http.MethodGet, "/missing.js", nil); rec.Code != http.StatusTeapot {
		t.Fatalf("missing asset must fall through, got %d", rec.Code)
	}
	if rec := do(h, http.MethodPost, "/assets/app.js", nil); rec.Code != http.StatusTeapot {
		t.Fatalf("non-GET must fall through, got %d", rec.Code)
	}
}

func TestWailsMiddlewareCorruptGzipFallsThrough(t *testing.T) {
	prefixed := fstest.MapFS{"frontend/dist/bad.js.gz": {Data: []byte("not gzip")}}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusTeapot) })
	h := WailsMiddleware(prefixed, "frontend/dist")(next)
	if rec := do(h, http.MethodGet, "/bad.js", nil); rec.Code != http.StatusTeapot {
		t.Fatalf("status %d, want fallthrough", rec.Code)
	}
}

func TestContentTypeByExtFallbacks(t *testing.T) {
	for name, want := range map[string]string{
		"a.js": "javascript", "a.css": "css", "a.html": "text/html", "a.svg": "svg", "a.json": "json",
		"a.unknownext": "application/octet-stream",
	} {
		if got := contentTypeByExt(name); !strings.Contains(got, want) {
			t.Fatalf("%s: %q does not contain %q", name, got, want)
		}
	}
}

// The bulk transport lives under /_kubikles/ and is chained in front of this
// middleware. The middleware must never serve anything for that prefix itself:
// it falls through to the next handler like any unknown path.
func TestWailsMiddlewareNeverInterceptsKubiklesPrefix(t *testing.T) {
	var reached bool
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		w.WriteHeader(http.StatusTeapot)
	})
	h := WailsMiddleware(prefixedAssets(t), "frontend/dist")(next)
	for _, p := range []string{"/_kubikles/v1/probe/ping", "/_kubikles/", "/_kubikles/v1/call/ListPods"} {
		reached = false
		rec := do(h, http.MethodGet, p, nil)
		if !reached || rec.Code != http.StatusTeapot {
			t.Fatalf("%s intercepted: reached=%v code=%d", p, reached, rec.Code)
		}
	}
}
