package bulk

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"runtime/debug"

	"kubikles/pkg/crashlog"
)

const (
	// HeaderMarker must be "1" on every request; it keeps simple cross-site
	// requests (which cannot set custom headers without a preflight) out.
	HeaderMarker = "X-Kubikles"
	// HeaderToken carries the per-process token handed out by GetBulkInfo.
	HeaderToken = "X-Kubikles-Token"
)

// guard enforces the rules shared by every /_kubikles route: GET/HEAD only,
// marker header, per-process token (constant-time compare), optional Host
// check, no caching, no CORS, never a 304. Wails on Windows turns a 304 into
// an error and WebView2 can hang on it, so conditional headers are dropped
// before any handler can honour them.
func guard(d Deps, next http.Handler) http.Handler {
	token := []byte(d.Token)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Cache-Control", "no-store")
		h.Set("X-Content-Type-Options", "nosniff")
		// Deliberately no Access-Control-Allow-Origin, ever.

		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			h.Set("Allow", "GET, HEAD")
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if d.AllowHost != nil && !d.AllowHost(r.Host) {
			writeJSONError(w, http.StatusForbidden, "host not allowed")
			return
		}
		if r.Header.Get(HeaderMarker) != "1" {
			writeJSONError(w, http.StatusForbidden, "missing marker header")
			return
		}
		if len(token) == 0 || subtle.ConstantTimeCompare([]byte(r.Header.Get(HeaderToken)), token) != 1 {
			writeJSONError(w, http.StatusForbidden, "invalid token")
			return
		}
		r.Header.Del("If-None-Match")
		r.Header.Del("If-Modified-Since")
		r.Header.Del("If-Match")
		r.Header.Del("If-Unmodified-Since")
		r.Header.Del("Range")
		r.Header.Del("If-Range")
		sw := &statusWriter{ResponseWriter: w}
		defer func() {
			// The desktop asset server runs handlers on its own goroutines, so
			// an escaped panic would take the whole app down.
			if rec := recover(); rec != nil {
				crashlog.LogError("PANIC in bulk handler %s: %v\nStack: %s", r.URL.Path, rec, string(debug.Stack()))
				if !sw.wrote {
					writeJSONError(w, http.StatusInternalServerError, "internal error")
				}
			}
		}()
		next.ServeHTTP(sw, r)
	})
}

// statusWriter records whether the response has started so the panic
// recovery knows if it can still send a clean error.
type statusWriter struct {
	http.ResponseWriter
	wrote bool
}

func (s *statusWriter) WriteHeader(code int) {
	s.wrote = true
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusWriter) Write(b []byte) (int, error) {
	s.wrote = true
	return s.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (s *statusWriter) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// writeJSONError writes a small buffered JSON error. Callers must not have
// written a body yet.
func writeJSONError(w http.ResponseWriter, status int, msg string) {
	body, _ := json.Marshal(map[string]string{"error": msg})
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// WriteError is exported for route providers.
func WriteError(w http.ResponseWriter, status int, msg string) { writeJSONError(w, status, msg) }

// WriteJSON writes v as a small buffered JSON body with an explicit length.
func WriteJSON(w http.ResponseWriter, status int, v interface{}) {
	body, err := json.Marshal(v)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "encode response: "+err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}
