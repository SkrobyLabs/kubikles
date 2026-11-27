package main

import (
	"errors"
	"testing"
	"time"

	"kubikles/pkg/k8s"
	"kubikles/pkg/rows"
)

func podResult(available bool, names ...string) *k8s.PodMetricsResult {
	r := &k8s.PodMetricsResult{Available: available}
	for _, n := range names {
		r.Metrics = append(r.Metrics, k8s.PodMetrics{Namespace: "ns", Name: n, CPUUsage: 100, CPUCommitted: 200, NodeCPUCapacity: 1000, MemoryUsage: 50, MemCommitted: 50, NodeMemCapacity: 100})
	}
	return r
}

func nodeResult(available bool, names ...string) *k8s.NodeMetricsResult {
	r := &k8s.NodeMetricsResult{Available: available}
	for _, n := range names {
		r.Metrics = append(r.Metrics, k8s.NodeMetrics{Name: n, CPUUsage: 500, CPUCapacity: 2000, MemoryUsage: 10, MemCapacity: 40, PodCount: 3, PodCapacity: 110})
	}
	return r
}

func TestOverlayFetchSourceSelection(t *testing.T) {
	o := newOverlaySources()
	defer o.close()
	calls := map[string]int{}
	o.podMetrics = func() (*k8s.PodMetricsResult, error) { calls["k8s"]++; return podResult(false), nil }
	o.podProm = func(string, *k8s.PrometheusInfo) (*k8s.PodMetricsResult, error) {
		calls["prom"]++
		return podResult(true, "web"), nil
	}
	o.detectProm = func(string) *k8s.PrometheusInfo { return &k8s.PrometheusInfo{Available: true} }

	// auto: metrics-server answers unavailable, Prometheus is the fallback.
	res := o.fetchPods(nil, "ctx")
	if !res.Available || len(res.Records) != 1 || calls["k8s"] != 1 || calls["prom"] != 1 {
		t.Fatalf("auto: %+v calls %v", res, calls)
	}
	rec := res.Records["ns/web"]
	if rec[0] != 10 || rec[4] != 20 || rec[1] != 50 {
		t.Fatalf("percentages %v", rec)
	}
	// k8s only: no fallback.
	o.setConfig("k8s", 30000)
	calls = map[string]int{}
	if res := o.fetchPods(nil, "ctx"); res.Available || len(res.Records) != 0 || calls["prom"] != 0 {
		t.Fatalf("k8s only: %+v calls %v", res, calls)
	}
	// prometheus only: metrics-server is not asked.
	o.setConfig("prometheus", 30000)
	calls = map[string]int{}
	if res := o.fetchPods(nil, "ctx"); !res.Available || calls["k8s"] != 0 {
		t.Fatalf("prometheus only: %+v calls %v", res, calls)
	}
	// A throwing metrics-server call keeps the previous samples and reports unavailable.
	o.setConfig("auto", 30000)
	o.podMetrics = func() (*k8s.PodMetricsResult, error) { return nil, errors.New("boom") }
	if res := o.fetchPods(nil, "ctx"); res.Available || !res.Keep {
		t.Fatalf("failure: %+v", res)
	}
}

func TestOverlayNamespaceNeedsPodsAndNodes(t *testing.T) {
	o := newOverlaySources()
	defer o.close()
	o.podMetrics = func() (*k8s.PodMetricsResult, error) { return podResult(true, "a", "b"), nil }
	o.nodeMetrics = func() (*k8s.NodeMetricsResult, error) { return nodeResult(true, "n1", "n2"), nil }
	res := o.fetchNamespaces(nil, "ctx")
	if !res.Available {
		t.Fatalf("both sources answer: %+v", res)
	}
	rec := res.Records["ns"]
	// 2 pods of 100m and 200m committed over 4000m of capacity: 5% and 10%.
	if len(rec) == 0 || rec[0] != 5 || rec[4] != 10 {
		t.Fatalf("namespace record %v", rec)
	}
	o.nodeMetrics = func() (*k8s.NodeMetricsResult, error) { return nodeResult(false), nil }
	if res := o.fetchNamespaces(nil, "ctx"); res.Available || len(res.Records) != 0 {
		t.Fatalf("nodes unavailable: %+v", res)
	}
}

// The poll follows demand and the window: nothing runs without a view or while hidden.
func TestOverlayPollingFollowsDemandAndVisibility(t *testing.T) {
	o := newOverlaySources()
	defer o.close()
	polls := make(chan struct{}, 1000)
	o.podMetrics = func() (*k8s.PodMetricsResult, error) { polls <- struct{}{}; return podResult(true, "a"), nil }
	o.setConfig("k8s", 1000) // 1 s is the shortest interval a setting can express
	count := func() int { return len(polls) }

	time.Sleep(50 * time.Millisecond)
	if count() != 0 {
		t.Fatal("polled without a view")
	}
	sub, _ := o.hub.Subscribe("ctx", rows.OverlayPodMetrics, true, func(rows.OverlayUpdate) {})
	deadline := time.Now().Add(2 * time.Second)
	for count() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if count() == 0 {
		t.Fatal("no poll after a view wanted the source")
	}
	// Hiding the window stops the loop from polling again, however long it waits.
	o.setVisible(false)
	time.Sleep(50 * time.Millisecond)
	n := count()
	time.Sleep(1200 * time.Millisecond)
	if count() != n {
		t.Fatalf("polled %d more times while the window was hidden", count()-n)
	}
	// Showing it polls at once.
	o.setVisible(true)
	deadline = time.Now().Add(2 * time.Second)
	for count() == n && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if count() == n {
		t.Fatal("no catch-up poll after the window came back")
	}
	sub.Cancel()
	time.Sleep(50 * time.Millisecond)
	if o.poller.Running("ctx", rows.OverlayPodMetrics) {
		t.Fatal("the loop must stop with the last view")
	}
}

// A poll of a context that is no longer the current one must not run the current-client calls.
func TestOverlayFetchSkipsOtherContext(t *testing.T) {
	o := newOverlaySources()
	defer o.close()
	called := false
	o.podMetrics = func() (*k8s.PodMetricsResult, error) { called = true; return podResult(true, "web"), nil }
	o.nodeMetrics = func() (*k8s.NodeMetricsResult, error) { called = true; return nodeResult(true, "n"), nil }
	o.isCurrent = func(c string) bool { return c == "B" }
	if res := o.fetchPods(nil, "A"); res.Available || !res.Keep || called {
		t.Fatalf("pods: %+v called=%v", res, called)
	}
	if res := o.fetchNodes(nil, "A"); res.Available || !res.Keep || called {
		t.Fatalf("nodes: %+v called=%v", res, called)
	}
	if res := o.fetchPods(nil, "B"); !res.Available || !called {
		t.Fatalf("current context: %+v called=%v", res, called)
	}
}
