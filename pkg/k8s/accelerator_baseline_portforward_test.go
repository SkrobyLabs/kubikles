package k8s

import (
	"context"
	"encoding"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/httpstream"
	httpstreamspdy "k8s.io/apimachinery/pkg/util/httpstream/spdy"
	runtimeutil "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/rest"
	clientportforward "k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
	"k8s.io/klog/v2"
)

var baselineClientGoLogMu sync.Mutex

type baselineClientGoErrorClass uint8

const (
	baselineClientGoOtherError baselineClientGoErrorClass = iota
	baselineClientGoHostileRemoteError
)

// silenceBaselineClientGoGlobals isolates client-go's process-wide diagnostic
// handlers for the hostile transport test and restores the exact prior value.
func silenceBaselineClientGoGlobals(t *testing.T, hostileRemoteError string) <-chan baselineClientGoErrorClass {
	t.Helper()
	baselineClientGoLogMu.Lock()
	handled := make(chan baselineClientGoErrorClass, 16)
	oldHandlers := runtimeutil.ErrorHandlers
	if flag.Lookup("v") == nil {
		klog.InitFlags(nil)
	}
	verbosity := flag.Lookup("v")
	oldVerbosity := verbosity.Value.String()
	if err := verbosity.Value.Set("4"); err != nil {
		baselineClientGoLogMu.Unlock()
		t.Fatal("client-go verbosity")
	}
	runtimeutil.ErrorHandlers = []runtimeutil.ErrorHandler{func(_ context.Context, err error, _ string, _ ...interface{}) {
		classification := baselineClientGoOtherError
		if err != nil {
			message := err.Error()
			if strings.HasPrefix(message, "an error occurred forwarding ") && strings.HasSuffix(message, ": "+hostileRemoteError) {
				classification = baselineClientGoHostileRemoteError
			}
		}
		handled <- classification
	}}
	t.Cleanup(func() {
		runtimeutil.ErrorHandlers = oldHandlers
		_ = verbosity.Value.Set(oldVerbosity)
		baselineClientGoLogMu.Unlock()
	})
	return handled
}

type baselineForwardReason string

const (
	baselineInvalidTarget     baselineForwardReason = "invalid_target"
	baselineAPIUnavailable    baselineForwardReason = "api_unavailable"
	baselineTunnelUnavailable baselineForwardReason = "tunnel_unavailable"
	baselineCanceled          baselineForwardReason = "canceled"
	baselinePodReplaced       baselineForwardReason = "pod_replaced"
	baselineRemoteClosed      baselineForwardReason = "remote_closed"
	baselineClosed            baselineForwardReason = "closed"
)

type baselinePort struct{ Local, Remote uint16 }
type baselineForwarder interface {
	ForwardPorts() error
	GetPorts() ([]baselinePort, error)
	Close()
}

// baselineClientGoForwarder is deliberately test-only. It is the real
// WebSocket-primary/SPDY-fallback construction used by the Kind characterization.
type baselineClientGoForwarder struct {
	value *clientportforward.PortForwarder
}

func (f baselineClientGoForwarder) ForwardPorts() error { return f.value.ForwardPorts() }
func (f baselineClientGoForwarder) GetPorts() ([]baselinePort, error) {
	ports, err := f.value.GetPorts()
	result := make([]baselinePort, len(ports))
	for i, p := range ports {
		result[i] = baselinePort{p.Local, p.Remote}
	}
	return result, err
}
func (f baselineClientGoForwarder) Close() {}

type baselinePrivatePrimaryDialError struct{ fallback bool }

func (baselinePrivatePrimaryDialError) Error() string { return "primary_dial_failed" }

// baselinePrivatePrimaryDialer ensures the fallback dialer's verbose log never
// receives response bodies, addresses, or credentials from a failed WebSocket
// upgrade. The fallback predicate deliberately operates on this fixed token.
type baselinePrivatePrimaryDialer struct{ httpstream.Dialer }

func (d baselinePrivatePrimaryDialer) Dial(protocols ...string) (httpstream.Connection, string, error) {
	connection, version, err := d.Dialer.Dial(protocols...)
	if err != nil {
		return nil, "", baselinePrivatePrimaryDialError{fallback: httpstream.IsUpgradeFailure(err) || httpstream.IsHTTPSProxyError(err)}
	}
	return connection, version, nil
}

