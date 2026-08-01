package main

import (
	"bytes"
	"context"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"kubikles/pkg/agent"
	"kubikles/pkg/debug"
	"kubikles/pkg/events"
	"kubikles/pkg/k8s"
	"kubikles/pkg/server"
)

type capturedServerModeServer struct {
	mu       sync.Mutex
	options  []server.Options
	handlers []http.Handler
	callers  []server.MethodCaller
}

func (s *capturedServerModeServer) newServer(caller server.MethodCaller, assets embed.FS, options server.Options) (serverModeServer, error) {
	constructed, err := server.NewWithOptions(caller, assets, options)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.options = append(s.options, options)
	s.handlers = append(s.handlers, constructed.Handler())
	s.callers = append(s.callers, caller)
	s.mu.Unlock()
	return s, nil
}

func (s *capturedServerModeServer) caller(t *testing.T) server.MethodCaller {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.callers) != 1 {
		t.Fatalf("captured callers = %d", len(s.callers))
	}
	return s.callers[0]
}

func (*capturedServerModeServer) EmitEvent(string, interface{})                   {}
func (*capturedServerModeServer) AddDisconnectListener(server.DisconnectListener) {}
func (*capturedServerModeServer) Run(context.Context) error                       { return nil }

func (s *capturedServerModeServer) option(t *testing.T) server.Options {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.options) != 1 {
		t.Fatalf("server construction calls = %d", len(s.options))
	}
	return s.options[0]
}

func (s *capturedServerModeServer) handler(t *testing.T) http.Handler {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.handlers) != 1 {
		t.Fatalf("captured handlers = %d", len(s.handlers))
	}
	return s.handlers[0]
}

func TestRunServerWithOptionsReturnsAcceleratorConstructionErrorBeforeListen(t *testing.T) {
	t.Setenv("KUBIKLES_ACCELERATOR_CREATOR_VERIFIER", testCreatorVerifier(t))
	want := errors.New("Accelerator client unavailable")
	err := RunServerWithOptions(context.Background(), embed.FS{}, 0, "test", AppOptions{
		Mode: RuntimeModeAccelerator,
		KubernetesClientFactory: func() (*k8s.Client, error) {
			return nil, want
		},
	})
	if !errors.Is(err, ErrAcceleratorClientInitialization) || !errors.Is(err, want) {
		t.Fatalf("RunServerWithOptions() error = %v, want wrapped construction error", err)
	}
}

func TestServerOptionsForRuntime(t *testing.T) {
	readiness := server.ReadinessFunc(func(context.Context) error { return nil })
	for _, mode := range []RuntimeMode{RuntimeModeDesktop, RuntimeModeServer} {
		options, err := serverOptionsForRuntime(mode, 8080, readiness)
		if err != nil || options.BoundaryMode != server.BoundaryModeCompatibility || options.ListenAddress != ":8080" || options.ReadinessProvider == nil || options.ProtectedRouteGuard != nil {
			t.Fatalf("compatibility options for %q = %#v, %v", mode, options, err)
		}
	}
	options, err := serverOptionsForRuntime(RuntimeModeAccelerator, 8080, readiness)
	if err != nil || options.BoundaryMode != server.BoundaryModeAccelerator || options.ListenAddress != "127.0.0.1:8080" || options.ReadinessProvider == nil || options.ProtectedRouteGuard == nil {
		t.Fatalf("Accelerator options = %#v, %v", options, err)
	}
	if _, err := serverOptionsForRuntime(RuntimeMode("unknown"), 8080, readiness); !errors.Is(err, ErrInvalidRuntimeMode) {
		t.Fatalf("unknown mode error = %v", err)
	}
}

type recordingRuntimeLifecycle struct {
	quiesced bool
	stopped  bool
	closed   bool
}

func (l *recordingRuntimeLifecycle) Quiesce(context.Context)       { l.quiesced = true }
func (l *recordingRuntimeLifecycle) StopProducers(context.Context) { l.stopped = true }
func (l *recordingRuntimeLifecycle) Close(context.Context)         { l.closed = true }

