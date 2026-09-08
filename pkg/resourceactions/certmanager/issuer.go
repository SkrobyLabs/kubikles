package certmanager

import (
	"context"
	"fmt"
	"sort"

	"kubikles/pkg/resourceactions"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// IssuerProvider supports namespace-scoped issuers. Cluster issuers opt in via
// registration so each resource type can evolve independently.
type IssuerProvider struct{ Cluster bool }

func (p IssuerProvider) resource() string {
	if p.Cluster {
		return "clusterissuers"
	}
	return "issuers"
}
func (p IssuerProvider) Actions(group, resource string) []resourceactions.Action {
	if group != "cert-manager.io" || resource != p.resource() {
		return nil
	}
	return []resourceactions.Action{{ID: "cert-manager." + p.resource() + ".renew-certificates", Label: "Renew certificates…", Description: "Request renewal of certificates referencing this issuer. Existing certificates remain available while cert-manager processes issuance.", Modes: []resourceactions.Mode{{ID: "all", Label: "All associated certificates"}, {ID: "selected", Label: "Selected certificates", SelectTargets: true}}}}
}
func (p IssuerProvider) matches(ref resourceactions.ResourceRef, obj *unstructured.Unstructured) bool {
	name, _, _ := unstructured.NestedString(obj.Object, "spec", "issuerRef", "name")
	group, _, _ := unstructured.NestedString(obj.Object, "spec", "issuerRef", "group")
	if group == "" {
		group = "cert-manager.io"
	}
	kind, _, _ := unstructured.NestedString(obj.Object, "spec", "issuerRef", "kind")
	if kind == "" {
		kind = "Issuer"
	}
	expected := "Issuer"
	if p.Cluster {
		expected = "ClusterIssuer"
	}
	return name == ref.Name && group == ref.Group && kind == expected && (p.Cluster || obj.GetNamespace() == ref.Namespace)
}
func (p IssuerProvider) Prepare(ctx context.Context, client dynamic.Interface, ref resourceactions.ResourceRef, id, mode string) (resourceactions.Plan, error) {
	plan := resourceactions.Plan{Source: ref, ActionID: id, Mode: mode, Targets: []resourceactions.Target{}, Summary: "Each certificate gets a guarded, atomic renewal request. Multiple certificates are processed individually; any failures are reported per certificate. The issuer decides whether fresh ACME challenges are required."}
	if ref.Version != "v1" {
		return plan, fmt.Errorf("manual renewal requires the cert-manager v1 API")
	}
	if _, err := resourceactions.ReadSource(ctx, client, ref); err != nil {
		return plan, err
	}
	namespace := ref.Namespace
	if p.Cluster {
		namespace = ""
	} else if namespace == "" {
		return plan, fmt.Errorf("Issuer namespace is required")
	}
	// Follow continuation tokens so an 'all' preview never silently omits objects.
	options := metav1.ListOptions{Limit: 500}
	for {
		list, err := client.Resource(schema.GroupVersionResource{Group: "cert-manager.io", Version: "v1", Resource: "certificates"}).Namespace(namespace).List(ctx, options)
		if err != nil {
			return plan, fmt.Errorf("cannot list associated certificates: %w", err)
		}
		for i := range list.Items {
			obj := &list.Items[i]
			if !p.matches(ref, obj) || obj.GetDeletionTimestamp() != nil {
				continue
			}
			target := resourceactions.ObjectTarget(ref.Context, "certificates", obj)
			target.Pending = issuing(obj)
			plan.Targets = append(plan.Targets, target)
		}
		options.Continue = list.GetContinue()
		if options.Continue == "" {
			break
		}
	}
	if len(plan.Targets) == 0 {
		return plan, fmt.Errorf("no certificates reference this issuer")
	}
	sort.Slice(plan.Targets, func(i, j int) bool {
		return plan.Targets[i].Namespace+"/"+plan.Targets[i].Name < plan.Targets[j].Namespace+"/"+plan.Targets[j].Name
	})
	return plan, nil
}
func (p IssuerProvider) Mutation(plan resourceactions.Plan, target resourceactions.Target, obj *unstructured.Unstructured) (resourceactions.Mutation, error) {
	if !p.matches(plan.Source, obj) {
		return resourceactions.Mutation{}, fmt.Errorf("certificate issuer changed; review the action again")
	}
	return (Provider{}).Mutation(plan, target, obj)
}
