package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"kubikles/pkg/bulk"
	"kubikles/pkg/events"
)

func TestBulkTokenIsStableRandomAndWithheldWhenDisabled(t *testing.T) {
	t.Setenv("KUBIKLES_DISABLE_BULK", "")
	a := &App{}
	info := a.GetBulkInfo()
	if !info.Enabled || len(info.Token) < 32 || info.Base != bulk.Base || info.Version != bulkProtocolVersion {
		t.Fatalf("info = %+v", info)
	}
	if again := a.GetBulkInfo(); again.Token != info.Token {
		t.Fatal("token must be stable for the process")
	}
	if other := (&App{}).GetBulkInfo(); other.Token == info.Token {
		t.Fatal("tokens must differ between processes")
	}

	a.SetBulkTransport("off")
	off := a.GetBulkInfo()
	if off.Enabled || off.Token != "" {
		t.Fatalf("setting off must disable and withhold the token: %+v", off)
	}
	a.SetBulkTransport("auto")
	if on := a.GetBulkInfo(); !on.Enabled || on.Token != info.Token {
		t.Fatalf("auto must re-enable with the same token: %+v", on)
	}
	a.SetBulkTransport("nonsense")
	if !a.GetBulkInfo().Enabled {
		t.Fatal("unknown mode must behave as auto")
	}
}

func TestBulkEnvKillSwitch(t *testing.T) {
	a := &App{}
	for _, v := range []string{"1", "true"} {
		t.Setenv("KUBIKLES_DISABLE_BULK", v)
		if info := a.GetBulkInfo(); info.Enabled || info.Token != "" {
			t.Fatalf("env %q: %+v", v, info)
		}
	}
	t.Setenv("KUBIKLES_DISABLE_BULK", "0")
	if !a.GetBulkInfo().Enabled {
		t.Fatal("env 0 must not disable")
	}
}

func TestBulkStreamingHintAndServerFlag(t *testing.T) {
	t.Setenv("KUBIKLES_DISABLE_BULK", "")
	server := &App{emitter: events.EmitterFunc(func(string, ...interface{}) {})}
	if info := server.GetBulkInfo(); !info.Server || !info.Streaming {
		t.Fatalf("server mode: %+v", info)
	}
	desktop := &App{emitter: events.NewWailsEmitter(nil)}
	if info := desktop.GetBulkInfo(); info.Server {
		t.Fatalf("desktop mode reported as server: %+v", info)
	}
}

func TestReportBulkProbeIsStored(t *testing.T) {
	a := &App{}
	if a.GetBulkProbeReport() != nil {
		t.Fatal("no report expected yet")
	}
	a.ReportBulkProbe(map[string]interface{}{"fetch": true, "hostLevel": "W"})
	got := a.GetBulkProbeReport()
	if got["fetch"] != true || got["hostLevel"] != "W" {
		t.Fatalf("report = %v", got)
	}
	a.ReportBulkProbe(map[string]interface{}{"fetch": false})
	if a.GetBulkProbeReport()["fetch"] != false {
		t.Fatal("latest report must win")
	}
}

func TestBulkBenchIPCMatchesBenchRoute(t *testing.T) {
	a := &App{}
	if n := len(a.BulkBenchIPC(0)); n != 1 {
		t.Fatalf("clamped low: %d", n)
	}
	if n := len(a.BulkBenchIPC(10_000_000)); n != 100000 {
		t.Fatalf("clamped high: %d", n)
	}
	ipc, _ := json.Marshal(a.BulkBenchIPC(3))
	want, _ := json.Marshal(bulk.BenchObjects(3))
	if string(ipc) != string(want) {
		t.Fatal("IPC bench payload differs from the route payload")
	}
}

func TestBulkHandlerDispatchesThroughAppCaller(t *testing.T) {
	t.Setenv("KUBIKLES_DISABLE_BULK", "")
	a := &App{}
	h := a.newBulkHandler(false, nil)
	token := a.GetBulkInfo().Token

	get := func(path string, withToken bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		r.Header.Set(bulk.HeaderMarker, "1")
		if withToken {
			r.Header.Set(bulk.HeaderToken, token)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		return rec
	}
	if rec := get(bulk.Base+"/probe/ping", false); rec.Code != http.StatusForbidden {
		t.Fatalf("no token: %d", rec.Code)
	}
	rec := get(bulk.Base+"/probe/ping", true)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"ok":true`) {
		t.Fatalf("ping: %d %s", rec.Code, rec.Body.String())
	}
	// ListContexts reaches the real dispatcher; with no client it reports the
	// application error in the standard envelope.
	rec = get(bulk.Base+"/call/ListContexts", true)
	if rec.Code != 500 || !strings.Contains(rec.Body.String(), "k8s client not initialized") {
		t.Fatalf("call: %d %s", rec.Code, rec.Body.String())
	}
	if rec = get(bulk.Base+"/call/DeleteContext?args=%5B%22x%22%5D", true); rec.Code != http.StatusForbidden {
		t.Fatalf("mutating call must be refused: %d", rec.Code)
	}
}
