package k8s

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// newAPIServerClient returns a Client backed by the real client-go REST stack
// talking to an httptest server. Unlike the fake clientset it honours query
// parameters such as limit/continue, tailLines and sinceTime exactly as the
// handler implements them, so it can pin behaviour that depends on what is
// actually sent over the wire.
func newAPIServerClient(t testing.TB, handler http.Handler) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	cs, err := kubernetes.NewForConfig(&rest.Config{Host: srv.URL, QPS: -1})
	if err != nil {
		t.Fatalf("build clientset: %v", err)
	}
	return &Client{clientset: cs, currentContext: "test-context"}
}
