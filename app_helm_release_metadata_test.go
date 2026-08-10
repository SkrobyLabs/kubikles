package main

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"kubikles/pkg/k8s"
)

func TestListHelmReleaseMetadataProjectsInsideAppBoundary(t *testing.T) {
	payload := appHelmStoragePayload(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/namespaces/team/secrets" || r.URL.Query().Get("fieldSelector") != "type="+k8s.HelmReleaseSecretType {
			t.Fatalf("request=%s query=%v", r.URL.Path, r.URL.Query())
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v1.SecretList{Items: []v1.Secret{{
			ObjectMeta: metav1.ObjectMeta{Name: "sh.helm.release.v1.example.v2", Namespace: "team"},
			Type:       k8s.HelmReleaseSecretType, Data: map[string][]byte{"release": payload},
		}}})
	}))
	defer server.Close()
	client, err := k8s.NewClientForRESTConfig(&rest.Config{Host: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	app := &App{runtimeMode: RuntimeModeAccelerator, k8sClient: client, listRequestManager: NewListRequestManager()}
	releases, err := app.ListHelmReleaseMetadata("request", "team")
	if err != nil || len(releases) != 1 {
		t.Fatalf("releases=%#v error=%v", releases, err)
	}
	if release := releases[0]; release.Name != "example" || release.Namespace != "team" || release.Revision != 2 || release.Status != "deployed" || release.Chart != "example-chart" {
		t.Fatalf("release=%#v", release)
	}
	serialized, err := json.Marshal(releases)
	if err != nil || bytes.Contains(serialized, []byte("must not escape")) {
		t.Fatalf("projection leaked stored data: %s (%v)", serialized, err)
	}
}

func appHelmStoragePayload(t *testing.T) []byte {
	t.Helper()
	plain := []byte(`{"name":"example","namespace":"team","version":2,"info":{"status":"deployed","last_deployed":"2026-08-10T19:30:00Z","description":"Ready","notes":"must not escape"},"chart":{"metadata":{"name":"example-chart","version":"1.2.3","appVersion":"4.5.6"}},"manifest":"must not escape","config":{"password":"must not escape"}}`)
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err := writer.Write(plain); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return []byte(base64.StdEncoding.EncodeToString(compressed.Bytes()))
}
