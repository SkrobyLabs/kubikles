package main

import (
	"context"
	"crypto/rand"
	"embed"
	"errors"
	"fmt"
	"io"
	"k8s.io/apimachinery/pkg/watch"
	"os"
	"os/signal"
	"syscall"
	"time"

	"kubikles/pkg/agent"
	"kubikles/pkg/crashlog"
	"kubikles/pkg/events"
	"kubikles/pkg/k8s"
	"kubikles/pkg/server"
)

// RunServer starts the HTTP/WebSocket server mode.
// This is shared between the desktop build (with -server flag) and headless builds.
func RunServer(assets embed.FS, port int, label string) {
	fmt.Printf("Kubikles - %s\n", label)
	fmt.Printf("Starting server on port %d...\n", port)

	// Create context that cancels on interrupt
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\nShutting down...")
		cancel()
	}()

	if err := RunServerWithOptions(ctx, assets, port, label, AppOptions{Mode: RuntimeModeServer}); err != nil {
		crashlog.LogFatal("Server error: %v", err)
	}
}

// RunServerWithOptions starts server mode with explicit App composition options.
func RunServerWithOptions(ctx context.Context, assets embed.FS, port int, label string, options AppOptions) error {
	return runServerWithOptions(ctx, assets, port, label, options, productionServerModeDependencies())
}

type serverModeDependencies struct {
	lookupEnv                     func(string) string
	newCreatorIdentity            func(string) (*server.CreatorAuthenticator, string, error)
	acceleratorObserver           server.AcceleratorSessionObserver
	browserSessionNow             func() time.Time
	browserSessionEntropy         io.Reader
	newAcceleratorSessions        func(string, server.AcceleratorSessionObserver) *server.AcceleratorSessionRegistry
	newBrowserSessions            func(func() time.Time, io.Reader, server.BrowserSessionRevoker) *server.BrowserSessionManager
	capabilityResolverFactory     func(*k8s.Client) agent.CapabilityResolverFactory
	newServer                     func(server.MethodCaller, embed.FS, server.Options) (serverModeServer, error)
	newAcceleratorIdleCoordinator func(agent.DisposableIdleLifecycle, context.Context, func()) acceleratorIdleCoordinator
}

type serverModeServer interface {
	EmitEvent(string, interface{})
	AddDisconnectListener(server.DisconnectListener)
	Run(context.Context) error
}

type acceleratorServerModeServer interface {
	serverModeServer
	RunWithReady(context.Context, func()) error
	Quiesce()
}

type acceleratorIdleCoordinator interface {
	server.AcceleratorSessionObserver
	MarkReady()
	Shutdown()
}

var errAcceleratorIdleServerUnsupported = errors.New("accelerator server does not support disposable idle lifecycle")

func productionServerModeDependencies() serverModeDependencies {
	return serverModeDependencies{
		lookupEnv: os.Getenv,
		newCreatorIdentity: func(encodedVerifier string) (*server.CreatorAuthenticator, string, error) {
			creator, err := server.NewCreatorAuthenticator(encodedVerifier)
			if err != nil {
				return nil, "", err
			}
			instanceID, err := server.NewAcceleratorInstanceID()
			if err != nil {
				return nil, "", err
			}
			return creator, instanceID, nil
		},
		acceleratorObserver:       server.NoopAcceleratorSessionObserver{},
		browserSessionNow:         time.Now,
		browserSessionEntropy:     rand.Reader,
		newAcceleratorSessions:    server.NewAcceleratorSessionRegistry,
		newBrowserSessions:        server.NewBrowserSessionManagerWithDependencies,
		capabilityResolverFactory: k8s.NewSecretCapabilityResolverFactory,
		newServer: func(caller server.MethodCaller, assets embed.FS, options server.Options) (serverModeServer, error) {
			return server.NewWithOptions(caller, assets, options)
		},
		newAcceleratorIdleCoordinator: func(lifecycle agent.DisposableIdleLifecycle, ctx context.Context, report func()) acceleratorIdleCoordinator {
			return server.NewAcceleratorIdleCoordinator(lifecycle, ctx, report)
		},
	}
}

type acceleratorStartupIdentity struct {
	creator    *server.CreatorAuthenticator
	instanceID string
}

func acceleratorIdentityForRuntime(mode RuntimeMode, dependencies serverModeDependencies) (acceleratorStartupIdentity, error) {
	if mode != RuntimeModeAccelerator {
		return acceleratorStartupIdentity{}, nil
	}
	creator, instanceID, err := dependencies.newCreatorIdentity(dependencies.lookupEnv("KUBIKLES_ACCELERATOR_CREATOR_VERIFIER"))
	if err != nil {
		return acceleratorStartupIdentity{}, err
	}
	return acceleratorStartupIdentity{creator: creator, instanceID: instanceID}, nil
}

