package k8s

// This is intentionally separate from the user-facing port forward manager:
// it has no persistence, retry, logging, or configurable address.

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"path"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/util/httpstream"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

const acceleratorTunnelCleanupTimeout = 5 * time.Second

type AcceleratorPodTunnel struct {
	port int
	stop chan struct{}
	done chan struct{}
	once sync.Once
}

type acceleratorSealedContext struct{ context.Context }

func (acceleratorSealedContext) Value(any) any { return nil }

type acceleratorRoundTripper func(*http.Request) (*http.Response, error)

func (f acceleratorRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type acceleratorPortForwarder interface {
	ForwardPorts() error
	GetPorts() ([]portforward.ForwardedPort, error)
}

var newAcceleratorPortForwarder = func(dialer httpstream.Dialer, addresses, ports []string, stop, ready chan struct{}) (acceleratorPortForwarder, error) {
	return portforward.NewOnAddresses(dialer, addresses, ports, stop, ready, io.Discard, io.Discard)
}

func (t *AcceleratorPodTunnel) Port() int {
	if t == nil {
		return 0
	}
	return t.port
}
func (t *AcceleratorPodTunnel) Done() <-chan struct{} {
	if t == nil {
		return nil
	}
	return t.done
}
func (t *AcceleratorPodTunnel) Close() {
	if t != nil {
		t.Stop()
		ctx, cancel := context.WithTimeout(context.Background(), acceleratorTunnelCleanupTimeout)
		_ = t.Wait(ctx)
		cancel()
	}
}

func (t *AcceleratorPodTunnel) Stop() {
	if t != nil {
		t.once.Do(func() { close(t.stop) })
	}
}

func (t *AcceleratorPodTunnel) Wait(ctx context.Context) error {
	if t == nil || ctx == nil {
		return ErrAcceleratorContextUnavailable
	}
	select {
	case <-t.done:
		return nil
	case <-ctx.Done():
		return ErrAcceleratorContextUnavailable
	}
}

func StartAcceleratorPodTunnel(ctx context.Context, snapshot *AcceleratorContextSnapshot, namespace, pod string) (*AcceleratorPodTunnel, error) {
	if ctx == nil || snapshot == nil || namespace == "" || pod == "" {
		return nil, ErrAcceleratorContextUnavailable
	}
	ctx = acceleratorSealedContext{ctx}
	cfg := snapshot.RESTConfig()
	if cfg == nil {
		return nil, ErrAcceleratorContextUnavailable
	}
	// Bind every SPDY/WebSocket request to the sealed operation context while
	// retaining all auth and caller-installed transport wrappers.
	previousWrap := cfg.WrapTransport
	cfg.WrapTransport = func(rt http.RoundTripper) http.RoundTripper {
		// The pre-existing wrapper remains in the chain, but it must receive
		// the sealed request as well.  In particular, a caller-provided
		// httptrace or wrapper cannot observe authentication added downstream.
		if previousWrap != nil {
			rt = previousWrap(rt)
		}
		return acceleratorRoundTripper(func(request *http.Request) (*http.Response, error) {
			return rt.RoundTrip(request.Clone(ctx))
		})
	}
	u, err := acceleratorPortForwardURL(cfg.Host, namespace, pod)
	if err != nil {
		return nil, ErrAcceleratorContextUnavailable
	}
	primary, err := portforward.NewSPDYOverWebsocketDialer(u, cfg)
	if err != nil {
		return nil, ErrAcceleratorContextUnavailable
	}
	rt, upgrader, err := spdy.RoundTripperFor(cfg)
	if err != nil {
		return nil, ErrAcceleratorContextUnavailable
	}
	secondary := spdy.NewDialer(upgrader, &http.Client{Transport: rt}, "POST", u)
	dialer := portforward.NewFallbackDialer(primary, secondary, acceleratorTunnelFallback)
	stop, ready, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	pf, err := newAcceleratorPortForwarder(dialer, []string{"127.0.0.1"}, []string{"0:8080"}, stop, ready)
	if err != nil {
		return nil, ErrAcceleratorContextUnavailable
	}
	var publicationMu sync.Mutex
	forwardFinished := false
	go func() {
		_ = pf.ForwardPorts()
		publicationMu.Lock()
		forwardFinished = true
		close(done)
		publicationMu.Unlock()
	}()
	var stopOnce sync.Once
	stopForwarding := func() { stopOnce.Do(func() { close(stop) }) }
	waitForForwarding := func() {
		wait, waitCancel := context.WithTimeout(context.Background(), acceleratorTunnelCleanupTimeout)
		defer waitCancel()
		select {
		case <-done:
		case <-wait.Done():
		}
	}
	select {
	case <-ctx.Done():
		// Closing stop and the sealed request context interrupts both transport
		// variants.  Do not wait here: a blocked transport must not turn caller
		// cancellation into an arbitrary multi-second delay.
		stopForwarding()
		return nil, ErrAcceleratorContextUnavailable
	case <-ready:
	case <-done:
		return nil, ErrAcceleratorContextUnavailable
	}
	select {
	case <-done:
		return nil, ErrAcceleratorContextUnavailable
	default:
	}
	ports, err := pf.GetPorts()
	if err != nil || len(ports) != 1 || ports[0].Local == 0 || ports[0].Remote != 8080 {
		stopForwarding()
		waitForForwarding()
		return nil, ErrAcceleratorContextUnavailable
	}
	// Completion and publication share this lock: a forwarder that has already
	// terminated can never be returned as a live tunnel, while a returned
	// tunnel owns any later termination through Done.
	publicationMu.Lock()
	if forwardFinished {
		publicationMu.Unlock()
		return nil, ErrAcceleratorContextUnavailable
	}
	tunnel := &AcceleratorPodTunnel{port: int(ports[0].Local), stop: stop, done: done}
	publicationMu.Unlock()
	return tunnel, nil
}

func acceleratorPortForwardURL(host, namespace, pod string) (*url.URL, error) {
	target, err := url.Parse(host)
	if err != nil || target.Scheme == "" || target.Host == "" || namespace == "" || pod == "" {
		return nil, ErrAcceleratorContextUnavailable
	}
	basePath := target.Path
	target.Path = path.Join(basePath, "api", "v1", "namespaces", namespace, "pods", pod, "portforward")
	target.RawPath = path.Join(basePath, "api", "v1", "namespaces", url.PathEscape(namespace), "pods", url.PathEscape(pod), "portforward")
	return target, nil
}

func acceleratorTunnelFallback(err error) bool {
	return httpstream.IsUpgradeFailure(err) || httpstream.IsHTTPSProxyError(err)
}
