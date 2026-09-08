package k8s

import (
	"context"
	"os"
	"testing"
	"time"

	"kubikles/pkg/resourceactions"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// This optional integration check only lists resources and prepares previews.
// It never calls ExecuteResourceAction or sends a Kubernetes mutation.
func TestResourceActionLivePreviews(t *testing.T) {
	contextName := os.Getenv("KUBIKLES_ACTION_PREVIEW_CONTEXT")
	if contextName == "" {
		t.Skip("set KUBIKLES_ACTION_PREVIEW_CONTEXT for read-only live previews")
	}
	c := &Client{}
	dc, err := c.getDynamicClientForContext(contextName)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	crds, err := c.ListCRDsWithContext(ctx, contextName)
	if err != nil {
		t.Fatal(err)
	}
	kinds, previews := 0, 0
	for _, crd := range crds {
		actions := operatorActions.Actions(crd.Spec.Group, crd.Spec.Names.Plural)
		if len(actions) == 0 {
			continue
		}
		kinds++
		version := ""
		for _, v := range crd.Spec.Versions {
			if v.Storage {
				version = v.Name
			}
		}
		ref := resourceactions.ResourceRef{Context: contextName, Group: crd.Spec.Group, Version: version, Resource: crd.Spec.Names.Plural}
		list, err := dc.Resource(ref.GVR()).List(ctx, metav1.ListOptions{Limit: 1})
		if err != nil {
			t.Errorf("%s discovery: %v", crd.Spec.Names.Kind, err)
			continue
		}
		if len(list.Items) == 0 {
			t.Logf("%s: registered, no instances to preview", crd.Spec.Names.Kind)
			continue
		}
		obj := list.Items[0]
		ref.Name = obj.GetName()
		ref.Namespace = obj.GetNamespace()
		ref.UID = string(obj.GetUID())
		for _, action := range actions {
			for _, mode := range action.Modes {
				plan, err := operatorActions.Prepare(ctx, dc, ref, action.ID, mode.ID)
				if err != nil {
					t.Logf("%s / %s: unavailable: %v", crd.Spec.Names.Kind, action.ID, err)
					continue
				}
				previews++
				t.Logf("%s / %s / %s: %d targets", crd.Spec.Names.Kind, action.ID, mode.ID, len(plan.Targets))
			}
		}
	}
	t.Logf("%d supported installed resource types; %d successful read-only previews", kinds, previews)
}