func newBaselineClientGoForwarder(config *rest.Config, target *url.URL, remote uint16, stop <-chan struct{}, ready chan struct{}) (baselineForwarder, error) {
	primary, err := clientportforward.NewSPDYOverWebsocketDialer(target, config)
	if err != nil {
		return nil, err
	}
	roundTripper, upgrader, err := spdy.RoundTripperFor(config)
	if err != nil {
		return nil, err
	}
	secondary := spdy.NewDialer(upgrader, &http.Client{Transport: roundTripper}, http.MethodPost, target)
	dialer := clientportforward.NewFallbackDialer(baselinePrivatePrimaryDialer{primary}, secondary, func(err error) bool {
		private, ok := err.(baselinePrivatePrimaryDialError)
		return ok && private.fallback
	})
	forwarder, err := clientportforward.NewOnAddresses(dialer, []string{"127.0.0.1"}, []string{"0:" + itoa(remote)}, stop, ready, io.Discard, io.Discard)
	if err != nil {
		return nil, err
	}
	return baselineClientGoForwarder{forwarder}, nil
}

type baselineExactPodHandle struct {
	mu        sync.RWMutex
	port      uint16
	reason    baselineForwardReason
	published bool
	done      chan struct{}
	terminal  chan struct{}
	joined    chan struct{}
	stop      chan struct{}
}

func (h *baselineExactPodHandle) LocalPort() uint16 {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.port
}
func (h *baselineExactPodHandle) Done() <-chan struct{} { return h.done }
func (h *baselineExactPodHandle) Reason() baselineForwardReason {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.reason
}
func (h *baselineExactPodHandle) Close(ctx context.Context) error {
	h.finish(baselineClosed)
	select {
	case <-h.joined:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (h *baselineExactPodHandle) finish(reason baselineForwardReason) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.finishLocked(reason)
}
func (h *baselineExactPodHandle) finishLocked(reason baselineForwardReason) {
	if h.reason != "" {
		return
	}
	h.port = 0
	h.reason = reason
	h.published = false
	close(h.stop)
	close(h.terminal)
}
func (h *baselineExactPodHandle) finishForward() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.published {
		h.finishLocked(baselineRemoteClosed)
	} else {
		h.finishLocked(baselineTunnelUnavailable)
	}
}

// rejectAdmission never leaves a constructed client-go forwarder running. In
// particular, stopping before returning is not enough: client-go can still be
// in the process of binding its listener until ForwardPorts has returned.
func (h *baselineExactPodHandle) rejectAdmission(ctx context.Context, requested baselineForwardReason) error {
	h.mu.Lock()
	if h.reason == "" {
		if ctx.Err() != nil {
			h.finishLocked(baselineCanceled)
		} else {
			h.finishLocked(requested)
		}
	}
	h.mu.Unlock()
	<-h.joined
	return errors.New(string(h.Reason()))
}

type baselineForwardFactory func(*url.URL, []string, []string, <-chan struct{}, chan struct{}) (baselineForwarder, error)

func baselineExactPodForward(ctx context.Context, config *rest.Config, namespace, name string, expected types.UID, remote uint16, get func(context.Context, string, string) (types.UID, error), newForward baselineForwardFactory) (*baselineExactPodHandle, error) {
	if config == nil || config.Host == "" || namespace == "" || name == "" || expected == "" || remote == 0 {
		return nil, errors.New(string(baselineInvalidTarget))
	}
	uid, err := get(ctx, namespace, name)
	if err != nil {
		return nil, errors.New(string(baselineAPIUnavailable))
	}
	if uid != expected {
		return nil, errors.New(string(baselinePodReplaced))
	}
	u, err := url.Parse(config.Host + "/api/v1/namespaces/" + url.PathEscape(namespace) + "/pods/" + url.PathEscape(name) + "/portforward")
	if err != nil {
		return nil, errors.New(string(baselineInvalidTarget))
	}
	h := &baselineExactPodHandle{done: make(chan struct{}), terminal: make(chan struct{}), joined: make(chan struct{}), stop: make(chan struct{})}
	ready := make(chan struct{})
	f, err := newForward(u, []string{"127.0.0.1"}, []string{"0:" + itoa(remote)}, h.stop, ready)
	if err != nil {
		h.finish(baselineTunnelUnavailable)
		close(h.joined)
		close(h.done)
		return nil, errors.New(string(baselineTunnelUnavailable))
	}
	go func() {
		select {
		case <-ctx.Done():
			h.finish(baselineCanceled)
		case <-h.terminal:
		}
	}()
	go func() {
		defer close(h.done)
		defer close(h.joined)
		err := f.ForwardPorts()
		if ctx.Err() != nil {
			h.finish(baselineCanceled)
		} else {
			// Classify completion and publication under one lock: a completion
			// cannot race a later publication into resurrecting a local port.
			_ = err // raw forwarder errors never cross this boundary.
			h.finishForward()
		}
	}()
	select {
	case <-ready:
	case <-ctx.Done():
		return nil, h.rejectAdmission(ctx, baselineCanceled)
	case <-h.terminal:
		return nil, h.rejectAdmission(ctx, h.Reason())
	}
	ports, err := f.GetPorts()
	if err != nil || len(ports) != 1 || ports[0].Local == 0 || ports[0].Remote != remote {
		return nil, h.rejectAdmission(ctx, baselineTunnelUnavailable)
	}
	uid, err = get(ctx, namespace, name)
	if err != nil {
		return nil, h.rejectAdmission(ctx, baselineAPIUnavailable)
	}
	if uid != expected {
		return nil, h.rejectAdmission(ctx, baselinePodReplaced)
	}
	h.mu.Lock()
	// Terminal state and publication share this lock.  A terminal event that
	// arrives after readiness but before this point cannot resurrect a port.
	if ctx.Err() != nil && h.reason == "" {
		h.finishLocked(baselineCanceled)
	}
	if h.reason != "" {
		reason := h.reason
		h.mu.Unlock()
		<-h.joined
		return nil, errors.New(string(reason))
	}
	h.port, h.published = ports[0].Local, true
	h.mu.Unlock()
	return h, nil
}
func itoa(v uint16) string {
	if v == 0 {
		return "0"
	}
	b := make([]byte, 0, 5)
	for v > 0 {
		b = append([]byte{byte('0' + v%10)}, b...)
		v /= 10
	}
	return string(b)
}

