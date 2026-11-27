package k8s

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"sync"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPaginatedListFastPathUsesSingleUnboundedRequest(t *testing.T) {
	var seen []metav1.ListOptions
	var pages int
	ctx := WithListPageObserver(context.Background(), func(ListPage) { pages++ })
	got, err := paginatedList(ctx, "test", 2, func(_ context.Context, o metav1.ListOptions) ([]int, string, *int64, error) {
		seen = append(seen, o)
		// A continue token is ignored on the fast path: the caller did not
		// ask for progress, so exactly one request is made.
		return []int{1, 2, 3}, "ignored", nil, nil
	}, nil)
	if err != nil || !reflect.DeepEqual(got, []int{1, 2, 3}) {
		t.Fatalf("got %v, err %v", got, err)
	}
	if len(seen) != 1 || seen[0].Limit != 0 || seen[0].Continue != "" {
		t.Fatalf("fast path must send one request without limit: %+v", seen)
	}
	if pages != 0 {
		t.Fatalf("fast path must not emit pages, got %d", pages)
	}
}

func TestPaginatedListFastPathPropagatesError(t *testing.T) {
	boom := errors.New("boom")
	got, err := paginatedList(context.Background(), "test", 2, func(context.Context, metav1.ListOptions) ([]int, string, *int64, error) {
		return nil, "", nil, boom
	}, nil)
	if !errors.Is(err, boom) || got != nil {
		t.Fatalf("got %v, err %v", got, err)
	}
}

func TestPaginatedListPaginatedPathThreadsLimitAndContinue(t *testing.T) {
	var seen []metav1.ListOptions
	pagesData := map[string]struct {
		items []int
		next  string
	}{
		"":   {[]int{1, 2}, "t1"},
		"t1": {[]int{3, 4}, "t2"},
		"t2": {[]int{5}, ""},
	}
	got, err := paginatedList(context.Background(), "test", 2, func(_ context.Context, o metav1.ListOptions) ([]int, string, *int64, error) {
		seen = append(seen, o)
		p := pagesData[o.Continue]
		return p.items, p.next, nil, nil
	}, func(int, int) {})
	if err != nil || !reflect.DeepEqual(got, []int{1, 2, 3, 4, 5}) {
		t.Fatalf("got %v, err %v", got, err)
	}
	if len(seen) != 3 {
		t.Fatalf("expected 3 requests, got %d", len(seen))
	}
	for i, want := range []string{"", "t1", "t2"} {
		if seen[i].Limit != 2 || seen[i].Continue != want {
			t.Fatalf("request %d = %+v, want limit 2 continue %q", i, seen[i], want)
		}
	}
}

func TestPaginatedListProgressAndObserverTotals(t *testing.T) {
	type progress struct{ loaded, total int }
	var prog []progress
	var pages []ListPage
	ctx := WithListPageObserver(context.Background(), func(p ListPage) { pages = append(pages, p) })

	remaining := []*int64{ptr(int64(3)), ptr(int64(1)), nil}
	items := [][]int{{1, 2}, {3, 4}, {5}}
	tokens := []string{"a", "b", ""}
	call := 0
	_, err := paginatedList(ctx, "test", 2, func(context.Context, metav1.ListOptions) ([]int, string, *int64, error) {
		i := call
		call++
		return items[i], tokens[i], remaining[i], nil
	}, func(l, tot int) { prog = append(prog, progress{l, tot}) })
	if err != nil {
		t.Fatal(err)
	}
	if want := []progress{{2, 5}, {4, 5}, {5, 5}}; !reflect.DeepEqual(prog, want) {
		t.Fatalf("progress = %v, want %v", prog, want)
	}
	if len(pages) != 3 {
		t.Fatalf("pages = %d, want 3", len(pages))
	}
	loaded := []int{2, 4, 5}
	totals := []int{5, 5, 5}
	for i, p := range pages {
		if p.Loaded != loaded[i] || p.Total == nil || *p.Total != totals[i] {
			t.Fatalf("page %d = loaded %d total %v, want %d/%d", i, p.Loaded, p.Total, loaded[i], totals[i])
		}
		if !reflect.DeepEqual(p.Items, items[i]) {
			t.Fatalf("page %d items = %v, want only the new page %v", i, p.Items, items[i])
		}
	}
}

