package certmanager

import (
	"context"
	"testing"

	"kubikles/pkg/resourceactions"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic/fake"
)

func TestRenewCertificateUsesAtomicStatusPatch(t *testing.T) {
	for _, hasStatus := range []bool{true, false} {
		t.Run(map[bool]string{true: "existing conditions", false: "no status"}[hasStatus], func(t *testing.T) {
			obj := &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "cert-manager.io/v1", "kind": "Certificate", "metadata": map[string]interface{}{"name": "cert", "namespace": "ns", "uid": "cert-uid", "resourceVersion": "1", "generation": int64(3)}, "spec": map[string]interface{}{"secretName": "tls"}}}
			if hasStatus {
				obj.Object["status"] = map[string]interface{}{"revision": int64(2), "conditions": []interface{}{map[string]interface{}{"type": "Ready", "status": "True"}, map[string]interface{}{"type": "Issuing", "status": "False"}}}
			}
			c := fake.NewSimpleDynamicClient(runtime.NewScheme(), obj)
			ref := resourceactions.ResourceRef{Context: "cluster", Group: "cert-manager.io", Version: "v1", Resource: "certificates", Namespace: "ns", Name: "cert", UID: "cert-uid"}
			r := resourceactions.New(Provider{})
			p, err := r.Prepare(context.Background(), c, ref, renewID, "certificate")
			if err != nil {
				t.Fatal(err)
			}
			results, err := r.Execute(context.Background(), c, p)
			if err != nil || results[0].Status != "requested" {
				t.Fatalf("%+v %v", results, err)
			}
			latest, _ := c.Resource(ref.GVR()).Namespace("ns").Get(context.Background(), ref.Name, metav1.GetOptions{})
			if !issuing(latest) {
				t.Fatal("renewal was not requested")
			}
			if hasStatus {
				revision, _, _ := unstructured.NestedInt64(latest.Object, "status", "revision")
				if revision != 2 {
					t.Fatal("lost revision")
				}
				conditions, _, _ := unstructured.NestedSlice(latest.Object, "status", "conditions")
				if len(conditions) != 2 || conditions[0].(map[string]interface{})["type"] != "Ready" {
					t.Fatal(conditions)
				}
			}
			patches := 0
			for _, a := range c.Actions() {
				if a.GetVerb() == "patch" {
					patches++
					if a.GetSubresource() != "status" || a.GetResource().Resource != "certificates" {
						t.Fatal(a)
					}
				}
			}
			if patches != 1 {
				t.Fatalf("expected one atomic status patch, got %d", patches)
			}
			results, err = r.Execute(context.Background(), c, p)
			if err != nil || results[0].Status != "pending" {
				t.Fatalf("%+v %v", results, err)
			}
		})
	}
}