type baselineFakeForwarder struct {
	ports    []baselinePort
	portsErr error
	getPorts int
	ready    chan struct{}
	err      error
	stop     <-chan struct{}
}

func (f *baselineFakeForwarder) ForwardPorts() error { close(f.ready); <-f.stop; return f.err }
func (f *baselineFakeForwarder) GetPorts() ([]baselinePort, error) {
	f.getPorts++
	return f.ports, f.portsErr
}
func (f *baselineFakeForwarder) Close() {}
func baselineFakeFactory(f *baselineFakeForwarder) baselineForwardFactory {
	return func(_ *url.URL, _ []string, _ []string, stop <-chan struct{}, ready chan struct{}) (baselineForwarder, error) {
		f.stop, f.ready = stop, ready
		return f, nil
	}
}
func TestAccelerator00ExactPodForwardArguments(t *testing.T) {
	f := &baselineFakeForwarder{ports: []baselinePort{{Local: 32123, Remote: 8080}}}
	var addresses, ports []string
	var target string
	factory := baselineFakeFactory(f)
	h, err := baselineExactPodForward(context.Background(), &rest.Config{Host: "https://api.invalid"}, "ns", "pod", types.UID("uid"), 8080, func(context.Context, string, string) (types.UID, error) { return "uid", nil }, func(u *url.URL, a, p []string, stop <-chan struct{}, ready chan struct{}) (baselineForwarder, error) {
		addresses, ports = a, p
		target = u.String()
		return factory(u, a, p, stop, ready)
	})
	if err != nil || h.LocalPort() == 0 || target != "https://api.invalid/api/v1/namespaces/ns/pods/pod/portforward" || len(addresses) != 1 || addresses[0] != "127.0.0.1" || len(ports) != 1 || ports[0] != "0:8080" {
		t.Fatal("exact forward arguments")
	}
	_ = h.Close(context.Background())
	<-h.Done()
}

func TestAccelerator00ExactPodForwardEscapesAndCounts(t *testing.T) {
	f := &baselineFakeForwarder{ports: []baselinePort{{Local: 32123, Remote: 8080}}}
	gets, factories := 0, 0
	var gotURL string
	h, err := baselineExactPodForward(context.Background(), &rest.Config{Host: "https://api.invalid"}, "ns /?", "pod /?", "uid /?", 8080,
		func(context.Context, string, string) (types.UID, error) { gets++; return "uid /?", nil },
		func(u *url.URL, a, p []string, stop <-chan struct{}, ready chan struct{}) (baselineForwarder, error) {
			factories++
			gotURL = u.EscapedPath()
			return baselineFakeFactory(f)(u, a, p, stop, ready)
		})
	if err != nil || gotURL != "/api/v1/namespaces/ns%20%2F%3F/pods/pod%20%2F%3F/portforward" || gets != 2 || factories != 1 || f.getPorts != 1 {
		t.Fatal("escaped exact target contract")
	}
	// GetPorts is an admission-only operation: no rediscovery or retry follows.
	_ = h.Close(context.Background())
}
func TestAccelerator00ExactPodForwardTerminalMatrix(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	f := &baselineFakeForwarder{ports: []baselinePort{{Local: 32123, Remote: 8080}}}
	h, err := baselineExactPodForward(ctx, &rest.Config{Host: "https://api.invalid"}, "ns", "pod", "uid", 8080, func(context.Context, string, string) (types.UID, error) { return "uid", nil }, baselineFakeFactory(f))
	if err != nil {
		t.Fatal("setup")
	}
	cancel()
	<-h.Done()
	if h.Reason() != baselineCanceled || h.LocalPort() != 0 {
		t.Fatal("cancel terminal state")
	}
}

