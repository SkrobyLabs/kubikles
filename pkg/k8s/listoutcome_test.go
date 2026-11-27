package k8s

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	clienttesting "k8s.io/client-go/testing"
)

func collectOutcome(ctx context.Context) (context.Context, *[]ListOutcome) {
	var got []ListOutcome
	return WithListOutcomeSink(ctx, func(o ListOutcome) { got = append(got, o) }), &got
}

func TestPaginatedListReturnsEmptyOnlyWhenPagesWereStreamed(t *testing.T) {
	pagedFetch := func(calls *int) func(context.Context, metav1.ListOptions) ([]int, string, *int64, error) {
		return func(ctx context.Context, opts metav1.ListOptions) ([]int, string, *int64, error) {
			*calls++
			noteListResourceVersion(ctx, "rv-"+string(rune('0'+*calls)))
			if *calls == 1 {
				return []int{1, 2}, "next", nil, nil
			}
			return []int{3}, "", nil, nil
		}
	}
	progress := func(int, int) {}

	t.Run("observer present streams every page and returns no items", func(t *testing.T) {
		var pages []ListPage
		ctx := WithListPageObserver(context.Background(), func(p ListPage) { pages = append(pages, p) })
		ctx, outcomes := collectOutcome(ctx)
		calls := 0
		got, err := paginatedList(ctx, "test", 2, pagedFetch(&calls), progress)
		if err != nil || len(got) != 0 {
			t.Fatalf("got %v, err %v", got, err)
		}
		want := []ListOutcome{{ResourceVersion: "rv-1", Streamed: true, Loaded: 3}}
		if !reflect.DeepEqual(*outcomes, want) {
			t.Fatalf("outcomes = %+v, want %+v", *outcomes, want)
		}
	})

	t.Run("without an observer the full list is returned", func(t *testing.T) {
		ctx, outcomes := collectOutcome(context.Background())
		calls := 0
		got, err := paginatedList(ctx, "test", 2, pagedFetch(&calls), progress)
		if err != nil || !reflect.DeepEqual(got, []int{1, 2, 3}) {
			t.Fatalf("got %v, err %v", got, err)
		}
		want := []ListOutcome{{ResourceVersion: "rv-1", Streamed: false, Loaded: 3}}
		if !reflect.DeepEqual(*outcomes, want) {
			t.Fatalf("outcomes = %+v, want %+v", *outcomes, want)
		}
	})

	t.Run("single page is returned in full and not reported as streamed", func(t *testing.T) {
		ctx := WithListPageObserver(context.Background(), func(ListPage) { t.Fatal("single page emitted") })
		ctx, outcomes := collectOutcome(ctx)
		got, err := paginatedList(ctx, "test", 5, func(ctx context.Context, _ metav1.ListOptions) ([]int, string, *int64, error) {
			noteListResourceVersion(ctx, "42")
			return []int{7, 8}, "", nil, nil
		}, progress)
		if err != nil || !reflect.DeepEqual(got, []int{7, 8}) {
			t.Fatalf("got %v, err %v", got, err)
		}
		want := []ListOutcome{{ResourceVersion: "42", Streamed: false, Loaded: 2}}
		if !reflect.DeepEqual(*outcomes, want) {
			t.Fatalf("outcomes = %+v, want %+v", *outcomes, want)
		}
	})

	t.Run("fast path reports its version", func(t *testing.T) {
		ctx, outcomes := collectOutcome(context.Background())
		got, err := paginatedList(ctx, "test", 5, func(ctx context.Context, _ metav1.ListOptions) ([]int, string, *int64, error) {
			noteListResourceVersion(ctx, "9")
			return []int{1}, "", nil, nil
		}, nil)
		if err != nil || len(got) != 1 {
			t.Fatalf("got %v, err %v", got, err)
		}
		if !reflect.DeepEqual(*outcomes, []ListOutcome{{ResourceVersion: "9", Loaded: 1}}) {
			t.Fatalf("outcomes = %+v", *outcomes)
		}
	})

	t.Run("a page the observer missed forces the full result", func(t *testing.T) {
		// Cancelling after the first page makes later emissions no-ops; the
		// observer then does not hold the whole list.
		ctx, cancel := context.WithCancel(context.Background())
		ctx = WithListPageObserver(ctx, func(ListPage) {})
		ctx, outcomes := collectOutcome(ctx)
		calls := 0
		got, err := paginatedList(ctx, "test", 2, func(_ context.Context, _ metav1.ListOptions) ([]int, string, *int64, error) {
			calls++
			if calls == 1 {
				return []int{1, 2}, "next", nil, nil
			}
			cancel()
			return []int{3}, "", nil, nil
		}, progress)
		if err != nil || !reflect.DeepEqual(got, []int{1, 2, 3}) {
			t.Fatalf("got %v, err %v", got, err)
		}
		if len(*outcomes) != 1 || (*outcomes)[0].Streamed {
			t.Fatalf("outcomes = %+v, want not streamed", *outcomes)
		}
	})

	t.Run("failures report no outcome", func(t *testing.T) {
		ctx, outcomes := collectOutcome(context.Background())
		_, err := paginatedList(ctx, "test", 2, func(context.Context, metav1.ListOptions) ([]int, string, *int64, error) {
			return nil, "", nil, errors.New("boom")
		}, progress)
		if err == nil || len(*outcomes) != 0 {
			t.Fatalf("err %v outcomes %+v", err, *outcomes)
		}
	})

	t.Run("only the first non-empty version is kept", func(t *testing.T) {
		ctx, outcomes := collectOutcome(context.Background())
		calls := 0
		_, _ = paginatedList(ctx, "test", 1, func(ctx context.Context, _ metav1.ListOptions) ([]int, string, *int64, error) {
			calls++
			noteListResourceVersion(ctx, map[int]string{1: "", 2: "second", 3: "third"}[calls])
			if calls < 3 {
				return []int{calls}, "next", nil, nil
			}
			return []int{calls}, "", nil, nil
		}, progress)
		if (*outcomes)[0].ResourceVersion != "second" {
			t.Fatalf("outcomes = %+v", *outcomes)
		}
	})
}

