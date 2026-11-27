package main

import (
	goruntime "runtime"
	"sort"
	"sync/atomic"
	"time"

	"kubikles/pkg/rows"
	"kubikles/pkg/store"
)

// =============================================================================
// Performance Metrics
// =============================================================================

// WatcherEventStats tracks event counts and timing for a single watcher
type WatcherEventStats struct {
	Key          string  `json:"key"`          // Watcher key (e.g., "pods:default")
	Added        int64   `json:"added"`        // ADDED event count
	Modified     int64   `json:"modified"`     // MODIFIED event count
	Deleted      int64   `json:"deleted"`      // DELETED event count
	TotalEvents  int64   `json:"totalEvents"`  // Total events (sum)
	LastEventMs  int64   `json:"lastEventMs"`  // Last event timestamp (Unix ms)
	EventsPerSec float64 `json:"eventsPerSec"` // Calculated rate
}

// ActivityStats tracks activity metrics sorted by event count
type ActivityStats struct {
	TopWatchers    []WatcherEventStats `json:"topWatchers"`    // Top watchers by event count
	TotalEvents    int64               `json:"totalEvents"`    // Total events across all watchers
	WindowStartMs  int64               `json:"windowStartMs"`  // Start of measurement window
	WindowDuration int64               `json:"windowDuration"` // Duration in ms
}

// PerformanceMetrics contains all performance-related metrics for the dashboard
type PerformanceMetrics struct {
	Timestamp int64 `json:"timestamp"` // Unix timestamp in ms

	// Go Runtime Memory Stats
	Memory struct {
		Alloc        uint64 `json:"alloc"`        // Bytes currently allocated
		TotalAlloc   uint64 `json:"totalAlloc"`   // Cumulative bytes allocated
		Sys          uint64 `json:"sys"`          // Total bytes obtained from OS
		HeapAlloc    uint64 `json:"heapAlloc"`    // Bytes allocated on heap
		HeapSys      uint64 `json:"heapSys"`      // Heap bytes obtained from OS
		HeapIdle     uint64 `json:"heapIdle"`     // Bytes in idle spans
		HeapInuse    uint64 `json:"heapInuse"`    // Bytes in in-use spans
		HeapReleased uint64 `json:"heapReleased"` // Bytes released to OS
		StackInuse   uint64 `json:"stackInuse"`   // Bytes in stack spans
		StackSys     uint64 `json:"stackSys"`     // Stack bytes obtained from OS
		MSpanInuse   uint64 `json:"mspanInuse"`   // Bytes in mspan structures
		MCacheInuse  uint64 `json:"mcacheInuse"`  // Bytes in mcache structures
	} `json:"memory"`

	// Garbage Collection Stats
	GC struct {
		NumGC         uint32  `json:"numGC"`         // Number of completed GC cycles
		LastGCPauseNs uint64  `json:"lastGCPauseNs"` // Duration of last GC pause
		TotalPauseNs  uint64  `json:"totalPauseNs"`  // Cumulative GC pause time
		NextGCBytes   uint64  `json:"nextGCBytes"`   // Target heap size for next GC
		GCCPUFraction float64 `json:"gcCPUFraction"` // Fraction of CPU used by GC
	} `json:"gc"`

	// Goroutine Stats
	Goroutines struct {
		Count       int `json:"count"`       // Current goroutine count
		MaxObserved int `json:"maxObserved"` // Peak observed during session
	} `json:"goroutines"`

	// Watcher Stats
	Watchers struct {
		Active       int      `json:"active"`       // Number of active watchers
		WatcherKeys  []string `json:"watcherKeys"`  // List of active watcher keys
		TotalCreated int64    `json:"totalCreated"` // Total watchers created in session
		TotalCleaned int64    `json:"totalCleaned"` // Total watchers cleaned up
	} `json:"watchers"`

	// Port Forward Stats
	PortForwards struct {
		Active  int `json:"active"`  // Active port forwards
		Configs int `json:"configs"` // Saved configurations
	} `json:"portForwards"`

	// Ingress Forward Stats
	IngressForwards struct {
		Active int `json:"active"` // Active ingress forwards
	} `json:"ingressForwards"`

	// Log Stream Stats
	LogStreams struct {
		Active int `json:"active"` // Active log streams
	} `json:"logStreams"`

	// Activity Stats - sorted by event count (highest first)
	Activity ActivityStats `json:"activity"`

	// Metrics Request Stats
	MetricsRequests MetricsRequestStats `json:"metricsRequests"`

	// List Request Stats
	ListRequests ListRequestStats `json:"listRequests"`

	// Shared store: shards, entries, estimated bytes, degraded fetch modes.
	// Nil when the store is not running.
	Store *store.Stats `json:"store,omitempty"`

	// Open row views: counts, last window size, version and cumulative notice and byte counters
	// (the panel derives rates). Nil when views are not running.
	Views []rows.ViewStats `json:"views,omitempty"`
}