func runServerWithOptions(ctx context.Context, assets embed.FS, port int, label string, options AppOptions, dependencies serverModeDependencies) error {
	identity, err := acceleratorIdentityForRuntime(options.Mode, dependencies)
	if err != nil {
		return err
	}
	app, err := NewAppWithOptions(options)
	if err != nil {
		return err
	}
	defer app.shutdown(ctx)

	readiness := newAppReadinessProvider(app)
	serverOptions, err := serverOptionsForRuntime(options.Mode, port, readiness)
	if err != nil {
		return err
	}
	var idle acceleratorIdleCoordinator
	var idleLifecycle *acceleratorDisposableLifecycle
	var idleProtection acceleratorHTTPProtection
	runCtx := ctx
	var requestExit context.CancelFunc
	if options.Mode == RuntimeModeAccelerator {
		runCtx, requestExit = context.WithCancel(ctx)
		defer requestExit()
		idleLifecycle = &acceleratorDisposableLifecycle{cancel: requestExit}
		newIdle := dependencies.newAcceleratorIdleCoordinator
		if newIdle == nil {
			newIdle = func(lifecycle agent.DisposableIdleLifecycle, ctx context.Context, report func()) acceleratorIdleCoordinator {
				return server.NewAcceleratorIdleCoordinator(lifecycle, ctx, report)
			}
		}
		idle = newIdle(idleLifecycle, runCtx, reportAcceleratorIdleCleanupFailure)
		if idle == nil {
			return errAcceleratorIdleLifecycleUnbound
		}
		defer idle.Shutdown()
		idleDependencies := dependencies
		// The registry has one observer; chain Secret membership maintenance before
		// the idle coordinator so synchronous revocation is fully cleaned first.
		var registry *server.AcceleratorSessionRegistry
		var secretManager *AcceleratorSecretWatchManager
		if app.k8sClient != nil {
			secretManager = newAcceleratorSecretWatchManager(
				func(wctx context.Context, namespace, rv string, listOptions k8s.SecretListOptions) (watch.Interface, error) {
					return app.k8sClient.WatchSecrets(wctx, namespace, rv, listOptions)
				},
				func(id agent.SessionID) (server.AcceleratorSessionLease, bool) {
					if registry == nil {
						return server.AcceleratorSessionLease{}, false
					}
					return registry.LookupSessionLease(id)
				},
				func(targets []server.AcceleratorSessionTarget, event server.Event) {
					if registry != nil {
						registry.EmitEventToTargets(targets, event)
					}
				},
			)
		}
		chain := &acceleratorSessionObserverChain{secret: secretManager, idle: idle}
		idleDependencies.acceleratorObserver = chain
		protection, err := newAcceleratorHTTPProtectionWithDependencies(runCtx, app, identity.creator, identity.instanceID, idleDependencies)
		if err != nil {
			return err
		}
		idleProtection = protection
		registry = protection.AcceleratorSessions
		app.acceleratorSecretWatches = secretManager
		installAcceleratorHTTPProtection(&serverOptions, protection)
	}
	newServer := dependencies.newServer
	if newServer == nil {
		newServer = func(caller server.MethodCaller, assets embed.FS, options server.Options) (serverModeServer, error) {
			return server.NewWithOptions(caller, assets, options)
		}
	}
	caller := server.MethodCaller(NewAppMethodCaller(app))
	if options.Mode == RuntimeModeAccelerator {
		caller = newAcceleratorSecretCaller(caller, app)
	}
	srv, err := newServer(caller, assets, serverOptions)
	if err != nil {
		return err
	}
	var acceleratorSrv acceleratorServerModeServer
	if options.Mode == RuntimeModeAccelerator {
		var ok bool
		acceleratorSrv, ok = srv.(acceleratorServerModeServer)
		if !ok {
			return errAcceleratorIdleServerUnsupported
		}
		if err := idleLifecycle.bind(acceleratorSrv, idleProtection.BrowserSessions, idleProtection.AcceleratorSessions, acceleratorSessionStateCleanerFunc(app.clearAcceleratorSessionState)); err != nil {
			return err
		}
	}
	app.SetEmitter(events.EmitterFunc(func(name string, data ...interface{}) {
		if len(data) > 0 {
			srv.EmitEvent(name, data[0])
		} else {
			srv.EmitEvent(name, nil)
		}
	}))
	app.startupServerMode(runCtx)
	readiness.markInitialized()
	for _, listener := range app.getDisconnectListeners() {
		srv.AddDisconnectListener(listener)
	}
	if options.Mode != RuntimeModeAccelerator {
		return srv.Run(ctx)
	}
	return acceleratorSrv.RunWithReady(runCtx, idle.MarkReady)
}

type acceleratorHTTPProtection struct {
	Guard                  server.ProtectedRouteGuard
	Info                   server.AcceleratorInfoProvider
	Authorizer             server.MethodAuthorizer
	BrowserSessions        *server.BrowserSessionManager
	AcceleratorSessions    *server.AcceleratorSessionRegistry
	WebSocketAuthenticator *server.AcceleratorWebSocketAuthenticator
}

