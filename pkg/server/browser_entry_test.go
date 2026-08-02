package server

import (
	"bytes"
	"compress/gzip"
	"embed"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func browserArtifact(version string, extra map[string]*fstest.MapFile) fstest.MapFS {
	marker := []byte(`{"schemaVersion":1,"kind":"kubikles-accelerator-browser","buildVersion":"` + version + `"}` + "\n")
	files := fstest.MapFS{
		browserArtifactRoot + "/" + browserMarkerPath: {Data: marker},
		browserArtifactRoot + "/assets/browser.js":    {Data: []byte("export {}")},
		browserArtifactRoot + "/assets/browser.css":   {Data: []byte("body{}")},
	}
	for name, file := range extra {
		files[browserArtifactRoot+"/"+name] = file
	}
	return files
}

func browserEntryTestHandler(t *testing.T, gate *BrowserEntryGate) http.Handler {
	t.Helper()
	token, err := ParseCreatorToken(creatorTestToken)
	if err != nil {
		t.Fatal(err)
	}
	creator, err := NewCreatorAuthenticator(DeriveCreatorVerifier(token).Encoded())
	if err != nil {
		t.Fatal(err)
	}
	sessions := NewBrowserSessionManager(NoopBrowserSessionRevoker{})
	options := AcceleratorOptions(0, nil, CreatorOrBrowserGuard(creator, sessions))
	options.BrowserSessions = sessions
	options.BrowserEntryAvailability = gate
	server, err := NewWithOptions(nil, embed.FS{}, options)
	if err != nil {
		t.Fatal(err)
	}
	return server.Handler()
}

func requestBrowserEntry(handler http.Handler, method, target string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, target, nil)
	request.Host = "localhost"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func TestBrowserEntryGateRequiresExactAuthoritativeTree(t *testing.T) {
	missing := func(path string) fstest.MapFS {
		files := browserArtifact("v1", nil)
		delete(files, browserArtifactRoot+"/"+path)
		return files
	}
	replace := func(path string, file *fstest.MapFile) fstest.MapFS {
		files := browserArtifact("v1", nil)
		files[browserArtifactRoot+"/"+path] = file
		return files
	}
	for name, files := range map[string]fstest.MapFS{
		"valid":              browserArtifact("v1", nil),
		"mismatched version": browserArtifact("v2", nil),
		"blank version":      replace(browserMarkerPath, &fstest.MapFile{Data: []byte("{\"schemaVersion\":1,\"kind\":\"kubikles-accelerator-browser\",\"buildVersion\":\"\"}\n")}),
		"malformed marker":   replace(browserMarkerPath, &fstest.MapFile{Data: []byte("{")}),
		"oversized marker":   replace(browserMarkerPath, &fstest.MapFile{Data: bytes.Repeat([]byte("x"), browserMarkerMax+1)}),
		"unknown marker":     replace(browserMarkerPath, &fstest.MapFile{Data: []byte("{\"schemaVersion\":1,\"kind\":\"kubikles-accelerator-browser\",\"buildVersion\":\"v1\",\"extra\":true}\n")}),
		"trailing marker":    replace(browserMarkerPath, &fstest.MapFile{Data: []byte("{\"schemaVersion\":1,\"kind\":\"kubikles-accelerator-browser\",\"buildVersion\":\"v1\"}\n\n")}),
		"missing marker":     missing(browserMarkerPath),
		"missing js":         missing(browserEntryPath),
		"missing css":        missing("assets/browser.css"),
		"non-file marker":    replace(browserMarkerPath, &fstest.MapFile{Mode: fs.ModeDir}),
		"non-file js":        replace(browserEntryPath, &fstest.MapFile{Mode: fs.ModeDir}),
		"non-file css":       replace("assets/browser.css", &fstest.MapFile{Mode: fs.ModeDir}),
		"extra root":         browserArtifact("v1", map[string]*fstest.MapFile{"extra.txt": {Data: []byte("x")}}),
		"extra assets":       browserArtifact("v1", map[string]*fstest.MapFile{"assets/extra.js": {Data: []byte("x")}}),
	} {
		t.Run(name, func(t *testing.T) {
			gate := newBrowserEntryGate(files, "v1")
			wantEnabled := name == "valid"
			if got, want := gate.BrowserEntryEnabled(), wantEnabled; got != want {
				t.Fatalf("enabled = %v, want %v", got, want)
			}
			handler := browserEntryTestHandler(t, gate)
			page := requestBrowserEntry(handler, http.MethodGet, "/accelerator/browser/")
			wantPage := http.StatusNotFound
			wantMint := http.StatusServiceUnavailable
			if wantEnabled {
				wantPage = http.StatusOK
				wantMint = http.StatusCreated
			}
			if page.Code != wantPage {
				t.Fatalf("page status = %d, want %d", page.Code, wantPage)
			}
			mintRequest := httptest.NewRequest(http.MethodPost, "/api/accelerator-browser-ticket", nil)
			mintRequest.Host = "localhost"
			mintRequest.Header.Set("Authorization", "Bearer "+creatorTestToken)
			mint := httptest.NewRecorder()
			handler.ServeHTTP(mint, mintRequest)
			if mint.Code != wantMint {
				t.Fatalf("mint status = %d body=%q, want %d", mint.Code, mint.Body.String(), wantMint)
			}
		})
	}
}

func TestDisabledBrowserEntryReturnsFixedNotFoundBeforeMethodHandling(t *testing.T) {
	gate := newBrowserEntryGate(browserArtifact("other", nil), "v1")
	for _, target := range []string{"/accelerator/browser/", "/accelerator/browser/bootstrap.js", "/accelerator/browser/assets/browser.js"} {
		request := httptest.NewRequest(http.MethodPost, target, nil)
		response := httptest.NewRecorder()
		gate.serve(response, request)
		if response.Code != http.StatusNotFound || response.Header().Get("Allow") != "" {
			t.Fatalf("disabled POST %s = %d Allow %q, want fixed 404", target, response.Code, response.Header().Get("Allow"))
		}
	}
}

func TestBrowserEntryGateExactPathsAndGzip(t *testing.T) {
	gate := newBrowserEntryGate(browserArtifact("v1", nil), "v1")
	for _, tc := range []struct {
		method, target, encoding string
		want                     int
	}{
		{http.MethodGet, "/accelerator/browser/", "", http.StatusOK},
		{http.MethodHead, "/accelerator/browser/bootstrap.js", "", http.StatusOK},
		{http.MethodGet, "/accelerator/browser/assets/browser.js", "gzip", http.StatusOK},
		{http.MethodGet, "/accelerator/browser/assets/browser.js", "gzip;q=0.0", http.StatusOK},
		{http.MethodGet, "/accelerator/browser/assets/browser.js?x=1", "", http.StatusNotFound},
		{http.MethodGet, "/accelerator/browser/assets/../browser.js", "", http.StatusNotFound},
		{http.MethodPost, "/accelerator/browser/", "", http.StatusMethodNotAllowed},
	} {
		t.Run(tc.method+tc.target+tc.encoding, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, tc.target, nil)
			r.Header.Set("Accept-Encoding", tc.encoding)
			w := httptest.NewRecorder()
			gate.serve(w, r)
			if w.Code != tc.want {
				t.Fatalf("status = %d, want %d", w.Code, tc.want)
			}
			if w.Header().Get("Cache-Control") != "no-store, no-cache" {
				t.Fatal("missing browser defensive headers")
			}
			if tc.encoding == "gzip;q=0.0" && w.Header().Get("Content-Encoding") != "" {
				t.Fatal("q=0 artifact was compressed")
			}
		})
	}
}

