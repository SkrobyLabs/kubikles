package main

import (
	"context"
	"embed"
	"fmt"
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
	lookupEnv                 func(string) string
	newCreatorIdentity        func(string) (*server.CreatorAuthenticator, string, error)
	capabilityResolverFactory func(*k8s.Client) agent.CapabilityResolverFactory
	newServer                 func(server.MethodCaller, embed.FS, server.Options) (serverModeServer, error)
}

type serverModeServer interface {
	EmitEvent(string, interface{})
	AddDisconnectListener(server.DisconnectListener)
	Run(context.Context) error
}

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
		capabilityResolverFactory: k8s.NewSecretCapabilityResolverFactory,
		newServer: func(caller server.MethodCaller, assets embed.FS, options server.Options) (serverModeServer, error) {
			return server.NewWithOptions(caller, assets, options)
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
	if options.Mode == RuntimeModeAccelerator {
		protection, err := newAcceleratorHTTPProtectionWithDependencies(ctx, app, identity.creator, identity.instanceID, dependencies.capabilityResolverFactory)
		if err != nil {
			return err
		}
		installAcceleratorHTTPProtection(&serverOptions, protection)
	}
	newServer := dependencies.newServer
	if newServer == nil {
		newServer = func(caller server.MethodCaller, assets embed.FS, options server.Options) (serverModeServer, error) {
			return server.NewWithOptions(caller, assets, options)
		}
	}
	srv, err := newServer(NewAppMethodCaller(app), assets, serverOptions)
	if err != nil {
		return err
	}
	app.SetEmitter(events.EmitterFunc(func(name string, data ...interface{}) {
		if len(data) > 0 {
			srv.EmitEvent(name, data[0])
		} else {
			srv.EmitEvent(name, nil)
		}
	}))
	app.startupServerMode(ctx)
	readiness.markInitialized()
	for _, listener := range app.getDisconnectListeners() {
		srv.AddDisconnectListener(listener)
	}
	return srv.Run(ctx)
}

type acceleratorHTTPProtection struct {
	Guard      server.ProtectedRouteGuard
	Info       server.AcceleratorInfoProvider
	Authorizer server.MethodAuthorizer
}

func newAcceleratorHTTPProtection(ctx context.Context, app *App, creator *server.CreatorAuthenticator, instanceID string) (acceleratorHTTPProtection, error) {
	return newAcceleratorHTTPProtectionWithDependencies(ctx, app, creator, instanceID, k8s.NewSecretCapabilityResolverFactory)
}

func newAcceleratorHTTPProtectionWithDependencies(ctx context.Context, app *App, creator *server.CreatorAuthenticator, instanceID string, newResolverFactory func(*k8s.Client) agent.CapabilityResolverFactory) (acceleratorHTTPProtection, error) {
	if creator == nil || instanceID == "" {
		return acceleratorHTTPProtection{}, server.ErrInvalidCreatorVerifier
	}
	resolutionCtx, cancel := context.WithTimeout(ctx, 7*time.Second)
	defer cancel()
	resolution := agent.CapabilityResolution{}
	resolverFactory := newResolverFactory(app.k8sClient)
	if resolverFactory != nil {
		if resolver := resolverFactory(); resolver != nil {
			resolution = resolver.ResolveCapabilities(resolutionCtx)
		}
	}
	build := agent.BuildIdentity{BuildVersion: BuildVersion, Commit: GitCommit, Dirty: GitDirty == "true"}
	return acceleratorHTTPProtection{Guard: creator.Guard, Info: server.NewAuthenticatedAcceleratorInfo(build, instanceID, resolution), Authorizer: server.NewAcceleratorMethodAuthorizer(resolution)}, nil
}

func installAcceleratorHTTPProtection(options *server.Options, protection acceleratorHTTPProtection) {
	options.ProtectedRouteGuard = protection.Guard
	options.AcceleratorInfoProvider = protection.Info
	options.MethodAuthorizer = protection.Authorizer
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