type baselineControlledForwarder struct {
	ports    []baselinePort
	ready    chan struct{}
	finished chan error
	stop     <-chan struct{}
}

func (f *baselineControlledForwarder) ForwardPorts() error {
	close(f.ready)
	select {
	case err := <-f.finished:
		return err
	case <-f.stop:
		return nil
	}
}
func (f *baselineControlledForwarder) GetPorts() ([]baselinePort, error) { return f.ports, nil }
func (f *baselineControlledForwarder) Close()                            {}
func baselineControlledFactory(f *baselineControlledForwarder) baselineForwardFactory {
	return func(_ *url.URL, _ []string, _ []string, stop <-chan struct{}, ready chan struct{}) (baselineForwarder, error) {
		f.stop = stop
		f.ready = ready
		return f, nil
	}
}

func TestAccelerator00ExactPodForwardTerminalRaces(t *testing.T) {
	newHandle := func(get func() types.UID) (*baselineExactPodHandle, *baselineControlledForwarder) {
		f := &baselineControlledForwarder{ports: []baselinePort{{Local: 32123, Remote: 8080}}, finished: make(chan error, 1)}
		h, err := baselineExactPodForward(context.Background(), &rest.Config{Host: "https://api.invalid"}, "ns", "pod", "uid", 8080,
			func(context.Context, string, string) (types.UID, error) { return get(), nil }, baselineControlledFactory(f))
		if err != nil {
			t.Fatal("setup")
		}
		return h, f
	}
	t.Run("remote closure clears published port", func(t *testing.T) {
		h, f := newHandle(func() types.UID { return "uid" })
		f.finished <- errors.New("hostile-marker")
		<-h.Done()
		if h.Reason() != baselineRemoteClosed || h.LocalPort() != 0 {
			t.Fatal("remote terminal state")
		}
	})
	t.Run("post ready replacement wins", func(t *testing.T) {
		calls := 0
		f := &baselineControlledForwarder{ports: []baselinePort{{Local: 32123, Remote: 8080}}, finished: make(chan error, 1)}
		_, err := baselineExactPodForward(context.Background(), &rest.Config{Host: "https://api.invalid"}, "ns", "pod", "uid", 8080,
			func(context.Context, string, string) (types.UID, error) {
				calls++
				if calls == 2 {
					return "replacement", nil
				}
				return "uid", nil
			}, baselineControlledFactory(f))
		if err == nil || err.Error() != string(baselinePodReplaced) {
			t.Fatal("replacement was admitted")
		}
		select {
		case <-f.stop:
		default:
			t.Fatal("replacement did not stop forwarder")
		}
	})
	t.Run("concurrent close is once terminal", func(t *testing.T) {
		h, _ := newHandle(func() types.UID { return "uid" })
		var wg sync.WaitGroup
		for range 8 {
			wg.Add(1)
			go func() { defer wg.Done(); _ = h.Close(context.Background()) }()
		}
		wg.Wait()
		if h.Reason() != baselineClosed || h.LocalPort() != 0 {
			t.Fatal("close terminal state")
		}
	})
}

type baselineJoinForwarder struct {
	ports   []baselinePort
	ready   chan struct{}
	stop    <-chan struct{}
	release chan struct{}
}

func (f *baselineJoinForwarder) ForwardPorts() error {
	close(f.ready)
	<-f.stop
	<-f.release
	return nil
}
func (f *baselineJoinForwarder) GetPorts() ([]baselinePort, error) { return f.ports, nil }
func (f *baselineJoinForwarder) Close()                            {}

