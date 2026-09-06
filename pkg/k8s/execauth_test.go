package k8s

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"k8s.io/client-go/rest"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// Use the test executable as a portable credential plugin, including on Windows.
func TestExecCredentialHelper(t *testing.T) {
	path := os.Getenv("KUBIKLES_TEST_EXEC_COUNT")
	if path == "" {
		return
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		os.Exit(2)
	}
	_, _ = f.WriteString("call\n")
	_ = f.Close()
	if _, err := os.Stat(path + ".fail"); err == nil {
		os.Exit(1)
	}
	fmt.Print(`{"apiVersion":"client.authentication.k8s.io/v1","kind":"ExecCredential","status":{"token":"test-token","expirationTimestamp":"2099-01-01T00:00:00Z"}}`)
	os.Exit(0)
}

func TestExecAuthFailureSharedAcrossClientsAndRecovers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "calls")
	if err := os.WriteFile(path+".fail", nil, 0600); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	var requests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Error("missing plugin credentials")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	config := &rest.Config{Host: server.URL, TLSClientConfig: rest.TLSClientConfig{Insecure: true}, ExecProvider: &clientcmdapi.ExecConfig{
		Command: executable, Args: []string{"-test.run=^TestExecCredentialHelper$"},
		APIVersion: "client.authentication.k8s.io/v1", InteractiveMode: clientcmdapi.NeverExecInteractiveMode,
		Env: []clientcmdapi.ExecEnvVar{{Name: "KUBIKLES_TEST_EXEC_COUNT", Value: path}},
	}}
	clients := make([]*http.Client, 10)
	for i := range clients {
		clients[i], err = newAuthenticatedClient(config, func(_ *rest.Config, c *http.Client) (*http.Client, error) { return c, nil })
		if err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	for _, client := range clients {
		wg.Add(1)
		go func(c *http.Client) {
			defer wg.Done()
			resp, err := c.Get(server.URL)
			if resp != nil {
				_ = resp.Body.Close()
			}
			if err == nil || !strings.Contains(err.Error(), "getting credentials") {
				t.Errorf("expected credential failure, got %v", err)
			}
		}(client)
	}
	wg.Wait()
	assertCalls := func(want int) {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Count(string(data), "call\n"); got != want {
			t.Fatalf("plugin calls = %d, want %d", got, want)
		}
	}
	assertCalls(1)
	if requests.Load() != 0 {
		t.Fatal("failed authentication reached cluster")
	}
	if err := os.Remove(path + ".fail"); err != nil {
		t.Fatal(err)
	}
	// A fixed credential environment does not bypass the cooldown automatically.
	if _, err := clients[0].Get(server.URL); err == nil || !strings.Contains(err.Error(), "retry paused") {
		t.Fatalf("expected cooldown, got %v", err)
	}
	assertCalls(1)
	guard, err := execGuardFor(config)
	if err != nil {
		t.Fatal(err)
	}
	guard.mu.Lock()
	guard.retryAt = time.Time{}
	guard.mu.Unlock()
	for _, client := range clients {
		resp, err := client.Get(server.URL)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
	}
	assertCalls(2) // successful credentials are still cached by client-go
}

func TestExecAuthCancellationKeepsSingleAttempt(t *testing.T) {
	started, finish := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	guard := &execAuthGuard{probe: roundTripFunc(func(*http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-finish
		return nil, errors.New("credential plugin failed")
	})}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- guard.check(ctx) }()
	<-started
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
	ctx2, cancel2 := context.WithCancel(context.Background())
	go func() { done <- guard.check(ctx2) }()
	cancel2()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
	close(finish)
	if err := guard.check(context.Background()); err == nil {
		t.Fatal("expected failure")
	}
	if calls.Load() != 1 {
		t.Fatal("cancellation started another credential process")
	}
}

func TestExecAuthTransportDoesNotThrottleNetworkErrors(t *testing.T) {
	networkErr := errors.New("connection refused")
	var calls int
	guard := &execAuthGuard{probe: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, nil })}
	transport := &execAuthTransport{guard: guard, base: roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, networkErr
	})}
	req, _ := http.NewRequest(http.MethodGet, "https://cluster.invalid", nil)
	for range 2 {
		if _, err := transport.RoundTrip(req); !errors.Is(err, networkErr) {
			t.Fatalf("got %v", err)
		}
	}
	if calls != 2 {
		t.Fatalf("network calls = %d, want 2", calls)
	}
}

func TestExecAuthTransportExplicitCredentialsBypassPlugin(t *testing.T) {
	guard := &execAuthGuard{probe: roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Error("plugin invoked despite explicit credentials")
		return nil, errors.New("unexpected plugin call")
	})}
	transport := &execAuthTransport{guard: guard, base: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "Bearer explicit" {
			t.Error("explicit credentials changed")
		}
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
	})}
	req, _ := http.NewRequest(http.MethodGet, "https://cluster.invalid", nil)
	req.Header.Set("Authorization", "Bearer explicit")
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
}
