package k8s

// This is intentionally separate from the user-facing port forward manager:
// it has no persistence, retry, or configurable address.

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"kubikles/pkg/debug"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/httpstream"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

const acceleratorTunnelCleanupTimeout = 5 * time.Second
const acceleratorTunnelCancellationSettlementTimeout = 100 * time.Millisecond

type AcceleratorPodTunnel struct {
	port     int
	stop     chan struct{}
	done     chan struct{}
	once     sync.Once
	mu       sync.RWMutex
	kind     AcceleratorTunnelFailureKind
	dialKind AcceleratorTunnelFailureKind
}

type AcceleratorTunnelFailureKind uint8

const (
	AcceleratorTunnelFailureGeneric AcceleratorTunnelFailureKind = iota + 1
	AcceleratorTunnelFailureUpgrade
	AcceleratorTunnelFailureHTTPSProxy
	AcceleratorTunnelFailureTransient
	AcceleratorTunnelFailureCleanupUnsettled
)

type acceleratorTunnelFailure struct{ kind AcceleratorTunnelFailureKind }

func (acceleratorTunnelFailure) Error() string { return "accelerator tunnel unavailable" }
func (e acceleratorTunnelFailure) Is(target error) bool {
	return target == ErrAcceleratorContextUnavailable
}

func newAcceleratorTunnelFailure(cause error) error {
	return acceleratorTunnelFailure{kind: classifyAcceleratorTunnelCause(cause)}
}

func newAcceleratorTunnelFailureKind(kind AcceleratorTunnelFailureKind) error {
	if kind == 0 {
		kind = AcceleratorTunnelFailureGeneric
	}
	return acceleratorTunnelFailure{kind: kind}
}

func classifyAcceleratorTunnelCause(cause error) AcceleratorTunnelFailureKind {
	if cause == nil {
		return 0
	}
	// A newly Running Pod can briefly be absent from the node's port-forward
	// runtime even though the API server already publishes it as Ready.
	if strings.Contains(strings.ToLower(cause.Error()), "pod not found") {
		return AcceleratorTunnelFailureTransient
	}
	if httpstream.IsUpgradeFailure(cause) {
		return AcceleratorTunnelFailureUpgrade
	}
	if httpstream.IsHTTPSProxyError(cause) {
		return AcceleratorTunnelFailureHTTPSProxy
	}
	if errors.Is(cause, context.DeadlineExceeded) {
		return AcceleratorTunnelFailureTransient
	}
	if apierrors.IsTimeout(cause) || apierrors.IsServerTimeout(cause) || apierrors.IsTooManyRequests(cause) || apierrors.IsInternalError(cause) || apierrors.IsServiceUnavailable(cause) {
		return AcceleratorTunnelFailureTransient
	}
	var status apierrors.APIStatus
	if errors.As(cause, &status) {
		code := status.Status().Code
		if code == 500 || code == 503 || code == 504 {
			return AcceleratorTunnelFailureTransient
		}
	}
	return AcceleratorTunnelFailureGeneric
}

type acceleratorTunnelCategoryDialer struct {
	next   httpstream.Dialer
	tunnel *AcceleratorPodTunnel
}

func (d *acceleratorTunnelCategoryDialer) Dial(protocols ...string) (httpstream.Connection, string, error) {
	connection, protocol, err := d.next.Dial(protocols...)
	d.tunnel.mu.Lock()
	d.tunnel.dialKind = classifyAcceleratorTunnelCause(err)
	d.tunnel.mu.Unlock()
	return connection, protocol, err
}