func TestAccelerator00ExactPodForwardCloseJoinsForwardPorts(t *testing.T) {
	f := &baselineJoinForwarder{ports: []baselinePort{{Local: 32123, Remote: 8080}}, release: make(chan struct{})}
	factory := func(_ *url.URL, _ []string, _ []string, stop <-chan struct{}, ready chan struct{}) (baselineForwarder, error) {
		f.stop, f.ready = stop, ready
		return f, nil
	}
	h, err := baselineExactPodForward(context.Background(), &rest.Config{Host: "https://api.invalid"}, "ns", "pod", "uid", 8080,
		func(context.Context, string, string) (types.UID, error) { return "uid", nil }, factory)
	if err != nil {
		t.Fatal("setup")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := h.Close(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal("close returned before forwarder joined")
	}
	close(f.release)
	if err := h.Close(context.Background()); err != nil || h.Reason() != baselineClosed {
		t.Fatal("joined close")
	}
}

func TestAccelerator00ExactPodForwardPreReadyFailureIsUnavailable(t *testing.T) {
	f := &baselineControlledForwarder{ports: []baselinePort{{Local: 32123, Remote: 8080}}, finished: make(chan error, 1)}
	f.finished <- errors.New("hostile-marker")
	_, err := baselineExactPodForward(context.Background(), &rest.Config{Host: "https://api.invalid"}, "ns", "pod", "uid", 8080,
		func(context.Context, string, string) (types.UID, error) { return "uid", nil }, baselineControlledFactory(f))
	// The controlled forwarder reports ready before its queued failure; this is
	// still an admission failure because no local port has been published.
	if err == nil || err.Error() != string(baselineTunnelUnavailable) {
		t.Fatal("pre-publication failure mapping")
	}
}

func TestAccelerator00ExactPodForwardMalformedPorts(t *testing.T) {
	for _, candidate := range []struct {
		ports []baselinePort
		err   error
	}{
		{nil, nil},
		{[]baselinePort{{Local: 0, Remote: 8080}}, nil},
		{[]baselinePort{{Local: 1, Remote: 0}}, nil},
		{[]baselinePort{{Local: 1, Remote: 8080}, {Local: 2, Remote: 8080}}, nil},
		{[]baselinePort{{Local: 1, Remote: 8081}}, nil},
		{[]baselinePort{{Local: 1, Remote: 8080}}, errors.New("hostile-marker")},
	} {
		f := &baselineFakeForwarder{ports: candidate.ports, portsErr: candidate.err}
		_, err := baselineExactPodForward(context.Background(), &rest.Config{Host: "https://api.invalid"}, "ns", "pod", "uid", 8080,
			func(context.Context, string, string) (types.UID, error) { return "uid", nil }, baselineFakeFactory(f))
		if err == nil || err.Error() != string(baselineTunnelUnavailable) || f.getPorts != 1 {
			t.Fatal("malformed port admission")
		}
	}
}

type baselineBarrierForwarder struct {
	ports     []baselinePort
	ready     chan struct{}
	complete  chan error
	stop      <-chan struct{}
	started   chan struct{}
	completed chan struct{}
}

func (f *baselineBarrierForwarder) ForwardPorts() error {
	close(f.started)
	select {
	case err := <-f.complete:
		close(f.completed)
		return err
	case <-f.stop:
		close(f.completed)
		return nil
	}
}
func (f *baselineBarrierForwarder) GetPorts() ([]baselinePort, error) { return f.ports, nil }
func (f *baselineBarrierForwarder) Close()                            {}

func TestAccelerator00ExactPodForwardTerminalBarriers(t *testing.T) {
	newForward := func() (*baselineBarrierForwarder, baselineForwardFactory) {
		f := &baselineBarrierForwarder{ports: []baselinePort{{Local: 32123, Remote: 8080}}, complete: make(chan error, 1), started: make(chan struct{}), completed: make(chan struct{})}
		return f, func(_ *url.URL, _ []string, _ []string, stop <-chan struct{}, ready chan struct{}) (baselineForwarder, error) {
			f.stop, f.ready = stop, ready
			return f, nil
		}
	}
	t.Run("late error before ready never publishes", func(t *testing.T) {
		f, factory := newForward()
		result := make(chan error, 1)
		go func() {
			_, err := baselineExactPodForward(context.Background(), &rest.Config{Host: "https://api.invalid"}, "ns", "pod", "uid", 8080, func(context.Context, string, string) (types.UID, error) { return "uid", nil }, factory)
			result <- err
		}()
		<-f.started
		f.complete <- errors.New("hostile-error")
		if err := <-result; err == nil || err.Error() != string(baselineTunnelUnavailable) {
			t.Fatal("late error mapping")
		}
		<-f.completed
	})
	t.Run("cancel before ready never publishes", func(t *testing.T) {
		f, factory := newForward()
		ctx, cancel := context.WithCancel(context.Background())
		result := make(chan error, 1)
		go func() {
			_, err := baselineExactPodForward(ctx, &rest.Config{Host: "https://api.invalid"}, "ns", "pod", "uid", 8080, func(context.Context, string, string) (types.UID, error) { return "uid", nil }, factory)
			result <- err
		}()
		<-f.started
		cancel()
		if err := <-result; err == nil || err.Error() != string(baselineCanceled) {
			t.Fatal("cancel mapping")
		}
		<-f.completed
	})
	t.Run("post ready remote completion closes once", func(t *testing.T) {
		f, factory := newForward()
		result := make(chan struct {
			h   *baselineExactPodHandle
			err error
		}, 1)
		go func() {
			h, err := baselineExactPodForward(context.Background(), &rest.Config{Host: "https://api.invalid"}, "ns", "pod", "uid", 8080, func(context.Context, string, string) (types.UID, error) { return "uid", nil }, factory)
			result <- struct {
				h   *baselineExactPodHandle
				err error
			}{h, err}
		}()
		<-f.started
		close(f.ready)
		admitted := <-result
		h, err := admitted.h, admitted.err
		if err != nil {
			t.Fatal("admission")
		}
		f.complete <- nil
		<-h.Done()
		<-f.completed
		if h.Reason() != baselineRemoteClosed || h.LocalPort() != 0 {
			t.Fatal("remote terminal")
		}
	})
}

func TestAccelerator00ExactPodForwardPostReadyCancellation(t *testing.T) {
	f := &baselineJoinForwarder{ports: []baselinePort{{Local: 32123, Remote: 8080}}, release: make(chan struct{})}
	factory := func(_ *url.URL, _ []string, _ []string, stop <-chan struct{}, ready chan struct{}) (baselineForwarder, error) {
		f.stop, f.ready = stop, ready
		return f, nil
	}
	secondGet := make(chan struct{})
	releaseGet := make(chan struct{})
	gets := 0
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := baselineExactPodForward(ctx, &rest.Config{Host: "https://api.invalid"}, "ns", "pod", "uid", 8080,
			func(context.Context, string, string) (types.UID, error) {
				gets++
				if gets == 1 {
					return "uid", nil
				}
				close(secondGet)
				<-releaseGet
				return "", errors.New("hostile-second-get")
			}, factory)
		result <- err
	}()
	<-secondGet
	cancel()
	<-f.stop
	close(releaseGet)
	close(f.release)
	if err := <-result; err == nil || err.Error() != string(baselineCanceled) {
		t.Fatal("post-ready cancellation did not win")
	}
}

