package k8s

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ListPage carries only the newly received page. Total is nil when the API
// server does not supply a remaining-item count and more pages exist.
type ListPage struct {
	Items  interface{} `json:"items"`
	Loaded int         `json:"loaded"`
	Total  *int        `json:"total"`
}

type listPageObserverKey struct{}

// WithListPageObserver opts a caller into incremental results without changing
// the final list response used by non-streaming clients and reconciliation.
func WithListPageObserver(ctx context.Context, observer func(ListPage)) context.Context {
	return context.WithValue(ctx, listPageObserverKey{}, observer)
}

// emitListPage reports whether the page was handed to an observer. Callers use
// that to decide whether the final result may omit items the observer already
// holds.
func emitListPage(ctx context.Context, items interface{}, loaded int, remaining *int64, more bool) bool {
	observer, ok := ctx.Value(listPageObserverKey{}).(func(ListPage))
	if !ok || ctx.Err() != nil {
		return false
	}
	var total *int
	if remaining != nil {
		value := loaded + int(*remaining)
		total = &value
	} else if !more {
		value := loaded
		total = &value
	}
	observer(ListPage{Items: items, Loaded: loaded, Total: total})
	return true
}

// ListOutcome describes a completed list call. It is delivered out of band
// because the list bindings return plain item arrays: when every page was
// streamed to an observer the array is empty, and the caller needs the outcome
// to tell that apart from a genuinely empty list and to anchor a watch.
type ListOutcome struct {
	// ResourceVersion is the list's snapshot version; a watch started from it
	// sees exactly the changes after the list. Empty when the list path did not
	// report one.
	ResourceVersion string `json:"resourceVersion"`
	// Streamed is true when all items were delivered through list pages and the
	// returned array is empty.
	Streamed bool `json:"streamed"`
	// Loaded is the total number of items listed.
	Loaded int `json:"loaded"`
}

type listOutcomeKey struct{}

type listOutcomeRecorder struct {
	sink func(ListOutcome)
	rv   string
}

// WithListOutcomeSink asks list calls under ctx to report their ListOutcome
// once, on success. One context must serve one list call.
func WithListOutcomeSink(ctx context.Context, sink func(ListOutcome)) context.Context {
	return context.WithValue(ctx, listOutcomeKey{}, &listOutcomeRecorder{sink: sink})
}

// noteListResourceVersion records the version of the list being fetched. All
// pages of one paginated list share the first page's snapshot version, so only
// the first non-empty value is kept.
func noteListResourceVersion(ctx context.Context, rv string) {
	if rec, ok := ctx.Value(listOutcomeKey{}).(*listOutcomeRecorder); ok && rec.rv == "" {
		rec.rv = rv
	}
}

func reportListOutcome(ctx context.Context, loaded int, streamed bool) {
	if rec, ok := ctx.Value(listOutcomeKey{}).(*listOutcomeRecorder); ok && rec.sink != nil {
		rec.sink(ListOutcome{ResourceVersion: rec.rv, Streamed: streamed, Loaded: loaded})
	}
}

// stripManagedFields clears metadata.managedFields on every item. The
// frontend never reads it (it is only needed by server-side apply tooling) and
// it routinely accounts for a large share of each object's JSON, so dropping it
// here shrinks every list payload that crosses the Go/JS bridge. The Get*Yaml
// paths strip it themselves. Items are freshly decoded, so mutating in place
// is safe. Typed objects are handled through metav1.Object, unstructured
// items (custom resources) through their metadata map.
func stripManagedFields[T any](items []T) {
	for i := range items {
		switch o := any(&items[i]).(type) {
		case metav1.Object:
			if o.GetManagedFields() != nil {
				o.SetManagedFields(nil)
			}
		case *map[string]interface{}:
			if md, ok := (*o)["metadata"].(map[string]interface{}); ok {
				delete(md, "managedFields")
			}
		}
	}
}
