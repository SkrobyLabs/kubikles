package velero

import (
	"context"
	"strings"
	"testing"

	"kubikles/pkg/resourceactions"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
)

func scheduleFixture() (*fake.FakeDynamicClient, resourceactions.ResourceRef) {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "velero.io/v1", "kind": "Schedule", "metadata": map[string]interface{}{"name": "daily", "namespace": "velero", "uid": "schedule-uid", "resourceVersion": "1", "labels": map[string]interface{}{"fallback": "label"}}, "spec": map[string]interface{}{"paused": true, "schedule": "0 0 * * *", "useOwnerReferencesInBackup": true, "template": map[string]interface{}{"includedNamespaces": []interface{}{"app"}, "storageLocation": "storage", "metadata": map[string]interface{}{"labels": map[string]interface{}{"template": "label"}}}}}}
	c := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{{Group: "velero.io", Version: "v1", Resource: "backups"}: "BackupList"}, obj)
	return c, resourceactions.ResourceRef{Context: "dev", Group: "velero.io", Version: "v1", Resource: "schedules", Namespace: "velero", Name: "daily", UID: "schedule-uid"}
}
func TestBackupFromScheduleIsIdempotentAndPreservesTemplate(t *testing.T) {
	c, ref := scheduleFixture()
	r := resourceactions.New(ScheduleProvider{})
	p, err := r.Prepare(context.Background(), c, ref, "velero.schedule.backup", "schedule")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"requested", "pending"} {
		results, err := r.Execute(context.Background(), c, p)
		if err != nil || results[0].Status != want {
			t.Fatalf("%+v %v", results, err)
		}
	}
	list, err := c.Resource(schema.GroupVersionResource{Group: "velero.io", Version: "v1", Resource: "backups"}).Namespace("velero").List(context.Background(), metav1.ListOptions{})
	if err != nil || len(list.Items) != 1 {
		t.Fatalf("%+v %v", list, err)
	}
	backup := list.Items[0]
	if !strings.HasSuffix(backup.GetName(), p.RequestID) || backup.GetLabels()["template"] != "label" || backup.GetLabels()["fallback"] != "" || backup.GetLabels()["velero.io/schedule-name"] != "daily" || len(backup.GetOwnerReferences()) != 1 {
		t.Fatal(backup.Object)
	}
	location, _, _ := unstructured.NestedString(backup.Object, "spec", "storageLocation")
	if location != "storage" {
		t.Fatal(backup.Object)
	}
	latest, _ := c.Resource(ref.GVR()).Namespace("velero").Get(context.Background(), ref.Name, metav1.GetOptions{})
	paused, _, _ := unstructured.NestedBool(latest.Object, "spec", "paused")
	if !paused {
		t.Fatal("one-off backup unpaused schedule")
	}
}
func TestChangedScheduleRequiresNewPreview(t *testing.T) {
	c, ref := scheduleFixture()
	r := resourceactions.New(ScheduleProvider{})
	p, _ := r.Prepare(context.Background(), c, ref, "velero.schedule.backup", "schedule")
	obj, _ := c.Resource(ref.GVR()).Namespace("velero").Get(context.Background(), ref.Name, metav1.GetOptions{})
	unstructured.SetNestedField(obj.Object, "other", "spec", "template", "storageLocation")
	c.Resource(ref.GVR()).Namespace("velero").Update(context.Background(), obj, metav1.UpdateOptions{})
	if _, err := r.Execute(context.Background(), c, p); err == nil {
		t.Fatal("configuration drift accepted")
	}
	for _, a := range c.Actions() {
		if a.GetVerb() == "create" {
			t.Fatal("created backup after drift")
		}
	}
}
