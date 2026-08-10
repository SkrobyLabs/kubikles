package k8s

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"kubikles/pkg/debug"
	"kubikles/pkg/events"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	failure error
}

type rawAcceleratorTunnelTestError struct{ secret string }

func (e *rawAcceleratorTunnelTestError) Error() string { return e.secret }

func (f *fakeAcceleratorForwarder) fail() error {
	if f.failure != nil {
		return f.failure
	}
	return errors.New("raw forward error")
}

func (f *fakeAcceleratorForwarder) ForwardPorts() error {
	switch f.mode {
	case "error-before-ready":
		return f.fail()
	case "error-after-ready":
		close(f.ready)
		return f.fail()
	case "ready-block-error":
		close(f.ready)
		<-f.release
		return f.fail()
	case "blocked":
		close(f.started)
		<-f.release
		return f.fail()
	case "wait-after-stop":
		close(f.ready)
		<-f.stop
		close(f.stopped)
		<-f.release
		return nil
	case "wait-stop-deadline":
		<-f.stop
		close(f.stopped)
		return context.DeadlineExceeded
	default:
		close(f.ready)
		<-f.stop
		return nil
	}
}

func TestAcceleratorTunnelClassifiesConstructionAndTerminalCausesOpaquely(t *testing.T) {
	original := newAcceleratorPortForwarder
	defer func() { newAcceleratorPortForwarder = original }()
	snapshot := &AcceleratorContextSnapshot{restConfig: &rest.Config{Host: "https://api.example.test", TLSClientConfig: rest.TLSClientConfig{Insecure: true}}}
	causes := []struct {
		name  string
		cause error
		match func(error) bool
	}{
		{name: "upgrade", cause: &httpstream.UpgradeFailureError{Cause: errors.New("raw upgrade")}, match: func(err error) bool { return ClassifyAcceleratorTunnelFailure(err) == AcceleratorTunnelFailureUpgrade }},
		{name: "https proxy", cause: errors.New("proxy: unknown scheme: https"), match: func(err error) bool {
			return ClassifyAcceleratorTunnelFailure(err) == AcceleratorTunnelFailureHTTPSProxy
		}},
	}
	for _, test := range causes {
		t.Run(test.name+" before ready", func(t *testing.T) {
			newAcceleratorPortForwarder = func(httpstream.Dialer, []string, []string, chan struct{}, chan struct{}) (acceleratorPortForwarder, error) {
				return nil, test.cause
			}
			tunnel, err := StartAcceleratorPodTunnel(context.Background(), snapshot, "team-a", "pod-a")
			var rawUpgrade *httpstream.UpgradeFailureError
			if tunnel != nil || !errors.Is(err, ErrAcceleratorContextUnavailable) || !test.match(err) || errors.Unwrap(err) != nil || errors.As(err, &rawUpgrade) || strings.Contains(fmt.Sprintf("%v %+v %#v", err, err, err), "raw") {
				t.Fatalf("tunnel=%#v err=%v", tunnel, err)
			}
		})
		t.Run(test.name+" after ready", func(t *testing.T) {
			release := make(chan struct{})
			newAcceleratorPortForwarder = func(_ httpstream.Dialer, _ []string, _ []string, stop, ready chan struct{}) (acceleratorPortForwarder, error) {
				return &fakeAcceleratorForwarder{mode: "ready-block-error", ready: ready, stop: stop, release: release, failure: test.cause, ports: []portforward.ForwardedPort{{Local: 43123, Remote: 8080}}}, nil
			}
			tunnel, err := StartAcceleratorPodTunnel(context.Background(), snapshot, "team-a", "pod-a")
			if err != nil || tunnel == nil {
				t.Fatalf("start=%#v %v", tunnel, err)
			}
			close(release)
			select {
			case <-tunnel.Done():
			case <-time.After(time.Second):
				t.Fatal("forwarder failure did not close tunnel")
			}
			cause := AcceleratorPodTunnelFailure(tunnel)
			var rawUpgrade *httpstream.UpgradeFailureError
			if !test.match(cause) || errors.Unwrap(cause) != nil || errors.As(cause, &rawUpgrade) || strings.Contains(fmt.Sprintf("%v %+v %#v", cause, cause, cause), "raw") {
				t.Fatalf("terminal cause=%v", cause)
			}
		})
	}

	t.Run("generic remains typed terminal", func(t *testing.T) {
		generic := errors.New("raw generic forward failure")
		newAcceleratorPortForwarder = func(httpstream.Dialer, []string, []string, chan struct{}, chan struct{}) (acceleratorPortForwarder, error) {
			return nil, generic
		}
		_, err := StartAcceleratorPodTunnel(context.Background(), snapshot, "team-a", "pod-a")
		if errors.Is(err, generic) || errors.Unwrap(err) != nil || ClassifyAcceleratorTunnelFailure(err) != AcceleratorTunnelFailureGeneric || httpstream.IsUpgradeFailure(err) || httpstream.IsHTTPSProxyError(err) || strings.Contains(err.Error(), "raw") {
			t.Fatalf("generic cause=%v", err)
		}
	})
}