func TestPaginatedListUnknownTotalWhileMorePagesExist(t *testing.T) {
	var pages []ListPage
	ctx := WithListPageObserver(context.Background(), func(p ListPage) { pages = append(pages, p) })
	call := 0
	_, err := paginatedList(ctx, "test", 1, func(context.Context, metav1.ListOptions) ([]int, string, *int64, error) {
		call++
		if call < 3 {
			return []int{call}, "more", nil, nil
		}
		return []int{call}, "", nil, nil
	}, func(int, int) {})
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 3 {
		t.Fatalf("pages = %d", len(pages))
	}
	// Without remainingItemCount the total is unknown until the last page,
	// where it equals what was loaded.
	if pages[0].Total != nil || pages[1].Total != nil {
		t.Fatalf("total must be nil while more pages exist: %+v %+v", pages[0], pages[1])
	}
	if pages[2].Total == nil || *pages[2].Total != 3 {
		t.Fatalf("final total = %v, want 3", pages[2].Total)
	}
}

func TestPaginatedListProgressWithoutRemainingUsesLoaded(t *testing.T) {
	var got [][2]int
	call := 0
	_, _ = paginatedList(context.Background(), "test", 1, func(context.Context, metav1.ListOptions) ([]int, string, *int64, error) {
		call++
		if call == 1 {
			return []int{1}, "n", nil, nil
		}
		return []int{2}, "", nil, nil
	}, func(l, tot int) { got = append(got, [2]int{l, tot}) })
	if want := [][2]int{{1, 1}, {2, 2}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("progress = %v, want %v", got, want)
	}
}

func TestPaginatedListEmptyResult(t *testing.T) {
	var pages int
	ctx := WithListPageObserver(context.Background(), func(ListPage) { pages++ })
	progressCalls := 0
	got, err := paginatedList(ctx, "test", 5, func(context.Context, metav1.ListOptions) ([]int, string, *int64, error) {
		return nil, "", nil, nil
	}, func(l, tot int) {
		progressCalls++
		if l != 0 || tot != 0 {
			t.Fatalf("progress %d/%d", l, tot)
		}
	})
	if err != nil || len(got) != 0 || pages != 0 || progressCalls != 1 {
		t.Fatalf("got %v err %v pages %d progress %d", got, err, pages, progressCalls)
	}
}

func TestPaginatedListFirstPageErrorReturnsNothing(t *testing.T) {
	boom := errors.New("boom")
	var pages int
	ctx := WithListPageObserver(context.Background(), func(ListPage) { pages++ })
	got, err := paginatedList(ctx, "test", 5, func(context.Context, metav1.ListOptions) ([]int, string, *int64, error) {
		return nil, "", nil, boom
	}, func(int, int) {})
	if !errors.Is(err, boom) || got != nil || pages != 0 {
		t.Fatalf("got %v err %v pages %d", got, err, pages)
	}
}

func TestPaginatedListCancellationBetweenPagesStopsFetching(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var pages int
	ctx = WithListPageObserver(ctx, func(ListPage) {
		pages++
		cancel() // the consumer goes away after the first page
	})
	fetches := 0
	got, err := paginatedList(ctx, "test", 1, func(context.Context, metav1.ListOptions) ([]int, string, *int64, error) {
		fetches++
		return []int{fetches}, "more", nil, nil
	}, func(int, int) {})
	if !errors.Is(err, context.Canceled) || got != nil {
		t.Fatalf("got %v err %v", got, err)
	}
	if fetches != 1 || pages != 1 {
		t.Fatalf("fetches %d pages %d, want 1 and 1", fetches, pages)
	}
}

func TestPaginatedListCancelledBeforeStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	fetches := 0
	_, err := paginatedList(ctx, "test", 1, func(context.Context, metav1.ListOptions) ([]int, string, *int64, error) {
		fetches++
		return nil, "", nil, nil
	}, func(int, int) {})
	if !errors.Is(err, context.Canceled) || fetches != 0 {
		t.Fatalf("err %v fetches %d", err, fetches)
	}
}

func TestPaginatedListNoObserverStillAccumulates(t *testing.T) {
	call := 0
	got, err := paginatedList(context.Background(), "test", 1, func(context.Context, metav1.ListOptions) ([]int, string, *int64, error) {
		call++
		if call < 3 {
			return []int{call}, "n", nil, nil
		}
		return []int{call}, "", nil, nil
	}, func(int, int) {})
	if err != nil || !reflect.DeepEqual(got, []int{1, 2, 3}) {
		t.Fatalf("got %v err %v", got, err)
	}
}

// pagedPodServer implements limit/continue over n synthetic pods and reports
// remainingItemCount like a real API server does when serving from etcd.
type pagedPodServer struct {
	total           int
	reportRemaining bool
	resourceVersion string // when set, reported as the list's resourceVersion
	mu              sync.Mutex
	queries         []string
}

