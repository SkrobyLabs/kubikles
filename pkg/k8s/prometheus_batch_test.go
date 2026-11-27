package k8s

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakePrometheus answers /services/<svc>/proxy/api/v1/query_range. The response
// value encodes which query was asked (a hash of its text), so tests can check
// that every series landed in the right field, and each request waits delay so
// concurrency is observable.
type fakePrometheus struct {
	delay    time.Duration
	failIf   func(query string) bool
	inFlight atomic.Int32
	peak     atomic.Int32
	total    atomic.Int32
	mu       sync.Mutex
	queries  []string
}

func (f *fakePrometheus) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !strings.HasSuffix(r.URL.Path, "/api/v1/query_range") {
		http.NotFound(w, r)
		return
	}
	body, _ := io.ReadAll(r.Body)
	form, _ := url.ParseQuery(string(body))
	query := form.Get("query")

	n := f.inFlight.Add(1)
	for {
		p := f.peak.Load()
		if n <= p || f.peak.CompareAndSwap(p, n) {
			break
		}
	}
	f.total.Add(1)
	f.mu.Lock()
	f.queries = append(f.queries, query)
	f.mu.Unlock()
	time.Sleep(f.delay)
	f.inFlight.Add(-1)

	if f.failIf != nil && f.failIf(query) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"status":"error","errorType":"bad_data","error":"nope"}`))
		return
	}
	container := ""
	if strings.Contains(query, "by (container)") {
		container = "app"
	}
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"status":"success","data":{"resultType":"matrix","result":[{"metric":{"container":%q},"values":[[1700000000,"%d"]]}]}}`, container, queryTag(query))
}

// queryTag gives each distinct query text a stable small number.
func queryTag(q string) int {
	h := 0
	for _, r := range q {
		h = (h*31 + int(r)) % 100003
	}
	return h
}

func testPrometheusInfo() *PrometheusInfo {
	return &PrometheusInfo{Available: true, Namespace: "monitoring", Service: "prometheus", Port: 9090}
}

func TestPodHistoryQueriesRunConcurrentlyAndKeepOrder(t *testing.T) {
	prom := &fakePrometheus{delay: 40 * time.Millisecond}
	c := newAPIServerClient(t, prom)
	end := time.Now()
	start := end.Add(-time.Hour)

	began := time.Now()
	res, err := c.GetPodMetricsHistoryWithContext(t.Context(), "", testPrometheusInfo(), "ns", "pod", "", start, end, 100)
	elapsed := time.Since(began)
	if err != nil {
		t.Fatal(err)
	}
	if prom.total.Load() != 8 {
		t.Fatalf("expected 8 queries (cpu, memory, 6 network), got %d", prom.total.Load())
	}
	// Sequential execution would take 8 x 40ms = 320ms.
	if elapsed > 250*time.Millisecond || prom.peak.Load() < 4 {
		t.Fatalf("queries did not overlap: elapsed %v, peak concurrency %d", elapsed, prom.peak.Load())
	}
	if len(res.Containers) != 1 || res.Containers[0].Container != "app" ||
		len(res.Containers[0].CPU) != 1 || len(res.Containers[0].Memory) != 1 {
		t.Fatalf("containers: %+v", res.Containers)
	}
	// CPU and memory must carry different series.
	if res.Containers[0].CPU[0].Value == res.Containers[0].Memory[0].Value {
		t.Fatal("cpu and memory series were swapped or merged")
	}
	net := res.Network
	fields := map[string][]MetricsDataPoint{
		"rx bytes": net.ReceiveBytes, "tx bytes": net.TransmitBytes, "rx packets": net.ReceivePackets,
		"tx packets": net.TransmitPackets, "rx dropped": net.ReceiveDropped, "tx dropped": net.TransmitDropped,
	}
	seen := map[float64]string{}
	for name, pts := range fields {
		if len(pts) != 1 {
			t.Fatalf("%s: %v", name, pts)
		}
		if other, dup := seen[pts[0].Value]; dup {
			t.Fatalf("%s and %s hold the same series", name, other)
		}
		seen[pts[0].Value] = name
	}
	if want := float64(queryTag(`sum(rate(container_network_receive_bytes_total{namespace="ns", pod="pod"}[5m]))`)); net.ReceiveBytes[0].Value != want {
		t.Fatalf("rx bytes value %v want %v", net.ReceiveBytes[0].Value, want)
	}
}

