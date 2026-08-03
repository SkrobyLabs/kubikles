package k8s

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/yaml"
)

func TestListSecretsMetadataProjectionPaginationAndHelmFilter(t *testing.T) {
	var requests []*http.Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Clone(r.Context()))
		table := metav1.Table{ColumnDefinitions: []metav1.TableColumnDefinition{{Name: "Name"}, {Name: "Type"}, {Name: "Data"}}}
		if r.URL.Query().Get("continue") == "" {
			table.Rows = append(table.Rows, metav1.TableRow{Cells: []interface{}{"cell-name", "Opaque", "2"}, Object: runtime.RawExtension{Raw: []byte(`{"metadata":{"namespace":"ns","uid":"safe-id","labels":{"ordinary":"label"},"annotations":{"ordinary":"annotation"}}}`)}})
			table.Continue = "next"
			table.RemainingItemCount = ptr(int64(1))
		} else {
			table.Rows = append(table.Rows, metav1.TableRow{Cells: []interface{}{"two", "kubernetes.io/tls", float64(1)}})
		}
		_ = json.NewEncoder(w).Encode(table)
	}))
	defer server.Close()
	cs, err := kubernetes.NewForConfig(&rest.Config{Host: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	var progress [][2]int
	items, err := (&Client{clientset: cs}).ListSecretsMetadataWithOptions(context.Background(), "ns", SecretListOptions{ExcludeHelmReleases: true}, func(loaded, total int) { progress = append(progress, [2]int{loaded, total}) })
	if err != nil || len(items) != 2 {
		t.Fatalf("list = %v, %v", items, err)
	}
	if items[0].Metadata.Name != "cell-name" || items[0].Metadata.Namespace != "ns" || items[0].DataKeys != 2 || items[1].DataKeys != 1 {
		t.Fatalf("projection = %#v", items)
	}
	if items[0].Metadata.Labels["ordinary"] != "label" || items[0].Metadata.Annotations["ordinary"] != "annotation" {
		t.Fatalf("ordinary metadata compatibility lost: %#v", items[0].Metadata)
	}
	if len(requests) != 2 || len(progress) != 2 || progress[0] != [2]int{1, 2} || progress[1] != [2]int{2, 2} {
		t.Fatalf("requests/progress = %d %#v", len(requests), progress)
	}
	for _, request := range requests {
		if request.Header.Get("Accept") != "application/json;as=Table;g=meta.k8s.io;v=v1" || request.URL.Query().Get("includeObject") != "Metadata" || request.URL.Query().Get("fieldSelector") != "type!="+HelmReleaseSecretType || request.URL.Query().Get("limit") != "1000" {
			t.Fatalf("request = %s %v", request.Header.Get("Accept"), request.URL.Query())
		}
	}
}

func TestGetSecretYamlPreservesEditableMetadataAndStripsManagedFields(t *testing.T) {
	secret := &v1.Secret{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"},
		ObjectMeta: metav1.ObjectMeta{
			Name: "example", Namespace: "ns", UID: "uid-1", ResourceVersion: "rv-9",
			Labels: map[string]string{"label": "value"}, Annotations: map[string]string{"annotation": "value"},
			Finalizers: []string{"example/finalizer"}, OwnerReferences: []metav1.OwnerReference{{APIVersion: "v1", Kind: "ConfigMap", Name: "owner", UID: "owner-uid"}},
			ManagedFields: []metav1.ManagedFieldsEntry{{Manager: "HOSTILE_MANAGER"}},
		},
		Type: v1.SecretTypeOpaque, Data: map[string][]byte{"z": []byte("z"), "a": []byte("a")},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/namespaces/ns/secrets/example" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(secret)
	}))
	defer server.Close()
	cs, err := kubernetes.NewForConfig(&rest.Config{Host: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	client := &Client{clientset: cs}
	first, err := client.GetSecretYaml("ns", "example")
	if err != nil {
		t.Fatal(err)
	}
	second, err := client.GetSecretYaml("ns", "example")
	if err != nil || first != second {
		t.Fatalf("determinism = %q, %v", second, err)
	}
	for _, required := range []string{"name: example", "namespace: ns", "uid: uid-1", "resourceVersion: rv-9", "labels:", "annotations:", "finalizers:", "ownerReferences:", "type: Opaque", "a: YQ==", "z: eg=="} {
		if !strings.Contains(first, required) {
			t.Fatalf("editable YAML missing %q: %s", required, first)
		}
	}
	if strings.Contains(first, "managedFields") || strings.Contains(first, "HOSTILE_MANAGER") {
		t.Fatalf("managed fields leaked: %s", first)
	}
	var decoded v1.Secret
	if err := yaml.Unmarshal([]byte(first), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.ResourceVersion != secret.ResourceVersion || decoded.UID != secret.UID || decoded.Labels["label"] != "value" || decoded.Annotations["annotation"] != "value" || len(decoded.Finalizers) != 1 || len(decoded.OwnerReferences) != 1 || decoded.Type != secret.Type || string(decoded.Data["a"]) != "a" {
		t.Fatalf("editable YAML lost fields: %#v", decoded)
	}
}

func TestListSecretsMetadataSkipsHelmRowsWhenServerIgnoresSelector(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("fieldSelector") != "type!="+HelmReleaseSecretType {
			t.Fatalf("missing Helm selector: %v", r.URL.Query())
		}
		table := metav1.Table{ColumnDefinitions: []metav1.TableColumnDefinition{{Name: "Name"}, {Name: "Type"}, {Name: "Data"}}, Rows: []metav1.TableRow{
			{Cells: []interface{}{"ordinary", "Opaque", float64(1)}},
			{Cells: []interface{}{"sh.helm.release.v1.app.v1", HelmReleaseSecretType, float64(1)}},
		}}
		_ = json.NewEncoder(w).Encode(table)
	}))
	defer server.Close()
	cs, err := kubernetes.NewForConfig(&rest.Config{Host: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	var progress [][2]int
	items, err := (&Client{clientset: cs}).ListSecretsMetadataWithOptions(context.Background(), "ns", SecretListOptions{ExcludeHelmReleases: true}, func(loaded, total int) { progress = append(progress, [2]int{loaded, total}) })
	if err != nil || len(items) != 1 || items[0].Metadata.Name != "ordinary" {
		t.Fatalf("retained items = %#v, %v", items, err)
	}
	if len(progress) != 1 || progress[0] != [2]int{1, 1} {
		t.Fatalf("progress = %#v", progress)
	}
}
