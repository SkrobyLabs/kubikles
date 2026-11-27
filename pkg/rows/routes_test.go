package rows

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"

	"kubikles/pkg/bulk"
)

type fakeObjects struct{}

func (fakeObjects) GetObject(ctx context.Context, ref ObjectRef) (json.RawMessage, error) {
	if ref.Name == "missing" {
		return nil, ErrObjectNotFound
	}
	return json.RawMessage(fmt.Sprintf(`{"kind":"ConfigMap","metadata":{"name":%q,"namespace":%q}}`, ref.Name, ref.Namespace)), nil
}

const testToken = "tok-123"

func routeServer(t *testing.T, env *testEnv) *httptest.Server {
	t.Helper()
	rt := &Routes{Views: func() *ViewManager { return env.m }, Objects: fakeObjects{}}
	h := bulk.NewHandler(bulk.Deps{Token: testToken, Routes: []bulk.RouteProvider{rt}})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

func get(t *testing.T, srv *httptest.Server, path string, hdr map[string]string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, srv.URL+path, nil)
	req.Header.Set(bulk.HeaderMarker, "1")
	req.Header.Set(bulk.HeaderToken, testToken)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { res.Body.Close() })
	return res
}

func TestWindowRoute(t *testing.T) {
	env := newEnv(t)
	sh := env.p.shard("configmaps")
	for i := 0; i < 5; i++ {
		sh.upsert(cm("default", fmt.Sprintf("n%d", i), t0, map[string]string{"k": "v"}))
	}
	id, _ := openCM(t, env)
	srv := routeServer(t, env)

	res := get(t, srv, bulk.Base+"/views/"+id+"/window?offset=0&limit=3&have=0", nil)
	if res.StatusCode != 200 || !strings.HasPrefix(res.Header.Get("Content-Type"), "application/json") {
		t.Fatalf("status %d type %s", res.StatusCode, res.Header.Get("Content-Type"))
	}
	if res.Header.Get("Cache-Control") != "no-store" || res.Header.Get("Access-Control-Allow-Origin") != "" || res.Header.Get("Content-Encoding") != "" {
		t.Fatalf("headers %v", res.Header)
	}
	var w ViewWindow
	if err := json.NewDecoder(res.Body).Decode(&w); err != nil {
		t.Fatal(err)
	}
	if len(w.Order) != 3 || len(w.Rows) != 3 || w.Filtered != 5 || w.Total != 5 || w.State.Phase != PhaseComplete {
		t.Fatalf("window %+v", w)
	}

	// have semantics over the wire.
	res = get(t, srv, fmt.Sprintf("%s/views/%s/window?offset=0&limit=3&have=%d", bulk.Base, id, w.Version), nil)
	var w2 ViewWindow
	_ = json.NewDecoder(res.Body).Decode(&w2)
	if len(w2.Order) != 3 || len(w2.Rows) != 0 {
		t.Fatalf("have must suppress known rows: %+v", w2)
	}
	// Raw shape check: rows is an object keyed by uid with the documented keys.
	res = get(t, srv, bulk.Base+"/views/"+id+"/window?limit=1", nil)
	var raw map[string]json.RawMessage
	_ = json.NewDecoder(res.Body).Decode(&raw)
	for _, k := range []string{"version", "total", "filtered", "offset", "order", "rows", "state"} {
		if _, ok := raw[k]; !ok {
			t.Errorf("window JSON lacks %q", k)
		}
	}
	var rowsRaw map[string]map[string]json.RawMessage
	_ = json.Unmarshal(raw["rows"], &rowsRaw)
	for _, row := range rowsRaw {
		for _, k := range []string{"seq", "ns", "name", "ts", "del", "c"} {
			if _, ok := row[k]; !ok {
				t.Errorf("row JSON lacks %q", k)
			}
		}
	}

	// Errors.
	if res := get(t, srv, bulk.Base+"/views/nope/window", nil); res.StatusCode != http.StatusGone {
		t.Fatalf("unknown view must be 410, got %d", res.StatusCode)
	}
	if res := get(t, srv, bulk.Base+"/views/"+id+"/window?offset=-1", nil); res.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad offset %d", res.StatusCode)
	}
	if res := get(t, srv, bulk.Base+"/views/"+id+"/window?have=x", nil); res.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad have %d", res.StatusCode)
	}
	if res := get(t, srv, bulk.Base+"/views/"+id+"/bogus", nil); res.StatusCode != http.StatusNotFound {
		t.Fatalf("bogus route %d", res.StatusCode)
	}
	// Limits are clamped, not rejected.
	if res := get(t, srv, bulk.Base+"/views/"+id+"/window?limit=999999", nil); res.StatusCode != 200 {
		t.Fatalf("clamped limit %d", res.StatusCode)
	}
}

