package main

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"kubikles/pkg/debug"
	"kubikles/pkg/k8s"
	"kubikles/pkg/rows"
)

// Metrics overlays for views. The pod, node and namespace metrics columns of a view are fed by
// sources polled here: they reuse the legacy backend calls (GetPodMetrics, GetNodeMetrics, the
// Prometheus fallbacks, which pin the pod and node shards) and apply the preferred-source rules
// of the legacy hooks. A source is polled only while a view wants it (rows.OverlayHub demand)
// and the window is visible (SetWindowVisible).

const defaultMetricsPollInterval = 30 * time.Second

// metricsOverlayConfig is the part of the user's settings the pollers need.
type metricsOverlayConfig struct {
	preferredSource string // auto, k8s, prometheus
	pollInterval    time.Duration
}

// overlaySources owns the hub, the poller and the settings behind them.
type overlaySources struct {
	hub    *rows.OverlayHub
	poller *rows.OverlayPoller

	visible atomic.Bool

	mu   sync.Mutex
	cfg  metricsOverlayConfig
	prom map[string]*k8s.PrometheusInfo // detected Prometheus per context, dropped when it stops answering

	// Test seams: the backend calls the fetchers make.
	podMetrics  func() (*k8s.PodMetricsResult, error)
	nodeMetrics func() (*k8s.NodeMetricsResult, error)
	podProm     func(contextName string, info *k8s.PrometheusInfo) (*k8s.PodMetricsResult, error)
	nodeProm    func(contextName string, info *k8s.PrometheusInfo) (*k8s.NodeMetricsResult, error)
	detectProm  func(contextName string) *k8s.PrometheusInfo
	// isCurrent reports whether contextName is the context the backend calls act on. The metrics
	// calls use the current client, so a poll of another context must not run them.
	isCurrent func(contextName string) bool
}

func newOverlaySources() *overlaySources {
	o := &overlaySources{
		hub:  rows.NewOverlayHub(nil),
		cfg:  metricsOverlayConfig{preferredSource: "auto", pollInterval: defaultMetricsPollInterval},
		prom: map[string]*k8s.PrometheusInfo{},
	}
	o.visible.Store(true)
	o.poller = rows.NewOverlayPoller(rows.PollerConfig{
		Hub:      o.hub,
		Interval: o.interval,
		Visible:  o.visible.Load,
	})
	o.poller.Register(rows.OverlayPodMetrics, o.fetchPods)
	o.poller.Register(rows.OverlayNodeMetrics, o.fetchNodes)
	o.poller.Register(rows.OverlayNamespaceMetrics, o.fetchNamespaces)
	return o
}

func (o *overlaySources) interval() time.Duration {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.cfg.pollInterval
}

func (o *overlaySources) config() metricsOverlayConfig {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.cfg
}

// setConfig applies the user's settings; the loops re-read them at once.
func (o *overlaySources) setConfig(preferredSource string, pollIntervalMs int) {
	switch preferredSource {
	case "k8s", "prometheus":
	default:
		preferredSource = "auto"
	}
	d := time.Duration(pollIntervalMs) * time.Millisecond
	if d < time.Second {
		d = defaultMetricsPollInterval
	}
	o.mu.Lock()
	changed := o.cfg.preferredSource != preferredSource || o.cfg.pollInterval != d
	o.cfg = metricsOverlayConfig{preferredSource: preferredSource, pollInterval: d}
	o.mu.Unlock()
	if changed {
		o.poller.Notify()
	}
}

func (o *overlaySources) setVisible(visible bool) {
	if o.visible.Swap(visible) != visible {
		o.poller.Notify()
	}
}

func (o *overlaySources) close() { o.poller.Close() }

// prometheusFor returns the detected Prometheus of a context (cached), nil when there is none.
func (o *overlaySources) prometheusFor(contextName string) *k8s.PrometheusInfo {
	o.mu.Lock()
	info := o.prom[contextName]
	o.mu.Unlock()
	if info != nil || o.detectProm == nil {
		return info
	}
	if info = o.detectProm(contextName); info != nil && info.Available {
		o.mu.Lock()
		o.prom[contextName] = info
		o.mu.Unlock()
		return info
	}
	return nil
}

// sourceOrder lists the sources to try, in order, for a preference: auto tries metrics-server
// then Prometheus, the others only their own.
func sourceOrder(preferred string) []string {
	switch preferred {
	case "k8s":
		return []string{"k8s"}
	case "prometheus":
		return []string{"prometheus"}
	}
	return []string{"k8s", "prometheus"}
}

// fetchPodInputs is the pod half of usePodMetrics.fetchMetrics. ok=false with failed=true is a
// poll that threw (the previous map is kept); ok=false alone is "no source answered".
func (o *overlaySources) fetchPodInputs(contextName string) (in []rows.PodMetricInput, ok, failed bool) {
	for _, src := range sourceOrder(o.config().preferredSource) {
		var res *k8s.PodMetricsResult
		switch src {
		case "k8s":
			if o.podMetrics == nil {
				continue
			}
			if o.isCurrent != nil && !o.isCurrent(contextName) {
				return nil, false, true
			}
			r, err := o.podMetrics()
			if err != nil {
				debug.LogPerformance("overlay pod metrics failed", map[string]interface{}{"error": err.Error()})
				return nil, false, true
			}
			res = r
		default:
			info := o.prometheusFor(contextName)
			if info == nil || o.podProm == nil {
				continue
			}
			r, err := o.podProm(contextName, info)
			if err != nil {
				continue
			}
			res = r
		}
		if res != nil && res.Available {
			in = make([]rows.PodMetricInput, len(res.Metrics))
			for i, m := range res.Metrics {
				in[i] = rows.PodMetricInput{Namespace: m.Namespace, Name: m.Name, CPUUsage: m.CPUUsage, MemoryUsage: m.MemoryUsage,
					CPURequested: m.CPURequested, MemRequested: m.MemRequested, CPUCommitted: m.CPUCommitted, MemCommitted: m.MemCommitted,
					NodeCPUCapacity: m.NodeCPUCapacity, NodeMemCapacity: m.NodeMemCapacity}
			}
			return in, true, false
		}
	}
	return nil, false, false
}

