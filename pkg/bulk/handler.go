// Package bulk serves same-origin GET routes for bulk reads, under
// /_kubikles/v1/. It is used by desktop mode (as asset server middleware, in
// front of the static asset handler) and by server mode (mounted on the HTTP
// mux). It has no Wails imports and builds under the headless tag.
//
// Wails notes that shape this package: the asset server builds requests with
// http.NewRequest (r.Context() is never cancelled, a write error is the only
// cancel signal); on Windows the response is buffered until the handler
// returns and a 304 is turned into an error; the desktop writers are not
// http.Flusher and write straight through.
package bulk

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"time"
)

const (
	// Base is the route prefix (without trailing slash).
	Base = "/_kubikles/v1"
	// Prefix is what the Chain helper matches.
	Prefix = "/_kubikles/"
	// MaxArgsBytes is the largest accepted args query value; larger calls must
	// use the binding.
	MaxArgsBytes = 64 << 10
)

// Caller dispatches a method by name, exactly as server mode's /api/call does.
type Caller interface {
	CallMethod(name string, args []json.RawMessage) (interface{}, error)
}

// RouteProvider lets later packages add routes under /_kubikles/v1/ without
// importing package main. Mounted routes inherit the guards.
type RouteProvider interface {
	Mount(mux *http.ServeMux)
}

// Deps configures NewHandler.
type Deps struct {
	Caller Caller
	// Token is the per-process token every request must present.
	Token string
	// AllowHost, when non-nil, vets r.Host (server mode DNS rebinding guard).
	AllowHost func(host string) bool
	// Compress enables gzip for clients that accept it. Desktop passes false:
	// the Wails scheme handlers must not see Content-Encoding.
	Compress bool
	// Routes are extra route providers.
	Routes []RouteProvider
	// Headless is reported by probe/ping.
	Headless bool
	// GOOS is reported by probe/ping.
	GOOS string
	// ChunkTimeout overrides the per-chunk write deadline extension.
	ChunkTimeout time.Duration
}

// NewHandler returns the handler for everything under /_kubikles/.
func NewHandler(d Deps) http.Handler {
	h := &handler{d: d}
	mux := http.NewServeMux()
	mux.HandleFunc(Base+"/probe/ping", h.ping)
	mux.HandleFunc(Base+"/probe/stream", h.probeStream)
	mux.HandleFunc(Base+"/probe/bench", h.probeBench)
	mux.HandleFunc(Base+"/probe/bytes", h.probeBytes)
	mux.HandleFunc(Base+"/call/", h.call)
	for _, rp := range d.Routes {
		rp.Mount(mux)
	}
	var out http.Handler = mux
	if d.Compress {
		out = withGzip(out)
	}
	return guard(d, out)
}

// Chain returns an asset server middleware that sends everything under
// /_kubikles/ (any method) to bulk and everything else to the wrapped
// middleware.
func Chain(bulk http.Handler, next func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	return func(inner http.Handler) http.Handler {
		rest := next(inner)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, Prefix) {
				bulk.ServeHTTP(w, r)
				return
			}
			rest.ServeHTTP(w, r)
		})
	}
}

type handler struct{ d Deps }

func (h *handler) ping(w http.ResponseWriter, r *http.Request) {
	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"ok":       true,
		"t":        time.Now().UnixMilli(),
		"goos":     h.d.GOOS,
		"headless": h.d.Headless,
	})
}

// call dispatches an allowlisted read method. Slice results are written as
// NDJSON (one element per line), everything else as {"data": ...}. A nil slice
// is written as {"data": null} so the client sees exactly what the binding
// returns.
func (h *handler) call(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, Base+"/call/")
	if !IsAllowed(name) {
		writeJSONError(w, http.StatusForbidden, "method not allowed: "+name)
		return
	}
	var args []json.RawMessage
	if raw := r.URL.Query().Get("args"); raw != "" {
		if len(raw) > MaxArgsBytes {
			writeJSONError(w, http.StatusRequestURITooLong, "args too large")
			return
		}
		if err := json.Unmarshal([]byte(raw), &args); err != nil {
			writeJSONError(w, http.StatusBadRequest, "args must be a JSON array")
			return
		}
	}
	if h.d.Caller == nil {
		writeJSONError(w, http.StatusInternalServerError, "no caller")
		return
	}
	result, err := h.d.Caller.CallMethod(name, args)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	rv := reflect.ValueOf(result)
	if rv.IsValid() && rv.Kind() == reflect.Slice && !rv.IsNil() && rv.Type().Elem().Kind() != reflect.Uint8 {
		h.writeNDJSONSlice(w, r, rv)
		return
	}
	body, err := json.Marshal(map[string]interface{}{"data": result})
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "encode result: "+err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	cw := NewChunkWriter(w, h.d.ChunkTimeout)
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	_, _ = cw.Write(body)
	_ = cw.Flush()
}

func (h *handler) writeNDJSONSlice(w http.ResponseWriter, r *http.Request, rv reflect.Value) {
	w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	cw := NewChunkWriter(w, h.d.ChunkTimeout)
	for i := 0; i < rv.Len(); i++ {
		line, err := json.Marshal(rv.Index(i).Interface())
		if err != nil {
			// Headers are gone; an error line keeps the framing valid and lets
			// the client fail the call.
			line, _ = json.Marshal(map[string]string{"$error": err.Error()})
			_ = cw.WriteLine(line)
			break
		}
		if cw.WriteLine(line) != nil {
			return // client gone
		}
	}
	_ = cw.Flush()
}
