package compressedassets

import (
	"io/fs"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"testing/fstest"
)

// countingFS counts Open calls per path so tests can see whether the handler
// re-reads embedded files on every request.
type countingFS struct {
	fs.FS
	mu    sync.Mutex
	opens map[string]int
}

func (c *countingFS) Open(name string) (fs.File, error) {
	c.mu.Lock()
	if c.opens == nil {
		c.opens = map[string]int{}
	}
	c.opens[name]++
	c.mu.Unlock()
	return c.FS.Open(name)
}

func (c *countingFS) count(name string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.opens[name]
}

func prefixedAssets(t *testing.T) fstest.MapFS {
	t.Helper()
	assets, _ := testAssets(t)
	prefixed := fstest.MapFS{}
	for name, f := range assets {
		prefixed["frontend/dist/"+name] = f
	}
	return prefixed
}

func TestGzipServerCachesAndSetsLengthAndCachePolicy(t *testing.T) {
	assets, plain := testAssets(t)
	cfs := &countingFS{FS: assets}
	h := GzipAwareFileServer(cfs)

	var firstBody string
	for i := 0; i < 5; i++ {
		rec := do(h, http.MethodGet, "/assets/app.js", nil)
		if rec.Code != http.StatusOK || rec.Body.String() != plain["assets/app.js"] {
			t.Fatalf("request %d: code %d", i, rec.Code)
		}
		if got := rec.Header().Get("Content-Length"); got != strconv.Itoa(len(plain["assets/app.js"])) {
			t.Fatalf("Content-Length = %q", got)
		}
		if got := rec.Header().Get("Cache-Control"); got != immutableCacheControl {
			t.Fatalf("hashed asset Cache-Control = %q", got)
		}
		if i == 0 {
			firstBody = rec.Body.String()
		} else if rec.Body.String() != firstBody {
			t.Fatal("cached body differs")
		}
	}
	if n := cfs.count("assets/app.js.gz"); n != 1 {
		t.Fatalf("embedded file opened %d times, want 1", n)
	}

	gzRec := do(h, http.MethodGet, "/assets/app.js", map[string]string{"Accept-Encoding": "gzip"})
	if got := gzRec.Header().Get("Content-Length"); got != strconv.Itoa(gzRec.Body.Len()) {
		t.Fatalf("gzip Content-Length %q for %d bytes", got, gzRec.Body.Len())
	}
	if gzRec.Header().Get("Content-Encoding") != "gzip" {
		t.Fatal("expected gzip passthrough")
	}
}

func TestIndexHTMLIsNeverImmutable(t *testing.T) {
	assets, _ := testAssets(t)
	for name, h := range map[string]http.Handler{
		"gzip":  GzipAwareFileServer(assets),
		"wails": WailsMiddleware(prefixedAssets(t), "frontend/dist")(http.NotFoundHandler()),
	} {
		for _, path := range []string{"/", "/index.html", "/some/spa/route"} {
			if name == "wails" && path == "/some/spa/route" {
				continue // the Wails middleware does not do SPA routing
			}
			rec := do(h, http.MethodGet, path, nil)
			cc := rec.Header().Get("Cache-Control")
			if cc == "" || cc == immutableCacheControl {
				t.Fatalf("%s %s: Cache-Control = %q", name, path, cc)
			}
		}
	}
}

func TestConditionalRequestsReturn304(t *testing.T) {
	assets, _ := testAssets(t)
	h := GzipAwareFileServer(assets)

	for _, enc := range []string{"gzip", ""} {
		hdr := map[string]string{"Accept-Encoding": enc}
		first := do(h, http.MethodGet, "/index.html", hdr)
		etag := first.Header().Get("ETag")
		if etag == "" {
			t.Fatalf("enc=%q: no ETag", enc)
		}
		hdr["If-None-Match"] = etag
		second := do(h, http.MethodGet, "/index.html", hdr)
		if second.Code != http.StatusNotModified || second.Body.Len() != 0 {
			t.Fatalf("enc=%q: code %d body %d", enc, second.Code, second.Body.Len())
		}
		hdr["If-None-Match"] = `"other"`
		if third := do(h, http.MethodGet, "/index.html", hdr); third.Code != http.StatusOK {
			t.Fatalf("enc=%q: stale validator got %d", enc, third.Code)
		}
	}
	gzTag := do(h, http.MethodGet, "/index.html", map[string]string{"Accept-Encoding": "gzip"}).Header().Get("ETag")
	idTag := do(h, http.MethodGet, "/index.html", nil).Header().Get("ETag")
	if gzTag == idTag {
		t.Fatal("gzip and identity representations must have distinct ETags")
	}
}

func TestWailsMiddlewareCachesDecompressedBytes(t *testing.T) {
	_, plain := testAssets(t)
	cfs := &countingFS{FS: prefixedAssets(t)}
	h := WailsMiddleware(cfs, "frontend/dist")(http.NotFoundHandler())
	for i := 0; i < 4; i++ {
		rec := do(h, http.MethodGet, "/assets/app.css", nil)
		if rec.Body.String() != plain["assets/app.css"] {
			t.Fatalf("body %q", rec.Body.String())
		}
		if rec.Header().Get("Content-Length") != strconv.Itoa(len(plain["assets/app.css"])) {
			t.Fatal("missing Content-Length")
		}
	}
	if n := cfs.count("frontend/dist/assets/app.css.gz"); n != 1 {
		t.Fatalf("opened %d times", n)
	}
}

func TestAssetCacheConcurrentRequests(t *testing.T) {
	assets, plain := testAssets(t)
	h := GzipAwareFileServer(assets)
	var bad atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			hdr := map[string]string{}
			if i%2 == 0 {
				hdr["Accept-Encoding"] = "gzip"
			}
			rec := do(h, http.MethodGet, "/assets/app.js", hdr)
			if rec.Code != 200 || (i%2 == 1 && rec.Body.String() != plain["assets/app.js"]) {
				bad.Add(1)
			}
		}(i)
	}
	wg.Wait()
	if bad.Load() != 0 {
		t.Fatalf("%d bad responses", bad.Load())
	}
}
