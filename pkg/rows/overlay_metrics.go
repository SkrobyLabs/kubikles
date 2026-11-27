package rows

import (
	"math"
	"strconv"
)

// Metrics overlays. The sources behind them (metrics-server, Prometheus) are polled outside this
// package; what lives here is the part that must match the legacy lists number for number: the
// record layout, the percentage rounding of utils/metricsTransform.ts and the cells and filters
// the pod, node and namespace specs derive from a record.

// Names of the overlay sources.
const (
	OverlayPodMetrics       = "podmetrics"
	OverlayNodeMetrics      = "nodemetrics"
	OverlayNamespaceMetrics = "namespacemetrics"
)

// Pod record layout (POD_RECORD_FIELDS in metricsTransform.ts).
const (
	podCPUPercent = iota
	podMemPercent
	podCPUReservedPercent
	podMemReservedPercent
	podCPUCommittedPercent
	podMemCommittedPercent
	podCPUUsage
	podMemoryUsage
	podCPURequested
	podMemRequested
	podCPUCommitted
	podMemCommitted
	podNodeCPUCapacity
	podNodeMemCapacity
	podRecordLen
)

// Node record layout (NODE_RECORD_FIELDS).
const (
	nodeCPUPercent = iota
	nodeMemPercent
	nodeCPUReservedPercent
	nodeMemReservedPercent
	nodeCPUCommittedPercent
	nodeMemCommittedPercent
	nodePodPercent
	nodeCPUUsage
	nodeMemoryUsage
	nodeCPUCapacity
	nodeMemCapacity
	nodeCPURequested
	nodeMemRequested
	nodeCPUCommitted
	nodeMemCommitted
	nodePodCount
	nodePodCapacity
	nodeRecordLen
)

// Namespace record layout (NAMESPACE_RECORD_FIELDS).
const (
	nsCPUUsagePercent = iota
	nsMemUsagePercent
	nsCPUReservedPercent
	nsMemReservedPercent
	nsCPUCommittedPercent
	nsMemCommittedPercent
	nsCPUUsage
	nsMemUsage
	nsCPURequested
	nsMemRequested
	nsCPUCommitted
	nsMemCommitted
	nsPodCount
	nsRecordLen
)

// jsRound is Math.round: halves round up (for the non-negative values percentages take).
func jsRound(x float64) float64 {
	f := math.Floor(x)
	if x-f >= 0.5 {
		return f + 1
	}
	return f
}

// metricPercent is `capacity > 0 ? Math.round((value / capacity) * 100) : 0`.
func metricPercent(value, capacity float64) float64 {
	if capacity > 0 {
		// The explicit conversion keeps the compiler from fusing the multiplication into the
		// arithmetic of jsRound, which would change the rounding of exact halves.
		return jsRound(float64(float64(value/capacity) * 100))
	}
	return 0
}

// PodMetricInput is one pod's metrics as the backend sources produce them (k8s.PodMetrics).
type PodMetricInput struct {
	Namespace, Name                                                 string
	CPUUsage, MemoryUsage, CPURequested, MemRequested, CPUCommitted int64
	MemCommitted, NodeCPUCapacity, NodeMemCapacity                  int64
}

// NodeMetricInput is one node's metrics (k8s.NodeMetrics).
type NodeMetricInput struct {
	Name                                                            string
	CPUUsage, MemoryUsage, CPUCapacity, MemCapacity, CPURequested   int64
	MemRequested, CPUCommitted, MemCommitted, PodCount, PodCapacity int64
}

// PodKey is the key of a pod's record: "namespace/name".
func PodKey(namespace, name string) string { return namespace + "/" + name }

// NodeKey is the key of a node's record: its name.
func NodeKey(_, name string) string { return name }

// PodRecords is transformPodMetrics followed by the record encoding. Later entries replace
// earlier ones with the same key, like assignments to a JavaScript object.
func PodRecords(in []PodMetricInput) map[string]OverlayRecord {
	out := make(map[string]OverlayRecord, len(in))
	for _, m := range in {
		rec := make(OverlayRecord, podRecordLen)
		cpuCap, memCap := float64(m.NodeCPUCapacity), float64(m.NodeMemCapacity)
		rec[podCPUPercent] = metricPercent(float64(m.CPUUsage), cpuCap)
		rec[podMemPercent] = metricPercent(float64(m.MemoryUsage), memCap)
		rec[podCPUReservedPercent] = metricPercent(float64(m.CPURequested), cpuCap)
		rec[podMemReservedPercent] = metricPercent(float64(m.MemRequested), memCap)
		rec[podCPUCommittedPercent] = metricPercent(float64(m.CPUCommitted), cpuCap)
		rec[podMemCommittedPercent] = metricPercent(float64(m.MemCommitted), memCap)
		rec[podCPUUsage] = float64(m.CPUUsage)
		rec[podMemoryUsage] = float64(m.MemoryUsage)
		rec[podCPURequested] = float64(m.CPURequested)
		rec[podMemRequested] = float64(m.MemRequested)
		rec[podCPUCommitted] = float64(m.CPUCommitted)
		rec[podMemCommitted] = float64(m.MemCommitted)
		rec[podNodeCPUCapacity] = cpuCap
		rec[podNodeMemCapacity] = memCap
		out[PodKey(m.Namespace, m.Name)] = rec
	}
	return out
}

