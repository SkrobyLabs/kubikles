package k8s

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsfake "k8s.io/metrics/pkg/client/clientset/versioned/fake"
)

func TestFlightCacheCoalescesConcurrentCallers(t *testing.T) {
	var f flightCache[int]
	var calls atomic.Int32
	release := make(chan struct{})
	fetch := func() (int, error) {
		calls.Add(1)
		<-release
		return 42, nil
	}
	var wg sync.WaitGroup
	results := make([]int, 20)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], _ = f.get("ctx", time.Minute, fetch)
		}(i)
	}
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("fetch ran %d times, want 1", calls.Load())
	}
	for _, r := range results {
		if r != 42 {
			t.Fatalf("result %d", r)
		}
	}
}

func TestFlightCacheTTLKeyAndErrors(t *testing.T) {
	var f flightCache[int]
	n := 0
	fetch := func() (int, error) { n++; return n, nil }

	if v, _ := f.get("a", time.Hour, fetch); v != 1 {
		t.Fatal(v)
	}
	if v, _ := f.get("a", time.Hour, fetch); v != 1 {
		t.Fatalf("cached value expected, got %d", v)
	}
	if v, _ := f.get("b", time.Hour, fetch); v != 2 {
		t.Fatalf("different key must refetch, got %d", v)
	}
	if v, _ := f.get("a", time.Hour, fetch); v != 3 {
		t.Fatalf("switching back must not serve the old context, got %d", v)
	}
	time.Sleep(5 * time.Millisecond)
	if v, _ := f.get("a", time.Millisecond, fetch); v != 4 {
		t.Fatalf("expired entry must refetch, got %d", v)
	}

	boom := errors.New("boom")
	if _, err := f.get("c", time.Hour, func() (int, error) { return 0, boom }); !errors.Is(err, boom) {
		t.Fatal(err)
	}
	if v, err := f.get("c", time.Hour, fetch); err != nil || v != 5 {
		t.Fatalf("errors must not be cached: %d %v", v, err)
	}
}

func TestFlightCachePanicReleasesWaiters(t *testing.T) {
	var f flightCache[int]
	func() {
		defer func() { _ = recover() }()
		_, _ = f.get("k", time.Hour, func() (int, error) { panic("x") })
	}()
	done := make(chan struct{})
	go func() {
		defer close(done)
		if v, err := f.get("k", time.Hour, func() (int, error) { return 7, nil }); err != nil || v != 7 {
			t.Errorf("got %d %v", v, err)
		}
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("cache stuck after a panicking fetch")
	}
}

type countingSource struct {
	pods, nodes atomic.Int32
	podList     *v1.PodList
	nodeList    *v1.NodeList
}

func (s *countingSource) ListPods(ctx context.Context) (*v1.PodList, error) {
	s.pods.Add(1)
	return s.podList, nil
}
func (s *countingSource) ListNodes(ctx context.Context) (*v1.NodeList, error) {
	s.nodes.Add(1)
	return s.nodeList, nil
}

func metricsFixture() (*countingSource, *metricsfake.Clientset) {
	node := v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "n1"},
		Status: v1.NodeStatus{
			Capacity:    v1.ResourceList{v1.ResourceCPU: resource.MustParse("4"), v1.ResourceMemory: resource.MustParse("8Gi")},
			Allocatable: v1.ResourceList{v1.ResourceCPU: resource.MustParse("4"), v1.ResourceMemory: resource.MustParse("8Gi"), v1.ResourcePods: resource.MustParse("110")},
		},
	}
	pod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "p1"},
		Spec: v1.PodSpec{NodeName: "n1", Containers: []v1.Container{{
			Name: "c",
			Resources: v1.ResourceRequirements{Requests: v1.ResourceList{
				v1.ResourceCPU: resource.MustParse("100m"), v1.ResourceMemory: resource.MustParse("64Mi")}},
		}}},
		Status: v1.PodStatus{Phase: v1.PodRunning},
	}
	src := &countingSource{podList: &v1.PodList{Items: []v1.Pod{pod}}, nodeList: &v1.NodeList{Items: []v1.Node{node}}}

	pm := &metricsv1beta1.PodMetrics{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "p1"},
		Containers: []metricsv1beta1.ContainerMetrics{{Name: "c", Usage: v1.ResourceList{
			v1.ResourceCPU: resource.MustParse("250m"), v1.ResourceMemory: resource.MustParse("128Mi")}}},
	}
	nm := &metricsv1beta1.NodeMetrics{
		ObjectMeta: metav1.ObjectMeta{Name: "n1"},
		Usage:      v1.ResourceList{v1.ResourceCPU: resource.MustParse("1"), v1.ResourceMemory: resource.MustParse("2Gi")},
	}
	mc := metricsfake.NewSimpleClientset()
	// The fake tracker cannot map the metrics kinds to resources, so answer the
	// LISTs directly.
	mc.PrependReactor("list", "pods", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, &metricsv1beta1.PodMetricsList{Items: []metricsv1beta1.PodMetrics{*pm}}, nil
	})
	mc.PrependReactor("list", "nodes", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, &metricsv1beta1.NodeMetricsList{Items: []metricsv1beta1.NodeMetrics{*nm}}, nil
	})
	return src, mc
}

