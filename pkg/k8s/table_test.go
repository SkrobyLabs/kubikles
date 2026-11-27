package k8s

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
)

func TestResourcePath(t *testing.T) {
	cases := []struct {
		gvr  schema.GroupVersionResource
		ns   string
		want string
	}{
		{schema.GroupVersionResource{Version: "v1", Resource: "secrets"}, "", "/api/v1/secrets"},
		{schema.GroupVersionResource{Version: "v1", Resource: "secrets"}, "ns", "/api/v1/namespaces/ns/secrets"},
		{schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}, "web", "/apis/apps/v1/namespaces/web/deployments"},
		{schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterroles"}, "", "/apis/rbac.authorization.k8s.io/v1/clusterroles"},
	}
	for _, c := range cases {
		if got := ResourcePath(c.gvr, c.ns); got != c.want {
			t.Errorf("ResourcePath(%v,%q) = %q, want %q", c.gvr, c.ns, got, c.want)
		}
	}
}

func TestRESTClientOfFakeIsNil(t *testing.T) {
	if rc := RESTClientOf(fake.NewSimpleClientset()); rc != nil {
		t.Fatalf("fake clientset has no usable REST client, got %#v", rc)
	}
	if RESTClientOf(nil) != nil {
		t.Fatal("nil clientset must give nil")
	}
	cs, err := kubernetes.NewForConfig(&rest.Config{Host: "http://127.0.0.1:1"})
	if err != nil {
		t.Fatal(err)
	}
	if RESTClientOf(cs) == nil {
		t.Fatal("real clientset must give a REST client")
	}
}

type tableRecorder struct {
	mu      sync.Mutex
	queries []string
	accepts []string
}

func tableHandler(rec *tableRecorder, kind string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.mu.Lock()
		rec.queries = append(rec.queries, r.URL.RawQuery)
		rec.accepts = append(rec.accepts, r.Header.Get("Accept"))
		rec.mu.Unlock()
		meta := map[string]interface{}{"resourceVersion": "9"}
		var rows []map[string]interface{}
		if r.URL.Query().Get("continue") == "" && r.URL.Query().Get("limit") != "" {
			meta["continue"] = "tok"
			rem := int64(1)
			meta["remainingItemCount"] = rem
			rows = append(rows, map[string]interface{}{"cells": []interface{}{"a"}})
		} else {
			rows = append(rows, map[string]interface{}{"cells": []interface{}{"b"}})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"kind": kind, "apiVersion": "meta.k8s.io/v1", "metadata": meta,
			"columnDefinitions": []map[string]interface{}{{"name": "Name"}}, "rows": rows,
		})
	})
}

func newTableTestClient(t *testing.T, h http.Handler) rest.Interface {
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	cs, err := kubernetes.NewForConfig(&rest.Config{Host: srv.URL, QPS: -1})
	if err != nil {
		t.Fatal(err)
	}
	return RESTClientOf(cs)
}

func TestListTablePagesOnlyWithPageSize(t *testing.T) {
	rec := &tableRecorder{}
	rc := newTableTestClient(t, tableHandler(rec, "Table"))

	var pages []TablePage
	collect := func(p TablePage) error { pages = append(pages, p); return nil }

	if err := ListTable(context.Background(), rc, "/api/v1/secrets", TableListOptions{}, collect); err != nil {
		t.Fatal(err)
	}
	if len(pages) != 1 || rec.queries[0] != "" {
		t.Fatalf("unbounded list: pages %d queries %q (no limit may be sent)", len(pages), rec.queries)
	}
	if rec.accepts[0] != TableAccept {
		t.Fatalf("accept = %q", rec.accepts[0])
	}

	pages = nil
	rec.queries = nil
	if err := ListTable(context.Background(), rc, "/api/v1/secrets", TableListOptions{PageSize: 500}, collect); err != nil {
		t.Fatal(err)
	}
	if len(pages) != 2 || pages[0].Continue != "tok" || pages[0].Remaining == nil || *pages[0].Remaining != 1 || pages[0].ResourceVersion != "9" {
		t.Fatalf("pages = %+v", pages)
	}
	if want := []string{"limit=500", "continue=tok&limit=500"}; !reflect.DeepEqual(rec.queries, want) {
		t.Fatalf("queries = %q, want %q", rec.queries, want)
	}
}