// fetchNodeInputs is the node half (useNodeMetrics.fetchMetrics).
func (o *overlaySources) fetchNodeInputs(contextName string) (in []rows.NodeMetricInput, ok, failed bool) {
	for _, src := range sourceOrder(o.config().preferredSource) {
		var res *k8s.NodeMetricsResult
		switch src {
		case "k8s":
			if o.nodeMetrics == nil {
				continue
			}
			if o.isCurrent != nil && !o.isCurrent(contextName) {
				return nil, false, true
			}
			r, err := o.nodeMetrics()
			if err != nil {
				debug.LogPerformance("overlay node metrics failed", map[string]interface{}{"error": err.Error()})
				return nil, false, true
			}
			res = r
		default:
			info := o.prometheusFor(contextName)
			if info == nil || o.nodeProm == nil {
				continue
			}
			r, err := o.nodeProm(contextName, info)
			if err != nil {
				continue
			}
			res = r
		}
		if res != nil && res.Available {
			in = make([]rows.NodeMetricInput, len(res.Metrics))
			for i, m := range res.Metrics {
				in[i] = rows.NodeMetricInput{Name: m.Name, CPUUsage: m.CPUUsage, MemoryUsage: m.MemoryUsage, CPUCapacity: m.CPUCapacity,
					MemCapacity: m.MemCapacity, CPURequested: m.CPURequested, MemRequested: m.MemRequested, CPUCommitted: m.CPUCommitted,
					MemCommitted: m.MemCommitted, PodCount: m.PodCount, PodCapacity: m.PodCapacity}
			}
			return in, true, false
		}
	}
	return nil, false, false
}

func (o *overlaySources) fetchPods(_ context.Context, contextName string) rows.OverlayResult {
	in, ok, failed := o.fetchPodInputs(contextName)
	switch {
	case failed:
		return rows.OverlayResult{Available: false, Keep: true}
	case !ok:
		return rows.OverlayResult{Available: false}
	}
	return rows.OverlayResult{Available: true, Records: rows.PodRecords(in)}
}

func (o *overlaySources) fetchNodes(_ context.Context, contextName string) rows.OverlayResult {
	in, ok, failed := o.fetchNodeInputs(contextName)
	switch {
	case failed:
		return rows.OverlayResult{Available: false, Keep: true}
	case !ok:
		return rows.OverlayResult{Available: false}
	}
	return rows.OverlayResult{Available: true, Records: rows.NodeRecords(in)}
}

// fetchNamespaces aggregates the pod samples per namespace against the cluster capacity from the
// node samples; it is available when both are (useNamespaceMetrics).
func (o *overlaySources) fetchNamespaces(_ context.Context, contextName string) rows.OverlayResult {
	pods, podOK, podFailed := o.fetchPodInputs(contextName)
	nodes, nodeOK, nodeFailed := o.fetchNodeInputs(contextName)
	if podFailed || nodeFailed {
		return rows.OverlayResult{Available: false, Keep: true}
	}
	if !podOK || !nodeOK {
		return rows.OverlayResult{Available: false}
	}
	return rows.OverlayResult{Available: true, Records: rows.NamespaceRecords(pods, nodes)}
}

// wireOverlayBackends points the fetchers at the app's backend calls.
func (a *App) wireOverlayBackends(o *overlaySources) {
	o.isCurrent = func(contextName string) bool { return contextName == a.GetCurrentContext() }
	o.podMetrics = func() (*k8s.PodMetricsResult, error) { return a.GetPodMetrics() }
	o.nodeMetrics = func() (*k8s.NodeMetricsResult, error) { return a.GetNodeMetrics() }
	o.podProm = func(contextName string, info *k8s.PrometheusInfo) (*k8s.PodMetricsResult, error) {
		if a.k8sClient == nil {
			return &k8s.PodMetricsResult{Available: false}, nil
		}
		return a.k8sClient.GetPodMetricsFromPrometheus(contextName, info)
	}
	o.nodeProm = func(contextName string, info *k8s.PrometheusInfo) (*k8s.NodeMetricsResult, error) {
		if a.k8sClient == nil {
			return &k8s.NodeMetricsResult{Available: false}, nil
		}
		return a.k8sClient.GetNodeMetricsFromPrometheus(contextName, info)
	}
	o.detectProm = func(contextName string) *k8s.PrometheusInfo {
		if contextName != a.GetCurrentContext() {
			return nil
		}
		info, err := a.DetectPrometheus()
		if err != nil || info == nil || !info.Available {
			return nil
		}
		return info
	}
}

// SetMetricsOverlayConfig tells the backend which metrics source the user prefers ("auto",
// "k8s" or "prometheus") and how often to poll, in milliseconds. The lists call it from their
// settings so view-mode metrics follow the same configuration as the legacy hooks.
func (a *App) SetMetricsOverlayConfig(preferredSource string, pollIntervalMs int) {
	if a.overlays != nil {
		a.overlays.setConfig(preferredSource, pollIntervalMs)
	}
}