// NodeRecords is transformNodeMetrics followed by the record encoding.
func NodeRecords(in []NodeMetricInput) map[string]OverlayRecord {
	out := make(map[string]OverlayRecord, len(in))
	for _, m := range in {
		rec := make(OverlayRecord, nodeRecordLen)
		cpuCap, memCap := float64(m.CPUCapacity), float64(m.MemCapacity)
		rec[nodeCPUPercent] = metricPercent(float64(m.CPUUsage), cpuCap)
		rec[nodeMemPercent] = metricPercent(float64(m.MemoryUsage), memCap)
		rec[nodeCPUReservedPercent] = metricPercent(float64(m.CPURequested), cpuCap)
		rec[nodeMemReservedPercent] = metricPercent(float64(m.MemRequested), memCap)
		rec[nodeCPUCommittedPercent] = metricPercent(float64(m.CPUCommitted), cpuCap)
		rec[nodeMemCommittedPercent] = metricPercent(float64(m.MemCommitted), memCap)
		rec[nodePodPercent] = metricPercent(float64(m.PodCount), float64(m.PodCapacity))
		rec[nodeCPUUsage] = float64(m.CPUUsage)
		rec[nodeMemoryUsage] = float64(m.MemoryUsage)
		rec[nodeCPUCapacity] = cpuCap
		rec[nodeMemCapacity] = memCap
		rec[nodeCPURequested] = float64(m.CPURequested)
		rec[nodeMemRequested] = float64(m.MemRequested)
		rec[nodeCPUCommitted] = float64(m.CPUCommitted)
		rec[nodeMemCommitted] = float64(m.MemCommitted)
		rec[nodePodCount] = float64(m.PodCount)
		rec[nodePodCapacity] = float64(m.PodCapacity)
		out[m.Name] = rec
	}
	return out
}

// NamespaceRecords is aggregateNamespaceMetrics over the transformed pod and node maps: pod
// usage summed per namespace, as percentages of the summed node capacity.
func NamespaceRecords(pods []PodMetricInput, nodes []NodeMetricInput) map[string]OverlayRecord {
	// The legacy aggregation runs over the maps, so duplicate keys count once (the last wins).
	podMap := make(map[string]PodMetricInput, len(pods))
	var podOrder []string
	for _, p := range pods {
		k := PodKey(p.Namespace, p.Name)
		if _, seen := podMap[k]; !seen {
			podOrder = append(podOrder, k)
		}
		podMap[k] = p
	}
	nodeMap := make(map[string]NodeMetricInput, len(nodes))
	for _, n := range nodes {
		nodeMap[n.Name] = n
	}
	var cpuCap, memCap float64
	for _, n := range nodeMap {
		cpuCap += float64(n.CPUCapacity)
		memCap += float64(n.MemCapacity)
	}
	type agg struct{ cpuUsage, memUsage, cpuReq, memReq, cpuCom, memCom, count float64 }
	byNS := map[string]*agg{}
	for _, k := range podOrder {
		p := podMap[k]
		a := byNS[p.Namespace]
		if a == nil {
			a = &agg{}
			byNS[p.Namespace] = a
		}
		a.cpuUsage += float64(p.CPUUsage)
		a.memUsage += float64(p.MemoryUsage)
		a.cpuReq += float64(p.CPURequested)
		a.memReq += float64(p.MemRequested)
		a.cpuCom += float64(p.CPUCommitted)
		a.memCom += float64(p.MemCommitted)
		a.count++
	}
	out := make(map[string]OverlayRecord, len(byNS))
	for ns, a := range byNS {
		rec := make(OverlayRecord, nsRecordLen)
		rec[nsCPUUsagePercent] = metricPercent(a.cpuUsage, cpuCap)
		rec[nsMemUsagePercent] = metricPercent(a.memUsage, memCap)
		rec[nsCPUReservedPercent] = metricPercent(a.cpuReq, cpuCap)
		rec[nsMemReservedPercent] = metricPercent(a.memReq, memCap)
		rec[nsCPUCommittedPercent] = metricPercent(a.cpuCom, cpuCap)
		rec[nsMemCommittedPercent] = metricPercent(a.memCom, memCap)
		rec[nsCPUUsage] = a.cpuUsage
		rec[nsMemUsage] = a.memUsage
		rec[nsCPURequested] = a.cpuReq
		rec[nsMemRequested] = a.memReq
		rec[nsCPUCommitted] = a.cpuCom
		rec[nsMemCommitted] = a.memCom
		rec[nsPodCount] = a.count
		out[ns] = rec
	}
	return out
}

// recordText is metricsRecordText: the record's numbers joined with commas, the display cell of
// the overlay's metrics column. Whole numbers (everything the sources produce) take a fast path.
func recordText(rec OverlayRecord) string {
	if len(rec) == 0 {
		return ""
	}
	var stack [160]byte
	buf := stack[:0]
	for i, v := range rec {
		if i > 0 {
			buf = append(buf, ',')
		}
		if v == math.Trunc(v) && math.Abs(v) < 1e15 {
			buf = strconv.AppendInt(buf, int64(v), 10)
		} else {
			buf = strconv.AppendFloat(buf, v, 'f', -1, 64)
		}
	}
	return string(buf)
}

// overlayNumber reads one field of the record found for key, ok=false without a record.
func overlayNumber(r *Row, source, key string, field int) (float64, bool) {
	rec := r.Overlay(source).Lookup(key)
	if field >= len(rec) {
		return 0, false
	}
	return rec[field], true
}

// overlayCell is `record?.field ?? -1`: the sort cell of a metrics column.
func overlayCell(r *Row, source, key string, field int) interface{} {
	if v, ok := overlayNumber(r, source, key, field); ok {
		return v
	}
	return float64(-1)
}

// overlayNumeric is `record?.field ?? NaN`: ok=false stands for NaN.
func overlayNumeric(r *Row, source, key string, field int) (float64, bool) {
	return overlayNumber(r, source, key, field)
}

// overlayRecordCell is the display cell: the record text, "" without a record.
func overlayRecordCell(r *Row, source, key string) interface{} {
	return recordText(r.Overlay(source).Lookup(key))
}
