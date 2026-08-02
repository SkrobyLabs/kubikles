package server

import (
	"bytes"
	"compress/gzip"
	"embed"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strconv"
	"strings"
)

const (
	browserArtifactRoot = "frontend/dist/accelerator-browser"
	browserMarkerPath   = ".kubikles-browser-v1.json"
	browserEntryPath    = "assets/browser.js"
	browserMarkerMax    = 512
)

// BrowserEntryAvailability deliberately exposes no artifact details.
type BrowserEntryAvailability interface{ BrowserEntryEnabled() bool }

// BrowserEntryGate is the immutable, exact-version admission point for the
// dedicated Browser artifact. Invalid artifacts are an expected disabled state.
type BrowserEntryGate struct{ assets fs.FS }

func (g *BrowserEntryGate) BrowserEntryEnabled() bool { return g != nil && g.assets != nil }

func NewBrowserEntryGate(assets embed.FS, buildVersion string) *BrowserEntryGate {
	return newBrowserEntryGate(assets, buildVersion)
}

// newBrowserEntryGate is the filesystem seam used by the embedded production
// artifact and by exact-tree handler tests. It intentionally remains internal:
// callers can observe only BrowserEntryAvailability.
func newBrowserEntryGate(assets fs.FS, buildVersion string) *BrowserEntryGate {
	root, err := fs.Sub(assets, browserArtifactRoot)
	if err != nil || buildVersion == "" || buildVersion != strings.TrimSpace(buildVersion) || strings.IndexFunc(buildVersion, func(r rune) bool { return r <= 0x1f || r == 0x7f }) >= 0 {
		return &BrowserEntryGate{}
	}
	marker, err := fs.ReadFile(root, browserMarkerPath)
	markerInfo, statErr := fs.Stat(root, browserMarkerPath)
	if err != nil || statErr != nil || !markerInfo.Mode().IsRegular() || len(marker) == 0 || len(marker) > browserMarkerMax {
		return &BrowserEntryGate{}
	}
	var value struct {
		SchemaVersion int    `json:"schemaVersion"`
		Kind          string `json:"kind"`
		BuildVersion  string `json:"buildVersion"`
	}
	decoder := json.NewDecoder(bytes.NewReader(marker))
	decoder.DisallowUnknownFields()
	expected, marshalErr := json.Marshal(struct {
		SchemaVersion int    `json:"schemaVersion"`
		Kind          string `json:"kind"`
		BuildVersion  string `json:"buildVersion"`
	}{1, "kubikles-accelerator-browser", buildVersion})
	if decoder.Decode(&value) != nil || decoder.Decode(&struct{}{}) != io.EOF || marshalErr != nil || !bytes.Equal(marker, append(expected, '\n')) || value.SchemaVersion != 1 || value.Kind != "kubikles-accelerator-browser" || value.BuildVersion != buildVersion {
		return &BrowserEntryGate{}
	}
	rootEntries, err := fs.ReadDir(root, ".")
	if err != nil || len(rootEntries) != 2 || rootEntries[0].Name() != browserMarkerPath || rootEntries[1].Name() != "assets" || !rootEntries[0].Type().IsRegular() || !rootEntries[1].IsDir() {
		return &BrowserEntryGate{}
	}
	assetEntries, err := fs.ReadDir(root, "assets")
	if err != nil || len(assetEntries) != 2 || assetEntries[0].Name() != "browser.css" || assetEntries[1].Name() != "browser.js" {
		return &BrowserEntryGate{}
	}
	for _, artifact := range []string{browserEntryPath, "assets/browser.css"} {
		info, statErr := fs.Stat(root, artifact)
		if statErr != nil || !info.Mode().IsRegular() {
			return &BrowserEntryGate{}
		}
	}
	return &BrowserEntryGate{assets: root}
}

// browserEntryHeaders apply to every Browser namespace outcome, including 404s.
func browserEntryHeaders(w http.ResponseWriter) {
	h := w.Header()
	h.Set("Cache-Control", "no-store, no-cache")
	h.Set("Pragma", "no-cache")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Cross-Origin-Opener-Policy", "same-origin")
	h.Set("Cross-Origin-Resource-Policy", "same-origin")
	h.Set("Permissions-Policy", "accelerometer=(), camera=(), geolocation=(), microphone=(), payment=(), usb=()")
	h.Set("Content-Security-Policy", "default-src 'none'; base-uri 'none'; object-src 'none'; frame-ancestors 'none'; form-action 'none'; script-src 'self'; style-src 'self'; img-src 'self' data:; font-src 'self'; connect-src 'self'; manifest-src 'none'; worker-src 'none'")
}

