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
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
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

// WailsMiddleware returns a Wails assetserver.Middleware that transparently
// decompresses pre-compressed .gz assets. When a request for e.g. "/index.js"
// arrives, the middleware looks for "index.js.gz" in the embedded FS, reads
// and decompresses it, then serves the original content with the correct
// Content-Type.
//
// If no .gz variant exists the request falls through to the default handler.
func WailsMiddleware(assets fs.FS, prefix string) func(next http.Handler) http.Handler {
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

			fsPath := prefix + "/" + reqPath
			gzPath := fsPath + ".gz"

			// Try the .gz variant first.
			gzFile, err := assets.Open(gzPath)
			if err != nil {
				// No .gz variant — maybe the file exists uncompressed (fonts, images).
				next.ServeHTTP(w, r)
				return
			}
			defer gzFile.Close()

			compressed, err := io.ReadAll(gzFile)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}

			gr, err := gzip.NewReader(bytes.NewReader(compressed))
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}
			defer gr.Close()

			decompressed, err := io.ReadAll(gr)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}

			w.Header().Set("Content-Type", contentTypeByExt(reqPath))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(decompressed)
		})
	}
}

// GzipAwareFileServer returns an http.Handler that serves pre-compressed
// assets from an fs.FS. For clients that accept gzip encoding, .gz files
// are served directly with Content-Encoding: gzip. Otherwise the handler
// decompresses on the fly.
//
// The subFS should already be rooted at the dist directory (e.g. via fs.Sub).
func GzipAwareFileServer(subFS fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqPath := strings.TrimPrefix(r.URL.Path, "/")
		if reqPath == "" {
			reqPath = "index.html"
		}

		// For SPA routing: if the path has no extension, serve index.html.
		if !strings.Contains(reqPath, ".") {
			reqPath = "index.html"
		}

		gzPath := reqPath + ".gz"

		gzFile, err := subFS.Open(gzPath)
		if err != nil {
			// No .gz variant — serve the original file (fonts, images, etc.).
			http.FileServer(http.FS(subFS)).ServeHTTP(w, r)
			return
		}
		defer gzFile.Close()

		compressed, err := io.ReadAll(gzFile)
		if err != nil {
			http.Error(w, "read error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", contentTypeByExt(reqPath))

		// If the client accepts gzip, serve the compressed bytes directly.
		if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			w.Header().Set("Content-Encoding", "gzip")
			w.Header().Set("Vary", "Accept-Encoding")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(compressed)
			return
		}

		// Client does not accept gzip — decompress and serve.
		gr, err := gzip.NewReader(bytes.NewReader(compressed))
		if err != nil {
			http.Error(w, "decompression error", http.StatusInternalServerError)
			return
		}
		defer gr.Close()

		decompressed, err := io.ReadAll(gr)
		if err != nil {
			http.Error(w, "decompression error", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(decompressed)
	})
}