func TestAcceleratorTunnelLogsRawTransientPodStartupFailure(t *testing.T) {
	eventsCh := make(chan []interface{}, 1)
	debug.Init(events.EmitterFunc(func(name string, data ...interface{}) {
		if name == "debug:log" {
			eventsCh <- data
		}
	}))
	debug.SetEnabled(true)
	t.Cleanup(func() {
		debug.SetEnabled(false)
		debug.Init(&events.NoopEmitter{})
	})

	original := newAcceleratorPortForwarder
	t.Cleanup(func() { newAcceleratorPortForwarder = original })
	rawErr := errors.New("unable to upgrade connection: pod not found (\"pod-a_team-a\")")
	newAcceleratorPortForwarder = func(_ httpstream.Dialer, _ []string, _ []string, stop, ready chan struct{}) (acceleratorPortForwarder, error) {
		return &fakeAcceleratorForwarder{mode: "error-before-ready", ready: ready, stop: stop, failure: rawErr}, nil
	}
	snapshot := &AcceleratorContextSnapshot{restConfig: &rest.Config{Host: "https://api.example.test", TLSClientConfig: rest.TLSClientConfig{Insecure: true}}}
	tunnel, err := StartAcceleratorPodTunnel(context.Background(), snapshot, "team-a", "pod-a")
	if tunnel != nil || ClassifyAcceleratorTunnelFailure(err) != AcceleratorTunnelFailureTransient {
		t.Fatalf("tunnel=%#v error=%v class=%d", tunnel, err, ClassifyAcceleratorTunnelFailure(err))
	}

	select {
	case data := <-eventsCh:
		if len(data) != 1 {
			t.Fatalf("debug event=%#v", data)
		}
		payload, ok := data[0].(map[string]interface{})
		if !ok || payload["category"] != debug.CategoryPortforward || payload["message"] != "Accelerator Pod tunnel failed" {
			t.Fatalf("debug payload=%#v", data[0])
		}
		details, ok := payload["details"].(map[string]interface{})
		if !ok || details["stage"] != "forward_ports" || details["failureKind"] != "transient" || details["namespace"] != "team-a" || details["pod"] != "pod-a" || details["error"] != rawErr.Error() {
			t.Fatalf("debug details=%#v", payload["details"])
		}
	case <-time.After(time.Second):
		t.Fatal("raw tunnel failure was not emitted to Debug")
	}
}

func TestAcceleratorTunnelPublicFailuresNeverExposeRawCause(t *testing.T) {
	funcError := &rawAcceleratorTunnelTestError{secret: "https://user:secret@example.test/private"}
	original := newAcceleratorPortForwarder
	defer func() { newAcceleratorPortForwarder = original }()
	newAcceleratorPortForwarder = func(httpstream.Dialer, []string, []string, chan struct{}, chan struct{}) (acceleratorPortForwarder, error) {
		return nil, funcError
	}
	snapshot := &AcceleratorContextSnapshot{restConfig: &rest.Config{Host: "https://api.example.test", TLSClientConfig: rest.TLSClientConfig{Insecure: true}}}
	_, err := StartAcceleratorPodTunnel(context.Background(), snapshot, "team-a", "pod-a")
	if err == nil || errors.Unwrap(err) != nil || errors.Is(err, funcError) || strings.Contains(fmt.Sprintf("%v %+v %#v", err, err, err), "secret") {
		t.Fatalf("public error exposed raw cause: %v", err)
	}
	var raw *rawAcceleratorTunnelTestError
	if errors.As(err, &raw) {
		t.Fatal("public error allowed raw errors.As traversal")
	}
}