func TestBrowserEntryActualServerHandlerContract(t *testing.T) {
	handler := browserEntryTestHandler(t, newBrowserEntryGate(browserArtifact("v1", nil), "v1"))
	request := func(method, target string, mutate func(*http.Request)) *httptest.ResponseRecorder {
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
	securityHeaders := map[string]string{
		"Cache-Control":                "no-store, no-cache",
		"Pragma":                       "no-cache",
		"Referrer-Policy":              "no-referrer",
		"X-Content-Type-Options":       "nosniff",
		"X-Frame-Options":              "DENY",
		"Cross-Origin-Opener-Policy":   "same-origin",
		"Cross-Origin-Resource-Policy": "same-origin",
		"Permissions-Policy":           "accelerometer=(), camera=(), geolocation=(), microphone=(), payment=(), usb=()",
		"Content-Security-Policy":      "default-src 'none'; base-uri 'none'; object-src 'none'; frame-ancestors 'none'; form-action 'none'; script-src 'self'; style-src 'self'; img-src 'self' data:; font-src 'self'; connect-src 'self'; manifest-src 'none'; worker-src 'none'",
	}
	assertHeaders := func(t *testing.T, response *httptest.ResponseRecorder) {
		t.Helper()
		for name, want := range securityHeaders {
			if got := response.Header().Get(name); got != want {
				t.Fatalf("%s = %q, want %q", name, got, want)
			}
		}
		for _, absent := range []string{"Access-Control-Allow-Origin", "Location", "Set-Cookie", "ETag", "Last-Modified", "Authorization"} {
			if got := response.Header().Get(absent); got != "" {
				t.Fatalf("unexpected %s = %q", absent, got)
			}
		}
	}
	for _, tc := range []struct {
		method, target, mime string
	}{
		{http.MethodGet, "/accelerator/browser/", "text/html; charset=utf-8"},
		{http.MethodGet, "/accelerator/browser/bootstrap.js", "application/javascript; charset=utf-8"},
		{http.MethodGet, "/accelerator/browser/assets/browser.js", "application/javascript; charset=utf-8"},
		{http.MethodGet, "/accelerator/browser/assets/browser.css", "text/css; charset=utf-8"},
		{http.MethodHead, "/accelerator/browser/assets/browser.js", "application/javascript; charset=utf-8"},
	} {
		t.Run(tc.method+tc.target, func(t *testing.T) {
			response := request(tc.method, tc.target, nil)
			if response.Code != http.StatusOK || response.Header().Get("Content-Type") != tc.mime {
				t.Fatalf("response = %d MIME %q", response.Code, response.Header().Get("Content-Type"))
			}
			if tc.method == http.MethodHead && response.Body.Len() != 0 {
				t.Fatalf("HEAD body = %q", response.Body.String())
			}
			assertHeaders(t, response)
		})
	}

	gzipped := request(http.MethodGet, "/accelerator/browser/assets/browser.js", func(r *http.Request) {
		r.Header.Set("Accept-Encoding", "br, gzip;q=1")
	})
	if gzipped.Code != http.StatusOK || gzipped.Header().Get("Content-Encoding") != "gzip" || gzipped.Header().Get("Vary") != "Accept-Encoding" {
		t.Fatalf("gzip response = %d %#v", gzipped.Code, gzipped.Header())
	}
	zr, err := gzip.NewReader(bytes.NewReader(gzipped.Body.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := io.ReadAll(zr)
	if closeErr := zr.Close(); err != nil || closeErr != nil || string(decoded) != "export {}" {
		t.Fatalf("gzip body = %q, read=%v close=%v", decoded, err, closeErr)
	}
	for _, tc := range []struct {
		method, target string
		want           int
	}{
		{http.MethodGet, "/accelerator/browser", http.StatusNotFound},
		{http.MethodGet, "/Accelerator/browser/", http.StatusNotFound},
		{http.MethodGet, "/accelerator/browser//", http.StatusNotFound},
		{http.MethodGet, "/accelerator/browser/?x=1", http.StatusNotFound},
		{http.MethodGet, "/accelerator/browser/assets/", http.StatusNotFound},
		{http.MethodGet, "/accelerator/browser/assets/../browser.js", http.StatusNotFound},
		{http.MethodGet, "/accelerator/browser/assets/%2e%2e/browser.js", http.StatusNotFound},
		{http.MethodGet, "/accelerator/browser/assets/browser.js%2fextra", http.StatusNotFound},
		{http.MethodGet, "/accelerator/browser/.kubikles-browser-v1.json", http.StatusNotFound},
		{http.MethodGet, "/accelerator/browser/assets/missing.js", http.StatusNotFound},
		{http.MethodGet, "/accelerator/browser/assets/browser.exe", http.StatusNotFound},
		{http.MethodGet, "/accelerator/browser/assets/api/call.js", http.StatusNotFound},
		{http.MethodPost, "/accelerator/browser/", http.StatusMethodNotAllowed},
		{http.MethodPost, "/accelerator/browser/bootstrap.js", http.StatusMethodNotAllowed},
		{http.MethodPost, "/accelerator/browser/assets/browser.js", http.StatusMethodNotAllowed},
		{http.MethodGet, "/", http.StatusNotFound},
		{http.MethodGet, "/index.html", http.StatusNotFound},
		{http.MethodGet, "/assets/browser.js", http.StatusNotFound},
	} {
		t.Run(tc.method+tc.target, func(t *testing.T) {
			response := request(tc.method, tc.target, nil)
			if response.Code != tc.want {
				t.Fatalf("status = %d, want %d", response.Code, tc.want)
			}
			if tc.target[:min(len(tc.target), len("/accelerator/browser"))] == "/accelerator/browser" {
				assertHeaders(t, response)
			}
			if response.Code >= 300 && response.Code < 400 {
				t.Fatalf("unexpected redirect %#v", response.Header())
			}
		})
	}
	for name, mutate := range map[string]func(*http.Request){
		"foreign host":   func(r *http.Request) { r.Host = "example.com" },
		"foreign origin": func(r *http.Request) { r.Header.Set("Origin", "http://example.com") },
	} {
		t.Run(name, func(t *testing.T) {
			response := request(http.MethodGet, "/accelerator/browser/", mutate)
			if response.Code != map[string]int{"foreign host": http.StatusBadRequest, "foreign origin": http.StatusForbidden}[name] {
				t.Fatalf("status = %d", response.Code)
			}
			assertHeaders(t, response)
		})
	}
	for name, mutate := range map[string]func(*http.Request){
		"range":       func(r *http.Request) { r.Header.Set("Range", "bytes=0-1") },
		"conditional": func(r *http.Request) { r.Header.Set("If-None-Match", `"anything"`) },
	} {
		t.Run(name, func(t *testing.T) {
			response := request(http.MethodGet, "/accelerator/browser/assets/browser.js", mutate)
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d", response.Code)
			}
		})
	}
}
