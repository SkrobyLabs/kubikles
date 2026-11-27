package rows

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"sigs.k8s.io/yaml"

	"kubikles/pkg/bulk"
)

// ErrObjectNotFound is returned by an ObjectSource for a missing object (HTTP 404).
var ErrObjectNotFound = errors.New("object not found")

// ObjectSource fetches one full object as JSON (managed fields stripped).
type ObjectSource interface {
	GetObject(ctx context.Context, ref ObjectRef) (json.RawMessage, error)
}

// Routes serves the data side of the view protocol under /_kubikles/v1/. It is GET only: window
// fetches, NDJSON streams of refs and rows, and single objects. Control (open, update, close)
// stays on the bindings. Mounted through bulk.Deps.Routes, it inherits the guards (marker header,
// token, Host check, no-store, never 304, no CORS).
type Routes struct {
	// Views resolves the manager lazily: the bulk handler is built before the application
	// starts the store, so the manager does not exist yet when the routes are mounted.
	Views   func() *ViewManager
	Objects ObjectSource
}

func (rt *Routes) manager(w http.ResponseWriter) *ViewManager {
	if rt.Views != nil {
		if m := rt.Views(); m != nil {
			return m
		}
	}
	bulk.WriteError(w, http.StatusServiceUnavailable, "views are not ready")
	return nil
}

const (
	defaultWindowLimit = 100
	maxWindowLimit     = 1000
)

// Mount implements bulk.RouteProvider.
func (rt *Routes) Mount(mux *http.ServeMux) {
	mux.HandleFunc(bulk.Base+"/views/", rt.views)
	mux.HandleFunc(bulk.Base+"/objects/", rt.object)
}

func viewError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrViewNotFound):
		// 410: the client re-opens the view from its retained spec.
		bulk.WriteError(w, http.StatusGone, err.Error())
	case errors.Is(err, ErrViewChanged):
		bulk.WriteError(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrTooManyViews):
		bulk.WriteError(w, http.StatusTooManyRequests, err.Error())
	default:
		bulk.WriteError(w, http.StatusInternalServerError, err.Error())
	}
}

func intParam(r *http.Request, name string, def int) (int, bool) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return def, true
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

func (rt *Routes) views(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, bulk.Base+"/views/")
	parts := strings.Split(rest, "/")
	if len(parts) != 2 || parts[0] == "" {
		bulk.WriteError(w, http.StatusNotFound, "unknown view route")
		return
	}
	id, what := parts[0], parts[1]
	switch what {
	case "window":
		rt.window(w, r, id)
	case "refs":
		rt.refs(w, r, id)
	case "rows":
		rt.rows(w, r, id)
	default:
		bulk.WriteError(w, http.StatusNotFound, "unknown view route")
	}
}

func (rt *Routes) window(w http.ResponseWriter, r *http.Request, id string) {
	offset, ok1 := intParam(r, "offset", 0)
	limit, ok2 := intParam(r, "limit", defaultWindowLimit)
	haveRaw := r.URL.Query().Get("have")
	var have uint64
	if haveRaw != "" {
		n, err := strconv.ParseUint(haveRaw, 10, 64)
		if err != nil {
			bulk.WriteError(w, http.StatusBadRequest, "bad have")
			return
		}
		have = n
	}
	if !ok1 || !ok2 {
		bulk.WriteError(w, http.StatusBadRequest, "bad offset or limit")
		return
	}
	if limit > maxWindowLimit {
		limit = maxWindowLimit
	}
	mgr := rt.manager(w)
	if mgr == nil {
		return
	}
	win, err := mgr.Window(id, offset, limit, have)
	if err != nil {
		viewError(w, err)
		return
	}
	bulk.WriteJSON(w, http.StatusOK, win)
}

type streamHeader struct {
	Version  uint64   `json:"version"`
	Total    int      `json:"total"`
	Filtered int      `json:"filtered"`
	Columns  []string `json:"columns,omitempty"`
}

func startNDJSON(w http.ResponseWriter, r *http.Request) bool {
	w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	return r.Method != http.MethodHead
}