func TestAcceleratorTunnelReadyTimeoutSettlementClassification(t *testing.T) {
	original := newAcceleratorPortForwarder
	defer func() { newAcceleratorPortForwarder = original }()
	snapshot := &AcceleratorContextSnapshot{restConfig: &rest.Config{Host: "https://api.example.test", TLSClientConfig: rest.TLSClientConfig{Insecure: true}}}

	t.Run("unsettled is terminal cleanup", func(t *testing.T) {
		release := make(chan struct{})
		started := make(chan struct{})
		newAcceleratorPortForwarder = func(_ httpstream.Dialer, _ []string, _ []string, stop, ready chan struct{}) (acceleratorPortForwarder, error) {
			return &fakeAcceleratorForwarder{mode: "blocked", ready: ready, stop: stop, release: release, started: started}, nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()
		result := make(chan error, 1)
		go func() {
			_, err := StartAcceleratorPodTunnel(ctx, snapshot, "team-a", "pod-a")
			result <- err
		}()
		<-started
		err := <-result
		if ClassifyAcceleratorTunnelFailure(err) != AcceleratorTunnelFailureCleanupUnsettled || errors.Unwrap(err) != nil || errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("unsettled error=%v class=%d", err, ClassifyAcceleratorTunnelFailure(err))
		}
		close(release)
	})

	t.Run("settled deadline is transient", func(t *testing.T) {
		stopped := make(chan struct{})
		newAcceleratorPortForwarder = func(_ httpstream.Dialer, _ []string, _ []string, stop, ready chan struct{}) (acceleratorPortForwarder, error) {
			return &fakeAcceleratorForwarder{mode: "wait-stop-deadline", ready: ready, stop: stop, stopped: stopped}, nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()
		_, err := StartAcceleratorPodTunnel(ctx, snapshot, "team-a", "pod-a")
		if ClassifyAcceleratorTunnelFailure(err) != AcceleratorTunnelFailureTransient || errors.Unwrap(err) != nil || errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("settled error=%v class=%d", err, ClassifyAcceleratorTunnelFailure(err))
		}
		select {
		case <-stopped:
		default:
			t.Fatal("cooperative forwarder was not settled")
		}
	})
}

type dialingAcceleratorForwarder struct{ dialer httpstream.Dialer }

func (f *dialingAcceleratorForwarder) ForwardPorts() error {
	_, _, err := f.dialer.Dial(portforward.PortForwardProtocolV1Name)
	return err
}
func (*dialingAcceleratorForwarder) GetPorts() ([]portforward.ForwardedPort, error) { return nil, nil }

type failingAcceleratorDialer struct{ failure error }

func (d failingAcceleratorDialer) Dial(...string) (httpstream.Connection, string, error) {
	return nil, "", d.failure
}

type stringifyingAcceleratorForwarder struct{ dialer httpstream.Dialer }

func (f *stringifyingAcceleratorForwarder) ForwardPorts() error {
	_, _, err := f.dialer.Dial(portforward.PortForwardProtocolV1Name)
	if err != nil {
		return fmt.Errorf("client-go port forward erased dial cause: %s", err)
	}
	return nil
}
func (*stringifyingAcceleratorForwarder) GetPorts() ([]portforward.ForwardedPort, error) {
	return nil, nil
}