func TestListOutcomeHelpersAreNoOpsWithoutASink(t *testing.T) {
	noteListResourceVersion(context.Background(), "1")
	reportListOutcome(context.Background(), 3, true)
}

func TestListPodsWithContextReportsListResourceVersionThroughRealClient(t *testing.T) {
	srv := &pagedPodServer{total: int(defaultPageSize) + 3, reportRemaining: true, resourceVersion: "5150"}
	c := newAPIServerClient(t, srv)
	ctx := WithListPageObserver(context.Background(), func(ListPage) {})
	ctx, outcomes := collectOutcome(ctx)
	pods, err := c.ListPodsWithContext(ctx, "ns", func(int, int) {})
	if err != nil || len(pods) != 0 {
		t.Fatalf("pods %d, err %v", len(pods), err)
	}
	want := []ListOutcome{{ResourceVersion: "5150", Streamed: true, Loaded: int(defaultPageSize) + 3}}
	if !reflect.DeepEqual(*outcomes, want) {
		t.Fatalf("outcomes = %+v, want %+v", *outcomes, want)
	}
}

// Secrets list through a Table request and bypass paginatedList, so they
// report their outcome themselves.
func TestListSecretsMetadataStreamsPagesAndReportsOutcome(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		meta := map[string]interface{}{"resourceVersion": "77"}
		rows := []map[string]interface{}{}
		row := func(name string) map[string]interface{} {
			return map[string]interface{}{"cells": []interface{}{name, "Opaque", 1},
				"object": map[string]interface{}{"metadata": map[string]interface{}{"name": name, "namespace": "ns", "uid": "uid-" + name}}}
		}
		if r.URL.Query().Get("continue") == "" {
			meta["continue"] = "more"
			rows = append(rows, row("a"), row("b"))
		} else {
			rows = append(rows, row("c"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"kind": "Table", "apiVersion": "meta.k8s.io/v1", "metadata": meta,
			"columnDefinitions": []map[string]interface{}{{"name": "Name"}, {"name": "Type"}, {"name": "Data"}},
			"rows":              rows,
		})
	})
	c := newAPIServerClient(t, handler)

	var pages []ListPage
	ctx := WithListPageObserver(context.Background(), func(p ListPage) { pages = append(pages, p) })
	ctx, outcomes := collectOutcome(ctx)
	items, err := c.ListSecretsMetadataWithContext(ctx, "ns", func(int, int) {})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 || len(pages) != 2 {
		t.Fatalf("items %d pages %d, want 0 and 2", len(items), len(pages))
	}
	want := []ListOutcome{{ResourceVersion: "77", Streamed: true, Loaded: 3}}
	if !reflect.DeepEqual(*outcomes, want) {
		t.Fatalf("outcomes = %+v, want %+v", *outcomes, want)
	}

	// Without an observer the same list is returned in full.
	ctx, outcomes = collectOutcome(context.Background())
	items, err = c.ListSecretsMetadataWithContext(ctx, "ns", func(int, int) {})
	if err != nil || len(items) != 3 || (*outcomes)[0].Streamed {
		t.Fatalf("items %d err %v outcomes %+v", len(items), err, *outcomes)
	}
}