func (rt *Routes) refs(w http.ResponseWriter, r *http.Request, id string) {
	offset, ok1 := intParam(r, "offset", 0)
	limit, ok2 := intParam(r, "limit", 0)
	if !ok1 || !ok2 {
		bulk.WriteError(w, http.StatusBadRequest, "bad offset or limit")
		return
	}
	mgr := rt.manager(w)
	if mgr == nil {
		return
	}
	var expected uint64
	if raw := r.URL.Query().Get("version"); raw != "" {
		n, perr := strconv.ParseUint(raw, 10, 64)
		if perr != nil {
			bulk.WriteError(w, http.StatusBadRequest, "bad version")
			return
		}
		expected = n
	}
	refs, st, err := mgr.Refs(id, offset, limit, expected)
	if err != nil {
		viewError(w, err)
		return
	}
	if !startNDJSON(w, r) {
		return
	}
	cw := bulk.NewChunkWriter(w, 0)
	hdr, _ := json.Marshal(streamHeader{Version: st.Version, Total: st.Total, Filtered: st.Filtered})
	if cw.WriteLine(hdr) != nil {
		return
	}
	for _, ref := range refs {
		line, _ := json.Marshal([]string{ref.UID, ref.NS, ref.Name})
		if cw.WriteLine(line) != nil {
			return // client gone
		}
	}
	_ = cw.Flush()
}

type streamRow struct {
	UID  string        `json:"uid"`
	NS   string        `json:"ns"`
	Name string        `json:"name"`
	TS   string        `json:"ts"`
	Del  bool          `json:"del"`
	Seq  uint64        `json:"seq"`
	C    []interface{} `json:"c"`
}

func (rt *Routes) rows(w http.ResponseWriter, r *http.Request, id string) {
	mgr := rt.manager(w)
	if mgr == nil {
		return
	}
	all, cols, st, err := mgr.Rows(id)
	if err != nil {
		viewError(w, err)
		return
	}
	if !startNDJSON(w, r) {
		return
	}
	cw := bulk.NewChunkWriter(w, 0)
	hdr, _ := json.Marshal(streamHeader{Version: st.Version, Total: st.Total, Filtered: st.Filtered, Columns: cols})
	if cw.WriteLine(hdr) != nil {
		return
	}
	for _, row := range all {
		line, err := json.Marshal(streamRow{UID: row.UID, NS: row.NS, Name: row.Name, TS: row.TS, Del: row.Del, Seq: row.Seq, C: row.C})
		if err != nil {
			line, _ = json.Marshal(map[string]string{"$error": err.Error()})
			_ = cw.WriteLine(line)
			return
		}
		if cw.WriteLine(line) != nil {
			return
		}
	}
	_ = cw.Flush()
}

func (rt *Routes) object(w http.ResponseWriter, r *http.Request) {
	typ := strings.TrimPrefix(r.URL.Path, bulk.Base+"/objects/")
	if typ == "" || strings.Contains(typ, "/") && !strings.HasPrefix(typ, "crd:") {
		bulk.WriteError(w, http.StatusNotFound, "unknown object route")
		return
	}
	q := r.URL.Query()
	ref := ObjectRef{Type: typ, Namespace: q.Get("ns"), Name: q.Get("name"), UID: q.Get("uid")}
	if ref.Name == "" {
		bulk.WriteError(w, http.StatusBadRequest, "name is required")
		return
	}
	if rt.Objects == nil {
		bulk.WriteError(w, http.StatusInternalServerError, "no object source")
		return
	}
	raw, err := rt.Objects.GetObject(r.Context(), ref)
	if err != nil {
		if errors.Is(err, ErrObjectNotFound) {
			bulk.WriteError(w, http.StatusNotFound, err.Error())
			return
		}
		bulk.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if strings.Contains(r.Header.Get("Accept"), "application/yaml") {
		out, err := yaml.JSONToYAML(raw)
		if err != nil {
			bulk.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		if r.Method != http.MethodHead {
			cw := bulk.NewChunkWriter(w, 0)
			_, _ = cw.Write(out)
			_ = cw.Flush()
		}
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		cw := bulk.NewChunkWriter(w, 0)
		_, _ = cw.Write(raw)
		_ = cw.Flush()
	}
}