func browserNotFound(w http.ResponseWriter) { browserEntryHeaders(w); http.NotFound(w, nil) }

func (g *BrowserEntryGate) serve(w http.ResponseWriter, r *http.Request) {
	browserEntryHeaders(w)
	if r.URL.RawQuery != "" || r.URL.ForceQuery || !exactRequestPath(r, r.URL.Path) {
		http.NotFound(w, r)
		return
	}
	if r.URL.Path == "/accelerator/browser/" || r.URL.Path == "/accelerator/browser/bootstrap.js" {
		if !g.BrowserEntryEnabled() {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		content := browserBootstrapHTML
		mime := "text/html; charset=utf-8"
		if r.URL.Path == "/accelerator/browser/bootstrap.js" {
			content, mime = browserBootstrapJS, "application/javascript; charset=utf-8"
		}
		w.Header().Set("Content-Type", mime)
		if r.Method == http.MethodGet {
			_, _ = w.Write(content)
		}
		return
	}
	if !strings.HasPrefix(r.URL.Path, "/accelerator/browser/assets/") {
		http.NotFound(w, r)
		return
	}
	if !g.BrowserEntryEnabled() {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	rel := strings.TrimPrefix(r.URL.Path, "/accelerator/browser/assets/")
	if rel == "" || strings.Contains(rel, "//") || strings.ContainsAny(rel, "\\\x00") || strings.Contains(rel, "%") || strings.Contains(rel, "/../") || strings.HasPrefix(rel, "../") || !fs.ValidPath("assets/"+rel) {
		http.NotFound(w, r)
		return
	}
	mime, ok := browserMIME(path.Ext(rel))
	if !ok {
		http.NotFound(w, r)
		return
	}
	data, err := fs.ReadFile(g.assets, "assets/"+rel)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	info, err := fs.Stat(g.assets, "assets/"+rel)
	if err != nil || !info.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", mime)
	if acceptsGzip(r.Header.Get("Accept-Encoding")) {
		var out bytes.Buffer
		zw := gzip.NewWriter(&out)
		_, _ = zw.Write(data)
		_ = zw.Close()
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Vary", "Accept-Encoding")
		data = out.Bytes()
	}
	if r.Method == http.MethodGet {
		_, _ = w.Write(data)
	}
}

func browserMIME(ext string) (string, bool) {
	switch ext {
	case ".js", ".mjs":
		return "application/javascript; charset=utf-8", true
	case ".css":
		return "text/css; charset=utf-8", true
	case ".json":
		return "application/json", true
	case ".svg":
		return "image/svg+xml", true
	case ".png":
		return "image/png", true
	case ".webp":
		return "image/webp", true
	case ".woff":
		return "font/woff", true
	case ".woff2":
		return "font/woff2", true
	case ".ttf":
		return "font/ttf", true
	}
	return "", false
}
func acceptsGzip(value string) bool {
	for _, part := range strings.Split(value, ",") {
		fields := strings.Split(part, ";")
		if !strings.EqualFold(strings.TrimSpace(fields[0]), "gzip") {
			continue
		}
		accepted := true
		for _, field := range fields[1:] {
			field = strings.TrimSpace(field)
			if strings.HasPrefix(strings.ToLower(field), "q=") {
				q, err := strconv.ParseFloat(strings.TrimSpace(field[2:]), 64)
				if err != nil || q < 0 || q > 1 || q == 0 {
					accepted = false
				}
			}
		}
		if accepted {
			return true
		}
	}
	return false
}

//go:embed browserbootstrap/index.html browserbootstrap/bootstrap.js
var browserBootstrapFiles embed.FS
var browserBootstrapHTML, _ = browserBootstrapFiles.ReadFile("browserbootstrap/index.html")
var browserBootstrapJS, _ = browserBootstrapFiles.ReadFile("browserbootstrap/bootstrap.js")