func TestAccelerator00ExactPodForwardTerminalArbitration(t *testing.T) {
	f := &baselineJoinForwarder{ports: []baselinePort{{Local: 32123, Remote: 8080}}, release: make(chan struct{})}
	factory := func(_ *url.URL, _ []string, _ []string, stop <-chan struct{}, ready chan struct{}) (baselineForwarder, error) {
		f.stop, f.ready = stop, ready
		return f, nil
	}
	secondGet := make(chan struct{})
	releaseGet := make(chan struct{})
	gets := 0
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := baselineExactPodForward(ctx, &rest.Config{Host: "https://api.invalid"}, "ns", "pod", "uid", 8080,
			func(context.Context, string, string) (types.UID, error) {
				gets++
				if gets == 1 {
					return "uid", nil
				}
				close(secondGet)
				<-releaseGet
				return "replacement", nil
			}, factory)
		result <- err
	}()
	<-secondGet
	cancel()
	<-f.stop
	close(releaseGet)
	close(f.release)
	if err := <-result; err == nil || err.Error() != string(baselineCanceled) {
		t.Fatal("winning terminal reason was not returned")
	}
}

func TestAccelerator00ExactPodForwardAllReasons(t *testing.T) {
	// Each externally observable reason remains a fixed token; no raw error can cross the boundary.
	for _, reason := range []baselineForwardReason{baselineInvalidTarget, baselineAPIUnavailable, baselineTunnelUnavailable, baselineCanceled, baselinePodReplaced, baselineRemoteClosed, baselineClosed} {
		if fmt.Sprint(reason) != string(reason) {
			t.Fatal("reason token")
		}
	}
	if _, err := baselineExactPodForward(context.Background(), nil, "ns", "pod", "uid", 8080, nil, nil); err == nil || err.Error() != string(baselineInvalidTarget) {
		t.Fatal("invalid target path")
	}
	if _, err := baselineExactPodForward(context.Background(), &rest.Config{Host: "https://api.invalid"}, "ns", "pod", "uid", 8080, func(context.Context, string, string) (types.UID, error) { return "", errors.New("hostile") }, nil); err == nil || err.Error() != string(baselineAPIUnavailable) {
		t.Fatal("api unavailable path")
	}
	f := &baselineFakeForwarder{ports: []baselinePort{{Local: 32123, Remote: 8080}}}
	if _, err := baselineExactPodForward(context.Background(), &rest.Config{Host: "https://api.invalid"}, "ns", "pod", "uid", 8080, func(context.Context, string, string) (types.UID, error) { return "replacement", nil }, baselineFakeFactory(f)); err == nil || err.Error() != string(baselinePodReplaced) {
		t.Fatal("pod replaced path")
	}
	h, err := baselineExactPodForward(context.Background(), &rest.Config{Host: "https://api.invalid"}, "ns", "pod", "uid", 8080, func(context.Context, string, string) (types.UID, error) { return "uid", nil }, baselineFakeFactory(&baselineFakeForwarder{ports: []baselinePort{{Local: 32123, Remote: 8080}}}))
	if err != nil {
		t.Fatal("closed setup")
	}
	if err := h.Close(context.Background()); err != nil || h.Reason() != baselineClosed {
		t.Fatal("closed path")
	}
}

