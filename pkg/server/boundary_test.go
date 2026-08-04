package server

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"kubikles/pkg/agent"
)

//go:embed all:frontend/dist
var acceleratorBoundaryTestAssets embed.FS

func newAcceleratorTestServer(t *testing.T, caller *recordingMethodCaller, readiness ReadinessProvider, guard ProtectedRouteGuard) *Server {
	t.Helper()
	options := AcceleratorOptions(0, readiness, func(next http.Handler) http.Handler {
		guarded := guard(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			guarded.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), creatorContextKey{}, agent.AuthenticatedCallContext{PrincipalID: "creator-test", SessionID: "http-test"})))
		})
	})
	options.MethodAuthorizer = MethodAuthorizerFunc(func(agent.AuthenticatedCallContext, string) bool { return true })
	server, err := NewWithOptions(caller, embed.FS{}, options)
	if err != nil {
		t.Fatal(err)
	}
	return server
}

func requestBoundary(handler http.Handler, method, target, host string, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.Host = host
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func TestAcceleratorBoundaryRejectsEmbeddedStaticRoutes(t *testing.T) {
	options := AcceleratorOptions(0, nil, func(next http.Handler) http.Handler { return next })
	options.MethodAuthorizer = MethodAuthorizerFunc(func(agent.AuthenticatedCallContext, string) bool { return true })
	server, err := NewWithOptions(&recordingMethodCaller{}, acceleratorBoundaryTestAssets, options)
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"/", "/index.html", "/assets/app.js", "/extensionless", "/missing"} {
		if got := requestBoundary(server.Handler(), http.MethodGet, target, "localhost", "").Code; got != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", target, got)
		}
	}
}

func TestAcceleratorBoundaryValidatesHost(t *testing.T) {
	caller := &recordingMethodCaller{}
	var guardCalls atomic.Int32
	server := newAcceleratorTestServer(t, caller, nil, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			guardCalls.Add(1)
			next.ServeHTTP(w, r)
		})
	})
	for _, host := range []string{"localhost", "LOCALHOST:8080", "localhost:01", "127.0.0.1", "127.42.0.9:0", "[::1]", "[::1]:65535"} {
		if got := requestBoundary(server.Handler(), http.MethodGet, "/livez", host, "").Code; got != http.StatusOK {
			t.Errorf("Host %q status = %d", host, got)
		}
	}
	for _, host := range []string{"", "example.com", "192.0.2.1", "http://localhost", "user@localhost", "localhost/path", "::1", "[::1", "[127.0.0.1]", "[127.0.0.1]:8080", "localhost:", "localhost:65536"} {
		t.Run(host, func(t *testing.T) {
			if got := requestBoundary(server.Handler(), http.MethodPost, "/api/call", host, `{"method":"M"}`).Code; got == http.StatusOK {
				t.Fatal("unsafe Host accepted")
			}
		})
	}
	if caller.callCount() != 0 || guardCalls.Load() != 0 {
		t.Fatalf("rejected requests reached downstream: caller=%d guard=%d", caller.callCount(), guardCalls.Load())
	}
}