func TestRunServerWithOptionsPropagatesBindErrorAndShutsDown(t *testing.T) {
	t.Setenv("KUBIKLES_ACCELERATOR_CREATOR_VERIFIER", testCreatorVerifier(t))
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	client, err := k8s.NewClientForRESTConfig(&rest.Config{Host: "http://127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	lifecycle := &recordingRuntimeLifecycle{}
	err = RunServerWithOptions(context.Background(), assets, port, "test", AppOptions{
		Mode:                    RuntimeModeAccelerator,
		KubernetesClientFactory: func() (*k8s.Client, error) { return client, nil },
		Lifecycle:               lifecycle,
	})
	if err == nil {
		t.Fatal("expected occupied-port bind error")
	}
	if !lifecycle.quiesced || !lifecycle.stopped || !lifecycle.closed {
		t.Fatalf("shutdown phases = %#v", lifecycle)
	}
}

func testCreatorVerifier(t *testing.T) string {
	t.Helper()
	token, err := server.ParseCreatorToken("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
	if err != nil {
		t.Fatal(err)
	}
	return server.DeriveCreatorVerifier(token).Encoded()
}

type recordingCapabilityResolver struct {
	calls      int
	deadline   time.Time
	resolution agent.CapabilityResolution
}

func (r *recordingCapabilityResolver) ResolveCapabilities(ctx context.Context) agent.CapabilityResolution {
	r.calls++
	r.deadline, _ = ctx.Deadline()
	return r.resolution
}

func TestAcceleratorCreatorProtectionStartup(t *testing.T) {
	for _, test := range []struct {
		name     string
		verifier string
		want     error
		identity func(string) (*server.CreatorAuthenticator, string, error)
	}{
		{name: "missing verifier", verifier: "", want: server.ErrInvalidCreatorVerifier},
		{name: "malformed verifier", verifier: "DISTINCTIVE_MALFORMED_VERIFIER", want: server.ErrInvalidCreatorVerifier},
		{name: "identity entropy error", verifier: testCreatorVerifier(t), want: errors.New("identity entropy unavailable"), identity: func(string) (*server.CreatorAuthenticator, string, error) {
			return nil, "", errors.New("identity entropy unavailable")
		}},
	} {
		t.Run(test.name+" before app client server and listen", func(t *testing.T) {
			dependencies := productionServerModeDependencies()
			lookupCalls := 0
			dependencies.lookupEnv = func(name string) string {
				lookupCalls++
				if name != "KUBIKLES_ACCELERATOR_CREATOR_VERIFIER" {
					t.Fatalf("environment key = %q", name)
				}
				return test.verifier
			}
			if test.identity != nil {
				dependencies.newCreatorIdentity = test.identity
			}
			clientCalls := 0
			err := runServerWithOptions(context.Background(), embed.FS{}, 0, "test", AppOptions{
				Mode: RuntimeModeAccelerator,
				KubernetesClientFactory: func() (*k8s.Client, error) {
					clientCalls++
					return nil, errors.New("app construction must not start")
				},
			}, dependencies)
			if test.identity == nil {
				if !errors.Is(err, test.want) {
					t.Fatalf("startup error = %v, want %v", err, test.want)
				}
			} else if err == nil || err.Error() != test.want.Error() {
				t.Fatalf("startup error = %v, want %v", err, test.want)
			}
			if lookupCalls != 1 || clientCalls != 0 {
				t.Fatalf("early startup calls = env:%d client:%d", lookupCalls, clientCalls)
			}
		})
	}

	creatorToken, err := server.ParseCreatorToken("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
	if err != nil {
		t.Fatal(err)
	}
	creator, err := server.NewCreatorAuthenticator(server.DeriveCreatorVerifier(creatorToken).Encoded())
	if err != nil {
		t.Fatal(err)
	}
	resolver := &recordingCapabilityResolver{resolution: agent.CapabilityResolution{
		Capabilities: []agent.Capability{agent.CapabilitySecretsList},
		Diagnostics: []agent.CapabilityDiagnostic{{
			Capability: agent.CapabilitySecretsList,
			Action:     agent.ResourceActionCoreV1SecretsList,
			Outcome:    agent.CapabilityCheckOutcomeAllowed,
		}},
	}}
	outerFactoryCalls := 0
	resolverFactoryCalls := 0
	newResolverFactory := func(*k8s.Client) agent.CapabilityResolverFactory {
		outerFactoryCalls++
		return func() agent.CapabilityResolver {
			resolverFactoryCalls++
			return resolver
		}
	}
	callerCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	callerDeadline, _ := callerCtx.Deadline()
	before := time.Now()
	protection, err := newAcceleratorHTTPProtectionWithDependencies(callerCtx, &App{}, creator, "accel-public", newResolverFactory, server.NewBrowserSessionManager)
	if err != nil {
		t.Fatal(err)
	}
	if outerFactoryCalls != 1 || resolverFactoryCalls != 1 || resolver.calls != 1 {
		t.Fatalf("resolver construction/calls = %d/%d/%d", outerFactoryCalls, resolverFactoryCalls, resolver.calls)
	}
	if resolver.deadline.IsZero() || resolver.deadline.After(callerDeadline) || resolver.deadline.After(before.Add(7*time.Second)) {
		t.Fatalf("resolution deadline = %v; caller=%v before+7s=%v", resolver.deadline, callerDeadline, before.Add(7*time.Second))
	}
	options := server.AcceleratorOptions(0, nil, server.DenyProtectedRoutes)
	installAcceleratorHTTPProtection(&options, protection)
	if options.ProtectedRouteGuard == nil || options.AcceleratorInfoProvider == nil || options.MethodAuthorizer == nil {
		t.Fatalf("protection not installed: %#v", options)
	}
	if !options.MethodAuthorizer.Authorize(agent.AuthenticatedCallContext{PrincipalID: "creator", SessionID: "http"}, "ListSecretsMetadata") || options.MethodAuthorizer.Authorize(agent.AuthenticatedCallContext{PrincipalID: "creator", SessionID: "http"}, "GetSecretData") {
		t.Fatal("reduced capability result was not preserved as authorization value")
	}
	info := options.AcceleratorInfoProvider.AcceleratorInfo()
	if len(info.Capabilities) != 1 || info.Capabilities[0] != agent.CapabilitySecretsList || len(info.CapabilityDiagnostics) != 1 {
		t.Fatalf("reduced info = %#v", info)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/accelerator-info", nil)
	request.Header.Set("Authorization", "Bearer AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
	response := httptest.NewRecorder()
	reached := false
	options.ProtectedRouteGuard(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { reached = true; w.WriteHeader(http.StatusNoContent) })).ServeHTTP(response, request)
	if response.Code != http.StatusNoContent || !reached {
		t.Fatalf("installed creator guard status/reached = %d/%v", response.Code, reached)
	}

	deniedResolver := &recordingCapabilityResolver{resolution: agent.CapabilityResolution{Diagnostics: []agent.CapabilityDiagnostic{{
		Capability: agent.CapabilitySecretsList,
		Action:     agent.ResourceActionCoreV1SecretsList,
		Outcome:    agent.CapabilityCheckOutcomeDenied,
		Reason:     "safe denied value",
	}}}}
	denied, err := newAcceleratorHTTPProtectionWithDependencies(context.Background(), &App{}, creator, "accel-denied", func(*k8s.Client) agent.CapabilityResolverFactory {
		return func() agent.CapabilityResolver { return deniedResolver }
	}, server.NewBrowserSessionManager)
	if err != nil {
		t.Fatalf("denied capability value failed startup: %v", err)
	}
	if deniedResolver.calls != 1 || denied.Authorizer.Authorize(agent.AuthenticatedCallContext{PrincipalID: "creator", SessionID: "http"}, "ListSecretsMetadata") || len(denied.Info.AcceleratorInfo().Capabilities) != 0 {
		t.Fatalf("denied capability was not a reduced value: calls=%d info=%#v", deniedResolver.calls, denied.Info.AcceleratorInfo())
	}
}

func TestRunServerWithOptionsInstallsAcceleratorProtectionBeforeConstruction(t *testing.T) {
	client, err := k8s.NewClientForRESTConfig(&rest.Config{Host: "http://127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	creator, err := server.NewCreatorAuthenticator(testCreatorVerifier(t))
	if err != nil {
		t.Fatal(err)
	}
	resolver := &recordingCapabilityResolver{resolution: agent.CapabilityResolution{Capabilities: []agent.Capability{agent.CapabilitySecretsList}}}
	captured := &capturedServerModeServer{}
	dependencies := productionServerModeDependencies()
	dependencies.lookupEnv = func(string) string { return "valid verifier is supplied through identity seam" }
	dependencies.newCreatorIdentity = func(string) (*server.CreatorAuthenticator, string, error) { return creator, "accel-public", nil }
	dependencies.capabilityResolverFactory = func(*k8s.Client) agent.CapabilityResolverFactory {
		return func() agent.CapabilityResolver { return resolver }
	}
	dependencies.newServer = captured.newServer
	if err := runServerWithOptions(context.Background(), assets, 0, "test", AppOptions{
		Mode: RuntimeModeAccelerator, KubernetesClientFactory: func() (*k8s.Client, error) { return client, nil },
	}, dependencies); err != nil {
		t.Fatal(err)
	}
	options := captured.option(t)
	if options.BoundaryMode != server.BoundaryModeAccelerator || options.ProtectedRouteGuard == nil || options.AcceleratorInfoProvider == nil || options.MethodAuthorizer == nil {
		t.Fatalf("accelerator construction options missing protection: %#v", options)
	}
	if resolver.calls != 1 || !options.MethodAuthorizer.Authorize(agent.AuthenticatedCallContext{PrincipalID: "creator", SessionID: "http"}, "ListSecretsMetadata") || len(options.AcceleratorInfoProvider.AcceleratorInfo().Capabilities) != 1 {
		t.Fatalf("protection did not retain one startup snapshot: calls=%d options=%#v", resolver.calls, options)
	}
	if _, ok := captured.caller(t).(*acceleratorSecretCaller); !ok {
		t.Fatalf("Accelerator caller = %T, want *acceleratorSecretCaller", captured.caller(t))
	}
	for i := 0; i < 3; i++ {
		_ = options.MethodAuthorizer.Authorize(agent.AuthenticatedCallContext{PrincipalID: "creator", SessionID: "http"}, "GetSecretData")
		_ = options.AcceleratorInfoProvider.AcceleratorInfo()
	}
	if resolver.calls != 1 {
		t.Fatalf("policy consulted resolver after startup snapshot: calls=%d", resolver.calls)
	}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/accelerator-info", nil)
	request.Host = "localhost"
	captured.handler(t).ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("constructed accelerator info route status = %d, want 401", response.Code)
	}
}

func TestAcceleratorBrowserSessionComposition(t *testing.T) {
	client, err := k8s.NewClientForRESTConfig(&rest.Config{Host: "http://127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	creator, err := server.NewCreatorAuthenticator(testCreatorVerifier(t))
	if err != nil {
		t.Fatal(err)
	}
	captured := &capturedServerModeServer{}
	dependencies := productionServerModeDependencies()
	dependencies.lookupEnv = func(string) string { return "creator identity seam" }
	dependencies.newCreatorIdentity = func(string) (*server.CreatorAuthenticator, string, error) {
		return creator, "accel-browser-composition", nil
	}
	dependencies.capabilityResolverFactory = func(*k8s.Client) agent.CapabilityResolverFactory { return nil }
	var factoryCalls int
	var composed *server.BrowserSessionManager
	dependencies.newBrowserSessions = func(revoker server.BrowserSessionRevoker) *server.BrowserSessionManager {
		factoryCalls++
		if _, ok := revoker.(server.NoopBrowserSessionRevoker); !ok {
			t.Fatalf("production revoker = %T, want NoopBrowserSessionRevoker", revoker)
		}
		composed = server.NewBrowserSessionManager(revoker)
		return composed
	}
	dependencies.newServer = captured.newServer
	if err := runServerWithOptions(context.Background(), assets, 0, "accelerator", AppOptions{
		Mode: RuntimeModeAccelerator, KubernetesClientFactory: func() (*k8s.Client, error) { return client, nil },
	}, dependencies); err != nil {
		t.Fatal(err)
	}
	options := captured.option(t)
	if factoryCalls != 1 || composed == nil || options.BrowserSessions != composed || options.ProtectedRouteGuard == nil || options.BoundaryMode != server.BoundaryModeAccelerator {
		t.Fatalf("composition factory/manager/options = %d/%p/%p/%#v", factoryCalls, composed, options.BrowserSessions, options)
	}
	handler := captured.handler(t)
	request := func(method, target, authorization, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, target, strings.NewReader(body))
		r.Host = "localhost"
		if authorization != "" {
			r.Header.Set("Authorization", authorization)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	creatorAuthorization := "Bearer AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	mint := request(http.MethodPost, "/api/accelerator-browser-ticket", creatorAuthorization, "")
	if mint.Code != http.StatusCreated {
		t.Fatalf("mint = %d %q", mint.Code, mint.Body.String())
	}
	var minted struct {
		Ticket string `json:"ticket"`
	}
	if err := json.Unmarshal(mint.Body.Bytes(), &minted); err != nil || len(minted.Ticket) != 43 {
		t.Fatalf("mint body = %q, err=%v", mint.Body.String(), err)
	}
	exchange := request(http.MethodPost, "/api/accelerator-browser-session", "", `{"ticket":"`+minted.Ticket+`"}`)
	if exchange.Code != http.StatusOK {
		t.Fatalf("exchange = %d %q", exchange.Code, exchange.Body.String())
	}
	var exchanged struct {
		Bearer string `json:"bearer"`
	}
	if err := json.Unmarshal(exchange.Body.Bytes(), &exchanged); err != nil || len(exchanged.Bearer) != 43 {
		t.Fatalf("exchange body = %q, err=%v", exchange.Body.String(), err)
	}
	browserAuthorization := "Bearer " + exchanged.Bearer
	if info := request(http.MethodGet, "/api/accelerator-info", browserAuthorization, ""); info.Code != http.StatusOK || !strings.Contains(info.Body.String(), `"instanceId":"accel-browser-composition"`) {
		t.Fatalf("browser info = %d %q", info.Code, info.Body.String())
	}
	if denied := request(http.MethodPost, "/api/accelerator-browser-ticket", browserAuthorization, ""); denied.Code != http.StatusForbidden {
		t.Fatalf("browser mint = %d %q", denied.Code, denied.Body.String())
	}
	if revoked := request(http.MethodPost, "/api/accelerator-browser-session/revoke", creatorAuthorization, ""); revoked.Code != http.StatusNoContent {
		t.Fatalf("creator revoke = %d %q", revoked.Code, revoked.Body.String())
	}
	if expired := request(http.MethodGet, "/api/accelerator-info", browserAuthorization, ""); expired.Code != http.StatusUnauthorized {
		t.Fatalf("revoked browser info = %d %q", expired.Code, expired.Body.String())
	}

	for _, mode := range []RuntimeMode{RuntimeModeDesktop, RuntimeModeServer} {
		t.Run(string(mode)+" constructs zero managers", func(t *testing.T) {
			ordinaryCapture := &capturedServerModeServer{}
			ordinaryDependencies := productionServerModeDependencies()
			ordinaryFactoryCalls := 0
			ordinaryDependencies.newBrowserSessions = func(server.BrowserSessionRevoker) *server.BrowserSessionManager {
				ordinaryFactoryCalls++
				return server.NewBrowserSessionManager(server.NoopBrowserSessionRevoker{})
			}
			ordinaryDependencies.newServer = ordinaryCapture.newServer
			if err := runServerWithOptions(context.Background(), assets, 0, "ordinary", AppOptions{
				Mode: mode, KubernetesClientFactory: func() (*k8s.Client, error) { return nil, errors.New("ordinary client unavailable") },
			}, ordinaryDependencies); err != nil {
				t.Fatal(err)
			}
			ordinaryOptions := ordinaryCapture.option(t)
			if ordinaryFactoryCalls != 0 || ordinaryOptions.BrowserSessions != nil || ordinaryOptions.ProtectedRouteGuard != nil {
				t.Fatalf("ordinary factory/options = %d/%#v", ordinaryFactoryCalls, ordinaryOptions)
			}
			for _, target := range []string{"/api/accelerator-browser-ticket", "/api/accelerator-browser-session", "/api/accelerator-browser-session/revoke"} {
				r := httptest.NewRequest(http.MethodPost, target, nil)
				w := httptest.NewRecorder()
				ordinaryCapture.handler(t).ServeHTTP(w, r)
				if w.Code != http.StatusNotFound {
					t.Fatalf("ordinary route %s = %d, want 404", target, w.Code)
				}
			}
		})
	}
}

func TestOrdinaryServerIgnoresCreatorVerifier(t *testing.T) {
	for _, mode := range []RuntimeMode{RuntimeModeDesktop, RuntimeModeServer} {
		t.Run(string(mode), func(t *testing.T) {
			lookupCalls := 0
			identityCalls := 0
			resolverFactoryCalls := 0
			dependencies := serverModeDependencies{
				lookupEnv: func(string) string { lookupCalls++; return "DISTINCTIVE_MALFORMED_VERIFIER" },
				newCreatorIdentity: func(string) (*server.CreatorAuthenticator, string, error) {
					identityCalls++
					return nil, "", errors.New("ordinary mode read creator verifier")
				},
				capabilityResolverFactory: func(*k8s.Client) agent.CapabilityResolverFactory {
					resolverFactoryCalls++
					return nil
				},
			}
			identity, err := acceleratorIdentityForRuntime(mode, dependencies)
			if err != nil || identity.creator != nil || identity.instanceID != "" {
				t.Fatalf("ordinary identity = %#v, %v", identity, err)
			}
			options, err := serverOptionsForRuntime(mode, 8080, nil)
			if err != nil || options.BoundaryMode != server.BoundaryModeCompatibility || options.ProtectedRouteGuard != nil || options.AcceleratorInfoProvider != nil || options.MethodAuthorizer != nil {
				t.Fatalf("ordinary options = %#v, %v", options, err)
			}
			if lookupCalls != 0 || identityCalls != 0 || resolverFactoryCalls != 0 {
				t.Fatalf("ordinary mode invoked accelerator dependencies: env=%d identity=%d resolver=%d", lookupCalls, identityCalls, resolverFactoryCalls)
			}
		})
	}
}

func TestRunServerWithOptionsOrdinaryModesIgnoreMalformedCreatorEnvironment(t *testing.T) {
	t.Setenv("KUBIKLES_ACCELERATOR_CREATOR_VERIFIER", "DISTINCTIVE_MALFORMED_VERIFIER")
	for _, mode := range []RuntimeMode{RuntimeModeDesktop, RuntimeModeServer} {
		t.Run(string(mode), func(t *testing.T) {
			captured := &capturedServerModeServer{}
			lookupCalls, identityCalls, resolverCalls := 0, 0, 0
			dependencies := productionServerModeDependencies()
			dependencies.lookupEnv = func(name string) string { lookupCalls++; return os.Getenv(name) }
			dependencies.newCreatorIdentity = func(string) (*server.CreatorAuthenticator, string, error) {
				identityCalls++
				return nil, "", errors.New("ordinary mode attempted creator authentication")
			}
			dependencies.capabilityResolverFactory = func(*k8s.Client) agent.CapabilityResolverFactory {
				resolverCalls++
				return nil
			}
			dependencies.newServer = captured.newServer
			if err := runServerWithOptions(context.Background(), assets, 0, "test", AppOptions{
				Mode: mode, KubernetesClientFactory: func() (*k8s.Client, error) { return nil, errors.New("ordinary client unavailable") },
			}, dependencies); err != nil {
				t.Fatal(err)
			}
			options := captured.option(t)
			if options.BoundaryMode != server.BoundaryModeCompatibility || options.ProtectedRouteGuard != nil || options.AcceleratorInfoProvider != nil || options.MethodAuthorizer != nil {
				t.Fatalf("ordinary mode installed accelerator routes/protection: %#v", options)
			}
			if lookupCalls != 0 || identityCalls != 0 || resolverCalls != 0 {
				t.Fatalf("ordinary mode consulted malformed auth environment: env=%d identity=%d resolver=%d", lookupCalls, identityCalls, resolverCalls)
			}
			if _, ok := captured.caller(t).(*AppMethodCaller); !ok {
				t.Fatalf("ordinary caller = %T, want *AppMethodCaller", captured.caller(t))
			}
			response := httptest.NewRecorder()
			captured.handler(t).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/livez", nil))
			if response.Code == http.StatusUnauthorized {
				t.Fatalf("ordinary mode live route unexpectedly installed Accelerator protection: %d", response.Code)
			}
		})
	}
}

func TestAcceleratorInfoBuildIdentityIsReportingOnly(t *testing.T) {
	previousVersion, previousCommit, previousDirty := BuildVersion, GitCommit, GitDirty
	BuildVersion = "v9.8.7-rc.1+exact"
	GitCommit = "0123456789abcdef0123456789abcdef01234567"
	GitDirty = "true"
	t.Cleanup(func() { BuildVersion, GitCommit, GitDirty = previousVersion, previousCommit, previousDirty })

	token, err := server.ParseCreatorToken("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
	if err != nil {
		t.Fatal(err)
	}
	creator, err := server.NewCreatorAuthenticator(server.DeriveCreatorVerifier(token).Encoded())
	if err != nil {
		t.Fatal(err)
	}
	resolver := &recordingCapabilityResolver{}
	protection, err := newAcceleratorHTTPProtectionWithDependencies(context.Background(), &App{}, creator, "accel-reporting", func(*k8s.Client) agent.CapabilityResolverFactory {
		return func() agent.CapabilityResolver { return resolver }
	}, server.NewBrowserSessionManager)
	if err != nil {
		t.Fatal(err)
	}
	info := protection.Info.AcceleratorInfo()
	want := (agent.BuildIdentity{BuildVersion: BuildVersion, Commit: GitCommit, Dirty: true})
	if info.Build != want {
		t.Fatalf("reported build = %#v, want %#v", info.Build, want)
	}
	compatibility := agent.CheckBuildCompatibility("v-adjacent-desktop", BuildVersion)
	if compatibility.Compatible || protection.Info.AcceleratorInfo().Build != want {
		t.Fatalf("desktop compatibility result altered server report: %#v %#v", compatibility, protection.Info.AcceleratorInfo())
	}
}

const acceleratorEvidenceCreatorToken = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

type recordingProjectionAuthorizer struct {
	mu       sync.Mutex
	delegate server.MethodAuthorizer
	contexts []agent.AuthenticatedCallContext
}

func (a *recordingProjectionAuthorizer) Authorize(call agent.AuthenticatedCallContext, method string) bool {
	a.mu.Lock()
	a.contexts = append(a.contexts, call)
	a.mu.Unlock()
	return a.delegate.Authorize(call, method)
}

func (a *recordingProjectionAuthorizer) firstContext(t *testing.T) agent.AuthenticatedCallContext {
	t.Helper()
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.contexts) == 0 {
		t.Fatal("authorizer did not receive an authenticated context")
	}
	return a.contexts[0]
}

func newProjectionCanonicalHandler(t *testing.T, client *k8s.Client, capabilities []agent.Capability) (http.Handler, *recordingProjectionAuthorizer) {
	t.Helper()
	authenticator, err := server.NewCreatorAuthenticator(testCreatorVerifier(t))
	if err != nil {
		t.Fatal(err)
	}
	policy := server.NewAcceleratorMethodAuthorizer(agent.CapabilityResolution{Capabilities: capabilities})
	recording := &recordingProjectionAuthorizer{delegate: policy}
	app := &App{k8sClient: client, listRequestManager: NewListRequestManager()}
	caller := newAcceleratorSecretCaller(NewAppMethodCaller(app), app)
	options := server.AcceleratorOptions(0, nil, authenticator.Guard)
	options.MethodAuthorizer = recording
	srv, err := server.NewWithOptions(caller, assets, options)
	if err != nil {
		t.Fatal(err)
	}
	return srv.Handler(), recording
}

func projectionCanonicalCall(handler http.Handler, authorization, method string, args ...interface{}) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]interface{}{"method": method, "args": args})
	request := httptest.NewRequest(http.MethodPost, "/api/call", strings.NewReader(string(body)))
	request.Host = "localhost"
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func TestAcceleratorSecretProjectionRequiresPolicy(t *testing.T) {
	var hitMu sync.Mutex
	var paths []string
	client := acceleratorProjectionTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitMu.Lock()
		paths = append(paths, r.URL.Path)
		hitMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/namespaces/evidence/secrets":
			table := metav1.Table{ColumnDefinitions: []metav1.TableColumnDefinition{{Name: "Name"}, {Name: "Type"}, {Name: "Data"}}, Rows: []metav1.TableRow{{Cells: []interface{}{"detail", "Opaque", float64(1)}}}}
			_ = json.NewEncoder(w).Encode(table)
		case "/api/v1/namespaces/evidence/secrets/detail":
			_ = json.NewEncoder(w).Encode(v1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "detail", Namespace: "evidence"}, Type: v1.SecretTypeOpaque, Data: map[string][]byte{"marker": []byte("POLICY_DETAIL_MARKER")}})
		default:
			http.NotFound(w, r)
		}
	}))
	allCapabilities := agent.V1Capabilities()
	firstHandler, firstAuthorizer := newProjectionCanonicalHandler(t, client, allCapabilities)
	secondHandler, secondAuthorizer := newProjectionCanonicalHandler(t, client, allCapabilities)
	authorization := "Bearer " + acceleratorEvidenceCreatorToken
	allowed := []struct {
		method string
		args   []interface{}
	}{
		{method: "ListSecretsMetadata", args: []interface{}{"", "evidence"}},
		{method: "GetSecretData", args: []interface{}{"evidence", "detail"}},
		{method: "GetSecretYaml", args: []interface{}{"evidence", "detail"}},
	}
	for _, test := range allowed {
		first := projectionCanonicalCall(firstHandler, authorization, test.method, test.args...)
		second := projectionCanonicalCall(secondHandler, authorization, test.method, test.args...)
		if first.Code != http.StatusOK || second.Code != http.StatusOK || first.Body.String() != second.Body.String() {
			t.Fatalf("%s identity-neutral responses = %d %q / %d %q", test.method, first.Code, first.Body.String(), second.Code, second.Body.String())
		}
	}
	firstContext := firstAuthorizer.firstContext(t)
	secondContext := secondAuthorizer.firstContext(t)
	if firstContext == secondContext || !firstContext.IsAuthenticated() || !secondContext.IsAuthenticated() {
		t.Fatalf("trusted contexts = %#v / %#v, want two distinct authenticated contexts", firstContext, secondContext)
	}

	hitMu.Lock()
	allowedHits := len(paths)
	hitMu.Unlock()
	if allowedHits != 6 {
		t.Fatalf("allowed facade Kubernetes hits = %d, want 6 (%#v)", allowedHits, paths)
	}
	unauthenticated := projectionCanonicalCall(firstHandler, "", "GetSecretData", "evidence", "detail")
	if unauthenticated.Code != http.StatusUnauthorized || unauthenticated.Body.String() != "{\"error\":\"unauthorized\"}\n" || unauthenticated.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("missing auth response = %d %q %#v", unauthenticated.Code, unauthenticated.Body.String(), unauthenticated.Header())
	}
	withoutCapabilities, _ := newProjectionCanonicalHandler(t, client, nil)
	for _, method := range []string{"ListSecretsMetadata", "GetSecretData", "GetSecretYaml"} {
		response := projectionCanonicalCall(withoutCapabilities, authorization, method, "evidence", "detail")
		if response.Code != http.StatusForbidden || response.Body.String() != "{\"error\":\"forbidden\"}\n" {
			t.Fatalf("missing capability %s response = %d %q", method, response.Code, response.Body.String())
		}
	}
	for _, method := range []string{"ListSecrets", "GetConfigMapData", "UpdateSecretData", "UpdateSecretYaml", "DeleteSecret", "CreateSecret"} {
		response := projectionCanonicalCall(firstHandler, authorization, method, map[string]string{"PrincipalID": "forged", "SessionID": "forged"})
		if response.Code != http.StatusForbidden || response.Body.String() != "{\"error\":\"forbidden\"}\n" || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("forbidden %s response = %d %q %#v", method, response.Code, response.Body.String(), response.Header())
		}
	}
	hitMu.Lock()
	defer hitMu.Unlock()
	if len(paths) != allowedHits {
		t.Fatalf("denied calls reached facade/client: before=%d after=%d paths=%#v", allowedHits, len(paths), paths)
	}
}

