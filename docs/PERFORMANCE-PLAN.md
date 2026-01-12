# Kubikles Performance Optimization Plan

**Created**: 2026-01-08
**Status**: Active
**Test Safety**: `pkg/k8s/dependencies_test.go` covers core dependency graph logic with fake k8s clients. Run `go test ./pkg/k8s/... -v` before and after changes.

---

## Executive Summary

Four-agent analysis identified 15+ optimization opportunities. The biggest bottleneck is `pkg/k8s/dependencies.go` which makes 19+ sequential API calls per dependency graph with no caching or field selectors.

**Estimated improvement from top 5 fixes: 5-20x faster for typical operations.**

---

## Test Commands

```bash
# Run all backend tests
go test ./... -v

# Run with coverage
go test ./pkg/k8s/... -cover

# Run specific dependency tests
go test ./pkg/k8s/... -run TestGet -v

# Frontend tests
cd frontend && npm test
```

---

## Phase 1: Quick Wins (Do First)

### 1.1 Request-Scoped Cache for Dependencies

**File**: `pkg/k8s/dependencies.go`
**Problem**: Functions like `findServicesSelectingPod()` list all services multiple times within one graph build.
**Solution**: Add cache parameter to traversal context.

```go
// Add to function signatures:
type dependencyContext struct {
    cs        kubernetes.Interface
    namespace string
    graph     *DependencyGraph
    nodeMap   map[string]bool
    // NEW: request-scoped caches
    podCache     []corev1.Pod
    serviceCache []corev1.Service
    ingressCache []networkingv1.Ingress
}

// Helper to get cached or fetch:
func (ctx *dependencyContext) getPods() ([]corev1.Pod, error) {
    if ctx.podCache != nil {
        return ctx.podCache, nil
    }
    list, err := ctx.cs.CoreV1().Pods(ctx.namespace).List(context.TODO(), metav1.ListOptions{})
    if err != nil {
        return nil, err
    }
    ctx.podCache = list.Items
    return ctx.podCache, nil
}
```

**Test**: Existing tests use fake clients - they should still pass. Add benchmark test.

**Impact**: 3-10x faster dependency graphs
**Difficulty**: Easy (2-3 hours)

---

### 1.2 Add Field Selectors to List() Calls

**File**: `pkg/k8s/dependencies.go`
**Lines**: 715, 905, 1072, 1162, 1194, 1353, 1490, 1646, 1709, 1747, etc.
**Problem**: `ListOptions{}` fetches ALL resources cluster-wide.

**Before**:
```go
pods, err := cs.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{})
for _, pod := range pods.Items {
    if pod.Spec.PriorityClassName == name { ... }
}
```

**After**:
```go
pods, err := cs.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{
    FieldSelector: fields.OneTermEqualSelector("spec.priorityClassName", name).String(),
})
```

**Locations to fix** (grep for `ListOptions{}`):
| Line | Resource | Add Selector |
|------|----------|--------------|
| 1490 | Pods (PriorityClass) | `spec.priorityClassName` |
| 1646 | Ingresses (IngressClass) | `spec.ingressClassName` |
| 1747 | Pods (ServiceAccount) | `spec.serviceAccountName` |

**Note**: Not all fields are indexable. Check k8s docs. For non-indexable fields, use request-scoped cache instead.

**Impact**: 2-10x faster for large clusters
**Difficulty**: Easy (1-2 hours)

---

### 1.3 Edge Deduplication O(n) -> O(1)

**File**: `pkg/k8s/dependencies.go`
**Lines**: 429-435

**Before**:
```go
for _, e := range newGraph.Edges {  // O(n) scan
    if e.Source == edge.Source && e.Target == summaryID {
        edgeExists = true
        break
    }
}
```