func TestAcceleratorBoundaryValidatesOrigin(t *testing.T) {
	server := newAcceleratorTestServer(t, &recordingMethodCaller{}, nil, func(next http.Handler) http.Handler { return next })
	tests := []struct {
		name    string
		host    string
		origins []string
		want    int
	}{
		{name: "absent", host: "localhost", want: 200},
		{name: "same", host: "localhost:8080", origins: []string{"http://localhost:8080"}, want: 200},
		{name: "same ipv4", host: "127.0.0.2", origins: []string{"http://127.0.0.2"}, want: 200},
		{name: "same ipv6", host: "[::1]:8080", origins: []string{"http://[::1]:8080"}, want: 200},
		{name: "bracketed ipv4", host: "127.0.0.1", origins: []string{"http://[127.0.0.1]"}, want: 400},
		{name: "bracketed ipv4 port", host: "127.0.0.1:8080", origins: []string{"http://[127.0.0.1]:8080"}, want: 400},
		{name: "null", host: "localhost", origins: []string{"null"}, want: 400},
		{name: "https", host: "localhost", origins: []string{"https://localhost"}, want: 403},
		{name: "foreign host", host: "localhost", origins: []string{"http://127.0.0.1"}, want: 403},
		{name: "foreign port", host: "localhost", origins: []string{"http://localhost:80"}, want: 403},
		{name: "credentials", host: "localhost", origins: []string{"http://u@localhost"}, want: 400},
		{name: "path", host: "localhost", origins: []string{"http://localhost/"}, want: 400},
		{name: "query", host: "localhost", origins: []string{"http://localhost?q=1"}, want: 400},
		{name: "fragment", host: "localhost", origins: []string{"http://localhost#x"}, want: 400},
		{name: "comma", host: "localhost", origins: []string{"http://localhost,http://localhost"}, want: 400},
		{name: "multiple", host: "localhost", origins: []string{"http://localhost", "http://localhost"}, want: 400},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/livez", nil)
			request.Host = test.host
			for _, origin := range test.origins {
				request.Header.Add("Origin", origin)
			}
			response := httptest.NewRecorder()
			server.Handler().ServeHTTP(response, request)
			if response.Code != test.want {
				t.Fatalf("status = %d, want %d", response.Code, test.want)
			}
			for name := range response.Header() {
				if strings.HasPrefix(name, "Access-Control-Allow-") {
					t.Fatalf("unexpected CORS header %s", name)
				}
			}
		})
	}
	response := requestBoundary(server.Handler(), http.MethodOptions, "/api/call", "localhost", "")
	if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != http.MethodPost {
		t.Fatalf("OPTIONS status/header = %d %q", response.Code, response.Header().Get("Allow"))
	}
}

func TestAcceleratorBoundaryRejectsRawOriginFragmentDelimiter(t *testing.T) {
	caller := &recordingMethodCaller{}
	var guardCalls atomic.Int32
	server := newAcceleratorTestServer(t, caller, nil, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			guardCalls.Add(1)
			next.ServeHTTP(w, r)
		})
	})
	for _, test := range []struct{ host, origin string }{
		{host: "localhost", origin: "http://localhost#"},
		{host: "localhost:8080", origin: "http://localhost:8080#"},
	} {
		request := httptest.NewRequest(http.MethodPost, "/api/call", strings.NewReader(`{"method":"Example"}`))
		request.Host = test.host
		request.Header.Set("Origin", test.origin)
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest {
			t.Errorf("Host %q Origin %q status = %d, want %d", test.host, test.origin, response.Code, http.StatusBadRequest)
		}
	}
	if caller.callCount() != 0 || guardCalls.Load() != 0 {
		t.Fatalf("rejected origins reached downstream: caller=%d guard=%d", caller.callCount(), guardCalls.Load())
	}
}

