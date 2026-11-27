package store

import (
	"context"
	"fmt"
	"reflect"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
)

// watchTimeoutSeconds matches the legacy watchers (k8s.WatchTimeout).
const defaultWatchTimeout int64 = 300

// listFn performs one LIST call with the given options.
type listFn func(ctx context.Context, opts metav1.ListOptions) (runtime.Object, error)

// watchFn opens one watch with the given options.
type watchFn func(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error)

// listPaged drives a paginated LIST over fn. It returns the snapshot
// resourceVersion of the first page, which every later page shares.
func listPaged(ctx context.Context, fn listFn, opts ListOpts, onPage func(Page)) (string, error) {
	var (
		rv     string
		loaded int
		cont   string
	)
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		lo := metav1.ListOptions{Continue: cont}
		if opts.PageSize > 0 {
			lo.Limit = opts.PageSize
		}
		list, err := fn(ctx, lo)
		if err != nil {
			return "", err
		}
		la, err := meta.ListAccessor(list)
		if err != nil {
			return "", fmt.Errorf("list result is not a list: %w", err)
		}
		if rv == "" {
			rv = la.GetResourceVersion()
		}
		items, err := extractItems(list)
		if err != nil {
			return "", err
		}
		loaded += len(items)
		remaining := la.GetRemainingItemCount()
		onPage(Page{Items: items, Loaded: loaded, Remaining: remaining})
		cont = la.GetContinue()
		if cont == "" {
			return rv, nil
		}
	}
}

// extractItems returns the items of a list as individually allocated objects.
// A typed list stores its items in one slice; handing out pointers into it
// would keep the whole page alive for as long as any one item is referenced, so
// each item is shallow-copied into its own allocation.
func extractItems(list runtime.Object) ([]runtime.Object, error) {
	items, err := meta.ExtractList(list)
	if err != nil {
		return nil, err
	}
	for i, it := range items {
		v := reflect.ValueOf(it)
		if v.Kind() != reflect.Ptr || v.IsNil() {
			continue
		}
		cp := reflect.New(v.Elem().Type())
		cp.Elem().Set(v.Elem())
		if obj, ok := cp.Interface().(runtime.Object); ok {
			items[i] = obj
		}
	}
	return items, nil
}

func watchOptions(rv string, timeout int64) metav1.ListOptions {
	if timeout <= 0 {
		timeout = defaultWatchTimeout
	}
	return metav1.ListOptions{
		TimeoutSeconds:      &timeout,
		AllowWatchBookmarks: true,
		ResourceVersion:     rv,
	}
}