func TestRoutesInheritGuards(t *testing.T) {
	env := newEnv(t)
	id, _ := openCM(t, env)
	srv := routeServer(t, env)
	path := bulk.Base + "/views/" + id + "/window"

	req, _ := http.NewRequest(http.MethodGet, srv.URL+path, nil)
	res, _ := http.DefaultClient.Do(req)
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("no marker: %d", res.StatusCode)
	}
	req.Header.Set(bulk.HeaderMarker, "1")
	req.Header.Set(bulk.HeaderToken, "wrong")
	res, _ = http.DefaultClient.Do(req)
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("wrong token: %d", res.StatusCode)
	}
	post, _ := http.NewRequest(http.MethodPost, srv.URL+path, strings.NewReader("{}"))
	post.Header.Set(bulk.HeaderMarker, "1")
	post.Header.Set(bulk.HeaderToken, testToken)
	res, _ = http.DefaultClient.Do(post)
	if res.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("POST must be refused: %d", res.StatusCode)
	}
	// Conditional headers are dropped: never a 304.
	res = get(t, srv, path, map[string]string{"If-None-Match": "*", "If-Modified-Since": "Mon, 02 Jan 2006 15:04:05 GMT"})
	if res.StatusCode != 200 {
		t.Fatalf("conditional request: %d", res.StatusCode)
	}
	// HEAD answers without a body.
	head, _ := http.NewRequest(http.MethodHead, srv.URL+bulk.Base+"/views/"+id+"/refs", nil)
	head.Header.Set(bulk.HeaderMarker, "1")
	head.Header.Set(bulk.HeaderToken, testToken)
	res, _ = http.DefaultClient.Do(head)
	if res.StatusCode != 200 || res.ContentLength > 0 {
		t.Fatalf("HEAD: %d len %d", res.StatusCode, res.ContentLength)
	}
}

func TestRefsAndRowsStreams(t *testing.T) {
	env := newEnv(t)
	sh := env.p.shard("configmaps")
	for _, n := range []string{"c", "a", "b", "d"} {
		sh.upsert(cm("default", n, t0, map[string]string{"k": "v"}))
	}
	id, _ := openCM(t, env)
	srv := routeServer(t, env)

	res := get(t, srv, bulk.Base+"/views/"+id+"/refs?offset=1&limit=2", nil)
	if !strings.HasPrefix(res.Header.Get("Content-Type"), "application/x-ndjson") {
		t.Fatalf("type %s", res.Header.Get("Content-Type"))
	}
	var lines []string
	sc := bufio.NewScanner(res.Body)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	if len(lines) != 3 {
		t.Fatalf("lines %v", lines)
	}
	var hdr struct{ Version, Total, Filtered int }
	_ = json.Unmarshal([]byte(lines[0]), &hdr)
	if hdr.Total != 4 || hdr.Filtered != 4 || hdr.Version == 0 {
		t.Fatalf("header %s", lines[0])
	}
	if res := get(t, srv, bulk.Base+"/views/"+id+"/refs?offset=0&limit=2&version="+strconv.Itoa(hdr.Version+99), nil); res.StatusCode != http.StatusConflict {
		t.Fatalf("refs at a stale version: %d", res.StatusCode)
	}
	if res := get(t, srv, bulk.Base+"/views/"+id+"/refs?offset=0&limit=2&version="+strconv.Itoa(hdr.Version), nil); res.StatusCode != http.StatusOK {
		t.Fatalf("refs at the current version: %d", res.StatusCode)
	}
	var ref []string
	_ = json.Unmarshal([]byte(lines[1]), &ref)
	if len(ref) != 3 || ref[0] != "uid-default-b" || ref[1] != "default" || ref[2] != "b" {
		t.Fatalf("ref line %s", lines[1])
	}

	res = get(t, srv, bulk.Base+"/views/"+id+"/rows", nil)
	sc = bufio.NewScanner(res.Body)
	n := 0
	for sc.Scan() {
		n++
		if n == 1 {
			var h struct{ Columns []string }
			_ = json.Unmarshal(sc.Bytes(), &h)
			if len(h.Columns) != 7 {
				t.Fatalf("rows header %s", sc.Text())
			}
			continue
		}
		var row struct {
			UID, NS, Name string
			C             []interface{}
		}
		if err := json.Unmarshal(sc.Bytes(), &row); err != nil || len(row.C) != 7 || row.UID == "" {
			t.Fatalf("row %s: %v", sc.Text(), err)
		}
	}
	if n != 5 {
		t.Fatalf("rows lines %d", n)
	}
	if res := get(t, srv, bulk.Base+"/views/gone/refs", nil); res.StatusCode != http.StatusGone {
		t.Fatalf("refs on a closed view: %d", res.StatusCode)
	}
}