func TestPodHistoryRequiredQueryErrorsKeepSemantics(t *testing.T) {
	end := time.Now()
	start := end.Add(-time.Hour)

	t.Run("cpu failure reported first", func(t *testing.T) {
		prom := &fakePrometheus{delay: 5 * time.Millisecond, failIf: func(q string) bool {
			return strings.Contains(q, "container_cpu_usage") || strings.Contains(q, "container_memory_working_set")
		}}
		c := newAPIServerClient(t, prom)
		_, err := c.GetPodMetricsHistoryWithContext(t.Context(), "", testPrometheusInfo(), "ns", "pod", "", start, end, 100)
		if err == nil || !strings.Contains(err.Error(), "failed to query CPU metrics") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("memory failure", func(t *testing.T) {
		prom := &fakePrometheus{delay: 5 * time.Millisecond, failIf: func(q string) bool { return strings.Contains(q, "container_memory_working_set") }}
		c := newAPIServerClient(t, prom)
		_, err := c.GetPodMetricsHistoryWithContext(t.Context(), "", testPrometheusInfo(), "ns", "pod", "", start, end, 100)
		if err == nil || !strings.Contains(err.Error(), "failed to query memory metrics") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("network failures stay silent", func(t *testing.T) {
		prom := &fakePrometheus{failIf: func(q string) bool { return strings.Contains(q, "container_network") }}
		c := newAPIServerClient(t, prom)
		res, err := c.GetPodMetricsHistoryWithContext(t.Context(), "", testPrometheusInfo(), "ns", "pod", "", start, end, 100)
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Network.ReceiveBytes) != 0 || len(res.Containers) != 1 {
			t.Fatalf("%+v", res)
		}
	})
}

func TestNodeAndControllerHistoryRunBounded(t *testing.T) {
	prom := &fakePrometheus{delay: 20 * time.Millisecond}
	c := newAPIServerClient(t, prom)
	end := time.Now()
	start := end.Add(-time.Hour)

	node, err := c.GetNodeMetricsHistoryWithContext(t.Context(), "", testPrometheusInfo(), "node-1", start, end, 100)
	if err != nil {
		t.Fatal(err)
	}
	if node.CPU.Usage == nil || len(node.CPU.Usage) != 1 || len(node.Network.TransmitDropped) != 1 || len(node.Memory.Committed) != 1 {
		t.Fatalf("node history incomplete: %+v", node.CPU)
	}
	if peak := prom.peak.Load(); peak > prometheusQueryConcurrency || peak < 2 {
		t.Fatalf("peak concurrency %d outside [2,%d]", peak, prometheusQueryConcurrency)
	}

	ctrl, err := c.GetControllerMetricsHistoryWithContext(t.Context(), "", testPrometheusInfo(), "ns", "web", "deployment", start, end, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(ctrl.CPU.Usage) != 1 || len(ctrl.Pods.Desired) != 1 || len(ctrl.Restarts) != 1 || len(ctrl.Network.ReceivePackets) != 1 {
		t.Fatalf("controller history incomplete: %+v", ctrl.Pods)
	}

	diag, err := c.GetNodeMemoryDiagnosticsHistoryWithContext(t.Context(), "", testPrometheusInfo(), "node-1", start, end, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(diag.Memory.Available) != 1 || len(diag.Memory.Reserved) != 1 {
		t.Fatalf("diagnostics incomplete: %+v", diag.Memory)
	}
}
