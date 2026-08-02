package k8s

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/util/httpstream"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
)

type fakeAcceleratorForwarder struct {
	ready   chan struct{}
	stop    chan struct{}
	ports   []portforward.ForwardedPort
	mode    string
	release <-chan struct{}
	started chan<- struct{}
	stopped chan<- struct{}
}

func (f *fakeAcceleratorForwarder) ForwardPorts() error {
	switch f.mode {
	case "error-before-ready":
		return errors.New("raw forward error")
	case "error-after-ready":
		close(f.ready)
		return errors.New("raw forward error")
	case "blocked":
		close(f.started)
		<-f.release
		return errors.New("raw forward error")
	case "wait-after-stop":
		close(f.ready)
		<-f.stop
		close(f.stopped)
		<-f.release
		return nil
	default:
		close(f.ready)
		<-f.stop
		return nil
	}
}

type dialingAcceleratorForwarder struct{ dialer httpstream.Dialer }

func (f *dialingAcceleratorForwarder) ForwardPorts() error {
	_, _, err := f.dialer.Dial(portforward.PortForwardProtocolV1Name)
	return err
}
func (*dialingAcceleratorForwarder) GetPorts() ([]portforward.ForwardedPort, error) { return nil, nil }

func TestAcceleratorTunnelCancellationDoesNotWaitForBlockedForwarder(t *testing.T) {
	original := newAcceleratorPortForwarder
	defer func() { newAcceleratorPortForwarder = original }()
	release := make(chan struct{})
	started := make(chan struct{})
	forwarder := &fakeAcceleratorForwarder{mode: "blocked", release: release, started: started}
	newAcceleratorPortForwarder = func(_ httpstream.Dialer, _ []string, _ []string, stop, ready chan struct{}) (acceleratorPortForwarder, error) {
		forwarder.stop, forwarder.ready = stop, ready
		return forwarder, nil
	}
	snapshot := &AcceleratorContextSnapshot{restConfig: &rest.Config{Host: "https://api.example.test", TLSClientConfig: rest.TLSClientConfig{Insecure: true}}}
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := StartAcceleratorPodTunnel(ctx, snapshot, "team-a", "pod-a")
		result <- err
	}()
	<-started
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, ErrAcceleratorContextUnavailable) {
			t.Fatalf("unexpected cancellation error: %v", err)
		}
	case <-time.After(250 * time.Millisecond):
		close(release)
		t.Fatal("cancellation waited for a blocked forwarder")
	}
	close(release)
}

func TestAcceleratorTunnelInvalidPortsWaitsForForwarderOwnership(t *testing.T) {
	original := newAcceleratorPortForwarder
	defer func() { newAcceleratorPortForwarder = original }()
	release := make(chan struct{})
	stopped := make(chan struct{})
	newAcceleratorPortForwarder = func(_ httpstream.Dialer, _ []string, _ []string, stop, ready chan struct{}) (acceleratorPortForwarder, error) {
		return &fakeAcceleratorForwarder{mode: "wait-after-stop", ready: ready, stop: stop, release: release, stopped: stopped}, nil
	}
	snapshot := &AcceleratorContextSnapshot{restConfig: &rest.Config{Host: "https://api.example.test", TLSClientConfig: rest.TLSClientConfig{Insecure: true}}}
	result := make(chan error, 1)
	go func() {
		_, err := StartAcceleratorPodTunnel(context.Background(), snapshot, "team-a", "pod-a")
		result <- err
	}()
	<-stopped
	select {
	case err := <-result:
		t.Fatalf("invalid-port path returned before forwarder ownership settled: %v", err)
	case <-time.After(25 * time.Millisecond):
	}
	close(release)
	select {
	case err := <-result:
		if !errors.Is(err, ErrAcceleratorContextUnavailable) {
			t.Fatalf("unexpected invalid-port error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("invalid-port cleanup did not finish")
	}
}

func TestAcceleratorTunnelPreservesWrapTransportAndSealsRequestContext(t *testing.T) {
	original := newAcceleratorPortForwarder
	defer func() { newAcceleratorPortForwarder = original }()
	newAcceleratorPortForwarder = func(dialer httpstream.Dialer, _ []string, _ []string, _, _ chan struct{}) (acceleratorPortForwarder, error) {
		return &dialingAcceleratorForwarder{dialer: dialer}, nil
	}
	var wrapperBuilt, wrapperCalled, leakedContext, serverSawWrapper atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("X-Accelerator-Wrapper") == "preserved" {
			serverSawWrapper.Add(1)
		}
		http.Error(writer, "upgrade rejected", http.StatusBadRequest)
	}))
	defer server.Close()
	type contextKey struct{}
	cfg := &rest.Config{Host: server.URL, WrapTransport: func(next http.RoundTripper) http.RoundTripper {
		wrapperBuilt.Add(1)
		return acceleratorRoundTripper(func(request *http.Request) (*http.Response, error) {
			wrapperCalled.Add(1)
			if request.Context().Value(contextKey{}) != nil || httptrace.ContextClientTrace(request.Context()) != nil {
				leakedContext.Add(1)
			}
			request.Header.Set("X-Accelerator-Wrapper", "preserved")
			return next.RoundTrip(request)
		})
	}}
	snapshot := &AcceleratorContextSnapshot{restConfig: cfg}
	trace := &httptrace.ClientTrace{GetConn: func(string) { leakedContext.Add(1) }}
	ctx := context.WithValue(context.Background(), contextKey{}, "private")
	ctx = httptrace.WithClientTrace(ctx, trace)
	if tunnel, err := StartAcceleratorPodTunnel(ctx, snapshot, "team-a", "pod-a"); err == nil || tunnel != nil {
		t.Fatalf("rejected real transport unexpectedly connected: %#v %v", tunnel, err)
	}
	if wrapperBuilt.Load() < 2 || wrapperCalled.Load() == 0 || serverSawWrapper.Load() == 0 || leakedContext.Load() != 0 {
		t.Fatalf("built=%d called=%d server=%d leaked=%d", wrapperBuilt.Load(), wrapperCalled.Load(), serverSawWrapper.Load(), leakedContext.Load())
	}
}

