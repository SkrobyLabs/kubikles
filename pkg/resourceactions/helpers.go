package resourceactions

import (
	"context"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
)

func ReadSource(ctx context.Context, client dynamic.Interface, ref ResourceRef) (*unstructured.Unstructured, error) {
	obj, err := client.Resource(ref.GVR()).Namespace(ref.Namespace).Get(ctx, ref.Name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	if string(obj.GetUID()) != ref.UID {
		return nil, fmt.Errorf("resource was replaced; reopen the action")
	}
	if obj.GetDeletionTimestamp() != nil {
		return nil, fmt.Errorf("resource is being deleted")
	}
	return obj, nil
}
func ObjectTarget(contextName, resource string, obj *unstructured.Unstructured) Target {
	gv := obj.GroupVersionKind().GroupVersion()
	return Target{ResourceRef: ResourceRef{Context: contextName, Group: gv.Group, Version: gv.Version, Resource: resource, Namespace: obj.GetNamespace(), Name: obj.GetName(), UID: string(obj.GetUID())}, Kind: obj.GetKind()}
}

// SetField adds missing parent maps and sets only the requested field. It does
// not replace the surrounding spec/status or discard other controller fields.
func SetField(obj *unstructured.Unstructured, value interface{}, fields ...string) []map[string]interface{} {
	ops := []map[string]interface{}{}
	path := ""
	for i, field := range fields {
		path += "/" + strings.ReplaceAll(strings.ReplaceAll(field, "~", "~0"), "/", "~1")
		if i == len(fields)-1 {
			ops = append(ops, map[string]interface{}{"op": "add", "path": path, "value": value})
			break
		}
		if parent, found, _ := unstructured.NestedFieldNoCopy(obj.Object, fields[:i+1]...); !found || parent == nil {
			ops = append(ops, map[string]interface{}{"op": "add", "path": path, "value": map[string]interface{}{}})
		}
	}
	return ops
}
