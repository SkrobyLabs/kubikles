package k8s

import (
	"context"
	"sync"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsclientset "k8s.io/metrics/pkg/client/clientset/versioned"
)

// GetNodeMetrics and GetPodMetrics both need cluster-wide Pod, Node and
// PodMetrics lists, and every open consumer polls them every 30 s. The lists are
// fetched through short-lived single-flight caches so that concurrent or
// back-to-back callers (node view and pod view polling together, several
// windows, MCP tools) share one LIST each. The TTL is far below the poll
// interval, so a consumer still sees fresh data on every poll. Entries are keyed
// by kube context and are dropped implicitly when the context changes.
const metricsSnapshotTTL = 5 * time.Second

const metricsFetchTimeout = 10 * time.Second

// PodNodeSource supplies the cluster-wide Pod and Node lists that metrics
// aggregation joins against. The default implementation LISTs from the API
// server; a shared informer store can replace it (SetPodNodeSource) without
// touching the aggregation code. Returned lists are shared and must be treated
// as read-only.
type PodNodeSource interface {
	ListPods(ctx context.Context) (*v1.PodList, error)
	ListNodes(ctx context.Context) (*v1.NodeList, error)
}

type apiPodNodeSource struct{ c *Client }

func (s apiPodNodeSource) ListPods(ctx context.Context) (*v1.PodList, error) {
	cs, err := s.c.getClientset()
	if err != nil {
		return nil, err
	}
	return cs.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
}

func (s apiPodNodeSource) ListNodes(ctx context.Context) (*v1.NodeList, error) {
	cs, err := s.c.getClientset()
	if err != nil {
		return nil, err
	}
	return cs.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
}

// metricsSnapshots holds the shared caches. The zero value is ready to use.
type metricsSnapshots struct {
	pods        flightCache[*v1.PodList]
	nodes       flightCache[*v1.NodeList]
	podMetrics  flightCache[*metricsv1beta1.PodMetricsList]
	nodeMetrics flightCache[*metricsv1beta1.NodeMetricsList]

	mu     sync.RWMutex
	source PodNodeSource
	ttl    time.Duration // zero means metricsSnapshotTTL
}

// SetPodNodeSource replaces the Pod/Node lister used by the metrics calls. A nil
// source restores the default API-server LIST.
func (c *Client) SetPodNodeSource(src PodNodeSource) {
	c.metricsSnap.mu.Lock()
	c.metricsSnap.source = src
	c.metricsSnap.mu.Unlock()
}

func (c *Client) podNodeSource() PodNodeSource {
	c.metricsSnap.mu.RLock()
	src := c.metricsSnap.source
	c.metricsSnap.mu.RUnlock()
	if src != nil {
		return src
	}
	return apiPodNodeSource{c}
}

func (c *Client) snapshotTTL() time.Duration {
	c.metricsSnap.mu.RLock()
	defer c.metricsSnap.mu.RUnlock()
	if c.metricsSnap.ttl > 0 {
		return c.metricsSnap.ttl
	}
	return metricsSnapshotTTL
}

func (c *Client) snapshotPods() (*v1.PodList, error) {
	src := c.podNodeSource()
	return c.metricsSnap.pods.get(c.GetCurrentContext(), c.snapshotTTL(), func() (*v1.PodList, error) {
		ctx, cancel := context.WithTimeout(context.Background(), metricsFetchTimeout)
		defer cancel()
		return src.ListPods(ctx)
	})
}

func (c *Client) snapshotNodes() (*v1.NodeList, error) {
	src := c.podNodeSource()
	return c.metricsSnap.nodes.get(c.GetCurrentContext(), c.snapshotTTL(), func() (*v1.NodeList, error) {
		ctx, cancel := context.WithTimeout(context.Background(), metricsFetchTimeout)
		defer cancel()
		return src.ListNodes(ctx)
	})
}

func (c *Client) snapshotPodMetrics(mc metricsclientset.Interface) (*metricsv1beta1.PodMetricsList, error) {
	return c.metricsSnap.podMetrics.get(c.GetCurrentContext(), c.snapshotTTL(), func() (*metricsv1beta1.PodMetricsList, error) {
		ctx, cancel := context.WithTimeout(context.Background(), metricsFetchTimeout)
		defer cancel()
		return mc.MetricsV1beta1().PodMetricses("").List(ctx, metav1.ListOptions{})
	})
}

func (c *Client) snapshotNodeMetrics(mc metricsclientset.Interface) (*metricsv1beta1.NodeMetricsList, error) {
	return c.metricsSnap.nodeMetrics.get(c.GetCurrentContext(), c.snapshotTTL(), func() (*metricsv1beta1.NodeMetricsList, error) {
		ctx, cancel := context.WithTimeout(context.Background(), metricsFetchTimeout)
		defer cancel()
		return mc.MetricsV1beta1().NodeMetricses().List(ctx, metav1.ListOptions{})
	})
}

// ensureMetricsClient lazily builds the metrics-server client for the current context.
func (c *Client) ensureMetricsClient() (metricsclientset.Interface, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.metricsClient == nil {
		config, err := c.configLoading.ClientConfig()
		if err != nil {
			return nil, err
		}
		mc, err := newAuthenticatedClient(config, metricsclientset.NewForConfigAndClient)
		if err != nil {
			return nil, err
		}
		c.metricsClient = mc
	}
	return c.metricsClient, nil
}

// flightCache is a single-slot TTL cache with request coalescing: callers that
// arrive while a fetch is running wait for it instead of starting another, and
// callers within ttl of the last success reuse its value. Errors are never
// cached. A different key (a context switch) bypasses and then replaces the slot.
type flightCache[T any] struct {
	mu      sync.Mutex
	key     string
	val     T
	has     bool
	at      time.Time
	call    *flightCall[T]
	callKey string
}

type flightCall[T any] struct {
	done chan struct{}
	val  T
	err  error
}

func (f *flightCache[T]) get(key string, ttl time.Duration, fetch func() (T, error)) (T, error) {
	f.mu.Lock()
	if f.has && f.key == key && time.Since(f.at) < ttl {
		v := f.val
		f.mu.Unlock()
		return v, nil
	}
	if f.call != nil && f.callKey == key {
		call := f.call
		f.mu.Unlock()
		<-call.done
		return call.val, call.err
	}
	call := &flightCall[T]{done: make(chan struct{})}
	f.call, f.callKey = call, key
	f.mu.Unlock()

	// A panic in fetch must still release waiters.
	completed := false
	defer func() {
		f.mu.Lock()
		if !completed && call.err == nil {
			call.err = context.Canceled
		}
		if f.call == call {
			// Only the current flight may publish; a flight overtaken by a
			// context switch must not overwrite the newer key's slot.
			if call.err == nil {
				f.key, f.val, f.has, f.at = key, call.val, true, time.Now()
			}
			f.call = nil
		}
		f.mu.Unlock()
		close(call.done)
	}()
	call.val, call.err = fetch()
	completed = true
	return call.val, call.err
}

func (f *flightCache[T]) invalidate() {
	f.mu.Lock()
	f.has = false
	f.mu.Unlock()
}
