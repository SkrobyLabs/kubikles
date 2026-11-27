package store

import (
	"context"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/metadata"
)

// metaSource lists and watches PartialObjectMetadata through the metadata
// client, which negotiates protobuf and falls back to JSON.
type metaSource struct {
	md           metadata.Interface
	gvr          schema.GroupVersionResource
	namespace    string
	watchTimeout int64
}

func newMetaSource(md metadata.Interface, gvr schema.GroupVersionResource, namespace string, watchTimeout int64) *metaSource {
	return &metaSource{md: md, gvr: gvr, namespace: namespace, watchTimeout: watchTimeout}
}

func (s *metaSource) Mode() Mode { return ModeMetadata }

func (s *metaSource) resource() metadata.ResourceInterface {
	if s.namespace != "" {
		return s.md.Resource(s.gvr).Namespace(s.namespace)
	}
	return s.md.Resource(s.gvr)
}

func (s *metaSource) List(ctx context.Context, opts ListOpts, onPage func(Page)) (string, error) {
	return listPaged(ctx, func(ctx context.Context, lo metav1.ListOptions) (runtime.Object, error) {
		return s.resource().List(ctx, lo)
	}, opts, func(p Page) {
		for _, it := range p.Items {
			clearManagedFields(it)
		}
		onPage(p)
	})
}

func (s *metaSource) Watch(ctx context.Context, rv string) (watch.Interface, error) {
	return s.resource().Watch(ctx, watchOptions(rv, s.watchTimeout))
}

func clearManagedFields(o runtime.Object) {
	if acc, err := meta.Accessor(o); err == nil && acc.GetManagedFields() != nil {
		acc.SetManagedFields(nil)
	}
}

// convertedSource wraps a full-object source and reduces every object to
// PartialObjectMetadata in process. It is the metadata fallback for clients
// that have no REST configuration (fake clientsets, the debug cluster).
type convertedSource struct {
	inner Source
}

func (s convertedSource) Mode() Mode { return ModeMetadata }

func (s convertedSource) List(ctx context.Context, opts ListOpts, onPage func(Page)) (string, error) {
	return s.inner.List(ctx, opts, func(p Page) {
		out := make([]runtime.Object, 0, len(p.Items))
		for _, it := range p.Items {
			out = append(out, toPartialMetadata(it))
		}
		p.Items = out
		onPage(p)
	})
}

func (s convertedSource) Watch(ctx context.Context, rv string) (watch.Interface, error) {
	w, err := s.inner.Watch(ctx, rv)
	if err != nil {
		return nil, err
	}
	return watch.Filter(w, func(ev watch.Event) (watch.Event, bool) {
		switch ev.Type {
		case watch.Added, watch.Modified, watch.Deleted, watch.Bookmark:
			ev.Object = toPartialMetadata(ev.Object)
		}
		return ev, true
	}), nil
}

// toPartialMetadata copies the identity and bookkeeping fields of any object
// into a PartialObjectMetadata. Objects that expose no metadata are returned
// unchanged.
func toPartialMetadata(o runtime.Object) runtime.Object {
	acc, err := meta.Accessor(o)
	if err != nil {
		return o
	}
	p := &metav1.PartialObjectMetadata{}
	p.APIVersion, p.Kind = "meta.k8s.io/v1", "PartialObjectMetadata"
	p.Name = acc.GetName()
	p.GenerateName = acc.GetGenerateName()
	p.Namespace = acc.GetNamespace()
	p.UID = acc.GetUID()
	p.ResourceVersion = acc.GetResourceVersion()
	p.Generation = acc.GetGeneration()
	p.CreationTimestamp = acc.GetCreationTimestamp()
	p.DeletionTimestamp = acc.GetDeletionTimestamp()
	p.DeletionGracePeriodSeconds = acc.GetDeletionGracePeriodSeconds()
	p.Labels = acc.GetLabels()
	p.Annotations = acc.GetAnnotations()
	p.OwnerReferences = acc.GetOwnerReferences()
	p.Finalizers = acc.GetFinalizers()
	return p
}
