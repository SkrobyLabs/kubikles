package main

import (
	"context"
	"embed"
	"errors"
	"net"
	"testing"

	"k8s.io/client-go/rest"
	"kubikles/pkg/k8s"
	"kubikles/pkg/server"
)

func TestRunServerWithOptionsReturnsAcceleratorConstructionErrorBeforeListen(t *testing.T) {
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