func TestMetricsShareListsBetweenConsumers(t *testing.T) {
	src, mc := metricsFixture()
	var metricListCalls atomic.Int32
	mc.PrependReactor("list", "*", func(k8stesting.Action) (bool, runtime.Object, error) {
		metricListCalls.Add(1)
		return false, nil, nil
	})
	c := &Client{clientset: fake.NewSimpleClientset(), metricsClient: mc, currentContext: "ctx-a"}
	c.SetPodNodeSource(src)

	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			res, err := c.GetPodMetrics()
			if err != nil || !res.Available || len(res.Metrics) != 1 {
				t.Errorf("pod metrics: %+v %v", res, err)
				return
			}
			pm := res.Metrics[0]
			if pm.CPUUsage != 250 || pm.CPURequested != 100 || pm.CPUCommitted != 250 || pm.NodeCPUCapacity != 4000 {
				t.Errorf("unexpected pod metrics %+v", pm)
			}
		}()
		go func() {
			defer wg.Done()
			res, err := c.GetNodeMetrics()
			if err != nil || !res.Available || len(res.Metrics) != 1 {
				t.Errorf("node metrics: %+v %v", res, err)
				return
			}
			nm := res.Metrics[0]
			if nm.CPUUsage != 1000 || nm.PodCount != 1 || nm.CPURequested != 100 || nm.PodCapacity != 110 {
				t.Errorf("unexpected node metrics %+v", nm)
			}
		}()
	}
	wg.Wait()

	if src.pods.Load() != 1 || src.nodes.Load() != 1 {
		t.Fatalf("Pods listed %d times, Nodes %d times; want 1 each", src.pods.Load(), src.nodes.Load())
	}
	if got := metricListCalls.Load(); got != 2 { // one PodMetrics LIST, one NodeMetrics LIST
		t.Fatalf("metrics-server listed %d times, want 2", got)
	}
}

func TestMetricsRefetchAfterContextSwitchAndTTL(t *testing.T) {
	src, mc := metricsFixture()
	c := &Client{clientset: fake.NewSimpleClientset(), metricsClient: mc, currentContext: "ctx-a"}
	c.SetPodNodeSource(src)

	if _, err := c.GetPodMetrics(); err != nil {
		t.Fatal(err)
	}
	if _, err := c.GetPodMetrics(); err != nil {
		t.Fatal(err)
	}
	if src.pods.Load() != 1 {
		t.Fatalf("second call should hit the cache, pods listed %d times", src.pods.Load())
	}

	c.mu.Lock()
	c.currentContext = "ctx-b"
	c.mu.Unlock()
	if _, err := c.GetPodMetrics(); err != nil {
		t.Fatal(err)
	}
	if src.pods.Load() != 2 {
		t.Fatalf("context switch must refetch, pods listed %d times", src.pods.Load())
	}

	c.metricsSnap.mu.Lock()
	c.metricsSnap.ttl = time.Nanosecond
	c.metricsSnap.mu.Unlock()
	time.Sleep(time.Millisecond)
	if _, err := c.GetPodMetrics(); err != nil {
		t.Fatal(err)
	}
	if src.pods.Load() != 3 {
		t.Fatalf("expired snapshot must refetch, pods listed %d times", src.pods.Load())
	}
}

func TestMetricsEmptyMetricsServerReportsUnavailable(t *testing.T) {
	src, _ := metricsFixture()
	c := &Client{clientset: fake.NewSimpleClientset(), metricsClient: metricsfake.NewSimpleClientset(), currentContext: "ctx"}
	c.SetPodNodeSource(src)
	res, err := c.GetPodMetrics()
	if err != nil || res.Available || res.Error != "metrics-server returned no data" {
		t.Fatalf("%+v %v", res, err)
	}
	nres, err := c.GetNodeMetrics()
	if err != nil || nres.Available {
		t.Fatalf("%+v %v", nres, err)
	}
}

func TestDefaultPodNodeSourceUsesClientset(t *testing.T) {
	cs := fake.NewSimpleClientset(&v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}, &v1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "p"}})
	c := &Client{clientset: cs, currentContext: "ctx"}
	nodes, err := c.snapshotNodes()
	if err != nil || len(nodes.Items) != 1 {
		t.Fatalf("%v %v", nodes, err)
	}
	pods, err := c.snapshotPods()
	if err != nil || len(pods.Items) != 1 {
		t.Fatalf("%v %v", pods, err)
	}
}
