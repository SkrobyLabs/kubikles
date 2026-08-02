//go:build headless && accelerator

package main

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"

	"kubikles/pkg/server"
)

//go:embed all:frontend/dist
var acceleratorTestAssets embed.FS

func TestAcceleratorMainUsesFixedComposition(t *testing.T) {
	called := 0
	var output strings.Builder
	runAccelerator(context.Background(), acceleratorTestAssets, nil, func(_ context.Context, injected embed.FS, port int, label string, options AppOptions) error {
		called++
		if port != 8080 || label != "Accelerator" || options.Mode != RuntimeModeAccelerator {
			t.Fatalf("unexpected fixed composition: %d %q %#v", port, label, options)
		}
		if options.KubernetesClientFactory == nil || reflect.ValueOf(options.KubernetesClientFactory).Pointer() != reflect.ValueOf(acceleratorKubernetesClientFactory).Pointer() {
			t.Fatal("dedicated main does not use the exact in-cluster Kubernetes client factory")
		}
		if options.AgentRouter != nil || options.Lifecycle != nil {
			t.Fatalf("dedicated main supplied ordinary composition seams: %#v", options)
		}
		assertAcceleratorEmbeddedComposition(t, injected)
		serverOptions, err := serverOptionsForRuntime(options.Mode, port, nil)
		if err != nil || serverOptions.BoundaryMode != server.BoundaryModeAccelerator || serverOptions.ListenAddress != "127.0.0.1:8080" || serverOptions.ProtectedRouteGuard == nil {
			t.Fatalf("dedicated main boundary is not fail closed: %#v, %v", serverOptions, err)
		}
		return nil
	}, &output, func(int) { t.Fatal("exit called") })
	if called != 1 {
		t.Fatalf("runner calls = %d", called)
	}
	if output.Len() != 0 {
		t.Fatalf("successful run wrote stderr: %q", output.String())
	}
}

func assertAcceleratorEmbeddedComposition(t *testing.T, injected embed.FS) {
	t.Helper()
	paths := []string{
		"frontend/dist/index.html",
		"frontend/dist/accelerator-browser/.kubikles-browser-v1.json",
		"frontend/dist/accelerator-browser/assets/browser.js",
		"frontend/dist/accelerator-browser/assets/browser.css",
	}
	for _, path := range paths {
		got, err := injected.ReadFile(path)
		if err != nil || len(got) == 0 {
			t.Fatalf("injected asset %s = %d bytes, %v", path, len(got), err)
		}
		want, err := assets.ReadFile(path)
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("injected asset %s differs from production embed: %v", path, err)
		}
	}
	markerBytes, err := injected.ReadFile("frontend/dist/accelerator-browser/.kubikles-browser-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var marker struct {
		SchemaVersion int    `json:"schemaVersion"`
		Kind          string `json:"kind"`
		BuildVersion  string `json:"buildVersion"`
	}
	if err := json.Unmarshal(markerBytes, &marker); err != nil {
		t.Fatalf("decode Browser marker: %v", err)
	}
	if marker.SchemaVersion != 1 || marker.Kind != "kubikles-accelerator-browser" || marker.BuildVersion != BuildVersion {
		t.Fatalf("Browser marker = %#v, want schema 1, exact kind, BuildVersion %q", marker, BuildVersion)
	}
	foundOrdinaryGzip := false
	err = fs.WalkDir(injected, "frontend/dist", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && strings.HasSuffix(path, ".gz") && !strings.Contains(path, "/accelerator-browser/") {
			contents, readErr := injected.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			if len(contents) < 2 || contents[0] != 0x1f || contents[1] != 0x8b {
				t.Fatalf("ordinary embedded asset %s is not gzip", path)
			}
			foundOrdinaryGzip = true
		}
		return nil
	})
	if err != nil || !foundOrdinaryGzip {
		t.Fatalf("ordinary embedded gzip evidence = %v, found=%t", err, foundOrdinaryGzip)
	}
}