func TestListTableRequireTable(t *testing.T) {
	rec := &tableRecorder{}
	rc := newTableTestClient(t, tableHandler(rec, "SecretList"))
	err := ListTable(context.Background(), rc, "/api/v1/secrets", TableListOptions{RequireTable: true}, func(TablePage) error { return nil })
	if !errors.Is(err, ErrTableUnsupported) {
		t.Fatalf("err = %v", err)
	}
	// Without RequireTable the legacy behaviour (parse whatever came back) holds.
	if err := ListTable(context.Background(), rc, "/api/v1/secrets", TableListOptions{}, func(TablePage) error { return nil }); err != nil {
		t.Fatal(err)
	}
}

func TestListTableNilClientAndCancellation(t *testing.T) {
	if err := ListTable(context.Background(), nil, "/x", TableListOptions{}, nil); !errors.Is(err, ErrTableUnsupported) {
		t.Fatalf("nil client err = %v", err)
	}
	rec := &tableRecorder{}
	rc := newTableTestClient(t, tableHandler(rec, "Table"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := ListTable(ctx, rc, "/api/v1/secrets", TableListOptions{}, func(TablePage) error { return nil }); !errors.Is(err, ErrRequestCancelled) {
		t.Fatalf("cancelled err = %v", err)
	}
}

func TestListSecretsMetadataOnFakeClientsetFailsInsteadOfPanicking(t *testing.T) {
	c := &Client{clientset: fake.NewSimpleClientset(), currentContext: "x"}
	if _, err := c.ListSecretsMetadataWithContext(context.Background(), "ns"); err == nil {
		t.Fatal("expected an error without a REST client")
	}
}

// Every registered type must open a watch exactly as the former switch did, and
// the registry must cover the same set of types.
func TestWatchResourceUsesRegistryForEveryType(t *testing.T) {
	want := []string{
		"clusterrolebindings", "clusterroles", "configmaps", "cronjobs", "csidrivers", "csinodes", "daemonsets",
		"deployments", "endpoints", "endpointslices", "events", "hpas", "ingressclasses", "ingresses", "jobs", "leases", "limitranges",
		"mutatingwebhookconfigurations", "namespaces", "networkpolicies", "nodes", "pdbs", "persistentvolumeclaims",
		"persistentvolumes", "pods", "priorityclasses", "replicasets", "resourcequotas", "rolebindings", "roles",
		"secrets", "serviceaccounts", "services", "statefulsets", "storageclasses", "validatingwebhookconfigurations",
	}
	if got := ResourceTypes(); !reflect.DeepEqual(got, want) {
		t.Fatalf("registry types = %v\nwant %v", got, want)
	}
	c := &Client{clientset: fake.NewSimpleClientset(), currentContext: "x"}
	for _, rt := range want {
		w, err := c.WatchResource(context.Background(), rt, "ns", "")
		if err != nil {
			t.Errorf("WatchResource(%s): %v", rt, err)
			continue
		}
		w.Stop()
	}
	if _, err := c.WatchResource(context.Background(), "nonsense", "", ""); err == nil {
		t.Fatal("unknown type must fail")
	}
}

func TestRegistryListsEveryTypeOnFakeClientset(t *testing.T) {
	cs := fake.NewSimpleClientset()
	for _, rt := range ResourceTypes() {
		def, _ := LookupResource(rt)
		l, err := def.List(context.Background(), cs, "ns", metav1.ListOptions{})
		if err != nil || l == nil {
			t.Errorf("List(%s): %v", rt, err)
		}
		if def.GVR.Resource == "" || def.GVR.Version == "" {
			t.Errorf("%s has an incomplete GVR %v", rt, def.GVR)
		}
	}
}

func TestStripManagedFieldsObject(t *testing.T) {
	p := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{ManagedFields: []metav1.ManagedFieldsEntry{{Manager: "x"}}}}
	StripManagedFieldsObject(p)
	if p.ManagedFields != nil {
		t.Fatal("typed object not stripped")
	}
	u := &unstructured.Unstructured{Object: map[string]interface{}{"metadata": map[string]interface{}{"name": "n", "managedFields": []interface{}{1}}}}
	StripManagedFieldsObject(u)
	if _, ok := u.Object["metadata"].(map[string]interface{})["managedFields"]; ok {
		t.Fatal("unstructured object not stripped")
	}
}
