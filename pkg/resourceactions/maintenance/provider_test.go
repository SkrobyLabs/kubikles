package maintenance

import (
	"context"
	"testing"

	"kubikles/pkg/resourceactions"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic/fake"
)

func TestPauseResumeAndRestartPreserveSpec(t *testing.T) {
	for _, field := range []string{"paused", "managementState"} {
		t.Run(field, func(t *testing.T) {
			provider := Provider{Group: "test.io", Resource: "workloads", Versions: []string{"v1"}, PauseField: field, PausedValue: true, RunningValue: false, PodAnnotations: []string{"spec", "podMetadata", "annotations"}}
			if field == "managementState" {
				provider.PausedValue = "unmanaged"
				provider.RunningValue = "managed"
				provider.PodAnnotations = []string{"spec", "podAnnotations"}
			}
			obj := &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "test.io/v1", "kind": "Workload", "metadata": map[string]interface{}{"name": "app", "namespace": "ns", "uid": "uid", "resourceVersion": "1"}, "spec": map[string]interface{}{"replicas": int64(3), "podMetadata": map[string]interface{}{"annotations": map[string]interface{}{"keep": "yes"}}}}}
			c := fake.NewSimpleDynamicClient(runtime.NewScheme(), obj)
			ref := resourceactions.ResourceRef{Context: "dev", Group: "test.io", Version: "v1", Resource: "workloads", Namespace: "ns", Name: "app", UID: "uid"}
			r := resourceactions.New(provider)
			for _, operation := range []string{"pause", "resume", "restart"} {
				p, err := r.Prepare(context.Background(), c, ref, "test.io.workloads."+operation, "resource")
				if err != nil {
					t.Fatal(err)
				}
				results, err := r.Execute(context.Background(), c, p)
				if err != nil || results[0].Status != "requested" {
					t.Fatalf("%+v %v", results, err)
				}
				replay, replayErr := r.Execute(context.Background(), c, p)
				if replayErr != nil || replay[0].Status != "pending" {
					t.Fatalf("request replay: %+v %v", replay, replayErr)
				}
				if operation == "pause" {
					if _, err := r.Prepare(context.Background(), c, ref, "test.io.workloads.restart", "resource"); err == nil {
						t.Fatal("restart while paused")
					}
				}
			}
			latest, _ := c.Resource(ref.GVR()).Namespace("ns").Get(context.Background(), ref.Name, metav1.GetOptions{})
			replicas, _, _ := unstructured.NestedInt64(latest.Object, "spec", "replicas")
			keep, _, _ := unstructured.NestedString(latest.Object, "spec", "podMetadata", "annotations", "keep")
			if replicas != 3 || keep != "yes" {
				t.Fatal(latest.Object)
			}
			key := append(provider.PodAnnotations, "kubikles.io/restart-request")
			stamp, _, _ := unstructured.NestedString(latest.Object, key...)
			if stamp == "" {
				t.Fatal("missing restart request")
			}
		})
	}
}
func TestSidecarRestartRejected(t *testing.T) {
	p := Provider{SidecarMode: true}
	obj := &unstructured.Unstructured{Object: map[string]interface{}{"spec": map[string]interface{}{"mode": "sidecar"}}}
	if p.validateRestart(obj) == nil {
		t.Fatal("sidecar restart must require application workload selection")
	}
}