func TestSecretProjectionModeIsolation(t *testing.T) {
	client, err := k8s.NewClientForRESTConfig(&rest.Config{Host: "http://127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	ordinaryCapture := &capturedServerModeServer{}
	ordinaryDependencies := productionServerModeDependencies()
	ordinaryDependencies.newServer = ordinaryCapture.newServer
	if err := runServerWithOptions(context.Background(), assets, 0, "ordinary", AppOptions{Mode: RuntimeModeServer, KubernetesClientFactory: func() (*k8s.Client, error) { return client, nil }}, ordinaryDependencies); err != nil {
		t.Fatal(err)
	}
	ordinary := ordinaryCapture.caller(t)
	if _, ok := ordinary.(*AppMethodCaller); !ok {
		t.Fatalf("ordinary caller = %T, want raw *AppMethodCaller", ordinary)
	}
	if _, decorated := ordinary.(*acceleratorSecretCaller); decorated {
		t.Fatalf("ordinary caller unexpectedly decorated: %T", ordinary)
	}

	acceleratorCapture := &capturedServerModeServer{}
	acceleratorDependencies := productionServerModeDependencies()
	acceleratorDependencies.lookupEnv = func(string) string { return testCreatorVerifier(t) }
	acceleratorDependencies.newCreatorIdentity = func(verifier string) (*server.CreatorAuthenticator, string, error) {
		creator, creatorErr := server.NewCreatorAuthenticator(verifier)
		return creator, "accel-mode-isolation", creatorErr
	}
	acceleratorDependencies.capabilityResolverFactory = func(*k8s.Client) agent.CapabilityResolverFactory { return nil }
	acceleratorDependencies.newServer = acceleratorCapture.newServer
	if err := runServerWithOptions(context.Background(), assets, 0, "accelerator", AppOptions{Mode: RuntimeModeAccelerator, KubernetesClientFactory: func() (*k8s.Client, error) { return client, nil }}, acceleratorDependencies); err != nil {
		t.Fatal(err)
	}
	decorated, ok := acceleratorCapture.caller(t).(*acceleratorSecretCaller)
	if !ok {
		t.Fatalf("Accelerator caller = %T, want *acceleratorSecretCaller", acceleratorCapture.caller(t))
	}
	if _, ok := decorated.delegate.(*AppMethodCaller); !ok {
		t.Fatalf("Accelerator delegate = %T, want raw *AppMethodCaller", decorated.delegate)
	}
	if _, nested := decorated.delegate.(*acceleratorSecretCaller); nested {
		t.Fatalf("Accelerator projection decorated more than once: %T", decorated.delegate)
	}
}

func TestAcceleratorSecretPayloadByteBoundaries(t *testing.T) {
	const (
		listPlainMarkerA      = "LIST_PLAINTEXT_VALUE_MARKER_A"
		listPlainMarkerB      = "LIST_PLAINTEXT_VALUE_MARKER_B"
		listBinaryMarkerA     = "/wBMSVNUX0JJTkFSWV9NQVJLRVJfQQ=="
		listBinaryMarkerB     = "/gFMSVNUX0JJTkFSWV9NQVJLRVJfQg=="
		listBase64MarkerA     = "QkFTRTY0X1ZBTFVFX01BUktFUl9B"
		listBase64MarkerB     = "QkFTRTY0X1ZBTFVFX01BUktFUl9C"
		listStringDataMarkerA = "LIST_STRING_DATA_MARKER_A"
		listStringDataMarkerB = "LIST_STRING_DATA_MARKER_B"
		listLabelMarkerA      = "LIST_LABEL_MARKER_A"
		listLabelMarkerB      = "LIST_LABEL_MARKER_B"
		listAnnotationMarkerA = "LIST_ANNOTATION_MARKER_A"
		listAnnotationMarkerB = "LIST_ANNOTATION_MARKER_B"
		listLastAppliedA      = "LIST_LAST_APPLIED_MARKER_A"
		listLastAppliedB      = "LIST_LAST_APPLIED_MARKER_B"
		listManagedMarkerA    = "LIST_MANAGED_FIELDS_MARKER_A"
		listManagedMarkerB    = "LIST_MANAGED_FIELDS_MARKER_B"
		listFinalizerMarkerA  = "LIST_FINALIZER_MARKER_A"
		listFinalizerMarkerB  = "LIST_FINALIZER_MARKER_B"
		listOwnerMarkerA      = "LIST_OWNER_REFERENCE_MARKER_A"
		listOwnerMarkerB      = "LIST_OWNER_REFERENCE_MARKER_B"
		listResourceMarkerA   = "LIST_RESOURCE_VERSION_MARKER_A"
		listResourceMarkerB   = "LIST_RESOURCE_VERSION_MARKER_B"
		progressMarker        = "LIST_PROGRESS_MARKER"
		logMarker             = "LIST_LOG_MARKER"
		errorMarker           = "DISTINCTIVE_LIST_ERROR_MARKER"
		detailMarker          = "DETAIL_ALLOWED_MARKER"
	)
	creation := metav1.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC)
	makeHostileObject := func(suffix, payload, plain, binary, encoded, stringData, label, annotation, lastApplied, managed, finalizer, owner, resourceVersion string) []byte {
		t.Helper()
		raw, err := json.Marshal(map[string]interface{}{
			"apiVersion": "v1", "kind": "Secret", "type": "Opaque",
			"metadata": map[string]interface{}{
				"name": "canonical-secret", "namespace": "evidence", "uid": "canonical-uid", "creationTimestamp": creation,
				"labels": map[string]string{"forbidden-label-key-" + suffix: label},
				"annotations": map[string]string{
					"forbidden-annotation-key-" + suffix:               annotation,
					"kubectl.kubernetes.io/last-applied-configuration": lastApplied + payload,
				},
				"managedFields": []map[string]interface{}{{"manager": managed, "fieldsV1": map[string]string{"raw": payload}}},
				"finalizers":    []string{finalizer}, "ownerReferences": []map[string]string{{"name": owner}},
				"resourceVersion": resourceVersion,
			},
			"data": map[string]string{
				"forbidden-plain-key-" + suffix:  plain + payload,
				"forbidden-binary-key-" + suffix: binary,
				"forbidden-base64-key-" + suffix: encoded,
			},
			"binaryData": map[string]string{"forbidden-binary-data-key-" + suffix: binary + encoded},
			"stringData": map[string]string{"forbidden-string-key-" + suffix: stringData + payload},
			"progress":   progressMarker, "log": logMarker,
		})
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	smallPayload := strings.Repeat("SMALL_FORBIDDEN_PAYLOAD_A_", 256)
	largePayload := strings.Repeat("RADICALLY_LARGE_FORBIDDEN_PAYLOAD_B_", 32768)
	hostileObjectA := makeHostileObject("A", smallPayload, listPlainMarkerA, listBinaryMarkerA, listBase64MarkerA, listStringDataMarkerA, listLabelMarkerA, listAnnotationMarkerA, listLastAppliedA, listManagedMarkerA, listFinalizerMarkerA, listOwnerMarkerA, listResourceMarkerA)
	hostileObjectB := makeHostileObject("B", largePayload, listPlainMarkerB, listBinaryMarkerB, listBase64MarkerB, listStringDataMarkerB, listLabelMarkerB, listAnnotationMarkerB, listLastAppliedB, listManagedMarkerB, listFinalizerMarkerB, listOwnerMarkerB, listResourceMarkerB)
	makeTable := func(raw []byte) metav1.Table {
		return metav1.Table{ColumnDefinitions: []metav1.TableColumnDefinition{{Name: "Name"}, {Name: "Type"}, {Name: "Data"}}, Rows: []metav1.TableRow{
			{Cells: []interface{}{"ignored-cell-name", "Opaque", float64(3)}, Object: runtime.RawExtension{Raw: raw}},
			{Cells: []interface{}{"sh.helm.release.v1.forbidden.v1", k8s.HelmReleaseSecretType, float64(3)}, Object: runtime.RawExtension{Raw: raw}},
		}}
	}
	tableA, tableB := makeTable(hostileObjectA), makeTable(hostileObjectB)
	tableBytesA, err := json.Marshal(tableA)
	if err != nil {
		t.Fatal(err)
	}
	tableBytesB, err := json.Marshal(tableB)
	if err != nil {
		t.Fatal(err)
	}
	if len(tableBytesB) < len(tableBytesA)*50 {
		t.Fatalf("hostile fixtures are not radically different in size: A=%d B=%d", len(tableBytesA), len(tableBytesB))
	}
	for _, encoded := range []string{listBinaryMarkerA, listBinaryMarkerB, listBase64MarkerA, listBase64MarkerB} {
		if _, err := base64.StdEncoding.DecodeString(encoded); err != nil {
			t.Fatalf("fixture value %q is not base64: %v", encoded, err)
		}
	}
	fixtureEvidence := []struct {
		name    string
		raw     []byte
		markers []string
	}{
		{name: "A", raw: tableBytesA, markers: []string{"forbidden-plain-key-A", "forbidden-binary-key-A", "forbidden-base64-key-A", listPlainMarkerA, listBinaryMarkerA, listBase64MarkerA, listStringDataMarkerA, listLabelMarkerA, listAnnotationMarkerA, listLastAppliedA, listManagedMarkerA, listFinalizerMarkerA, listOwnerMarkerA, listResourceMarkerA, "SMALL_FORBIDDEN_PAYLOAD_A_"}},
		{name: "B", raw: tableBytesB, markers: []string{"forbidden-plain-key-B", "forbidden-binary-key-B", "forbidden-base64-key-B", listPlainMarkerB, listBinaryMarkerB, listBase64MarkerB, listStringDataMarkerB, listLabelMarkerB, listAnnotationMarkerB, listLastAppliedB, listManagedMarkerB, listFinalizerMarkerB, listOwnerMarkerB, listResourceMarkerB, "RADICALLY_LARGE_FORBIDDEN_PAYLOAD_B_"}},
	}
	for _, fixture := range fixtureEvidence {
		for _, marker := range append(fixture.markers, "stringData", "binaryData", "managedFields", "finalizers", "ownerReferences", "resourceVersion", "kubectl.kubernetes.io/last-applied-configuration") {
			if !bytes.Contains(fixture.raw, []byte(marker)) {
				t.Fatalf("hostile fixture %s does not contain required evidence %q", fixture.name, marker)
			}
		}
	}
	detailSecret := v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: "detail", Namespace: "evidence", Labels: map[string]string{"private": "DETAIL_LABEL_FORBIDDEN"},
			Annotations:   map[string]string{"kubectl.kubernetes.io/last-applied-configuration": "DETAIL_LAST_APPLIED_FORBIDDEN"},
			ManagedFields: []metav1.ManagedFieldsEntry{{Manager: "DETAIL_MANAGED_FORBIDDEN"}}, Finalizers: []string{"DETAIL_FINALIZER_FORBIDDEN"},
			OwnerReferences: []metav1.OwnerReference{{Name: "DETAIL_OWNER_FORBIDDEN"}}, ResourceVersion: "DETAIL_RESOURCE_VERSION_FORBIDDEN",
		},
		Type:       v1.SecretTypeOpaque,
		Data:       map[string][]byte{"a-plain": []byte(detailMarker), "z-binary": {0xff, 0x00, 0x7f}},
		StringData: map[string]string{"never": "DETAIL_STRING_DATA_FORBIDDEN"},
	}
	listCalls := 0
	client := acceleratorProjectionTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/namespaces/evidence/secrets":
			listCalls++
			switch listCalls {
			case 1:
				_ = json.NewEncoder(w).Encode(tableA)
			case 2:
				_ = json.NewEncoder(w).Encode(tableB)
			default:
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(metav1.Status{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Status"}, Status: metav1.StatusFailure, Message: errorMarker, Reason: metav1.StatusReasonInternalError, Code: http.StatusInternalServerError})
			}
		case "/api/v1/namespaces/evidence/secrets/detail":
			_ = json.NewEncoder(w).Encode(detailSecret)
		case "/api/v1/namespaces/evidence/secrets/empty":
			_ = json.NewEncoder(w).Encode(v1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "empty", Namespace: "evidence"}, Data: map[string][]byte{}})
		default:
			http.NotFound(w, r)
		}
	}))
	var standardLogs bytes.Buffer
	previousLogWriter, previousLogFlags, previousLogPrefix := log.Writer(), log.Flags(), log.Prefix()
	log.SetOutput(&standardLogs)
	log.SetFlags(0)
	log.SetPrefix("")
	t.Cleanup(func() {
		log.SetOutput(previousLogWriter)
		log.SetFlags(previousLogFlags)
		log.SetPrefix(previousLogPrefix)
	})
	var debugLogs bytes.Buffer
	debug.Init(events.EmitterFunc(func(name string, data ...interface{}) {
		debugLogs.WriteString(name)
		encoded, _ := json.Marshal(data)
		debugLogs.Write(encoded)
	}))
	debug.SetEnabled(true)
	t.Cleanup(func() {
		debug.SetEnabled(false)
		debug.Init(&events.NoopEmitter{})
	})
	handler, _ := newProjectionCanonicalHandler(t, client, agent.V1Capabilities())
	authorization := "Bearer " + acceleratorEvidenceCreatorToken

	listResponseA := projectionCanonicalCall(handler, authorization, "ListSecretsMetadata", "request-fixture-a", "evidence")
	listResponseB := projectionCanonicalCall(handler, authorization, "ListSecretsMetadata", "request-fixture-b", "evidence")
	for i, response := range []*httptest.ResponseRecorder{listResponseA, listResponseB} {
		if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("list response %d = %d %q %#v", i, response.Code, response.Body.String(), response.Header())
		}
	}
	wantListBytes := []byte("{\"data\":[{\"metadata\":{\"name\":\"canonical-secret\",\"namespace\":\"evidence\",\"uid\":\"canonical-uid\",\"creationTimestamp\":\"2026-08-01T00:00:00Z\"},\"type\":\"Opaque\",\"dataKeys\":3}]}\n")
	if !bytes.Equal(listResponseA.Body.Bytes(), listResponseB.Body.Bytes()) || !bytes.Equal(listResponseA.Body.Bytes(), wantListBytes) {
		t.Fatalf("content-dependent list responses:\nA=%q\nB=%q\nwant=%q", listResponseA.Body.Bytes(), listResponseB.Body.Bytes(), wantListBytes)
	}
	for fixture, rawSize := range map[string]int{"A": len(tableBytesA), "B": len(tableBytesB)} {
		if rawSize < len(wantListBytes)*25 {
			t.Fatalf("list response is not dramatically bounded for fixture %s: raw=%d response=%d", fixture, rawSize, len(wantListBytes))
		}
	}
	forbiddenMarkers := []string{
		"forbidden-plain-key-A", "forbidden-plain-key-B", "forbidden-binary-key-A", "forbidden-binary-key-B", "forbidden-base64-key-A", "forbidden-base64-key-B",
		listPlainMarkerA, listPlainMarkerB, listBinaryMarkerA, listBinaryMarkerB, listBase64MarkerA, listBase64MarkerB, listStringDataMarkerA, listStringDataMarkerB,
		listLabelMarkerA, listLabelMarkerB, listAnnotationMarkerA, listAnnotationMarkerB, listLastAppliedA, listLastAppliedB, listManagedMarkerA, listManagedMarkerB,
		listFinalizerMarkerA, listFinalizerMarkerB, listOwnerMarkerA, listOwnerMarkerB, listResourceMarkerA, listResourceMarkerB,
		k8s.HelmReleaseSecretType, "sh.helm.release", progressMarker, logMarker, errorMarker, "SMALL_FORBIDDEN_PAYLOAD_A_", "RADICALLY_LARGE_FORBIDDEN_PAYLOAD_B_",
	}
	for _, forbidden := range forbiddenMarkers {
		if bytes.Contains(listResponseA.Body.Bytes(), []byte(forbidden)) || bytes.Contains(listResponseB.Body.Bytes(), []byte(forbidden)) {
			t.Fatalf("list envelope leaked %q", forbidden)
		}
	}

	listErrorResponse := projectionCanonicalCall(handler, authorization, "ListSecretsMetadata", "request-error", "evidence")
	if listErrorResponse.Code != http.StatusInternalServerError || listErrorResponse.Body.String() != "{\"error\":\"internal error\"}\n" || listErrorResponse.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("list error response = %d %q %#v", listErrorResponse.Code, listErrorResponse.Body.String(), listErrorResponse.Header())
	}
	if listCalls != 3 {
		t.Fatalf("canonical list error used a substitute detail path: list calls=%d", listCalls)
	}
	allFailureOutput := listErrorResponse.Body.String() + standardLogs.String() + debugLogs.String()
	if strings.Contains(allFailureOutput, "GetSecretData") || strings.Contains(allFailureOutput, detailMarker) {
		t.Fatalf("canonical list error was substituted with a detail path: %q", allFailureOutput)
	}
	for _, forbidden := range append(forbiddenMarkers, errorMarker) {
		if strings.Contains(allFailureOutput, forbidden) {
			t.Fatalf("response or standard/debug logs leaked %q: %q", forbidden, allFailureOutput)
		}
	}

	secretEditorSource, err := os.ReadFile("frontend/src/components/shared/SecretEditor.tsx")
	if err != nil {
		t.Fatal(err)
	}
	fetchStart := bytes.Index(secretEditorSource, []byte("    const fetchData = async () => {"))
	if fetchStart < 0 {
		t.Fatal("SecretEditor fetchData source boundary not found")
	}
	fetchEndRelative := bytes.Index(secretEditorSource[fetchStart:], []byte("\n    const handleSaveYaml = async () => {"))
	if fetchEndRelative < 0 {
		t.Fatal("SecretEditor fetchData source boundary not found")
	}
	fetchDataSource := string(secretEditorSource[fetchStart : fetchStart+fetchEndRelative])
	exactFetchPair := "const [yaml, data] = await Promise.all([\n                GetSecretYaml(namespace, resourceName),\n                GetSecretData(namespace, resourceName)\n            ]);"
	if !strings.Contains(fetchDataSource, exactFetchPair) || strings.Count(fetchDataSource, "GetSecretYaml(namespace, resourceName)") != 1 || strings.Count(fetchDataSource, "GetSecretData(namespace, resourceName)") != 1 {
		t.Fatalf("SecretEditor fetchData must invoke the exact projected detail methods: %s", fetchDataSource)
	}

	dataResponse := projectionCanonicalCall(handler, authorization, "GetSecretData", "evidence", "detail")
	if dataResponse.Code != http.StatusOK || !strings.Contains(dataResponse.Body.String(), detailMarker) {
		t.Fatalf("data response = %d %q", dataResponse.Code, dataResponse.Body.String())
	}
	var gotData interface{}
	if err := json.Unmarshal(dataResponse.Body.Bytes(), &gotData); err != nil {
		t.Fatal(err)
	}
	wantDataJSON := `{"data":[{"key":"a-plain","value":"DETAIL_ALLOWED_MARKER","base64Value":"REVUQUlMX0FMTE9XRURfTUFSS0VS","isBinary":false,"source":"data","encoding":"text"},{"key":"z-binary","value":"","base64Value":"/wB/","isBinary":true,"source":"data","encoding":"base64"}]}`
	var wantData interface{}
	if err := json.Unmarshal([]byte(wantDataJSON), &wantData); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotData, wantData) {
		t.Fatalf("data envelope = %s, want %s", dataResponse.Body.String(), wantDataJSON)
	}

	emptyDataResponse := projectionCanonicalCall(handler, authorization, "GetSecretData", "evidence", "empty")
	if emptyDataResponse.Code != http.StatusOK || emptyDataResponse.Body.String() != "{\"data\":[]}\n" {
		t.Fatalf("empty data response = %d %q", emptyDataResponse.Code, emptyDataResponse.Body.String())
	}

	yamlResponse := projectionCanonicalCall(handler, authorization, "GetSecretYaml", "evidence", "detail")
	if yamlResponse.Code != http.StatusOK {
		t.Fatalf("YAML response = %d %q", yamlResponse.Code, yamlResponse.Body.String())
	}
	var yamlEnvelope struct {
		Data string `json:"data"`
	}
	if err := json.Unmarshal(yamlResponse.Body.Bytes(), &yamlEnvelope); err != nil {
		t.Fatal(err)
	}
	wantYAML := "apiVersion: v1\ndata:\n  a-plain: REVUQUlMX0FMTE9XRURfTUFSS0VS\n  z-binary: /wB/\nkind: Secret\nmetadata:\n  name: detail\n  namespace: evidence\ntype: Opaque\n"
	if yamlEnvelope.Data != wantYAML || !strings.Contains(yamlEnvelope.Data, "a-plain") {
		t.Fatalf("YAML envelope = %q, want %q", yamlEnvelope.Data, wantYAML)
	}
	for _, forbidden := range []string{"DETAIL_LABEL_FORBIDDEN", "DETAIL_LAST_APPLIED_FORBIDDEN", "DETAIL_MANAGED_FORBIDDEN", "DETAIL_FINALIZER_FORBIDDEN", "DETAIL_OWNER_FORBIDDEN", "DETAIL_RESOURCE_VERSION_FORBIDDEN", "DETAIL_STRING_DATA_FORBIDDEN"} {
		if strings.Contains(yamlEnvelope.Data, forbidden) || strings.Contains(dataResponse.Body.String(), forbidden) {
			t.Fatalf("detail envelope leaked %q", forbidden)
		}
	}
}