func TestAcceleratorTunnelDialRecorderSurvivesClientGoStringification(t *testing.T) {
	originalForwarder := newAcceleratorPortForwarder
	originalDialer := newAcceleratorFinalDialer
	defer func() {
		newAcceleratorPortForwarder = originalForwarder
		newAcceleratorFinalDialer = originalDialer
	}()
	snapshot := &AcceleratorContextSnapshot{restConfig: &rest.Config{Host: "https://api.example.test", TLSClientConfig: rest.TLSClientConfig{Insecure: true}}}
	tests := []struct {
		name string
		err  error
		kind AcceleratorTunnelFailureKind
	}{
		{name: "upgrade", err: &httpstream.UpgradeFailureError{Cause: errors.New("raw-upgrade-secret")}, kind: AcceleratorTunnelFailureUpgrade},
		{name: "https proxy", err: errors.New("proxy: unknown scheme: https raw-proxy-secret"), kind: AcceleratorTunnelFailureHTTPSProxy},
		{name: "too many requests", err: apierrors.NewTooManyRequests("raw-429-secret", 1), kind: AcceleratorTunnelFailureTransient},
		{name: "service unavailable", err: apierrors.NewServiceUnavailable("raw-503-secret"), kind: AcceleratorTunnelFailureTransient},
		{name: "gateway timeout", err: &apierrors.StatusError{ErrStatus: metav1.Status{Code: 504, Message: "raw-504-secret"}}, kind: AcceleratorTunnelFailureTransient},
		{name: "generic", err: errors.New("raw-generic-secret"), kind: AcceleratorTunnelFailureGeneric},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			newAcceleratorFinalDialer = func(httpstream.Dialer, httpstream.Dialer) httpstream.Dialer {
				return failingAcceleratorDialer{failure: test.err}
			}
			newAcceleratorPortForwarder = func(dialer httpstream.Dialer, _ []string, _ []string, _, _ chan struct{}) (acceleratorPortForwarder, error) {
				return &stringifyingAcceleratorForwarder{dialer: dialer}, nil
			}
			tunnel, err := StartAcceleratorPodTunnel(context.Background(), snapshot, "team-a", "pod-a")
			if tunnel != nil || ClassifyAcceleratorTunnelFailure(err) != test.kind || errors.Unwrap(err) != nil || errors.Is(err, test.err) || strings.Contains(fmt.Sprintf("%v %+v %#v", err, err, err), "secret") {
				t.Fatalf("tunnel=%#v err=%v kind=%d", tunnel, err, ClassifyAcceleratorTunnelFailure(err))
			}
		})
	}
}

func TestAcceleratorTunnelLostPodAfterReadyIsTransient(t *testing.T) {
	original := newAcceleratorPortForwarder
	defer func() { newAcceleratorPortForwarder = original }()
	release := make(chan struct{})
	newAcceleratorPortForwarder = func(_ httpstream.Dialer, _ []string, _ []string, stop, ready chan struct{}) (acceleratorPortForwarder, error) {
		return &fakeAcceleratorForwarder{mode: "ready-block-error", ready: ready, stop: stop, release: release, failure: portforward.ErrLostConnectionToPod, ports: []portforward.ForwardedPort{{Local: 43123, Remote: 8080}}}, nil
	}
	snapshot := &AcceleratorContextSnapshot{restConfig: &rest.Config{Host: "https://api.example.test", TLSClientConfig: rest.TLSClientConfig{Insecure: true}}}
	tunnel, err := StartAcceleratorPodTunnel(context.Background(), snapshot, "team-a", "pod-a")
	if err != nil || tunnel == nil {
		t.Fatalf("start=%#v err=%v", tunnel, err)
	}
	close(release)
	select {
	case <-tunnel.Done():
	case <-time.After(time.Second):
		t.Fatal("lost-Pod forwarder did not terminate")
	}
	if failure := AcceleratorPodTunnelFailure(tunnel); ClassifyAcceleratorTunnelFailure(failure) != AcceleratorTunnelFailureTransient || errors.Unwrap(failure) != nil || errors.Is(failure, portforward.ErrLostConnectionToPod) {
		t.Fatalf("lost-Pod failure=%v class=%d", failure, ClassifyAcceleratorTunnelFailure(failure))
	}
}

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