**After**:
```go
// At top of aggregateGraph:
edgeSet := make(map[string]bool)
for _, e := range graph.Edges {
    edgeSet[e.Source+":"+e.Target+":"+e.Relation] = true
}

// In loop:
edgeKey := edge.Source + ":" + summaryID + ":" + edge.Relation
if !edgeSet[edgeKey] {
    newGraph.Edges = append(newGraph.Edges, ...)
    edgeSet[edgeKey] = true
}
```

**Test**: `TestAggregateGraphStorageClass` and `TestAggregateGraphPreservesConnectors` cover this.

**Impact**: O(n^2) -> O(n) for graphs with 100+ edges
**Difficulty**: Easy (30 minutes)

---

### 1.4 Context Timeouts

**File**: `pkg/k8s/dependencies.go`
**Problem**: All API calls use `context.TODO()` - can hang indefinitely.

**Before**:
```go
list, err := cs.CoreV1().Pods(namespace).List(context.TODO(), metav1.ListOptions{})
```

**After**:
```go
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()
list, err := cs.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
```

**Better**: Pass context from caller through entire call chain.

**Impact**: Prevents UI hangs on slow clusters
**Difficulty**: Easy (1 hour)

---

## Phase 2: Medium Effort (Do Next)

### 2.1 Helm Repository Index Cache

**File**: `pkg/helm/repo.go`
**Lines**: 632-741
**Problem**: `SearchChart()` reads index YAML from disk for every repo on every search.

**Solution**:
```go
type Client struct {
    // existing fields...
    indexCache      map[string]*repo.IndexFile
    indexCacheMutex sync.RWMutex
    indexCacheTTL   time.Duration // 5 minutes
    indexCacheTime  map[string]time.Time
}

func (c *Client) getIndexFile(repoName, indexPath string) (*repo.IndexFile, error) {
    c.indexCacheMutex.RLock()
    if idx, ok := c.indexCache[repoName]; ok {
        if time.Since(c.indexCacheTime[repoName]) < c.indexCacheTTL {
            c.indexCacheMutex.RUnlock()
            return idx, nil
        }
    }
    c.indexCacheMutex.RUnlock()

    // Load from disk
    idx, err := repo.LoadIndexFile(indexPath)
    if err != nil {
        return nil, err
    }

    c.indexCacheMutex.Lock()
    c.indexCache[repoName] = idx
    c.indexCacheTime[repoName] = time.Now()
    c.indexCacheMutex.Unlock()

    return idx, nil
}
```

**Impact**: 80-95% faster chart searches
**Difficulty**: Medium (2-3 hours)

---

### 2.2 Parallel Helm Repository Update

**File**: `pkg/helm/repo.go`
**Lines**: 566-603
**Problem**: Sequential downloads, 2-3s per repo.

**Solution**:
```go
func (c *Client) UpdateAllRepositories() error {
    // ...existing setup...

    var wg sync.WaitGroup
    errChan := make(chan error, len(f.Repositories))
    sem := make(chan struct{}, 5) // max 5 concurrent

    for _, r := range f.Repositories {
        wg.Add(1)
        go func(r *repo.Entry) {
            defer wg.Done()
            sem <- struct{}{}
            defer func() { <-sem }()

            chartRepo, err := repo.NewChartRepository(r, getter.All(c.settings))
            if err != nil {
                errChan <- err
                return
            }
            if _, err := chartRepo.DownloadIndexFile(); err != nil {
                errChan <- err
            }
        }(r)
    }

    wg.Wait()
    close(errChan)
    // collect errors...
}
```

**Impact**: 3-5x faster repo refresh
**Difficulty**: Medium (2 hours)

---

### 2.3 K8s Client Caching per Context

**File**: `pkg/k8s/client.go`
**Lines**: 1641-1671
**Problem**: `getClientForContext()` creates new clientset every call.