// acceleratorSessionObserverChain keeps Secret cleanup ahead of idle accounting.
// It is private composition only; the registry still invokes it out of locks.
type acceleratorSessionObserverChain struct {
	secret *AcceleratorSecretWatchManager
	idle   server.AcceleratorSessionObserver
}

func (c *acceleratorSessionObserverChain) SessionConnected(s server.AcceleratorSessionSnapshot) {
	if c.secret != nil {
		c.secret.SessionConnected(s)
	}
	if c.idle != nil {
		c.idle.SessionConnected(s)
	}
}
func (c *acceleratorSessionObserverChain) SessionDisconnected(s server.AcceleratorSessionSnapshot) {
	if c.secret != nil {
		c.secret.SessionDisconnected(s)
	}
	if c.idle != nil {
		c.idle.SessionDisconnected(s)
	}
}
func (c *acceleratorSessionObserverChain) SessionRevoked(s server.AcceleratorSessionSnapshot) {
	if c.secret != nil {
		c.secret.SessionRevoked(s)
	}
	if c.idle != nil {
		c.idle.SessionRevoked(s)
	}
}

func newAcceleratorHTTPProtection(ctx context.Context, app *App, creator *server.CreatorAuthenticator, instanceID string) (acceleratorHTTPProtection, error) {
	return newAcceleratorHTTPProtectionWithDependencies(ctx, app, creator, instanceID, productionServerModeDependencies())
}

func newAcceleratorHTTPProtectionWithDependencies(ctx context.Context, app *App, creator *server.CreatorAuthenticator, instanceID string, dependencies serverModeDependencies) (acceleratorHTTPProtection, error) {
	if creator == nil || instanceID == "" {
		return acceleratorHTTPProtection{}, server.ErrInvalidCreatorVerifier
	}
	resolutionCtx, cancel := context.WithTimeout(ctx, 7*time.Second)
	defer cancel()
	resolution := agent.CapabilityResolution{}
	newResolverFactory := dependencies.capabilityResolverFactory
	if newResolverFactory == nil {
		newResolverFactory = k8s.NewSecretCapabilityResolverFactory
	}
	resolverFactory := newResolverFactory(app.k8sClient)
	if resolverFactory != nil {
		if resolver := resolverFactory(); resolver != nil {
			resolution = resolver.ResolveCapabilities(resolutionCtx)
		}
	}
	build := agent.BuildIdentity{BuildVersion: BuildVersion, Commit: GitCommit, Dirty: GitDirty == "true"}
	newBrowserSessions := dependencies.newBrowserSessions
	if newBrowserSessions == nil {
		newBrowserSessions = server.NewBrowserSessionManagerWithDependencies
	}
	newAcceleratorSessions := dependencies.newAcceleratorSessions
	if newAcceleratorSessions == nil {
		newAcceleratorSessions = server.NewAcceleratorSessionRegistry
	}
	observer := dependencies.acceleratorObserver
	if observer == nil {
		observer = server.NoopAcceleratorSessionObserver{}
	}
	now := dependencies.browserSessionNow
	if now == nil {
		now = time.Now
	}
	entropy := dependencies.browserSessionEntropy
	if entropy == nil {
		entropy = rand.Reader
	}
	registry := newAcceleratorSessions(instanceID, observer)
	sessions := newBrowserSessions(now, entropy, registry)
	return acceleratorHTTPProtection{Guard: server.CreatorOrBrowserGuard(creator, sessions), Info: server.NewAuthenticatedAcceleratorInfo(build, instanceID, resolution), Authorizer: server.NewAcceleratorMethodAuthorizer(resolution), BrowserSessions: sessions, AcceleratorSessions: registry, WebSocketAuthenticator: &server.AcceleratorWebSocketAuthenticator{Creator: creator, BrowserSessions: sessions, Registry: registry}}, nil
}

func installAcceleratorHTTPProtection(options *server.Options, protection acceleratorHTTPProtection) {
	options.ProtectedRouteGuard = protection.Guard
	options.AcceleratorInfoProvider = protection.Info
	options.MethodAuthorizer = protection.Authorizer
	options.BrowserSessions = protection.BrowserSessions
	options.AcceleratorSessions = protection.AcceleratorSessions
	options.AcceleratorWebSocketAuthenticator = protection.WebSocketAuthenticator
}

func serverOptionsForRuntime(mode RuntimeMode, port int, readiness server.ReadinessProvider) (server.Options, error) {
	switch mode {
	case RuntimeModeAccelerator:
		return server.AcceleratorOptions(port, readiness, server.DenyProtectedRoutes), nil
	case RuntimeModeDesktop, RuntimeModeServer:
		return server.CompatibilityOptions(port, readiness), nil
	default:
		return server.Options{}, fmt.Errorf("%w: %q", ErrInvalidRuntimeMode, mode)
	}
}
