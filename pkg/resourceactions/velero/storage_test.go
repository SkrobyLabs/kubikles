package velero

import (
	"context"
	"testing"

	"kubikles/pkg/resourceactions"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic/fake"
)

func TestStorageValidationPreservesPolicyAndStatus(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "velero.io/v1", "kind": "BackupStorageLocation", "metadata": map[string]interface{}{"name": "storage", "namespace": "velero", "uid": "uid", "resourceVersion": "1"}, "spec": map[string]interface{}{"validationFrequency": "0s"}, "status": map[string]interface{}{"lastValidationTime": "2026-09-01T00:00:00Z", "phase": "Unavailable", "message": "previous failure"}}}
	c := fake.NewSimpleDynamicClient(runtime.NewScheme(), obj)
	ref := resourceactions.ResourceRef{Context: "dev", Group: "velero.io", Version: "v1", Resource: "backupstoragelocations", Namespace: "velero", Name: "storage", UID: "uid"}
	r := resourceactions.New(StorageProvider{})
	plan, err := r.Prepare(context.Background(), c, ref, "velero.storage.revalidate", "location")
	if err != nil {
		t.Fatal(err)
	}
	results, err := r.Execute(context.Background(), c, plan)
	if err != nil || results[0].Status != "requested" {
		t.Fatalf("%+v %v", results, err)
	}
	latest, _ := c.Resource(ref.GVR()).Namespace(ref.Namespace).Get(context.Background(), ref.Name, metav1.GetOptions{})
	if _, found, _ := unstructured.NestedFieldNoCopy(latest.Object, "status", "lastValidationTime"); found {
		t.Fatal("validation timestamp retained")
	}
	phase, _, _ := unstructured.NestedString(latest.Object, "status", "phase")
	frequency, _, _ := unstructured.NestedString(latest.Object, "spec", "validationFrequency")
	if phase != "Unavailable" || frequency != "0s" {
		t.Fatal(latest.Object)
	}
	for _, a := range c.Actions() {
		if a.GetVerb() == "patch" && a.GetSubresource() != "" {
			t.Fatal("unexpected status endpoint")
		}
	}
}
