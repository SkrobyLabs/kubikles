package k8s

import (
	"context"
	"errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"reflect"
	"testing"
)

func TestPaginatedListPublishesPagesBeforeCompletion(t *testing.T) {
	var pages []ListPage
	ctx := WithListPageObserver(context.Background(), func(page ListPage) { pages = append(pages, page) })
	calls := 0
	result, err := paginatedList(ctx, "test", 2, func(_ context.Context, options metav1.ListOptions) ([]int, string, *int64, error) {
		calls++
		if calls == 1 {
			remaining := int64(1)
			return []int{1, 2}, "next", &remaining, nil
		}
		if len(pages) != 1 || options.Continue != "next" {
			t.Fatal("first page was not published before fetching the next page")
		}
		return []int{3}, "", nil, nil
	}, func(int, int) {})
	if err != nil || !reflect.DeepEqual(result, []int{1, 2, 3}) {
		t.Fatalf("result %v, error %v", result, err)
	}
	if len(pages) != 2 || pages[0].Loaded != 2 || *pages[0].Total != 3 || *pages[1].Total != 3 {
		t.Fatalf("unexpected pages: %+v", pages)
	}
	if !reflect.DeepEqual(pages[1].Items, []int{3}) {
		t.Fatal("page includes previously delivered items")
	}
}

func TestPaginatedListRetainsDeliveredPageOnFailure(t *testing.T) {
	var pages []ListPage
	ctx := WithListPageObserver(context.Background(), func(page ListPage) { pages = append(pages, page) })
	failure := errors.New("connection interrupted")
	_, err := paginatedList(ctx, "test", 1, func(_ context.Context, options metav1.ListOptions) ([]int, string, *int64, error) {
		if options.Continue == "" {
			return []int{1}, "next", nil, nil
		}
		return nil, "", nil, failure
	}, func(int, int) {})
	if !errors.Is(err, failure) || len(pages) != 1 || pages[0].Total != nil {
		t.Fatalf("error %v, pages %+v", err, pages)
	}
}

func TestListPageObserverIgnoresCancelledRequests(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ctx = WithListPageObserver(ctx, func(ListPage) { t.Fatal("cancelled page emitted") })
	cancel()
	emitListPage(ctx, []int{1}, 1, nil, false)
}

func TestPaginatedListDoesNotDuplicateSinglePagePayload(t *testing.T) {
	ctx := WithListPageObserver(context.Background(), func(ListPage) { t.Fatal("single page sent twice") })
	result, err := paginatedList(ctx, "test", 1000, func(context.Context, metav1.ListOptions) ([]int, string, *int64, error) {
		return []int{1}, "", nil, nil
	}, func(int, int) {})
	if err != nil || len(result) != 1 {
		t.Fatalf("result %v, error %v", result, err)
	}
}