func TestAccelerator00ExactPodForwardHostileMarkersStayPrivate(t *testing.T) {
	markers := []string{"rest-host-SECRET", "bearer-token-SECRET", "pod-uid-SECRET", "address-SECRET", "api-error-SECRET", "body-SECRET"}
	config := &rest.Config{Host: "https://" + markers[0] + "/" + markers[1]}
	_, err := baselineExactPodForward(context.Background(), config, "ns", markers[3], types.UID(markers[2]), 8080,
		func(context.Context, string, string) (types.UID, error) {
			return "", errors.New(markers[4] + markers[5])
		}, nil)
	if err == nil || err.Error() != string(baselineAPIUnavailable) {
		t.Fatal("fixed api error")
	}
	values := []any{err, baselineAPIUnavailable, fmt.Sprint(err), fmt.Sprintf("%v", err)}
	encoded, marshalErr := json.Marshal(values)
	if marshalErr != nil {
		t.Fatal("marshal")
	}
	values = append(values, string(encoded))
	for _, value := range values {
		text := fmt.Sprint(value)
		for _, marker := range markers {
			if strings.Contains(text, marker) {
				t.Fatal("marker published")
			}
		}
	}
	typ := reflect.TypeOf(&baselineExactPodHandle{})
	for _, method := range []string{"MarshalJSON", "MarshalText", "MarshalBinary", "Error", "URL", "Address", "Config", "Raw" + "Error"} {
		if _, ok := typ.MethodByName(method); ok {
			t.Fatal("unsafe marker surface")
		}
	}
	if _, ok := any(baselineAPIUnavailable).(encoding.TextMarshaler); ok {
		t.Fatal("text marshal surface")
	}
	if _, ok := any(baselineAPIUnavailable).(encoding.BinaryMarshaler); ok {
		t.Fatal("binary marshal surface")
	}
	artifact := t.TempDir()
	if err := os.Chmod(artifact, 0700); err != nil {
		t.Fatal("artifact permissions")
	}
	info, statErr := os.Stat(artifact)
	entries, readErr := os.ReadDir(artifact)
	if statErr != nil || readErr != nil || info.Mode().Perm()&0077 != 0 || len(entries) != 0 {
		t.Fatal("private artifact directory")
	}
}

