package store

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"

	"kubikles/pkg/k8s"
)

// dynamicSource lists and watches a custom resource as unstructured objects.
type dynamicSource struct {
	dc           dynamic.Interface
	gvr          schema.GroupVersionResource
	namespace    string
	watchTimeout int64
}

func newDynamicSource(dc dynamic.Interface, gvr schema.GroupVersionResource, namespace string, watchTimeout int64) *dynamicSource {
	return &dynamicSource{dc: dc, gvr: gvr, namespace: namespace, watchTimeout: watchTimeout}
}

func (s *dynamicSource) Mode() Mode { return ModeFull }

func (s *dynamicSource) resource() dynamic.ResourceInterface {
	if s.namespace != "" {
		return s.dc.Resource(s.gvr).Namespace(s.namespace)
	}
	return s.dc.Resource(s.gvr)
}

func (s *dynamicSource) List(ctx context.Context, opts ListOpts, onPage func(Page)) (string, error) {
	return listPaged(ctx, func(ctx context.Context, lo metav1.ListOptions) (runtime.Object, error) {
		return s.resource().List(ctx, lo)
	}, opts, func(p Page) {
		for _, it := range p.Items {
			k8s.StripManagedFieldsObject(it)
		}
		onPage(p)
	})
}

func (s *dynamicSource) Watch(ctx context.Context, rv string) (watch.Interface, error) {
	return s.resource().Watch(ctx, watchOptions(rv, s.watchTimeout))
}