func TestProductionAcceleratorBrowserEntryComposition(t *testing.T) {
	creator, err := server.NewCreatorAuthenticator(testCreatorVerifier(t))
	if err != nil {
		t.Fatal(err)
	}
	protection, err := newAcceleratorHTTPProtectionWithDependencies(context.Background(), assets, &App{}, creator, "production-browser-entry", productionServerModeDependencies())
	if err != nil {
		t.Fatal(err)
	}
	if protection.BrowserEntry == nil || !protection.BrowserEntry.BrowserEntryEnabled() {
		t.Fatalf("production embedded Browser artifact is disabled for BuildVersion %q", BuildVersion)
	}
	options, err := serverOptionsForRuntime(RuntimeModeAccelerator, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	installAcceleratorHTTPProtection(&options, protection)
	newHandler := func(t *testing.T, configured server.Options) http.Handler {
		t.Helper()
		srv, err := server.NewWithOptions(nil, assets, configured)
		if err != nil {
			t.Fatal(err)
		}
		return srv.Handler()
	}
	handler := newHandler(t, options)
	request := func(handler http.Handler, method, target string, mutate func(*http.Request)) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, target, nil)
		r.Host = "localhost"
		if mutate != nil {
			mutate(r)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	assertBoundary := func(t *testing.T, response *httptest.ResponseRecorder) {
		t.Helper()
		for name, want := range map[string]string{
			"Cache-Control":                "no-store, no-cache",
			"Pragma":                       "no-cache",
			"Referrer-Policy":              "no-referrer",
			"X-Content-Type-Options":       "nosniff",
			"X-Frame-Options":              "DENY",
			"Cross-Origin-Opener-Policy":   "same-origin",
			"Cross-Origin-Resource-Policy": "same-origin",
			"Permissions-Policy":           "accelerometer=(), camera=(), geolocation=(), microphone=(), payment=(), usb=()",
			"Content-Security-Policy":      "default-src 'none'; base-uri 'none'; object-src 'none'; frame-ancestors 'none'; form-action 'none'; script-src 'self'; style-src 'self'; img-src 'self' data:; font-src 'self'; connect-src 'self'; manifest-src 'none'; worker-src 'none'",
		} {
			if got := response.Header().Get(name); got != want {
				t.Fatalf("%s = %q, want %q", name, got, want)
			}
		}
		for _, name := range []string{"Access-Control-Allow-Origin", "Location", "Set-Cookie", "ETag", "Last-Modified"} {
			if got := response.Header().Get(name); got != "" {
				t.Fatalf("unexpected %s = %q", name, got)
			}
		}
	}
	for _, tc := range []struct {
		target, mime string
	}{
		{"/accelerator/browser/", "text/html; charset=utf-8"},
		{"/accelerator/browser/bootstrap.js", "application/javascript; charset=utf-8"},
		{"/accelerator/browser/assets/browser.js", "application/javascript; charset=utf-8"},
		{"/accelerator/browser/assets/browser.css", "text/css; charset=utf-8"},
	} {
		t.Run(tc.target, func(t *testing.T) {
			response := request(handler, http.MethodGet, tc.target, nil)
			if response.Code != http.StatusOK || response.Header().Get("Content-Type") != tc.mime || response.Body.Len() == 0 {
				t.Fatalf("production asset = %d MIME %q bytes %d", response.Code, response.Header().Get("Content-Type"), response.Body.Len())
			}
			assertBoundary(t, response)
		})
	}
	gzipResponse := request(handler, http.MethodGet, "/accelerator/browser/assets/browser.js", func(r *http.Request) { r.Header.Set("Accept-Encoding", "gzip") })
	if gzipResponse.Code != http.StatusOK || gzipResponse.Header().Get("Content-Encoding") != "gzip" || gzipResponse.Header().Get("Vary") != "Accept-Encoding" {
		t.Fatalf("production gzip = %d %#v", gzipResponse.Code, gzipResponse.Header())
	}
	for _, tc := range []struct {
		method, target string
		want           int
	}{
		{http.MethodHead, "/accelerator/browser/assets/browser.js", http.StatusOK},
		{http.MethodPost, "/accelerator/browser/", http.StatusMethodNotAllowed},
		{http.MethodPost, "/accelerator/browser/assets/browser.js", http.StatusMethodNotAllowed},
		{http.MethodGet, "/accelerator/browser", http.StatusNotFound},
		{http.MethodGet, "/accelerator/browser/?query=forbidden", http.StatusNotFound},
		{http.MethodGet, "/accelerator/browser/assets/%2e%2e/browser.js", http.StatusNotFound},
		{http.MethodGet, "/accelerator/browser/assets/browser.js%2fextra", http.StatusNotFound},
		{http.MethodGet, "/accelerator/browser/assets/missing.js", http.StatusNotFound},
		{http.MethodGet, "/", http.StatusNotFound},
		{http.MethodGet, "/index.html", http.StatusNotFound},
		{http.MethodGet, "/assets/index.js", http.StatusNotFound},
	} {
		response := request(handler, tc.method, tc.target, nil)
		if response.Code != tc.want {
			t.Fatalf("%s %s = %d, want %d", tc.method, tc.target, response.Code, tc.want)
		}
		if strings.HasPrefix(tc.target, "/accelerator/browser") {
			assertBoundary(t, response)
		}
		if tc.method == http.MethodHead && response.Body.Len() != 0 {
			t.Fatalf("HEAD body = %d bytes", response.Body.Len())
		}
	}
	for _, tc := range []struct {
		name string
		want int
		set  func(*http.Request)
	}{
		{"foreign Host", http.StatusBadRequest, func(r *http.Request) { r.Host = "example.com" }},
		{"foreign Origin", http.StatusForbidden, func(r *http.Request) { r.Header.Set("Origin", "http://example.com") }},
	} {
		response := request(handler, http.MethodGet, "/accelerator/browser/", tc.set)
		if response.Code != tc.want {
			t.Fatalf("%s = %d, want %d", tc.name, response.Code, tc.want)
		}
		assertBoundary(t, response)
	}
	mint := request(handler, http.MethodPost, "/api/accelerator-browser-ticket", func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
	})
	if mint.Code != http.StatusCreated {
		t.Fatalf("enabled production mint = %d %q", mint.Code, mint.Body.String())
	}

	disabledOptions := options
	disabledOptions.BrowserEntryAvailability = server.NewBrowserEntryGate(assets, BuildVersion+"-mismatch")
	disabled := newHandler(t, disabledOptions)
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		response := request(disabled, method, "/accelerator/browser/", nil)
		if response.Code != http.StatusNotFound || response.Header().Get("Allow") != "" {
			t.Fatalf("disabled %s page = %d Allow %q", method, response.Code, response.Header().Get("Allow"))
		}
		assertBoundary(t, response)
	}
	disabledMint := request(disabled, http.MethodPost, "/api/accelerator-browser-ticket", func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
	})
	if disabledMint.Code != http.StatusServiceUnavailable || disabledMint.Body.String() != "{\"error\":\"unavailable\"}\n" {
		t.Fatalf("disabled production mint = %d %q", disabledMint.Code, disabledMint.Body.String())
	}
}

