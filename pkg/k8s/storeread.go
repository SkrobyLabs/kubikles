package k8s

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
)

// StoreReader serves lists from warm store shards. The store implements it (pkg/store) and the
// application installs it with Client.SetStoreReader; pkg/k8s cannot import the store.
//
// ReadObjects never starts a LIST: it answers only when a synced, watching shard of full typed
// objects exists for the type (a cluster-wide shard may serve a namespace), and reports ok=false
// otherwise. The returned objects are shared with the store and must be treated as read-only.
type StoreReader interface {
	ReadObjects(contextName, resourceType, namespace string) ([]runtime.Object, bool)
}

// StoreCounter is the optional part of a StoreReader that sizes shards of any representation
// (Table and metadata shards know how many objects exist even though they hold no spec).
type StoreCounter interface {
	CountObjects(contextName, resourceType, namespace string) (int, bool)
}

type storeReaderBox struct{ r StoreReader }

// SetStoreReader installs (or with nil removes) the warm-store reader.
func (c *Client) SetStoreReader(r StoreReader) {
	c.storeReader.Store(&storeReaderBox{r: r})
}

func (c *Client) readerOrNil() StoreReader {
	if b := c.storeReader.Load(); b != nil {
		return b.r
	}
	return nil
}

type storeReadKey struct{}

// WithStoreRead marks a context as one whose lists may be answered from the warm store. Lists
// that feed the UI never set it: they either run through a view (which owns its shard) or must
// see the API server's answer.
func WithStoreRead(ctx context.Context) context.Context {
	return context.WithValue(ctx, storeReadKey{}, true)
}

// StoreReadEnabled reports whether ctx allows warm-store reads.
func StoreReadEnabled(ctx context.Context) bool {
	v, _ := ctx.Value(storeReadKey{}).(bool)
	return v
}

// StoreList returns the warm store's objects of resourceType (and namespace, "" for all) as
// values of T, or ok=false when ctx does not allow store reads, no reader is installed, the
// shard is cold or the objects are not *T. Items are shallow copies: maps and slices inside
// them are shared with the store and must not be mutated.
//
// It is a package function because Go methods cannot be generic.
func StoreList[T any](ctx context.Context, c *Client, contextName, resourceType, namespace string) ([]T, bool) {
	if c == nil || !StoreReadEnabled(ctx) {
		return nil, false
	}
	r := c.readerOrNil()
	if r == nil {
		return nil, false
	}
	return storeListFrom[T](r, contextName, resourceType, namespace)
}

func storeListFrom[T any](r StoreReader, contextName, resourceType, namespace string) ([]T, bool) {
	objs, ok := r.ReadObjects(contextName, resourceType, namespace)
	if !ok {
		return nil, false
	}
	out := make([]T, 0, len(objs))
	for _, o := range objs {
		p, isT := any(o).(*T)
		if !isT {
			return nil, false
		}
		out = append(out, *p)
	}
	return out, true
}

// NewClientWithClientset wraps an existing clientset in a Client for the given context name. It
// exists for tests of packages that consume Client (the issue detector, the tools) and for
// embedding a fake cluster; it performs no connection setup.
func NewClientWithClientset(cs kubernetes.Interface, contextName string) *Client {
	return &Client{clientset: cs, currentContext: contextName}
}
