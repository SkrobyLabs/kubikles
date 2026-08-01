package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"kubikles/pkg/k8s"
)

func TestTypedSecretWatchProjectorMatches20AAcceleratorListBytes(t *testing.T) {
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: "shared-projection", Namespace: "evidence", UID: "shared-uid",
			CreationTimestamp: metav1.NewTime(time.Date(2026, 8, 1, 4, 5, 6, 0, time.UTC)),
			Labels:            map[string]string{"PRIVATE_LABEL": "PRIVATE_VALUE"}, Annotations: map[string]string{"PRIVATE_ANNOTATION": "PRIVATE_VALUE"},
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
	client := acceleratorProjectionTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/namespaces/evidence/secrets" {
			t.Fatalf("20A list path = %q", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(metav1.Table{
			ColumnDefinitions: []metav1.TableColumnDefinition{{Name: "Name"}, {Name: "Type"}, {Name: "Data"}},
			Rows:              []metav1.TableRow{{Cells: []interface{}{secret.Name, string(secret.Type), float64(len(secret.Data))}, Object: runtime.RawExtension{Raw: metadata}}},
		})
	}))
	listed, err := (&App{k8sClient: client}).acceleratorSecretsMetadata("", secret.Namespace, k8s.SecretListOptions{})
	if err != nil || len(listed) != 1 {
		t.Fatalf("20A Accelerator list = %#v, %v", listed, err)
	}
	watchProjection := projectAcceleratorSecretListItem(k8s.ProjectSecretListItem(secret))
	watchBytes, err := json.Marshal(watchProjection)
	if err != nil {
		t.Fatal(err)
	}
	listBytes, err := json.Marshal(listed[0])
	if err != nil {
		t.Fatal(err)
	}
	if string(watchBytes) != string(listBytes) {
		t.Fatalf("typed watch/20A list bytes differ: %s / %s", watchBytes, listBytes)
	}
	var closed map[string]json.RawMessage
	if err := json.Unmarshal(watchBytes, &closed); err != nil {
		t.Fatal(err)
	}
	if len(closed) != 3 || closed["metadata"] == nil || closed["type"] == nil || closed["dataKeys"] == nil {
		t.Fatalf("shared Accelerator DTO is not closed: %s", watchBytes)
	}
	for _, private := range []string{"PRIVATE_", "labels", "annotations", "resourceVersion"} {
		if strings.Contains(string(watchBytes), private) || strings.Contains(string(listBytes), private) {
			t.Fatalf("shared projection leaked %q: %s / %s", private, watchBytes, listBytes)
		}
	}
}
