package main

import (
	"runtime"
	"runtime/metrics"
	"testing"
)

func within(a, b uint64, pct float64) bool {
	hi, lo := a, b
	if lo > hi {
		hi, lo = lo, hi
	}
	if hi == 0 {
		return true
	}
	return float64(hi-lo)/float64(hi) <= pct
}

func TestMemMetricNamesExistInThisRuntime(t *testing.T) {
	known := map[string]bool{}
	for _, d := range metrics.All() {
		known[d.Name] = true
	}
	for _, n := range memMetricNames {
		if !known[n] {
			t.Errorf("runtime/metrics has no %q; update the mapping in perfmem.go", n)
		}
	}
}

func TestReadMemSnapshotAgreesWithMemStats(t *testing.T) {
	// Hold a recognisable amount of live heap so the numbers are not tiny.
	keep := make([][]byte, 64)
	for i := range keep {
		keep[i] = make([]byte, 256<<10)
	}
	runtime.GC()

	snap := readMemSnapshot()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	runtime.KeepAlive(keep)

	// Allocation continues between the two reads, so compare with tolerance.
	checks := []struct {
		name      string
		got, want uint64
		pct       float64
	}{
		{"HeapAlloc", snap.HeapAlloc, ms.HeapAlloc, 0.25},
		{"Alloc", snap.Alloc, ms.Alloc, 0.25},
		{"HeapInuse", snap.HeapInuse, ms.HeapInuse, 0.25},
		{"HeapSys", snap.HeapSys, ms.HeapSys, 0.25},
		{"HeapIdle", snap.HeapIdle, ms.HeapIdle, 0.35},
		{"Sys", snap.Sys, ms.Sys, 0.25},
		{"StackInuse", snap.StackInuse, ms.StackInuse, 0.5},
		{"StackSys", snap.StackSys, ms.StackSys, 0.5},
		{"MSpanInuse", snap.MSpanInuse, ms.MSpanInuse, 0.35},
		{"MCacheInuse", snap.MCacheInuse, ms.MCacheInuse, 0.35},
		{"TotalAlloc", snap.TotalAlloc, ms.TotalAlloc, 0.25},
		{"NextGC", snap.NextGC, ms.NextGC, 0.25},
	}
	for _, c := range checks {
		if c.got == 0 {
			t.Errorf("%s is zero", c.name)
			continue
		}
		if !within(c.got, c.want, c.pct) {
			t.Errorf("%s: snapshot %d, MemStats %d", c.name, c.got, c.want)
		}
	}
	if snap.HeapAlloc < 64*(256<<10) {
		t.Errorf("HeapAlloc %d does not include the live buffers", snap.HeapAlloc)
	}
	if snap.NumGC == 0 || snap.NumGC < ms.NumGC-1 || snap.NumGC > ms.NumGC+1 {
		t.Errorf("NumGC snapshot %d, MemStats %d", snap.NumGC, ms.NumGC)
	}
	if snap.TotalPauseNs == 0 || snap.LastGCPauseNs == 0 {
		t.Errorf("pause fields empty: total %d last %d", snap.TotalPauseNs, snap.LastGCPauseNs)
	}
	if !within(snap.TotalPauseNs, ms.PauseTotalNs, 0.5) {
		t.Errorf("TotalPauseNs snapshot %d, MemStats %d", snap.TotalPauseNs, ms.PauseTotalNs)
	}
	if snap.GCCPUFraction < 0 || snap.GCCPUFraction > 1 {
		t.Errorf("GCCPUFraction out of range: %v", snap.GCCPUFraction)
	}
}

func TestReadMemSnapshotConcurrent(t *testing.T) {
	done := make(chan struct{})
	for i := 0; i < 8; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			for j := 0; j < 200; j++ {
				if readMemSnapshot().Sys == 0 {
					t.Error("zero Sys")
					return
				}
			}
		}()
	}
	for i := 0; i < 8; i++ {
		<-done
	}
}

func BenchmarkReadMemSnapshot(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = readMemSnapshot()
	}
}

func BenchmarkReadMemStatsSTW(b *testing.B) {
	var m runtime.MemStats
	for i := 0; i < b.N; i++ {
		runtime.ReadMemStats(&m)
	}
}
