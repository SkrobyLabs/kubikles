package store

import (
	"sort"

	"k8s.io/apimachinery/pkg/runtime"
)

// Reader adapts a Manager to k8s.StoreReader: warm reads for consumers that are not lists in the
// UI (dependency graphs, the issue detector, MCP tools, namespace counts). Readers never start
// shards; only views and explicit pins do.
type Reader struct{ M *Manager }

// ReadObjects returns the objects of a warm, synced full-object shard. A cluster-wide shard
// answers a namespaced request by filtering; a namespaced shard never answers a cluster-wide one
// (it may not cover every namespace, and the union of namespace shards is unknowable here).
func (r Reader) ReadObjects(contextName, resourceType, namespace string) ([]runtime.Object, bool) {
	if r.M == nil {
		return nil, false
	}
	sh, ns := r.M.shardFor(Key{Context: contextName, Type: resourceType, Namespace: namespace})
	if sh == nil || !sh.peekable() {
		return nil, false
	}
	entries, _ := sh.Snapshot(ns)
	// Snapshot order is unspecified; the API server lists in key order and readers (tool output,
	// graphs) expect the same, so sort by namespace then name.
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Namespace != entries[j].Namespace {
			return entries[i].Namespace < entries[j].Namespace
		}
		return entries[i].Name < entries[j].Name
	})
	out := make([]runtime.Object, len(entries))
	for i, e := range entries {
		out[i] = e.Obj
	}
	r.M.mu.Lock()
	sh.lastUsed = r.M.clock.Now()
	r.M.mu.Unlock()
	return out, true
}

// CountObjects returns the number of objects in a warm shard (or of one namespace in a cluster-wide
// shard) without copying them. ok is false when no synced shard exists, whatever its mode: a
// Table or metadata shard knows how many objects there are too.
func (r Reader) CountObjects(contextName, resourceType, namespace string) (int, bool) {
	if r.M == nil {
		return 0, false
	}
	sh, ns := r.M.shardFor(Key{Context: contextName, Type: resourceType, Namespace: namespace})
	if sh == nil {
		return 0, false
	}
	sh.mu.RLock()
	ok := sh.synced && (sh.phase == PhaseWatching || sh.phase == PhaseRefreshing)
	sh.mu.RUnlock()
	if !ok {
		return 0, false
	}
	entries, _ := sh.Snapshot(ns)
	return len(entries), true
}
