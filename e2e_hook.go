//go:build e2e && !headless

package main

import (
	"context"
	"crypto/subtle"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"kubikles/pkg/crashlog"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// Test hook for driving the real desktop window from a script. It exists only
// in builds made with the "e2e" tag, and even there it stays off unless both
// KUBIKLES_E2E_PORT and KUBIKLES_E2E_TOKEN are set. It listens on loopback and
// every request must carry the token, because /eval runs arbitrary script in
// the app's webview.
//
//	POST /eval            body: the body of an async function; replies with
//	                      {"ok":true,"value":...} or {"ok":false,"error":"..."}
//	POST /window?op=...   hide | show | minimise | unminimise

// E2EBridge is bound next to App so the webview can hand an /eval result back.
type E2EBridge struct {
	mu      sync.Mutex
	next    int
	pending map[int]chan string
}

var e2eBridge = &E2EBridge{pending: make(map[int]chan string)}

// Result is called from the webview with the JSON outcome of one /eval.
func (b *E2EBridge) Result(id int, payload string) {
	b.mu.Lock()
	ch := b.pending[id]
	delete(b.pending, id)
	b.mu.Unlock()
	if ch != nil {
		ch <- payload
	}
}

func e2eBindings() []interface{} { return []interface{}{e2eBridge} }

// minE2ETokenLen is the shortest accepted token.
const minE2ETokenLen = 32

func startE2EHook(ctx context.Context) {
	port, token := os.Getenv("KUBIKLES_E2E_PORT"), os.Getenv("KUBIKLES_E2E_TOKEN")
	if port == "" || token == "" {
		return
	}
	// Keep the token out of child processes (kubectl exec plugins, terminals).
	_ = os.Unsetenv("KUBIKLES_E2E_TOKEN")
	// The hook runs arbitrary script in the webview, so a guessable token is refused.
	if len(token) < minE2ETokenLen {
		crashlog.Log("e2e hook: refusing to start, KUBIKLES_E2E_TOKEN must be at least %d characters", minE2ETokenLen)
		return
	}
	ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", port))
	if err != nil {
		crashlog.Log("e2e hook: %v", err)
		return
	}
	authed := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			got := r.Header.Get("X-E2E-Token")
			if r.Method != http.MethodPost || subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			h(w, r)
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/eval", authed(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		timeout := 30 * time.Second
		if ms, err := strconv.Atoi(r.URL.Query().Get("timeoutMs")); err == nil && ms > 0 {
			timeout = time.Duration(ms) * time.Millisecond
		}
		b := e2eBridge
		ch := make(chan string, 1)
		b.mu.Lock()
		b.next++
		id := b.next
		b.pending[id] = ch
		b.mu.Unlock()
		wailsruntime.WindowExecJS(ctx, fmt.Sprintf(
			"(async()=>{let r;try{r={ok:true,value:await (async()=>{%s\n})()}}catch(e){r={ok:false,error:String((e&&e.stack)||e)}}"+
				"let s;try{s=JSON.stringify(r)}catch(e){s=JSON.stringify({ok:false,error:'unserialisable result: '+e})}"+
				"window.go.main.E2EBridge.Result(%d,s)})()", body, id))
		w.Header().Set("Content-Type", "application/json")
		select {
		case payload := <-ch:
			_, _ = io.WriteString(w, payload)
		case <-time.After(timeout):
			b.mu.Lock()
			delete(b.pending, id)
			b.mu.Unlock()
			w.WriteHeader(http.StatusGatewayTimeout)
			_, _ = io.WriteString(w, `{"ok":false,"error":"timeout"}`)
		}
	}))
	mux.HandleFunc("/window", authed(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("op") {
		case "hide":
			wailsruntime.WindowHide(ctx)
		case "show":
			wailsruntime.WindowShow(ctx)
		case "minimise":
			wailsruntime.WindowMinimise(ctx)
		case "unminimise":
			wailsruntime.WindowUnminimise(ctx)
		default:
			http.Error(w, "unknown op", http.StatusBadRequest)
			return
		}
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	crashlog.Log("e2e hook listening on %s", ln.Addr())
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = srv.Serve(ln) }()
}