func TestObjectRoute(t *testing.T) {
	env := newEnv(t)
	srv := routeServer(t, env)
	res := get(t, srv, bulk.Base+"/objects/configmaps?ns=default&name=a", nil)
	body := new(strings.Builder)
	_, _ = bufioCopy(body, res)
	if res.StatusCode != 200 || !strings.Contains(body.String(), `"name":"a"`) || !strings.HasPrefix(res.Header.Get("Content-Type"), "application/json") {
		t.Fatalf("json object: %d %s", res.StatusCode, body.String())
	}
	res = get(t, srv, bulk.Base+"/objects/configmaps?ns=default&name=a", map[string]string{"Accept": "application/yaml"})
	body.Reset()
	_, _ = bufioCopy(body, res)
	if !strings.HasPrefix(res.Header.Get("Content-Type"), "application/yaml") || !strings.Contains(body.String(), "name: a") {
		t.Fatalf("yaml object: %s %q", res.Header.Get("Content-Type"), body.String())
	}
	if res := get(t, srv, bulk.Base+"/objects/configmaps?name=missing", nil); res.StatusCode != http.StatusNotFound {
		t.Fatalf("missing: %d", res.StatusCode)
	}
	if res := get(t, srv, bulk.Base+"/objects/configmaps", nil); res.StatusCode != http.StatusBadRequest {
		t.Fatalf("no name: %d", res.StatusCode)
	}
	if res := get(t, srv, bulk.Base+"/objects/a/b?name=x", nil); res.StatusCode != http.StatusNotFound {
		t.Fatalf("slash in type: %d", res.StatusCode)
	}
	if res := get(t, srv, bulk.Base+"/objects/crd:example.com/v1/widgets?name=x", nil); res.StatusCode != 200 {
		t.Fatalf("crd type: %d", res.StatusCode)
	}
}

func bufioCopy(b *strings.Builder, res *http.Response) (int64, error) {
	sc := bufio.NewReader(res.Body)
	var n int64
	for {
		line, err := sc.ReadString('\n')
		b.WriteString(line)
		n += int64(len(line))
		if err != nil {
			return n, nil
		}
	}
}

func TestRoutesBeforeViewsExist(t *testing.T) {
	rt := &Routes{Views: func() *ViewManager { return nil }}
	h := bulk.NewHandler(bulk.Deps{Token: testToken, Routes: []bulk.RouteProvider{rt}})
	srv := httptest.NewServer(h)
	defer srv.Close()
	res := get(t, srv, bulk.Base+"/views/x/window", nil)
	if res.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status %d", res.StatusCode)
	}
}

func TestGeneratedSchemaIsCurrent(t *testing.T) {
	committed, err := os.ReadFile("../../frontend/src/generated/rowSchemas.ts")
	if err != nil {
		t.Fatalf("read generated schema: %v (run go generate ./...)", err)
	}
	if string(committed) != string(GenerateTS()) {
		t.Fatal("frontend/src/generated/rowSchemas.ts is stale: run go generate ./...")
	}
}

func TestGeneratedSchemaContent(t *testing.T) {
	out := string(GenerateTS())
	for _, want := range []string{
		"export interface NamespacesCells {", "export interface ConfigmapsCells {", "export interface SecretsCells {", "export interface EventsCells {",
		"keyCount: number;", "age: string | null;",
		"predefined: ['Normal', 'Warning']", "aliases: ['phase', 'state']", "presets: ['helm']", "migrated: true",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("generated schema lacks %q", want)
		}
	}
}
