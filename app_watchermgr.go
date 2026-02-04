package main

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// =============================================================================
// Resource Watcher Manager
// =============================================================================

// WatcherCleanupDelay is the time to wait before stopping a watcher with no subscribers
const WatcherCleanupDelay = 5 * time.Second

// ResourceWatcher tracks a single watcher instance with reference counting
type ResourceWatcher struct {
	Key             string             // "resourceType:namespace" or "crd:group/version/resource:namespace"
	ResourceType    string             // Resource type identifier
	Namespace       string             // Namespace being watched (empty for cluster-scoped)
	Group           string             // API group (for CRDs)
	Version         string             // API version (for CRDs)
	Resource        string             // Resource plural name (for CRDs)
	IsCRD           bool               // Whether this is a CRD watcher
	RefCount        int32              // Atomic counter for subscribers
	Cancel          context.CancelFunc // Cancel function for the watch loop
	CleanupTimer    *time.Timer        // Delayed cleanup timer
	ResourceVersion string             // Last known resourceVersion for resumable watches
	mu              sync.RWMutex       // Protects ResourceVersion
}

// ResourceWatcherManager manages all active resource watchers with reference counting
type ResourceWatcherManager struct {
	ctx      context.Context
	app      *App
	watchers map[string]*ResourceWatcher
	mutex    sync.RWMutex
}

// NewResourceWatcherManager creates a new ResourceWatcherManager
func NewResourceWatcherManager(ctx context.Context, app *App) *ResourceWatcherManager {
	return &ResourceWatcherManager{
		ctx:      ctx,
		app:      app,
		watchers: make(map[string]*ResourceWatcher),
	}
}

// Subscribe subscribes to a resource watcher, returning the watcher key
// If a watcher already exists, it increments the refCount and cancels any pending cleanup
// If no watcher exists, it creates a new one and starts the watch loop
func (m *ResourceWatcherManager) Subscribe(resourceType, namespace string) string {
	key := resourceType + ":" + namespace

	m.mutex.Lock()
	defer m.mutex.Unlock()

	if watcher, exists := m.watchers[key]; exists {
		// Cancel any pending cleanup
		if watcher.CleanupTimer != nil {
			watcher.CleanupTimer.Stop()
			watcher.CleanupTimer = nil
		}
		// Increment ref count
		atomic.AddInt32(&watcher.RefCount, 1)
		m.app.logDebug("ResourceWatcher: Reusing existing watcher for %s (refCount=%d)", key, atomic.LoadInt32(&watcher.RefCount))
		return key
	}

	// Create new watcher
	ctx, cancel := context.WithCancel(context.Background())
	watcher := &ResourceWatcher{
		Key:          key,
		ResourceType: resourceType,
		Namespace:    namespace,
		IsCRD:        false,
		RefCount:     1,
		Cancel:       cancel,
	}
	m.watchers[key] = watcher

	// Track watcher creation for performance metrics
	m.app.perfMutex.Lock()
	m.app.totalWatchersCreated++
	m.app.perfMutex.Unlock()

	m.app.logDebug("ResourceWatcher: Starting new watcher for %s", key)

	// Start watch loop in goroutine
	go m.app.watchResourceLoop(ctx, resourceType, namespace)

	return key
}

// SubscribeCRD subscribes to a CRD watcher using GVR, returning the watcher key
func (m *ResourceWatcherManager) SubscribeCRD(group, version, resource, namespace string) string {
	key := "crd:" + group + "/" + version + "/" + resource + ":" + namespace

	m.mutex.Lock()
	defer m.mutex.Unlock()

	if watcher, exists := m.watchers[key]; exists {
		// Cancel any pending cleanup
		if watcher.CleanupTimer != nil {
			watcher.CleanupTimer.Stop()
			watcher.CleanupTimer = nil
		}
		// Increment ref count
		atomic.AddInt32(&watcher.RefCount, 1)
		m.app.logDebug("ResourceWatcher: Reusing existing CRD watcher for %s (refCount=%d)", key, atomic.LoadInt32(&watcher.RefCount))
		return key
	}

	// Create new watcher
	ctx, cancel := context.WithCancel(context.Background())
	watcher := &ResourceWatcher{
		Key:          key,
		ResourceType: resource,
		Namespace:    namespace,
		Group:        group,
		Version:      version,
		Resource:     resource,
		IsCRD:        true,
		RefCount:     1,
		Cancel:       cancel,
	}
	m.watchers[key] = watcher

	// Track watcher creation for performance metrics
	m.app.perfMutex.Lock()
	m.app.totalWatchersCreated++
	m.app.perfMutex.Unlock()

	m.app.logDebug("ResourceWatcher: Starting new CRD watcher for %s", key)

	// Start watch loop in goroutine
	go m.app.watchCRDLoop(ctx, group, version, resource, namespace)

	return key
}

// Unsubscribe decrements the refCount for a watcher and schedules cleanup if refCount reaches 0
func (m *ResourceWatcherManager) Unsubscribe(watcherKey string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	watcher, exists := m.watchers[watcherKey]
	if !exists {
		m.app.logDebug("ResourceWatcher: Unsubscribe called for non-existent watcher %s", watcherKey)
		return
	}

	newCount := atomic.AddInt32(&watcher.RefCount, -1)
	m.app.logDebug("ResourceWatcher: Unsubscribe from %s (refCount=%d)", watcherKey, newCount)

	if newCount <= 0 {
		// Schedule cleanup after delay
		watcher.CleanupTimer = time.AfterFunc(WatcherCleanupDelay, func() {
			m.cleanup(watcherKey)
		})
		m.app.logDebug("ResourceWatcher: Scheduled cleanup for %s in %v", watcherKey, WatcherCleanupDelay)
	}
}

// cleanup stops and removes a watcher (called after cleanup delay if refCount is still 0)
func (m *ResourceWatcherManager) cleanup(watcherKey string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	watcher, exists := m.watchers[watcherKey]
	if !exists {
		return
	}

	// Check if refCount is still 0 (someone might have subscribed during the delay)
	if atomic.LoadInt32(&watcher.RefCount) > 0 {
		m.app.logDebug("ResourceWatcher: Cleanup canceled for %s - new subscribers", watcherKey)
		return
	}

	// Stop the watcher
	if watcher.Cancel != nil {
		watcher.Cancel()
	}
	delete(m.watchers, watcherKey)

	// Clean up event statistics for this watcher (prevents memory leak)
	m.app.clearEventStats(watcherKey)

	// Track watcher cleanup for performance metrics
	m.app.perfMutex.Lock()
	m.app.totalWatchersCleaned++
	m.app.perfMutex.Unlock()

	m.app.logDebug("ResourceWatcher: Cleaned up watcher %s", watcherKey)
}

// StopAll stops all active watchers immediately (called on context switch)
func (m *ResourceWatcherManager) StopAll() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.app.logDebug("ResourceWatcher: Stopping all watchers (%d active)", len(m.watchers))

	for key, watcher := range m.watchers {
		// Cancel any pending cleanup timers
		if watcher.CleanupTimer != nil {
			watcher.CleanupTimer.Stop()
		}
		// Stop the watcher
		if watcher.Cancel != nil {
			watcher.Cancel()
		}
		// Clean up event statistics (prevents memory leak)
		m.app.clearEventStats(key)
		m.app.logDebug("ResourceWatcher: Stopped watcher %s", key)
	}

	// Clear all watchers
	m.watchers = make(map[string]*ResourceWatcher)
}