func TestAcceleratorCanonicalRPCBoundary(t *testing.T) {
	caller := &recordingMethodCaller{}
	var guardCalls atomic.Int32
	server := newAcceleratorTestServer(t, caller, nil, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { guardCalls.Add(1); next.ServeHTTP(w, r) })
	})
	response := requestBoundary(server.Handler(), http.MethodPost, "/api/call", "localhost", `{"method":"Example","args":["value"]}`)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"data":"ok"`) {
		t.Fatalf("canonical response = %d %s", response.Code, response.Body.String())
	}
	if !caller.context.IsAuthenticated() || caller.method != "Example" || len(caller.args) != 1 || guardCalls.Load() != 1 {
		t.Fatalf("call = %#v %q %#v guard=%d", caller.context, caller.method, caller.args, guardCalls.Load())
	}

	before := caller.callCount()
	for _, test := range []struct{ method, path string }{
		{http.MethodGet, "/api/call"}, {http.MethodPut, "/api/call"}, {http.MethodPatch, "/api/call"},
		{http.MethodDelete, "/api/call"}, {http.MethodPost, "/api/Foo"}, {http.MethodPost, "/api/"},
		{http.MethodPost, "/api/call/"}, {http.MethodPost, "/api/%63all"},
	} {
		if got := requestBoundary(server.Handler(), test.method, test.path, "localhost", `{"method":"Example"}`).Code; got < 400 {
			t.Errorf("%s %s status = %d", test.method, test.path, got)
		}
	}
	if caller.callCount() != before || guardCalls.Load() != 1 {
		t.Fatalf("invalid routes reached downstream: calls=%d guard=%d", caller.callCount(), guardCalls.Load())
	}

	denied := newAcceleratorTestServer(t, caller, nil, DenyProtectedRoutes)
	response = requestBoundary(denied.Handler(), http.MethodPost, "/api/call", "localhost", `{"method":"Example"}`)
	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), "unauthorized") || caller.callCount() != before {
		t.Fatalf("deny response = %d %s calls=%d", response.Code, response.Body.String(), caller.callCount())
	}
}

func TestAcceleratorCanonicalRPCBodyValidation(t *testing.T) {
	caller := &recordingMethodCaller{}
	server := newAcceleratorTestServer(t, caller, nil, func(next http.Handler) http.Handler { return next })
	prefix, suffix := `{"method":"Example","args":["`, `"]}`
	atLimit := prefix + strings.Repeat("a", int(AcceleratorMaxRequestBodyBytes)-len(prefix)-len(suffix)) + suffix
	if len(atLimit) != int(AcceleratorMaxRequestBodyBytes) {
		t.Fatal("bad exact-limit fixture")
	}
	if got := requestBoundary(server.Handler(), http.MethodPost, "/api/call", "localhost", atLimit).Code; got != http.StatusOK {
		t.Fatalf("at-limit status = %d", got)
	}
	validCalls := caller.callCount()
	for _, test := range []struct {
		name string
		body string
		want int
	}{
		{name: "over limit", body: atLimit + " ", want: http.StatusRequestEntityTooLarge},
		{name: "empty", body: "", want: http.StatusBadRequest},
		{name: "malformed", body: "{", want: http.StatusBadRequest},
		{name: "trailing", body: `{"method":"Example"}{}`, want: http.StatusBadRequest},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := requestBoundary(server.Handler(), http.MethodPost, "/api/call", "localhost", test.body)
			if response.Code != test.want {
				t.Fatalf("status = %d, want %d; %s", response.Code, test.want, response.Body.String())
			}
			if strings.Contains(response.Body.String(), "unexpected") || strings.Contains(response.Body.String(), "invalid character") {
				t.Fatalf("decoder detail leaked: %s", response.Body.String())
			}
		})
	}
	if caller.callCount() != validCalls {
		t.Fatalf("invalid bodies invoked caller: %d -> %d", validCalls, caller.callCount())
	}
}

func TestHealthHandlers(t *testing.T) {
	deadlineSeen := make(chan time.Time, 1)
	server := newAcceleratorTestServer(t, &recordingMethodCaller{}, ReadinessFunc(func(ctx context.Context) error {
		deadline, ok := ctx.Deadline()
		if !ok {
			return errors.New("missing deadline")
		}
		deadlineSeen <- deadline
		return nil
	}), DenyProtectedRoutes)
	for path, body := range map[string]string{"/livez": "live\n", "/readyz": "ready\n"} {
		response := requestBoundary(server.Handler(), http.MethodGet, path, "localhost", "")
		if response.Code != 200 || response.Body.String() != body || response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("Content-Type") != "text/plain; charset=utf-8" {
			t.Fatalf("%s response = %d %q %#v", path, response.Code, response.Body.String(), response.Header())
		}
	}
	if deadline := <-deadlineSeen; time.Until(deadline) > 2*time.Second {
		t.Fatalf("readiness deadline too late: %v", deadline)
	}

	failing := newAcceleratorTestServer(t, &recordingMethodCaller{}, ReadinessFunc(func(context.Context) error { return errors.New("private detail") }), DenyProtectedRoutes)
	response := requestBoundary(failing.Handler(), http.MethodGet, "/readyz", "localhost", "")
	if response.Code != 503 || response.Body.String() != "not ready\n" || strings.Contains(response.Body.String(), "private") {
		t.Fatalf("failure response = %d %q", response.Code, response.Body.String())
	}

	blocked := newAcceleratorTestServer(t, &recordingMethodCaller{}, ReadinessFunc(func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }), DenyProtectedRoutes)
	request := httptest.NewRequest(http.MethodGet, "/readyz", nil).WithContext(func() context.Context {
		ctx, _ := context.WithTimeout(context.Background(), 20*time.Millisecond)
		return ctx
	}())
	request.Host = "localhost"
	response = httptest.NewRecorder()
	blocked.Handler().ServeHTTP(response, request)
	if response.Code != 503 {
		t.Fatalf("blocked status = %d", response.Code)
	}

	server.Quiesce()
	if response := requestBoundary(server.Handler(), http.MethodGet, "/livez", "localhost", ""); response.Code != 200 {
		t.Fatalf("quiesced live status = %d", response.Code)
	}
	if response := requestBoundary(server.Handler(), http.MethodGet, "/readyz", "localhost", ""); response.Code != 503 {
		t.Fatalf("quiesced ready status = %d", response.Code)
	}
	if response := requestBoundary(server.Handler(), http.MethodPost, "/api/call", "localhost", `{"method":"M"}`); response.Code != 503 {
		t.Fatalf("quiesced RPC status = %d", response.Code)
	}
	if response := requestBoundary(server.Handler(), http.MethodPost, "/livez", "localhost", ""); response.Code != 405 {
		t.Fatalf("wrong health method status = %d", response.Code)
	}
}

func TestHealthHandlerQuiesceWinsInFlightReadiness(t *testing.T) {
	providerEntered := make(chan struct{})
	releaseProvider := make(chan struct{})
	server := newAcceleratorTestServer(t, &recordingMethodCaller{}, ReadinessFunc(func(ctx context.Context) error {
		close(providerEntered)
		select {
		case <-releaseProvider:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}), DenyProtectedRoutes)

	responseReady := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		responseReady <- requestBoundary(server.Handler(), http.MethodGet, "/readyz", "localhost", "")
	}()

	select {
	case <-providerEntered:
	case <-time.After(time.Second):
		t.Fatal("readiness provider was not called")
	}
	server.Quiesce()
	close(releaseProvider)

	select {
	case response := <-responseReady:
		if response.Code != http.StatusServiceUnavailable || response.Body.String() != "not ready\n" {
			t.Fatalf("quiesced in-flight readiness response = %d %q", response.Code, response.Body.String())
		}
	case <-time.After(time.Second):
		t.Fatal("readiness request did not complete")
	}
}

func TestCompatibilityBoundaryRegression(t *testing.T) {
	caller := &recordingMethodCaller{}
	server, err := NewWithOptions(caller, embed.FS{}, CompatibilityOptions(0, nil))
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ method, path, body string }{
		{http.MethodPost, "/api/call", `{"method":"Canonical"}`},
		{http.MethodPost, "/api/Alternate", `[]`},
	} {
		response := requestBoundary(server.Handler(), test.method, test.path, "example.com", test.body)
		if response.Code != 200 || response.Header().Get("Access-Control-Allow-Origin") != "*" || !strings.Contains(response.Body.String(), `"data":"ok"`) {
			t.Fatalf("compatibility %s response = %d %s", test.path, response.Code, response.Body.String())
		}
	}
	response := requestBoundary(server.Handler(), http.MethodOptions, "/api/call", "example.com", "")
	if response.Code != 200 || response.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("compatibility OPTIONS = %d %#v", response.Code, response.Header())
	}
	response = requestBoundary(server.Handler(), http.MethodGet, "/ws", "example.com", "")
	if response.Code == http.StatusNotFound {
		t.Fatal("compatibility /ws route absent")
	}
	large := `[` + `"` + strings.Repeat("a", 2<<20) + `"` + `]`
	response = requestBoundary(server.Handler(), http.MethodPost, "/api/Large", "example.com", large)
	if response.Code != 200 {
		t.Fatalf("compatibility body over 1 MiB rejected: %d", response.Code)
	}

	accelerator := newAcceleratorTestServer(t, caller, nil, DenyProtectedRoutes)
	for _, path := range []string{"/api/Alternate", "/ws"} {
		if response := requestBoundary(accelerator.Handler(), http.MethodPost, path, "localhost", ""); response.Code != http.StatusNotFound {
			t.Errorf("Accelerator %s status = %d", path, response.Code)
		}
	}
}

type eventBody struct {
	events *[]string
	body   *strings.Reader
	seen   bool
}

func (b *eventBody) Read(p []byte) (int, error) {
	if !b.seen {
		*b.events = append(*b.events, "decode")
		b.seen = true
	}
	return b.body.Read(p)
}

func (*eventBody) Close() error { return nil }

type eventMethodCaller struct {
	events  *[]string
	context agent.AuthenticatedCallContext
	method  string
	args    []json.RawMessage
	err     error
	calls   int
}

func (c *eventMethodCaller) CallMethod(call agent.AuthenticatedCallContext, method string, args []json.RawMessage) (interface{}, error) {
	*c.events = append(*c.events, "caller")
	c.context, c.method, c.args = call, method, args
	c.calls++
	return "ok", c.err
}

func TestAuthenticatedCanonicalRPCOrderingAndErrors(t *testing.T) {
	trusted := agent.AuthenticatedCallContext{PrincipalID: "creator-server-owned", SessionID: "http-server-owned"}
	events := []string{}
	caller := &eventMethodCaller{events: &events}
	options := AcceleratorOptions(0, nil, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			events = append(events, "auth")
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), creatorContextKey{}, trusted)))
		})
	})
	options.MethodAuthorizer = MethodAuthorizerFunc(func(call agent.AuthenticatedCallContext, method string) bool {
		events = append(events, "authorizer")
		return call == trusted && method == "GetSecretData"
	})
	srv, err := NewWithOptions(caller, embed.FS{}, options)
	if err != nil {
		t.Fatal(err)
	}
	body := &eventBody{events: &events, body: strings.NewReader(`{"method":"GetSecretData","args":[{"PrincipalID":"forged","SessionID":"forged"}]}`)}
	request := httptest.NewRequest(http.MethodPost, "/api/call", nil)
	request.Host = "localhost"
	request.Body = body
	response := httptest.NewRecorder()
	srv.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || strings.Join(events, ",") != "auth,decode,authorizer,caller" {
		t.Fatalf("response/events = %d %q / %#v", response.Code, response.Body.String(), events)
	}
	if caller.context != trusted || caller.method != "GetSecretData" || len(caller.args) != 1 || strings.Contains(string(caller.args[0]), string(trusted.PrincipalID)) {
		t.Fatalf("caller identity/args = %#v %q %#v", caller.context, caller.method, caller.args)
	}

	token, err := ParseCreatorToken(creatorTestToken)
	if err != nil {
		t.Fatal(err)
	}
	auth, err := NewCreatorAuthenticator(DeriveCreatorVerifier(token).Encoded())
	if err != nil {
		t.Fatal(err)
	}
	invalidCaller := &recordingMethodCaller{}
	invalidOptions := AcceleratorOptions(0, nil, auth.Guard)
	invalidOptions.MethodAuthorizer = MethodAuthorizerFunc(func(agent.AuthenticatedCallContext, string) bool {
		t.Fatal("invalid auth reached authorizer")
		return false
	})
	invalidServer, err := NewWithOptions(invalidCaller, embed.FS{}, invalidOptions)
	if err != nil {
		t.Fatal(err)
	}
	recording := &recordingBody{data: strings.NewReader(`{"method":"GetSecretData"}`)}
	request = httptest.NewRequest(http.MethodPost, "/api/call", nil)
	request.Host = "localhost"
	request.Body = recording
	response = httptest.NewRecorder()
	invalidServer.Handler().ServeHTTP(response, request)
	assertUnauthorizedResponse(t, response)
	if recording.reads != 0 || invalidCaller.callCount() != 0 {
		t.Fatalf("invalid auth read body or dispatched: reads=%d calls=%d", recording.reads, invalidCaller.callCount())
	}

	for _, partial := range []agent.AuthenticatedCallContext{{PrincipalID: "creator-only"}, {SessionID: "session-only"}, {}} {
		partialOptions := AcceleratorOptions(0, nil, func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), creatorContextKey{}, partial)))
			})
		})
		partialOptions.MethodAuthorizer = MethodAuthorizerFunc(func(agent.AuthenticatedCallContext, string) bool {
			t.Fatal("partial auth reached authorizer")
			return true
		})
		partialServer, err := NewWithOptions(&recordingMethodCaller{}, embed.FS{}, partialOptions)
		if err != nil {
			t.Fatal(err)
		}
		partialBody := &recordingBody{data: strings.NewReader(`{"method":"GetSecretData"}`)}
		request = httptest.NewRequest(http.MethodPost, "/api/call", nil)
		request.Host = "localhost"
		request.Body = partialBody
		response = httptest.NewRecorder()
		partialServer.Handler().ServeHTTP(response, request)
		assertUnauthorizedResponse(t, response)
		if partialBody.reads != 0 {
			t.Fatalf("partial context %#v caused %d body reads", partial, partialBody.reads)
		}
	}

	nilCaller := &recordingMethodCaller{}
	nilOptions := AcceleratorOptions(0, nil, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), creatorContextKey{}, trusted)))
		})
	})
	nilServer, err := NewWithOptions(nilCaller, embed.FS{}, nilOptions)
	if err != nil {
		t.Fatal(err)
	}
	nilBody := &recordingBody{data: strings.NewReader(`{"method":"GetSecretData"}`)}
	request = httptest.NewRequest(http.MethodPost, "/api/call", nil)
	request.Host = "localhost"
	request.Body = nilBody
	response = httptest.NewRecorder()
	nilServer.Handler().ServeHTTP(response, request)
	assertAcceleratorErrorResponse(t, response, http.StatusForbidden, "forbidden")
	if nilBody.reads == 0 || nilCaller.callCount() != 0 {
		t.Fatalf("nil authorizer ordering/dispatch = reads=%d calls=%d", nilBody.reads, nilCaller.callCount())
	}

	deniedEvents := []string{}
	deniedCaller := &eventMethodCaller{events: &deniedEvents}
	deniedOptions := AcceleratorOptions(0, nil, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), creatorContextKey{}, trusted)))
		})
	})
	deniedOptions.MethodAuthorizer = MethodAuthorizerFunc(func(agent.AuthenticatedCallContext, string) bool {
		deniedEvents = append(deniedEvents, "authorizer")
		return false
	})
	deniedServer, err := NewWithOptions(deniedCaller, embed.FS{}, deniedOptions)
	if err != nil {
		t.Fatal(err)
	}
	response = requestBoundary(deniedServer.Handler(), http.MethodPost, "/api/call", "localhost", `{"method":"GetSecretData"}`)
	assertAcceleratorErrorResponse(t, response, http.StatusForbidden, "forbidden")
	if deniedCaller.calls != 0 || strings.Join(deniedEvents, ",") != "authorizer" {
		t.Fatalf("denied method events/calls = %#v / %d", deniedEvents, deniedCaller.calls)
	}

	caller.err = errors.New("underlying token verifier GetSecretData secrets.detail creator-server-owned")
	events = events[:0]
	body = &eventBody{events: &events, body: strings.NewReader(`{"method":"GetSecretData"}`)}
	request = httptest.NewRequest(http.MethodPost, "/api/call", nil)
	request.Host = "localhost"
	request.Body = body
	response = httptest.NewRecorder()
	srv.Handler().ServeHTTP(response, request)
	assertAcceleratorErrorResponse(t, response, http.StatusInternalServerError, "internal error")
	if strings.Join(events, ",") != "auth,decode,authorizer,caller" {
		t.Fatalf("error ordering = %#v", events)
	}
}

func TestAcceleratorHTTPRedactionCorpus(t *testing.T) {
	token, err := ParseCreatorToken(creatorTestToken)
	if err != nil {
		t.Fatal(err)
	}
	verifier := DeriveCreatorVerifier(token)
	auth, err := NewCreatorAuthenticator(verifier.Encoded())
	if err != nil {
		t.Fatal(err)
	}
	const (
		instanceID       = "accel-DISTINCTIVE-INSTANCE"
		deniedMethod     = "DistinctiveDeniedMethod"
		underlyingDetail = "DISTINCTIVE_CALLER_ERROR"
		resolverRawError = "DISTINCTIVE_RESOLVER_RAW_ERROR"
	)
	resolution := agent.CapabilityResolution{
		Capabilities: []agent.Capability{agent.CapabilitySecretsDetail},
		Diagnostics: []agent.CapabilityDiagnostic{
			{Capability: "unknown/filtered", Action: "unknown/filtered", Outcome: agent.CapabilityCheckOutcomeDenied, Reason: resolverRawError},
			{Capability: agent.CapabilitySecretsWatch, Action: agent.ResourceActionCoreV1SecretsWatch, Outcome: agent.CapabilityCheckOutcomeDenied, Reason: "safe bounded denial reason"},
		},
	}
	caller := &recordingMethodCaller{err: errors.New(underlyingDetail + " " + creatorTestToken)}
	options := AcceleratorOptions(0, nil, auth.Guard)
	options.AcceleratorInfoProvider = NewAuthenticatedAcceleratorInfo(agent.BuildIdentity{BuildVersion: "v-sensitive-build", Commit: "sensitive-looking-commit", Dirty: true}, instanceID, resolution)
	options.MethodAuthorizer = MethodAuthorizerFunc(func(_ agent.AuthenticatedCallContext, method string) bool { return method != deniedMethod })
	srv, err := NewWithOptions(caller, embed.FS{}, options)
	if err != nil {
		t.Fatal(err)
	}

	var capturedLog bytes.Buffer
	previousWriter := log.Writer()
	previousFlags := log.Flags()
	previousPrefix := log.Prefix()
	log.SetOutput(&capturedLog)
	log.SetFlags(0)
	log.SetPrefix("")
	t.Cleanup(func() {
		log.SetOutput(previousWriter)
		log.SetFlags(previousFlags)
		log.SetPrefix(previousPrefix)
	})

	request := func(method, path, body, authorization string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Host = "localhost"
		if authorization != "" {
			r.Header.Set("Authorization", authorization)
		}
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, r)
		return w
	}
	authorization := "Bearer " + creatorTestToken
	badRequest := request(http.MethodPost, "/api/call", "{DISTINCTIVE_BAD_JSON", authorization)
	unauthorized := request(http.MethodPost, "/api/call", `{"method":"GetSecretData"}`, "Bearer DISTINCTIVE_INVALID_AUTHORIZATION")
	forbidden := request(http.MethodPost, "/api/call", `{"method":"`+deniedMethod+`"}`, authorization)
	internal := request(http.MethodPost, "/api/call", `{"method":"GetSecretData"}`, authorization)
	info := request(http.MethodGet, "/api/accelerator-info", "", authorization)
	assertAcceleratorErrorResponse(t, badRequest, http.StatusBadRequest, "invalid request")
	assertUnauthorizedResponse(t, unauthorized)
	assertAcceleratorErrorResponse(t, forbidden, http.StatusForbidden, "forbidden")
	assertAcceleratorErrorResponse(t, internal, http.StatusInternalServerError, "internal error")
	if info.Code != http.StatusOK || !strings.Contains(info.Body.String(), "safe bounded denial reason") {
		t.Fatalf("info response = %d %q", info.Code, info.Body.String())
	}

	invalidTokenInput := "DISTINCTIVE_INVALID_TOKEN_INPUT_1234567890AB"
	_, tokenErr := ParseCreatorToken(invalidTokenInput)
	invalidVerifierInput := "DISTINCTIVE_INVALID_VERIFIER_INPUT_123456"
	_, verifierErr := ParseCreatorVerifier(invalidVerifierInput)
	constructionText := fmt.Sprintf("%v|%v", tokenErr, verifierErr)
	allOutput := badRequest.Body.String() + unauthorized.Body.String() + forbidden.Body.String() + internal.Body.String() + info.Body.String() + capturedLog.String() + constructionText + fmt.Sprintf("%v|%+v|%#v|%s|%q|%x", token, verifier, token, verifier, token, verifier)
	for _, secret := range []string{creatorTestToken, verifier.Encoded(), string(auth.context.PrincipalID), string(auth.context.SessionID), invalidTokenInput, invalidVerifierInput, underlyingDetail, resolverRawError, "DISTINCTIVE_INVALID_AUTHORIZATION"} {
		if strings.Contains(allOutput, secret) {
			t.Fatalf("redaction corpus exposed %q in %q", secret, allOutput)
		}
	}
	failureOutput := badRequest.Body.String() + unauthorized.Body.String() + forbidden.Body.String() + internal.Body.String()
	for _, detail := range []string{deniedMethod, "GetSecretData", string(agent.CapabilitySecretsDetail), "v-sensitive-build", "sensitive-looking-commit", underlyingDetail} {
		if strings.Contains(failureOutput, detail) {
			t.Fatalf("failure corpus exposed %q in %q", detail, failureOutput)
		}
	}
}

func TestAcceleratorTypedNilAdaptersFailClosed(t *testing.T) {
	guardCalls := 0
	options := AcceleratorOptions(0, nil, countingProtectedRouteGuard(&guardCalls, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			trusted := agent.AuthenticatedCallContext{PrincipalID: "creator", SessionID: "http"}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), creatorContextKey{}, trusted)))
		})
	}))
	options.AcceleratorInfoProvider = AcceleratorInfoProviderFunc(nil)
	options.MethodAuthorizer = MethodAuthorizerFunc(nil)
	srv, err := NewWithOptions(&recordingMethodCaller{}, embed.FS{}, options)
	if err != nil {
		t.Fatal(err)
	}

	info := requestBoundary(srv.Handler(), http.MethodGet, "/api/accelerator-info", "localhost", "")
	if info.Code != http.StatusNotFound || guardCalls != 0 {
		t.Fatalf("typed-nil info provider status = %d guard calls = %d, want 404 and zero", info.Code, guardCalls)
	}
	call := requestBoundary(srv.Handler(), http.MethodPost, "/api/call", "localhost", `{"method":"ListSecretsMetadata"}`)
	assertAcceleratorErrorResponse(t, call, http.StatusForbidden, "forbidden")
}

func TestDenyProtectedRoutesNoStore(t *testing.T) {
	response := httptest.NewRecorder()
	DenyProtectedRoutes(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("deny guard called next handler")
	})).ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/call", nil))
	assertUnauthorizedResponse(t, response)
}

func assertAcceleratorErrorResponse(t *testing.T, response *httptest.ResponseRecorder, status int, message string) {
	t.Helper()
	wantBody := "{\"error\":\"" + message + "\"}\n"
	if response.Code != status || response.Body.String() != wantBody || response.Header().Get("Content-Type") != "application/json" || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("error response = %d %q %#v, want %d %q", response.Code, response.Body.String(), response.Header(), status, wantBody)
	}
}