func (s *pagedPodServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.queries = append(s.queries, r.URL.RawQuery)
	s.mu.Unlock()
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit == 0 {
		limit = s.total
	}
	start := 0
	if c := q.Get("continue"); c != "" {
		start, _ = strconv.Atoi(c)
	}
	end := start + limit
	list := corev1.PodList{TypeMeta: metav1.TypeMeta{Kind: "PodList", APIVersion: "v1"}}
	list.ResourceVersion = s.resourceVersion
	if end >= s.total {
		end = s.total
	} else {
		list.Continue = strconv.Itoa(end)
		if s.reportRemaining {
			rem := int64(s.total - end)
			list.RemainingItemCount = &rem
		}
	}
	for i := start; i < end; i++ {
		list.Items = append(list.Items, corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("pod-%04d", i)}})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(list)
}

func TestListPodsWithContextPagesThroughRealClient(t *testing.T) {
	// defaultPageSize is a constant, so use more pods than one page.
	total := int(defaultPageSize)*2 + 5
	srv := &pagedPodServer{total: total, reportRemaining: true}
	c := newAPIServerClient(t, srv)

	var pages []ListPage
	var progress [][2]int
	ctx := WithListPageObserver(context.Background(), func(p ListPage) { pages = append(pages, p) })
	pods, err := c.ListPodsWithContext(ctx, "ns", func(l, tot int) { progress = append(progress, [2]int{l, tot}) })
	if err != nil {
		t.Fatal(err)
	}
	// Every page was streamed to the observer, so the result is not repeated.
	if len(pods) != 0 {
		t.Fatalf("got %d pods, want none because the pages carried them", len(pods))
	}
	var streamedNames []string
	for _, p := range pages {
		for _, pod := range p.Items.([]corev1.Pod) {
			streamedNames = append(streamedNames, pod.Name)
		}
	}
	if len(streamedNames) != total || streamedNames[0] != "pod-0000" || streamedNames[total-1] != fmt.Sprintf("pod-%04d", total-1) {
		t.Fatalf("pages carried %d pods", len(streamedNames))
	}
	if len(pages) != 3 {
		t.Fatalf("pages = %d, want 3", len(pages))
	}
	sizes := []int{int(defaultPageSize), int(defaultPageSize), 5}
	for i, p := range pages {
		items := p.Items.([]corev1.Pod)
		if len(items) != sizes[i] {
			t.Fatalf("page %d has %d items, want %d", i, len(items), sizes[i])
		}
		if p.Total == nil || *p.Total != total {
			t.Fatalf("page %d total = %v, want %d", i, p.Total, total)
		}
	}
	if want := [2]int{total, total}; progress[len(progress)-1] != want {
		t.Fatalf("final progress = %v, want %v", progress[len(progress)-1], want)
	}
	for _, q := range srv.queries {
		if q == "" {
			t.Fatal("paginated path must send limit")
		}
	}
}

func TestListPodsWithContextWithoutProgressIsSingleUnlimitedRequest(t *testing.T) {
	srv := &pagedPodServer{total: 50, reportRemaining: true}
	c := newAPIServerClient(t, srv)
	var pages int
	ctx := WithListPageObserver(context.Background(), func(ListPage) { pages++ })
	pods, err := c.ListPodsWithContext(ctx, "ns")
	if err != nil || len(pods) != 50 {
		t.Fatalf("pods %d err %v", len(pods), err)
	}
	if len(srv.queries) != 1 || srv.queries[0] != "" {
		t.Fatalf("expected one request with no query, got %q", srv.queries)
	}
	if pages != 0 {
		t.Fatalf("observer must not fire on the fast path, got %d", pages)
	}
}

func TestListPodsWithContextMapsCancellationToErrRequestCancelled(t *testing.T) {
	srv := &pagedPodServer{total: int(defaultPageSize) * 3}
	c := newAPIServerClient(t, srv)
	ctx, cancel := context.WithCancel(context.Background())
	ctx = WithListPageObserver(ctx, func(ListPage) { cancel() })
	pods, err := c.ListPodsWithContext(ctx, "ns", func(int, int) {})
	if !errors.Is(err, ErrRequestCancelled) || pods != nil {
		t.Fatalf("pods %d err %v, want ErrRequestCancelled", len(pods), err)
	}
	srv.mu.Lock()
	n := len(srv.queries)
	srv.mu.Unlock()
	if n != 1 {
		t.Fatalf("requests after cancellation = %d, want 1", n)
	}
}