**Solution**:
```go
type Client struct {
    // existing fields...
    clientCache map[string]*kubernetes.Clientset
    cacheMutex  sync.RWMutex
}

func (c *Client) getClientForContext(contextName string) (*kubernetes.Clientset, error) {
    c.cacheMutex.RLock()
    if cs, ok := c.clientCache[contextName]; ok {
        c.cacheMutex.RUnlock()
        return cs, nil
    }
    c.cacheMutex.RUnlock()

    // Create new client...
    cs, err := kubernetes.NewForConfig(config)
    if err != nil {
        return nil, err
    }

    c.cacheMutex.Lock()
    c.clientCache[contextName] = cs
    c.cacheMutex.Unlock()

    return cs, nil
}
```

**Impact**: 3-5x faster for multi-cluster users
**Difficulty**: Medium (1-2 hours)

---

## Phase 3: Frontend Optimizations

### 3.1 React.memo All Detail Components

**Files**: `frontend/src/components/shared/*Details.jsx` (29 files)

**Before**:
```jsx
export default function PodDetails({ pod, tabContext = '' }) {
```

**After**:
```jsx
export default React.memo(function PodDetails({ pod, tabContext = '' }) {
    // ...
}, (prev, next) => prev.pod?.metadata?.uid === next.pod?.metadata?.uid);
```

**Impact**: 10-20% faster tab switching
**Difficulty**: Easy (2-3 hours total)

---

### 3.2 Lazy Load Monaco & XFlow

**Files**: `YamlEditor.jsx`, `DependencyGraph.jsx`

**Before**:
```jsx
import Editor from '@monaco-editor/react';
```

**After**:
```jsx
const Editor = React.lazy(() => import('@monaco-editor/react'));

// In render:
<Suspense fallback={<div className="animate-pulse bg-gray-800 h-full" />}>
    <Editor ... />
</Suspense>
```

**Impact**: 3-4MB bundle reduction
**Difficulty**: Medium (1-2 hours)

---

### 3.3 Split K8sContext

**File**: `frontend/src/context/K8sContext.jsx`
**Problem**: Any state change causes tree-wide re-renders.

**Solution**: Split into K8sDataContext (read) and K8sActionsContext (callbacks).

**Impact**: Significant reduction in re-renders
**Difficulty**: Medium (3-4 hours)

---

## Phase 4: Advanced (Future)

### 4.1 SharedInformerFactory

Replace polling List() calls with watches using `k8s.io/client-go/informers`.

**Impact**: 50-100x for real-time updates
**Difficulty**: Hard (20+ hours)

### 4.2 Metrics Caching

Cache `GetNodeMetrics()` and `GetPodMetrics()` results for 10-15 seconds.

**Impact**: 5-10x on metrics tab
**Difficulty**: Medium (2-3 hours)

---

## Implementation Order

| Priority | Task | Time | Impact |
|----------|------|------|--------|
| 1 | Request-scoped cache | 2-3h | HIGH |
| 2 | Field selectors | 1-2h | HIGH |
| 3 | Edge dedup O(1) | 30m | MEDIUM |
| 4 | Context timeouts | 1h | MEDIUM |
| 5 | Helm index cache | 2-3h | HIGH |
| 6 | Parallel repo update | 2h | MEDIUM |
| 7 | React.memo Details | 2-3h | MEDIUM |
| 8 | Client caching | 1-2h | MEDIUM |
| 9 | Lazy load Monaco | 1-2h | LOW |
| 10 | Split K8sContext | 3-4h | MEDIUM |

**Total estimated time for Phase 1-2**: ~15 hours
**Expected improvement**: 5-20x faster for typical operations

---

## Validation

After each change:

1. Run tests: `go test ./... -v`
2. Manual test: Open dependency graph for a Deployment with 10+ pods
3. Check no regressions in existing functionality
4. Measure improvement with pprof if available

---

## Profiling Setup

Created `profiling.go` with build tag. To use:

```bash
# Build with profiling
go build -tags profiling

# Run with pprof enabled
PPROF_PORT=6060 ./kubikles

# Profile CPU for 30 seconds
go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30

# View heap
go tool pprof http://localhost:6060/debug/pprof/heap
```

Bundle analysis available at `frontend/dist/stats.html` after `npm run build`.
