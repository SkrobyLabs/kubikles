package k8s

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"k8s.io/client-go/pkg/apis/clientauthentication"
	"k8s.io/client-go/plugin/pkg/client/auth/exec"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/transport"
)

const execAuthFailureCooldown = 30 * time.Second

// Match client-go's authenticator identity so pools, metrics and resource clients
// share failures as well as successful credentials. Never key by context name:
// contexts can share credentials, and their configuration can change.
var execAuthGuards sync.Map // *exec.Authenticator -> *execAuthGuard

type execAuthGuard struct {
	mu       sync.Mutex
	inFlight *execAuthAttempt
	err      error
	retryAt  time.Time
	probe    http.RoundTripper
}

type execAuthAttempt struct {
	done chan struct{}
	err  error
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// Check credentials separately from the network request. client-go still owns
// the exec protocol, credential expiration, certificates and credential cache.
// The probe never contacts the cluster or exposes credentials to another server.
func execGuardFor(config *rest.Config) (*execAuthGuard, error) {
	if config.ExecProvider == nil || config.BearerToken != "" || config.BearerTokenFile != "" || config.Username != "" || config.Password != "" || config.CertFile != "" || len(config.CertData) != 0 {
		return nil, nil
	}
	var cluster *clientauthentication.Cluster
	if config.ExecProvider.ProvideClusterInfo {
		var err error
		cluster, err = rest.ConfigToExecCluster(config)
		if err != nil {
			return nil, err
		}
	}
	authenticator, err := exec.GetAuthenticator(config.ExecProvider, cluster)
	if err != nil {
		return nil, err
	}
	if guard, ok := execAuthGuards.Load(authenticator); ok {
		return guard.(*execAuthGuard), nil
	}
	probeConfig := &transport.Config{}
	if err := authenticator.UpdateTransportConfig(probeConfig); err != nil {
		return nil, err
	}
	probe, err := transport.HTTPWrappersForConfig(probeConfig, roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: http.NoBody, Request: r}, nil
	}))
	if err != nil {
		return nil, err
	}
	guard, _ := execAuthGuards.LoadOrStore(authenticator, &execAuthGuard{probe: probe})
	return guard.(*execAuthGuard), nil
}

func (g *execAuthGuard) check(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	g.mu.Lock()
	if g.err != nil && time.Now().Before(g.retryAt) {
		err := fmt.Errorf("credential plugin retry paused until %s: %w", g.retryAt.Format(time.RFC3339), g.err)
		g.mu.Unlock()
		return err
	}
	attempt := g.inFlight
	if attempt == nil {
		attempt = &execAuthAttempt{done: make(chan struct{})}
		g.inFlight = attempt
		// A canceled caller must not start a second credential process. client-go's
		// exec is not context-aware; retain ownership until it actually completes.
		go func() {
			req, _ := http.NewRequest(http.MethodGet, "https://credential-check.invalid", nil)
			response, err := g.probe.RoundTrip(req)
			if response != nil && response.Body != nil {
				_ = response.Body.Close()
			}
			g.mu.Lock()
			g.err = err
			attempt.err = err
			if err != nil {
				g.retryAt = time.Now().Add(execAuthFailureCooldown)
			}
			g.inFlight = nil
			close(attempt.done)
			g.mu.Unlock()
		}()
	}
	g.mu.Unlock()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-attempt.done:
		return attempt.err
	}
}

type execAuthTransport struct {
	base  http.RoundTripper
	guard *execAuthGuard
}

func (t *execAuthTransport) WrappedRoundTripper() http.RoundTripper { return t.base }

func (t *execAuthTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	// Match client-go: an explicit request credential takes precedence over exec.
	if r.Header.Get("Authorization") != "" {
		return t.base.RoundTrip(r)
	}
	if err := t.guard.check(r.Context()); err != nil {
		return nil, err
	}
	return t.base.RoundTrip(r)
}

// GuardExecAuthTransport adds shared credential failure throttling outside an
// already authenticated transport, including SPDY exec and port-forward clients.
func GuardExecAuthTransport(config *rest.Config, base http.RoundTripper) (http.RoundTripper, error) {
	guard, err := execGuardFor(config)
	if err != nil {
		return nil, err
	}
	if guard == nil {
		return base, nil
	}
	return &execAuthTransport{base: base, guard: guard}, nil
}

// Wrap outside client-go's authentication middleware. Config.WrapTransport is
// inside that middleware and cannot prevent a failing plugin from running.
func newAuthenticatedClient[T any](config *rest.Config, create func(*rest.Config, *http.Client) (T, error)) (T, error) {
	var zero T
	config = rest.CopyConfig(config)
	if config.UserAgent == "" {
		config.UserAgent = rest.DefaultKubernetesUserAgent()
	}
	client, err := rest.HTTPClientFor(config)
	if err != nil {
		return zero, err
	}
	client.Transport, err = GuardExecAuthTransport(config, client.Transport)
	if err != nil {
		return zero, err
	}
	return create(config, client)
}
