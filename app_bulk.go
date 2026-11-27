package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"

	"kubikles/pkg/bulk"
	"kubikles/pkg/crashlog"
	"kubikles/pkg/debug"
	"kubikles/pkg/events"
)

// bulkProtocolVersion is bumped when the /_kubikles/v1 wire format changes.
const bulkProtocolVersion = 1

// BulkInfo tells the frontend whether and how to reach the bulk transport.
type BulkInfo struct {
	Enabled bool `json:"enabled"`
	// Base is the route prefix, e.g. "/_kubikles/v1".
	Base string `json:"base"`
	// Token must be sent as X-Kubikles-Token. Empty when disabled.
	Token string `json:"token"`
	// Streaming is a platform hint: bodies stream on macOS and in server mode;
	// the Windows desktop writer buffers the whole body. The runtime probe
	// measures the real behaviour.
	Streaming bool `json:"streaming"`
	// Server is true when running as an HTTP server (browser client).
	Server  bool `json:"server"`
	Version int  `json:"version"`
}

// bulkState is the App's bulk transport state. The zero value is usable.
type bulkState struct {
	once  sync.Once
	token string

	mu     sync.RWMutex
	mode   string // "auto" (default) or "off"
	report map[string]interface{}
}

func (b *bulkState) ensureToken() string {
	b.once.Do(func() {
		buf := make([]byte, 32)
		if _, err := rand.Read(buf); err != nil {
			// crypto/rand failing is fatal for a security token.
			crashlog.LogFatal("bulk token: %v", err)
		}
		b.token = hex.EncodeToString(buf)
	})
	return b.token
}

func (b *bulkState) enabled() bool {
	if v := strings.TrimSpace(os.Getenv("KUBIKLES_DISABLE_BULK")); v == "1" || strings.EqualFold(v, "true") {
		return false
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.mode != "off"
}

func (a *App) isServerMode() bool {
	_, wails := a.emitter.(*events.WailsEmitter)
	return !wails
}

// GetBulkInfo returns the bulk transport descriptor. The token is generated
// once per process with crypto/rand and withheld while the transport is
// disabled (setting bulkTransport=off or env KUBIKLES_DISABLE_BULK=1).
func (a *App) GetBulkInfo() BulkInfo {
	info := BulkInfo{
		Base:    bulk.Base,
		Server:  a.isServerMode(),
		Version: bulkProtocolVersion,
	}
	info.Streaming = info.Server || runtime.GOOS == "darwin"
	if a.bulk.enabled() {
		info.Enabled = true
		info.Token = a.bulk.ensureToken()
	}
	return info
}

// SetBulkTransport applies the bulkTransport setting: "auto" (default) or
// "off". Anything else is treated as "auto".
func (a *App) SetBulkTransport(mode string) {
	if mode != "off" {
		mode = "auto"
	}
	a.bulk.mu.Lock()
	a.bulk.mode = mode
	a.bulk.mu.Unlock()
	debug.LogConfig("Bulk transport", map[string]interface{}{"mode": mode})
}

// ReportBulkProbe records the result of the in-app capability probe. It is
// logged for field diagnostics and kept for GetBulkProbeReport.
func (a *App) ReportBulkProbe(report map[string]interface{}) {
	a.bulk.mu.Lock()
	a.bulk.report = report
	a.bulk.mu.Unlock()
	if b, err := json.Marshal(report); err == nil {
		if len(b) > 4096 {
			b = append(b[:4096:4096], []byte("...")...)
		}
		crashlog.Log("bulk probe: %s", b)
	}
	debug.LogPerformance("Bulk probe", report)
}

// GetBulkProbeReport returns the latest ReportBulkProbe payload (nil if none).
func (a *App) GetBulkProbeReport() map[string]interface{} {
	a.bulk.mu.RLock()
	defer a.bulk.mu.RUnlock()
	return a.bulk.report
}

// BulkBenchIPC returns the same objects as /_kubikles/v1/probe/bench so the
// probe can time the binding path against the fetch path.
func (a *App) BulkBenchIPC(objects int) []map[string]interface{} {
	if objects < 1 {
		objects = 1
	}
	if objects > 100000 {
		objects = 100000
	}
	return bulk.BenchObjects(objects)
}

// newBulkHandler builds the /_kubikles/ handler. Desktop passes
// compress=false (the Wails scheme handlers must not see Content-Encoding);
// server mode passes gzip and a Host check.
func (a *App) newBulkHandler(compress bool, allowHost func(string) bool) http.Handler {
	return bulk.NewHandler(bulk.Deps{
		Caller: NewAppMethodCaller(a),
		// The token is fixed per process; a disabled transport hides it from
		// GetBulkInfo, so clients cannot authenticate.
		Token:     a.bulk.ensureToken(),
		AllowHost: allowHost,
		Compress:  compress,
		Headless:  isHeadlessBuild,
		GOOS:      runtime.GOOS,
		Routes:    []bulk.RouteProvider{a.viewRoutes()},
	})
}
