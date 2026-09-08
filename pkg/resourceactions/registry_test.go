package resourceactions

import (
	"context"
	"fmt"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/fake"
	kt "k8s.io/client-go/testing"
)

type testProvider struct{ targets []Target }

func (p *testProvider) Actions(group, resource string) []Action {
	if group != "test.io" || resource != "clusters" {
		return nil
	}
	return []Action{{ID: "restart", Modes: []Mode{{ID: "all"}, {ID: "selected", SelectTargets: true}}}}
}
func (p *testProvider) Prepare(_ context.Context, _ dynamic.Interface, ref ResourceRef, id, mode string) (Plan, error) {
	return Plan{Source: ref, ActionID: id, Mode: mode, Targets: p.targets, Annotations: map[string]string{"operator/restart": "true"}}, nil
}
func fixture() (*Registry, *fake.FakeDynamicClient, Plan, *testProvider) {
	source := ResourceRef{Context: "cluster-a", Group: "test.io", Version: "v1", Resource: "clusters", Namespace: "ns", Name: "cluster", UID: "source"}
	target := Target{ResourceRef: ResourceRef{Context: "cluster-a", Version: "v1", Resource: "pods", Namespace: "ns", Name: "pod", UID: "pod-uid"}, Kind: "Pod"}
	obj := &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "v1", "kind": "Pod", "metadata": map[string]interface{}{"name": "pod", "namespace": "ns", "uid": "pod-uid", "resourceVersion": "1", "annotations": map[string]interface{}{"keep": "value"}}}}
	client := fake.NewSimpleDynamicClient(runtime.NewScheme(), obj)
	provider := &testProvider{targets: []Target{target}}
	registry := New(provider)
	plan, _ := registry.Prepare(context.Background(), client, source, "restart", "all")
	plan.Targets = append([]Target(nil), plan.Targets...)
	return registry, client, plan, provider
}
func TestExecutePreservesAnnotationsAndDetectsPending(t *testing.T) {
	r, c, p, _ := fixture()
	p.Annotations = map[string]string{"untrusted": "value"}
	results, err := r.Execute(context.Background(), c, p)
	if err != nil || results[0].Status != "requested" {
		t.Fatalf("%+v %v", results, err)
	}
	obj, _ := c.Resource(p.Targets[0].GVR()).Namespace("ns").Get(context.Background(), "pod", metav1.GetOptions{})
	if obj.GetAnnotations()["keep"] != "value" || obj.GetAnnotations()["operator/restart"] != "true" || obj.GetAnnotations()["untrusted"] != "" {
		t.Fatal(obj.GetAnnotations())
	}
	results, err = r.Execute(context.Background(), c, p)
	if err != nil || results[0].Status != "pending" {
		t.Fatalf("%+v %v", results, err)
	}
}
func TestExecuteRejectsChangedOrForgedTargetsBeforeAnyWrite(t *testing.T) {
	for _, scenario := range []string{"replacement", "context", "duplicate", "new-target", "empty", "invalid-mode"} {
		t.Run(scenario, func(t *testing.T) {
			r, c, p, provider := fixture()
			switch scenario {
			case "replacement":
				p.Targets[0].UID = "other"
			case "context":
				p.Targets[0].Context = "cluster-b"
			case "duplicate":
				p.Mode = "selected"
				p.Targets = append(p.Targets, p.Targets[0])
			case "new-target":
				provider.targets = append(provider.targets, Target{ResourceRef: ResourceRef{UID: "new"}})
			case "empty":
				p.Targets = nil
			case "invalid-mode":
				p.Mode = "other"
			}
			if _, err := r.Execute(context.Background(), c, p); err == nil {
				t.Fatal("expected rejection")
			}
			for _, a := range c.Actions() {
				if a.GetVerb() == "patch" {
					t.Fatal("unexpected mutation")
				}
			}
		})
	}
}
func TestExecuteReportsPartialFailures(t *testing.T) {
	r, c, p, provider := fixture()
	second := p.Targets[0]
	second.Name = "other"
	second.UID = "other-uid"
	obj := &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "v1", "kind": "Pod", "metadata": map[string]interface{}{"name": "other", "namespace": "ns", "uid": "other-uid", "resourceVersion": "1"}}}
	c.Tracker().Create(schema.GroupVersionResource{Version: "v1", Resource: "pods"}, obj, "ns")
	provider.targets = append(provider.targets, second)
	p.Targets = provider.targets
	c.PrependReactor("patch", "pods", func(a kt.Action) (bool, runtime.Object, error) {
		if a.(kt.PatchAction).GetName() == "other" {
			return true, nil, fmt.Errorf("forbidden")
		}
		return false, nil, nil
	})
	results, err := r.Execute(context.Background(), c, p)
	if err != nil || len(results) != 2 || results[0].Status != "requested" || results[1].Status != "failed" {
		t.Fatalf("%+v %v", results, err)
	}
}

func TestConcurrentReplacementFailsPatchPrecondition(t *testing.T) {
	r, c, p, _ := fixture()
	c.PrependReactor("patch", "pods", func(a kt.Action) (bool, runtime.Object, error) {
		// Simulate replacement after the executor's GET but before its PATCH.
		obj, _ := c.Tracker().Get(p.Targets[0].GVR(), "ns", "pod")
		replacement := obj.(*unstructured.Unstructured).DeepCopy()
		replacement.SetUID("replacement-uid")
		replacement.SetResourceVersion("2")
		if err := c.Tracker().Update(p.Targets[0].GVR(), replacement, "ns"); err != nil {
			t.Fatal(err)
		}
		return false, nil, nil
	})
	result, err := r.Execute(context.Background(), c, p)
	if err != nil || result[0].Status != "failed" {
		t.Fatalf("%+v %v", result, err)
	}
	obj, _ := c.Resource(p.Targets[0].GVR()).Namespace("ns").Get(context.Background(), "pod", metav1.GetOptions{})
	if obj.GetAnnotations()["operator/restart"] != "" {
		t.Fatal("mutated replacement resource")
	}
}
