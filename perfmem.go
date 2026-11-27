package main

import (
	"runtime/debug"
	"runtime/metrics"
	"sync"
	"time"
)

// runtime.ReadMemStats stops the world, which the performance dashboard paid on
// every refresh. The same numbers are available without stopping the world from
// runtime/metrics (heap and stack classes, GC goal and CPU split) and
// debug.ReadGCStats (pause history, which runtime/metrics only offers as a
// histogram). readMemSnapshot assembles the subset of MemStats the dashboard
// exposes, mapped as documented in the runtime/metrics package:
//
//	Alloc, HeapAlloc = heap/objects
//	TotalAlloc       = gc/heap/allocs
//	Sys              = memory/classes/total
//	HeapInuse        = heap/objects + heap/unused
//	HeapIdle         = heap/free + heap/released
//	HeapSys          = HeapInuse + HeapIdle
//	HeapReleased     = heap/released
//	StackInuse       = heap/stacks
//	StackSys         = heap/stacks + os-stacks
//	MSpanInuse       = metadata/mspan/inuse
//	MCacheInuse      = metadata/mcache/inuse
//	NextGC           = gc/heap/goal
//	GCCPUFraction    = cpu/classes/gc/total / cpu/classes/total
type memSnapshot struct {
	Alloc, TotalAlloc, Sys                  uint64
	HeapAlloc, HeapSys, HeapIdle, HeapInuse uint64
	HeapReleased, StackInuse, StackSys      uint64
	MSpanInuse, MCacheInuse                 uint64
	NumGC                                   uint32
	LastGCPauseNs, TotalPauseNs             uint64
	NextGC                                  uint64
	GCCPUFraction                           float64
}

var memMetricNames = []string{
	"/memory/classes/heap/objects:bytes",
	"/memory/classes/heap/unused:bytes",
	"/memory/classes/heap/free:bytes",
	"/memory/classes/heap/released:bytes",
	"/memory/classes/heap/stacks:bytes",
	"/memory/classes/os-stacks:bytes",
	"/memory/classes/metadata/mspan/inuse:bytes",
	"/memory/classes/metadata/mcache/inuse:bytes",
	"/memory/classes/total:bytes",
	"/gc/heap/allocs:bytes",
	"/gc/heap/goal:bytes",
	"/cpu/classes/gc/total:cpu-seconds",
	"/cpu/classes/total:cpu-seconds",
}

var (
	memSamplePool = sync.Pool{New: func() any {
		s := make([]metrics.Sample, len(memMetricNames))
		for i, n := range memMetricNames {
			s[i].Name = n
		}
		return &s
	}}
)

func sampleUint(v metrics.Value) uint64 {
	if v.Kind() == metrics.KindUint64 {
		return v.Uint64()
	}
	return 0
}

func sampleFloat(v metrics.Value) float64 {
	switch v.Kind() {
	case metrics.KindFloat64:
		return v.Float64()
	case metrics.KindUint64:
		return float64(v.Uint64())
	}
	return 0
}

// readMemSnapshot reads memory and GC statistics without a stop-the-world pause.
func readMemSnapshot() memSnapshot {
	sp := memSamplePool.Get().(*[]metrics.Sample)
	defer memSamplePool.Put(sp)
	samples := *sp
	metrics.Read(samples)

	objects := sampleUint(samples[0].Value)
	unused := sampleUint(samples[1].Value)
	free := sampleUint(samples[2].Value)
	released := sampleUint(samples[3].Value)
	stacks := sampleUint(samples[4].Value)
	osStacks := sampleUint(samples[5].Value)

	var snap memSnapshot
	snap.Alloc = objects
	snap.HeapAlloc = objects
	snap.HeapInuse = objects + unused
	snap.HeapIdle = free + released
	snap.HeapSys = snap.HeapInuse + snap.HeapIdle
	snap.HeapReleased = released
	snap.StackInuse = stacks
	snap.StackSys = stacks + osStacks
	snap.MSpanInuse = sampleUint(samples[6].Value)
	snap.MCacheInuse = sampleUint(samples[7].Value)
	snap.Sys = sampleUint(samples[8].Value)
	snap.TotalAlloc = sampleUint(samples[9].Value)
	snap.NextGC = sampleUint(samples[10].Value)
	if total := sampleFloat(samples[12].Value); total > 0 {
		snap.GCCPUFraction = sampleFloat(samples[11].Value) / total
	}

	// Pause history: ReadGCStats does not stop the world. Pause[0] is the most
	// recent pause.
	var gc debug.GCStats
	gc.Pause = make([]time.Duration, 0, 1)
	debug.ReadGCStats(&gc)
	snap.NumGC = uint32(gc.NumGC)
	snap.TotalPauseNs = uint64(gc.PauseTotal)
	if len(gc.Pause) > 0 {
		snap.LastGCPauseNs = uint64(gc.Pause[0])
	}
	return snap
}
