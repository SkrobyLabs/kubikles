// Package compressedassets provides middleware and file-serving helpers for
// pre-compressed (gzip) embedded frontend assets.
//
// At build time, scripts/compress-dist.sh replaces text assets in frontend/dist
// with their .gz equivalents. The utilities here transparently decompress or
// pass-through those files depending on the serving context:
//
//   - Desktop (Wails): WebView does not negotiate gzip, so the middleware
//     decompresses .gz files before serving them. The binary is smaller but
//     runtime content is uncompressed.
//
//   - Headless (HTTP server): Browsers send Accept-Encoding: gzip, so
//     .gz files are served directly with Content-Encoding: gzip. Clients
//     that do not accept gzip get a decompressed response.
package compressedassets

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// contentTypeByExt returns the MIME type for the original (pre-gzip) extension.
func contentTypeByExt(name string) string {
	ext := filepath.Ext(name)
	if ct := mime.TypeByExtension(ext); ct != "" {
		return ct
	}
	// Fallback for common types that may not be in the OS mime database.
	switch ext {
	case ".js":
		return "application/javascript"
	case ".css":
		return "text/css"
	case ".html":
		return "text/html; charset=utf-8"
	case ".svg":
		return "image/svg+xml"
	case ".json":
		return "application/json"
	}
	return "application/octet-stream"
}

// maxCachedDecompressedBytes bounds the memory spent on decompressed copies.
// The embedded dist is a few MB; anything beyond the cap is served without
// being retained rather than evicting.
const maxCachedDecompressedBytes int64 = 64 << 20

// immutableCacheControl is for Vite's content-hashed files under assets/: the
// file name changes whenever the content does, so clients may keep them forever.
const immutableCacheControl = "public, max-age=31536000, immutable"

// revalidateCacheControl is for everything else (index.html above all): the
// name is stable, so clients must revalidate (cheaply, through the ETag).
const revalidateCacheControl = "no-cache"

// assetEntry is one embedded .gz file, read once. decompressed is filled lazily
// because headless gzip clients never need it.
type assetEntry struct {
	compressed []byte
	etag       string

	once         sync.Once
	decompressed []byte
	decompErr    error
}

func (e *assetEntry) plain() ([]byte, error) {
	e.once.Do(func() {
		gr, err := gzip.NewReader(bytes.NewReader(e.compressed))
		if err != nil {
			e.decompErr = err
			return
		}
		defer gr.Close()
		e.decompressed, e.decompErr = io.ReadAll(gr)
	})
	return e.decompressed, e.decompErr
}

// assetCache holds read-only embedded assets by path, so each request does not
// re-read and re-inflate the same bytes. Embedded content never changes during
// a process lifetime, which is what makes this safe.
type assetCache struct {
	mu      sync.Mutex
	entries map[string]*assetEntry
	bytes   int64
}

func newAssetCache() *assetCache { return &assetCache{entries: map[string]*assetEntry{}} }