// ClassifyAcceleratorTunnelFailure returns only the closed retry category. The
// returned error itself never unwraps or retains its raw construction cause.
func ClassifyAcceleratorTunnelFailure(err error) AcceleratorTunnelFailureKind {
	failure, ok := err.(acceleratorTunnelFailure)
	if !ok {
		return 0
	}
	return failure.kind
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

var newAcceleratorFinalDialer = func(primary, secondary httpstream.Dialer) httpstream.Dialer {
	return portforward.NewFallbackDialer(primary, secondary, acceleratorTunnelFallback)
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

// AcceleratorPodTunnelFailure returns an opaque fixed error after Done closes.
func AcceleratorPodTunnelFailure(t *AcceleratorPodTunnel) error {
	if t == nil {
		return nil
	}
	select {
	case <-t.done:
	default:
		return nil
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.kind == 0 {
		return nil
	}
	return newAcceleratorTunnelFailureKind(t.kind)
}

func acceleratorPodTunnelStartFailure(t *AcceleratorPodTunnel) error {
	if failure := AcceleratorPodTunnelFailure(t); failure != nil {
		return failure
	}
	return ErrAcceleratorContextUnavailable
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
		logAcceleratorPodTunnelFailure("validate_input", namespace, pod, ErrAcceleratorContextUnavailable, AcceleratorTunnelFailureGeneric)
		return nil, ErrAcceleratorContextUnavailable
	}
	ctx = acceleratorSealedContext{ctx}
	cfg := snapshot.RESTConfig()
	if cfg == nil {
		logAcceleratorPodTunnelFailure("load_rest_config", namespace, pod, ErrAcceleratorContextUnavailable, AcceleratorTunnelFailureGeneric)
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
		logAcceleratorPodTunnelFailure("build_url", namespace, pod, err, classifyAcceleratorTunnelCause(err))
		return nil, newAcceleratorTunnelFailure(err)
	}
	primary, err := portforward.NewSPDYOverWebsocketDialer(u, cfg)
	if err != nil {
		logAcceleratorPodTunnelFailure("create_websocket_dialer", namespace, pod, err, classifyAcceleratorTunnelCause(err))
		return nil, newAcceleratorTunnelFailure(err)
	}
	rt, upgrader, err := spdy.RoundTripperFor(cfg)
	if err != nil {
		logAcceleratorPodTunnelFailure("create_spdy_roundtripper", namespace, pod, err, classifyAcceleratorTunnelCause(err))
		return nil, newAcceleratorTunnelFailure(err)
	}
	secondary := spdy.NewDialer(upgrader, &http.Client{Transport: rt}, "POST", u)
	stop, ready, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	tunnel := &AcceleratorPodTunnel{stop: stop, done: done}
	dialer := &acceleratorTunnelCategoryDialer{next: newAcceleratorFinalDialer(primary, secondary), tunnel: tunnel}
	pf, err := newAcceleratorPortForwarder(dialer, []string{"127.0.0.1"}, []string{"0:8080"}, stop, ready)
	if err != nil {
		logAcceleratorPodTunnelFailure("create_forwarder", namespace, pod, err, classifyAcceleratorTunnelCause(err))
		return nil, newAcceleratorTunnelFailure(err)
	}
	var publicationMu sync.Mutex
	forwardFinished := false
	go func() {
		forwardErr := pf.ForwardPorts()
		publicationMu.Lock()
		forwardFinished = true
		tunnel.mu.Lock()
		kind := classifyAcceleratorTunnelCause(forwardErr)
		if errors.Is(forwardErr, portforward.ErrLostConnectionToPod) {
			kind = AcceleratorTunnelFailureTransient
		} else if tunnel.dialKind != 0 {
			kind = tunnel.dialKind
		}
		tunnel.kind = kind
		tunnel.mu.Unlock()
		if forwardErr != nil {
			logAcceleratorPodTunnelFailure("forward_ports", namespace, pod, forwardErr, kind)
		}
		close(done)
		publicationMu.Unlock()
	}()
	var stopOnce sync.Once
	stopForwarding := func() { stopOnce.Do(func() { close(stop) }) }
	settleAfterContextDone := func() error {
		stopForwarding()
		settle := time.NewTimer(acceleratorTunnelCancellationSettlementTimeout)
		defer settle.Stop()
		select {
		case <-done:
			tunnel.mu.Lock()
			if (tunnel.kind == 0 || tunnel.kind == AcceleratorTunnelFailureGeneric) && errors.Is(ctx.Err(), context.DeadlineExceeded) {
				tunnel.kind = AcceleratorTunnelFailureTransient
			}
			tunnel.mu.Unlock()
			return acceleratorPodTunnelStartFailure(tunnel)
		case <-settle.C:
			return newAcceleratorTunnelFailureKind(AcceleratorTunnelFailureCleanupUnsettled)
		}
	}
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
		return nil, settleAfterContextDone()
	case <-ready:
		if ctx.Err() != nil {
			return nil, settleAfterContextDone()
		}
	case <-done:
		return nil, acceleratorPodTunnelStartFailure(tunnel)
	}
	select {
	case <-done:
		return nil, acceleratorPodTunnelStartFailure(tunnel)
	default:
	}
	ports, err := pf.GetPorts()
	if err != nil || len(ports) != 1 || ports[0].Local == 0 || ports[0].Remote != 8080 {
		logAcceleratorPodTunnelFailure("resolve_forwarded_port", namespace, pod, err, classifyAcceleratorTunnelCause(err))
		stopForwarding()
		waitForForwarding()
		return nil, newAcceleratorTunnelFailure(err)
	}
	// Completion and publication share this lock: a forwarder that has already
	// terminated can never be returned as a live tunnel, while a returned
	// tunnel owns any later termination through Done.
	publicationMu.Lock()
	if forwardFinished {
		publicationMu.Unlock()
		return nil, acceleratorPodTunnelStartFailure(tunnel)
	}
	tunnel.port = int(ports[0].Local)
	publicationMu.Unlock()
	return tunnel, nil
}

func logAcceleratorPodTunnelFailure(stage, namespace, pod string, err error, kind AcceleratorTunnelFailureKind) {
	details := map[string]interface{}{
		"stage":       stage,
		"namespace":   namespace,
		"pod":         pod,
		"failureKind": acceleratorTunnelFailureKindName(kind),
	}
	if err != nil {
		details["error"] = err.Error()
	}
	debug.LogPortforward("Accelerator Pod tunnel failed", details)
}

func acceleratorTunnelFailureKindName(kind AcceleratorTunnelFailureKind) string {
	switch kind {
	case AcceleratorTunnelFailureUpgrade:
		return "upgrade"
	case AcceleratorTunnelFailureHTTPSProxy:
		return "https_proxy"
	case AcceleratorTunnelFailureTransient:
		return "transient"
	case AcceleratorTunnelFailureCleanupUnsettled:
		return "cleanup_unsettled"
	default:
		return "generic"
	}
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