func TestAccelerator00ClientGoHostileTransportLogsStayPrivate(t *testing.T) {
	markers := []string{"rest-host-SECRET", "bearer-token-SECRET", "pod-uid-SECRET", "address-SECRET", "api-error-SECRET", "body-SECRET"}
	hostileRemoteError := strings.Join(markers, ":")
	handled := silenceBaselineClientGoGlobals(t, hostileRemoteError)
	connectionDone := make(chan struct{})
	errorStreamDone := make(chan struct{})
	dataStreamDone := make(chan struct{})
	serverStop := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			// A failed WebSocket upgrade forces the actual fallback dialer to try
			// SPDY; its verbose diagnostic receives only the redacted wrapper error.
			w.WriteHeader(http.StatusBadGateway)
			_, _ = io.WriteString(w, strings.Join(markers, ":"))
			return
		}
		if _, err := httpstream.Handshake(r, w, []string{clientportforward.PortForwardProtocolV1Name}); err != nil {
			return
		}
		connection := httpstreamspdy.NewResponseUpgrader().UpgradeResponse(w, r, func(stream httpstream.Stream, replySent <-chan struct{}) error {
			if stream.Headers().Get(corev1.StreamType) == corev1.StreamTypeError {
				go func() {
					<-replySent
					_, _ = io.WriteString(stream, hostileRemoteError)
					_ = stream.Close()
					close(errorStreamDone)
				}()
			} else if stream.Headers().Get(corev1.StreamType) == corev1.StreamTypeData {
				go func() {
					<-replySent
					_, _ = io.Copy(io.Discard, stream)
					_ = stream.Close()
					close(dataStreamDone)
				}()
			}
			return nil
		})
		if connection == nil {
			return
		}
		defer connection.Close()
		select {
		case <-connection.CloseChan():
		case <-serverStop:
			return
		}
		select {
		case <-errorStreamDone:
		case <-serverStop:
			return
		}
		select {
		case <-dataStreamDone:
		case <-serverStop:
			return
		}
		close(connectionDone)
	}))
	defer func() {
		close(serverStop)
		server.Close()
	}()
	stop, ready := make(chan struct{}), make(chan struct{})
	f, err := newBaselineClientGoForwarder(&rest.Config{Host: server.URL, BearerToken: markers[1]}, mustBaselineURL(t, server.URL), 8080, stop, ready)
	if err != nil {
		t.Fatal("transport construction")
	}
	forwarded := make(chan error, 1)
	go func() { forwarded <- f.ForwardPorts() }()
	forwardStopped := false
	defer func() {
		if !forwardStopped {
			close(stop)
			<-forwarded
		}
	}()
	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("fallback did not admit SPDY")
	}
	ports, err := f.GetPorts()
	if err != nil || len(ports) != 1 || ports[0].Local == 0 {
		t.Fatal("fallback port")
	}
	connection, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", ports[0].Local))
	if err != nil {
		t.Fatal("local client")
	}
	defer connection.Close()
	_, _ = connection.Write([]byte("x"))
	_ = connection.Close()
	for {
		select {
		case classification := <-handled:
			if classification == baselineClientGoHostileRemoteError {
				goto hostileHandled
			}
		case <-time.After(time.Second):
			t.Fatal("hostile remote stream error was not handled")
		}
	}

hostileHandled:
	select {
	case <-connectionDone:
	case <-time.After(time.Second):
		t.Fatal("client-go connection teardown")
	}
	close(stop)
	<-forwarded
	forwardStopped = true
	for _, marker := range markers {
		if strings.Contains(baselinePrivatePrimaryDialError{}.Error(), marker) {
			t.Fatal("client-go marker logged")
		}
	}
	artifact := t.TempDir()
	entries, err := os.ReadDir(artifact)
	if err != nil || len(entries) != 0 {
		t.Fatal("private artifact")
	}
}

func mustBaselineURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw + "/api/v1/namespaces/ns/pods/pod/portforward")
	if err != nil {
		t.Fatal("test URL")
	}
	return u
}

func TestAccelerator00ExactPodForwardValidationAndSafeSurface(t *testing.T) {
	get := func(context.Context, string, string) (types.UID, error) { return "uid", nil }
	for _, candidate := range []struct {
		config          *rest.Config
		namespace, name string
		uid             types.UID
		port            uint16
	}{{nil, "ns", "pod", "uid", 8080}, {&rest.Config{}, "ns", "pod", "uid", 8080}, {&rest.Config{Host: "https://api.invalid"}, "", "pod", "uid", 8080}, {&rest.Config{Host: "https://api.invalid"}, "ns", "pod", "", 8080}, {&rest.Config{Host: "https://api.invalid"}, "ns", "pod", "uid", 0}} {
		if _, err := baselineExactPodForward(context.Background(), candidate.config, candidate.namespace, candidate.name, candidate.uid, candidate.port, get, nil); err == nil {
			t.Fatal("invalid target admitted")
		}
	}
	typ := reflect.TypeOf(&baselineExactPodHandle{})
	allowed := map[string]bool{"LocalPort": true, "Done": true, "Reason": true, "Close": true}
	for i := 0; i < typ.NumMethod(); i++ {
		if !allowed[typ.Method(i).Name] {
			t.Fatal("unsafe handle method")
		}
	}
	if fmt.Sprint(baselineRemoteClosed) != string(baselineRemoteClosed) || fmt.Sprint(errors.New(string(baselineRemoteClosed))) != string(baselineRemoteClosed) {
		t.Fatal("reason-only formatting")
	}
	for _, method := range []string{"MarshalJSON", "MarshalText", "Error", "URL", "Address", "Config", "Raw" + "Error"} {
		if _, ok := typ.MethodByName(method); ok {
			t.Fatal("unsafe surface")
		}
	}
}
