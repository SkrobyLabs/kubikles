package k8s

import "context"

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

func emitListPage(ctx context.Context, items interface{}, loaded int, remaining *int64, more bool) {
	observer, ok := ctx.Value(listPageObserverKey{}).(func(ListPage))
	if !ok || ctx.Err() != nil {
		return
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
}