// Custom resource lists must report the list resourceVersion and outcome like
// built-in lists, so the frontend can anchor the CRD watch to it.
func TestListCustomResourcesReportsResourceVersionAndOutcome(t *testing.T) {
	gvr := schema.GroupVersionResource{Group: "example.io", Version: "v1", Resource: "widgets"}
	newClient := func() *dynamicfake.FakeDynamicClient {
		dc := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(),
			map[schema.GroupVersionResource]string{gvr: "WidgetList"})
		dc.PrependReactor("list", "widgets", func(clienttesting.Action) (bool, runtime.Object, error) {
			list := &unstructured.UnstructuredList{}
			list.SetGroupVersionKind(schema.GroupVersionKind{Group: "example.io", Version: "v1", Kind: "WidgetList"})
			list.SetResourceVersion("77")
			for _, name := range []string{"a", "b"} {
				u := unstructured.Unstructured{Object: map[string]interface{}{
					"apiVersion": "example.io/v1", "kind": "Widget",
					"metadata": map[string]interface{}{"name": name, "namespace": "ns"},
				}}
				list.Items = append(list.Items, u)
			}
			return true, list, nil
		})
		return dc
	}

	t.Run("fast path", func(t *testing.T) {
		ctx, outcomes := collectOutcome(context.Background())
		got, err := listCustomResourcesFrom(ctx, newClient(), gvr, "ns", nil)
		if err != nil || len(got) != 2 {
			t.Fatalf("got %d items, err %v", len(got), err)
		}
		want := []ListOutcome{{ResourceVersion: "77", Streamed: false, Loaded: 2}}
		if !reflect.DeepEqual(*outcomes, want) {
			t.Fatalf("outcomes = %+v, want %+v", *outcomes, want)
		}
	})

	t.Run("paginated path", func(t *testing.T) {
		ctx, outcomes := collectOutcome(context.Background())
		got, err := listCustomResourcesFrom(ctx, newClient(), gvr, "", func(int, int) {})
		if err != nil || len(got) != 2 {
			t.Fatalf("got %d items, err %v", len(got), err)
		}
		want := []ListOutcome{{ResourceVersion: "77", Streamed: false, Loaded: 2}}
		if !reflect.DeepEqual(*outcomes, want) {
			t.Fatalf("outcomes = %+v, want %+v", *outcomes, want)
		}
	})
}
