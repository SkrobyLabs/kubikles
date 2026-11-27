package k8s

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	metricsclientset "k8s.io/metrics/pkg/client/clientset/versioned"
)

// acceptRecorder is an API server stand-in that records the Accept header of
// every request by path and always answers with JSON.
type acceptRecorder struct {
	mu      sync.Mutex
	accepts map[string][]string
}

func (a *acceptRecorder) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	if a.accepts == nil {
		a.accepts = map[string][]string{}
	}
	a.accepts[r.URL.Path] = append(a.accepts[r.URL.Path], r.Header.Get("Accept"))
	a.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.URL.Path == "/api/v1/pods":
		fmt.Fprint(w, `{"kind":"PodList","apiVersion":"v1","metadata":{},"items":[]}`)
	case strings.HasPrefix(r.URL.Path, "/apis/example.com/"):
		fmt.Fprint(w, `{"kind":"WidgetList","apiVersion":"example.com/v1","metadata":{},"items":[]}`)
	case strings.HasPrefix(r.URL.Path, "/apis/apiextensions.k8s.io/"):
		fmt.Fprint(w, `{"kind":"CustomResourceDefinitionList","apiVersion":"apiextensions.k8s.io/v1","metadata":{},"items":[]}`)
	case strings.HasPrefix(r.URL.Path, "/apis/metrics.k8s.io/"):
		fmt.Fprint(w, `{"kind":"NodeMetricsList","apiVersion":"metrics.k8s.io/v1beta1","metadata":{},"items":[]}`)
	default:
		fmt.Fprint(w, `{"kind":"Status","apiVersion":"v1","status":"Failure","code":404}`)
	}
}

func (a *acceptRecorder) acceptFor(path string) []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.accepts[path]...)
}

// newKubeconfigClient writes a two-context kubeconfig under a temporary HOME
// that production loadConfig reads.
func newKubeconfigClient(t *testing.T, server string) {
	t.Helper()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".kube"), 0o700); err != nil {
		t.Fatal(err)
	}
	kubeconfig := fmt.Sprintf(`apiVersion: v1
kind: Config
clusters:
- name: c
  cluster:
    server: %s
users:
- name: u
  user: {}
contexts:
- name: ctx
  context: {cluster: c, user: u}
- name: other
  context: {cluster: c, user: u}
current-context: ctx
`, server)
	if err := os.WriteFile(filepath.Join(home, ".kube", "config"), []byte(kubeconfig), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("KUBECONFIG", "")
}

func TestApplyProtobuf(t *testing.T) {
	cfg := &rest.Config{}
	applyProtobuf(cfg)
	if cfg.ContentType != "application/vnd.kubernetes.protobuf" {
		t.Fatalf("ContentType = %q", cfg.ContentType)
	}
	if cfg.AcceptContentTypes != "application/vnd.kubernetes.protobuf,application/json" {
		t.Fatalf("AcceptContentTypes = %q", cfg.AcceptContentTypes)
	}
	applyProtobuf(nil) // must not panic
}

func TestApplyProtobufOptOut(t *testing.T) {
	t.Setenv("KUBIKLES_DISABLE_PROTOBUF", "1")
	cfg := &rest.Config{}
	applyProtobuf(cfg)
	if cfg.ContentType != "" || cfg.AcceptContentTypes != "" {
		t.Fatalf("opt-out ignored: %+v", cfg)
	}
}

// client-go v0.34 generated clients already prefer protobuf when the config
// leaves the content type empty (apiextensions and metrics included), so the
// explicit setting is about two things: it pins the typed clientset (current,
// pooled, per-context) to protobuf including raw RESTClient() requests, and it
// must not change anything else. The dynamic client stays JSON, and the
// apiextensions and metrics clients negotiate exactly what a default config
// gives them.
func TestProtobufIsScopedToTypedClientsets(t *testing.T) {
	rec := &acceptRecorder{}
	srv := httptest.NewServer(rec)
	t.Cleanup(srv.Close)
	newKubeconfigClient(t, srv.URL)

	c := &Client{}
	c.SetClientPoolSize(2)
	if err := c.loadConfig(""); err != nil {
		t.Fatal(err)
	}

	// Rotates over main + 2 pool clientsets; every one must be protobuf.
	for i := 0; i < 3; i++ {
		if _, err := c.ListPods(""); err != nil {
			t.Fatalf("ListPods: %v", err)
		}
	}
	// A different context builds a throwaway typed client.
	if _, err := c.ListPodsForContext("other", ""); err != nil {
		t.Fatalf("ListPodsForContext: %v", err)
	}
	// Raw requests through the typed clientset's REST client only get protobuf
	// from the explicit config.
	cs, err := c.getClientset()
	if err != nil {
		t.Fatal(err)
	}
	_, _ = cs.CoreV1().RESTClient().Get().AbsPath("/api/v1/raw").DoRaw(context.Background())

	for i, a := range rec.acceptFor("/api/v1/pods") {
		if !strings.HasPrefix(a, "application/vnd.kubernetes.protobuf") || !strings.Contains(a, "application/json") {
			t.Fatalf("typed request %d Accept = %q, want protobuf with JSON fallback", i, a)
		}
	}
	if n := len(rec.acceptFor("/api/v1/pods")); n != 4 {
		t.Fatalf("pod list requests = %d, want 4", n)
	}
	if got := rec.acceptFor("/api/v1/raw"); len(got) != 1 || !strings.HasPrefix(got[0], "application/vnd.kubernetes.protobuf") {
		t.Fatalf("raw REST client Accept = %v, want protobuf first", got)
	}

	// Clients built from a fresh rest.Config.
	if _, err := c.ListCustomResources("", "example.com", "v1", "widgets", ""); err != nil {
		t.Fatalf("ListCustomResources: %v", err)
	}
	if _, err := c.ListCRDs(""); err != nil {
		t.Fatalf("ListCRDs: %v", err)
	}
	_, _ = c.GetNodeMetrics()

	if got := rec.acceptFor("/apis/example.com/v1/widgets"); len(got) != 1 || strings.Contains(got[0], "protobuf") {
		t.Fatalf("dynamic client Accept = %v, must stay JSON", got)
	}

	// Baseline: the same typed clients on an untouched config.
	base := &acceptRecorder{}
	baseSrv := httptest.NewServer(base)
	t.Cleanup(baseSrv.Close)
	plain := &rest.Config{Host: baseSrv.URL}
	if mc, err := metricsclientset.NewForConfig(plain); err == nil {
		_, _ = mc.MetricsV1beta1().NodeMetricses().List(context.Background(), metav1.ListOptions{})
	}
	if ac, err := apiextensionsclientset.NewForConfig(plain); err == nil {
		_, _ = ac.ApiextensionsV1().CustomResourceDefinitions().List(context.Background(), metav1.ListOptions{})
	}
	for _, path := range []string{"/apis/metrics.k8s.io/v1beta1/nodes", "/apis/apiextensions.k8s.io/v1/customresourcedefinitions"} {
		want, got := base.acceptFor(path), rec.acceptFor(path)
		if len(want) != 1 || len(got) != 1 || want[0] != got[0] {
			t.Fatalf("%s Accept changed: baseline %v, now %v", path, want, got)
		}
	}

	cfg, err := c.GetRestConfigForContext("")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(cfg.ContentType, "protobuf") || strings.Contains(cfg.AcceptContentTypes, "protobuf") {
		t.Fatalf("GetRestConfigForContext leaked protobuf: %q / %q", cfg.ContentType, cfg.AcceptContentTypes)
	}
}
