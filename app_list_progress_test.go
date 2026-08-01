package main

import (
	"encoding/json"
	"net/http"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"kubikles/pkg/events"
	"kubikles/pkg/k8s"
)

func TestListProgressRequestCorrelation(t *testing.T) {
	var progress []ListProgress
	app := &App{emitter: events.EmitterFunc(func(name string, data ...interface{}) {
		if name == "list-progress" {
			progress = append(progress, data[0].(ListProgress))
		}
	})}
	app.listProgressCallbackForRequest("secrets", "secret-request-a")(3, 9)
	got := progress[len(progress)-1]
	if got.RequestID != "secret-request-a" || got.Loaded != 3 || got.Total != 9 {
		t.Fatalf("got %#v", got)
	}
	encoded, err := json.Marshal(got)
	if err != nil || string(encoded) != `{"resourceType":"secrets","loaded":3,"total":9,"requestId":"secret-request-a"}` {
		t.Fatalf("json = %s, %v", encoded, err)
	}
	app.listProgressCallback("pods")(1, 1)
	got = progress[len(progress)-1]
	encoded, _ = json.Marshal(got)
	if string(encoded) != `{"resourceType":"pods","loaded":1,"total":1}` {
		t.Fatalf("legacy json = %s", encoded)
	}

	client := acceleratorProjectionTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(metav1.Table{
			ColumnDefinitions: []metav1.TableColumnDefinition{{Name: "Name"}, {Name: "Type"}, {Name: "Data"}},
			Rows:              []metav1.TableRow{{Cells: []interface{}{"safe", "Opaque", float64(1)}}},
		})
	}))
	app.k8sClient = client
	app.listRequestManager = NewListRequestManager()
	if _, err := app.ListSecretsMetadata("direct-request", "direct"); err != nil {
		t.Fatal(err)
	}
	if _, err := app.acceleratorSecretsMetadata("accelerator-request", "accelerator", k8s.SecretListOptions{ExcludeHelmReleases: true}); err != nil {
		t.Fatal(err)
	}
	if len(progress) != 4 || progress[2].RequestID != "direct-request" || progress[3].RequestID != "accelerator-request" {
		t.Fatalf("direct/Accelerator progress = %#v", progress)
	}
	if progress[2].ResourceType != "secrets" || progress[3].ResourceType != "secrets" {
		t.Fatalf("resource correlation = %#v", progress)
	}
}

func TestSecretListCancelIDIsolation(t *testing.T) {
	manager := NewListRequestManager()
	app := &App{listRequestManager: manager}
	first, firstSequence := manager.StartRequest("secret-child-a")
	second, secondSequence := manager.StartRequest("secret-child-b")
	if !app.CancelListRequest("secret-child-a") {
		t.Fatal("first child was not canceled")
	}
	select {
	case <-first.Done():
	default:
		t.Fatal("first child context remains active")
	}
	select {
	case <-second.Done():
		t.Fatal("canceling first child canceled second child")
	default:
	}
	if app.CancelListRequest("unknown-child") {
		t.Fatal("unknown child reported canceled")
	}
	manager.CompleteRequest("secret-child-a", firstSequence)
	manager.CompleteRequest("secret-child-b", secondSequence)
	stats := manager.GetStats()
	if stats.Canceled != 1 || stats.Completed != 1 || stats.Pending != 0 {
		t.Fatalf("isolated stats = %#v", stats)
	}
}
