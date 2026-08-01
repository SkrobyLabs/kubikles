package k8s

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
)

func TestProjectSecretListItemValueFree(t *testing.T) {
	one := &v1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "same", Namespace: "ns", Labels: map[string]string{"LEAK_LABEL": "1"}, Annotations: map[string]string{"LEAK_ANNOTATION": "1"}}, Type: v1.SecretTypeOpaque, Data: map[string][]byte{"LEAK_KEY": []byte("LEAK_VALUE")}}
	two := one.DeepCopy()
	two.Data = map[string][]byte{"OTHER_KEY": []byte("OTHER_VALUE")}
	two.Labels = map[string]string{"OTHER_LABEL": "2"}
	two.Annotations = map[string]string{"OTHER_ANNOTATION": "2"}
	a, _ := json.Marshal(ProjectSecretListItem(one))
	b, _ := json.Marshal(ProjectSecretListItem(two))
	if string(a) != string(b) {
		t.Fatalf("projection bytes differ: %s / %s", a, b)
	}
	for _, marker := range []string{"LEAK_", "OTHER_", "VALUE"} {
		if string(a) != "" && contains(string(a), marker) {
			t.Fatalf("projection leaked %q: %s", marker, a)
		}
	}
}

// This deliberately keeps the same public shape and number of data keys while
// replacing every private byte.  The projection must be a closed schema, not a
// redacted Secret that accidentally gains new fields with a future API type.
func TestProjectSecretListItemUsesExactClosedKeysForHostileEqualCountSecrets(t *testing.T) {
	first := &v1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "same", Namespace: "ns", UID: "uid", Labels: map[string]string{"PRIVATE_LABEL_A": "a"}, Annotations: map[string]string{"PRIVATE_ANNOTATION_A": "a"}}, Type: v1.SecretTypeOpaque, Data: map[string][]byte{"PRIVATE_KEY_A": []byte("PRIVATE_VALUE_A"), "PRIVATE_KEY_B": []byte("PRIVATE_VALUE_B")}}
	second := first.DeepCopy()
	second.Labels = map[string]string{"PRIVATE_LABEL_Z": "z"}
	second.Annotations = map[string]string{"PRIVATE_ANNOTATION_Z": "z"}
	second.Data = map[string][]byte{"PRIVATE_KEY_X": []byte("PRIVATE_VALUE_X"), "PRIVATE_KEY_Y": []byte("PRIVATE_VALUE_Y")}

	for _, secret := range []*v1.Secret{first, second} {
		encoded, err := json.Marshal(ProjectSecretListItem(secret))
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(encoded, &fields); err != nil {
			t.Fatal(err)
		}
		if len(fields) != 3 || fields["metadata"] == nil || fields["type"] == nil || fields["dataKeys"] == nil {
			t.Fatalf("projected closed keys = %s", encoded)
		}
		for _, private := range []string{"PRIVATE_", "VALUE_", "labels", "annotations"} {
			if contains(string(encoded), private) {
				t.Fatalf("projection leaked %q in %s", private, encoded)
			}
		}
	}
	firstBytes, _ := json.Marshal(ProjectSecretListItem(first))
	secondBytes, _ := json.Marshal(ProjectSecretListItem(second))
	if string(firstBytes) != string(secondBytes) {
		t.Fatalf("equal-count private replacements altered projected bytes: %s / %s", firstBytes, secondBytes)
	}
}