func TestAcceleratorMainRejectsArgumentsAndPropagatesFailures(t *testing.T) {
	var output strings.Builder
	exits := 0
	runAccelerator(context.Background(), acceleratorTestAssets, []string{"--port=9"}, func(context.Context, embed.FS, int, string, AppOptions) error { t.Fatal("runner called"); return nil }, &output, func(code int) {
		if code != 1 {
			t.Fatalf("exit %d", code)
		}
		exits++
	})
	if exits != 1 || !strings.Contains(output.String(), "does not accept") {
		t.Fatalf("argument result: exits=%d output=%q", exits, output.String())
	}
	output.Reset()
	runAccelerator(context.Background(), acceleratorTestAssets, nil, func(context.Context, embed.FS, int, string, AppOptions) error { return context.Canceled }, &output, func(int) { t.Fatal("cancellation exited") })
	if output.Len() != 0 {
		t.Fatalf("cancellation wrote stderr: %q", output.String())
	}
	output.Reset()
	runAccelerator(context.Background(), acceleratorTestAssets, nil, func(context.Context, embed.FS, int, string, AppOptions) error { return errors.New("secret sentinel") }, &output, func(code int) {
		if code != 1 {
			t.Fatal(code)
		}
		exits++
	})
	if exits != 2 {
		t.Fatalf("failure exit count = %d", exits)
	}
	if got := output.String(); got != "accelerator server failed\n" || strings.Contains(got, "secret sentinel") {
		t.Fatalf("failure stderr = %q", got)
	}
}

func TestAcceleratorSignalContextCancels(t *testing.T) {
	deliver := make(chan os.Signal, 1)
	runnerStarted := make(chan struct{})
	cleanupReturned := make(chan struct{})
	mainReturned := make(chan struct{})
	var notified []os.Signal
	stopCalled := false
	notify := func(parent context.Context, signals ...os.Signal) (context.Context, context.CancelFunc) {
		notified = append([]os.Signal(nil), signals...)
		ctx, cancel := context.WithCancel(parent)
		go func() {
			select {
			case signal := <-deliver:
				if signal == syscall.SIGTERM {
					cancel()
				}
			case <-ctx.Done():
			}
		}()
		return ctx, func() {
			stopCalled = true
			cancel()
		}
	}
	go func() {
		acceleratorMain(notify, acceleratorTestAssets, nil, func(ctx context.Context, _ embed.FS, _ int, _ string, _ AppOptions) error {
			close(runnerStarted)
			<-ctx.Done()
			close(cleanupReturned)
			return ctx.Err()
		}, io.Discard, func(int) { panic("signal cancellation exited") })
		close(mainReturned)
	}()
	<-runnerStarted
	deliver <- syscall.SIGTERM
	select {
	case <-mainReturned:
	case <-time.After(2 * time.Second):
		t.Fatal("dedicated main did not return after SIGTERM cancellation")
	}
	select {
	case <-cleanupReturned:
	default:
		t.Fatal("main returned before runner cleanup")
	}
	if !stopCalled {
		t.Fatal("signal notification cleanup was not called")
	}
	if len(notified) != 2 || notified[0] != os.Interrupt || notified[1] != syscall.SIGTERM {
		t.Fatalf("registered signals = %#v, want SIGINT and SIGTERM", notified)
	}
}
