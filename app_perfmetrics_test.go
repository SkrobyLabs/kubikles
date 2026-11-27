package main

import (
	"sync"
	"testing"
)

func newPerfApp() *App {
	return &App{
		eventStats:            make(map[string]*WatcherEventStats),
		metricsRequestManager: NewMetricsRequestManager(),
		listRequestManager:    NewListRequestManager(),
	}
}

func TestRecordWatcherEventCountsByType(t *testing.T) {
	a := newPerfApp()
	for i := 0; i < 3; i++ {
		a.recordWatcherEvent("pods:default", "ADDED")
	}
	a.recordWatcherEvent("pods:default", "MODIFIED")
	a.recordWatcherEvent("pods:default", "DELETED")
	a.recordWatcherEvent("nodes:", "MODIFIED")

	m := a.GetPerformanceMetrics()
	if m.Activity.TotalEvents != 6 {
		t.Fatalf("total events = %d, want 6", m.Activity.TotalEvents)
	}
	top := m.Activity.TopWatchers
	if len(top) != 2 || top[0].Key != "pods:default" {
		t.Fatalf("top watchers = %+v", top)
	}
	p := top[0]
	if p.Added != 3 || p.Modified != 1 || p.Deleted != 1 || p.TotalEvents != 5 || p.LastEventMs == 0 {
		t.Fatalf("pods stats = %+v", p)
	}
}

func TestRecordWatcherEventWorksWithoutInitialisedMap(t *testing.T) {
	a := &App{}
	a.recordWatcherEvent("k", "ADDED") // must not panic on a nil map
	if a.eventStats["k"] == nil || a.eventStats["k"].Added != 1 {
		t.Fatalf("stats not created: %+v", a.eventStats)
	}
}

func TestClearEventStatsDropsWatcher(t *testing.T) {
	a := newPerfApp()
	stats := a.watcherStatsFor("pods:")
	a.recordEvent(stats, "ADDED")
	a.clearEventStats("pods:")
	if len(a.GetPerformanceMetrics().Activity.TopWatchers) != 0 {
		t.Fatal("cleared watcher still reported")
	}
	// A loop that still holds the old pointer must not resurrect the key.
	a.recordEvent(stats, "MODIFIED")
	if len(a.GetPerformanceMetrics().Activity.TopWatchers) != 0 {
		t.Fatal("late event resurrected a cleared watcher")
	}
}

// Many watch goroutines record while the dashboard polls; run with -race.
func TestRecordWatcherEventConcurrentWithMetricsReads(t *testing.T) {
	a := newPerfApp()
	const writers, perWriter = 8, 2000
	var wg sync.WaitGroup
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			stats := a.watcherStatsFor("pods:ns" + string(rune('a'+w%3)))
			for i := 0; i < perWriter; i++ {
				a.recordEvent(stats, []string{"ADDED", "MODIFIED", "DELETED"}[i%3])
			}
		}(w)
	}
	stop := make(chan struct{})
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		for {
			select {
			case <-stop:
				return
			default:
				_ = a.GetPerformanceMetrics()
			}
		}
	}()
	wg.Wait()
	close(stop)
	<-readerDone

	m := a.GetPerformanceMetrics()
	if m.Activity.TotalEvents != writers*perWriter {
		t.Fatalf("total = %d, want %d", m.Activity.TotalEvents, writers*perWriter)
	}
}