// watcherStatsFor returns the counters for a watcher key, creating them on
// first use. Watch loops resolve this once and keep the pointer, so the
// per-event path in recordEvent touches no lock.
func (a *App) watcherStatsFor(watcherKey string) *WatcherEventStats {
	a.eventStatsMutex.RLock()
	stats, exists := a.eventStats[watcherKey]
	a.eventStatsMutex.RUnlock()
	if exists {
		return stats
	}

	a.eventStatsMutex.Lock()
	defer a.eventStatsMutex.Unlock()
	if stats, exists = a.eventStats[watcherKey]; exists {
		return stats
	}
	if a.eventStats == nil {
		a.eventStats = make(map[string]*WatcherEventStats)
	}
	stats = &WatcherEventStats{Key: watcherKey}
	a.eventStats[watcherKey] = stats
	return stats
}

// recordEvent bumps the counters with atomics. Counters may be read while
// being written by GetPerformanceMetrics, which loads them atomically; the
// snapshot is not a single consistent cut, which is fine for a dashboard.
func (a *App) recordEvent(stats *WatcherEventStats, eventType string) {
	switch eventType {
	case "ADDED":
		atomic.AddInt64(&stats.Added, 1)
	case "MODIFIED":
		atomic.AddInt64(&stats.Modified, 1)
	case "DELETED":
		atomic.AddInt64(&stats.Deleted, 1)
	}
	atomic.AddInt64(&stats.TotalEvents, 1)
	atomic.StoreInt64(&stats.LastEventMs, time.Now().UnixMilli())
}

// recordWatcherEvent records an event for a watcher key. Prefer resolving the
// stats once with watcherStatsFor and calling recordEvent per event.
func (a *App) recordWatcherEvent(watcherKey string, eventType string) {
	a.recordEvent(a.watcherStatsFor(watcherKey), eventType)
}

// clearEventStats removes event statistics for a watcher key (called when watcher is cleaned up)
func (a *App) clearEventStats(watcherKey string) {
	a.eventStatsMutex.Lock()
	defer a.eventStatsMutex.Unlock()
	delete(a.eventStats, watcherKey)
}

