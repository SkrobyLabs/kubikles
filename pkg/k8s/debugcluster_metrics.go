//go:build debugcluster

package k8s

import (
	"hash/fnv"
	"os"
	"time"
)

// EnvDebugMetrics turns on synthetic metrics for the debug cluster (for browser checks of the
// metrics columns). Without it the debug cluster reports no metrics, as before. The values
// derive from the object names and drift every few seconds, so successive polls differ.
const EnvDebugMetrics = "KUBIKLES_DEBUG_METRICS"

func debugMetricsOn() bool {
	switch os.Getenv(EnvDebugMetrics) {
	case "1", "true", "on":
		return true
	}
	return false
}

// debugJitter is a deterministic number in [0, n) from a name and the current 5 second bucket.
func debugJitter(name string, bucket int64, n int64) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(name))
	v := h.Sum64() + uint64(bucket)*2654435761
	return int64(v % uint64(n))
}

func (c *Client) debugNodeMetrics() (*NodeMetricsResult, error) {
	if !debugMetricsOn() {
		return &NodeMetricsResult{Available: false, Error: "metrics not available on debug cluster"}, nil
	}
	nodes, err := c.snapshotNodes()
	if err != nil {
		return &NodeMetricsResult{Available: false, Error: err.Error()}, nil
	}
	pods, err := c.snapshotPods()
	if err != nil {
		return &NodeMetricsResult{Available: false, Error: err.Error()}, nil
	}
	bucket := time.Now().Unix() / 5
	count := map[string]int64{}
	for i := range pods.Items {
		count[pods.Items[i].Spec.NodeName]++
	}
	res := &NodeMetricsResult{Available: true}
	for i := range nodes.Items {
		n := &nodes.Items[i]
		cpuCap := n.Status.Capacity.Cpu().MilliValue()
		memCap := n.Status.Capacity.Memory().Value()
		cpuUse := cpuCap * (10 + debugJitter(n.Name, bucket, 60)) / 100
		memUse := memCap * (15 + debugJitter(n.Name+"m", bucket, 60)) / 100
		res.Metrics = append(res.Metrics, NodeMetrics{
			Name: n.Name, CPUUsage: cpuUse, MemoryUsage: memUse, CPUCapacity: cpuCap, MemCapacity: memCap,
			CPURequested: cpuCap / 3, MemRequested: memCap / 4, CPUCommitted: max(cpuUse, cpuCap/3), MemCommitted: max(memUse, memCap/4),
			PodCount: count[n.Name], PodCapacity: 110,
		})
	}
	return res, nil
}

func (c *Client) debugPodMetrics() (*PodMetricsResult, error) {
	if !debugMetricsOn() {
		return &PodMetricsResult{Available: false, Error: "metrics not available on debug cluster"}, nil
	}
	nodes, err := c.snapshotNodes()
	if err != nil {
		return &PodMetricsResult{Available: false, Error: err.Error()}, nil
	}
	pods, err := c.snapshotPods()
	if err != nil {
		return &PodMetricsResult{Available: false, Error: err.Error()}, nil
	}
	type capacity struct{ cpu, mem int64 }
	caps := map[string]capacity{}
	for i := range nodes.Items {
		caps[nodes.Items[i].Name] = capacity{nodes.Items[i].Status.Capacity.Cpu().MilliValue(), nodes.Items[i].Status.Capacity.Memory().Value()}
	}
	bucket := time.Now().Unix() / 5
	res := &PodMetricsResult{Available: true}
	for i := range pods.Items {
		p := &pods.Items[i]
		// Pods without a node have no sample, like the real aggregation.
		if p.Spec.NodeName == "" {
			continue
		}
		key := p.Namespace + "/" + p.Name
		cap := caps[p.Spec.NodeName]
		cpuReq, memReq := effectivePodRequests(p)
		cpuUse := 5 + debugJitter(key, bucket, 400)
		memUse := (20 + debugJitter(key+"m", bucket, 300)) << 20
		res.Metrics = append(res.Metrics, PodMetrics{
			Namespace: p.Namespace, Name: p.Name, NodeName: p.Spec.NodeName,
			CPUUsage: cpuUse, MemoryUsage: memUse, CPURequested: cpuReq, MemRequested: memReq,
			CPUCommitted: max(cpuUse, cpuReq), MemCommitted: max(memUse, memReq),
			NodeCPUCapacity: cap.cpu, NodeMemCapacity: cap.mem,
		})
	}
	return res, nil
}
