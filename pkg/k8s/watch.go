// Code split from client.go; see that file for the Client type and lifecycle.
package k8s

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
)

func (c *Client) WatchResource(ctx context.Context, resourceType, namespace, resourceVersion string) (watch.Interface, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}

	opts := metav1.ListOptions{
		TimeoutSeconds:      ptr(WatchTimeout),
		AllowWatchBookmarks: true,
		ResourceVersion:     resourceVersion,
	}

	d, ok := LookupResource(resourceType)
	if !ok {
		return nil, fmt.Errorf("unsupported resource type: %s", resourceType)
	}
	return d.Watch(ctx, cs, namespace, opts)
}

// WatchCRD creates a watch for a custom resource using the dynamic client.
// resourceVersion: if non-empty, resumes watch from this version (avoids duplicate ADDED events)
func (c *Client) WatchCRD(ctx context.Context, group, version, resource, namespace, resourceVersion string) (watch.Interface, error) {
	dc, err := c.getDynamicClientForContext("")
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}

	gvr := schema.GroupVersionResource{
		Group:    group,
		Version:  version,
		Resource: resource,
	}

	opts := metav1.ListOptions{
		TimeoutSeconds:      ptr(WatchTimeout),
		AllowWatchBookmarks: true,
		ResourceVersion:     resourceVersion,
	}

	if namespace != "" {
		return dc.Resource(gvr).Namespace(namespace).Watch(ctx, opts)
	}
	return dc.Resource(gvr).Watch(ctx, opts)
}

// WatchObjectMeta is the identity of a watched object, read through
// meta.Accessor so callers never need a decoded map.
type WatchObjectMeta struct {
	Name            string
	Namespace       string
	ResourceVersion string
}

// WatchObjectInfo extracts identity metadata from a watch event object without
// serializing it. It is all a BOOKMARK event needs.
func WatchObjectInfo(obj runtime.Object) (WatchObjectMeta, error) {
	acc, err := meta.Accessor(obj)
	if err != nil {
		return WatchObjectMeta{}, err
	}
	return WatchObjectMeta{
		Name:            acc.GetName(),
		Namespace:       acc.GetNamespace(),
		ResourceVersion: acc.GetResourceVersion(),
	}, nil
}

// EncodeWatchObject strips managedFields and marshals the object to JSON
// exactly once. The returned bytes travel as json.RawMessage so the Wails
// bridge only validates and copies them instead of re-walking a map, which
// previously cost a YAML round trip plus a second marshal per event.
//
// The object must be owned by the caller (watch decoders hand out a fresh
// object per event), because managedFields are cleared in place.
func EncodeWatchObject(obj runtime.Object) (WatchObjectMeta, json.RawMessage, error) {
	acc, err := meta.Accessor(obj)
	if err != nil {
		return WatchObjectMeta{}, nil, err
	}
	if acc.GetManagedFields() != nil {
		acc.SetManagedFields(nil)
	}
	info := WatchObjectMeta{
		Name:            acc.GetName(),
		Namespace:       acc.GetNamespace(),
		ResourceVersion: acc.GetResourceVersion(),
	}

	var payload interface{} = obj
	if u, ok := obj.(*unstructured.Unstructured); ok {
		// Marshal the content map directly: Unstructured.MarshalJSON appends a
		// newline and goes through the scheme, neither of which we want here.
		payload = u.Object
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return info, nil, fmt.Errorf("failed to marshal watch object: %w", err)
	}
	return info, raw, nil
}

// StripManagedFieldsObject clears metadata.managedFields in place on a typed
// or unstructured object. Objects kept in the shared store call it on ingest;
// the field is large and no list or watch consumer reads it.
func StripManagedFieldsObject(obj runtime.Object) {
	switch o := obj.(type) {
	case *unstructured.Unstructured:
		if md, ok := o.Object["metadata"].(map[string]interface{}); ok {
			delete(md, "managedFields")
		}
	case metav1.Object:
		if o.GetManagedFields() != nil {
			o.SetManagedFields(nil)
		}
	default:
		if acc, err := meta.Accessor(obj); err == nil && acc.GetManagedFields() != nil {
			acc.SetManagedFields(nil)
		}
	}
}

// IsStaleResourceVersionError reports whether a watch open failed because the
// requested resourceVersion is no longer (or never was) usable.
func IsStaleResourceVersionError(err error) bool {
	if apierrors.IsGone(err) || apierrors.IsResourceExpired(err) || apierrors.IsInvalid(err) || apierrors.IsBadRequest(err) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "too old") || strings.Contains(msg, "expired")
}
