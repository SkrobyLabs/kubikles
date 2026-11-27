package main

import (
	"errors"
	"math"
	"os"
	rtdebug "runtime/debug"
	"strconv"
	"time"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/metadata"

	"kubikles/pkg/debug"
	"kubikles/pkg/k8s"
	"kubikles/pkg/store"
	"kubikles/pkg/watchevents"
)

// envStoreMaxBytes overrides the store byte budget. 0 means unlimited.
const envStoreMaxBytes = "KUBIKLES_STORE_MAX_BYTES"

// storeClients feeds the shared store the application's authenticated clients.
type storeClients struct{ a *App }

var errNoK8sClient = errors.New("k8s client not initialized")

func (p storeClients) Kube(contextName string) (kubernetes.Interface, error) {
	if p.a.k8sClient == nil {
		return nil, errNoK8sClient
	}
	return p.a.k8sClient.KubeClient(contextName)
}

func (p storeClients) Dynamic(contextName string) (dynamic.Interface, error) {
	if p.a.k8sClient == nil {
		return nil, errNoK8sClient
	}
	return p.a.k8sClient.DynamicClient(contextName)
}

func (p storeClients) Metadata(contextName string) (metadata.Interface, error) {
	if p.a.k8sClient == nil {
		return nil, errNoK8sClient
	}
	md, err := p.a.k8sClient.MetadataClient(contextName)
	if err != nil {
		return nil, errors.Join(store.ErrNoMetadata, err)
	}
	return md, nil
}

// initStore creates the shared store next to the watcher manager. It is
// created on the same startup path for desktop and server mode. With no open
// shards it is inert, and the metrics Pod/Node source falls back to the API
// server until a shard is warm.
func (a *App) initStore() {
	if a.store != nil {
		a.store.Close()
	}
	a.store = store.New(storeClients{a}, store.Options{MaxBytes: storeMaxBytes()})
	a.store.SetSinks(
		func(e watchevents.Status) {
			debug.LogWatcher("store shard status", map[string]interface{}{"type": e.ResourceType, "namespace": e.Namespace, "status": e.Status})
			a.emitEvent("watcher-status", e)
		},
		func(e watchevents.Error) {
			debug.LogWatcher("store shard error", map[string]interface{}{"type": e.ResourceType, "namespace": e.Namespace, "error": e.Error, "recoverable": e.Recoverable})
			a.emitEvent("watcher-error", e)
		},
	)
	if a.k8sClient != nil {
		a.k8sClient.SetStoreReader(store.Reader{M: a.store})
		a.k8sClient.SetPodNodeSource(store.NewPodNodeSource(a.store, a.GetCurrentContext, k8s.APIPodNodeSource(a.k8sClient)))
	}
	a.initViews()
}

func storeMaxBytes() int64 {
	if v := os.Getenv(envStoreMaxBytes); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n >= 0 {
			return n
		}
	}
	return defaultStoreBudget(int64(containerMemoryLimit()), debugMemoryLimit())
}

// storeBudgetPercent is the share of the effective memory limit the store may fill with idle and
// leased shards together; the rest is left for the WebView bridge, the legacy watchers and GC
// headroom.
const storeBudgetPercent = 40

// defaultStoreBudget bounds the default budget by the container memory limit (cgroup) or the
// soft GOMEMLIMIT, whichever is smaller, so a pod with a small limit is not asked to cache more
// than it can hold. Zero or negative limits mean "no limit detected".
func defaultStoreBudget(cgroupBytes, goMemLimit int64) int64 {
	limit := int64(0)
	for _, l := range []int64{cgroupBytes, goMemLimit} {
		if l > 0 && (limit == 0 || l < limit) {
			limit = l
		}
	}
	if limit == 0 {
		return store.DefaultMaxBytes
	}
	if b := limit / 100 * storeBudgetPercent; b < store.DefaultMaxBytes {
		return b
	}
	return store.DefaultMaxBytes
}

// debugMemoryLimit returns the current soft memory limit, or 0 when none is set.
func debugMemoryLimit() int64 {
	l := rtdebug.SetMemoryLimit(-1)
	if l == math.MaxInt64 {
		return 0
	}
	return l
}

// resetStore stops every store shard. It runs wherever the legacy watchers are
// stopped for a context switch.
func (a *App) resetStore() {
	// Views hold leases on shards: close them first so the frontend sees "context" and re-opens.
	a.resetViews()
	if a.store != nil {
		a.store.Reset()
	}
}

// resyncStore makes every shard rebuild its clients and relist; the counterpart
// of ResourceWatcherManager.RestartAll.
func (a *App) resyncStore() {
	if a.store != nil {
		a.store.Resync()
	}
}

// closeStore stops the store for good (application shutdown).
func (a *App) closeStore() {
	a.closeViews()
	if a.store != nil {
		a.store.Close()
	}
}

// storeStats returns the store summary for the performance panel, or nil when
// the store is not running.
func (a *App) storeStats() *store.Stats {
	if a.store == nil {
		return nil
	}
	st := a.store.Stats()
	return &st
}

// metricsPinTTL is how long a metrics call keeps the pod and node shards warm. The metrics UI
// polls every 30 seconds, so the shards stay warm for as long as it is open and linger out
// shortly after it closes.
const metricsPinTTL = 90 * time.Second

// pinMetricsShards keeps already-open cluster-wide shards of the given types warm for the next
// metrics poll. It never starts a shard: with the windowed-lists setting off nothing else wants
// one, and the metrics source falls back to a plain list when no shard is warm.
func (a *App) pinMetricsShards(types ...string) {
	if a.store == nil {
		return
	}
	ctxName := a.GetCurrentContext()
	if ctxName == "" {
		return
	}
	a.pinExistingShards(ctxName, types...)
}

func (a *App) pinExistingShards(ctxName string, types ...string) {
	for _, typ := range types {
		a.store.PinExisting(store.Key{Context: ctxName, Type: typ}, "metrics", metricsPinTTL)
	}
}
