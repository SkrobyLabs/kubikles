package certmanager

import (
	"context"
	"testing"

	"kubikles/pkg/resourceactions"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
)

func TestIssuerDiscoveryHonorsKindGroupAndNamespace(t *testing.T) {
	for _, cluster := range []bool{false, true} {
		t.Run(map[bool]string{true: "cluster", false: "namespace"}[cluster], func(t *testing.T) {
			provider := IssuerProvider{Cluster: cluster}
			kind := "Issuer"
			ns := "ns"
			if cluster {
				kind = "ClusterIssuer"
				ns = ""
			}
			source := &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "cert-manager.io/v1", "kind": kind, "metadata": map[string]interface{}{"name": "issuer", "namespace": ns, "uid": "issuer-uid"}}}
			objects := []runtime.Object{source}
			for _, item := range []struct{ name, namespace, kind, group string }{{"matching", "ns", kind, ""}, {"other-namespace", "other", kind, ""}, {"other-group", "ns", kind, "external.io"}, {"other-kind", "ns", "ExternalIssuer", ""}} {
				objects = append(objects, &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "cert-manager.io/v1", "kind": "Certificate", "metadata": map[string]interface{}{"name": item.name, "namespace": item.namespace, "uid": item.name}, "spec": map[string]interface{}{"issuerRef": map[string]interface{}{"name": "issuer", "kind": item.kind, "group": item.group}}}})
			}
			c := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{{Group: "cert-manager.io", Version: "v1", Resource: "certificates"}: "CertificateList"}, objects...)
			ref := resourceactions.ResourceRef{Context: "dev", Group: "cert-manager.io", Version: "v1", Resource: provider.resource(), Namespace: ns, Name: "issuer", UID: "issuer-uid"}
			r := resourceactions.New(provider)
			plan, err := r.Prepare(context.Background(), c, ref, provider.Actions(ref.Group, ref.Resource)[0].ID, "all")
			want := 1
			if cluster {
				want = 2
			}
			if err != nil || len(plan.Targets) != want {
				t.Fatalf("%+v %v", plan, err)
			}
		})
	}
}
