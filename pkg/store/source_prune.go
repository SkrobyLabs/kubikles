package store

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
)

// pruneSource wraps a full-object source over custom resources and drops every top-level field
// except apiVersion, kind, metadata and the named roots. A custom resource list that only shows
// a few printer columns then holds a fraction of the object in memory. The pruned objects are
// stored and served as ordinary unstructured objects; detail views fetch the full object on
// demand (GetResourceObject never answers from a pruned shard, see Manager.Lookup).
type pruneSource struct {
	inner Source
	keep  map[string]bool
}

func newPruneSource(inner Source, roots []string) Source {
	keep := map[string]bool{"apiVersion": true, "kind": true, "metadata": true}
	for _, r := range roots {
		keep[r] = true
	}
	return pruneSource{inner: inner, keep: keep}
}

func (s pruneSource) Mode() Mode { return s.inner.Mode() }

func (s pruneSource) prune(o runtime.Object) {
	u, ok := o.(*unstructured.Unstructured)
	if !ok {
		return
	}
	for k := range u.Object {
		if !s.keep[k] {
			delete(u.Object, k)
		}
	}
}

func (s pruneSource) List(ctx context.Context, opts ListOpts, onPage func(Page)) (string, error) {
	return s.inner.List(ctx, opts, func(p Page) {
		for _, it := range p.Items {
			s.prune(it)
		}
		onPage(p)
	})
}

func (s pruneSource) Watch(ctx context.Context, rv string) (watch.Interface, error) {
	w, err := s.inner.Watch(ctx, rv)
	if err != nil {
		return nil, err
	}
	return watch.Filter(w, func(ev watch.Event) (watch.Event, bool) {
		switch ev.Type {
		case watch.Added, watch.Modified, watch.Deleted:
			s.prune(ev.Object)
		}
		return ev, true
	}), nil
}

// scrubSecretSource wraps a full-object source over secrets and removes every value before the
// object reaches a shard: data keys stay (so the key count is available) but their values are
// nil, and stringData is dropped. It is applied to every Full-mode secrets source, so a Table
// shard that falls back to full objects never holds secret values in memory.
type scrubSecretSource struct{ inner Source }

func (s scrubSecretSource) Mode() Mode { return s.inner.Mode() }

func scrubSecret(o runtime.Object) {
	sec, ok := o.(*corev1.Secret)
	if !ok {
		return
	}
	for k := range sec.Data {
		sec.Data[k] = nil
	}
	sec.StringData = nil
}

func (s scrubSecretSource) List(ctx context.Context, opts ListOpts, onPage func(Page)) (string, error) {
	return s.inner.List(ctx, opts, func(p Page) {
		for _, it := range p.Items {
			scrubSecret(it)
		}
		onPage(p)
	})
}

func (s scrubSecretSource) Watch(ctx context.Context, rv string) (watch.Interface, error) {
	w, err := s.inner.Watch(ctx, rv)
	if err != nil {
		return nil, err
	}
	return watch.Filter(w, func(ev watch.Event) (watch.Event, bool) {
		switch ev.Type {
		case watch.Added, watch.Modified, watch.Deleted:
			scrubSecret(ev.Object)
		}
		return ev, true
	}), nil
}