func TestProjectSecretListItemMatches20AAcceleratorListProjectionBytes(t *testing.T) {
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "byte-identical",
			Namespace:         "evidence",
			UID:               "uid-20a-watch",
			CreationTimestamp: metav1.NewTime(time.Date(2026, 8, 1, 2, 3, 4, 0, time.UTC)),
			Labels:            map[string]string{"PRIVATE_LABEL": "PRIVATE_VALUE"},
			Annotations:       map[string]string{"PRIVATE_ANNOTATION": "PRIVATE_VALUE"},
		},
		Type: v1.SecretTypeOpaque,
		Data: map[string][]byte{"PRIVATE_KEY_A": []byte("PRIVATE_VALUE_A"), "PRIVATE_KEY_B": []byte("PRIVATE_VALUE_B")},
	}
	metadata, err := json.Marshal(map[string]interface{}{"metadata": map[string]interface{}{
		"name": secret.Name, "namespace": secret.Namespace, "uid": secret.UID, "creationTimestamp": secret.CreationTimestamp,
	}})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/namespaces/evidence/secrets" {
			t.Fatalf("20A list request = %s %s", r.Method, r.URL.Path)
		}
		table := metav1.Table{
			ColumnDefinitions: []metav1.TableColumnDefinition{{Name: "Name"}, {Name: "Type"}, {Name: "Data"}},
			Rows: []metav1.TableRow{{
				Cells:  []interface{}{secret.Name, string(secret.Type), float64(len(secret.Data))},
				Object: runtime.RawExtension{Raw: metadata},
			}},
		}
		_ = json.NewEncoder(w).Encode(table)
	}))
	defer server.Close()
	client, err := NewClientForRESTConfig(&rest.Config{Host: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	listed, err := client.ListSecretsMetadataWithContext(context.Background(), secret.Namespace)
	if err != nil || len(listed) != 1 {
		t.Fatalf("20A list projection = %#v, %v", listed, err)
	}
	watchBytes, err := json.Marshal(ProjectSecretListItem(secret))
	if err != nil {
		t.Fatal(err)
	}
	listBytes, err := json.Marshal(listed[0])
	if err != nil {
		t.Fatal(err)
	}
	if string(watchBytes) != string(listBytes) {
		t.Fatalf("watch/list projection bytes differ: %s / %s", watchBytes, listBytes)
	}
	var closed map[string]json.RawMessage
	if err := json.Unmarshal(watchBytes, &closed); err != nil {
		t.Fatal(err)
	}
	if len(closed) != 3 || closed["metadata"] == nil || closed["type"] == nil || closed["dataKeys"] == nil {
		t.Fatalf("shared DTO top-level keys are not closed: %s", watchBytes)
	}
	for _, private := range []string{"PRIVATE_", "labels", "annotations"} {
		if contains(string(watchBytes), private) || contains(string(listBytes), private) {
			t.Fatalf("shared DTO leaked %q: watch=%s list=%s", private, watchBytes, listBytes)
		}
	}
}

func TestWatchSecretsExactRequest(t *testing.T) {
	requests := make(chan *http.Request, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.Clone(r.Context())
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"type": "BOOKMARK", "object": map[string]any{"metadata": map[string]any{"resourceVersion": "9"}}})
	}))
	defer server.Close()
	client, err := NewClientForRESTConfig(&rest.Config{Host: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	stream, err := client.WatchSecrets(context.Background(), "team", "7", SecretListOptions{ExcludeHelmReleases: true})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Stop()
	select {
	case <-stream.ResultChan():
	case <-time.After(time.Second):
		t.Fatal("typed watch did not decode")
	}
	select {
	case request := <-requests:
		if request.Method != http.MethodGet || request.URL.Path != "/api/v1/namespaces/team/secrets" {
			t.Fatalf("request = %s %s", request.Method, request.URL.Path)
		}
		q := request.URL.Query()
		if q.Get("watch") != "true" || q.Get("allowWatchBookmarks") != "true" || q.Get("resourceVersion") != "7" || q.Get("timeoutSeconds") != "300" || q.Get("fieldSelector") != "type!="+HelmReleaseSecretType {
			t.Fatalf("query = %v", q)
		}
	case <-time.After(time.Second):
		t.Fatal("watch request missing")
	}
}

func TestWatchSecretsExactNamespacedAndAllNamespaceRequests(t *testing.T) {
	requests := make(chan *http.Request, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.Clone(r.Context())
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"type":"BOOKMARK","object":{"metadata":{"resourceVersion":"next"}}}`))
	}))
	defer server.Close()
	client, err := NewClientForRESTConfig(&rest.Config{Host: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, namespace, path, rv string
		options                   SecretListOptions
	}{
		{"namespaced filtered", "team", "/api/v1/namespaces/team/secrets", "47", SecretListOptions{ExcludeHelmReleases: true}},
		{"all namespaces unfiltered", "", "/api/v1/secrets", "", SecretListOptions{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			stream, err := client.WatchSecrets(context.Background(), test.namespace, test.rv, test.options)
			if err != nil {
				t.Fatal(err)
			}
			defer stream.Stop()
			select {
			case <-stream.ResultChan():
			case <-time.After(time.Second):
				t.Fatal("typed watch did not decode bookmark")
			}
			select {
			case request := <-requests:
				if request.Method != http.MethodGet || request.URL.Path != test.path {
					t.Fatalf("request = %s %s, want GET %s", request.Method, request.URL.Path, test.path)
				}
				q := request.URL.Query()
				if q.Get("watch") != "true" || q.Get("allowWatchBookmarks") != "true" || q.Get("timeoutSeconds") != "300" || q.Get("resourceVersion") != test.rv {
					t.Fatalf("query = %v", q)
				}
				wantSelector := ""
				if test.options.ExcludeHelmReleases {
					wantSelector = "type!=" + HelmReleaseSecretType
				}
				if q.Get("fieldSelector") != wantSelector {
					t.Fatalf("fieldSelector = %q, want %q", q.Get("fieldSelector"), wantSelector)
				}
			case <-time.After(time.Second):
				t.Fatal("watch request missing")
			}
		})
	}
}
func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
