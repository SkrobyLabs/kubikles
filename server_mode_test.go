package main

import (
	"context"
	"embed"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"k8s.io/client-go/rest"
	"kubikles/pkg/agent"
	"kubikles/pkg/k8s"
	"kubikles/pkg/server"
)

type capturedServerModeServer struct {
	mu       sync.Mutex
	options  []server.Options
	handlers []http.Handler
}

func (s *capturedServerModeServer) newServer(caller server.MethodCaller, assets embed.FS, options server.Options) (serverModeServer, error) {
	constructed, err := server.NewWithOptions(caller, assets, options)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.options = append(s.options, options)
	s.handlers = append(s.handlers, constructed.Handler())
	s.mu.Unlock()
	return s, nil
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
	protection, err := newAcceleratorHTTPProtectionWithDependencies(callerCtx, &App{}, creator, "accel-public", newResolverFactory)
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
	})
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
	})
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
