package resourceactions

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/google/uuid"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
)

type Registry struct{ providers []Provider }

func New(providers ...Provider) *Registry { return &Registry{providers: providers} }
func (r *Registry) Actions(group, resource string) []Action {
	actions := []Action{}
	for _, p := range r.providers {
		actions = append(actions, p.Actions(group, resource)...)
	}
	return actions
}
func (r *Registry) Prepare(ctx context.Context, client dynamic.Interface, ref ResourceRef, id, mode string) (Plan, error) {
	if ref.Context == "" || ref.Name == "" || ref.UID == "" || ref.Version == "" {
		return Plan{}, fmt.Errorf("resource context, name, UID and version are required")
	}
	for _, p := range r.providers {
		for _, action := range p.Actions(ref.Group, ref.Resource) {
			if action.ID != id {
				continue
			}
			for _, m := range action.Modes {
				if m.ID == mode {
					plan, err := p.Prepare(ctx, client, ref, id, mode)
					if err == nil {
						plan.RequestID = uuid.NewString()
					}
					return plan, err
				}
			}
			return Plan{}, fmt.Errorf("unsupported action scope %q", mode)
		}
	}
	return Plan{}, fmt.Errorf("unsupported resource action %q", id)
}

// Execute re-resolves targets and trusted mutations before writing. Cluster scope
// must still match the preview; selection scopes may choose a subset by identity.
func (r *Registry) Execute(ctx context.Context, client dynamic.Interface, submitted Plan) ([]Result, error) {
	if _, err := uuid.Parse(submitted.RequestID); err != nil {
		return nil, fmt.Errorf("invalid action request ID; refresh the preview")
	}
	fresh, err := r.Prepare(ctx, client, submitted.Source, submitted.ActionID, submitted.Mode)
	if err != nil {
		return nil, err
	}
	if fresh.Fingerprint != submitted.Fingerprint {
		return nil, fmt.Errorf("resource configuration changed; review the action again")
	}
	fresh.RequestID = submitted.RequestID
	selectable := false
	for _, a := range r.Actions(submitted.Source.Group, submitted.Source.Resource) {
		if a.ID == submitted.ActionID {
			for _, m := range a.Modes {
				if m.ID == submitted.Mode {
					selectable = m.SelectTargets
				}
			}
		}
	}
	if len(submitted.Targets) == 0 {
		return nil, fmt.Errorf("select at least one target")
	}
	if !selectable && len(submitted.Targets) != len(fresh.Targets) {
		return nil, fmt.Errorf("affected resources changed; review the action again")
	}
	allowed := map[string]Target{}
	for _, t := range fresh.Targets {
		allowed[t.UID] = t
	}
	seen := map[string]bool{}
	for _, t := range submitted.Targets {
		actual, ok := allowed[t.UID]
		if !ok || seen[t.UID] || !reflect.DeepEqual(actual.ResourceRef, t.ResourceRef) {
			return nil, fmt.Errorf("affected resources changed; review the action again")
		}
		seen[t.UID] = true
	}
	results := make([]Result, 0, len(submitted.Targets))
	for _, selected := range submitted.Targets {
		target := allowed[selected.UID]
		result := Result{Target: target, Status: "failed"}
		api := client.Resource(target.GVR()).Namespace(target.Namespace)
		obj, err := api.Get(ctx, target.Name, metav1.GetOptions{})
		if err == nil && string(obj.GetUID()) != target.UID {
			err = fmt.Errorf("resource was replaced; review the action again")
		}
		if err == nil && obj.GetDeletionTimestamp() != nil {
			err = fmt.Errorf("resource is being deleted")
		}
		if err == nil {
			mutation, mutationErr := r.mutation(fresh, target, obj)
			err = mutationErr
			if err == nil && mutation.Pending {
				result.Status = "pending"
			} else if err == nil && mutation.Create != nil {
				creation := mutation.Create
				creation.Object.SetAnnotations(mergeRequestAnnotations(creation.Object.GetAnnotations(), fresh.RequestID, target.UID))
				creationAPI := client.Resource(creation.Resource).Namespace(creation.Object.GetNamespace())
				created, createErr := creationAPI.Create(ctx, creation.Object, metav1.CreateOptions{})
				if apierrors.IsAlreadyExists(createErr) {
					existing, getErr := creationAPI.Get(ctx, creation.Object.GetName(), metav1.GetOptions{})
					if getErr == nil && existing.GetAnnotations()["kubikles.io/action-request"] == fresh.RequestID && existing.GetAnnotations()["kubikles.io/action-source-uid"] == target.UID {
						created = existing
						createErr = nil
						result.Status = "pending"
					}
				}
				err = createErr
				if err == nil {
					if result.Status != "pending" {
						result.Status = "requested"
					}
					result.Message = fmt.Sprintf("%s %s/%s", created.GetKind(), created.GetNamespace(), created.GetName())
				}
			} else if err == nil {
				ops := []map[string]interface{}{
					{"op": "test", "path": "/metadata/uid", "value": target.UID},
					{"op": "test", "path": "/metadata/resourceVersion", "value": obj.GetResourceVersion()},
				}
				ops = append(ops, mutation.Operations...)
				patch, marshalErr := json.Marshal(ops)
				if marshalErr != nil {
					err = marshalErr
				} else {
					subresources := []string{}
					if mutation.Subresource != "" {
						subresources = append(subresources, mutation.Subresource)
					}
					_, err = api.Patch(ctx, target.Name, types.JSONPatchType, patch, metav1.PatchOptions{}, subresources...)
				}
				if err == nil {
					result.Status = "requested"
				}
			}
		}
		if err != nil {
			result.Error = err.Error()
		}
		results = append(results, result)
	}
	return results, nil
}

func (r *Registry) mutation(plan Plan, target Target, obj *unstructured.Unstructured) (Mutation, error) {
	for _, provider := range r.providers {
		for _, action := range provider.Actions(plan.Source.Group, plan.Source.Resource) {
			if action.ID == plan.ActionID {
				if custom, ok := provider.(Mutator); ok {
					return custom.Mutation(plan, target, obj)
				}
			}
		}
	}
	if len(plan.Annotations) == 0 {
		return Mutation{}, fmt.Errorf("action has no mutation")
	}
	pending := true
	for key, value := range plan.Annotations {
		if obj.GetAnnotations()[key] != value {
			pending = false
		}
	}
	if pending || target.Pending {
		return Mutation{Pending: true}, nil
	}
	ops := []map[string]interface{}{}
	if obj.GetAnnotations() == nil {
		ops = append(ops, map[string]interface{}{"op": "add", "path": "/metadata/annotations", "value": map[string]string{}})
	}
	for key, value := range plan.Annotations {
		path := strings.ReplaceAll(strings.ReplaceAll(key, "~", "~0"), "/", "~1")
		ops = append(ops, map[string]interface{}{"op": "add", "path": "/metadata/annotations/" + path, "value": value})
	}
	return Mutation{Operations: ops}, nil
}

func mergeRequestAnnotations(existing map[string]string, requestID, uid string) map[string]string {
	if existing == nil {
		existing = map[string]string{}
	}
	existing["kubikles.io/action-request"] = requestID
	existing["kubikles.io/action-source-uid"] = uid
	return existing
}