// get returns the entry for gzPath, or an error if it cannot be opened or read.
// Read failures are not cached.
func (c *assetCache) get(fsys fs.FS, gzPath string) (*assetEntry, error) {
	c.mu.Lock()
	if e, ok := c.entries[gzPath]; ok {
		c.mu.Unlock()
		return e, nil
	}
	c.mu.Unlock()

	f, err := fsys.Open(gzPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	compressed, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(compressed)
	e := &assetEntry{compressed: compressed, etag: `"` + hex.EncodeToString(sum[:8]) + `"`}

	c.mu.Lock()
	defer c.mu.Unlock()
	if existing, ok := c.entries[gzPath]; ok {
		return existing, nil
	}
	// Budget by compressed size times a typical text ratio; the exact inflated
	// size is unknown until first use, and the cap is a safety net, not a quota.
	cost := int64(len(compressed)) * 4
	if c.bytes+cost <= maxCachedDecompressedBytes {
		c.entries[gzPath] = e
		c.bytes += cost
	}
	return e, nil
}

// setStaticHeaders sets validators and the cache policy for reqPath. identity
// distinguishes the decompressed representation from the gzip one.
func setStaticHeaders(w http.ResponseWriter, reqPath string, e *assetEntry, identity bool) {
	etag := e.etag
	if identity {
		etag = etag[:len(etag)-1] + `-id"`
	}
	w.Header().Set("ETag", etag)
	if strings.HasPrefix(reqPath, "assets/") {
		w.Header().Set("Cache-Control", immutableCacheControl)
	} else {
		w.Header().Set("Cache-Control", revalidateCacheControl)
	}
}

// notModified answers a conditional request and reports whether it did.
func notModified(w http.ResponseWriter, r *http.Request) bool {
	inm := r.Header.Get("If-None-Match")
	if inm == "" {
		return false
	}
	etag := w.Header().Get("ETag")
	for _, candidate := range strings.Split(inm, ",") {
		candidate = strings.TrimPrefix(strings.TrimSpace(candidate), "W/")
		if candidate == etag || candidate == "*" {
			w.WriteHeader(http.StatusNotModified)
			return true
		}
	}
	return false
}

func writeBody(w http.ResponseWriter, body []byte) {
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

// WailsMiddleware returns a Wails assetserver.Middleware that transparently
// decompresses pre-compressed .gz assets. When a request for e.g. "/index.js"
// arrives, the middleware looks for "index.js.gz" in the embedded FS, reads
// and decompresses it, then serves the original content with the correct
// Content-Type.
//
// Decompressed bytes are cached after the first request.
//
// If no .gz variant exists the request falls through to the default handler.
func WailsMiddleware(assets fs.FS, prefix string) func(next http.Handler) http.Handler {
	cache := newAssetCache()
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Only intercept GET requests for static files.
			if r.Method != http.MethodGet {
				next.ServeHTTP(w, r)
				return
			}

			reqPath := strings.TrimPrefix(r.URL.Path, "/")
			if reqPath == "" {
				reqPath = "index.html"
			}

			entry, err := cache.get(assets, prefix+"/"+reqPath+".gz")
			if err != nil {
				// No .gz variant (fonts, images) or unreadable: default handler.
				next.ServeHTTP(w, r)
				return
			}
			decompressed, err := entry.plain()
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}

			w.Header().Set("Content-Type", contentTypeByExt(reqPath))
			setStaticHeaders(w, reqPath, entry, true)
			if notModified(w, r) {
				return
			}
			writeBody(w, decompressed)
		})
	}
}

// GzipAwareFileServer returns an http.Handler that serves pre-compressed
// assets from an fs.FS. For clients that accept gzip encoding, .gz files
// are served directly with Content-Encoding: gzip. Otherwise the handler
// serves the (cached) decompressed bytes.
//
// The subFS should already be rooted at the dist directory (e.g. via fs.Sub).
func GzipAwareFileServer(subFS fs.FS) http.Handler {
	cache := newAssetCache()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqPath := strings.TrimPrefix(r.URL.Path, "/")
		if reqPath == "" {
			reqPath = "index.html"
		}

		// For SPA routing: if the path has no extension, serve index.html.
		if !strings.Contains(reqPath, ".") {
			reqPath = "index.html"
		}

		entry, err := cache.get(subFS, reqPath+".gz")
		if err != nil {
			// No .gz variant — serve the original file (fonts, images, etc.).
			http.FileServer(http.FS(subFS)).ServeHTTP(w, r)
			return
		}

		w.Header().Set("Content-Type", contentTypeByExt(reqPath))
		w.Header().Set("Vary", "Accept-Encoding")

		// If the client accepts gzip, serve the compressed bytes directly.
		if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			w.Header().Set("Content-Encoding", "gzip")
			setStaticHeaders(w, reqPath, entry, false)
			if notModified(w, r) {
				return
			}
			writeBody(w, entry.compressed)
			return
		}

		// Client does not accept gzip — serve the decompressed bytes.
		decompressed, err := entry.plain()
		if err != nil {
			http.Error(w, "decompression error", http.StatusInternalServerError)
			return
		}
		setStaticHeaders(w, reqPath, entry, true)
		if notModified(w, r) {
			return
		}
		writeBody(w, decompressed)
	})
}