// GetPerformanceMetrics returns current performance metrics for the dashboard
func (a *App) GetPerformanceMetrics() PerformanceMetrics {
	m := readMemSnapshot()

	goroutineCount := goruntime.NumGoroutine()

	// Track max goroutines
	a.perfMutex.Lock()
	if goroutineCount > a.maxGoroutines {
		a.maxGoroutines = goroutineCount
	}
	maxGoroutines := a.maxGoroutines
	totalCreated := a.totalWatchersCreated
	totalCleaned := a.totalWatchersCleaned
	a.perfMutex.Unlock()

	// Get watcher stats
	var watcherKeys []string
	activeWatchers := 0
	if a.watcherManager != nil {
		a.watcherManager.mutex.RLock()
		activeWatchers = len(a.watcherManager.watchers)
		watcherKeys = make([]string, 0, activeWatchers)
		for key := range a.watcherManager.watchers {
			watcherKeys = append(watcherKeys, key)
		}
		a.watcherManager.mutex.RUnlock()
	}

	// Get port forward stats
	activePortForwards := 0
	portForwardConfigs := 0
	if a.portForwardManager != nil {
		a.portForwardManager.mutex.RLock()
		for _, pf := range a.portForwardManager.active {
			if pf.Status == "running" {
				activePortForwards++
			}
		}
		portForwardConfigs = len(a.portForwardManager.configs)
		a.portForwardManager.mutex.RUnlock()
	}

	// Get ingress forward stats
	activeIngressForwards := 0
	if a.ingressForwardManager != nil {
		a.ingressForwardManager.mutex.RLock()
		if a.ingressForwardManager.state.Active {
			activeIngressForwards = 1
		}
		a.ingressForwardManager.mutex.RUnlock()
	}

	// Get log stream stats
	a.logStreamsMutex.Lock()
	activeLogStreams := len(a.logStreams)
	a.logStreamsMutex.Unlock()

	metrics := PerformanceMetrics{
		Timestamp: time.Now().UnixMilli(),
	}

	// Memory stats
	metrics.Memory.Alloc = m.Alloc
	metrics.Memory.TotalAlloc = m.TotalAlloc
	metrics.Memory.Sys = m.Sys
	metrics.Memory.HeapAlloc = m.HeapAlloc
	metrics.Memory.HeapSys = m.HeapSys
	metrics.Memory.HeapIdle = m.HeapIdle
	metrics.Memory.HeapInuse = m.HeapInuse
	metrics.Memory.HeapReleased = m.HeapReleased
	metrics.Memory.StackInuse = m.StackInuse
	metrics.Memory.StackSys = m.StackSys
	metrics.Memory.MSpanInuse = m.MSpanInuse
	metrics.Memory.MCacheInuse = m.MCacheInuse

	// GC stats
	metrics.GC.NumGC = m.NumGC
	metrics.GC.LastGCPauseNs = m.LastGCPauseNs
	metrics.GC.TotalPauseNs = m.TotalPauseNs
	metrics.GC.NextGCBytes = m.NextGC
	metrics.GC.GCCPUFraction = m.GCCPUFraction

	// Goroutine stats
	metrics.Goroutines.Count = goroutineCount
	metrics.Goroutines.MaxObserved = maxGoroutines

	// Watcher stats
	metrics.Watchers.Active = activeWatchers
	metrics.Watchers.WatcherKeys = watcherKeys
	metrics.Watchers.TotalCreated = totalCreated
	metrics.Watchers.TotalCleaned = totalCleaned

	// Port forward stats
	metrics.PortForwards.Active = activePortForwards
	metrics.PortForwards.Configs = portForwardConfigs

	// Ingress forward stats
	metrics.IngressForwards.Active = activeIngressForwards

	// Log stream stats
	metrics.LogStreams.Active = activeLogStreams

	// Activity stats - collect and sort by total events
	now := time.Now().UnixMilli()
	a.eventStatsMutex.RLock()
	windowStart := a.eventWindowStart
	allStats := make([]WatcherEventStats, 0, len(a.eventStats))
	var totalEvents int64
	for _, stats := range a.eventStats {
		// Calculate events per second
		windowDuration := float64(now-windowStart) / 1000.0
		total := atomic.LoadInt64(&stats.TotalEvents)
		eventsPerSec := 0.0
		if windowDuration > 0 {
			eventsPerSec = float64(total) / windowDuration
		}
		statsCopy := WatcherEventStats{
			Key:          stats.Key,
			Added:        atomic.LoadInt64(&stats.Added),
			Modified:     atomic.LoadInt64(&stats.Modified),
			Deleted:      atomic.LoadInt64(&stats.Deleted),
			TotalEvents:  total,
			LastEventMs:  atomic.LoadInt64(&stats.LastEventMs),
			EventsPerSec: eventsPerSec,
		}
		allStats = append(allStats, statsCopy)
		totalEvents += total
	}
	a.eventStatsMutex.RUnlock()

	// Sort by total events (descending)
	sort.Slice(allStats, func(i, j int) bool {
		return allStats[i].TotalEvents > allStats[j].TotalEvents
	})

	// Return top 20 watchers
	topN := 20
	if len(allStats) < topN {
		topN = len(allStats)
	}
	metrics.Activity.TopWatchers = allStats[:topN]
	metrics.Activity.TotalEvents = totalEvents
	metrics.Activity.WindowStartMs = windowStart
	metrics.Activity.WindowDuration = now - windowStart

	// Metrics request stats
	metrics.MetricsRequests = a.metricsRequestManager.GetStats()

	// List request stats
	metrics.ListRequests = a.listRequestManager.GetStats()

	// Shared store stats
	metrics.Store = a.storeStats()
	if a.views != nil {
		metrics.Views = a.views.Stats()
	}

	return metrics
}
