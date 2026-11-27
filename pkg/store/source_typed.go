package store

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"

	"kubikles/pkg/k8s"
)

// typedSource lists and watches a built-in type through a typed clientset. The
// per-type wiring lives in the shared k8s.ResourceDef table, the same one
// Client.WatchResource uses. It works on kubernetes/fake, so the debug cluster
// and tests need nothing else.
type typedSource struct {
	cs           kubernetes.Interface
	def          k8s.ResourceDef
	namespace    string
	watchTimeout int64
}

func newTypedSource(cs kubernetes.Interface, resourceType, namespace string, watchTimeout int64) (*typedSource, error) {
	def, ok := k8s.LookupResource(resourceType)
	if !ok {
		return nil, fmt.Errorf("unsupported resource type: %s", resourceType)
	}
	if !def.Namespaced {
		namespace = ""
	}
	return &typedSource{cs: cs, def: def, namespace: namespace, watchTimeout: watchTimeout}, nil
}

func (s *typedSource) Mode() Mode { return ModeFull }

func (s *typedSource) List(ctx context.Context, opts ListOpts, onPage func(Page)) (string, error) {
	return listPaged(ctx, func(ctx context.Context, lo metav1.ListOptions) (runtime.Object, error) {
		return s.def.List(ctx, s.cs, s.namespace, lo)
	}, opts, func(p Page) {
		for _, it := range p.Items {
			k8s.StripManagedFieldsObject(it)
		}
		onPage(p)
	})
}

func (s *typedSource) Watch(ctx context.Context, rv string) (watch.Interface, error) {
	return s.def.Watch(ctx, s.cs, s.namespace, watchOptions(rv, s.watchTimeout))
}
