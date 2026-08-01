package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"k8s.io/client-go/rest"
	"kubikles/pkg/k8s"
)

func readinessTestClient(t *testing.T, config *rest.Config) *k8s.Client {
	t.Helper()
	client, err := k8s.NewClientForRESTConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestAppReadinessProvider(t *testing.T) {
	client := readinessTestClient(t, &rest.Config{Host: "http://127.0.0.1"})
	app := &App{k8sClient: client}
	provider := newAppReadinessProvider(app)
	var calls atomic.Int32
	provider.probe = func(ctx context.Context, got *k8s.Client) error {
		calls.Add(1)
		if got != client {
			t.Fatal("probe received wrong client")
		}
		return ctx.Err()
	}
	if err := provider.Ready(context.Background()); !errors.Is(err, errAppNotInitialized) || calls.Load() != 0 {
		t.Fatalf("before initialization error/calls = %v/%d", err, calls.Load())
	}
	provider.markInitialized()
	if err := provider.Ready(context.Background()); err != nil || calls.Load() != 1 {
		t.Fatalf("after initialization error/calls = %v/%d", err, calls.Load())
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := provider.Ready(canceled); !errors.Is(err, errKubernetesUnavailable) {
		t.Fatalf("canceled error = %v", err)
	}

	for name, candidate := range map[string]*appReadinessProvider{
		"nil app":    {initialized: atomic.Bool{}},
		"nil client": {app: &App{}, probe: provider.probe},
		"init error": {app: &App{k8sClient: client, k8sInitError: errors.New("init")}, probe: provider.probe},
		"nil probe":  {app: app},
	} {
		t.Run(name, func(t *testing.T) {
			candidate.markInitialized()
			if err := candidate.Ready(context.Background()); !errors.Is(err, errKubernetesUnavailable) {
				t.Fatalf("Ready() error = %v", err)
			}
		})
	}
}

func TestRESTConfigReachabilityProbe(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusForbidden, http.StatusInternalServerError} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/prefix/version" {
					t.Errorf("request = %s %s", r.Method, r.URL.Path)
				}
				if got := r.Header.Get("Authorization"); got != "Bearer secret-token" {
					t.Errorf("Authorization = %q", got)
				}
				w.WriteHeader(status)
				_, _ = io.WriteString(w, "private response content")
			}))
			defer server.Close()
			err := probeRESTConfigReachability(context.Background(), &rest.Config{Host: server.URL + "/prefix", BearerToken: "secret-token"})
			if status < 500 && err != nil {
				t.Fatalf("status %d error = %v", status, err)
			}
			if status >= 500 && err == nil {
				t.Fatalf("status %d unexpectedly ready", status)
			}
		})
	}
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "http://127.0.0.1:1/unreachable")
		w.WriteHeader(http.StatusFound)
	}))
	defer redirect.Close()
	if err := probeRESTConfigReachability(context.Background(), &rest.Config{Host: redirect.URL}); err != nil {
		t.Fatalf("redirect response below 500 should prove reachability: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { time.Sleep(100 * time.Millisecond) }))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := probeRESTConfigReachability(ctx, &rest.Config{Host: server.URL}); err == nil {
		t.Fatal("timeout unexpectedly ready")
	}
	if err := probeRESTConfigReachability(context.Background(), &rest.Config{Host: "://bad"}); err == nil {
		t.Fatal("malformed host unexpectedly ready")
	}
}

func TestRESTConfigReachabilityProbeDoesNotMutateDefaultClient(t *testing.T) {
	originalRedirectPolicy := http.DefaultClient.CheckRedirect
	t.Cleanup(func() { http.DefaultClient.CheckRedirect = originalRedirectPolicy })

	wantRedirectError := errors.New("default redirect policy invoked")
	defaultRedirectPolicy := func(*http.Request, []*http.Request) error { return wantRedirectError }
	http.DefaultClient.CheckRedirect = defaultRedirectPolicy

	var redirected atomic.Int32
	redirectTarget := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		redirected.Add(1)
	}))
	defer redirectTarget.Close()
	redirectSource := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/version" {
			t.Errorf("request path = %q, want /version", r.URL.Path)
		}
		w.Header().Set("Location", redirectTarget.URL+"/followed")
		w.WriteHeader(http.StatusFound)
	}))
	defer redirectSource.Close()

	config := &rest.Config{Host: redirectSource.URL}
	httpClient, err := rest.HTTPClientFor(rest.CopyConfig(config))
	if err != nil {
		t.Fatal(err)
	}
	if httpClient != http.DefaultClient {
		t.Fatal("bare REST config did not select http.DefaultClient")
	}
	if err := probeRESTConfigReachability(context.Background(), config); err != nil {
		t.Fatalf("redirect response should prove reachability: %v", err)
	}
	if redirected.Load() != 0 {
		t.Fatal("readiness probe followed redirect")
	}
	if got, want := reflect.ValueOf(http.DefaultClient.CheckRedirect).Pointer(), reflect.ValueOf(defaultRedirectPolicy).Pointer(); got != want {
		t.Fatal("readiness probe mutated http.DefaultClient.CheckRedirect")
	}
}

func TestRESTConfigReachabilityProbeConcurrentBareConfig(t *testing.T) {
	originalRedirectPolicy := http.DefaultClient.CheckRedirect
	t.Cleanup(func() { http.DefaultClient.CheckRedirect = originalRedirectPolicy })
	defaultRedirectPolicy := func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	http.DefaultClient.CheckRedirect = defaultRedirectPolicy

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/version" {
			t.Errorf("request path = %q, want /version", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	config := &rest.Config{Host: server.URL}

	const probes = 32
	start := make(chan struct{})
	errorsSeen := make(chan error, probes)
	var group sync.WaitGroup
	group.Add(probes)
	for range probes {
		go func() {
			defer group.Done()
			<-start
			errorsSeen <- probeRESTConfigReachability(context.Background(), config)
		}()
	}
	close(start)
	group.Wait()
	close(errorsSeen)
	for err := range errorsSeen {
		if err != nil {
			t.Fatalf("concurrent readiness probe: %v", err)
		}
	}
	if got, want := reflect.ValueOf(http.DefaultClient.CheckRedirect).Pointer(), reflect.ValueOf(defaultRedirectPolicy).Pointer(); got != want {
		t.Fatal("concurrent readiness probes mutated http.DefaultClient.CheckRedirect")
	}
}

type trackingBody struct {
	closed atomic.Bool
	reader *strings.Reader
}

func (b *trackingBody) Read(data []byte) (int, error) { return b.reader.Read(data) }
func (b *trackingBody) Close() error                  { b.closed.Store(true); return nil }

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestRESTConfigReachabilityProbeDiscardsAndClosesBody(t *testing.T) {
	body := &trackingBody{reader: strings.NewReader("response")}
	config := &rest.Config{
		Host: "http://127.0.0.1",
		Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusForbidden, Body: body, Header: make(http.Header)}, nil
		}),
	}
	if err := probeRESTConfigReachability(context.Background(), config); err != nil {
		t.Fatal(err)
	}
	if !body.closed.Load() || body.reader.Len() != 0 {
		t.Fatalf("body closed/remaining = %v/%d", body.closed.Load(), body.reader.Len())
	}
}