func (f *fakeAcceleratorForwarder) GetPorts() ([]portforward.ForwardedPort, error) {
	return append([]portforward.ForwardedPort(nil), f.ports...), nil
}

func TestAcceleratorTunnelUsesExactLoopbackOSPort(t *testing.T) {
	target, err := acceleratorPortForwardURL("https://api.example.test/prefix", "team-a", "pod-a")
	if err != nil || target.String() != "https://api.example.test/prefix/api/v1/namespaces/team-a/pods/pod-a/portforward" {
		t.Fatalf("unexpected target: %v %v", target, err)
	}
	if !acceleratorTunnelFallback(&httpstream.UpgradeFailureError{Cause: errors.New("upgrade")}) || !acceleratorTunnelFallback(errors.New("proxy: unknown scheme: https")) || acceleratorTunnelFallback(errors.New("connection refused")) {
		t.Fatal("fallback predicate widened beyond upgrade/HTTPS proxy failures")
	}

	original := newAcceleratorPortForwarder
	defer func() { newAcceleratorPortForwarder = original }()
	var gotAddresses, gotPorts []string
	newAcceleratorPortForwarder = func(_ httpstream.Dialer, addresses, ports []string, stop, ready chan struct{}) (acceleratorPortForwarder, error) {
		gotAddresses, gotPorts = append([]string(nil), addresses...), append([]string(nil), ports...)
		return &fakeAcceleratorForwarder{ready: ready, stop: stop, ports: []portforward.ForwardedPort{{Local: 43123, Remote: 8080}}}, nil
	}
	snapshot := &AcceleratorContextSnapshot{restConfig: &rest.Config{Host: "https://api.example.test", TLSClientConfig: rest.TLSClientConfig{Insecure: true}}}
	tunnel, err := StartAcceleratorPodTunnel(context.Background(), snapshot, "team-a", "pod-a")
	if err != nil {
		t.Fatal(err)
	}
	if tunnel.Port() != 43123 || !reflect.DeepEqual(gotAddresses, []string{"127.0.0.1"}) || !reflect.DeepEqual(gotPorts, []string{"0:8080"}) {
		t.Fatalf("binding contract mismatch port=%d addresses=%v ports=%v", tunnel.Port(), gotAddresses, gotPorts)
	}
	tunnel.Stop()
	if err = tunnel.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Concurrent stop/wait remains idempotent and cannot close a channel twice.
	for index := 0; index < 16; index++ {
		go tunnel.Stop()
	}
}

func TestAcceleratorTunnelReadyFailureRaces(t *testing.T) {
	original := newAcceleratorPortForwarder
	defer func() { newAcceleratorPortForwarder = original }()
	snapshot := &AcceleratorContextSnapshot{restConfig: &rest.Config{Host: "https://api.example.test", TLSClientConfig: rest.TLSClientConfig{Insecure: true}}}
	tests := []struct {
		name               string
		mode               string
		ports              []portforward.ForwardedPort
		mayPublishThenStop bool
	}{
		{"error before ready", "error-before-ready", []portforward.ForwardedPort{{Local: 43123, Remote: 8080}}, false},
		// Completion and publication race at the explicit linearization lock.
		// Either completion rejects startup or publication returns a tunnel whose
		// Done channel is already (or imminently) closed; neither outcome can
		// return a durable live tunnel.
		{"error immediately after ready", "error-after-ready", []portforward.ForwardedPort{{Local: 43123, Remote: 8080}}, true},
		{"missing port", "", nil, false},
		{"zero local", "", []portforward.ForwardedPort{{Local: 0, Remote: 8080}}, false},
		{"wrong remote", "", []portforward.ForwardedPort{{Local: 43123, Remote: 8081}}, false},
		{"multiple", "", []portforward.ForwardedPort{{Local: 43123, Remote: 8080}, {Local: 43124, Remote: 8080}}, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			newAcceleratorPortForwarder = func(_ httpstream.Dialer, _ []string, _ []string, stop, ready chan struct{}) (acceleratorPortForwarder, error) {
				return &fakeAcceleratorForwarder{ready: ready, stop: stop, ports: test.ports, mode: test.mode}, nil
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			tunnel, err := StartAcceleratorPodTunnel(ctx, snapshot, "team-a", "pod-a")
			if test.mayPublishThenStop && err == nil && tunnel != nil {
				select {
				case <-tunnel.Done():
				case <-time.After(time.Second):
					t.Fatal("completion lost after publication")
				}
				return
			}
			if err == nil || tunnel != nil {
				t.Fatalf("invalid readiness accepted: %#v %v", tunnel, err)
			}
		})
	}
}
